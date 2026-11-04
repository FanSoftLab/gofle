package tui

import (
	"GoFLE/pkg/config"
	"GoFLE/pkg/indexer"
	"GoFLE/pkg/search"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbletea"
)

var ansiRE = regexp.MustCompile(`\x1b\[[0-9;]*m`)

// runSearchToCompletion presses Enter to start a search, executes the async
// command returned by the key handler, and applies the searchDoneMsg result.
// Use this instead of checking m.matches immediately after Enter, since the
// search runs off the UI thread via startSearchCmd().
//
// The key handler wraps startSearchCmd() in an outer tea.Batch together with a
// status-clear ticker (clearCmd) that blocks for 2.5s. startSearchCmd() itself
// returns tea.Batch(searchCmd, tickCmd). So cmd() yields a nested BatchMsg:
// BatchMsg([ func() BatchMsg([searchCmd, tickCmd]), func() statusClearMsg ]).
// We recurse into nested BatchMsgs and return early on the first searchDoneMsg,
// which avoids ever calling clearCmd (and its 2.5s block).
func runSearchToCompletion(t *testing.T, m model) model {
	t.Helper()
	nm, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = nm.(model)
	if cmd == nil {
		return m
	}
	msg := cmd()
	if msg == nil {
		return m
	}

	var apply func(msg tea.Msg) bool
	apply = func(msg tea.Msg) bool {
		if bm, ok := msg.(tea.BatchMsg); ok {
			for _, c := range bm {
				if apply(c()) {
					return true
				}
			}
			return false
		}
		if sdm, ok := msg.(searchDoneMsg); ok {
			nm, _ := m.Update(sdm)
			m = nm.(model)
			return true
		}
		return false
	}

	apply(msg)
	return m
}

// runeAtCol returns the rune whose display cell starts at column col (0-based) in s.
func runeAtCol(s string, col int) rune {
	c := 0
	for _, r := range s {
		if c == col {
			return r
		}

		c += runeWidth(r)
	}

	return 0
}

func TestSearchOptionsFromQuery(t *testing.T) {
	cases := []struct {
		raw    string
		query  string
		regex  bool
		insens bool
		invert bool
	}{
		{"error", "error", false, false, false},
		{"/err.+", "err.+", true, false, false},
		{"!warn", "warn", false, false, true},
		{"i:warn", "warn", false, true, false},
		{"!/error", "error", true, false, true},
		{"!i:warn", "warn", false, true, true},
		{"!", "", false, false, true},
	}
	for _, c := range cases {
		o := searchOptionsFromQuery(c.raw)
		if o.Query != c.query || o.IsRegex != c.regex || o.CaseSensitive == c.insens || o.InvertMatch != c.invert {
			t.Errorf("searchOptionsFromQuery(%q) = %+v, want query=%q regex=%v insens=%v invert=%v",
				c.raw, o, c.query, c.regex, c.insens, c.invert)
		}
	}
}

// newTestModel builds a model over a temp log file for smoke testing.
func newTestModel(t *testing.T, lines []string) model {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "test.log")

	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	for _, l := range lines {
		if _, err := f.WriteString(l + "\n"); err != nil {
			t.Fatalf("write: %v", err)
		}
	}

	f.Close()

	idx, err := indexer.NewFileIndexer(path)
	if err != nil {
		t.Fatalf("indexer: %v", err)
	}

	se := search.NewSearchEngine(idx)
	si := textinput.New()
	gi := textinput.New()
	ei := textinput.New()

	return model{
		idx:         idx,
		se:          se,
		height:      40,
		width:       100,
		searchInput: si,
		gotoInput:   gi,
		exportInput: ei,
		// Mirror the production default: case-insensitive search.
		searchOpts: search.SearchOptions{CaseSensitive: false},
		ctx:        context.Background(),
	}
}

func TestModelScroll(t *testing.T) {
	lines := make([]string, 100)
	for i := range lines {
		lines[i] = fmt.Sprintf("line-%d", i)
	}

	m := newTestModel(t, lines)
	if m.idx.TotalLines() != 100 {
		t.Fatalf("total lines = %d, want 100", m.idx.TotalLines())
	}

	// down moves top by 1
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})

	m = nm.(model)
	if m.top != 1 {
		t.Errorf("after down top = %d, want 1", m.top)
	}

	// end jumps to last line
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnd})

	m = nm.(model)
	if m.top != 60 {
		t.Errorf("after end top = %d, want 60", m.top)
	}

	// home jumps to 0
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyHome})

	m = nm.(model)
	if m.top != 0 {
		t.Errorf("after home top = %d, want 0", m.top)
	}
}

