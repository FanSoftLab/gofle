# Config Files Reference

> **Content type**: Reference — structure and contents of GoFLE configuration files.
> **Audience**: Advanced users, administrators
> **Verified**: 2026-10-29 — matches `pkg/config/config.go`

## Purpose

Document the JSON configuration files used by GoFLE for persistence.

## Config Directory

| Platform | Path |
|----------|------|
| Linux | `~/.config/gofle/` |
| macOS | `~/Library/Application Support/gofle/` |
| Windows | `%APPDATA%\gofle\` |

## Files Overview

| File | Purpose | Encrypted Fields |
|------|---------|------------------|
| `settings.json` | UI settings, theme, language, window geometry, secret key | `secret_key` (AES-GCM key) |
| `recent.json` | Recent files list (local, SSH, FTP) | SSH passwords, FTP passwords |
| `saved_tabs.json` | Session snapshot (when enabled) | SSH passwords, FTP passwords |
| `search_presets.json` | Named advanced filter presets | None |

---

## settings.json

```json
{
  "theme": "system",
  "language": "system",
  "save_tabs_exit": true,
  "search_pane_height": 250,
  "window_width": 1200,
  "window_height": 800,
  "window_maximized": true,
  "font_family": "JetBrains Mono",
  "font_size": 13,
  "line_density": "normal",
  "word_wrap": true,
  "show_line_numbers": true,
  "view_mode": "split",
  "default_follow": true,
  "default_search_case": false,
  "default_search_regex": false,
  "default_search_invert": false,
  "search_orientation": "horizontal",
  "search_side": "bottom",
  "startup_behavior": "session",
  "confirm_close_remote_tab": true,
  "max_recent": 50,
  "max_history": 50,
  "ssh_timeout": 15,
  "stall_timeout": 30,
  "auto_accept_host_key": false,
  "conn_max_attempts": 3,
  "conn_window": 10,
  "conn_lockout": 15,
  "secret_key": "base64-encoded-32-byte-key"
}
```

### Field Details

| Field | Type | Description |
|-------|------|-------------|
| `theme` | string | `dark`, `light`, `dracula`, `system` |
| `language` | string | `uk`, `en`, `system` |
| `save_tabs_exit` | bool | Enable session restore |
| `search_pane_height` | int | Results pane height in pixels |
| `window_width/height` | int | Window geometry (pixels) |
| `window_maximized` | bool | Start maximized |
| `font_family` | string | Log font name |
| `font_size` | int | Font size in pixels |
| `line_density` | string | `compact`, `normal`, `comfortable` |
| `word_wrap` | bool | Enable line wrapping |
| `show_line_numbers` | bool | Show line number column |
| `view_mode` | string | `split`, `results`, `normal` |
| `default_follow` | bool | Follow on file open |
| `default_search_case` | bool | Match case default |
| `default_search_regex` | bool | Regex default |
| `default_search_invert` | bool | Invert match default |
| `search_orientation` | string | `horizontal`, `vertical` |
| `search_side` | string | `bottom`, `top`, `left`, `right` |
| `startup_behavior` | string | `session`, `empty`, `open` |
| `confirm_close_remote_tab` | bool | Prompt before closing remote tab |
| `max_recent` | int | Recent list capacity |
| `max_history` | int | Search history capacity |
| `ssh_timeout` | int | Connection timeout (seconds) |
| `stall_timeout` | int | Download silence timeout (seconds) |
| `auto_accept_host_key` | bool | Auto-trust unknown keys (dangerous) |
| `conn_max_attempts` | int | Failed attempt limit per host |
| `conn_window` | int | Attempt counting window (minutes) |
| `conn_lockout` | int | Lockout duration (minutes) |
| `secret_key` | string | Base64 AES-GCM key (generated once) |

---

## recent.json

```json
{
  "local": [
    "/var/log/syslog",
    "/home/user/app.log"
  ],
  "ssh": [
    {
      "scheme": "ssh",
      "host": "10.0.0.5",
      "port": 22,
      "username": "deploy",
      "auth_type": "key",
      "key_path": "/home/user/.ssh/id_ed25519",
      "key_passphrase": "encrypted-base64",
      "remote_file_path": "/var/log/nginx/access.log",
      "canonical_key": "ssh://10.0.0.5:22/var/log/nginx/access.log",
      "last_opened": "2026-10-29T10:00:00Z"
    }
  ],
  "ftp": [
    {
      "scheme": "ftp",
      "host": "ftp.example.com",
      "port": 21,
      "username": "user",
      "password": "encrypted-base64",
      "tls_mode": "explicit",
      "remote_file_path": "/logs/app.log",
      "canonical_key": "ftp://ftp.example.com:21/logs/app.log",
      "last_opened": "2026-10-29T10:00:00Z"
    }
  ]
}
```

### Field Details

**Local entries**: Simple string array of file paths.

**SSH entries**:
| Field | Type | Description |
|-------|------|-------------|
| `scheme` | string | Always `"ssh"` |
| `host` | string | Hostname or IP |
| `port` | int | SSH port |
| `username` | string | Remote username |
| `auth_type` | string | `"password"` or `"key"` |
| `key_path` | string | Private key path (if key auth) |
| `key_passphrase` | string | **Encrypted** passphrase (if key encrypted) |
| `remote_file_path` | string | Absolute path on server |
| `canonical_key` | string | Dedup key: `scheme://host:port/path` |
| `last_opened` | string | ISO 8601 timestamp |

