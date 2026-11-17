# Changelog

> **Content type**: Release documentation — user-visible changes per version.
> **Format**: [Keep a Changelog](https://keepachangelog.com/en/1.1.0/)
> **Versioning**: [Semantic Versioning](https://semver.org/spec/v2.0.0.html)
> **Verified**: 2026-10-29 — matches repository `CHANGELOG.md`

## Purpose

Complete history of user-visible changes for each GoFLE release.

---

## [Unreleased]

### Added

### Changed

### Fixed

### Security

---

## [2.0.0] — 2026-10-04

### Added

- Cross-platform desktop log viewer (Linux/macOS/Windows) built with Wails v2 and Go
- Byte-offset file indexer with random line access and live tail / follow for very large logs
- Parallel regex and substring search engine with an LRU compiled-regex cache
- Remote log streaming over SSH with host-key verification (known_hosts)
- AES-GCM encryption of stored SSH secrets at rest
- Session restore, search-pane layout persistence, and per-item input-history management

### Changed

### Fixed

### Security

- Shell-injection hardening on SSH cat/tail paths; configurable remote path validation

---

## Version History Summary

| Version | Date | Key Changes |
|---------|------|-------------|
| 2.0.0 | 2026-10-04 | Initial Wails v2 release: GUI, indexer, search, SSH, encryption, session restore |
| Unreleased | — | CLI companion (`gofle`), FTP/FTPS transport, advanced search filters, tab groups, presets, Top unique, web mode |

---

## Release Categories

### Added
New user-visible capabilities.

### Changed
Changes to existing behavior or workflows.

### Fixed
Corrected problems in user terms (not internal implementation details).

### Removed or Deprecated
Removed capabilities with migration requirements.

### Security
Security improvements and vulnerability fixes.

---

## Conventional Commits

This project uses [Conventional Commits](https://www.conventionalcommits.org/) for automated changelog generation:

| Prefix | Category | Example |
|--------|----------|---------|
| `feat:` | Added | `feat: add FTP/FTPS transport` |
| `fix:` | Fixed | `fix: handle log rotation in follow` |
| `refactor:` | Changed | `refactor: simplify search engine` |
| `perf:` | Changed | `perf: optimize indexer memory` |
| `security:` | Security | `security: harden SSH path validation` |
| `docs:` | (excluded) | `docs: update CLI reference` |
| `test:` | (excluded) | `test: add indexer benchmarks` |
| `chore:` | (excluded) | `chore: update dependencies` |
| `ci:` | (excluded) | `ci: add Vale linting` |

Only `feat`, `fix`, `refactor`, `perf`, `security` appear in changelog.

---

## Related Information

- [Release Process](release-process.md)
- [Migration Guides](migration-guides.md)
- [GitHub Releases](https://github.com/FanSoftLab/gofle/releases)