func TestModelSearchAndMatchNav(t *testing.T) {
	m := newTestModel(t, []string{"alpha", "beta", "alpha", "gamma", "alpha"})

	// Enter search mode with '/'
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})

	m = nm.(model)
	if !m.searchMode {
		t.Fatal("searchMode not enabled after '/'")
	}

	// Type "alpha"
	for _, r := range "alpha" {
		nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = nm.(model)
	}

	// Enter runs the search.
	m = runSearchToCompletion(t, m)

	if m.searchMode {
		t.Fatal("searchMode still enabled after enter")
	}

	if len(m.matches) != 3 {
		t.Fatalf("matches = %d, want 3", len(m.matches))
	}

	if m.top != m.matches[0] {
		t.Errorf("top = %d, want first match %d", m.top, m.matches[0])
	}

	// 'n' advances to the next match.
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})

	m = nm.(model)
	if m.matchCursor != 1 {
		t.Errorf("matchCursor = %d, want 1", m.matchCursor)
	}

	if m.top != m.matches[1] {
		t.Errorf("top = %d, want match 1 (%d)", m.top, m.matches[1])
	}
}

func TestModelInitNoPanic(t *testing.T) {
	m := newTestModel(t, []string{"x", "y"})
	if cmd := m.Init(); cmd == nil {
		t.Fatal("Init returned nil command")
	}
}

func TestModelSplitView(t *testing.T) {
	m := newTestModel(t, []string{"alpha", "beta", "alpha", "gamma", "alpha", "delta", "alpha"})

	// Run a search so there are results to show in the split.
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})

	m = nm.(model)
	for _, r := range "alpha" {
		nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = nm.(model)
	}

	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	m = runSearchToCompletion(t, m)
	if len(m.matches) != 4 {
		t.Fatalf("matches = %d, want 4", len(m.matches))
	}

	// 's' enables split view (requires results).
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})

	m = nm.(model)
	if !m.splitView {
		t.Fatal("split view not enabled after 's'")
	}
	// Rendering the split must not panic and should include the divider.
	if out := m.View(); !strings.Contains(out, "│") {
		t.Errorf("split view output missing divider: %q", out)
	}

	// Tab moves focus to the results pane.
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})

	m = nm.(model)
	if m.splitFocus != 1 {
		t.Errorf("splitFocus = %d, want 1 after Tab", m.splitFocus)
	}

	// Down in the results pane advances the match and the log follows (keeping it centered).
	first := m.matchCursor
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})

	m = nm.(model)
	if m.matchCursor != first+1 {
		t.Errorf("matchCursor = %d, want %d", m.matchCursor, first+1)
	}
	// With centered keepMatchVisible, the log pane should keep the match
	// visible with context (not jump to top). Match at line 2 is already
	// visible in viewport 0..39, so top stays 0.
	if m.top != 0 {
		t.Errorf("top = %d, want 0 (match centered, not jumped to top)", m.top)
	}

	// 's' again disables split view.
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})

	m = nm.(model)
	if m.splitView {
		t.Error("split view still enabled after second 's'")
	}
}

func TestModelSplitViewNeedsSearch(t *testing.T) {
	m := newTestModel(t, []string{"alpha", "beta"})
	// No search performed: 's' should refuse and report it.
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})

	m = nm.(model)
	if m.splitView {
		t.Error("split view enabled without a search")
	}

	if !strings.Contains(m.status, "search") {
		t.Errorf("status = %q, want hint about running a search", m.status)
	}
}