**FTP entries**:
| Field | Type | Description |
|-------|------|-------------|
| `scheme` | string | Always `"ftp"` |
| `host` | string | Hostname or IP |
| `port` | int | FTP port |
| `username` | string | FTP username |
| `password` | string | **Encrypted** password |
| `tls_mode` | string | `none`, `explicit`, `implicit` |
| `remote_file_path` | string | Absolute path on server |
| `canonical_key` | string | Dedup key: `scheme://host:port/path` |
| `last_opened` | string | ISO 8601 timestamp |

**Encryption**: Passwords and passphrases encrypted with AES-GCM using `secret_key` from `settings.json`. Never stored in plaintext.

---

## saved_tabs.json

```json
{
  "tabs": [
    {
      "type": "local",
      "path": "/var/log/syslog",
      "name": "syslog",
      "group": "System",
      "scroll_position": 1250,
      "search_query": "ERROR",
      "follow": true
    },
    {
      "type": "ssh",
      "host": "10.0.0.5",
      "port": 22,
      "username": "deploy",
      "auth_type": "key",
      "key_path": "/home/user/.ssh/id_ed25519",
      "key_passphrase": "encrypted-base64",
      "remote_file_path": "/var/log/app.log",
      "name": "app-prod",
      "group": "Production",
      "scroll_position": 0,
      "search_query": "",
      "follow": true
    }
  ],
  "active_tab_index": 0,
  "groups": [
    {"name": "System", "color": "#3b82f6"},
    {"name": "Production", "color": "#ef4444"}
  ]
}
```

### Field Details

**Common tab fields**:
| Field | Type | Description |
|-------|------|-------------|
| `type` | string | `local`, `ssh`, `ftp` |
| `name` | string | Custom tab name |
| `group` | string | Group name (empty = no group) |
| `scroll_position` | int | Line number at top of view |
| `search_query` | string | Active primary search |
| `follow` | bool | Follow state |

**SSH tab fields**: Same as `recent.json` SSH entry + common fields.

**FTP tab fields**: Same as `recent.json` FTP entry + common fields.

---

## search_presets.json

```json
[
  {
    "name": "Production Errors",
    "nearby": {"pattern": "UserService", "mode": "before", "window": 15},
    "levels": ["ERROR"],
    "time": {"from": "2026-10-01", "to": "2026-10-07", "require_timestamp": false},
    "exclude": {"patterns": ["healthcheck"], "as_regex": false},
    "range": {"mode": "cursor_down"}
  },
  {
    "name": "Debug Trace",
    "nearby": {"pattern": "", "mode": "either", "window": 10},
    "levels": ["DEBUG", "TRACE"],
    "time": {"from": "", "to": "", "require_timestamp": false},
    "exclude": {"patterns": [], "as_regex": false},
    "range": {"mode": "all"}
  }
]
```

### Field Details

| Field | Type | Description |
|-------|------|-------------|
| `name` | string | Preset display name |
| `nearby` | object | Nearby filter config |
| `nearby.pattern` | string | Pattern to find near matches |
| `nearby.mode` | string | `before`, `after`, `either` |
| `nearby.window` | int | Lines window (1–200) |
| `levels` | array | Level strings: `ERROR`, `WARN`, `INFO`, `DEBUG`, `OTHER` |
| `time` | object | Time filter config |
| `time.from` | string | Start date `YYYY-MM-DD` (empty = no limit) |
| `time.to` | string | End date `YYYY-MM-DD` (empty = no limit) |
| `time.require_timestamp` | bool | Drop lines without parseable timestamp |
| `exclude` | object | Exclude filter config |
| `exclude.patterns` | array | Pattern strings (one per array element) |
| `exclude.as_regex` | bool | Treat patterns as regex |
| `range` | object | Range filter config |
| `range.mode` | string | `all`, `cursor_down`, `cursor_up`, `lines`, `last_n` |

---

## Encryption Details

- **Algorithm**: AES-GCM (256-bit key)
- **Key**: 32-byte random key generated once, stored in `settings.json` as base64
- **Nonce**: Random per-encryption, stored with ciphertext
- **Scope**: Only SSH passwords, key passphrases, FTP passwords
- **Never encrypted**: File paths, hostnames, usernames, settings, presets

## Backup & Migration

**Backup**: Copy entire config directory.

**Migration**: Config files are forward-compatible. New fields get defaults. Downgrade may lose new settings.

**Reset**: Delete config directory → fresh defaults on next launch.

## Related Information

- [Settings UI](settings-ui.md)
- [Settings Reference](settings-reference.md)
- [Security Notes](../../../SECURITY.md)