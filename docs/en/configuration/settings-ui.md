# Settings UI Reference

> **Content type**: Reference — complete settings organized by tab.
> **Audience**: All users
> **Verified**: 2026-10-29 — matches `frontend/src/settings_manager.ts` and `pkg/config/config.go`

## Purpose

Document every user-facing setting in the GoFLE Settings dialog.

## Settings Tabs

### Appearance

| Setting | Values | Default | Description |
|---------|--------|---------|-------------|
| **Theme** | `dark`, `light`, `dracula`, `system` | `system` | UI color theme. "System" follows OS preference. |
| **Language** | `uk`, `en`, `system` | `system` | Interface language. "System" follows OS locale. |
| **Log Font** | `JetBrains Mono`, `Fira Code`, `Cascadia Code`, `System` | `JetBrains Mono` | Font family for log text. Bundled fonts guaranteed available. |
| **Font Size** | `10`–`24` px | `13` | Log text size in pixels. |
| **Line Density** | `Compact`, `Normal`, `Comfortable` | `Normal` | Vertical spacing between log rows. |
| **Word Wrap** | `On`, `Off` | `On` | Wrap long lines (no horizontal scroll). Heavier scrolling on huge files. |
| **Line Numbers** | `On`, `Off` | `On` | Show line number column left of log text. |

### Viewer

| Setting | Values | Default | Description |
|---------|--------|---------|-------------|
| **View Mode** | `Split` (Log + Results), `Results Only`, `Log Only` | `Split` | Default pane layout when opening a file. |
| **Follow (Tail)** | `On`, `Off` | `On` | Enable follow mode when opening a file. |
| **Search Orientation** | `Horizontal`, `Vertical` | `Horizontal` | Results pane above/below (Horizontal) or left/right (Vertical). |
| **Results Side** | `Bottom`, `Top`, `Left`, `Right` | `Bottom` | Which side of log holds results pane. |

### Search

| Setting | Values | Default | Description |
|---------|--------|---------|-------------|
| **Context Lines Before** | `0`–`100` | `0` | Reserved for future/compat. Results show match lines only. |
| **Context Lines After** | `0`–`100` | `0` | Reserved for future/compat. |
| **Default Nearby Mode** | `Either`, `Before`, `After` | `Either` | Default for Nearby filter. |
| **Default Nearby Window** | `1`–`200` | `10` | Default window in lines for Nearby filter. |
| **Exclude as Regex** | `On`, `Off` | `Off` | Treat Exclude patterns as regex by default. |
| **Require Timestamp** | `On`, `Off` | `Off` | Default for Time filter: drop lines without parseable timestamp. |
| **Top: Max Unique Rows** | `10`–`10000` | `100` | Maximum unique patterns shown in Top aggregation. |
| **Highlight: Primary** | `On`, `Off` | `On` | Highlight primary query terms in log text. |
| **Highlight: AND** | `On`, `Off` | `On` | Highlight AND field terms in log text. |
| **Highlight: Nearby** | `On`, `Off` | `On` | Highlight Nearby pattern terms in log text. |
| **Highlight: Exclude** | `On`, `Off` | `On` | Highlight Exclude pattern terms in log text. |

### Session

| Setting | Values | Default | Description |
|---------|--------|---------|-------------|
| **Remember Open Tabs** | `On`, `Off` | `On` | Save tabs on exit; restore on next launch. |
| **Recent List Size** | `10`–`500` | `50` | Maximum entries in Recent (Ctrl+R) list. |
| **Search History Size** | `10`–`200` | `50` | Maximum saved queries in search input history. |
| **Confirm Closing SSH/FTP Tabs** | `On`, `Off` | `On` | Prompt before closing tab with active remote connection. |

### Remote

| Setting | Values | Default | Description |
|---------|--------|---------|-------------|
| **Connection Timeout** | `5`–`120` sec | `15` | Seconds to wait for SSH/FTP connection before abort. |
| **Download Silence Timeout** | `10`–`300` sec | `30` | Abort if no bytes received for this long during initial download. Slow but active transfers not affected. |
| **Auto-Accept Host Key** | `On`, `Off` | `Off` | **Dangerous**: store unknown SSH host key / FTPS cert without prompting. Unsafe on untrusted networks. |
| **Failed Attempt Limit** | `1`–`10` | `3` | After this many failed connections to one host, pause further tries. |
| **Attempt Window** | `1`–`60` min | `10` | Time span for counting failed attempts. |
| **Lockout Duration** | `1`–`120` min | `15` | Minutes to block new attempts after limit exceeded. |

## Settings Persistence

- Stored in `settings.json` in config directory
- Changes apply immediately (most settings)
- Some require restart (Theme, Language)
- Exported/imported with config directory backup

## Config Directory Locations

| Platform | Path |
|----------|------|
| Linux | `~/.config/gofle/` |
| macOS | `~/Library/Application Support/gofle/` |
| Windows | `%APPDATA%\gofle\` |

## Related Information

- [Config Files](config-files.md)
- [Settings Reference](settings-reference.md)
- [CLI Settings](../../cli-reference/cli-commands.md#settings)