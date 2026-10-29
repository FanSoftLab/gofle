# Local Build

> **Content type**: Task — build GoFLE using local Go and Node.js toolchain.
> **Audience**: Developers, contributors
> **Prerequisites**: [Prerequisites](prerequisites.md) — Go 1.27+, Node.js 18+, Wails CLI v2, system libraries (Linux)
> **Verified**: 2026-10-29 — tested on Linux, macOS

## Purpose

Build GoFLE from source using your local development toolchain. Required for frontend hot-reload development and for contributing changes.

## Prerequisites

- All items from [Prerequisites](prerequisites.md)
- Frontend dependencies installed (done automatically by build steps below)

## Procedure

### 1. Build Frontend Bundle

```bash
cd frontend
npm install
npm run build
cd ..
```

This creates `frontend/dist/` with production assets. The Go binary embeds this via `//go:embed`.

### 2. Build Desktop Binary

**Production build:**
```bash
wails build
```
Binary created at `build/bin/GoFLE`.

**Development build (with hot reload and dev tools):**
```bash
wails dev
```
Runs the app connected to Vite dev server. Frontend changes hot-reload automatically.

**Cross-platform build:**
```bash
wails build -platform windows/amd64,darwin/arm64,linux/amd64
```
Requires Go cross-compilation setup. CGO disabled for cross-builds (no GTK on non-Linux).

## Expected Result

- Production binary at `build/bin/GoFLE` (or `build/bin/GoFLE.exe` on Windows)
- Development server running at `http://localhost:34115` (with `wails dev`)

## Verification

```bash
# Production binary
./build/bin/GoFLE --version

# Development mode
# Opens app window with dev tools (F12)
```

## Binary Location Note

| Build Method | Binary Location |
|--------------|-----------------|
| `./build.sh` (Docker) | `./GoFLE` (repository root) |
| `wails build` (local) | `build/bin/GoFLE` |
| `wails dev` | Runs in memory, no persistent binary |

## Troubleshooting

| Symptom | Cause | Resolution |
|---------|-------|------------|
| `frontend/dist not found` | Frontend not built | Run `cd frontend && npm run build` first |
| `CGO_ENABLED=0` on Linux | Cross-compile or env var | Ensure `CGO_ENABLED=1` for native Linux build |
| `libwebkit2gtk-4.1.so not found` | Missing system libs | Install prerequisites from [Prerequisites](prerequisites.md) |
| `wails: command not found` | Wails CLI not installed | Run `go install github.com/wailsapp/wails/v2/cmd/wails@latest` |
| Frontend changes not reflecting | Using production binary | Use `wails dev` for hot reload |

## Related Information

- [Docker Build](install-docker.md)
- [Development Workflow](../contributing/dev-setup.md)
- [First Run](first-run.md)