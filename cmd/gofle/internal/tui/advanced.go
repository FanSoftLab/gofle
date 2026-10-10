package tui

import (
	"GoFLE/pkg/config"
	"GoFLE/pkg/search"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Advanced filter form field indices (order in advForm).
const (
	advFieldExclude = iota
	advFieldExcludeRegex
	advFieldNearby
	advFieldNearbyMode
	advFieldNearbyWin
	advFieldLevels
	advFieldTimeFrom
	advFieldTimeTo
	advFieldTimeReq
	advFieldLineStart
	advFieldLineEnd
	advFieldCtxBefore
	advFieldCtxAfter
	advFieldCount
)

var advFieldLabels = []string{
	"Exclude",
	"Exclude regex",
	"Nearby query",
	"Nearby mode",
	"Nearby window",
	"Levels",
	"Time from",
	"Time to",
	"Require stamp",
	"Line start",
	"Line end",
	"Context before",
	"Context after",
}

var advFieldHints = []string{
	"comma-separated patterns to drop",
	"y/n — treat exclude as regex",
	"pattern that must appear nearby",
	"before | after | either",
	"window size in lines (default 10)",
	"ERROR,WARN,INFO,DEBUG,OTHER",
	"RFC3339 or 2006-01-02 15:04:05",
	"RFC3339 or 2006-01-02 15:04:05",
	"y/n — drop lines without timestamp",
	"0-based inclusive start (empty=0)",
	"exclusive end (empty=EOF)",
	"context lines before each match",
	"context lines after each match",
}

// newAdvForm builds the advanced-filter text inputs with placeholders.
func newAdvForm() []textinput.Model {
	forms := make([]textinput.Model, advFieldCount)
	placeholders := []string{
		"debug,trace",
		"n",
		"timeout",
		"either",
		"10",
		"ERROR,WARN",
		"",
		"",
		"n",
		"",
		"",
		"0",
		"0",
	}
	for i := range forms {
		ti := textinput.New()
		ti.Placeholder = placeholders[i]
		ti.CharLimit = 256
		forms[i] = ti
	}
	return forms
}

// fillAdvFormFromState copies applied advanced filters into the form inputs.
func (m *model) fillAdvFormFromState() {
	if len(m.advForm) < advFieldCount {
		m.advForm = newAdvForm()
	}
	m.advForm[advFieldExclude].SetValue(strings.Join(m.advExclude, ","))
	m.advForm[advFieldExcludeRegex].SetValue(boolYN(m.advExcludeRegex))
	m.advForm[advFieldNearby].SetValue(m.advNearbyQuery)
	mode := m.advNearbyMode
	if mode == "" {
		mode = "either"
	}
	m.advForm[advFieldNearbyMode].SetValue(mode)
	win := m.advNearbyWindow
	if win <= 0 {
		win = 10
		if m.cfg != nil && m.cfg.NearbyWindow > 0 {
			win = m.cfg.NearbyWindow
		}
	}
	m.advForm[advFieldNearbyWin].SetValue(strconv.Itoa(win))
	m.advForm[advFieldLevels].SetValue(strings.Join(m.advLevels, ","))
	m.advForm[advFieldTimeFrom].SetValue(m.advTimeFrom)
	m.advForm[advFieldTimeTo].SetValue(m.advTimeTo)
	m.advForm[advFieldTimeReq].SetValue(boolYN(m.advTimeRequire))
	if m.advLineStart > 0 {
		m.advForm[advFieldLineStart].SetValue(strconv.FormatInt(m.advLineStart, 10))
	} else {
		m.advForm[advFieldLineStart].SetValue("")
	}
	if m.advLineEnd > 0 {
		m.advForm[advFieldLineEnd].SetValue(strconv.FormatInt(m.advLineEnd, 10))
	} else {
		m.advForm[advFieldLineEnd].SetValue("")
	}
	m.advForm[advFieldCtxBefore].SetValue(strconv.Itoa(m.advCtxBefore))
	m.advForm[advFieldCtxAfter].SetValue(strconv.Itoa(m.advCtxAfter))
}

// openAdvancedForm opens the advanced-filter overlay and focuses the first field.
func (m *model) openAdvancedForm() tea.Cmd {
	m.fillAdvFormFromState()
	m.advMode = true
	m.advFocus = 0
	for i := range m.advForm {
		m.advForm[i].Blur()
	}
	return m.advForm[0].Focus()
}

// applyAdvForm reads form values into model state (does not run search).
func (m *model) applyAdvForm() error {
	if len(m.advForm) < advFieldCount {
		return nil
	}
	m.advExclude = splitCSV(m.advForm[advFieldExclude].Value())
	m.advExcludeRegex = parseYN(m.advForm[advFieldExcludeRegex].Value(), m.advExcludeRegex)
	m.advNearbyQuery = strings.TrimSpace(m.advForm[advFieldNearby].Value())
	mode := strings.ToLower(strings.TrimSpace(m.advForm[advFieldNearbyMode].Value()))
	switch mode {
	case "before", "after", "either", "":
		if mode == "" {
			mode = "either"
		}
		m.advNearbyMode = mode
	default:
		return fmt.Errorf("nearby mode must be before|after|either, got %q", mode)
	}
	if w := strings.TrimSpace(m.advForm[advFieldNearbyWin].Value()); w != "" {
		n, err := strconv.Atoi(w)
		if err != nil || n < 0 || n > 200 {
			return fmt.Errorf("nearby window must be 0..200, got %q", w)
		}
		m.advNearbyWindow = n
	} else {
		m.advNearbyWindow = 10
	}
	m.advLevels = splitCSVUpper(m.advForm[advFieldLevels].Value())
	m.advTimeFrom = strings.TrimSpace(m.advForm[advFieldTimeFrom].Value())
	m.advTimeTo = strings.TrimSpace(m.advForm[advFieldTimeTo].Value())
	m.advTimeRequire = parseYN(m.advForm[advFieldTimeReq].Value(), m.advTimeRequire)

	ls, err := parseOptionalInt64(m.advForm[advFieldLineStart].Value())
	if err != nil {
		return fmt.Errorf("line start: %w", err)
	}
	le, err := parseOptionalInt64(m.advForm[advFieldLineEnd].Value())
	if err != nil {
		return fmt.Errorf("line end: %w", err)
	}
	m.advLineStart = ls
	m.advLineEnd = le

	cb, err := parseOptionalInt(m.advForm[advFieldCtxBefore].Value())
	if err != nil {
		return fmt.Errorf("context before: %w", err)
	}
	ca, err := parseOptionalInt(m.advForm[advFieldCtxAfter].Value())
	if err != nil {
		return fmt.Errorf("context after: %w", err)
	}
	if cb > 50 {
		cb = 50
	}
	if ca > 50 {
		ca = 50
	}
	m.advCtxBefore = cb
	m.advCtxAfter = ca
	return nil
}

// clearAdvancedFilters resets all advanced filter state to empty.
func (m *model) clearAdvancedFilters() {
	m.advExclude = nil
	m.advExcludeRegex = false
	m.advNearbyQuery = ""
	m.advNearbyMode = ""
	m.advNearbyWindow = 0
	m.advLevels = nil
	m.advTimeFrom = ""
	m.advTimeTo = ""
	m.advTimeRequire = false
	m.advLineStart = 0
	m.advLineEnd = 0
	m.advCtxBefore = 0
	m.advCtxAfter = 0
}

// seedAdvancedDefaults applies shared settings.json defaults (context, nearby
// window/mode, exclude-as-regex, time-require) without enabling any filter.
func (m *model) seedAdvancedDefaults() {
	if m.cfg == nil {
		return
	}
	if m.cfg.ExcludeRegexDefault {
		m.advExcludeRegex = true
	}
	if m.cfg.TimeRequireStamp {
		m.advTimeRequire = true
	}
	if m.cfg.CtxBefore > 0 {
		m.advCtxBefore = m.cfg.CtxBefore
	}
	if m.cfg.CtxAfter > 0 {
		m.advCtxAfter = m.cfg.CtxAfter
	}
	if m.cfg.NearbyWindow > 0 {
		m.advNearbyWindow = m.cfg.NearbyWindow
	}
	if m.cfg.NearbyMode != "" {
		m.advNearbyMode = m.cfg.NearbyMode
	}
}

// hasAdvancedFilters reports whether any advanced filter is active.
func (m model) hasAdvancedFilters() bool {
	return len(m.advExclude) > 0 ||
		m.advNearbyQuery != "" ||
		len(m.advLevels) > 0 ||
		m.advTimeFrom != "" || m.advTimeTo != "" ||
		m.advLineStart > 0 || m.advLineEnd > 0
}

// hasContextExpand reports whether match list should be expanded with context.
func (m model) hasContextExpand() bool {
	return m.advCtxBefore > 0 || m.advCtxAfter > 0
}

// applyAdvancedToOpts copies advanced state into SearchOptions (delegates to pkg/search).
func (m model) applyAdvancedToOpts(opts *search.SearchOptions) {
	if len(m.advExclude) > 0 {
		opts.ExcludePatterns = append([]string(nil), m.advExclude...)
		opts.ExcludeIsRegex = m.advExcludeRegex
	}
	if m.advNearbyQuery != "" {
		opts.NearbyQuery = m.advNearbyQuery
		opts.NearbyMode = m.advNearbyMode
		if opts.NearbyMode == "" {
			opts.NearbyMode = "either"
		}
		opts.NearbyWindow = m.advNearbyWindow
		if opts.NearbyWindow <= 0 {
			opts.NearbyWindow = 10
		}
	}
	if len(m.advLevels) > 0 {
		opts.Levels = append([]string(nil), m.advLevels...)
	}
	opts.TimeFrom = m.advTimeFrom
	opts.TimeTo = m.advTimeTo
	opts.TimeRequireStamp = m.advTimeRequire
	opts.LineStart = m.advLineStart
	opts.LineEnd = m.advLineEnd
	opts.ContextBefore = m.advCtxBefore
	opts.ContextAfter = m.advCtxAfter
}

// advancedSummary returns a short status fragment for active advanced filters.
func (m model) advancedSummary() string {
	var parts []string
	if n := len(m.advExclude); n > 0 {
		parts = append(parts, fmt.Sprintf("excl:%d", n))
	}
	if m.advNearbyQuery != "" {
		parts = append(parts, fmt.Sprintf("near:%s±%d", truncate(m.advNearbyQuery, 12), m.advNearbyWindow))
	}
	if len(m.advLevels) > 0 {
		parts = append(parts, "lvl:"+strings.Join(m.advLevels, ","))
	}
	if m.advTimeFrom != "" || m.advTimeTo != "" {
		parts = append(parts, "time")
	}
	if m.advLineStart > 0 || m.advLineEnd > 0 {
		parts = append(parts, fmt.Sprintf("range:%d-%d", m.advLineStart, m.advLineEnd))
	}
	if m.advCtxBefore > 0 || m.advCtxAfter > 0 {
		parts = append(parts, fmt.Sprintf("ctx:-%d/+%d", m.advCtxBefore, m.advCtxAfter))
	}
	return strings.Join(parts, " ")
}

// advancedFormKey handles keys while the advanced form overlay is open.
func (m *model) advancedFormKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	switch {
	case msg.Type == tea.KeyEsc || key == "ctrl+c":
		m.advMode = false
		return *m, nil
	case msg.Type == tea.KeyEnter:
		if err := m.applyAdvForm(); err != nil {
			m.status = "advanced: " + err.Error()
			return *m, nil
		}
		m.advMode = false
		sum := m.advancedSummary()
		if sum != "" {
			m.status = "advanced applied · " + sum
		} else {
			m.status = "advanced filters cleared"
		}
		return *m, m.startSearchCmd()
	case msg.Type == tea.KeyTab || key == "down" || key == "j":
		m.advForm[m.advFocus].Blur()
		m.advFocus = (m.advFocus + 1) % advFieldCount
		return *m, m.advForm[m.advFocus].Focus()
	case key == "shift+tab" || key == "up" || key == "k":
		m.advForm[m.advFocus].Blur()
		m.advFocus = (m.advFocus + advFieldCount - 1) % advFieldCount
		return *m, m.advForm[m.advFocus].Focus()
	case key == "ctrl+x":
		// Clear all advanced filters from the form.
		m.clearAdvancedFilters()
		m.fillAdvFormFromState()
		m.status = "advanced form reset"
		return *m, m.advForm[m.advFocus].Focus()
	}

	var cmd tea.Cmd
	m.advForm[m.advFocus], cmd = m.advForm[m.advFocus].Update(msg)
	return *m, cmd
}

