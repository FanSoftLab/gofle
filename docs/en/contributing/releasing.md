# Releasing

> **Content type**: Reference — release automation for maintainers.
> **Audience**: Maintainers
> **Verified**: 2026-10-29 — matches `scripts/release.sh` and `.goreleaser.yaml`

## Purpose

Document the release process for GoFLE maintainers.

---

## Release Principles

- **Local only**: No GitHub Actions for releases
- **Platforms**: Linux + Windows (amd64/arm64)
- **No macOS**: Wails GUI cross-compile from Linux not supported
- **Orchestrator**: `scripts/release.sh`
- **Artifacts**: CLI + GUI binaries, checksums

---

## Prerequisites (Maintainer Machine)

- Docker 20.10+ with BuildKit
- Go 1.27+, Node.js 18+, Wails CLI v2
- GoReleaser: `go install github.com/goreleaser/goreleaser@latest`
- GitHub CLI (`gh`) or `GITHUB_TOKEN` (classic PAT, `repo` scope)
- GPG key for signing (recommended)

---

## Release Command

```bash
# Full release (build + package + publish)
./scripts/release.sh v2.1.0

# Dry run (build + package, no publish)
./scripts/release.sh v2.1.0 --snapshot

# Skip build (use existing dist/)
./scripts/release.sh v2.1.0 --skip-build
```

---

## Release Script Flow (`scripts/release.sh`)

```bash
#!/bin/bash
# 1. Validate version format (semver)
# 2. Build Linux artifacts
./build.sh -os linux -arch all -target all -v $VERSION
# 3. Build Windows artifacts
./build.sh -os windows -arch all -target all -v $VERSION
# 4. Create git tag
git tag -a $VERSION -m "$VERSION"
git push origin $VERSION
# 5. Run GoReleaser
export GITHUB_TOKEN=...  # or gh auth token
export GORELEASER_CURRENT_TAG=$VERSION
goreleaser release --clean
```

---

## Build Script (`build.sh`)

```bash
# Linux (all arch, both targets)
./build.sh -os linux -arch all -target all -v 2.1.0

# Windows (all arch, both targets)
./build.sh -os windows -arch all -target all -v 2.1.0

# Output: ./dist/
#   GoFLE-linux-amd64, GoFLE-linux-arm64
#   GoFLE-windows-amd64.exe, GoFLE-windows-arm64.exe
#   gofle-linux-amd64, gofle-linux-arm64
#   gofle-windows-amd64.exe, gofle-windows-arm64.exe
```

**Docker**: Uses `Dockerfile` with Go, Node, GTK/WebKit, Wails CLI

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
    builder: prebuilt  # uses ./dist/ binaries
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

## Version Embedding

Via `-ldflags` in `build.sh`:

```go
// main.go
var (
    Version = "dev"
    Commit  = "none"
    Date    = "unknown"
    BuiltBy = "unknown"
)
```

Set at build time:
- `Version` → `v2.1.0`
- `Commit` → git SHA
- `Date` → RFC3339
- `BuiltBy` → builder identity

---

## Manual Release (If Script Fails)

```bash
# 1. Build artifacts
./build.sh -os linux -arch all -target all -v 2.1.0
./build.sh -os windows -arch all -target all -v 2.1.0

# 2. Verify dist/
ls -la dist/

# 3. Tag
git tag -a v2.1.0 -m "v2.1.0"
git push origin v2.1.0

# 4. GoReleaser
export GITHUB_TOKEN=ghp_...  # classic PAT, repo scope
export GORELEASER_CURRENT_TAG=v2.1.0
goreleaser release --clean
```

---

## Post-Release Verification

1. **Check GitHub Release**: https://github.com/sankam-nikolya/GoFLE/releases/tag/v2.1.0
2. **Download each artifact** and verify:
   - `./GoFLE --version` → `v2.1.0`
   - `./gofle version` → `v2.1.0`
3. **Test basic functionality**:
   - Open local file
   - SSH connection
   - Search
4. **Verify checksums**:
   ```bash
   sha256sum -c GoFLE-2.1.0_checksums.txt
   ```

---

## Rollback Procedure

If critical issue found post-release:

```bash
# 1. Delete GitHub release
gh release delete v2.1.0 --yes

# 2. Delete local and remote tag
git tag -d v2.1.0
git push origin :refs/tags/v2.1.0

# 3. Fix issue, commit, re-tag
git commit -am "fix: critical issue"
git tag -a v2.1.1 -m "v2.1.1"
git push origin v2.1.1

# 4. Re-run release script
./scripts/release.sh v2.1.1
```

---

## Changelog

- Generated from conventional commits by GoReleaser
- Categories: Added, Changed, Fixed, Removed, Security
- Excludes: `docs:`, `test:`, `chore:`, `ci:`, `refactor:`, `perf:`
- Manual edit if needed before release

---

## Related Information

- [Release Process](../release/release-process.md)
- [Changelog](../release/changelog.md)
- [Migration Guides](../release/migration-guides.md)
- [Build Script](../getting-started/install-docker.md#build-script-flags)