func TestKeepMatchVisibleScrollsOnlyWhenNeeded(t *testing.T) {
	m := newTestModel(t, []string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j"})
	m.height = 5
	// Build a fake match list and put the viewport such that match 1 is visible.
	m.matches = []int64{0, 2, 4, 6, 8}
	m.matchCursor = 1 // line 2
	m.top = 0         // viewport covers lines 0..4, so line 2 is visible
	m.keepMatchVisible()

	if m.top != 0 {
		t.Errorf("viewport moved while match already centered: top=%d", m.top)
	}

	// Move the cursor to a match below the viewport: it should scroll to
	// center the match (half a page above). maxTop = 10-5=5, so top=5.
	m.matchCursor = 4 // line 8, viewport 0..4 -> off screen
	m.keepMatchVisible()

	want := int64(5) // maxTop = total(10) - height(5) = 5
	if m.top != want {
		t.Errorf("top = %d, want %d to center match", m.top, want)
	}

	// A match above the viewport scrolls it to the match line.
	m.matchCursor = 0 // line 0, viewport now 4..8 -> off screen
	m.keepMatchVisible()

	if m.top != 0 {
		t.Errorf("top = %d, want 0 to center match", m.top)
	}
}

func TestPadRuneTruncatesOverlong(t *testing.T) {
	if got := padRuneDisplay("abcdef", 3); got != "abc" {
		t.Errorf("padRuneDisplay overlong = %q, want %q", got, "abc")
	}

	if got := padRuneDisplay("ab", 4); got != "ab  " {
		t.Errorf("padRuneDisplay short = %q, want %q", got, "ab  ")
	}
}

func TestDisplayWidth(t *testing.T) {
	// ASCII = 1 cell
	if displayWidth("abc") != 3 {
		t.Errorf("displayWidth('abc') = %d, want 3", displayWidth("abc"))
	}
	// Cyrillic = 1 cell
	if displayWidth("Христинівка") != 11 {
		t.Errorf("displayWidth('Христинівка') = %d, want 11", displayWidth("Христинівка"))
	}
	// CJK = 2 cells
	if displayWidth("中文") != 4 {
		t.Errorf("displayWidth('中文') = %d, want 4", displayWidth("中文"))
	}
	// Fullwidth = 2 cells
	if displayWidth("ＡＢＣ") != 6 {
		t.Errorf("displayWidth('ＡＢＣ') = %d, want 6", displayWidth("ＡＢＣ"))
	}
	// Mixed
	if displayWidth("a中b") != 4 {
		t.Errorf("displayWidth('a中b') = %d, want 4", displayWidth("a中b"))
	}
}

func TestModelSelectionAndCopy(t *testing.T) {
	lines := make([]string, 100)
	for i := range lines {
		lines[i] = fmt.Sprintf("line-%d", i)
	}

	m := newTestModel(t, lines)

	// 'v' starts selection at the current top (0).
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'v'}})

	m = nm.(model)
	if !m.selActive {
		t.Fatal("selection not active after 'v'")
	}

	if m.selAnchor != 0 {
		t.Errorf("selAnchor = %d, want 0", m.selAnchor)
	}

	// Scroll down twice: selection should span 0..2.
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = nm.(model)
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = nm.(model)

	lo, hi, ok := m.selectionBounds()
	if !ok || lo != 0 || hi != 2 {
		t.Fatalf("selection bounds = (%d,%d,%v), want (0,2,true)", lo, hi, ok)
	}

	// 'c' copies the range and clears the selection.
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})

	m = nm.(model)
	if m.selActive {
		t.Error("selection still active after copy")
	}

	if !strings.Contains(m.status, "Copied") {
		t.Errorf("status = %q, want it to contain 'Copied'", m.status)
	}

	// 'c' with no active selection reports a hint instead of panicking.
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})

	m = nm.(model)
	if !strings.Contains(m.status, "no selection") {
		t.Errorf("status = %q, want it to contain 'no selection'", m.status)
	}

	// 'esc' clears an active selection without quitting.
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'v'}})
	m = nm.(model)
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})

	m = nm.(model)
	if m.selActive {
		t.Error("selection not cleared by esc")
	}
}

func TestModelKeys(t *testing.T) {
	m := newTestModel(t, []string{"a", "b", "c"})

	// 'f' toggles follow/tail on.
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'f'}})

	m = nm.(model)
	if !m.tailOn {
		t.Error("tailOn not enabled after 'f'")
	}

	// 'g' enters goto mode.
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}})

	m = nm.(model)
	if !m.gotoMode {
		t.Error("gotoMode not enabled after 'g'")
	}
	// esc leaves goto mode so 'q' is interpreted as quit.
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})

	m = nm.(model)
	if m.gotoMode {
		t.Error("gotoMode still set after esc")
	}

	// 'q' requests quit.
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	if cmd == nil {
		t.Fatal("'q' returned nil command")
	}

	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Error("'q' did not return tea.Quit")
	}
}

func searchAlphaModel(t *testing.T) model {
	t.Helper()
	m := newTestModel(t, []string{"alpha here", "beta", "alpha there", "gamma", "delta"}) // "alpha" (case-sensitive) -> matches at indices 0 and 2
	// enter search mode
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})

	m = nm.(model)
	for _, r := range "alpha" {
		nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = nm.(model)
	}

	m = runSearchToCompletion(t, m)

	return m
}

func TestModelMatchNavFirstLast(t *testing.T) {
	m := searchAlphaModel(t) // 2 matches: "Alpha"(0), "alpha"(1)

	// '[' jumps to the first match.
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'['}})

	m = nm.(model)
	if m.matchCursor != 0 {
		t.Errorf("after '[' matchCursor = %d, want 0", m.matchCursor)
	}

	// ']' jumps to the last match.
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{']'}})

	m = nm.(model)
	if m.matchCursor != len(m.matches)-1 {
		t.Errorf("after ']' matchCursor = %d, want %d", m.matchCursor, len(m.matches)-1)
	}
}