// advancedFormView renders the full-screen advanced filter form.
func (m model) advancedFormView() string {
	var b strings.Builder
	b.WriteString(headerStyle.Render("GoFLE  ·  advanced filters"))
	b.WriteString("\n")
	b.WriteString(dimStyle.Render("Tab/↑↓ move · Enter apply & search · Ctrl+X clear form · Esc cancel"))
	b.WriteString("\n\n")

	for i := 0; i < advFieldCount && i < len(m.advForm); i++ {
		label := fmt.Sprintf("%-16s", advFieldLabels[i]+":")
		if i == m.advFocus {
			b.WriteString(keyStyle.Render("▸ " + label))
			b.WriteString(m.advForm[i].View())
		} else {
			b.WriteString(dimStyle.Render("  " + label))
			val := m.advForm[i].Value()
			if val == "" {
				val = "(" + m.advForm[i].Placeholder + ")"
				b.WriteString(dimStyle.Render(val))
			} else {
				b.WriteString(val)
			}
		}
		b.WriteString("\n")
		if i == m.advFocus {
			b.WriteString(dimStyle.Render("    " + advFieldHints[i]))
			b.WriteString("\n")
		}
	}
	b.WriteString("\n")
	b.WriteString(infoStyle.Render("Filters are applied via pkg/search (no duplicated match logic)."))
	return b.String()
}

