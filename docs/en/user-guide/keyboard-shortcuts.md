# Keyboard Shortcuts Reference

> **Content type**: Reference — complete keyboard shortcuts for GoFLE GUI and terminal viewer.
> **Audience**: All users
> **Verified**: 2026-10-29 — matches actual keybindings in code

## Purpose

Complete reference of all keyboard shortcuts for efficient GoFLE usage.

## GUI Shortcuts

### File & Navigation

| Shortcut | Action | Context |
|----------|--------|---------|
| `Ctrl+O` | Open file(s) | Global |
| `Ctrl+R` | Recent files | Global |
| `Ctrl+G` | Go to line | Global |
| `Ctrl+W` | Close current tab | Global |
| `Ctrl+Tab` | Next tab | Global |
| `Ctrl+Shift+Tab` | Previous tab | Global |
| `1`–`9` | Jump to tab 1–9 | Global |
| `Ctrl+Shift+T` | Reopen closed tab | Global |

### Search

| Shortcut | Action | Context |
|----------|--------|---------|
| `/` | Focus search box | Global |
| `Enter` | Execute search | Search field |
| `Esc` | Clear search / close dialog | Search field, dialogs |
| `Alt+C` | Toggle Match Case | Global |
| `Alt+R` | Toggle Regex | Global |
| `Alt+I` | Toggle Invert Match | Global |
| `\` | Focus AND field | Global |
| `F3` / `Alt+↓` | Next match | Global |
| `Shift+F3` / `Alt+↑` | Previous match | Global |
| `Alt+Home` | First match | Global |
| `Alt+End` | Last match | Global |

### Follow & View

| Shortcut | Action | Context |
|----------|--------|---------|
| `F` | Toggle Follow (Tail) | Global |
| `r` | Toggle Results Only view | Global |
| `s` / `m` | Split view / cycle view mode | Global |

### Help & Settings

| Shortcut | Action | Context |
|----------|--------|---------|
| `F1` | Open Help | Global |
| `?` | Open Help | Global |
| `Ctrl+,` | Open Settings | Global |

### Export

| Shortcut | Action | Context |
|----------|--------|---------|
| `Ctrl+S` | Save Search Results | Results pane focused |
| `Ctrl+Shift+S` | Save Full Log | Log pane focused |

### Dialogs & Forms

| Shortcut | Action | Context |
|----------|--------|---------|
| `Esc` | Close dialog / blur focus | Any dialog |
| `Enter` | Submit form / apply filter | Forms |
| `Tab` / `Shift+Tab` | Next/previous field | Forms |

## Terminal Viewer Shortcuts (CLI `gofle`)

### Navigation

| Key | Action |
|-----|--------|
| `↑` / `↓` | Scroll line by line |
| `PgUp` / `PgDn` | Scroll page |
| `Home` / `End` | Jump to top / bottom |
| `g` | Go to line number |

### Search

| Key | Action |
|-----|--------|
| `/` | Start search |
| `Alt+R` | Toggle Regex |
| `Alt+I` | Toggle Invert |
| `Alt+C` | Toggle Case |
| `n` / `p` | Next / previous match |
| `[` / `]` | First / last match |
| `\` | Secondary AND filter |

### Advanced Filters

| Key | Action |
|-----|--------|
| `A` | Open Advanced Filters form |
| `u` | Toggle Top Unique |
| `P` | Search Presets |

### Tabs & Groups

| Key | Action |
|-----|--------|
| `Ctrl+Tab` / `Ctrl+PgDn` | Next tab |
| `Ctrl+Shift+Tab` / `Ctrl+PgUp` | Previous tab |
| `1`–`9` | Jump to tab N |
| `T` | Open recent as new tab |
| `t` | In recent picker: open as new tab |
| `G` | Tab group (name/color) |
| `Y` | Restore tabs from session |

### View & Export

| Key | Action |
|-----|--------|
| `r` | Toggle Results Only |
| `s` / `m` | Split view / cycle view mode |
| `f` | Toggle Follow |
| `e` | Export (file or matches) |
| `v` | Toggle view mode |

### Settings & Help

| Key | Action |
|-----|--------|
| `F2` / `,` | Settings |
| `?` | Full help overlay |
| `q` / `Ctrl+C` | Quit |

### Clipboard

| Key | Action |
|-----|--------|
| `Ctrl+C` | Copy selection |
| `Ctrl+Shift+C` | Copy full lines |

## Mouse Shortcuts

| Action | Result |
|--------|--------|
| Middle-click tab | Close tab |
| Drag tab | Reorder tabs |
| Drag splitter | Resize panes |
| Double-click result | Jump to line in log |
| Right-click log/results | Context menu |
| Right-click tab | Tab context menu |
| Right-click group label | Group context menu |
| Scroll wheel | Scroll pane |
| `Ctrl` + Scroll | Zoom font size |

## Customization

Shortcuts are **not currently customizable**. They are hardcoded in:
- GUI: `frontend/src/main.ts` and component files
- CLI: `cmd/gofle/internal/viewer/*.go`

## Related Information

- [Search Basics](search-basics.md)
- [Advanced Search](search-advanced.md)
- [Tabs and Groups](tabs-groups.md)
- [Follow (Live Tail)](follow-tail.md)
- [CLI Viewer Keys](../../cli-reference/cli-viewer-keys.md)