# Search Basics

> **Content type**: Task — search log content using primary query, regex, case sensitivity, and invert match.
> **Audience**: All users
> **Prerequisites**: [Open Files](open-files.md) or [Open Remote](open-remote-ssh.md) — file loaded in tab
> **Verified**: 2026-10-29 — tested with local and remote files

## Purpose

Find lines matching a pattern in the currently open log file using the search toolbar.

## Prerequisites

- Log file open in active tab
- Search toolbar visible (default)

## Procedure

### 1. Focus Search Box

- Press `/` (forward slash) — focuses primary search field
- Or click the primary search field (left field)

### 2. Enter Primary Query

Type your search pattern:

- **Plain text** (default): matches substring anywhere in line
- **Regular expression**: enable Regex toggle (see step 3)

Examples:
```
ERROR
timeout
request-id-12345
Failed to connect
```

### 3. Configure Search Options

| Option | Shortcut | Description |
|--------|----------|-------------|
| **Match Case** | `Alt+C` | Case-sensitive search (default: off) |
| **Regex** | `Alt+R` | Interpret query as RE2/Go regex (default: off) |
| **Invert Match** | `Alt+I` | Show lines that do NOT match (default: off) |

Click the toggle buttons or use keyboard shortcuts.

### 4. Execute Search

- Press `Enter` in search field, OR
- Click the search icon (magnifying glass), OR
- Search runs automatically after brief debounce

### 5. View Results

- Results appear in the **results pane** (lower pane by default)
- Each result shows: line number, matching line content
- Click a result to jump to that line in the full log (upper pane)
- Counter shows: "X matches of Y lines"

### 6. Navigate Matches

| Action | Shortcut | Button |
|--------|----------|--------|
| Next match | `F3` / `Alt+↓` | ↓ |
| Previous match | `Shift+F3` / `Alt+↑` | ↑ |
| First match | `Alt+Home` | ⇈ |
| Last match | `Alt+End` | ⇊ |

### 7. Use AND Field (Secondary Filter)

- Enter text in the **AND field** (right field, labeled "AND")
- Both primary AND secondary must match on the same line
- Logic: AND (default) or OR (toggle via dropdown)

Example:
```
Primary: ERROR
AND: database
→ Lines containing BOTH "ERROR" and "database"
```

### 8. Clear Search

- Press `Esc` in search field, OR
- Click **×** (clear) button next to search field
- Both primary and AND fields cleared

## Expected Result

- Results pane populated with matching lines
- Matching terms highlighted in both panes (configurable)
- Line numbers shown in results
- Full log pane jumps to selected result line

## Verification

1. Open a file with known content (e.g., `/var/log/syslog`)
2. Search for `ERROR` — confirm ERROR lines appear
3. Enable **Match Case** — confirm `error` no longer matches
4. Enable **Regex** — search `ERROR|WARN` — confirm both match
5. Enable **Invert** — confirm non-matching lines shown
6. Use AND field — confirm both conditions required

## Search Behavior Details

| Aspect | Behavior |
|--------|----------|
| **Engine** | Parallel worker pool (CPU cores) over line-offset index |
| **Index** | Only line offsets in memory; lines read via `pread` on demand |
| **Regex** | RE2/Go syntax; invalid regex highlighted, search paused |
| **Case** | Applied in engine, not post-filter |
| **Invert** | Applied in engine, not post-filter |
| **AND** | Refine pass over primary hits (when only AND changes) |
| **Results** | Virtualized — only visible rows in DOM |

## Troubleshooting

| Symptom | Cause | Resolution |
|---------|-------|------------|
| No results | Pattern not in file | Check spelling; try broader pattern |
| Regex error (red field) | Invalid RE2 syntax | Fix regex; escape special chars: `. * + ? ^ $ ( ) [ ] { } | \` |
| Search slow on huge file | First search builds cache | Subsequent searches faster (regex cached) |
| Results not jumping | Line wrap enabled | Disable Word Wrap in Settings → Viewer |
| AND field ignored | Primary empty + advanced filters | Advanced filters apply post-primary; AND still works |

## Related Information

- [Advanced Search](search-advanced.md)
- [Search Presets](search-presets.md)
- [Keyboard Shortcuts](keyboard-shortcuts.md)
- [Search Settings](../../configuration/settings-ui.md#search-settings)