// --- Top unique -----------------------------------------------------------

type topUniqueEntry struct {
	Text  string
	Count int
}

// openTopUnique computes frequent match texts over the current result set and
// opens an overlay. Memory-bounded: scans at most 20k matches, keeps top N.
//
// Prefers primaryMatches (pre-context-expansion) so context rows do not dilute
// the ranking. When a primary query is set, groups by the matched substring
// when it can be extracted; otherwise by the full line.
func (m *model) openTopUnique() {
	src := m.matches
	if len(m.primaryMatches) > 0 {
		src = m.primaryMatches
	}
	if len(src) == 0 {
		m.status = "top: no matches — run a search first"
		return
	}
	limit := 100
	if m.cfg != nil && m.cfg.TopLimit > 0 {
		limit = m.cfg.TopLimit
	}
	const maxScan = 20000
	counts := make(map[string]int, 256)
	n := len(src)
	if n > maxScan {
		n = maxScan
	}
	opts := m.effectiveSearchOptions()
	for i := 0; i < n; i++ {
		ln := src[i]
		txt, err := m.idx.GetLine(ln)
		if err != nil {
			continue
		}
		txt = strings.TrimSpace(txt)
		if txt == "" {
			continue
		}
		key := topGroupKey(txt, opts)
		counts[key]++
	}
	if len(counts) == 0 {
		m.status = "top: could not read any match lines"
		return
	}
	entries := make([]topUniqueEntry, 0, len(counts))
	for t, c := range counts {
		entries = append(entries, topUniqueEntry{Text: t, Count: c})
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Count != entries[j].Count {
			return entries[i].Count > entries[j].Count
		}
		return entries[i].Text < entries[j].Text
	})
	if len(entries) > limit {
		entries = entries[:limit]
	}
	m.topEntries = entries
	m.topCursor = 0
	m.topMode = true
	m.status = fmt.Sprintf("top unique: %d groups (scanned %d lines)", len(entries), n)
}

