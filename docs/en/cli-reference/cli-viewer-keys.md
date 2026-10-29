# CLI Viewer Keybindings

> **Content type**: Reference — all keybindings in the `gofle` terminal viewer.
> **Audience**: CLI users
> **Verified**: 2026-10-29 — matches `cmd/gofle/internal/viewer/*.go`

## Purpose

Complete keybinding reference for the Bubbletea-based interactive terminal viewer.

---

## Navigation

| Key | Action |
|-----|--------|
| `↑` / `↓` | Scroll line by line |
| `PgUp` / `PgDn` | Scroll page |
| `Home` / `End` | Jump to top / bottom |
| `g` | Go to line (prompts for number) |

## Search

| Key | Action |
|-----|--------|
| `/` | Start search (opens search input at bottom) |
| `Enter` (in search) | Execute search |
| `Esc` (in search) | Cancel search input |
| `Alt+R` | Toggle Regex mode |
| `Alt+I` | Toggle Invert Match |
| `Alt+C` | Toggle Match Case |
| `n` / `p` | Next / Previous match |
| `[` / `]` | First / Last match |
| `\` | Secondary AND filter (opens AND input) |

## Advanced Filters

| Key | Action |
|-----|--------|
| `A` | Open Advanced Filters form (Nearby, Levels, Time, Exclude, Range, Top, Presets) |
| `u` | Toggle Top Unique overlay |
| `P` | Open Search Presets (load/save/delete) |

## Tabs & Groups

| Key | Action |
|-----|--------|
| `Ctrl+Tab` / `Ctrl+PgDn` | Next tab |
| `Ctrl+Shift+Tab` / `Ctrl+PgUp` | Previous tab |
| `1`–`9` | Jump to tab N |
| `T` | Open Recent list → open selection as new tab |
| `t` | In Recent picker: open highlighted as new tab |
| `G` | Tab Group — name/color for active tab |
| `Y` | Restore tabs from saved session |
| `Ctrl+W` | Close current tab (last tab quits) |

## View & Export

| Key | Action |
|-----|--------|
| `r` | Toggle Results Only view (filter mode) |
| `s` / `m` | Split view / cycle view mode (Log+Results / Results Only / Log Only) |
| `f` | Toggle Follow (Tail) |
| `e` | Export — opens export dialog (whole file or matches) |
| `v` | Toggle view mode |

## Settings & Help

| Key | Action |
|-----|--------|
| `F2` / `,` | Open Settings (view mode, search defaults, context/nearby/top) |
| `?` | Show full help overlay |
| `q` / `Ctrl+C` | Quit viewer |

## Clipboard

| Key | Action |
|-----|--------|
| `Ctrl+C` | Copy selection (if text selected) |
| `Ctrl+Shift+C` | Copy full lines (entire visible lines) |

## Context Menu (Right-Click)

Not available in terminal viewer. Use keybindings above.

## Mouse Support

- **Scroll wheel**: Scroll log/results
- **Click**: Focus pane, select line
- **Drag**: Select text (for `Ctrl+C`)

Mouse support depends on terminal emulator (most modern terminals support it).

## Search Input Mode

When `/` opens search input:

| Key | Action |
|-----|--------|
| `Tab` | Cycle: Primary → AND → Regex → Case → Invert → Search History |
| `Shift+Tab` | Reverse cycle |
| `Enter` | Execute search |
| `Esc` | Close search input |
| `↑` / `↓` | Navigate search history |

## Advanced Filters Form

When `A` opens form:

| Key | Action |
|-----|--------|
| `Tab` / `Shift+Tab` | Navigate fields |
| `Enter` | Apply / save current filter |
| `Esc` | Close form |
| `Space` | Toggle checkboxes (Levels, etc.) |
| `←` / `→` | Change radio options (Nearby mode, Range mode) |

## Presets Menu

When `P` opens presets:

| Key | Action |
|-----|--------|
| `↑` / `↓` | Navigate presets |
| `Enter` | Load preset |
| `n` | New preset (save current filters) |
| `d` | Delete highlighted preset |
| `Esc` | Close |

## Settings Menu

When `F2` / `,` opens settings:

| Key | Action |
|-----|--------|
| `Tab` / `Shift+Tab` | Navigate sections |
| `Enter` / `Space` | Toggle / edit value |
| `Esc` | Close and save |

## Tab Indicators

Tab bar shows:
- Tab name (truncated)
- Group color indicator (if grouped)
- Remote badge: `SSH` / `FTP` (for remote tabs)
- Active tab highlighted

## Status Bar

Bottom bar shows:
- File path / remote URL
- Line count / match count
- Follow state: `Tail: ACTIVE` / `Tail: PAUSED` / `Tail: —`
- Search state: `Results Only` badge (press `r` to toggle)
- Current time (optional)

## Color Highlighting

| Element | Color | Toggle |
|---------|-------|--------|
| Primary match | Gold/Yellow | Settings |
| AND match | Green | Settings |
| Nearby match | Cyan | Settings |
| Exclude match | Red | Settings |
| Line numbers | Dim | Settings |

Colors configurable in Settings (shared with GUI via `settings.json`).

## Related Information

- [CLI Overview](cli-overview.md)
- [CLI Commands](cli-commands.md)
- [CLI Examples](cli-examples.md)
- [GUI Keyboard Shortcuts](../user-guide/keyboard-shortcuts.md)