func TestModelMatchNavPrev(t *testing.T) {
	m := searchAlphaModel(t)

	// 'n' then 'p' returns to the previous match.
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})

	m = nm.(model)
	if m.matchCursor != 1 {
		t.Fatalf("after 'n' matchCursor = %d, want 1", m.matchCursor)
	}

	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})

	m = nm.(model)
	if m.matchCursor != 0 {
		t.Errorf("after 'p' matchCursor = %d, want 0", m.matchCursor)
	}
}

func TestModelSearchFoundCounter(t *testing.T) {
	m := searchAlphaModel(t)
	if len(m.matches) != 2 {
		t.Fatalf("matches = %d, want 2", len(m.matches))
	}

	if !strings.Contains(m.status, "found 2 matches") {
		t.Errorf("status = %q, want it to contain 'found 2 matches'", m.status)
	}
}

func TestModelSearchOptionToggles(t *testing.T) {
	// Alt+C toggles case-sensitivity ON (the default is case-insensitive);
	// "alpha" case-sensitive then matches only the lowercase "alpha".
	m := newTestModel(t, []string{"Alpha", "alpha", "BETA", "beta"})
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	m = nm.(model)
	nm, _ = m.Update(tea.KeyMsg{Alt: true, Type: tea.KeyRunes, Runes: []rune{'c'}})

	m = nm.(model)
	if !m.searchOpts.CaseSensitive {
		t.Fatal("Alt+C did not toggle CaseSensitive on")
	}

	m = typeQueryAndRun(t, m, "alpha")
	assertMatchLines(t, m, []int64{1})

	// Alt+R makes the search a regex; combining with Alt+C (case-sensitive)
	// makes "Al" match only "Alpha".
	m = newTestModel(t, []string{"Alpha", "alpha", "BETA"})
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	m = nm.(model)
	nm, _ = m.Update(tea.KeyMsg{Alt: true, Type: tea.KeyRunes, Runes: []rune{'r'}})

	m = nm.(model)
	if !m.searchOpts.IsRegex {
		t.Fatal("Alt+R did not toggle IsRegex on")
	}

	nm, _ = m.Update(tea.KeyMsg{Alt: true, Type: tea.KeyRunes, Runes: []rune{'c'}})

	m = nm.(model)
	if !m.searchOpts.CaseSensitive {
		t.Fatal("Alt+C did not toggle CaseSensitive on")
	}

	m = typeQueryAndRun(t, m, "Al")
	assertMatchLines(t, m, []int64{0})

	// Alt+I inverts the match; "z" (case-sensitive) -> only "alpha" lacks 'z'.
	m = newTestModel(t, []string{"zoo", "zebra", "alpha"})
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	m = nm.(model)
	nm, _ = m.Update(tea.KeyMsg{Alt: true, Type: tea.KeyRunes, Runes: []rune{'i'}})

	m = nm.(model)
	if !m.searchOpts.InvertMatch {
		t.Fatal("Alt+I did not toggle InvertMatch on")
	}

	m = typeQueryAndRun(t, m, "z")
	assertMatchLines(t, m, []int64{2})
}

// typeQueryAndRun types query into the focused search box one key at a time and
// runs it with enter, returning the resulting model with search results applied.
func typeQueryAndRun(t *testing.T, m model, query string) model {
	t.Helper()

	for _, r := range query {
		nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = nm.(model)
	}

	m = runSearchToCompletion(t, m)

	return m
}

// assertMatchLines fails unless the model's match set is exactly want.
func assertMatchLines(t *testing.T, m model, want []int64) {
	t.Helper()

	if !slices.Equal(m.matches, want) {
		t.Fatalf("matches = %v, want %v", m.matches, want)
	}
}

func TestModelOnlyMatchesFilterView(t *testing.T) {
	m := searchAlphaModel(t) // matches indices 0,1 (Alpha, alpha)
	if len(m.matches) != 2 {
		t.Fatalf("precondition: matches = %d, want 2", len(m.matches))
	}

	// Toggle results-only view on.
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})

	m = nm.(model)
	if !m.onlyMatches {
		t.Fatal("onlyMatches not enabled after 'r'")
	}

	// The filter view renders exactly len(matches) body lines.
	v := m.View()
	lines := strings.Split(v, "\n")
	statusLineIdx := -1

	for i, l := range lines {
		// Status bar uses "lines N" in full mode, "results" in compact mode.
		if strings.Contains(l, "lines ") || strings.Contains(l, "results") {
			statusLineIdx = i
			break
		}
	}

	if statusLineIdx < 0 {
		t.Fatalf("status line not found in view:\n%s", v)
	}

	n := 0

	for i := range statusLineIdx {
		line := strings.TrimSpace(lines[i])
		if line == "" || strings.Contains(line, "AND in results:") {
			continue // permanent AND-filter chrome is not a body row
		}

		n++
	}

	if n != len(m.matches) {
		t.Errorf("filter view rendered %d body lines, want %d", n, len(m.matches))
	}

	// Navigation goes by match index: 'n' moves to match index 1.
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})

	m = nm.(model)
	if m.top != int64(m.matchCursor) {
		t.Errorf("in onlyMatches mode top = %d, want matchCursor %d", m.top, m.matchCursor)
	}
}

