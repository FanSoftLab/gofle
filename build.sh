#!/bin/bash
set -e

APP_NAME="GoFLE"
CLI_APP_NAME="gofle"
TARGET_UID=$(id -u)
TARGET_GID=$(id -g)
RUN_TESTS=false

# Defaults
MODE="prod"
TARGET_OS="linux"
TARGET_ARCH="amd64"
BUILD_TARGET="all"

usage() {
    cat <<EOF
Usage: $0 [options]

  -run-tests          Run go tests + vet inside the build container
  -mode <dev|prod>    Build mode (default: prod)
  -os <target>        linux | mac | windows | all   (default: linux)
  -arch <target>      amd64 | arm64 | all           (default: amd64)
  -target <target>    cli | gui | all               (default: all)
  -cli-name <name>    CLI binary name (default: gofle)
  -h, --help          Show this help
EOF
}

while [[ $# -gt 0 ]]; do
    case "$1" in
        -run-tests|--run-tests) RUN_TESTS=true; shift;;
        -mode) MODE="$2"; shift 2;;
        -os) TARGET_OS="$2"; shift 2;;
        -arch) TARGET_ARCH="$2"; shift 2;;
        -target) BUILD_TARGET="$2"; shift 2;;
        -cli-name) CLI_APP_NAME="$2"; shift 2;;
        -h|--help) usage; exit 0;;
        *) echo "Unknown argument: $1"; usage; exit 1;;
    esac
done

# Validate mode
case "$MODE" in
    dev|prod) ;;
    *) echo "Invalid -mode: $MODE (use dev or prod)"; exit 1;;
esac

# Validate target
case "$BUILD_TARGET" in
    cli|gui|all) ;;
    *) echo "Invalid -target: $BUILD_TARGET (use cli|gui|all)"; exit 1;;
esac

# Expand OS targets
case "$TARGET_OS" in
    linux) OS_LIST=(linux);;
    mac|darwin) OS_LIST=(darwin);;
    windows) OS_LIST=(windows);;
    all) OS_LIST=(linux windows darwin);;
    *) echo "Invalid -os: $TARGET_OS (use linux|mac|windows|all)"; exit 1;;
esac

# Expand architecture targets
case "$TARGET_ARCH" in
    amd64) ARCH_LIST=(amd64);;
    arm64) ARCH_LIST=(arm64);;
    all) ARCH_LIST=(amd64 arm64);;
    *) echo "Invalid -arch: $TARGET_ARCH (use amd64|arm64|all)"; exit 1;;
esac

# Build the GOOS/GOARCH platform list (comma-separated for wails build -platform)
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

# Legacy single-binary behaviour only when building the native prod linux/amd64.
# Anything else (dev, multiple OSes, multiple arches) is treated as a multi-build
# and its artifacts are placed under ./dist.
if [ "$PLATFORMS" = "linux/amd64" ] && [ "$MODE" = "prod" ]; then
    MULTI=false
else
    MULTI=true
fi

CDPATH= cd -- "$(dirname -- "$0")" && pwd > /dev/null

# Drop the orphaned scratch dir left behind by a previously interrupted run.
# `dist` is intentionally NOT touched here: it holds the user's previous binaries
# and is only replaced once a new multi-target build has actually succeeded.
rm -rf .buildout 2>/dev/null || true

echo "──────────────────────────────────────────────────"
echo "Building $APP_NAME"
echo "  mode:    $MODE"
echo "  targets: $PLATFORMS"
echo "  target:  $BUILD_TARGET"
echo "  cli:     $CLI_APP_NAME"
echo "  tests:   $RUN_TESTS"
echo "──────────────────────────────────────────────────"

