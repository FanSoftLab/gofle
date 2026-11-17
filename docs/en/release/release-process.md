# Release Process

> **Content type**: Reference — how GoFLE releases are built and published.
> **Audience**: Maintainers, contributors
> **Verified**: 2026-10-29 — matches `scripts/release.sh` and `.goreleaser.yaml`

## Purpose

Document the local-only release workflow for GoFLE.

---

## Release Principles

- **Local only**: No GitHub Actions for releases
- **Platforms**: Linux + Windows (amd64/arm64)
- **No macOS**: Wails GUI cross-compile from Linux not supported
- **Artifacts**: CLI + GUI binaries, checksums, release notes
- **Orchestrator**: `scripts/release.sh`

---

## Prerequisites

- Local machine with Docker (for `build.sh`)
- Go 1.27+, Node.js 18+, Wails CLI v2
- GitHub CLI (`gh`) or `GITHUB_TOKEN` (classic PAT, `repo` scope)
- GoReleaser installed (`go install github.com/goreleaser/goreleaser@latest`)
- GPG key for signing (optional but recommended)

---

## Release Script

```bash
# Build, package, and publish
./scripts/release.sh v2.1.0

# Dry run (build + package, no publish)
./scripts/release.sh v2.1.0 --snapshot

# Skip build (use existing dist/)
./scripts/release.sh v2.1.0 --skip-build
```

**Script Flow**:
1. Parse version argument (validate semver)
2. Run `./build.sh -os linux -arch all -target all -v $VERSION`
3. Run `./build.sh -os windows -arch all -target all -v $VERSION`
4. Create git tag: `git tag -a v2.1.0 -m "v2.1.0"`
5. Push tag: `git push origin v2.1.0`
6. Run GoReleaser: `goreleaser release --clean`

---

## Build Script (`build.sh`)

```bash
# Linux GUI + CLI (all arch)
./build.sh -os linux -arch all -target all -v 2.1.0

# Windows GUI + CLI (all arch)
./build.sh -os windows -arch all -target all -v 2.1.0

# Output: ./dist/
#   GoFLE-linux-amd64
#   GoFLE-linux-arm64
#   GoFLE-windows-amd64.exe
#   GoFLE-windows-arm64.exe
#   gofle-linux-amd64
#   gofle-linux-arm64
#   gofle-windows-amd64.exe
#   gofle-windows-arm64.exe
```

**Docker Image**: `Dockerfile` with Go, Node, GTK/WebKit, Wails CLI

---

## GoReleaser Config (`.goreleaser.yaml`)

```yaml
project_name: GoFLE
before:
  hooks:
    - go mod tidy
builds:
  - id: gofle-cli
    main: ./cmd/gofle/main.go
    binary: gofle
    goos: [linux, windows]
    goarch: [amd64, arm64]
    builder: prebuilt  # uses binaries from dist/
    ldflags:
      - -s -w
      - -X main.version={{.Version}}
      - -X main.commit={{.Commit}}
      - -X main.date={{.Date}}
  - id: gofle-gui
    main: ./main.go
    binary: GoFLE
    goos: [linux, windows]
    goarch: [amd64, arm64]
    builder: prebuilt
    ldflags: same as above
archives:
  - format: tar.gz
    name_template: "{{.ProjectName}}-{{.Version}}-{{.Os}}-{{.Arch}}"
    files:
      - LICENSE.md
      - README.md
checksum:
  name_template: "{{.ProjectName}}-{{.Version}}_checksums.txt"
  algorithm: sha256
release:
  github:
    owner: sankam-nikolya
    name: GoFLE
  draft: false
  prerelease: false
  name_template: "GoFLE {{.Version}}"
changelog:
  sort: desc
  filters:
    exclude:
      - '^docs:'
      - '^test:'
      - '^chore:'
```

---

## Manual Release (Alternative)

```bash
# 1. Build all artifacts
./build.sh -os linux -arch all -target all -v 2.1.0
./build.sh -os windows -arch all -target all -v 2.1.0

# 2. Tag
git tag -a v2.1.0 -m "v2.1.0"
git push origin v2.1.0

# 3. GoReleaser
export GITHUB_TOKEN=ghp_...  # classic PAT, repo scope
export GORELEASER_CURRENT_TAG=v2.1.0
goreleaser release --clean
```

---

## Version Embedding

Version info embedded via `-ldflags`:
- `main.version` → `v2.1.0`
- `main.commit` → git commit hash
- `main.date` → build date (RFC3339)
- `main.builtBy` → builder identity

Accessible via:
- `gofle version`
- GUI Help → About
- `window.go.main.App.GetVersion()`

---

## Artifacts Published

| Artifact | Description |
|----------|-------------|
| `GoFLE-linux-amd64.tar.gz` | Linux GUI (amd64) |
| `GoFLE-linux-arm64.tar.gz` | Linux GUI (arm64) |
| `GoFLE-windows-amd64.zip` | Windows GUI (amd64) |
| `GoFLE-windows-arm64.zip` | Windows GUI (arm64) |
| `gofle-linux-amd64.tar.gz` | Linux CLI (amd64) |
| `gofle-linux-arm64.tar.gz` | Linux CLI (arm64) |
| `gofle-windows-amd64.zip` | Windows CLI (amd64) |
| `gofle-windows-arm64.zip` | Windows CLI (arm64) |
| `GoFLE-2.1.0_checksums.txt` | SHA256 checksums |

---

## Post-Release

1. Verify GitHub Release page: https://github.com/FanSoftLab/gofle/releases
2. Download and test each artifact
3. Update `CHANGELOG.md` (if not done via conventional commits)
4. Announce in relevant channels

---

## Rollback

If critical issue found:

```bash
# Delete GitHub release
gh release delete v2.1.0 --yes

# Delete tag
git tag -d v2.1.0
git push origin :refs/tags/v2.1.0

# Fix, re-tag, re-release
```

---

## Related Information

- [Changelog](changelog.md)
- [Migration Guides](migration-guides.md)
- [Build Script](../getting-started/install-docker.md#build-script-flags)
- [CI/CD](../contributing/releasing.md)