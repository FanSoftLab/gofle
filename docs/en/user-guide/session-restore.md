# Session Restore

> **Content type**: Task — configure and use automatic session restore on startup.
> **Audience**: All users
> **Prerequisites**: [Tabs and Groups](tabs-groups.md) — tabs open
> **Verified**: 2026-10-29 — tested across restarts with local and remote tabs

## Purpose

Automatically restore open tabs (local files, SSH, FTP) on next GoFLE launch.

## Prerequisites

- GoFLE installed
- At least one tab open (to save)

## Procedure

### Enable Session Restore

1. Open **Settings** (gear icon) → **Session** tab
2. Enable **Remember open tabs** (default: on)
3. (Optional) Adjust **Recent list size** and **Search history size**
4. Close Settings

### Save Session Manually

Session saves automatically on clean exit. To force save:
- Close all tabs except one, then close GoFLE
- Or: session saves on each tab change (debounced)

### Restore Session

1. Launch GoFLE without file arguments:
   ```bash
   ./GoFLE
   ```
2. All previously open tabs restore:
   - Local files: reopen directly
   - SSH tabs: reconnect (may prompt for password if key not saved)
   - FTP tabs: reconnect (may prompt for password)
3. Active tab, tab order, groups, scroll positions restored

### Startup Behavior Options

**Settings → Session → On startup**:

| Option | Behavior |
|--------|----------|
| **Restore session** | Restore saved tabs (default) |
| **Empty window** | Start with no tabs |
| **Open dialog** | Show Open File dialog immediately |

### Disable Session Restore

1. Settings → Session → **Remember open tabs** → Off
2. Next launch: empty window (or per Startup Behavior)

### Clear Saved Session

**GUI**:
- Settings → Session → **Clear saved session** (if button exists)
- Or: close all tabs, exit, restart

**CLI**:
```bash
gofle session clear
```

**Config file**:
- Delete `~/.config/gofle/saved_tabs.json`

## Expected Result

- On launch: previously open tabs appear
- Remote tabs show "Connecting..." → "Indexing..." → content
- Tab order, groups, active tab preserved
- Scroll position per tab preserved

## Verification

1. Open 2 local files + 1 SSH connection
2. Arrange tabs, create a group
3. Exit GoFLE cleanly (close window)
4. Launch `./GoFLE`
5. Confirm: 3 tabs open, group present, SSH reconnects

## Troubleshooting

| Symptom | Cause | Resolution |
|---------|-------|------------|
| Tabs not restored | Setting disabled | Enable "Remember open tabs" in Settings → Session |
| SSH tab fails to restore | Password not saved / key changed | Re-enter password; verify SSH key in `known_hosts` |
| FTP tab fails to restore | Certificate not trusted | Accept certificate on reconnect |
| Wrong tab active | Session data corrupted | Clear saved session (`gofle session clear`) |
| Local file not found | File moved/deleted | Tab shows error; remove from Recent |

## Config Files Involved

| File | Purpose |
|------|---------|
| `saved_tabs.json` | Snapshot of open tabs (when enabled) |
| `recent.json` | Recent files list (credentials encrypted) |
| `settings.json` | `save_tabs_exit` flag |

## Related Information

- [Tabs and Groups](tabs-groups.md)
- [Recent Files](../configuration/config-files.md#recentjson)
- [Session Settings](../../configuration/settings-ui.md#session-settings)
- [CLI Session Command](../../cli-reference/cli-commands.md#session)