# Search Issues Troubleshooting

> **Content type**: Troubleshooting — diagnose and resolve search-related issues.
> **Audience**: All users
> **Severity**: Medium
> **Verified**: 2026-10-29 — tested against common search failure modes

## Purpose

Resolve no results, regex errors, advanced filter issues, highlighting problems, and navigation issues.

---

## Symptoms Index

| Symptom | Jump To |
|---------|---------|
| No results found | [No Results](#no-results) |
| Regex error (red field) | [Regex Error](#regex-error) |
| Advanced filters not working | [Advanced Filters Not Working](#advanced-filters-not-working) |
| Highlighting not showing | [Highlighting Not Showing](#highlighting-not-showing) |
| Match navigation broken | [Match Navigation Broken](#match-navigation-broken) |
| Search case sensitivity wrong | [Case Sensitivity Wrong](#case-sensitivity-wrong) |
| AND field ignored | [AND Field Ignored](#and-field-ignored) |
| Results not jumping to line | [Results Not Jumping](#results-not-jumping) |

---

## No Results

**Symptom**: Search executes but results pane shows "0 matches"

### Conditions

- Pattern not in file
- Case sensitivity mismatch
- Invert match enabled
- Advanced filters too restrictive
- Range filter excludes matches

### Possible Causes

| Cause | Verified |
|-------|----------|
| Pattern not present | Yes |
| Match Case on, pattern wrong case | Yes |
| Invert Match enabled | Yes |
| Levels filter excludes | Yes |
| Time filter excludes | Yes |
| Exclude filter removes all | Yes |
| Range filter limits scope | Yes |

### Diagnosis

1. Clear all filters (Esc) — search again
2. Disable Match Case (Alt+C)
3. Disable Invert Match (Alt+I)
4. Check active filter chips (highlighted = active)
5. Try broader pattern: `error` → `err`

### Resolution

1. Verify pattern exists: `grep -i "pattern" /path/file` (local) or manual check
2. Disable restrictive filters
3. Adjust Range: "Entire file" instead of "Last N lines"
4. Check Time filter: disable "Require timestamp" if logs lack timestamps

---

## Regex Error

**Symptom**: Search field turns red, search doesn't execute

### Conditions

- Invalid RE2/Go regex syntax
- Unescaped special characters
- Unsupported regex feature

### RE2 Syntax Notes

RE2 (Go regex) does NOT support:
- Lookahead/lookbehind: `(?=...)`, `(?!...)`, `(?<=...)`, `(?<!...)`
- Backreferences: `\1`, `\2`
- Conditional: `(?(cond)yes|no)`
- Recursion: `(?R)`

### Common Errors

| Invalid | Valid Alternative |
|---------|-------------------|
| `\d` | `[0-9]` or `\d` (supported in RE2) |
| `\w` | `[a-zA-Z0-9_]` |
| `(?=foo)` | Not supported — restructure |
| `\1` | Not supported — use capture groups differently |

### Diagnosis

1. Read error tooltip on red field
2. Test regex at: https://regex101.com/ (select Golang flavor)
3. Simplify pattern step by step

### Resolution

1. Escape special chars: `. * + ? ^ $ ( ) [ ] { } | \` → `\.` `\*` etc.
2. Use RE2-compatible syntax only
3. For complex patterns: use substring search + AND field + advanced filters

---

## Advanced Filters Not Working

**Symptom**: Filter chip clicked, form opens, Apply clicked, but results don't change

### Conditions

- Filter form values not saved
- Filter logic misunderstood
- Primary query empty + filter behavior

### Possible Causes

| Cause | Verified |
|-------|----------|
| Didn't click Apply | Yes |
| Filter form values invalid | Yes |
| Primary empty + filter behavior | Yes |
| Filter combination logic | Yes |

### Diagnosis

1. Verify chip highlighted after Apply
2. Check form values: pattern non-empty, window > 0
3. Try with primary query set
4. Check filter combination order (Range → Primary/AND → Post-filters)

### Resolution

1. Always click **Apply** after configuring filter
2. For Nearby: pattern required, window 1-200
3. For Levels: at least one level selected
4. For Time: valid date format (YYYY-MM-DD)
5. For Range: "From cursor down" requires cursor in log pane

---

## Highlighting Not Showing

**Symptom**: Matches found but no color highlighting in log/results

### Conditions

- Highlight toggles off in Settings
- Word Wrap interferes
- Results pane focused (different highlighting)

### Possible Causes

| Cause | Verified |
|-------|----------|
| Highlight disabled in Settings | Yes |
| Word Wrap on | Yes |
| Results Only view | Yes |

### Diagnosis

1. Settings → Search → Highlight toggles: Primary, AND, Nearby, Exclude
2. Disable Word Wrap: Settings → Viewer
3. Check view mode: Split vs Results Only

### Resolution

1. Enable desired highlight toggles in Settings → Search
2. Disable Word Wrap for reliable highlighting
3. Highlight works in both panes in Split mode

---

## Match Navigation Broken

**Symptom**: F3 / Shift+F3 / navigation buttons don't move to next match

### Conditions

- No active search
- Results pane empty
- Focus in wrong pane

### Possible Causes

| Cause | Verified |
|-------|----------|
| No search executed | Yes |
| Results cleared | Yes |
| Focus in log pane (shortcuts global) | Yes |

### Diagnosis

1. Check results pane has matches (counter > 0)
2. Execute search (Enter in search field)
3. Try navigation buttons in results pane toolbar

### Resolution

1. Ensure search has results
2. Navigation shortcuts work globally (F3, Alt+↓, etc.)
3. Click result in results pane to jump

---

## Case Sensitivity Wrong

**Symptom**: Search matches wrong case (e.g., "error" matches "ERROR" when Match Case on)

### Conditions

- Match Case toggle state unclear
- Setting default overrides

### Possible Causes

| Cause | Verified |
|-------|----------|
| Match Case off (default) | Yes |
| Default setting overrides | Yes |

### Diagnosis

1. Check Match Case button state (Cc) — highlighted = on
2. Settings → Search → default Match Case

### Resolution

1. Toggle Match Case (Alt+C) to desired state
2. Set default in Settings → Search → Match Case

---

## AND Field Ignored

**Symptom**: Text in AND field doesn't filter results

### Conditions

- Primary query empty
- AND logic set to OR
- Advanced filters override

### Possible Causes

| Cause | Verified |
|-------|----------|
| Primary empty + advanced filters | Yes |
| AND/OR logic set to OR | Yes |

### Diagnosis

1. Enter primary query (required for AND to apply)
2. Check AND/OR dropdown: should be AND
3. Clear advanced filters

### Resolution

1. Always set primary query when using AND field
2. Set logic to AND (default)
3. AND field refines primary matches only

---

## Results Not Jumping

**Symptom**: Click result in results pane, log pane doesn't scroll to line

### Conditions

- Word Wrap enabled (line height variable)
- Line numbers disabled
- Results pane sync issue

### Possible Causes

| Cause | Verified |
|-------|----------|
| Word Wrap on | Yes |
| Line numbers off | Possible |
| Results virtualization | Possible |

### Diagnosis

1. Disable Word Wrap: Settings → Viewer
2. Enable Line Numbers: Settings → Viewer
3. Try keyboard navigation (F3) instead of click

### Resolution

1. **Disable Word Wrap** — fixes jump position calculation
2. Enable Line Numbers for reference
3. Use keyboard shortcuts for reliable navigation

---

## Related Information

- [Search Basics](../user-guide/search-basics.md)
- [Advanced Search](../user-guide/search-advanced.md)
- [Search Presets](../user-guide/search-presets.md)
- [Search Settings](../configuration/settings-ui.md#search-settings)
- [Keyboard Shortcuts](../user-guide/keyboard-shortcuts.md)