// topGroupKey returns the string used to bucket a match line in Top unique.
func topGroupKey(line string, opts search.SearchOptions) string {
	q := strings.TrimSpace(opts.Query)
	if q != "" && !opts.MatchAll {
		if extract := extractMatchSnippet(line, q, opts.IsRegex, opts.CaseSensitive); extract != "" {
			if len(extract) > 200 {
				return extract[:200]
			}
			return extract
		}
	}
	if len(line) > 200 {
		return line[:200]
	}
	return line
}

// extractMatchSnippet finds the first occurrence of pattern in line.
func extractMatchSnippet(line, pattern string, isRegex, caseSensitive bool) string {
	if pattern == "" || line == "" {
		return ""
	}
	if isRegex {
		flags := ""
		if !caseSensitive {
			flags = "(?i)"
		}
		re, err := regexp.Compile(flags + pattern)
		if err != nil {
			return extractMatchSnippet(line, pattern, false, caseSensitive)
		}
		loc := re.FindStringIndex(line)
		if loc == nil {
			return ""
		}
		return line[loc[0]:loc[1]]
	}
	if caseSensitive {
		i := strings.Index(line, pattern)
		if i < 0 {
			return ""
		}
		return line[i : i+len(pattern)]
	}
	lower := strings.ToLower(line)
	lp := strings.ToLower(pattern)
	i := strings.Index(lower, lp)
	if i < 0 {
		return ""
	}
	end := i + len(pattern)
	if end > len(line) {
		end = len(line)
	}
	return line[i:end]
}

