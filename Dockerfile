# Dockerfile
# syntax=docker/dockerfile:1.4
ARG RUN_TESTS=false
ARG APP_NAME=GoFLE
ARG BUILD_MODE=prod
ARG PLATFORMS=linux/amd64
ARG MULTI=false
ARG CLI_APP_NAME=gofle
ARG GOFILE_VERSION=dev
ARG BUILD_TARGET=all

FROM golang:1.26-bookworm AS builder

ARG RUN_TESTS
ARG APP_NAME
ARG BUILD_MODE
ARG PLATFORMS
ARG MULTI
ARG CLI_APP_NAME
ARG GOFILE_VERSION
ARG BUILD_TARGET
ARG TARGET_UID=0
ARG TARGET_GID=0

# Install GTK/WebKit dependencies only for gui or all targets
RUN if [ "$BUILD_TARGET" = "gui" ] || [ "$BUILD_TARGET" = "all" ]; then \
        apt-get update && apt-get install -y \
        build-essential \
        pkg-config \
        libgtk-3-dev \
        libwebkit2gtk-4.1-dev \
        nodejs \
        npm \
        git \
        && rm -rf /var/lib/apt/lists/*; \
    else \
        apt-get update && apt-get install -y \
        nodejs \
        npm \
        git \
        && rm -rf /var/lib/apt/lists/*; \
    fi

RUN if [ "$BUILD_TARGET" = "gui" ] || [ "$BUILD_TARGET" = "all" ]; then \
        ln -s /usr/lib/x86_64-linux-gnu/pkgconfig/webkit2gtk-4.1.pc \
              /usr/lib/x86_64-linux-gnu/pkgconfig/webkit2gtk-4.0.pc 2>/dev/null || \
        ln -s /usr/lib/pkgconfig/webkit2gtk-4.1.pc \
              /usr/lib/pkgconfig/webkit2gtk-4.0.pc 2>/dev/null || true; \
    fi

WORKDIR /app

COPY go.mod* go.sum* ./

RUN go clean -modcache

RUN if [ ! -f go.mod ]; then \
        echo "go.mod не знайдено. Ініціалізуємо модуль '$APP_NAME'..." && \
        go mod init "$APP_NAME"; \
    fi && \
    if [ ! -f go.sum ]; then \
        touch go.sum; \
    fi && \
    go mod download 2>/dev/null || true

# Frontend handling only for gui or all targets
RUN if [ "$BUILD_TARGET" = "gui" ] || [ "$BUILD_TARGET" = "all" ]; then \
        COPY frontend/package.json frontend/package-lock.json* ./frontend/ && \
        cd frontend && (npm ci --no-audit --no-fund 2>/dev/null || npm install --no-audit --no-fund); \
    fi

COPY . .

RUN go mod tidy && go mod verify

# Wails generate module only for gui or all targets
RUN if [ "$BUILD_TARGET" = "gui" ] || [ "$BUILD_TARGET" = "all" ]; then \
        go install github.com/wailsapp/wails/v2/cmd/wails@latest; \
    fi

RUN mkdir -p build

# Build frontend only for gui or all targets
RUN if [ "$BUILD_TARGET" = "gui" ] || [ "$BUILD_TARGET" = "all" ]; then \
        cd frontend && npm run build && cd ..; \
    else \
        mkdir -p frontend/dist && echo '<!DOCTYPE html><html><body></body></html>' > frontend/dist/index.html; \
    fi

# Run wails generate module only for gui or all targets
RUN if [ "$BUILD_TARGET" = "gui" ] || [ "$BUILD_TARGET" = "all" ]; then \
        wails generate module; \
    fi

RUN touch test_results.log && \
    if [ "$RUN_TESTS" = "true" ]; then \
        go test -v ./... 2>&1 | tee test_results.log; \
    else \
        echo "Tests skipped" > test_results.log; \
    fi

RUN go vet ./...

# Build via wails build only for gui or all targets
RUN if [ "$BUILD_TARGET" = "gui" ] || [ "$BUILD_TARGET" = "all" ]; then \
        TAGS="desktop webkit2gtk_4_1" \
        && FLAGS="" \
        && if [ "$BUILD_MODE" = "prod" ]; then TAGS="$TAGS production"; fi \
        && if [ "$BUILD_MODE" = "dev" ]; then FLAGS="$FLAGS -debug"; fi \
        && if [ "$MULTI" = "true" ]; then FLAGS="$FLAGS -platform $PLATFORMS"; fi \
        && wails build $FLAGS -ldflags="-s -w" -tags "$TAGS" -o "$APP_NAME"; \
    fi

# Build CLI only for cli or all targets
RUN if [ "$BUILD_TARGET" = "cli" ] || [ "$BUILD_TARGET" = "all" ]; then \
        ext=""; \
        if [ "${PLATFORMS%/*}" = "windows" ]; then ext=".exe"; fi; \
        if [ "$MULTI" = "true" ]; then \
            IFS=','; set -- $PLATFORMS; \
            for p in "$@"; do \
                goos="${p%/*}"; goarch="${p#*/}"; \
                e=""; [ "$goos" = "windows" ] && e=".exe"; \
                CGO_ENABLED=0 GOOS="$goos" GOARCH="$goarch" \
                    go build -buildvcs=false -ldflags="-s -w -X main.version=${GOFILE_VERSION}" -o "build/bin/${CLI_APP_NAME}-${goos}-${goarch}${e}" ./cmd/gofle; \
            done; \
        else \
            CGO_ENABLED=0 GOOS="${PLATFORMS%/*}" GOARCH="${PLATFORMS#*/}" \
                go build -buildvcs=false -ldflags="-s -w -X main.version=${GOFILE_VERSION}" -o "build/bin/${CLI_APP_NAME}${ext}" ./cmd/gofle; \
        fi; \
    fi

# Re-stage the full /app working tree as the current host user so that every
# file created inside the container is exported with the host user's ownership
# instead of root. The `exporter` target then copies the entire /app tree out.
FROM builder AS chowned
ARG TARGET_UID
ARG TARGET_GID
# The frontend is only needed to produce dist, which wails build embeds into the
# Go binary. Drop the bulky frontend build cache (node_modules, dist, npm cache)
# so it never lands in the exported build tree.
RUN if [ -d "/app/frontend/node_modules" ] || [ -d "/app/frontend/dist" ] || [ -d "/app/frontend/.vite" ]; then \
        rm -rf /app/frontend/node_modules /app/frontend/dist /app/frontend/.vite \
        && rm -rf /root/.npm /root/.cache; \
    fi
RUN if [ "${TARGET_UID}" != "0" ] || [ "${TARGET_GID}" != "0" ]; then \
        chown -R "${TARGET_UID}:${TARGET_GID}" /app; \
    fi

FROM scratch AS exporter
# Copy the contents of /app (not /app itself) so the exported tree is rooted at
# the working tree, i.e. build/bin/<app> lands at the top level of the export
# rather than under a nested app/ directory.
COPY --from=chowned /app/ /
