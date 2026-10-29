# Wails Bridge API Reference

> **Content type**: Reference — Go methods exposed to frontend via Wails v2.
> **Audience**: Frontend developers, contributors
> **Verified**: 2026-10-29 — matches `app.go` and generated `wailsjs/go/main/App.d.ts`

## Purpose

Document the `App` struct methods bound to the JavaScript runtime. Frontend calls these via `window.go.main.App.*`.

---

## File Operations

### `OpenFile(paths []string) error`

Open one or more local files.

**Parameters**:
- `paths`: Array of absolute file paths

**Returns**: Error if any file fails to open

**Frontend**:
```typescript
await window.go.main.App.OpenFile(["/var/log/syslog", "/home/user/app.log"])
```

---

### `OpenSSHFile(config SSHFileConfig) error`

Open remote file via SSH.

**Parameters** (`SSHFileConfig`):
```typescript
interface SSHFileConfig {
  host: string;
  port: number;
  username: string;
  auth_type: "password" | "key";
  password?: string;
  key_path?: string;
  key_passphrase?: string;
  remote_file_path: string;
}
```

**Returns**: Error on connection/auth failure

**Frontend**:
```typescript
await window.go.main.App.OpenSSHFile({
  host: "10.0.0.5",
  port: 22,
  username: "deploy",
  auth_type: "key",
  key_path: "/home/user/.ssh/id_ed25519",
  remote_file_path: "/var/log/nginx/access.log"
})
```

---

### `OpenFTPFile(config FTPFileConfig) error`

Open remote file via FTP/FTPS.

**Parameters** (`FTPFileConfig`):
```typescript
interface FTPFileConfig {
  host: string;
  port: number;
  username: string;
  password: string;
  tls_mode: "none" | "explicit" | "implicit";
  remote_file_path: string;
}
```

**Returns**: Error on connection/auth failure

---

### `CloseTab(tabID string) error`

Close a tab by ID.

**Parameters**:
- `tabID`: Tab identifier (from `GetTabs`)

**Returns**: Error if tab not found

---

### `GetTabs() []TabInfo`

Get all open tabs.

**Returns**: Array of `TabInfo`:
```typescript
interface TabInfo {
  id: string;
  name: string;
  path: string;
  type: "local" | "ssh" | "ftp";
  group: string;
  active: boolean;
  scroll_position: number;
  follow: boolean;
  search_query: string;
}
```

---

### `SetActiveTab(tabID string) error`

Switch to tab.

**Parameters**:
- `tabID`: Tab identifier

**Returns**: Error if not found

---

### `RenameTab(tabID string, name string) error`

Set custom tab name.

---

### `MoveTab(tabID string, direction "left" | "right") error`

Reorder tab.

---

### `GetTabGroups() []TabGroup`

Get tab groups.

**Returns**: Array of `TabGroup`:
```typescript
interface TabGroup {
  name: string;
  color: string;
  tabs: string[]; // tab IDs
  collapsed: boolean;
}
```

---

### `CreateTabGroup(name string, color string) error`

Create new tab group.

---

### `UpdateTabGroup(name string, color string) error`

Update group color.

---

### `DeleteTabGroup(name string) error`

Delete group (tabs ungrouped).

---

### `AssignTabToGroup(tabID string, groupName string) error`

Assign tab to group.

---

### `RemoveTabFromGroup(tabID string) error`

Remove tab from group.

---

## Log Content

### `GetLogLines(tabID string, start int, count int) ([]LogLine, error)`

Get visible log lines for virtualized rendering.

**Parameters**:
- `tabID`: Tab identifier
- `start`: Starting line index (0-based)
- `count`: Number of lines to fetch

**Returns**: Array of `LogLine`:
```typescript
interface LogLine {
  line_number: number;
  text: string;
  highlights: HighlightRange[];
}
```

---

### `GetLineCount(tabID string) (int, error)`

Get total line count for tab.

---

### `GoToLine(tabID string, lineNumber int) error`

Scroll to specific line (1-based).

---

### `GetFileInfo(tabID string) (FileInfo, error)`

Get file metadata.

**Returns** `FileInfo`:
```typescript
interface FileInfo {
  path: string;
  size: int64;
  lines: int;
  modified: string; // RFC3339
  is_remote: boolean;
}
```

---

## Search

### `SearchLines(tabID string, req SearchRequest) (SearchResult, error)`

