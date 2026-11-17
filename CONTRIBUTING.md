# Contributing to GoFLE

Thanks for your interest in improving GoFLE! This guide gets you from a fresh
clone to a running build and a green test suite in under 10 minutes.

GoFLE is two products in one repo: the **desktop GUI** (Wails/Go plus a
TypeScript frontend) and the **`gofle` terminal companion** (`cmd/gofle`, a
cobra CLI with a Bubbletea viewer) that reuses the same Go backend. This guide
covers building and testing both.

## Prerequisites

- **Go 1.27+**
- **Node.js 18+** and npm
- **Wails CLI v2**: `go install github.com/wailsapp/wails/v2/cmd/wails@latest`
- **git**

### Linux only — system webview libraries

```bash
# Debian / Ubuntu
sudo apt-get update
sudo apt-get install -y build-essential pkg-config \
    libgtk-3-dev libwebkit2gtk-4.1-dev

# Fedora
sudo dnf install gtk3-devel webkit2gtk4.1-devel gcc pkgconf-pkg-config
```

macOS needs only `xcode-select --install`. Windows needs the
[Wails prerequisites](https://wails.io/docs/gettingstarted/installation/#windows).

## Clone & build

```bash
git clone https://github.com/FanSoftLab/gofle.git
cd GoFLE

# 1. Build the frontend bundle (embedded by Go via //go:embed)
cd frontend && npm install && npm run build && cd ..

# 2. Build the desktop binary (CGO required on Linux)
wails build            # production binary at build/bin/gofle
wails dev              # dev build with hot-reload + dev tools
```

A raw `wails build` places the binary at `build/bin/gofle`.

## Run the dev loop

```bash
cd frontend && npm run dev      # Vite dev server (hot reload) in one terminal
wails dev                       # Go app wired to the dev server in another
```

## Test & lint

```bash
go build ./cmd/gofle           # build the terminal companion (produces ./gofle)
go test ./...                   # backend unit tests (indexer, search, config, ssh, remote, ftp, and the cmd/gofle CLI)
go vet ./...                    # static analysis
golangci-lint run               # lint — also enforced in CI
```

The `gofle` CLI has no frontend dependency, so you can iterate on it without a
built `frontend/dist`: build it with `go build ./cmd/gofle` and run the resulting
binary for manual viewer testing (e.g. `./gofle /var/log/syslog` or
`./gofle list`).

The same commands run in CI (`.github/workflows/ci.yml`). CI also runs
`govulncheck ./...` in a separate job.

> A frontend build is required before any `go build`/`go test` because `main.go`
> embeds `frontend/dist` via `//go:embed`.

## Project layout

| Path | Responsibility |
| --- | --- |
| `main.go` / `app.go` | Wails entrypoint and the `App` bound to the frontend |
| `cmd/gofle` | Terminal companion: cobra CLI (`ssh`/`ftp`/`list`/`version`) + Bubbletea viewer, reusing the backend |
| `pkg/config` | Settings, recent files, SSH secret encryption, session persistence |
| `pkg/indexer` | Byte-offset file index, random line access, live tail |
| `pkg/search` | Parallel regex/substring search engine |
| `pkg/remote` | Shared transport contract: `Streamer`/`Lister` interfaces + unified `Config` (implemented by ssh & ftp) |
| `pkg/ssh` | Remote log streaming + host-key verification |
| `pkg/ftp` | Remote log streaming over FTP/FTPS (explicit/implicit) + cert trust store |
| `pkg/logging` | Centralized `slog` setup for both binaries |
| `frontend/` | TypeScript + Vite UI |

## Coding conventions

- Document every exported identifier with a godoc comment that starts with the
  name and explains **why**, not just what.
- Keep `//go:embed all:frontend/dist` valid: never `go build` without a built frontend.
- SSH credentials are encrypted at rest with AES-GCM; never log plaintext secrets.
- Use the existing `sync.RWMutex` patterns in `App`/`FileIndexer` for concurrency.

## Pull requests

1. Fork and branch from `main`.
2. Make focused commits with clear messages.
3. Ensure `go test ./...`, `go vet ./...`, and `golangci-lint run` pass.
4. Open a PR; CI must be green.

## Releasing

Releases are built with `./build.sh` (Docker, reproducible) and published via
GitHub Releases. See `build.sh -h` for flags.

Build output locations:
- `wails build` → binary at `build/bin/gofle`.
- `./build.sh` (single target) → binary at `./gofle` in the repo root.
- `./build.sh` (multi-target) → artifacts under `./dist/`.
