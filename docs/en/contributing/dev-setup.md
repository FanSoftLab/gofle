# Development Setup

> **Content type**: Task — set up local development environment for GoFLE.
> **Audience**: Contributors
> **Verified**: 2026-10-29 — matches `CONTRIBUTING.md`

## Purpose

Get from fresh clone to running build and green test suite in under 10 minutes.

---

## Prerequisites

| Tool | Version | Install |
|------|---------|---------|
| Go | 1.27+ | https://go.dev/dl/ |
| Node.js | 18+ | https://nodejs.org/ |
| npm | 9+ | Bundled with Node.js |
| Wails CLI v2 | latest | `go install github.com/wailsapp/wails/v2/cmd/wails@latest` |
| Git | any | System package manager |

### Linux — System Libraries

**Debian / Ubuntu**:
```bash
sudo apt-get update
sudo apt-get install -y build-essential pkg-config \
    libgtk-3-dev libwebkit2gtk-4.1-dev
```

**Fedora**:
```bash
sudo dnf install gtk3-devel webkit2gtk4.1-devel gcc pkgconf-pkg-config
```

**Arch**:
```bash
sudo pacman -S gtk3 webkit2gtk-4.1 base-devel pkgconf
```

### macOS

```bash
xcode-select --install
```

### Windows

[Wails Prerequisites](https://wails.io/docs/gettingstarted/installation/#windows) — C compiler + WebView2 runtime.

---

## Clone & Build

```bash
git clone https://github.com/FanSoftLab/gofle.git
cd GoFLE

# 1. Build frontend (required before any Go build)
cd frontend && npm install && npm run build && cd ..

# 2. Build desktop binary (CGO required on Linux)
wails build            # production → build/bin/GoFLE
wails dev              # dev build with hot-reload + dev tools
```

**Binary Locations**:
| Build Method | Binary Location |
|--------------|-----------------|
| `wails build` | `build/bin/GoFLE` |
| `./build.sh` (Docker) | `./GoFLE` (repo root) |
| `./build.sh` (multi-target) | `./dist/` |

---

## Development Loop

### Frontend Hot Reload

```bash
# Terminal 1: Vite dev server
cd frontend && npm run dev

# Terminal 2: Go app wired to dev server
wails dev
```

- Frontend changes hot-reload in browser and app
- Go changes require `wails dev` restart (fast rebuild)

### Backend Only (CLI)

```bash
go build ./cmd/gofle     # produces ./gofle
./gofle /var/log/syslog  # test CLI viewer
```

No frontend build needed for CLI-only iteration.

---

## Test & Lint

```bash
# Full test suite
go test ./...

# Static analysis
go vet ./...

# Lint (also in CI)
golangci-lint run

# Frontend tests
cd frontend && npm run test
```

**CI Pipeline** (`.github/workflows/ci.yml`):
1. Build frontend
2. `go vet ./...`
3. `go test ./...`
4. `golangci-lint run`
5. `govulncheck ./...` (separate job)

---

## Project Layout

| Path | Responsibility |
|------|----------------|
| `main.go` / `app.go` | Wails entrypoint & `App` bound to frontend |
| `cmd/gofle` | Terminal companion: cobra CLI + Bubbletea viewer |
| `pkg/config` | Settings, recent files, SSH encryption, session persistence |
| `pkg/indexer` | Byte-offset file index, random line access, live tail |
| `pkg/search` | Parallel regex/substring search engine |
| `pkg/remote` | Shared transport contract: `Streamer`/`Lister` interfaces |
| `pkg/ssh` | Remote log streaming + host-key verification |
| `pkg/ftp` | Remote log streaming over FTP/FTPS + cert trust |
| `pkg/logging` | Centralized `slog` setup for both binaries |
| `pkg/version` | Version info (injected at build) |
| `frontend/` | TypeScript + Vite UI |

---

## Coding Conventions

### Go

- **Godoc**: Every exported identifier — comment starts with name, explains **why**, not just what
- **Embedding**: `//go:embed all:frontend/dist` — never `go build` without built frontend
- **Secrets**: SSH credentials encrypted at rest (AES-GCM); never log plaintext
- **Concurrency**: Use existing `sync.RWMutex` patterns in `App`/`FileIndexer`

### TypeScript

- **Strict mode**: Enabled in `tsconfig.json`
- **Types**: Use generated `wailsjs/go/models.ts` for backend contracts
- **State**: Reactive proxies in `app_context.ts` — mutate via methods
- **Events**: Wails runtime events for backend→frontend (`file_updated`, `search_complete`)

### Commits

Follow [Conventional Commits](https://www.conventionalcommits.org/):

```
feat: add FTP/FTPS transport
fix: handle log rotation in follow mode
refactor: simplify search engine internals
perf: optimize indexer memory allocation
security: harden SSH path validation
docs: update CLI reference
test: add indexer benchmarks
chore: update dependencies
ci: add Vale linting
```

---

## Pull Request Workflow

1. **Fork** and **branch** from `main`
2. **Make focused commits** with clear messages
3. **Ensure green CI**:
   ```bash
   go test ./...
   go vet ./...
   golangci-lint run
   cd frontend && npm run test
   ```
4. **Open PR** — CI must pass
5. **Review** — address feedback
6. **Merge** — squash or rebase as appropriate

---

## Adding Dependencies

### Go

```bash
go get github.com/new/dependency
go mod tidy
```

- Check license compatibility (MIT/BSD/Apache preferred)
- Run `govulncheck ./...` after adding

### TypeScript

```bash
cd frontend && npm install package-name && cd ..
```

- Prefer minimal dependencies
- Check bundle size impact

---

## Regenerating Wails Bridge

After changing `App` method signatures in `app.go`:

```bash
wails generate module
```

Updates `frontend/src/wailsjs/go/models.ts` and `App.d.ts`.

---

## Debugging

### Backend

```bash
# Delve debugger
dlv debug ./main.go

# Or in VS Code: F5 with Go extension
```

### Frontend

```bash
# In wails dev: press F12 for DevTools
# Or: cd frontend && npm run dev → open browser at localhost:5173
```

### Logs

```bash
# Application logs
tail -f ~/.config/gofle/gofle.log

# Debug logging
GOFLE_LOG_LEVEL=debug ./GoFLE
```

---

## Related Information

- [Coding Conventions](coding-conventions.md)
- [Testing](testing.md)
- [PR Workflow](pr-workflow.md)
- [Releasing](releasing.md)