# Advanced Search

> **Content type**: Task — use advanced filters (Nearby, Levels, Time, Exclude, Range, Top, Presets).
> **Audience**: All users
> **Prerequisites**: [Search Basics](search-basics.md) — basic search working
> **Verified**: 2026-10-29 — tested with local and remote files

## Purpose

Apply additional post-search filters to narrow results beyond the primary query and AND field.

## Prerequisites

- Log file open in active tab
- Search toolbar visible
- Basic search executed (primary query entered)

## Procedure

### 1. Open Advanced Filters Bar

The advanced filters bar appears above the results pane as a row of chips:
**Nearby** | **Levels** | **Time** | **Exclude** | **Range** | **Top** | **Presets**

- Click a chip to open its configuration form below the bar
- Active chips are highlighted
- Chip with outline = form currently open
- Click chip again or press `Esc` to close form

### 2. Nearby Filter

**Purpose**: Keep only primary-match lines that have another pattern within ±N lines.

**Configuration**:
1. Click **Nearby** chip
2. Enter **Nearby pattern** (text or regex)
3. Select **Mode**:
   - **Before** — pattern appears above the match
   - **After** — pattern appears below the match
   - **Either** — pattern on either side
4. Set **Window** (lines): 1–200 (default: 10)
5. Click **Apply**

**Example**:
```
Primary: Exception
Nearby pattern: UserService
Mode: Before
Window: 15
→ Keep Exception lines that have "UserService" within 15 lines above
```

### 3. Levels Filter

**Purpose**: Filter by log level heuristic.

**Configuration**:
1. Click **Levels** chip
2. Select one or more levels:
   - **ERROR** — includes FATAL, CRITICAL, PANIC
   - **WARN** — includes WARNING
   - **INFO**
   - **DEBUG** — includes TRACE, DBG
   - **OTHER** — lines not matching known markers
3. Click **Apply**

**Note**: Without primary query, acts as "all lines of selected levels".

### 4. Time Filter

**Purpose**: Limit results to a date/time range.

**Configuration**:
1. Click **Time** chip
2. Enter **From** date: `YYYY-MM-DD` (or `YYYYMMDD`)
3. Enter **To** date: `YYYY-MM-DD` (or `YYYYMMDD`)
4. (Optional) Enter **From** time: `HH:MM`
5. (Optional) Enter **To** time: `HH:MM`
6. (Optional) Enable **Require timestamp** — drops lines without parseable timestamp
7. Click **Apply**

**Time defaults**:
- Date only, no time: From = 00:00:00, To = 23:59:59 of that day
- Keyboard: after 2 hour digits, focus moves to minutes

**Example**:
```
From: 2026-10-07
To: 2026-10-07
→ All events on Oct 7, 2026
```

### 5. Exclude Filter

**Purpose**: Remove lines matching noise patterns.

**Configuration**:
1. Click **Exclude** chip
2. Enter patterns (one per line)
3. Enable **As regex** if patterns are regular expressions
4. Click **Apply**

**Example**:
```
healthcheck
metrics
/GET \/health/
→ Removes lines containing "healthcheck", "metrics", or matching the regex
```

### 6. Range Filter

**Purpose**: Limit search scope to a portion of the file.

**Configuration**:
1. Click **Range** chip
2. Select **Mode**:
   - **Entire file** — default, no position limit
   - **From cursor down** — from visible line in log pane downward
   - **Up to cursor** — from file start to visible line
   - **Line numbers** — specific start/end line numbers
   - **Last N lines** — tail of file only
3. For Line numbers: enter **Start line** (1-based) and **End line** (0 = EOF)
4. For Last N lines: enter number of trailing lines
5. Click **Apply**

**Use cases**:
- **From cursor down**: Scroll to incident area, then search only below
- **Last N lines**: Fast search on huge files when only recent logs matter

### 7. Top Unique

**Purpose**: Aggregate frequent match patterns from current results.

**Configuration**:
1. Click **Top** chip
2. Results pane replaces with aggregated view:
   - Count × normalized pattern
   - Click a row to jump to first occurrence
3. Click **Top** again to return to normal results

**Normalization**: Timestamps and long numbers replaced with placeholders.

**Limit**: Configurable in Settings → Search → Top: max unique rows (default: 100)

### 8. Presets

**Purpose**: Save and load named filter combinations.

**Save Preset**:
1. Configure desired filters
2. Click **Presets** chip
3. Click **Save**
4. Enter name
5. Click **OK**

**Load Preset**:
1. Click **Presets** chip
2. Select preset from list
3. Filters apply and search re-runs

**Manage**:
- **Delete**: hover preset, click ×
- Stored in `localStorage` (shared GUI/CLI via `search_presets.json`)

## Expected Result

- Results narrowed by active filters
- Active chips highlighted
- Results pane updates on each Apply
- Counter reflects filtered count

## Verification

1. Open file with mixed log levels and timestamps
2. Apply **Levels**: ERROR only — confirm only ERROR lines
3. Apply **Time**: today — confirm only today's lines
4. Apply **Exclude**: healthcheck — confirm healthcheck lines removed
5. Apply **Range**: Last 1000 lines — confirm only tail searched
6. Enable **Top** — confirm aggregated patterns shown
7. Save preset — reload — confirm filters restored

## Filter Combination Logic

Filters apply in this order:
1. **Range** — limits file scan scope
2. **Primary + AND** — initial match set
3. **Levels, Time, Exclude, Nearby** — post-filters on match set
4. **Top** — aggregation of filtered results (display only)

## Troubleshooting

| Symptom | Cause | Resolution |
|---------|-------|------------|
| Nearby finds nothing | Pattern not in window | Increase window; check mode (Before/After/Either) |
| Time filter removes all | Timestamps not parsed | Disable "Require timestamp"; check log timestamp format |
| Exclude not working | Regex not enabled | Enable "As regex" for regex patterns |
| Range "From cursor" wrong | Cursor not in log pane | Click in upper (log) pane first |
| Top shows few results | Limit too low | Increase Top limit in Settings → Search |

## Related Information

- [Search Basics](search-basics.md)
- [Search Presets](search-presets.md)
- [Search Settings](../../configuration/settings-ui.md#search-settings)
- [Keyboard Shortcuts](keyboard-shortcuts.md)