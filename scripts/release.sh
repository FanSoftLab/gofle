#!/usr/bin/env bash
# Local-only release: build everything via build.sh, package + publish via GoReleaser.
#
# Usage:
#   ./scripts/release.sh v2.1.0              # full release (needs GITHUB_TOKEN)
#   ./scripts/release.sh v2.1.0 --snapshot   # build + package, no publish
#   ./scripts/release.sh v2.1.0 --skip-build # only package existing dist/ + publish
#
# Prerequisites:
#   - goreleaser: go install github.com/goreleaser/goreleaser/v2@latest
#   - Docker (for ./build.sh)
#   - GITHUB_TOKEN with repo scope (for publish)
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

VERSION_ARG=""
SNAPSHOT=false
SKIP_BUILD=false

usage() {
  cat <<EOFU
Usage: $0 vX.Y.Z [--snapshot] [--skip-build]

  --snapshot     Package only, do not publish to GitHub
  --skip-build   Do not run build.sh (use existing files in dist/)
EOFU
  exit 0
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    -h|--help) usage ;;
    --snapshot) SNAPSHOT=true; shift ;;
    --skip-build) SKIP_BUILD=true; shift ;;
    v*) VERSION_ARG="$1"; shift ;;
    *)
      echo "Unknown argument: $1" >&2
      usage
      ;;
  esac
done

if [[ -z "$VERSION_ARG" ]]; then
  echo "Usage: $0 vX.Y.Z [--snapshot] [--skip-build]" >&2
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

# ── build.sh: CLI + GUI for all platforms ──────────────────────────────────
if [[ "$SKIP_BUILD" == false ]]; then
  echo "──────────────────────────────────────────────────"
  echo "build.sh  →  CLI + GUI  ($VERSION, all OS/arch)"
  echo "──────────────────────────────────────────────────"
  # linux + windows only (macOS skipped — no simple cross-build for Wails GUI)
  ./build.sh -os linux -arch all -target all -v "$VERSION"
  ./build.sh -os windows -arch all -target all -v "$VERSION"
  echo ""
  echo "dist/ after build:"
  ls -la dist/ 2>/dev/null || true
fi

# Required CLI prebuilts for GoReleaser
missing=0
for pair in \
  linux/amd64 linux/arm64 \
  windows/amd64 windows/arm64
do
  os="${pair%/*}"
  arch="${pair#*/}"
  ext=""
  [[ "$os" == windows ]] && ext=".exe"
  f="dist/gofle-${os}-${arch}${ext}"
  if [[ ! -f "$f" ]]; then
    echo "missing prebuilt CLI: $f" >&2
    missing=1
  fi
done

# Required GUI prebuilts for GoReleaser (extra_files: ./dist/GoFLE*)
gui_missing=0
for pair in \
  linux/amd64 linux/arm64 \
  windows/amd64
do
  os="${pair%/*}"
  arch="${pair#*/}"
  ext=""
  [[ "$os" == windows ]] && ext=".exe"
  f="dist/GoFLE-${os}-${arch}${ext}"
  if [[ ! -f "$f" ]]; then
    echo "missing prebuilt GUI: $f" >&2
    gui_missing=1
  fi
done

if [[ "$missing" -ne 0 ]] || [[ "$gui_missing" -ne 0 ]]; then
  echo "Run without --skip-build, or build missing targets first." >&2
  exit 1
fi

echo "──────────────────────────────────────────────────"
echo "GoReleaser  →  package + release ($TAG)"
echo "──────────────────────────────────────────────────"

export GORELEASER_CURRENT_TAG="$TAG"

if [[ "$SNAPSHOT" == true ]]; then
  goreleaser release --snapshot --clean --skip=publish
  echo "Snapshot done (not published). See ./dist/"
  exit 0
fi

if [[ -z "${GITHUB_TOKEN:-}" ]]; then
  echo "GITHUB_TOKEN is not set." >&2
  echo "  export GITHUB_TOKEN=ghp_…   # classic token with 'repo' scope" >&2
  exit 1
fi

git push origin "$TAG" 2>/dev/null || git push origin "refs/tags/$TAG"

goreleaser release --clean

echo "──────────────────────────────────────────────────"
echo "Release $TAG published."
echo "  https://github.com/sankam-nikolya/GoFLE/releases/tag/$TAG"
echo "──────────────────────────────────────────────────"