Execute search.

**Parameters** (`SearchRequest`):
```typescript
interface SearchRequest {
  query: string;           // primary
  regex: boolean;
  case_sensitive: boolean;
  invert: boolean;
  and_query: string;       // secondary AND
  and_logic: "and" | "or";
  range: SearchRange;
  nearby: NearbyFilter;
  levels: string[];        // "ERROR", "WARN", etc.
  time: TimeFilter;
  exclude: ExcludeFilter;
  top: boolean;
  context_before: int;
  context_after: int;
}
```

**Returns** `SearchResult`:
```typescript
interface SearchResult {
  matches: MatchLine[];
  total_matches: int;
  total_lines: int;
  duration_ms: int;
}
```

---

### `GetSearchMatches(tabID string) ([]MatchLine, error)`

Get current search matches.

---

### `NavigateMatch(tabID string, direction "next" | "prev" | "first" | "last") (int, error)`

Navigate matches. Returns line number.

---

### `NavigateMatchFromCurrent(tabID string, direction "next" | "prev") (int, error)`

Navigate from current viewport line.

---

### `SaveSearchResults(tabID string, path string) error`

Export matches to file.

---

### `SaveFullLog(tabID string, path string) error`

Export full log to file.

---

### `GetSearchPresets() ([]SearchPreset, error)`

Get saved presets.

---

### `SaveSearchPreset(preset SearchPreset) error`

Save preset.

---

### `DeleteSearchPreset(name string) error`

Delete preset.

---

## Follow (Tail)

### `ToggleFollow(tabID string) (bool, error)`

Toggle follow mode. Returns new state.

---

### `GetFollowState(tabID string) (bool, error)`

Get follow state.

---

## Remote Browsing

### `ListSSHDirectory(config SSHFileConfig, path string) ([]RemoteEntry, error)`

Browse SSH remote directory.

**Returns** `RemoteEntry[]`:
```typescript
interface RemoteEntry {
  name: string;
  path: string;
  is_dir: boolean;
  size: int64;
  mod_time: string;
  mode: string;
}
```

---

### `ListFTPDirectory(config FTPFileConfig, path string) ([]RemoteEntry, error)`

Browse FTP remote directory.

---

## Settings

### `GetSettings() (Settings, error)`

Get all settings.

---

### `SaveSettings(settings Settings) error`

Save settings (partial update allowed).

---

### `GetRecentFiles() (RecentFiles, error)`

Get recent files (local, ssh, ftp).

---

### `ClearRecentFiles() error`

Clear recent history.

---

### `GetSavedSession() (SavedSession, error)`

Get saved session tabs.

---

### `ClearSavedSession() error`

Clear saved session.

---

## System

### `GetVersion() (string, error)`

Get app version.

---

### `OpenExternal(url string) error`

Open URL in system browser.

---

### `ShowMessage(title string, message string) error`

Show native message dialog.

---

## Events (Frontend → Go)

Frontend listens to these runtime events:

| Event | Payload | Description |
|-------|---------|-------------|
| `file_updated` | `{tab_id, lines_added}` | New lines appended (follow) |
| `search_progress` | `{tab_id, progress, total}` | Search progress (0-100) |
| `search_complete` | `{tab_id, result}` | Search finished |
| `ssh_progress` | `{tab_id, stage, message}` | SSH connect/download/index |
| `ftp_progress` | `{tab_id, stage, message}` | FTP connect/download/index |
| `tab_closed` | `{tab_id}` | Tab closed |
| `tab_created` | `{tab}` | New tab opened |
| `settings_changed` | `{key, value}` | Setting changed |

---

## Types Reference

All types defined in `wailsjs/go/models.ts` (generated). Key interfaces:

- `TabInfo`
- `TabGroup`
- `LogLine`
- `HighlightRange`
- `MatchLine`
- `SearchRequest`
- `SearchRange`
- `NearbyFilter`
- `TimeFilter`
- `ExcludeFilter`
- `SearchPreset`
- `FileInfo`
- `RemoteEntry`
- `Settings`
- `RecentFiles`
- `SavedSession`
- `SSHFileConfig`
- `FTPFileConfig`

---

## Related Information

- [Go Packages](go-packages.md)
- [Frontend Modules](frontend-modules.md)
- [Frontend Source](../frontend/src/main.ts)