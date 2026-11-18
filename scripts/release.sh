#!/usr/bin/env bash
# Local-only release: build everything via build.sh, package + publish via GoReleaser.
#
# Usage:
#   ./scripts/release.sh v2.1.0              # full release (needs GITHUB_TOKEN)
#   ./scripts/release.sh v2.1.0 --snapshot   # build + package, no publish
#   ./scripts/release.sh v2.1.0 --skip-build # only package existing dist/ + publish
#   ./scripts/release.sh v2.1.0 --no-publish # build + package, create tag, but don't publish release
#
# Prerequisites:
#   - goreleaser: go install github.com/goreleaser/goreleaser/v2@latest
#   - Docker (for ./build.sh)
#   - GITHUB_TOKEN with repo scope (for publish) — will be prompted securely
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

VERSION_ARG=""
SNAPSHOT=false
SKIP_BUILD=false
NO_PUBLISH=false

usage() {
  cat <<EOFU
Usage: $0 vX.Y.Z [--snapshot] [--skip-build] [--no-publish]

  --snapshot     Package only, do not publish to GitHub
  --skip-build   Do not run build.sh (use existing files in dist/)
  --no-publish   Build and package, create git tag, but don't publish release
EOFU
  exit 0
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    -h|--help) usage ;;
    --snapshot) SNAPSHOT=true; shift ;;
    --skip-build) SKIP_BUILD=true; shift ;;
    --no-publish) NO_PUBLISH=true; shift ;;
    v*) VERSION_ARG="$1"; shift ;;
    *)
      echo "Unknown argument: $1" >&2
      usage
      ;;
  esac
done

if [[ -z "$VERSION_ARG" ]]; then
  echo "Usage: $0 vX.Y.Z [--snapshot] [--skip-build] [--no-publish]" >&2
  exit 1
fi

TAG="$VERSION_ARG"
[[ "$TAG" == v* ]] || TAG="v${TAG}"
VERSION="${TAG#v}"

if ! command -v goreleaser >/dev/null 2>&1; then
  echo "goreleaser not found. Install:" >&2
  echo "  go install github.com/goreleaser/goreleaser/v2@latest" >&2
  exit 1
fi

if [[ -n "$(git status --porcelain 2>/dev/null || true)" ]]; then
  echo "⚠  Working tree is not clean. Continue? (y/N)"
  read -r ans
  [[ "$ans" == y || "$ans" == Y ]] || exit 1
fi

if ! git rev-parse "$TAG" >/dev/null 2>&1; then
  echo "Creating tag $TAG …"
  git tag -a "$TAG" -m "$TAG"
else
  echo "Tag $TAG already exists."
fi

# ── build.sh: GUI only (CLI is built by GoReleaser — pure Go, cross-compiles easily) ──
if [[ "$SKIP_BUILD" == false ]]; then
  echo "──────────────────────────────────────────────────"
  echo "build.sh  →  GUI only  ($VERSION)"
  echo "──────────────────────────────────────────────────"
  # linux: GUI amd64 only (arm64 GUI cross-compile from x86_64 fails — needs native arm64 builder)
  # windows: GUI amd64 only (arm64 MinGW not in Debian bookworm)
  # macOS skipped — no simple cross-build for Wails GUI
  ./build.sh -os linux -arch amd64 -target gui -v "$VERSION"
  # Normalize name (first build uses MULTI=false → no platform suffix)
  [[ -f dist/gofle ]] && mv dist/gofle dist/gofle-linux-amd64
  ./build.sh -os windows -arch amd64 -target gui -v "$VERSION"
  echo ""
  echo "dist/ after build:"
  ls -la dist/ 2>/dev/null || true
fi

# Required GUI prebuilts for GoReleaser
# GUI: linux/amd64, windows/amd64 only (arm64 GUI cross-compile not supported from x86_64)
gui_missing=0
for pair in \
  linux/amd64 \
  windows/amd64
do
  os="${pair%/*}"
  arch="${pair#*/}"
  ext=""
  [[ "$os" == windows ]] && ext=".exe"
  f="dist/gofle-${os}-${arch}${ext}"
  if [[ ! -f "$f" ]]; then
    echo "missing prebuilt GUI: $f" >&2
    gui_missing=1
  fi
done

if [[ "$gui_missing" -ne 0 ]]; then
  echo "Run without --skip-build, or build missing GUI targets first." >&2
  exit 1
fi

# Copy prebuilt GUI binaries to staging for GoReleaser extra_files
mkdir -p .goreleaser/staging
cp dist/gofle-linux-amd64 .goreleaser/staging/gofle-linux-amd64
cp dist/gofle-windows-amd64.exe .goreleaser/staging/gofle-windows-amd64.exe

echo "──────────────────────────────────────────────────"
echo "GoReleaser  →  package + release ($TAG)"
echo "──────────────────────────────────────────────────"

export GORELEASER_CURRENT_TAG="$TAG"

if [[ "$SNAPSHOT" == true ]]; then
  goreleaser release --snapshot --clean --skip=publish
  echo "Snapshot done (not published). See ./dist/"
  exit 0
fi

# Prompt for GITHUB_TOKEN securely (like a password)
if [[ -z "${GITHUB_TOKEN:-}" ]]; then
  echo "GITHUB_TOKEN is not set." >&2
  read -rsp "Enter GITHUB_TOKEN (input hidden): " GITHUB_TOKEN
  echo
  if [[ -z "$GITHUB_TOKEN" ]]; then
    echo "GITHUB_TOKEN is required for publishing." >&2
    exit 1
  fi
  export GITHUB_TOKEN
fi

# Push tag
git push origin "$TAG" 2>/dev/null || git push origin "refs/tags/$TAG"

if [[ "$NO_PUBLISH" == true ]]; then
  echo "──────────────────────────────────────────────────"
  echo "Release $TAG packaged (not published)."
  echo "  Tag pushed: https://github.com/FanSoftLab/gofle/releases/tag/$TAG"
  echo "  Run 'goreleaser release --clean' manually to publish."
  echo "──────────────────────────────────────────────────"
  exit 0
fi

goreleaser release --clean

echo "──────────────────────────────────────────────────"
echo "Release $TAG published."
echo "  https://github.com/FanSoftLab/gofle/releases/tag/$TAG"
echo "──────────────────────────────────────────────────"
