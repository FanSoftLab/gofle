# CLI Commands Reference

> **Content type**: Reference — complete command syntax, flags, and behavior.
> **Audience**: Terminal users, automation
> **Verified**: 2026-10-29 — matches `cmd/gofle/internal/cmds/*.go`

## Purpose

Complete reference for all `gofle` CLI commands and flags.

---

## Global Flags

| Flag | Description |
|------|-------------|
| `--help`, `-h` | Show help for command |
| `--version`, `-v` | Print version and exit |

---

## `gofle [target]`

Open a local file or remote URL in the interactive viewer.

```bash
gofle /var/log/syslog
gofle ssh://user@host:22/var/log/app.log
gofle ftp://user@host:21/logs/app.log
gofle /path/a.log /path/b.log  # multiple files → multiple tabs
```

**Positional**: `target` — file path or `ssh://` / `ftp://` URL (repeatable)

**Behavior**: Always opens viewer. No dump mode for root targets.

---

## `gofle version`

```bash
gofle version
# Output: gofle version v2.1.0 (commit abc123, built 2026-10-29)
```

Prints version, git commit, build date.

---

## `gofle ssh [flags] <host>`

Connect via SSH and stream a remote file.

```bash
# Interactive viewer (default)
gofle ssh user@host
gofle ssh -H user@host -p 2222 --remote /var/log/app.log

# Dump to stdout
gofle ssh host --remote /var/log/app.log --dump | grep ERROR

# Browse remote filesystem (no --remote)
gofle ssh host
```

### Flags

| Flag | Short | Description |
|------|-------|-------------|
| `--host` | `-H` | Remote host (`user@host` or `host`). Alternative to positional `<host>`. |
| `--user` | `-u` | Username (overrides `user@` in host). |
| `--port` | `-p` | SSH port (default: 22). |
| `--remote` | | Remote file path. Omit to launch interactive directory browser. |
| `--key` | `-k` | Private key path for public-key auth. |
| `--view` | | Open in interactive viewer (default). |
| `--dump` | | Print to stdout instead of viewer. |
| `--insecure-accept-host-key` | | Trust unknown host key without prompt. **Dangerous**. |

### Positional

`<host>` — target host (`user@host` or `host`). Required unless `--host` used.

---

## `gofle ftp [flags] <host>`

Connect via FTP/FTPS and stream a remote file.

```bash
# Plain FTP
gofle ftp user@host --remote /logs/app.log

# Explicit FTPS
gofle ftp user@host --remote /logs/app.log --tls explicit

# Implicit FTPS
gofle ftp user@host --remote /logs/app.log --tls implicit

# Dump
gofle ftp host --remote /logs/app.log --dump
```

### Flags

| Flag | Short | Description |
|------|-------|-------------|
| `--host` | `-H` | Remote host (`user@host` or `host`). |
| `--user` | `-u` | Username. |
| `--port` | `-p` | Port (default: 21 for FTP/Explicit, 990 for Implicit). |
| `--remote` | | Remote file path. Omit for directory browser. |
| `--tls` | | `""` (plain), `explicit`, `implicit`. |
| `--view` | | Open in viewer (default). |
| `--dump` | | Print to stdout. |
| `--insecure-accept-host-key` | | Trust unknown FTPS cert without prompt. **Dangerous**. |

### Positional

`<host>` — target host. Required unless `--host` used.

---

## `gofle recent`

Interactive recent-file picker (shared with GUI).

```bash
gofle recent
```

Opens TUI with tabs: Local, SSH, FTP. Navigate with arrows, `Enter` to open.

### Subcommands

| Command | Description |
|---------|-------------|
| `gofle recent open <n>` | Reopen recent entry `<n>` in viewer (non-interactive). |
| `gofle recent clear` | Clear all recent entries. |

---

## `gofle presets` / `gofle list`

List shared search presets.

```bash
gofle presets
gofle list  # alias
```

Output:
```
1. Production Errors
2. Debug Trace
3. SSH Timeouts
```

### Subcommands

| Command | Description |
|---------|-------------|
| `gofle presets import <file.json>` | Import presets from JSON file (GUI-compatible). |
| `gofle presets export [file.json]` | Export presets to stdout or file. |

**Import/Export Format**: See [Search Presets](../user-guide/search-presets.md#cli-preset-management).

---

## `gofle session`

Manage saved session.

### Subcommands

| Command | Description |
|---------|-------------|
| `gofle session show` | List tabs in saved session. |
| `gofle session restore` | Open saved session, auto-load all tabs. |
| `gofle session clear` | Clear saved session. |

---

## Common SSH/FTP Flags Summary

| Flag | SSH | FTP | Description |
|------|-----|-----|-------------|
| `-H`, `--host` | ✅ | ✅ | Remote host |
| `-u`, `--user` | ✅ | ✅ | Username |
| `-p`, `--port` | ✅ (22) | ✅ (21/990) | Port |
| `--remote` | ✅ | ✅ | Remote file path |
| `-k`, `--key` | ✅ | ❌ | Private key path |
| `--tls` | ❌ | ✅ | `none`, `explicit`, `implicit` |
| `--view` | ✅ | ✅ | Open viewer (default) |
| `--dump` | ✅ | ✅ | Print to stdout |
| `--insecure-accept-host-key` | ✅ | ✅ | Auto-trust (dangerous) |

## Password Handling

- **No `--password` flag** for security
- Password prompted interactively: `Password:` (no echo)
- Applies to: SSH password auth, all FTP connections
- Never appears in shell history or `ps` output

## Exit Codes

| Code | Meaning |
|------|---------|
| `0` | Success |
| `1` | General error |
| `2` | Usage/flag error |
| `130` | Ctrl+C / interrupt |

## Configuration

CLI uses same config directory as GUI:
- `~/.config/gofle/` (Linux)
- `~/Library/Application Support/gofle/` (macOS)
- `%APPDATA%\gofle\` (Windows)

Shared files: `settings.json`, `recent.json`, `saved_tabs.json`, `search_presets.json`

## Related Information

- [CLI Overview](cli-overview.md)
- [CLI Viewer Keys](cli-viewer-keys.md)
- [CLI Examples](cli-examples.md)
- [GUI Remote SSH](../user-guide/open-remote-ssh.md)
- [GUI Remote FTP](../user-guide/open-remote-ftp.md)