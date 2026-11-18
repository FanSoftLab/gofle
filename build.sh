#!/bin/bash
set -euo pipefail

APP_NAME="gofle"
CLI_APP_NAME="gofle_cli"
TARGET_UID=$(id -u)
TARGET_GID=$(id -g)
RUN_TESTS=false
NO_CACHE=false

# Defaults
MODE="prod"
TARGET_OS="linux"
TARGET_ARCH="amd64"
BUILD_TARGET="all"
EXPLICIT_VERSION=""

usage() {
    cat <<EOF
Usage: $0 [options]

  -run-tests          Run go tests + vet inside the build container
  -no-cache           Force full rebuild (ignore Docker/BuildKit layer cache)
  -mode <dev|prod>    Build mode (default: prod)
  -os <target>        linux,mac,windows,all or comma-separated list (default: linux)
  -arch <target>      amd64,arm64,all or comma-separated list       (default: amd64)
  -target <target>    cli | gui | all               (default: all)
  -cli-name <name>    CLI binary name (default: gofle_cli)
  -v, --version <ver> Explicit version string (SemVer, e.g. 2.1.0). If omitted,
                      derived from git: tag (v2.1.0) or short commit (dev-abc123).
  -h, --help          Show this help

Notes:
  • CLI is fully cross-compiled (linux/windows/darwin × amd64/arm64).
  • GUI Linux  — native binary (webkit2gtk).
  • GUI Windows — cross-compiled via MinGW (amd64 only; arm64 not available in Debian bookworm).
  • GUI macOS  — NOT supported from Linux (Wails v2 limitation).
    Use a real Mac or Wails v3 for macOS GUI.
  • BuildKit cache is enabled by default. Layers re-run only when:
      - apt packages: system stage changes (rare)
      - go.mod/go.sum change: Go module re-download
      - package-lock.json change: npm ci re-run
      - code-only change: reuse all deps, rebuild app
  • Use -no-cache for a completely clean build.
EOF
}

while [[ $# -gt 0 ]]; do
    case "$1" in
        -run-tests|--run-tests) RUN_TESTS=true; shift;;
        -no-cache|--no-cache) NO_CACHE=true; shift;;
        -mode) MODE="$2"; shift 2;;
        -os) TARGET_OS="$2"; shift 2;;
        -arch) TARGET_ARCH="$2"; shift 2;;
        -target) BUILD_TARGET="$2"; shift 2;;
        -cli-name) CLI_APP_NAME="$2"; shift 2;;
        -v|--version) EXPLICIT_VERSION="$2"; shift 2;;
        -h|--help) usage; exit 0;;
        *) echo "Unknown argument: $1"; usage; exit 1;;
    esac
done

# Validate mode
case "$MODE" in
    dev|prod) ;;
    *) echo "Invalid -mode: $MODE (use dev or prod)" >&2; exit 1;;
esac

# Validate target
case "$BUILD_TARGET" in
    cli|gui|all) ;;
    *) echo "Invalid -target: $BUILD_TARGET (use cli|gui|all)" >&2; exit 1;;
esac

# Validate and expand OS targets (supports comma-separated: linux,windows)
# Valid values: linux, mac, darwin, windows, all
parse_os_list() {
    local input="$1"
    local result=()
    IFS=',' read -ra parts <<< "$input"
    for part in "${parts[@]}"; do
        case "$part" in
            linux) result+=(linux);;
            mac|darwin) result+=(darwin);;
            windows) result+=(windows);;
            all) result+=(linux windows darwin);;
            *) echo "Invalid -os value: $part (use linux,mac,darwin,windows,all or comma-separated list)" >&2; exit 1;;
        esac
    done
    # Deduplicate
    printf '%s\n' "${result[@]}" | sort -u
}

# Validate and expand arch targets (supports comma-separated: amd64,arm64)
# Valid values: amd64, arm64, all
parse_arch_list() {
    local input="$1"
    local result=()
    IFS=',' read -ra parts <<< "$input"
    for part in "${parts[@]}"; do
        case "$part" in
            amd64) result+=(amd64);;
            arm64) result+=(arm64);;
            all) result+=(amd64 arm64);;
            *) echo "Invalid -arch value: $part (use amd64,arm64,all or comma-separated list)" >&2; exit 1;;
        esac
    done
    # Deduplicate
    printf '%s\n' "${result[@]}" | sort -u
}

# Expand OS targets
mapfile -t OS_LIST < <(parse_os_list "$TARGET_OS")

# Expand architecture targets
mapfile -t ARCH_LIST < <(parse_arch_list "$TARGET_ARCH")