// topUniqueKey handles the top-unique overlay.
func (m *model) topUniqueKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	switch key {
	case keyEsc, "q", "ctrl+c":
		m.topMode = false
		m.topEntries = nil
		return *m, nil
	case "up", "k":
		if len(m.topEntries) > 0 {
			m.topCursor = (m.topCursor + len(m.topEntries) - 1) % len(m.topEntries)
		}
	case keyDown, "j":
		if len(m.topEntries) > 0 {
			m.topCursor = (m.topCursor + 1) % len(m.topEntries)
		}
	case "pgup", "ctrl+u":
		m.topCursor = max(0, m.topCursor-10)
	case "pgdown", "pgdn", "ctrl+d":
		m.topCursor = min(len(m.topEntries)-1, m.topCursor+10)
	case "home":
		m.topCursor = 0
	case "end", "G":
		if len(m.topEntries) > 0 {
			m.topCursor = len(m.topEntries) - 1
		}
	case keyEnter:
		// Jump to first result line that belongs to the selected unique group.
		if m.topCursor >= 0 && m.topCursor < len(m.topEntries) {
			want := m.topEntries[m.topCursor].Text
			opts := m.effectiveSearchOptions()
			for i, ln := range m.matches {
				txt, err := m.idx.GetLine(ln)
				if err != nil {
					continue
				}
				txt = strings.TrimSpace(txt)
				if topGroupKey(txt, opts) == want || strings.Contains(txt, want) {
					m.gotoMatch(i)
					m.topMode = false
					m.topEntries = nil
					m.status = fmt.Sprintf("jumped to first of %q", truncate(want, 40))
					return *m, nil
				}
			}
			m.status = "top: no matching line found"
		}
	}
	return *m, nil
}

