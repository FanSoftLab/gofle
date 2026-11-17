# syntax=docker/dockerfile:1.4
# Cross-compilation Dockerfile for GoFLE (Wails v2)
# Host: Linux (Docker)
# Targets:
#   CLI  — linux / windows / darwin  ×  amd64 / arm64
#   GUI  — linux (amd64/arm64) + windows (amd64, arm64 if toolchain present)
#   GUI darwin is intentionally unsupported from Linux in Wails v2

# Global build-args (used only after the system stage so apt is not cache-busted
# by version/commit/time that change on every build).
ARG RUN_TESTS=false
ARG APP_NAME=gofle
ARG GO_VERSION=1.27
ARG BUILD_MODE=prod
ARG PLATFORMS=linux/amd64
ARG MULTI=false
ARG CLI_APP_NAME=gofle_cli
ARG GOFILE_VERSION=prod
ARG BUILD_TIME=unknown
ARG BUILD_COMMIT=unknown
ARG BUILD_SOURCE=local
ARG BUILD_TARGET=all

# ---------- system packages (stable stage) ----------
# No build-args here. This layer is reused until the package list or base image
# changes — not on every code/version rebuild.
FROM golang:${GO_VERSION}-bookworm AS system

RUN --mount=type=cache,target=/var/cache/apt,sharing=locked \
    --mount=type=cache,target=/var/lib/apt,sharing=locked \
    rm -f /etc/apt/apt.conf.d/docker-clean && \
    apt-get update && apt-get install -y --no-install-recommends \
        build-essential \
        pkg-config \
        nodejs \
        npm \
        git \
        ca-certificates \
        libgtk-3-dev \
        libwebkit2gtk-4.1-dev \
        gcc-mingw-w64-x86-64 \
        g++-mingw-w64-x86-64 \
        gcc-aarch64-linux-gnu \
        g++-aarch64-linux-gnu \
        nsis

# Compatibility symlink for older Wails templates that look for webkit2gtk-4.0
RUN ln -sf /usr/lib/x86_64-linux-gnu/pkgconfig/webkit2gtk-4.1.pc \
           /usr/lib/x86_64-linux-gnu/pkgconfig/webkit2gtk-4.0.pc 2>/dev/null || \
    ln -sf /usr/lib/pkgconfig/webkit2gtk-4.1.pc \
           /usr/lib/pkgconfig/webkit2gtk-4.0.pc 2>/dev/null || true


# ---------- build stage ----------
FROM system AS builder

ARG RUN_TESTS
ARG APP_NAME
ARG BUILD_MODE
ARG PLATFORMS
ARG MULTI
ARG CLI_APP_NAME
ARG GOFILE_VERSION
ARG BUILD_TIME
ARG BUILD_COMMIT
ARG BUILD_SOURCE
ARG BUILD_TARGET
ARG TARGET_UID=1000
ARG TARGET_GID=1000

# Version ldflags for injection into binaries (changes every build — declared
# only after apt so it cannot invalidate the system package layer).
ARG VERSION_LDFLAGS="-X main.version=${GOFILE_VERSION} -X GoFLE/pkg/version.Version=${GOFILE_VERSION} -X GoFLE/pkg/version.BuildTime=${BUILD_TIME} -X GoFLE/pkg/version.BuildCommit=${BUILD_COMMIT} -X GoFLE/pkg/version.BuildSource=${BUILD_SOURCE}"

WORKDIR /app

# ---------- Go modules (cached unless go.mod / go.sum change) ----------
# Do NOT run `go clean -modcache` here — that forces a full re-download every build.
COPY go.mod* go.sum* ./
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    if [ ! -f go.mod ]; then \
        echo "go.mod not found — initialising module '$APP_NAME'..." && \
        go mod init "$APP_NAME"; \
    fi && \
    if [ ! -f go.sum ]; then touch go.sum; fi && \
    go mod download

