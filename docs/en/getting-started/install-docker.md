# Build with Docker

> **Content type**: Task — build GoFLE using the provided Docker image.
> **Audience**: All users
> **Prerequisites**: [Prerequisites](prerequisites.md) — Docker 20.10+, BuildKit enabled
> **Verified**: 2026-10-29 — tested on Linux, macOS, Windows (WSL2)

## Purpose

Build a production GoFLE binary using the reproducible Docker build. This is the recommended method for releases and for users without a local Go/Node.js toolchain.

## Prerequisites

- Docker 20.10+ with BuildKit enabled (`DOCKER_BUILDKIT=1`)
- Git (to clone repository)
- No Go, Node.js, or system libraries required on host

## Procedure

1. Clone the repository:
   ```bash
   git clone https://github.com/FanSoftLab/gofle.git
   cd GoFLE
   ```

2. Build the native Linux binary (default):
   ```bash
   ./build.sh
   ```
   The binary appears at `./GoFLE` in the repository root.

3. (Optional) Build for multiple platforms:
   ```bash
   ./build.sh -os all -arch all
   ```
   Artifacts appear in `./dist/`.

4. (Optional) Run tests before building:
   ```bash
   ./build.sh -run-tests
   ```

5. (Optional) Build development binary:
   ```bash
   ./build.sh -mode dev
   ```

## Build Script Flags

| Flag | Values | Default | Description |
|------|--------|---------|-------------|
| `-mode` | `dev` \| `prod` | `prod` | Build mode |
| `-os` | `linux` \| `mac` \| `windows` \| `all` | `linux` | Target operating system |
| `-arch` | `amd64` \| `arm64` \| `all` | `amd64` | Target architecture |
| `-run-tests` | (flag) | off | Run `go test ./...` and `go vet` before building |
| `-target` | `gui` \| `cli` \| `all` | `all` | Build targets |
| `-v` | version string | git tag | Version to embed in binary |
| `-h` | — | — | Show help |

## Expected Result

- Single binary at `./GoFLE` (Linux) or `./dist/` (multi-platform)
- Binary includes embedded frontend assets
- Binary runs without external dependencies

## Verification

```bash
# Check binary exists and runs
./GoFLE --version
# Output: GoFLE version v2.x.x (or dev)

# Check help
./GoFLE --help
```

## Troubleshooting

| Symptom | Cause | Resolution |
|---------|-------|------------|
| `docker: command not found` | Docker not installed | Install Docker from [docker.com](https://docker.com) |
| `permission denied` | User not in docker group | Run `sudo usermod -aG docker $USER` and re-login |
| `buildx failed` | BuildKit not enabled | Export `DOCKER_BUILDKIT=1` or run `docker buildx create --use` |
| Binary not found at `./GoFLE` | Wrong working directory | Run `./build.sh` from repository root |

## Related Information

- [Local Build](install-local.md)
- [Linux Package Install](install-linux.md)
- [First Run](first-run.md)
- [Release Process](../../release/release-process.md)