# Tabs and Groups

> **Content type**: Task — manage multiple log files with tabs and colored tab groups.
> **Audience**: All users
> **Prerequisites**: [Open Files](open-files.md) — at least one file open
> **Verified**: 2026-10-29 — tested with 10+ concurrent tabs

## Purpose

Organize multiple open log files using tabs and tab groups for visual separation (e.g., Production vs Staging).

## Prerequisites

- GoFLE running
- At least one file open

## Procedure

### Basic Tab Operations

| Action | Method |
|--------|--------|
| **Open new tab** | `Ctrl+O` (Open), drag-drop, CLI args, Recent |
| **Switch tab** | Click tab, `Ctrl+Tab` (next), `Ctrl+Shift+Tab` (prev), `1`–`9` (tab 1–9) |
| **Close tab** | Middle-click tab, `Ctrl+W`, right-click → Close Tab |
| **Reorder tabs** | Drag tab horizontally |
| **Reopen closed tab** | `Ctrl+Shift+T` (if implemented) or right-click tab bar → Reopen Closed |

### Tab Context Menu

Right-click a tab to access:

- **Tab Settings** — rename, assign group
- **Close Tab** — close this tab
- **Close Other Tabs** — keep only this tab
- **Close Tabs to Right** — close all tabs right of this one
- **Move Tab Left/Right** — reorder
- **New Tab** — open file dialog
- **Move to Group** — assign to existing group
- **New Group...** — create new group and assign
- **Remove from Group** — ungroup this tab
- **Rename Tab** — custom tab name
- **Browse Server** (remote only) — open another file on same server

### Tab Groups

**Create a Group**:
1. Right-click a tab → **New Group...**
2. Enter **Group Name** (e.g., "Production", "Incident-2026-10-07")
3. Select **Color** from palette
4. Click **Save**

**Assign Tab to Group**:
1. Right-click tab → **Move to Group** → select group
2. Or: Tab Settings → Group dropdown → select group

**Group Visuals**:
- Group name appears as colored label in tab bar
- Tabs in group show group color accent
- Groups can be collapsed/expanded (click group label)

**Group Context Menu** (right-click group label):
- **Edit Group** — rename, change color
- **Collapse Group** — hide tabs in group
- **Expand Group** — show tabs in group
- **Close Group** — close all tabs in group

### Session Restore

Enable in **Settings → Session → Remember open tabs**:

- On exit: open tabs (local + remote) saved to `saved_tabs.json`
- On next launch: all tabs restored automatically
- Remote tabs reconnect (SSH/FTP) — may prompt for password if not saved

### Keyboard Shortcuts

| Shortcut | Action |
|----------|--------|
| `Ctrl+O` | Open file(s) — new tab(s) |
| `Ctrl+W` | Close current tab |
| `Ctrl+Tab` | Next tab |
| `Ctrl+Shift+Tab` | Previous tab |
| `1`–`9` | Jump to tab 1–9 |
| `Ctrl+Shift+T` | Reopen last closed tab (if available) |

## Expected Result

- Multiple files open in separate tabs
- Tabs grouped visually by color/label
- Tab state (scroll, search, follow) preserved per tab
- Session restore works across restarts

## Verification

1. Open 3–4 log files — confirm each in own tab
2. Create group "Test Group" (red)
3. Assign 2 tabs to group — confirm red accent
4. Collapse group — confirm tabs hidden
5. Expand group — confirm tabs visible
6. Close group — confirm all tabs in group closed
7. Enable session restore — restart GoFLE — confirm tabs restored

## Troubleshooting

| Symptom | Cause | Resolution |
|---------|-------|------------|
| Tab closes immediately | Remote tab, confirm close enabled | Disable "Confirm closing SSH/FTP tabs" in Settings |
| Session not restored | Setting disabled | Enable "Remember open tabs" in Settings → Session |
| Remote tab fails to restore | Password not saved | Re-enter password on reconnect; or save in Recent |
| Group color not visible | Theme issue | Check theme supports group colors (all built-in do) |

## Related Information

- [Session Restore](session-restore.md)
- [Recent Files](../configuration/config-files.md#recentjson)
- [Session Settings](../../configuration/settings-ui.md#session-settings)
- [Keyboard Shortcuts](keyboard-shortcuts.md)