# ---------- Frontend deps (cached unless package.json / lock change) ----------
COPY frontend/package.json frontend/package-lock.json* ./frontend/
RUN --mount=type=cache,target=/root/.npm \
    if [ "$BUILD_TARGET" = "gui" ] || [ "$BUILD_TARGET" = "all" ]; then \
        cd frontend && \
        if [ -f package-lock.json ]; then \
            npm ci --no-audit --no-fund; \
        else \
            npm install --no-audit --no-fund; \
        fi && \
        npm install --no-audit --no-fund jsdom; \
    fi

# Source code — this layer invalidates on any code change, but deps layers above stay cached.
COPY . .

# Refresh module graph from full source tree without wiping the download cache.
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go mod tidy && go mod verify

# Install Wails CLI only when GUI is requested (module cache reuse).
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    if [ "$BUILD_TARGET" = "gui" ] || [ "$BUILD_TARGET" = "all" ]; then \
        go install github.com/wailsapp/wails/v2/cmd/wails@latest; \
    fi

RUN mkdir -p build/bin dist

# ---------- Frontend build (deps already installed; only rebuilds when sources change) ----------
RUN --mount=type=cache,target=/root/.npm \
    if [ "$BUILD_TARGET" = "gui" ] || [ "$BUILD_TARGET" = "all" ]; then \
        cd frontend && npm run build && cd ..; \
    else \
        mkdir -p frontend/dist/assets && echo '<!DOCTYPE html><html><body></body></html>' > frontend/dist/index.html; \
    fi && \
    mkdir -p frontend/dist/assets && \
    chown -R "${TARGET_UID}:${TARGET_GID}" frontend/dist

# Generate bindings once on the host platform (Linux) — required before any
# cross-compile that uses -skipbindings
RUN if [ "$BUILD_TARGET" = "gui" ] || [ "$BUILD_TARGET" = "all" ]; then \
        wails generate module; \
    fi

# ---------- Tests / vet ----------
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    touch test_results.log && \
    if [ "$RUN_TESTS" = "true" ]; then \
        go test -v ./... 2>&1 | tee test_results.log; \
    else \
        echo "Tests skipped" > test_results.log; \
    fi
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go vet ./...

