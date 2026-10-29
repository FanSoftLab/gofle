# CLI Overview

> **Content type**: Concept — what the `gofle` CLI companion does and when to use it.
> **Audience**: Terminal users, automation scripts, SSH sessions
> **Verified**: 2026-10-29 — matches `cmd/gofle/main.go`

## Purpose

The `gofle` CLI is a terminal companion to the GoFLE desktop app. It reuses the same Go backend (`pkg/indexer`, `pkg/search`, `pkg/remote`, `pkg/ssh`, `pkg/ftp`) to open logs — local or remote — in an interactive terminal viewer (Bubbletea), or dump remote files to stdout.

## When to Use CLI

| Scenario | Use CLI |
|----------|---------|
| Quick log check over SSH without GUI | ✅ |
| Pipe remote log to `grep`/`jq`/etc. | ✅ |
| Headless server / no display | ✅ |
| Automation / scripts | ✅ |
| Full-featured interactive browsing | ✅ (Bubbletea viewer) |
| Multiple files with tabs/groups | ❌ Use GUI |
| Visual search highlighting | ❌ Use GUI |
| Drag-and-drop / file associations | ❌ Use GUI |

## Installation

```bash
# Build from source (requires Go 1.27+)
go build ./cmd/gofle
# Binary at ./gofle

# Or run directly
go run ./cmd/gofle [args]
```

**Note**: Module path is `GoFLE` (not proxy-resolvable). `go install GoFLE/cmd/gofle@latest` will not work. Build locally.

## Architecture

```
┌─────────────────────────────────────────────────────────┐
│                    gofle CLI                            │
├─────────────────────────────────────────────────────────┤
│  cobra CLI commands: ssh, ftp, recent, presets, session │
│  Bubbletea interactive viewer (tabs, search, follow)   │
└─────────────────────────────────────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────────┐
│              Shared Go Backend (pkg/*)                  │
├─────────────────────────────────────────────────────────┤
│  pkg/indexer   pkg/search   pkg/remote   pkg/ssh       │
│  pkg/ftp       pkg/config   pkg/logging   pkg/version  │
└─────────────────────────────────────────────────────────┘
```

Same backend as GUI → identical search, indexing, remote streaming behavior.

## Command Surface

| Command | Description |
|---------|-------------|
| `gofle [target]` | Open local file or `ssh://`/`ftp://` URL in interactive viewer. No arg = help. |
| `gofle version` | Print version. |
| `gofle ssh [flags] <host>` | Connect via SSH, stream remote file. Default: viewer. `--dump` = stdout. No `--remote` = browse remote FS. |
| `gofle ftp [flags] <host>` | Same as SSH, over FTP/FTPS. `--tls explicit|implicit` for FTPS. |
| `gofle recent` | Interactive recent-file picker (shared with GUI). |
| `gofle recent open <n>` | Reopen recent entry `<n>` in viewer (non-interactive). |
| `gofle recent clear` | Clear recent list. |
| `gofle presets` / `list` | List shared search presets. |
| `gofle presets import <file.json>` | Import GUI localStorage-format presets. |
| `gofle presets export [file.json]` | Export presets (stdout or file, GUI-compatible JSON). |
| `gofle session show` | List tabs in saved session. |
| `gofle session restore` | Open saved session, auto-load all tabs. |
| `gofle session clear` | Clear saved session. |

## Shared State with GUI

| State | Shared Via | Location |
|-------|------------|----------|
| Recent files | `recent.json` | Config directory |
| Search presets | `search_presets.json` | Config directory |
| Saved session | `saved_tabs.json` | Config directory |
| Settings | `settings.json` | Config directory (viewer settings only) |

## Viewer Modes

| Flag | Behavior |
|------|----------|
| (none) / `--view` | Stream and open in interactive terminal viewer (default) |
| `--dump` | Stream and print to stdout (for piping) |

Root-level targets (`gofle /path`, `gofle ssh://...`) always open viewer; no dump mode.

## Password Handling

- No `--password` flag
- Password prompted interactively on terminal (`Password:`, no echo)
- Secret never in shell history or process listings
- SSH key passphrase prompted similarly

## Host Key / Certificate Trust

- First unknown SSH host key: fingerprint shown, prompt `Trust this host? (y/N)`
- First unknown FTPS cert: fingerprint shown, prompt to trust
- On accept: written to `known_hosts` / `ftp_certs`, connection retried
- Changed key/cert: rejected as possible MITM, never auto-trusted

## Exit Codes

| Code | Meaning |
|------|---------|
| `0` | Success |
| `1` | General error (invalid args, connection failed, etc.) |
| `2` | Usage error (invalid flags) |
| `130` | Interrupted (Ctrl+C) |

## Related Information

- [CLI Commands](cli-commands.md)
- [CLI Viewer Keys](cli-viewer-keys.md)
- [CLI Examples](cli-examples.md)
- [GUI User Guide](../user-guide/index.md)