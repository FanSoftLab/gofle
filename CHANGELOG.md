# Changelog

All notable changes to this project are documented in this file.
The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- **CLI companion (`gofle`).** A terminal companion that reuses the Go backend to
  open a log — local, or over `ssh://` / `ftp://` — in an interactive Bubbletea
  viewer. Commands: `gofle [target]`, `gofle version`, `gofle ssh [flags] <host>`,
  `gofle ftp [flags] <host>`, and `gofle list` / `gofle list open <n>` /
  `gofle list clear` (recent files shared with the GUI).
- **FTP / FTPS transport.** Files can now be opened over `ftp://` as well as
  `ssh://`. FTPS (explicit/implicit) pins the server certificate in the
  `ftp_certs` trust store; an unknown certificate is confirmed before trust, and
  a changed one is rejected as a possible MITM.

### Changed

### Fixed

- **Jump to a selected search result with word wrap on.** In the terminal
  viewer, navigating the results pane (arrows, Enter, next/previous) and the
  goto-line prompt now position the log pane in visual rows: a match can no
  longer land below the viewport when the lines above it soft-wrap, and a stale
  wrapped-row offset no longer hides the first rows of the target line. In the
  desktop/web UI, clicking a result, next/previous match, and go-to-line jump
  through the wrap-safe content anchor, so the target row stays in place while
  row heights are measured (previously the jump could stop well above the
  target in unvisited regions of wrapped logs).

### Security

## [2.0.0] - 2026-10-04

### Added

- Cross-platform desktop log viewer (Linux/macOS/Windows) built with Wails v2 and Go.
- Byte-offset file indexer with random line access and live tail / follow for very large logs.
- Parallel regex and substring search engine with an LRU compiled-regex cache.
- Remote log streaming over SSH with host-key verification (known_hosts).
- AES-GCM encryption of stored SSH secrets at rest.
- Session restore, search-pane layout persistence, and per-item input-history management.

### Changed

### Fixed

### Security

- Shell-injection hardening on SSH cat/tail paths; configurable remote path validation.