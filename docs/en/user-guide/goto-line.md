# Go to Line

> **Content type**: Task — jump to a specific line number in the log.
> **Audience**: All users
> **Prerequisites**: [Open Files](open-files.md) — file loaded in tab
> **Verified**: 2026-10-29 — tested with files up to 10M lines

## Purpose

Navigate directly to a known line number in the full log pane.

## Prerequisites

- Log file open in active tab
- Line number known (1-based, up to total lines in file)

## Procedure

### Open Go to Line Dialog

- Press `Ctrl+G`, OR
- Click **Go to Line** in tab context menu (right-click tab → Go to Line)

### Enter Line Number

1. Dialog opens with "Line Number (1..N):" field
2. Enter line number (1 to total lines)
3. Press `Enter` or click **Go**

### Result

- Full log pane scrolls to the specified line
- Line highlighted briefly
- Line number visible in line number column (if enabled)

### Line Number Limits

- Minimum: 1
- Maximum: total lines in file (shown in dialog as `{max}`)
- Invalid input: shows error "Enter a valid line number (between 1 and {max})"

## Expected Result

- Log pane scrolls to exact line
- Line centered in view (if possible)
- Line number column shows target line

## Verification

1. Open a file with known line count (e.g., `wc -l file.log`)
2. Press `Ctrl+G`
3. Enter line 1 — confirm first line visible
4. Enter last line — confirm last line visible
5. Enter middle line — confirm correct line centered
6. Enter invalid (0 or >max) — confirm error shown

## Troubleshooting

| Symptom | Cause | Resolution |
|---------|-------|------------|
| "Enter a valid line number" | Input out of range | Check max lines in dialog; enter 1–max |
| Dialog doesn't open | Shortcut conflict | Check `Ctrl+G` not overridden; use context menu |
| Jumps to wrong line | Line wrap enabled | Disable Word Wrap in Settings → Viewer |
| Line number column missing | Setting disabled | Enable "Line numbers" in Settings → Viewer |

## Related Information

- [Keyboard Shortcuts](keyboard-shortcuts.md)
- [Viewer Settings](../../configuration/settings-ui.md#viewer-settings)
- [Search Results Navigation](search-basics.md#navigate-matches)