func TestModelOnlyMatchesNoSearch(t *testing.T) {
	// With no active search, onlyMatches must degrade to the full log and show
	// a "no matches" status instead of an empty viewer.
	m := newTestModel(t, []string{"a", "b", "c"})
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})

	m = nm.(model)
	if !m.onlyMatches {
		t.Fatal("onlyMatches not enabled after 'r'")
	}

	if !strings.Contains(m.View(), "no matches") {
		t.Errorf("expected 'no matches' status, got:\n%s", m.View())
	}
}

func TestModelHelpOverlay(t *testing.T) {
	m := newTestModel(t, []string{"a", "b"})

	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})

	m = nm.(model)
	if !m.help {
		t.Fatal("help overlay not opened by '?'")
	}

	if !strings.Contains(m.View(), "help") {
		t.Error("help overlay view did not contain 'help'")
	}

	// '?' again closes the overlay.
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})

	m = nm.(model)
	if m.help {
		t.Error("help overlay not closed by second '?'")
	}
}

func TestModelExportToFile(t *testing.T) {
	m := newTestModel(t, []string{"a", "b", "c"})
	m.matches = []int64{0, 2}

	p := filepath.Join(t.TempDir(), "out.log")
	m.doExport(p)

	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("read exported file: %v", err)
	}

	s := string(data)
	if !strings.Contains(s, "1:a") || !strings.Contains(s, "3:c") {
		t.Errorf("exported content = %q, want lines 1:a and 3:c", s)
	}
}

func TestModelDefaultFilterNoSearch(t *testing.T) {
	// The production default enables the results-only view; with no active
	// search it degrades to the full log and signals that via the status.
	m := newTestModel(t, []string{"a", "b", "c"})

	m.onlyMatches = true
	if !strings.Contains(m.View(), "no matches") {
		t.Errorf("expected 'no matches' when filter view is on with no matches:\n%s", m.View())
	}
}

func TestModelRecentPickerOpens(t *testing.T) {
	m := newTestModel(t, []string{"a", "b"})
	dir := t.TempDir()

	p := filepath.Join(dir, "recent.log")
	if err := os.WriteFile(p, []byte("x\ny\nz\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	m.recentList = []config.RecentFileItem{{Path: p, IsSSH: false}}
	m.recentCursor = 0
	m.recentMode = true

	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	m = nm.(model)
	if m.recentMode {
		t.Fatal("recent mode should close after opening an entry")
	}

	if m.idx.TotalLines() != 3 {
		t.Errorf("reopened file has %d lines, want 3", m.idx.TotalLines())
	}

	if !strings.Contains(m.status, "opened") {
		t.Errorf("status = %q, want it to contain 'opened'", m.status)
	}
}

func TestModelRecentPickerNav(t *testing.T) {
	m := newTestModel(t, []string{"a", "b"})
	m.recentList = []config.RecentFileItem{
		{Path: "/x"}, {Path: "/y"}, {Path: "/z"},
	}
	m.recentCursor = 0
	m.recentMode = true

	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})

	m = nm.(model)
	if m.recentCursor != 1 {
		t.Errorf("after 'j' recentCursor = %d, want 1", m.recentCursor)
	}

	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})

	m = nm.(model)
	if m.recentCursor != 0 {
		t.Errorf("after 'k' recentCursor = %d, want 0", m.recentCursor)
	}

	// 'q' is ignored in recent mode (only esc/enter/up/down act).
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})

	m = nm.(model)
	if !m.recentMode {
		t.Error("'q' should not close recent mode")
	}

	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})

	m = nm.(model)
	if m.recentMode {
		t.Error("esc should close recent mode")
	}
}

func TestModelRecentViewShowsDetails(t *testing.T) {
	m := newTestModel(t, []string{"a", "b"})
	m.recentList = []config.RecentFileItem{
		{Path: "/var/log/app.log", FileSize: 2048, LastOpened: "2026-08-21 10:00:00", IsSSH: false},
		{
			Path: "", FileSize: 1024, LastOpened: "2026-08-20 09:30:00", IsSSH: true,
			SSH: &config.SSHConfigSummary{Username: "bob", Host: "host", Port: 22, RemoteFilePath: "/var/log/x.log"},
		},
	}
	m.recentCursor = 0
	m.recentMode = true

	v := m.View()
	if !strings.Contains(v, "last:") {
		t.Errorf("recent view should show last-opened time:\n%s", v)
	}

	if !strings.Contains(v, "[local]") || !strings.Contains(v, "[ssh]") {
		t.Errorf("recent view should show the local/ssh kind:\n%s", v)
	}

	if !strings.Contains(v, "2.0 KB") {
		t.Errorf("recent view should show a human-readable size:\n%s", v)
	}
	// SSH entries render WITHOUT the username — credentials must never leak
	// into the picker listing.
	if !strings.Contains(v, "host:22/var/log/x.log") {
		t.Errorf("recent view should show the ssh target without userinfo:\n%s", v)
	}

	if strings.Contains(v, "bob@") {
		t.Errorf("recent view must not display the username:\n%s", v)
	}
}