# Build the GOOS/GOARCH platform list (comma-separated)
PLATFORMS=""
for os in "${OS_LIST[@]}"; do
    for arch in "${ARCH_LIST[@]}"; do
        if [ -z "$PLATFORMS" ]; then
            PLATFORMS="$os/$arch"
        else
            PLATFORMS="$PLATFORMS,$os/$arch"
        fi
    done
done

# Warn early about macOS GUI
if [[ "$BUILD_TARGET" == "gui" || "$BUILD_TARGET" == "all" ]]; then
    if [[ "$PLATFORMS" == *darwin* ]]; then
        echo "⚠  GUI for macOS (darwin) will be SKIPPED — Wails v2 cannot cross-compile GUI from Linux."
        echo "   CLI for darwin will still be built."
    fi
fi

# Single-binary behaviour only for native prod linux/amd64
if [ "$PLATFORMS" = "linux/amd64" ] && [ "$MODE" = "prod" ]; then
    MULTI=false
else
    MULTI=true
fi


# ─── Version derivation ────────────────────────────────────────────
# Priority: 1) explicit -v/--version, 2) git tag (vX.Y.Z), 3) git describe (dev-<commit>)
# The version is injected via -ldflags into both CLI (main.version) and GUI (GoFLE/pkg/version.Version)
if [ -n "$EXPLICIT_VERSION" ]; then
    CLI_VERSION="$EXPLICIT_VERSION"
    BUILD_SOURCE="release"
else
    # Try to get the latest semver tag (vX.Y.Z)
    TAG=$(git describe --tags --match 'v[0-9]*.[0-9]*.[0-9]*' --exact-match 2>/dev/null || true)
    if [ -n "$TAG" ]; then
        CLI_VERSION="${TAG#v}"  # strip leading 'v'
        BUILD_SOURCE="tag"
    else
        # No exact semver tag: use git describe for dev builds
        DESCRIBE=$(git describe --tags --always --dirty 2>/dev/null || echo "dev")
        if [[ "$DESCRIBE" == v[0-9]*.[0-9]*.[0-9]* ]]; then
            # It's a tag but not exact match (we're past the tag)
            CLI_VERSION="dev-${DESCRIBE#v}"
        else
            CLI_VERSION="dev-${DESCRIBE}"
        fi
        BUILD_SOURCE="local"
    fi
fi

# Build metadata
BUILD_TIME=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
BUILD_COMMIT=$(git rev-parse --short HEAD 2>/dev/null || echo "unknown")

echo "  version:   $CLI_VERSION"
echo "  source:    $BUILD_SOURCE"
echo "  time:      $BUILD_TIME"
echo "  commit:    $BUILD_COMMIT"

# Verify required files exist
if [ ! -f "Dockerfile" ]; then
    echo "ERROR: Dockerfile not found in project root" >&2
    exit 1
fi


CDPATH= cd -- "$(dirname -- "$0")" && pwd > /dev/null

rm -rf .buildout 2>/dev/null || true

echo "──────────────────────────────────────────────────"
echo "Building $APP_NAME"
echo "  mode:    $MODE"
echo "  targets: $PLATFORMS"
echo "  target:  $BUILD_TARGET"
echo "  cli:     $CLI_APP_NAME"
echo "  tests:   $RUN_TESTS"
echo "  multi:   $MULTI"
echo "──────────────────────────────────────────────────"

INTERRUPTED=0
BUILD_PID=""
OUT_DIR=".buildout"
CLEANED=0

cleanup() {
    if [ "$CLEANED" = "1" ]; then
        return
    fi
    CLEANED=1

    if [ -n "$BUILD_PID" ] && kill -0 "$BUILD_PID" 2>/dev/null; then
        kill "$BUILD_PID" 2>/dev/null || true
        wait "$BUILD_PID" 2>/dev/null || true
    fi

    rm -rf "$OUT_DIR" 2>/dev/null || true

    # Do not run `docker builder prune` here — it wipes BuildKit layer cache and
    # forces a full re-download of Go/npm deps on the next build.
    docker image prune -f --label=stage=builder > /dev/null 2>&1 || true
}

trap 'INTERRUPTED=1; cleanup' INT TERM
trap 'cleanup' EXIT

OUT_DIR=".buildout"
rm -rf "$OUT_DIR" 2>/dev/null || true

export DOCKER_BUILDKIT=1
export BUILDKIT_PROGRESS="${BUILDKIT_PROGRESS:-auto}"

