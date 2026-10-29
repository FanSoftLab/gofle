# Search Presets

> **Content type**: Task — save, load, and manage named advanced filter combinations.
> **Audience**: All users
> **Prerequisites**: [Advanced Search](search-advanced.md) — filters configured
> **Verified**: 2026-10-29 — tested GUI and CLI preset sharing

## Purpose

Save a combination of advanced search filters (Nearby, Levels, Time, Exclude, Range, Top) under a name for quick reuse.

## Prerequisites

- Log file open
- Advanced filters configured as desired
- At least one filter active (otherwise preset saves empty state)

## Procedure

### Save a Preset (GUI)

1. Configure desired advanced filters (Nearby, Levels, Time, Exclude, Range)
2. Click **Presets** chip in advanced filters bar
3. Click **Save** button in preset form
4. Enter a descriptive name (e.g., "Production Errors", "SSH Debug")
5. Click **OK** or press `Enter`
6. Preset appears in preset list

### Load a Preset (GUI)

1. Click **Presets** chip
2. Select preset from list
3. Filters apply automatically
4. Search re-runs with preset values

### Delete a Preset (GUI)

1. Click **Presets** chip
2. Hover over preset in list
3. Click **×** (delete) icon
4. Confirm deletion

### Share Presets (GUI ↔ CLI)

Presets stored in `localStorage` (browser/webview) and `~/.config/gofle/search_presets.json` (shared).

**Export from GUI**:
1. Click **Presets** chip
2. Click **Export** (if available) or use CLI

**Import to GUI**:
- Presets from `search_presets.json` load automatically on startup

### CLI Preset Management

```bash
# List presets
gofle presets list

# Export presets (GUI-compatible JSON)
gofle presets export > my-presets.json
gofle presets export my-presets.json

# Import presets
gofle presets import my-presets.json
```

**Preset JSON format**:
```json
[
  {
    "name": "Production Errors",
    "nearby": {"pattern": "UserService", "mode": "before", "window": 15},
    "levels": ["ERROR"],
    "time": {"from": "2026-10-01", "to": "2026-10-07"},
    "exclude": {"patterns": ["healthcheck"], "asRegex": false},
    "range": {"mode": "cursor_down"}
  }
]
```

## Expected Result

- Preset saved and appears in list
- Loading preset restores all filter values
- Search re-runs automatically
- Presets persist across sessions (localStorage + config file)

## Verification

1. Configure: Levels=ERROR, Time=today, Exclude=healthcheck
2. Save as "Daily Errors"
3. Clear all filters
4. Load "Daily Errors"
5. Confirm Levels, Time, Exclude restored
6. Run CLI: `gofle presets list` — confirm preset listed

## Troubleshooting

| Symptom | Cause | Resolution |
|---------|-------|------------|
| Preset not loading | Corrupted localStorage | Clear browser data or delete `search_presets.json` |
| CLI doesn't see GUI presets | Different config directory | Check `~/.config/gofle/search_presets.json` exists |
| Import fails | Invalid JSON | Validate JSON syntax; use exported file as template |
| Preset name conflict | Duplicate name | Delete old preset first or use different name |

## Related Information

- [Advanced Search](search-advanced.md)
- [Search Settings](../../configuration/settings-ui.md#search-settings)
- [CLI Presets Command](../../cli-reference/cli-commands.md#presets)
- [Config Files](../../configuration/config-files.md#search_presetsjson)