func TestRecentPickerModelNav(t *testing.T) {
	m := recentPickerModel{
		items: []config.RecentFileItem{
			{Path: "/x"}, {Path: "/y"}, {Path: "/z"},
		},
		cursor: 0,
	}

	down := func(mm recentPickerModel) recentPickerModel {
		nm, _ := mm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
		return nm.(recentPickerModel)
	}
	up := func(mm recentPickerModel) recentPickerModel {
		nm, _ := mm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
		return nm.(recentPickerModel)
	}

	m = down(m)
	if m.cursor != 1 {
		t.Errorf("after down cursor = %d, want 1", m.cursor)
	}

	m = down(m)
	if m.cursor != 2 {
		t.Errorf("after down cursor = %d, want 2", m.cursor)
	}

	m = up(m)
	if m.cursor != 1 {
		t.Errorf("after up cursor = %d, want 1", m.cursor)
	}

	// enter selects (not canceled)
	entered, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	em := entered.(recentPickerModel)
	if em.canceled {
		t.Error("enter should select, not cancel")
	}

	// esc cancels
	esc, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if !esc.(recentPickerModel).canceled {
		t.Error("esc should cancel")
	}
}

// TestModelSplitViewRendersAllRows guards against a regression where the
// per-row safety truncation checked the ENTIRE accumulated buffer and reset it
// on every iteration after the first, collapsing the split body to ~1 row.
func TestModelSplitViewRendersAllRows(t *testing.T) {
	m := newTestModel(t, []string{"alpha", "beta", "alpha", "gamma", "alpha", "delta", "alpha"})
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})

	m = nm.(model)
	for _, r := range "alpha" {
		nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = nm.(model)
	}

	m = runSearchToCompletion(t, m)
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})

	m = nm.(model)
	if !m.splitView {
		t.Fatal("split view not enabled")
	}
	// Count non-empty body rows in the rendered split view. The log pane must
	// show every source line (here 7), not just the first one or two.
	out := m.View()
	rows := strings.Split(out, "\n")
	dividerRows := 0

	for _, r := range rows {
		if strings.Contains(r, "│") {
			dividerRows++
		}
	}

	if dividerRows < 7 {
		t.Errorf("split body rendered %d divider rows, want >= 7 (one per log line); got:\n%s", dividerRows, out)
	}
}

// TestModelSplitViewSyncScroll verifies that in split view the right (results)
// pane follows the log pane as it is scrolled, and vice versa (navigating the
// results pane centers the log pane on the selected match). This guards against
// the "line shifts" where the two panes drift to unrelated regions.
func TestModelSplitViewSyncScroll(t *testing.T) {
	lines := make([]string, 100)
	for i := range lines {
		if i%10 == 0 {
			lines[i] = fmt.Sprintf("line-%d MATCH", i)
		} else {
			lines[i] = fmt.Sprintf("line-%d filler", i)
		}
	}

	m := newTestModel(t, lines)

	// Search for MATCH -> matches at lines 0,10,...,90.
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})

	m = nm.(model)
	for _, r := range "MATCH" {
		nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = nm.(model)
	}

	m = runSearchToCompletion(t, m)
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})

	m = nm.(model)
	if !m.splitView {
		t.Fatal("split view not enabled")
	}

	// Scroll the log pane (splitFocus==0) down to around line 25: the results
	// pane must point at the first match at/after line 25 (line 30, index 3),
	// not drift away.
	for range 25 {
		nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
		m = nm.(model)
	}

	if m.top != 25 {
		t.Fatalf("log top = %d, want 25", m.top)
	}

	if got := m.matches[m.matchCursor]; got != 30 {
		t.Errorf("matchCursor line = %d, want 30 (first match at/after log line 25)", got)
	}

	// Navigate the results pane: the log pane must center on the selected match
	// (line 40 here), keeping the panes in step.
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})

	m = nm.(model)
	if m.splitFocus != 1 {
		t.Fatalf("splitFocus = %d, want 1", m.splitFocus)
	}

	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = nm.(model)

	wantLine := m.matches[m.matchCursor]
	if wantLine != 40 {
		t.Fatalf("after results down, selected match line = %d, want 40", wantLine)
	}

	if m.top > wantLine || m.top+int64(m.height) <= wantLine {
		t.Errorf("log pane (top=%d, height=%d) does not contain selected match line %d", m.top, m.height, wantLine)
	}
}

