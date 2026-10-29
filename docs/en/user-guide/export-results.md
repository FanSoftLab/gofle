# Export Results

> **Content type**: Task — save search results or full log to a file.
> **Audience**: All users
> **Prerequisites**: [Open Files](open-files.md) — file loaded; [Search Basics](search-basics.md) — search executed (for results export)
> **Verified**: 2026-10-29 — tested local and remote files

## Purpose

Export matched lines (search results) or the entire log file to disk for sharing or archival.

## Prerequisites

- Log file open in active tab
- Write access to destination directory
- For results export: search executed with matches

## Procedure

### Export Search Results

**Method 1: Toolbar Button**
1. Execute a search (matches visible in results pane)
2. Click **Save Search Results** button (download icon) in results pane toolbar, OR
3. Press `Ctrl+S`

**Method 2: Context Menu**
1. Right-click in results pane
2. Select **Save Search Results**

**Method 3: Keyboard Shortcut**
- Press `Ctrl+S` with results pane focused

**Result**: File save dialog opens. Default name: `search-results-<timestamp>.log`

### Export Full Log

**Method 1: Tab Context Menu**
1. Right-click the tab (or in log pane)
2. Select **Save Log As...**

**Method 2: Context Menu in Log Pane**
1. Right-click in upper (log) pane
2. Select **Save Log As...**

**Result**: File save dialog opens. Default name: `<original-filename>-export.log`

### Export Format

**Search Results Export**:
- Each line: `<line-number>: <log-line>`
- Only matched lines (respecting all active filters)
- UTF-8 encoding

**Full Log Export**:
- Complete file content as read by GoFLE
- All lines, no filtering
- UTF-8 encoding

### Remote Files

- Works identically for SSH/FTP tabs
- Exports from local temporary file (complete indexed content)
- Search results export uses current filtered results

## Expected Result

- File written to chosen location
- Success toast notification: "Search results successfully saved to: <path>" or "Full log successfully saved to: <path>"
- File readable in any text editor

## Verification

1. Open a log file
2. Search for `ERROR` — confirm matches in results
3. Export results (`Ctrl+S`) to `~/test-results.log`
4. Open `~/test-results.log` — confirm format: `123: ERROR something`
5. Export full log via context menu to `~/test-full.log`
6. Compare line count with status bar

## Troubleshooting

| Symptom | Cause | Resolution |
|---------|-------|------------|
| "Permission denied" | No write access to destination | Choose writable directory (e.g., `~/Documents`) |
| Export empty | No matches (results) / file empty (full) | Verify search has results; verify file has content |
| Export truncated | Disk full | Free disk space |
| Remote export incomplete | Indexing not finished | Wait for "Indexing..." to complete |

## Related Information

- [Search Basics](search-basics.md)
- [Advanced Search](search-advanced.md)
- [Context Menu](../configuration/settings-ui.md#context-menu)
- [Keyboard Shortcuts](keyboard-shortcuts.md)