# Dependency layers (apt / go mod / npm) stay cached across builds.
# They re-run only when lockfiles change, or when -no-cache is passed.
DOCKER_CACHE_OPTS=()
if [ "$NO_CACHE" = "true" ]; then
    echo "Full rebuild (-no-cache): Docker layer cache disabled."
    DOCKER_CACHE_OPTS+=(--no-cache)
else
    echo "Incremental build (BuildKit cache enabled):"
    echo "  • apt packages             → cached in 'system' stage (no reinstall)"
    echo "  • go.mod / go.sum change   → re-download Go modules"
    echo "  • package-lock.json change → re-run npm ci"
    echo "  • code-only change         → reuse deps, rebuild app"
fi

docker build \
  "${DOCKER_CACHE_OPTS[@]}" \
  --build-arg RUN_TESTS="$RUN_TESTS" \
  --build-arg GOFILE_VERSION="$CLI_VERSION" \
  --build-arg BUILD_TIME="$BUILD_TIME" \
  --build-arg BUILD_COMMIT="$BUILD_COMMIT" \
  --build-arg BUILD_SOURCE="$BUILD_SOURCE" \
  --build-arg APP_NAME="$APP_NAME" \
  --build-arg BUILD_MODE="$MODE" \
  --build-arg PLATFORMS="$PLATFORMS" \
  --build-arg MULTI="$MULTI" \
  --build-arg TARGET_UID="$TARGET_UID" \
  --build-arg TARGET_GID="$TARGET_GID" \
  --build-arg CLI_APP_NAME="$CLI_APP_NAME" \
  --build-arg BUILD_TARGET="$BUILD_TARGET" \
  --target exporter \
  --output "type=local,dest=$OUT_DIR" . &
BUILD_PID=$!
wait "$BUILD_PID" || BUILD_RC=$?
if [ "${BUILD_RC:-0}" -ne 0 ]; then
    echo "Docker build failed (rc=${BUILD_RC:-1})"
    exit "${BUILD_RC:-1}"
fi
BUILD_PID=""

chown -R "${TARGET_UID}:${TARGET_GID}" . 2>/dev/null || true

# Collect artefacts
if [ -d "$OUT_DIR/build/bin" ]; then
    if [ "$MULTI" = "false" ]; then
        # Single linux/amd64 prod
        mkdir -p dist
        if [ "$BUILD_TARGET" = "gui" ] || [ "$BUILD_TARGET" = "all" ]; then
            # Prefer the normalised name, fall back to plain name
            src=""
            if [ -f "$OUT_DIR/build/bin/${APP_NAME}-linux-amd64" ]; then
                src="$OUT_DIR/build/bin/${APP_NAME}-linux-amd64"
            elif [ -f "$OUT_DIR/build/bin/$APP_NAME" ]; then
                src="$OUT_DIR/build/bin/$APP_NAME"
            fi
            if [ -n "$src" ]; then
                cp -f "$src" "dist/$APP_NAME" || {
                    echo "ERROR: failed to install ./$APP_NAME — close the running instance and re-run."
                    exit 1
                }
                chmod +x "dist/$APP_NAME" 2>/dev/null || true
            fi
        fi
        if [ "$BUILD_TARGET" = "cli" ] || [ "$BUILD_TARGET" = "all" ]; then
            if [ -f "$OUT_DIR/build/bin/$CLI_APP_NAME" ]; then
                cp -f "$OUT_DIR/build/bin/$CLI_APP_NAME" "dist/$CLI_APP_NAME" || {
                    echo "ERROR: failed to install ./$CLI_APP_NAME — close the running instance and re-run."
                    exit 1
                }
                chmod +x "dist/$CLI_APP_NAME" 2>/dev/null || true
            fi
        fi
    else
        # Multi-target
        mkdir -p dist
        cp -f "$OUT_DIR/build/bin/"* dist/ 2>/dev/null || true
        echo ""
        echo "Artefacts placed in ./dist/ :"
        ls -la dist/ 2>/dev/null || true
    fi
fi

[ -f "$OUT_DIR/test_results.log" ] && cp "$OUT_DIR/test_results.log" ./test_results.log 2>/dev/null || true

# Generate SHA256 checksums for all artifacts in dist/
if [ -d "dist" ] && [ -n "$(ls -A dist/ 2>/dev/null)" ]; then
    echo "Generating checksums..."
    (cd dist && sha256sum * > checksums.txt 2>/dev/null || true)
    echo "Checksums saved to dist/checksums.txt"
    cat dist/checksums.txt 2>/dev/null || true
fi

echo "──────────────────────────────────────────────────"
echo "Build complete. Working directory: $(pwd)"
echo "Binaries are in ./dist/"
echo "──────────────────────────────────────────────────"