// topUniqueView renders the top-unique overlay.
func (m model) topUniqueView() string {
	var b strings.Builder
	b.WriteString(headerStyle.Render("GoFLE  ·  top unique matches"))
	b.WriteString("\n")
	b.WriteString(dimStyle.Render("↑↓ select · Enter jump to first · Esc close"))
	b.WriteString("\n\n")
	if len(m.topEntries) == 0 {
		b.WriteString(dimStyle.Render("(empty)"))
		return b.String()
	}
	// Show a window around the cursor.
	height := max(m.height-4, 8)
	start := 0
	if m.topCursor >= height {
		start = m.topCursor - height + 1
	}
	end := min(len(m.topEntries), start+height)
	for i := start; i < end; i++ {
		e := m.topEntries[i]
		line := fmt.Sprintf("%6d  %s", e.Count, truncate(e.Text, max(m.width-12, 20)))
		if i == m.topCursor {
			b.WriteString(matchCursorStyle.Render(line))
		} else {
			b.WriteString(line)
		}
		b.WriteString("\n")
	}
	return b.String()
}

// --- Presets (shared with GUI via pkg/config) ------------------------------

// openPresetPicker loads presets from config and opens the picker overlay.
func (m *model) openPresetPicker() {
	if m.cfg == nil {
		m.status = "presets: no config"
		return
	}
	m.presetList = m.cfg.GetSearchPresets()
	m.presetMode = true
	m.presetCursor = 0
	m.presetSaveMode = false
	if len(m.presetList) == 0 {
		m.status = "presets: empty — press s to save current filters"
	}
}

// presetKey handles the preset picker / save overlay.
func (m *model) presetKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	if m.presetSaveMode {
		switch {
		case msg.Type == tea.KeyEsc:
			m.presetSaveMode = false
			return *m, nil
		case msg.Type == tea.KeyEnter:
			name := strings.TrimSpace(m.presetNameInput.Value())
			if name == "" {
				name = "preset"
			}
			p := m.currentPreset(name)
			if err := m.cfg.AddSearchPreset(p); err != nil {
				m.status = "preset save: " + err.Error()
			} else {
				m.presetList = m.cfg.GetSearchPresets()
				m.status = "preset saved: " + name
			}
			m.presetSaveMode = false
			return *m, nil
		}
		var cmd tea.Cmd
		m.presetNameInput, cmd = m.presetNameInput.Update(msg)
		return *m, cmd
	}

	switch key {
	case keyEsc, "q", "ctrl+c":
		m.presetMode = false
		return *m, nil
	case "up", "k":
		if len(m.presetList) > 0 {
			m.presetCursor = (m.presetCursor + len(m.presetList) - 1) % len(m.presetList)
		}
	case keyDown, "j":
		if len(m.presetList) > 0 {
			m.presetCursor = (m.presetCursor + 1) % len(m.presetList)
		}
	case "s":
		// Save current advanced filters as a new preset.
		m.presetSaveMode = true
		m.presetNameInput.SetValue("")
		return *m, m.presetNameInput.Focus()
	case "d", "x":
		if len(m.presetList) == 0 {
			return *m, nil
		}
		name := m.presetList[m.presetCursor].Name
		if err := m.cfg.RemoveSearchPreset(name); err != nil {
			m.status = "preset delete: " + err.Error()
		} else {
			m.presetList = m.cfg.GetSearchPresets()
			if m.presetCursor >= len(m.presetList) && m.presetCursor > 0 {
				m.presetCursor--
			}
			m.status = "preset deleted: " + name
		}
	case keyEnter:
		if len(m.presetList) == 0 {
			return *m, nil
		}
		name := m.presetList[m.presetCursor].Name
		m.applyPreset(m.presetList[m.presetCursor])
		m.presetMode = false
		m.status = "preset loaded: " + name
		return *m, m.startSearchCmd()
	}
	return *m, nil
}

// currentPreset snapshots the current advanced filters into a SearchPreset.
func (m model) currentPreset(name string) config.SearchPreset {
	return config.SearchPreset{
		Name:            name,
		ExcludePatterns: append([]string(nil), m.advExclude...),
		ExcludeIsRegex:  m.advExcludeRegex,
		NearbyQuery:     m.advNearbyQuery,
		NearbyMode:      m.advNearbyMode,
		NearbyWindow:    m.advNearbyWindow,
		Levels:          append([]string(nil), m.advLevels...),
		TimeFrom:        m.advTimeFrom,
		TimeTo:          m.advTimeTo,
		TimeRequire:     m.advTimeRequire,
		LineStart:       m.advLineStart,
		LineEnd:         m.advLineEnd,
		CtxBefore:       m.advCtxBefore,
		CtxAfter:        m.advCtxAfter,
		Query:           m.query,
		SecondaryQuery:  m.secondaryQuery,
		IsRegex:         m.searchOpts.IsRegex,
		CaseSensitive:   m.searchOpts.CaseSensitive,
		InvertMatch:     m.searchOpts.InvertMatch,
	}
}

