# Settings Reference Table

> **Content type**: Reference — complete settings in tabular format for quick lookup.
> **Audience**: Advanced users, administrators
> **Verified**: 2026-10-29 — matches `pkg/config/config.go` defaults

## Purpose

Single-table reference of all settings with keys, types, defaults, and config file location.

## Complete Settings Table

| Setting Key | JSON Key | Type | Default | Valid Values | Config File | Since | Requires Restart |
|-------------|----------|------|---------|--------------|-------------|-------|------------------|
| **Appearance** |
| Theme | `theme` | string | `system` | `dark`, `light`, `dracula`, `system` | settings.json | v2.0 | Yes |
| Language | `language` | string | `system` | `uk`, `en`, `system` | settings.json | v2.0 | Yes |
| Log Font | `font_family` | string | `JetBrains Mono` | `JetBrains Mono`, `Fira Code`, `Cascadia Code`, `System` | settings.json | v2.0 | No |
| Font Size | `font_size` | int | `13` | `10`–`24` | settings.json | v2.0 | No |
| Line Density | `line_density` | string | `normal` | `compact`, `normal`, `comfortable` | settings.json | v2.0 | No |
| Word Wrap | `word_wrap` | bool | `true` | `true`, `false` | settings.json | v2.0 | No |
| Line Numbers | `show_line_numbers` | bool | `true` | `true`, `false` | settings.json | v2.0 | No |
| **Viewer** |
| View Mode | `view_mode` | string | `split` | `split`, `results`, `normal` | settings.json | v2.0 | No |
| Follow on Open | `default_follow` | bool | `true` | `true`, `false` | settings.json | v2.0 | No |
| Search Orientation | `search_orientation` | string | `horizontal` | `horizontal`, `vertical` | settings.json | v2.0 | No |
| Results Side | `search_side` | string | `bottom` | `bottom`, `top`, `left`, `right` | settings.json | v2.0 | No |
| **Search** |
| Context Before | `context_before` | int | `0` | `0`–`100` | settings.json | v2.0 | No |
| Context After | `context_after` | int | `0` | `0`–`100` | settings.json | v2.0 | No |
| Default Nearby Mode | `nearby_mode` | string | `either` | `either`, `before`, `after` | settings.json | v2.0 | No |
| Default Nearby Window | `nearby_window` | int | `10` | `1`–`200` | settings.json | v2.0 | No |
| Exclude as Regex | `exclude_as_regex` | bool | `false` | `true`, `false` | settings.json | v2.0 | No |
| Require Timestamp | `require_timestamp` | bool | `false` | `true`, `false` | settings.json | v2.0 | No |
| Top Max Unique | `top_limit` | int | `100` | `10`–`10000` | settings.json | v2.0 | No |
| Highlight Primary | `hl_primary` | bool | `true` | `true`, `false` | settings.json | v2.0 | No |
| Highlight AND | `hl_secondary` | bool | `true` | `true`, `false` | settings.json | v2.0 | No |
| Highlight Nearby | `hl_nearby` | bool | `true` | `true`, `false` | settings.json | v2.0 | No |
| Highlight Exclude | `hl_exclude` | bool | `true` | `true`, `false` | settings.json | v2.0 | No |
| **Session** |
| Remember Tabs | `save_tabs_exit` | bool | `true` | `true`, `false` | settings.json | v2.0 | No |
| Recent List Size | `max_recent` | int | `50` | `10`–`500` | settings.json | v2.0 | No |
| Search History Size | `max_history` | int | `50` | `10`–`200` | settings.json | v2.0 | No |
| Confirm Close Remote | `confirm_close_remote_tab` | bool | `true` | `true`, `false` | settings.json | v2.0 | No |
| **Remote** |
| Connection Timeout | `ssh_timeout` | int | `15` | `5`–`120` (sec) | settings.json | v2.0 | No |
| Stall Timeout | `stall_timeout` | int | `30` | `10`–`300` (sec) | settings.json | v2.0 | No |
| Auto-Accept Host Key | `auto_accept_host_key` | bool | `false` | `true`, `false` | settings.json | v2.0 | No |
| Failed Attempt Limit | `conn_max_attempts` | int | `3` | `1`–`10` | settings.json | v2.0 | No |
| Attempt Window | `conn_window` | int | `10` | `1`–`60` (min) | settings.json | v2.0 | No |
| Lockout Duration | `conn_lockout` | int | `15` | `1`–`120` (min) | settings.json | v2.0 | No |
| **Internal** |
| Secret Key | `secret_key` | string | generated | base64 32-byte | settings.json | v2.0 | N/A |
| Window Width | `window_width` | int | `1200` | — | settings.json | v2.0 | No |
| Window Height | `window_height` | int | `800` | — | settings.json | v2.0 | No |
| Window Maximized | `window_maximized` | bool | `true` | — | settings.json | v2.0 | No |
| Search Pane Height | `search_pane_height` | int | `250` | — | settings.json | v2.0 | No |
| Startup Behavior | `startup_behavior` | string | `session` | `session`, `empty`, `open` | settings.json | v2.0 | No |

## Default Search Presets (Built-in)

None — presets are user-created only.

## Environment Variables

GoFLE does not read settings from environment variables. All configuration via JSON files.

## Version History

| Version | Changes |
|---------|---------|
| v2.0 | Initial settings structure |
| v2.1 | Added `dracula` theme, `comfortable` density, remote lockout settings |

## Related Information

- [Settings UI](settings-ui.md)
- [Config Files](config-files.md)
- [Security Notes](../../../SECURITY.md#security-related-configuration)