INTERRUPTED=0
BUILD_PID=""
# OUT_DIR must exist before the traps are installed so cleanup never runs with an
# empty path.
OUT_DIR=".buildout"
# CLEANED guards cleanup so a single interrupt (INT→EXIT) does not run the
# Docker builder prune twice.
CLEANED=0
cleanup() {
    if [ "$CLEANED" = "1" ]; then
        return
    fi
    CLEANED=1

    # Guarantee the in-flight docker build is killed, even if SIGINT never
    # reached docker directly (e.g. launched through an IDE wrapper).
    if [ -n "$BUILD_PID" ] && kill -0 "$BUILD_PID" 2>/dev/null; then
        kill "$BUILD_PID" 2>/dev/null || true
        wait "$BUILD_PID" 2>/dev/null || true
    fi

    # Always drop our own temporary scratch dir so an interrupted or failed build
    # does NOT litter the working tree, regardless of how the script ends.
    rm -rf "$OUT_DIR" 2>/dev/null || true

    # Prune Docker builder artifacts on any exit. The --label filter only removes
    # our own dangling builder stages, so this is cheap and safe.
    docker image prune -f --label=stage=builder > /dev/null 2>&1 || true
    docker builder prune -f > /dev/null 2>&1 || true
}

# Run cleanup on every exit path (normal, error under set -e, INT/TERM).
trap 'INTERRUPTED=1; cleanup' INT TERM
trap 'cleanup' EXIT

OUT_DIR=".buildout"
rm -rf "$OUT_DIR" 2>/dev/null || true

export DOCKER_BUILDKIT=1
# Stamp the gofle CLI version from git (falls back to "dev" outside a repo).
CLI_VERSION="$(git describe --tags --always --dirty 2>/dev/null || echo dev)"

docker build \
  --build-arg RUN_TESTS="$RUN_TESTS" \
  --build-arg GOFILE_VERSION="$CLI_VERSION" \
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

# The docker export placed the whole container tree in $OUT_DIR (.buildout).
# Move the actual build products out before cleanup removes $OUT_DIR on exit, so
# the generated binary actually reaches the host (it used to be deleted with the
# scratch dir). Single-target builds emit build/bin/<app> at the repo root;
# multi-target (wails -platform) builds emit platform binaries under dist/.
if [ -d "$OUT_DIR/build/bin" ]; then
    if [ "$MULTI" = "false" ]; then
        # A failing cp MUST abort the script: silently keeping the previous
        # binary (e.g. ETXTBSY because the app is still running) makes a stale
        # build look freshly shipped.
        if [ "$BUILD_TARGET" = "gui" ] || [ "$BUILD_TARGET" = "all" ]; then
            cp -r "$OUT_DIR/build/bin/$APP_NAME" "./$APP_NAME" || {
                echo "ERROR: failed to install ./$APP_NAME — close the running GoFLE and re-run."
                exit 1
            }
            chmod +x "./$APP_NAME" 2>/dev/null || true
        fi
        # GoFLE-CLI: a plain `$CLI_APP_NAME` is emitted for single-target builds.
        if [ "$BUILD_TARGET" = "cli" ] || [ "$BUILD_TARGET" = "all" ]; then
            if [ -f "$OUT_DIR/build/bin/$CLI_APP_NAME" ]; then
                cp -r "$OUT_DIR/build/bin/$CLI_APP_NAME" "./$CLI_APP_NAME" || {
                    echo "ERROR: failed to install ./$CLI_APP_NAME — close the running instance and re-run."
                    exit 1
                }
                chmod +x "./$CLI_APP_NAME" 2>/dev/null || true
            fi
        fi
    else
        mkdir -p dist
        if [ "$BUILD_TARGET" = "gui" ] || [ "$BUILD_TARGET" = "all" ]; then
            cp -r "$OUT_DIR/build/bin/." dist/ 2>/dev/null || true
        fi
        if [ "$BUILD_TARGET" = "cli" ] || [ "$BUILD_TARGET" = "all" ]; then
            cp -r "$OUT_DIR/build/bin/." dist/ 2>/dev/null || true
        fi
    fi
fi
[ -f "$OUT_DIR/test_results.log" ] && cp "$OUT_DIR/test_results.log" ./test_results.log 2>/dev/null || true

echo "──────────────────────────────────────────────────"
echo "Build complete. Container working tree synced to: $(pwd)"
echo "──────────────────────────────────────────────────"