// TestHangulRuneWidth guards against East-Asian wide characters (modern Korean
// Hangul, U+AC00–U+D7A3) being counted as width 1. Counting them as 1 made wide
// left-pane lines overflow past the split divider into the right pane.
func TestHangulRuneWidth(t *testing.T) {
	for _, s := range []string{"시", "치", "우", "카", "한", "글", "시치우카"} {
		if got := displayWidth(s); got != 2*len([]rune(s)) {
			t.Errorf("displayWidth(%q) = %d, want %d (2 per rune)", s, got, 2*len([]rune(s)))
		}
	}
}

// TestSplitNoBleed proves a left-pane line can never spill into the right pane:
// every body row's visible width fits the terminal (no wrap), and the split
// divider ('│') sits at the same display column on every row.
func TestSplitNoBleed(t *testing.T) {
	lines := []string{
		`    "name:ko": "시치우카 시치우카 시치우카",`,
		`    "name": "звичайний текст",`,
		`    "long": "це довгий рядок з корейською всередині щоб перевірити перенесення до правого вікна",`,
		`    "wiki": "uk:Іванівка (Христинівська міська громада)",`,
	}
	m := newTestModel(t, lines)
	m.width = 100
	m.height = 6
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})

	m = nm.(model)
	for _, r := range "name" {
		nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = nm.(model)
	}

	m = runSearchToCompletion(t, m)
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	m = nm.(model)

	// buildLayout: inner = max(termW - gapW - 2, 2); leftW = inner / 2;
	// divider sits at display column leftW + 1 (after left pane + one space).
	leftW := (m.width - 5) / 2 // gapW=3, minus 2 margins
	divCol := leftW + 1
	out := m.View()
	bodyRows := 0

	for row := range strings.SplitSeq(out, "\n") {
		clean := ansiRE.ReplaceAllString(row, "")
		if !strings.Contains(clean, "│") {
			continue
		}

		bodyRows++

		if dw := displayWidth(clean); dw > m.width {
			t.Errorf("row visible width %d exceeds terminal %d (bleed): %q", dw, m.width, clean)
		}

		if r := runeAtCol(clean, divCol); r != '│' {
			t.Errorf("row has %q (not split divider '│') at display col %d: %q", string(r), divCol, clean)
		}
	}

	if bodyRows == 0 {
		t.Fatal("no split rows rendered")
	}
}

// wrapJumpModel builds a wrap-mode jump fixture: long lines (every 5th) wrap
// into several visual rows at the test width, the rest are short. Matches sit
// on both long and short lines, which is the worst case for jump positioning:
// a half-page of logical lines above a match can span far more than half a
// viewport of visual rows.
func wrapJumpModel(t *testing.T) model {
	t.Helper()

	lines := make([]string, 120)
	for i := range lines {
		if i%5 == 0 {
			lines[i] = fmt.Sprintf("LONG-%03d %s", i, strings.Repeat("payload-", 40))
		} else {
			lines[i] = fmt.Sprintf("short-%03d", i)
		}
	}

	m := newTestModel(t, lines)
	m.width = 100
	m.height = 10
	m.wordWrap = true

	return m
}

// wrapJumpMatches returns match line indices covering both long (ln%5==0) and
// short (ln%5==2) fixture lines.
func wrapJumpMatches(total int64) []int64 {
	var matches []int64

	for ln := range total {
		if ln%5 == 0 || ln%5 == 2 {
			matches = append(matches, ln)
		}
	}

	return matches
}

// wrapRowsAbove counts the visual rows the viewport spends above line mc when
// the viewport starts at (top, topWrap) in wrap mode.
func wrapRowsAbove(m model, top, mc int64, topWrap int) int {
	above := 0
	for ln := top; ln < mc; ln++ {
		above += m.wrapRows(ln)
	}

	return above - topWrap
}

// wrapRowsBelow counts visual rows from the start of line ln to EOF, capped at
// the given row budget.
func wrapRowsBelow(m model, ln, budget int64) int {
	b := int(budget)
	rows := 0

	for ; ln < m.idx.TotalLines() && rows < b; ln++ {
		rows += m.wrapRows(ln)
	}

	return rows
}