# ---------- GUI builds ----------
# Iterate platforms so we can set correct compiler, tags and skip darwin GUI.
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    if [ "$BUILD_TARGET" = "gui" ] || [ "$BUILD_TARGET" = "all" ]; then \
        set -e; \
        IFS=','; set -- $PLATFORMS; \
        FIRST=true; \
        for p in "$@"; do \
            goos="${p%/*}"; \
            goarch="${p#*/}"; \
            echo "════════════════════════════════════════"; \
            echo " GUI  →  $goos/$goarch"; \
            echo "════════════════════════════════════════"; \
            case "$goos" in \
                linux) \
                    TAGS="desktop,webkit2gtk_4_1"; \
                    [ "$BUILD_MODE" = "prod" ] && TAGS="$TAGS,production"; \
                    FLAGS=""; \
                    [ "$BUILD_MODE" = "dev" ] && FLAGS="-debug"; \
                    if [ "$goarch" = "arm64" ]; then \
                        export CC=aarch64-linux-gnu-gcc; \
                        export CXX=aarch64-linux-gnu-g++; \
                        export CGO_ENABLED=1; \
                    fi; \
                    if [ "$FIRST" = "true" ]; then \
                        wails build $FLAGS -platform "$goos/$goarch" \
                            -ldflags="${VERSION_LDFLAGS} -s -w" -tags "$TAGS" -o "$APP_NAME"; \
                    else \
                        wails build $FLAGS -platform "$goos/$goarch" -skipbindings \
                            -ldflags="${VERSION_LDFLAGS} -s -w" -tags "$TAGS" -o "$APP_NAME"; \
                    fi; \
                    if [ "$goarch" = "arm64" ]; then \
                        unset CC CXX; \
                    fi; \
                    if [ -f "build/bin/$APP_NAME" ]; then \
                        mv "build/bin/$APP_NAME" "build/bin/${APP_NAME}-linux-${goarch}"; \
                    fi; \
                    ;; \
                windows) \
                    TAGS="desktop"; \
                    [ "$BUILD_MODE" = "prod" ] && TAGS="$TAGS,production"; \
                    FLAGS=""; \
                    [ "$BUILD_MODE" = "dev" ] && FLAGS="-debug"; \
                    if [ "$goarch" = "amd64" ]; then \
                        export CC=x86_64-w64-mingw32-gcc; \
                        export CXX=x86_64-w64-mingw32-g++; \
                    elif [ "$goarch" = "arm64" ]; then \
                        echo "WARNING: Windows/arm64 GUI cross-compile needs aarch64 MinGW which is not in Debian bookworm."; \
                        echo "         Skipping GUI for windows/arm64. Use -arch amd64 or build natively."; \
                        continue; \
                    else \
                        echo "Unsupported Windows arch: $goarch"; exit 1; \
                    fi; \
                    export CGO_ENABLED=1; \
                    if [ "$FIRST" = "true" ]; then \
                        wails build $FLAGS -platform "$goos/$goarch" \
                            -ldflags="${VERSION_LDFLAGS} -s -w -H windowsgui" -tags "$TAGS" -o "$APP_NAME"; \
                    else \
                        wails build $FLAGS -platform "$goos/$goarch" -skipbindings \
                            -ldflags="${VERSION_LDFLAGS} -s -w -H windowsgui" -tags "$TAGS" -o "$APP_NAME"; \
                    fi; \
                    if [ -f "build/bin/${APP_NAME}.exe" ]; then \
                        mv "build/bin/${APP_NAME}.exe" "build/bin/${APP_NAME}-windows-${goarch}.exe"; \
                    elif [ -f "build/bin/$APP_NAME" ]; then \
                        mv "build/bin/$APP_NAME" "build/bin/${APP_NAME}-windows-${goarch}.exe"; \
                    fi; \
                    unset CC CXX; \
                    ;; \
                darwin) \
                    echo "WARNING: GUI cross-compile to macOS (darwin) is NOT supported by Wails v2 from Linux."; \
                    echo "         Skipping GUI for $goos/$goarch. Build on a real macOS machine (or migrate to Wails v3)."; \
                    ;; \
                *) \
                    echo "Unknown OS for GUI: $goos"; exit 1; \
                    ;; \
            esac; \
            FIRST=false; \
        done; \
    fi

# ---------- CLI builds (pure Go, CGO=0) ----------
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    if [ "$BUILD_TARGET" = "cli" ] || [ "$BUILD_TARGET" = "all" ]; then \
        set -e; \
        IFS=','; set -- $PLATFORMS; \
        for p in "$@"; do \
            goos="${p%/*}"; \
            goarch="${p#*/}"; \
            e=""; [ "$goos" = "windows" ] && e=".exe"; \
            out="build/bin/${CLI_APP_NAME}-${goos}-${goarch}${e}"; \
            if [ "$MULTI" = "false" ]; then \
                out="build/bin/${CLI_APP_NAME}${e}"; \
            fi; \
            echo " CLI  →  $goos/$goarch  →  $out"; \
            CGO_ENABLED=0 GOOS="$goos" GOARCH="$goarch" \
                go build -buildvcs=false \
                    -ldflags="${VERSION_LDFLAGS} -s -w" \
                    -o "$out" ./cmd/gofle; \
        done; \
    fi

# ---------- ownership + cleanup ----------
FROM builder AS chowned
ARG TARGET_UID
ARG TARGET_GID
RUN rm -rf /app/frontend/node_modules /app/frontend/.vite /root/.npm /root/.cache 2>/dev/null || true
# Keep frontend/dist (incl. assets) and set ownership for host UID/GID (default 1000:1000)
RUN mkdir -p /app/frontend/dist/assets && \
    chown -R "${TARGET_UID}:${TARGET_GID}" /app

FROM scratch AS exporter
COPY --from=chowned /app/ /