// applyPreset loads a preset into model state (search + advanced).
func (m *model) applyPreset(p config.SearchPreset) {
	m.advExclude = append([]string(nil), p.ExcludePatterns...)
	m.advExcludeRegex = p.ExcludeIsRegex
	m.advNearbyQuery = p.NearbyQuery
	m.advNearbyMode = p.NearbyMode
	m.advNearbyWindow = p.NearbyWindow
	m.advLevels = append([]string(nil), p.Levels...)
	m.advTimeFrom = p.TimeFrom
	m.advTimeTo = p.TimeTo
	m.advTimeRequire = p.TimeRequire
	m.advLineStart = p.LineStart
	m.advLineEnd = p.LineEnd
	m.advCtxBefore = p.CtxBefore
	m.advCtxAfter = p.CtxAfter
	if p.Query != "" {
		m.query = p.Query
		m.searchInput.SetValue(p.Query)
	}
	m.secondaryQuery = p.SecondaryQuery
	m.secondaryInput.SetValue(p.SecondaryQuery)
	m.searchOpts.IsRegex = p.IsRegex
	m.searchOpts.CaseSensitive = p.CaseSensitive
	m.searchOpts.InvertMatch = p.InvertMatch
}

// presetView renders the preset picker.
func (m model) presetView() string {
	var b strings.Builder
	b.WriteString(headerStyle.Render("GoFLE  ·  search presets"))
	b.WriteString("\n")
	if m.presetSaveMode {
		b.WriteString(dimStyle.Render("Enter name · Enter save · Esc cancel"))
		b.WriteString("\n\n")
		b.WriteString(searchStyle.Render("Name: ") + m.presetNameInput.View())
		return b.String()
	}
	b.WriteString(dimStyle.Render("↑↓ select · Enter load · s save current · d delete · Esc close"))
	b.WriteString("\n\n")
	if len(m.presetList) == 0 {
		b.WriteString(dimStyle.Render("(no presets — press s to save current advanced filters)"))
		return b.String()
	}
	for i, p := range m.presetList {
		line := fmt.Sprintf("%2d  %s", i+1, p.Name)
		if i == m.presetCursor {
			b.WriteString(matchCursorStyle.Render(line))
		} else {
			b.WriteString(line)
		}
		b.WriteString("\n")
	}
	return b.String()
}

// --- helpers --------------------------------------------------------------

func splitCSV(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func splitCSVUpper(s string) []string {
	parts := splitCSV(s)
	for i := range parts {
		parts[i] = strings.ToUpper(parts[i])
	}
	return parts
}

func boolYN(b bool) string {
	if b {
		return "y"
	}
	return "n"
}

func parseYN(s string, def bool) bool {
	s = strings.ToLower(strings.TrimSpace(s))
	switch s {
	case "y", "yes", "1", "true", "on":
		return true
	case "n", "no", "0", "false", "off":
		return false
	case "":
		return def
	default:
		return def
	}
}

func parseOptionalInt64(s string) (int64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, nil
	}
	return strconv.ParseInt(s, 10, 64)
}

func parseOptionalInt(s string) (int, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, nil
	}
	return strconv.Atoi(s)
}

func truncate(s string, n int) string {
	if n <= 0 || len(s) <= n {
		return s
	}
	if n <= 1 {
		return s[:n]
	}
	return s[:n-1] + "…"
}

// Ensure lipgloss is referenced when styles are used from this file.
var _ = lipgloss.NewStyle
