# Pull Request Workflow

> **Content type**: Task — contribution process from fork to merge.
> **Audience**: Contributors
> **Verified**: 2026-10-29 — matches `CONTRIBUTING.md`

## Purpose

Standardized process for contributing changes to GoFLE.

---

## Before You Start

1. **Check existing issues** — avoid duplicate work
2. **Open an issue** for significant changes (design discussion)
3. **Keep scope small** — one logical change per PR

---

## Workflow Steps

### 1. Fork & Branch

```bash
# Fork on GitHub, then clone your fork
git clone https://github.com/YOUR_USERNAME/GoFLE.git
cd GoFLE

# Add upstream
git remote add upstream https://github.com/FanSoftLab/gofle.git

# Create branch
git checkout -b feat/your-feature-name
```

**Branch naming**: `feat/`, `fix/`, `refactor/`, `docs/`, `chore/`

### 2. Develop

```bash
# Make changes
# Follow coding conventions (see coding-conventions.md)
# Add tests for new behavior
# Update documentation if user-visible
```

### 3. Validate Locally

```bash
# Backend
go test ./...
go vet ./...
golangci-lint run

# Frontend
cd frontend && npm run test && cd ..

# Build check
wails build  # or ./build.sh for Docker
```

### 4. Commit

```bash
git add .
git commit -m "feat(scope): add amazing feature

Explain why this change is needed.
Reference issue: #123"
```

**Commit message format**: Conventional Commits
- `feat:`, `fix:`, `refactor:`, `perf:`, `security:`, `docs:`, `test:`, `chore:`, `ci:`
- Subject ≤ 72 chars, imperative mood
- Body: why, not what

### 5. Push & PR

```bash
git push origin feat/your-feature-name
```

Open PR on GitHub:
- **Base**: `main`
- **Title**: Match commit subject
- **Description**: 
  - What changed
  - Why
  - Testing done
  - Screenshots (if UI)
  - Breaking changes (if any)

### 6. CI Checks

Required checks (must pass):
- `build-test` (Go build, test, vet, lint + frontend test)
- `govulncheck`

### 7. Review

- Address reviewer feedback
- Push follow-up commits (no force-push after review starts)
- Keep CI green

### 8. Merge

- Maintainer merges (squash or rebase)
- Branch deleted automatically
- Changelog updated via conventional commit

---

## PR Checklist

- [ ] Single logical change
- [ ] Tests added/updated
- [ ] Documentation updated (if user-visible)
- [ ] `go test ./...` passes
- [ ] `go vet ./...` passes
- [ ] `golangci-lint run` passes
- [ ] `npm run test` passes (frontend)
- [ ] Conventional commit messages
- [ ] No plaintext secrets
- [ ] No breaking changes without discussion

---

## Breaking Changes

If PR introduces breaking changes:

1. **Discuss in issue first**
2. **Mark PR title**: `[BREAKING] feat: ...`
3. **Update migration guide** (`docs/en/release/migration-guides.md`)
4. **Bump major version** in release

---

## Labels

| Label | Meaning |
|-------|---------|
| `bug` | Fixes a bug |
| `enhancement` | New feature/improvement |
| `documentation` | Docs only |
| `good first issue` | Suitable for newcomers |
| `help wanted` | Needs contributor |
| `breaking` | Breaking change |
| `needs review` | Ready for review |
| `approved` | Approved, awaiting merge |

---

## Related Information

- [Development Setup](dev-setup.md)
- [Coding Conventions](coding-conventions.md)
- [Testing](testing.md)
- [Releasing](releasing.md)
- [Conventional Commits](https://www.conventionalcommits.org/)