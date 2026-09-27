# syntax=docker/dockerfile:1.4
# Cross-compilation Dockerfile for GoFLE (Wails v2)
# Host: Linux (Docker)
# Targets:
#   CLI  — linux / windows / darwin  ×  amd64 / arm64
#   GUI  — linux (amd64/arm64) + windows (amd64, arm64 if toolchain present)
#   GUI darwin is intentionally unsupported from Linux in Wails v2

ARG RUN_TESTS=false
ARG APP_NAME=GoFLE
ARG BUILD_MODE=prod
ARG PLATFORMS=linux/amd64
ARG MULTI=false
ARG CLI_APP_NAME=gofle
ARG GOFILE_VERSION=dev
ARG BUILD_TIME=unknown
ARG BUILD_COMMIT=unknown
ARG BUILD_SOURCE=local
ARG BUILD_TARGET=all

FROM golang:1.26-bookworm AS builder

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
ARG TARGET_UID=0
ARG TARGET_GID=0

# Version ldflags for injection into binaries
ARG VERSION_LDFLAGS="-X main.version=${GOFILE_VERSION} -X GoFLE/pkg/version.Version=${GOFILE_VERSION} -X GoFLE/pkg/version.BuildTime=${BUILD_TIME} -X GoFLE/pkg/version.BuildCommit=${BUILD_COMMIT} -X GoFLE/pkg/version.BuildSource=${BUILD_SOURCE}"

# ---------- system packages ----------
# Always install Node + Git.
# For GUI we need GTK/WebKit (Linux) and MinGW (Windows cross).
RUN apt-get update && apt-get install -y --no-install-recommends \
        build-essential \
        pkg-config \
        nodejs \
        npm \
        git \
        ca-certificates \
    && if [ "$BUILD_TARGET" = "gui" ] || [ "$BUILD_TARGET" = "all" ]; then \
        apt-get install -y --no-install-recommends \
            libgtk-3-dev \
            libwebkit2gtk-4.1-dev \
            gcc-mingw-w64-x86-64 \
            g++-mingw-w64-x86-64 \
            nsis \
            ; \
    fi \
    && rm -rf /var/lib/apt/lists/*

# Compatibility symlink for older Wails templates that look for webkit2gtk-4.0
RUN if [ "$BUILD_TARGET" = "gui" ] || [ "$BUILD_TARGET" = "all" ]; then \
        ln -sf /usr/lib/x86_64-linux-gnu/pkgconfig/webkit2gtk-4.1.pc \
               /usr/lib/x86_64-linux-gnu/pkgconfig/webkit2gtk-4.0.pc 2>/dev/null || \
        ln -sf /usr/lib/pkgconfig/webkit2gtk-4.1.pc \
               /usr/lib/pkgconfig/webkit2gtk-4.0.pc 2>/dev/null || true; \
    fi

WORKDIR /app

# ---------- Go modules (layer cache) ----------
COPY go.mod* go.sum* ./
RUN go clean -modcache
RUN if [ ! -f go.mod ]; then \
        echo "go.mod not found — initialising module '$APP_NAME'..." && \
        go mod init "$APP_NAME"; \
    fi && \
    if [ ! -f go.sum ]; then touch go.sum; fi && \
    go mod download 2>/dev/null || true

# ---------- Frontend deps (layer cache) ----------
COPY frontend/package.json frontend/package-lock.json* ./frontend/
RUN if [ "$BUILD_TARGET" = "gui" ] || [ "$BUILD_TARGET" = "all" ]; then \
        cd frontend && (npm ci --no-audit --no-fund 2>/dev/null || npm install --no-audit --no-fund); \
    fi

COPY . .

RUN go mod tidy && go mod verify

# Install Wails CLI only when GUI is requested
RUN if [ "$BUILD_TARGET" = "gui" ] || [ "$BUILD_TARGET" = "all" ]; then \
        go install github.com/wailsapp/wails/v2/cmd/wails@latest; \
    fi

RUN mkdir -p build/bin dist

# ---------- Frontend build ----------
RUN if [ "$BUILD_TARGET" = "gui" ] || [ "$BUILD_TARGET" = "all" ]; then \
        cd frontend && npm run build && cd ..; \
    else \
        mkdir -p frontend/dist && echo '<!DOCTYPE html><html><body></body></html>' > frontend/dist/index.html; \
    fi

# Generate bindings once on the host platform (Linux) — required before any
# cross-compile that uses -skipbindings
RUN if [ "$BUILD_TARGET" = "gui" ] || [ "$BUILD_TARGET" = "all" ]; then \
        wails generate module; \
    fi

# ---------- Tests / vet ----------
RUN touch test_results.log && \
    if [ "$RUN_TESTS" = "true" ]; then \
        go test -v ./... 2>&1 | tee test_results.log; \
    else \
        echo "Tests skipped" > test_results.log; \
    fi
RUN go vet ./...

# ---------- GUI builds ----------
# Iterate platforms so we can set correct compiler, tags and skip darwin GUI.
RUN if [ "$BUILD_TARGET" = "gui" ] || [ "$BUILD_TARGET" = "all" ]; then \
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
                    if [ "$FIRST" = "true" ]; then \
                        wails build $FLAGS -platform "$goos/$goarch" \
                            -ldflags="${VERSION_LDFLAGS} -s -w" -tags "$TAGS" -o "$APP_NAME"; \
                    else \
                        wails build $FLAGS -platform "$goos/$goarch" -skipbindings \
                            -ldflags="${VERSION_LDFLAGS} -s -w" -tags "$TAGS" -o "$APP_NAME"; \
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
RUN if [ "$BUILD_TARGET" = "cli" ] || [ "$BUILD_TARGET" = "all" ]; then \
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
RUN if [ -d "/app/frontend/node_modules" ] || [ -d "/app/frontend/dist" ] || [ -d "/app/frontend/.vite" ]; then \
        rm -rf /app/frontend/node_modules /app/frontend/dist /app/frontend/.vite \
        && rm -rf /root/.npm /root/.cache; \
    fi
RUN if [ "${TARGET_UID}" != "0" ] || [ "${TARGET_GID}" != "0" ]; then \
        chown -R "${TARGET_UID}:${TARGET_GID}" /app; \
    fi

FROM scratch AS exporter
COPY --from=chowned /app/ /