// TestKeepMatchVisibleWrapCentersMatchOnScreen pins the wrap-mode jump: after
// keepMatchVisible the selected match must be centered in visual rows when
// both a half-page above and a half-page below exist — and never positioned by
// raw logical-line count, which leaves the match below the viewport when the
// lines in between wrap.
func TestKeepMatchVisibleWrapCentersMatchOnScreen(t *testing.T) {
	m := wrapJumpModel(t)
	if m.wrapRows(0) < 3 {
		t.Fatalf("fixture: wrapRows(0) = %d, want >= 3 wrapped rows", m.wrapRows(0))
	}

	m.matches = wrapJumpMatches(m.idx.TotalLines())

	need := m.height / 2
	for cursor := range m.matches {
		m.matchCursor = cursor
		m.keepMatchVisible()

		mc := m.matches[cursor]
		above := wrapRowsAbove(m, m.top, mc, m.topWrap)

		// The core invariant: the match's first visual row is on screen.
		if above < 0 || above >= m.height {
			t.Errorf("cursor %d: match first row at %d of a %d-row viewport — off screen (top=%d topWrap=%d)",
				cursor, above, m.height, m.top, m.topWrap)
		}

		if m.topWrap < 0 || m.topWrap >= m.wrapRows(m.top) {
			t.Errorf("cursor %d: topWrap %d outside 0..%d for line %d",
				cursor, m.topWrap, m.wrapRows(m.top)-1, m.top)
		}

		// Exact centering is only required when the file has room for half a
		// page of visual rows above AND below the match; near either end the
		// viewport pins to the boundary instead.
		rowsAbove := wrapRowsAbove(m, 0, mc, 0)
		rowsBelow := wrapRowsBelow(m, mc, int64(m.height-need))

		if rowsAbove >= need && rowsBelow >= m.height-need && above != need {
			t.Errorf("cursor %d: visual rows above match = %d, want exact center %d (top=%d topWrap=%d)",
				cursor, above, need, m.top, m.topWrap)
		}

		body := m.bodyView()
		if !strings.Contains(body, fmt.Sprintf("LONG-%03d", mc)) && !strings.Contains(body, fmt.Sprintf("short-%03d", mc)) {
			t.Errorf("cursor %d: rendered body does not show the matched line %d (top=%d topWrap=%d)",
				cursor, mc, m.top, m.topWrap)
		}
	}
}

// TestKeepMatchVisibleWrapFillsViewportNearEOF checks the wrap-mode jump near
// the end of the file: clampViewport must walk the top back far enough that
// the viewport stays filled instead of pinning the match to an empty screen.
func TestKeepMatchVisibleWrapFillsViewportNearEOF(t *testing.T) {
	m := wrapJumpModel(t)
	m.matches = []int64{m.idx.TotalLines() - 1}
	m.matchCursor = 0
	m.keepMatchVisible()

	if got := m.remainingVisualRows(); got < m.height {
		t.Errorf("remainingVisualRows = %d, want >= %d (top=%d topWrap=%d)",
			got, m.height, m.top, m.topWrap)
	}
}

// TestResultsEnterResetsTopWrap pins the enter-from-results jump: the stale
// wrapped-row offset left by earlier wrap-mode scrolling must be dropped, or
// the first visual rows of the target line are cut off.
func TestResultsEnterResetsTopWrap(t *testing.T) {
	m := wrapJumpModel(t)
	m.matches = []int64{10, 50}
	m.splitView = true
	m.splitFocus = 1
	m.matchCursor = 1

	// Leave the log pane mid-line: the top line's last visual row is showing.
	m.top = 5
	m.topWrap = m.wrapRows(5) - 1

	if m.topWrap < 1 {
		t.Fatalf("fixture: topWrap = %d, want >= 1", m.topWrap)
	}

	if !m.resultsPaneKey(keyEnter) {
		t.Fatal("resultsPaneKey did not handle enter")
	}

	if m.splitFocus != 0 {
		t.Errorf("splitFocus = %d, want 0 (back to log pane)", m.splitFocus)
	}

	if m.top != 50 {
		t.Errorf("top = %d, want 50 (the match line)", m.top)
	}

	if m.topWrap != 0 {
		t.Errorf("topWrap = %d, want 0 after enter", m.topWrap)
	}

	if !strings.Contains(m.bodyView(), "LONG-050") {
		t.Error("rendered body does not start at the jumped-to match")
	}
}

// TestRunGotoResetsTopWrap covers the goto-line prompt under wrap: a stale
// topWrap must not chop the target line.
func TestRunGotoResetsTopWrap(t *testing.T) {
	m := wrapJumpModel(t)
	m.top = 5
	m.topWrap = m.wrapRows(5) - 1
	m.gotoInput.SetValue("51")

	m.runGoto()

	if m.top != 50 {
		t.Errorf("top = %d, want 50 (1-based line 51)", m.top)
	}

	if m.topWrap != 0 {
		t.Errorf("topWrap = %d, want 0 after goto", m.topWrap)
	}

	if !strings.Contains(m.bodyView(), "LONG-050") {
		t.Error("rendered body does not start at the goto target")
	}
}
