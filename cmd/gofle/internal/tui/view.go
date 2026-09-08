package tui

import (
	"GoFLE/cmd/gofle/internal/shared"
	"GoFLE/pkg/config"
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
)

// isMatchLine reports whether idx is one of the (sorted) matched line indices,
// using a binary search so the lookup stays cheap even with millions of matches.
func isMatchLine(matches []int64, idx int64) bool {
	i := sort.Search(len(matches), func(i int) bool { return matches[i] >= idx })
	return i < len(matches) && matches[i] == idx
}

// bodyKey captures every field that affects the rendered visible body, so the
// body can be memoized and only recomputed when one of them changes (MED).
type bodyKey struct {
	top         int64
	topWrap     int
	height      int
	width       int
	onlyMatches bool
	splitView   bool
	selActive   bool
	selLo       int64
	selHi       int64
	matchCursor int
	resTop      int
	splitFocus  int
	totalLines  int64
	matchCount  int
	firstMatch  int64
	lastMatch   int64
}

// viewBodyCache memoizes the expensive visible-body render (MED). It is a
// single package-scoped entry because the TUI runs as one tea.Program per
// process and View() is invoked single-threaded; the lock guards it for safety
// against any concurrent callers. The body is a pure function of bodyKey, so
// caching across models with identical keys is correct.
var (
	viewBodyMu  sync.Mutex
	viewBodyKey bodyKey
	viewBody    string
)

// bodyKey computes the memo key for the current viewer state.
func (m model) bodyKey() bodyKey {
	k := bodyKey{
		top:         m.top,
		topWrap:     m.topWrap,
		height:      m.height,
		width:       m.width,
		onlyMatches: m.onlyMatches,
		splitView:   m.splitView,
		matchCursor: m.matchCursor,
		resTop:      m.resTop,
		splitFocus:  m.splitFocus,
		totalLines:  m.idx.TotalLines(),
		matchCount:  len(m.matches),
	}
	if m.selActive {
		k.selActive = true

		lo, hi, _ := m.selectionBounds()
		k.selLo = lo
		k.selHi = hi
	}

	if len(m.matches) > 0 {
		k.firstMatch = m.matches[0]
		k.lastMatch = m.matches[len(m.matches)-1]
	}

	return k
}

// spaceCache backs spaces(n): returning a shared substring avoids allocating a
// fresh padding string for every rendered gutter (MED render-alloc reduction).
var spaceCache = "                                "

// spaces returns a string of n spaces without allocating when n is within the
// cache; the cache doubles on demand.
func spaces(n int) string {
	if n <= 0 {
		return ""
	}

	for len(spaceCache) < n {
		spaceCache += spaceCache
	}

	return spaceCache[:n]
}

// View implements tea.Model: body + footer composed from a single layout plan
// so status/hint never fight the body for rows on small terminals.
func (m model) View() string {
	if overlay, ok := m.overlayView(); ok {
		return overlay
	}

	l := m.buildLayout()
	bodyStr := m.bodyView()

	out := bodyStr
	if out != "" {
		out += "\n"
	}

	if l.showPrompt {
		if prompt := m.promptView(); prompt != "" {
			out += prompt + "\n"
		}
	}

	if l.showStatus {
		out += m.renderStatus()
	}

	// Optional message rows (same count as layout.msgRows).
	if m.onlyMatches && len(m.matches) == 0 {
		out += "\n" + infoStyle.Render(tr(m, "no_matches_hint"))
	}
	if m.err != nil {
		out += "\n" + errorStyle.Render(tr(m, "error_prefix")+m.err.Error())
	}
	if m.status != "" {
		out += "\n" + infoStyle.Render(m.status)
	}

	if l.showHint {
		if m.searchMode || m.secondaryMode {
			out += "\n" + hintStyle.Render(tr(m, "search_hint"))
		} else {
			out += "\n" + hintStyle.Render(m.navHint())
		}
	}

	return out
}

// overlayView renders the modal overlay that currently owns the screen, if any.
// ok is false when the normal log view should be rendered instead.
func (m model) overlayView() (view string, ok bool) {
	switch {
	case m.help:
		return m.helpView(), true
	case m.recentMode:
		return m.recentView(), true
	case m.remoteMode:
		return m.remoteFormView(), true
	case m.remoteBrowseMode:
		return m.remoteBrowseView(), true
	}

	return "", false
}

// bodyView renders the visible body, which is the expensive part of a frame, and
// reuses the memoized copy when no field affecting it has changed (MED viewport
// memoization). The cache is package-scoped: the TUI runs as a single tea.Program
// per process and View() is invoked single-threaded (UI goroutine / sequentially
// in tests), so a single global entry is correct and avoids relying on model-field
// persistence that a value-receiver View() cannot provide.
func (m model) bodyView() string {
	k := m.bodyKey()

	viewBodyMu.Lock()

	if viewBody != "" && viewBodyKey == k {
		cached := viewBody

		viewBodyMu.Unlock()

		return cached
	}

	viewBodyMu.Unlock()

	body := &strings.Builder{}
	// Split only when the right pane has matches; otherwise a lone divider
	// over an empty half-screen looks broken (see empty-results screenshot).
	if m.effectiveSplit() {
		body.WriteString(m.renderSplitBody())
	} else {
		body.WriteString(m.renderLogLines(m.width))
	}

	bodyStr := body.String()

	viewBodyMu.Lock()

	viewBody = bodyStr
	viewBodyKey = k

	viewBodyMu.Unlock()

	return bodyStr
}

// promptView renders the prompt line (search / AND filter / goto / export /
// export confirmation) that sits between the body and the status bar. It is
// empty when no prompt is focused.
func (m model) promptView() string {
	switch {
	case m.searchMode, m.secondaryMode:
		return searchStyle.Render("Search "+m.searchFlagString()+": ") + m.searchInput.View() +
			"\n" + m.andFilterLine()
	case m.gotoMode:
		return gotoStyle.Render("Go to line: ") + m.gotoInput.View()
	case m.exportMode:
		return exportStyle.Render("Export to: ") + m.exportInput.View()
	case m.exportConfirm:
		return confirmStyle.Render("Directory does not exist. Create it? (y/N) -> " + m.exportTarget)
	}

	return ""
}

// renderStatus builds the status line from localized labels. On compact
// terminals only the essentials are shown so the bar stays on one row.
func (m model) renderStatus() string {
	val := func(s string) string { return statusValueStyle.Render(s) }
	onOff := func(b bool) string {
		if b {
			return statusOnStyle.Render(tr(m, "status_tail_on"))
		}
		return statusOffStyle.Render(tr(m, "status_tail_off"))
	}
	sep := statusSepStyle.Render(" · ")

	matchIdx := "—"
	if len(m.matches) > 0 {
		matchIdx = fmt.Sprintf("%d/%d", m.matchCursor+1, len(m.matches))
	}

	file := statusFileStyle.Render(filepath.Base(m.idx.FilePath()))
	l := m.buildLayout()

	var parts []string
	parts = append(parts, file)
	if l.compact {
		// Minimal bar: file · matches · mode
		parts = append(parts, tr(m, "status_matches")+" "+val(matchIdx))
		if m.effectiveSplit() {
			parts = append(parts, tr(m, "status_split"))
		} else if m.onlyMatches {
			parts = append(parts, tr(m, "status_results"))
		}
		return statusStyle.Render(strings.Join(parts, sep))
	}

	parts = append(parts, tr(m, "status_lines")+" "+val(strconv.FormatInt(m.idx.TotalLines(), 10)))
	parts = append(parts, tr(m, "status_top")+" "+val(strconv.FormatInt(m.top+1, 10)))
	parts = append(parts, "tail "+onOff(m.tailOn))
	parts = append(parts, tr(m, "status_filter")+" "+onOff(m.onlyMatches))
	parts = append(parts, tr(m, "status_matches")+" "+val(matchIdx))

	if m.selActive {
		lo, hi, _ := m.selectionBounds()
		parts = append(parts, "sel "+val(fmt.Sprintf("%d–%d (%d)", lo+1, hi+1, hi-lo+1)))
	}

	if m.effectiveSplit() {
		// High-contrast focus label so the active pane is unmistakable.
		if m.splitFocus == 1 {
			parts = append(parts, tr(m, "status_split")+" "+
				matchCursorStyle.Render(" RESULTS "))
		} else {
			parts = append(parts, tr(m, "status_split")+" "+
				lipgloss.NewStyle().
					Foreground(lipgloss.Color("#1e1e2e")).
					Background(lipgloss.Color("#89b4fa")).
					Render("   LOG   "))
		}
	}

	return statusStyle.Render(strings.Join(parts, sep))
}

// recentView renders the recent-files picker overlay.
// renderRecentList renders the recent-entries body (shared by the in-viewer
// picker and the standalone `gofle list` picker). The entry at `cursor` is
// highlighted.
func renderRecentList(items []config.RecentFileItem, cursor int) string {
	if len(items) == 0 {
		return dimStyle.Render("  No recent files.")
	}

	var b strings.Builder

	for i, it := range items {
		label := it.Path
		if it.IsSSH && it.SSH != nil {
			// No username/credentials in the listing — connection details stay
			// in the encrypted summary and are resolved at open time.
			label = fmt.Sprintf("%s:%d%s", it.SSH.Host, it.SSH.Port, it.SSH.RemoteFilePath)
		}

		kind := "local"
		if it.IsSSH {
			kind = "ssh"
		}

		var line string
		if i == cursor {
			line = "> " + matchStyle.Render(fmt.Sprintf("%d. %s", i+1, label))
		} else {
			line = "  " + fmt.Sprintf("%d. %s", i+1, label)
		}

		b.WriteString(line + "\n")
		// Metadata stays muted so the path remains the primary visual focus.
		b.WriteString(dimStyle.Render(fmt.Sprintf("      [%s]  last: %s  ·  size: %s",
			kind, it.LastOpened, shared.HumanSize(it.FileSize))) + "\n")
	}

	return b.String()
}

func (m model) recentView() string {
	var b strings.Builder
	b.WriteString(headerStyle.Render("Open recent") + "\n\n")
	b.WriteString(renderRecentList(m.recentList, m.recentCursor))
	b.WriteString("\n" + hintStyle.Render("↑↓/jk move  ·  enter open  ·  esc cancel"))

	return b.String()
}

// remoteFormLabels label the connect-form fields in the order of m.remoteForm.
var remoteFormLabels = []string{"Protocol", "Host", "Port", "User", "Auth", "Password", "Key path", "Remote path"}

// remoteFormView renders the protocol-agnostic remote connect form. The remote
// file path is optional — submitting with it empty opens the remote directory
// browser. The key-path field is hidden for ftp and for ssh password auth; the
// password field is hidden for ssh key auth.
func (m model) remoteFormView() string {
	var b strings.Builder
	b.WriteString(headerStyle.Render("Connect to remote") + "\n\n")

	for i, f := range m.remoteForm {
		cursor := "  "
		if i == m.remoteFormFocus {
			cursor = "> "
		}

		label := fmt.Sprintf("%-12s %s", remoteFormLabels[i]+":", f.View())
		if !m.remoteFieldFocusable(i) {
			// Non-applicable field (e.g. key path for ftp): show dimmed so the
			// user understands why Tab skips it.
			label = dimStyle.Render(fmt.Sprintf("%-12s (n/a for this protocol/auth)", remoteFormLabels[i]+":"))
		} else if i == m.remoteFormFocus {
			label = matchStyle.Render(label)
		}

		b.WriteString(cursor + label + "\n")
	}

	b.WriteString("\n" + hintStyle.Render("Tab next field  ·  Enter on last field opens file  ·  empty path → browse  ·  Esc cancel"))

	if m.remoteBrowseErr != "" {
		b.WriteString("\n" + errorStyle.Render(m.remoteBrowseErr))
	}

	if m.remoteHostPrompt != nil {
		b.WriteString("\n\n" + errorStyle.Render(
			fmt.Sprintf("Unknown host %s (fingerprint %s).\nPress y to trust and continue, n to cancel.",
				m.remoteHostPrompt.Host, m.remoteHostPrompt.Fingerprint)))
	}

	return b.String()
}

// remoteBrowseView renders the remote directory listing overlay.
func (m model) remoteBrowseView() string {
	var b strings.Builder
	b.WriteString(headerStyle.Render("Remote directory") + "\n")
	pathLabel := m.remoteCurrentPath
	if pathLabel == "" {
		pathLabel = "~"
	}
	b.WriteString(dimStyle.Render("path: "+pathLabel) + "\n\n")

	switch {
	case m.remoteLoading:
		b.WriteString(infoStyle.Render("  listing…") + "\n")
	case len(m.remoteEntries) == 0 && m.remoteBrowseErr == "":
		b.WriteString(dimStyle.Render("  (empty directory)") + "\n")
	default:
		for i, e := range m.remoteEntries {
			icon := "·"
			suffix := ""
			switch e.Kind {
			case entryKindDir:
				icon = "▸"
				suffix = "/"
			case entryKindSymlink:
				icon = "→"
			}

			line := fmt.Sprintf("%s %s%s", icon, e.Name, suffix)
			if i == m.remoteCursor {
				b.WriteString("> " + matchStyle.Render(line) + "\n")
			} else {
				b.WriteString("  " + line + "\n")
			}
		}
	}

	if m.remoteBrowseErr != "" {
		b.WriteString("\n" + errorStyle.Render(m.remoteBrowseErr))
	}

	b.WriteString("\n" + hintStyle.Render("↑↓/jk move  ·  Enter open file / enter dir  ·  Backspace parent  ·  Esc back to form"))

	return b.String()
}

// renderLogLines renders the log body into a single string using the given pane
// width. In split view it always shows the full log (matches live in the right
// pane); outside split view it honors the results-only (onlyMatches) filter.
//
// Every row is hard-clipped to `width` cells so a single over-wide line cannot
// soft-wrap in the terminal and push the next row's gutter out of column
// (visible as a jagged "|" gutter strip).
func (m model) renderLogLines(width int) string {
	rows := m.logRows(width)
	// Clip to width-2: writing into the last columns makes many terminals
	// soft-wrap (spurious blank line after every row).
	clipW := width
	if clipW > 2 {
		clipW -= 2
	}
	for i, r := range rows {
		if j := strings.IndexByte(r, '\n'); j >= 0 {
			r = r[:j]
		}
		// Truncate only — do NOT pad to full width (padding to termW causes wrap).
		rows[i] = clipToWidth(r, clipW)
	}
	for len(rows) < m.height {
		rows = append(rows, "")
	}
	if len(rows) > m.height && m.height > 0 {
		rows = rows[:m.height]
	}
	return joinRows(rows)
}

// clipToWidth truncates s to at most width display cells without padding.
func clipToWidth(s string, width int) string {
	if width <= 0 {
		return ""
	}
	if displayWidthNoANSI(s) <= width {
		return s
	}
	return forceFitWidth(s, width)
}

// logRows is the row-producing core behind renderLogLines: it returns the body
// as one string per terminal row, without the newlines that join them.
//
// The split view consumes these rows directly (M2). Rendering the pane into a
// single string and then splitting it back apart meant every visible row was
// copied into the pane buffer, the whole buffer copied out by Builder.String,
// and then scanned and re-sliced by strings.Split — a full extra copy of both
// panes on every keystroke, purely to undo the join that had just been done.
func (m model) logRows(width int) []string {
	gutterW := shared.MaxLineNumWidth(m.idx.TotalLines())

	lo, hi, selOK := m.selectionBounds()

	// gutter occupies gutterW digits + " | " (3 cells). Content budget is the rest.
	avail := max(width-gutterW-3, 1)
	layout := rowLayout{
		gutterW: gutterW,
		avail:   avail,
		selLo:   lo,
		selHi:   hi,
		selOK:   selOK,
	}

	if m.onlyMatches && len(m.matches) > 0 && !m.splitView {
		return m.matchOnlyRows(layout)
	}

	return m.fullLogRows(layout)
}

// rowLayout carries the per-render geometry and selection bounds shared by the
// two row producers behind logRows.
type rowLayout struct {
	gutterW int
	avail   int
	selLo   int64
	selHi   int64
	selOK   bool
}

// matchOnlyRows renders the results-only filter view: only matched lines, using
// the match index as the logical row position (m.top is a match index). Each
// match is hard-wrapped to the pane width.
func (m model) matchOnlyRows(layout rowLayout) []string {
	// rows is both the output and the row budget: len(rows) is how many terminal
	// rows have been emitted, which is what m.height bounds. Deriving the count
	// from the accumulated buffer (countNewlines(b.String())) instead copied and
	// rescanned everything written so far once per row, making the render
	// O(rows²).
	rows := make([]string, 0, m.height)

	var segs []string // reused scratch for wrapSegments across every row

	for i := int(m.top); i < len(m.matches) && len(rows) < m.height; i++ {
		if i < 0 {
			continue
		}

		ln := m.matches[i]

		content, e := m.idx.GetLine(ln)
		if e != nil {
			content = fmt.Sprintf("<error reading line %d>", ln)
		}

		for si, seg := range wrapSegments(content, layout.avail, segs) {
			if len(rows) >= m.height {
				break
			}
			if si > 0 && seg == "" {
				continue
			}

			var line string
			switch {
			case layout.selOK && int64(i) >= layout.selLo && int64(i) <= layout.selHi:
				line = m.wrapLineStyled(layout.gutterW, ln, seg, si == 0, selectionStyle)
			case i == m.matchCursor:
				line = m.wrapLineStyled(layout.gutterW, ln, seg, si == 0, matchCursorStyle)
			default:
				// In results-only every row is a match — keep them tinted.
				line = m.wrapLineStyled(layout.gutterW, ln, seg, si == 0, matchStyle)
			}
			rows = appendRow(rows, line)
		}
	}

	return rows
}

// fullLogRows renders the full log (normal view and the left pane of split
// view). One logical line = one screen row; topWrap is ignored (no soft-wrap).
func (m model) fullLogRows(layout rowLayout) []string {
	rows := make([]string, 0, m.height)

	var segs []string
	for ln := m.top; ln < m.idx.TotalLines() && len(rows) < m.height; ln++ {
		content, e := m.idx.GetLine(ln)
		if e != nil {
			content = fmt.Sprintf("<error reading line %d>", ln)
		}

		// wrapSegments returns exactly one segment (truncate, never wrap).
		segList := wrapSegments(content, layout.avail, segs)
		seg := ""
		if len(segList) > 0 {
			seg = segList[0]
		}

		var line string
		switch {
		case layout.selOK && ln >= layout.selLo && ln <= layout.selHi:
			line = m.wrapLineStyled(layout.gutterW, ln, seg, true, selectionStyle)
		case m.isMatchCursorLine(ln):
			line = m.wrapLineStyled(layout.gutterW, ln, seg, true, matchCursorStyle)
		case isMatchLine(m.matches, ln):
			line = m.wrapLineStyled(layout.gutterW, ln, seg, true, matchStyle)
		default:
			line = m.wrapLine(layout.gutterW, ln, seg, true)
		}
		// Never let embedded newlines expand one logical line into many rows.
		if i := strings.IndexByte(line, '\n'); i >= 0 {
			line = line[:i]
		}
		rows = append(rows, line)
	}

	// Pad to exact viewport height so the terminal never keeps ghost rows
	// from a previous frame (looks like vertical gaps / shifted lines).
	for len(rows) < m.height {
		rows = append(rows, "")
	}
	return rows
}

// isMatchCursorLine reports whether ln is the line under the match cursor (n/p).
func (m model) isMatchCursorLine(ln int64) bool {
	if m.matchCursor < 0 || m.matchCursor >= len(m.matches) {
		return false
	}
	return m.matches[m.matchCursor] == ln
}

// wrapLineStyled is wrapLine with a lipgloss style applied only to the content
// column so gutter metrics stay stable.
func (m model) wrapLineStyled(gutterW int, ln int64, seg string, first bool, style lipgloss.Style) string {
	seg = style.Render(seg)
	return m.wrapLine(gutterW, ln, seg, first)
}

// wrapLine formats one log row: fixed-width line number + continuous vertical
// rule (U+2502 │) + content. The box-drawing glyph joins row-to-row into a
// solid column; plain ASCII "|" leaves visible gaps between cells.
//
// Gutter (number + rule) is rendered in gutterStyle so it stays secondary to
// log text and search highlights. Width is still gutterW+3 cells.
func (m model) wrapLine(gutterW int, ln int64, seg string, first bool) string {
	var gutter string
	if first {
		num := strconv.FormatInt(ln+1, 10)
		if len(num) > gutterW {
			num = num[len(num)-gutterW:]
		}
		// " N│ " — spaces around continuous rule (3 cells, same budget as before).
		gutter = spaces(gutterW-len(num)) + num + " │ "
	} else {
		gutter = spaces(gutterW) + " │ "
	}
	return gutterStyle.Render(gutter) + seg
}

// sanitizeLogDisplay removes characters that do not belong in a log viewport:
// C0 controls, DEL, C1 controls, bidi/zero-width format chars,
// and invalid UTF-8 bytes. ESC is kept so intentional ANSI in the file still
// parses. Tabs are explicitly expanded to spaces to prevent terminal cell math bugs.
func sanitizeLogDisplay(s string) string {
	if s == "" {
		return s
	}

	s = strings.ReplaceAll(s, "\t", "    ")

	// Fast path: pure printable ASCII.
	clean := true
	for i := 0; i < len(s); i++ {
		c := s[i]

		if c >= 0x20 && c < 0x7f {
			continue
		}
		clean = false
		break
	}
	if clean {
		return s
	}

	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); {
		c := s[i]
		if c == 0x1b {
			b.WriteByte(c)
			i++
			continue
		}
		if c < 0x20 || c == 0x7f {
			i++
			continue
		}
		r, size := utf8.DecodeRuneInString(s[i:])
		if size <= 0 {
			break
		}
		if r == utf8.RuneError && size == 1 {
			i++
			continue
		}
		if shouldDropRune(r) {
			i += size
			continue
		}
		b.WriteString(s[i : i+size])
		i += size
	}
	return b.String()
}

// shouldDropRune reports runes that break terminal layout or appear as junk.
func shouldDropRune(r rune) bool {
	switch {
	case r == 0xFFFD: // replacement character
		return true
	case r >= 0x80 && r <= 0x9F: // C1 controls
		return true
	case r == 0x200B || r == 0x200C || r == 0x200D || r == 0x2060 || r == 0xFEFF:
		return true // ZWSP, ZWNJ, ZWJ, WJ, BOM
	case r >= 0x202A && r <= 0x202E: // bidi embeddings
		return true
	case r >= 0x2066 && r <= 0x2069: // bidi isolates
		return true
	case r >= 0xFE00 && r <= 0xFE0F: // variation selectors
		return true
	case r >= 0xE0100 && r <= 0xE01EF:
		return true
	default:
		return false
	}
}

// appendRow appends one rendered row. Embedded newlines are collapsed to a
// single row (soft-wrap is disabled; multi-row expansion caused vertical gaps).
func appendRow(rows []string, line string) []string {
	if i := strings.IndexByte(line, '\n'); i >= 0 {
		line = line[:i]
	}
	return append(rows, line)
}

// joinRows writes rows back into the single newline-terminated body string the
// non-split renderers return.
func joinRows(rows []string) string {
	if len(rows) == 0 {
		return ""
	}
	n := 0
	for _, r := range rows {
		n += len(r) + 1
	}

	var b strings.Builder
	b.Grow(n)

	// Join with newlines between rows only — no trailing \n on the last row.
	// View() appends the status/hint block directly after the body; a trailing
	// newline here was an off-by-one that pushed the footer past the terminal
	// height (overlapping lines, especially visible in split view).
	for i, r := range rows {
		b.WriteString(r)
		if i < len(rows)-1 {
			b.WriteByte('\n')
		}
	}

	return b.String()
}

// renderResultsLines renders the search-result list (matched lines) into a single
// string of at most m.height rows using the given pane width. The current match
// (matchCursor) is highlighted.
func (m model) renderResultsLines(width int) string {
	return joinRows(m.resultRows(width))
}

// resultRows is the row-producing core behind renderResultsLines, consumed
// directly by the split view (see logRows).
func (m model) resultRows(width int) []string {
	gutterW := shared.MaxLineNumWidth(m.idx.TotalLines())

	avail := max(width-gutterW-3, 1)

	if len(m.matches) == 0 {
		return appendRow(make([]string, 0, 1), dimStyle.Render("(no search results — run / first)"))
	}
	// In split view, the results pane follows the log pane: start at resTop
	// (which is synced to the log pane's top line). Otherwise center on the
	// current match so it's always visible.
	start := m.resTop
	if !m.splitView {
		start = max(m.matchCursor-m.height/2, 0)

		if start > len(m.matches)-m.height {
			start = max(0, len(m.matches)-m.height)
		}
	}

	rows := make([]string, 0, m.height)

	var segs []string // reused scratch for wrapSegments across every row

	for i := start; i < len(m.matches) && len(rows) < m.height; i++ {
		ln := m.matches[i]

		content, e := m.idx.GetLine(ln)
		if e != nil {
			content = fmt.Sprintf("<error reading line %d>", ln)
		}

		for si, seg := range wrapSegments(content, avail, segs) {
			if len(rows) >= m.height {
				break
			}
			if si > 0 && seg == "" {
				continue
			}

			var line string
			if i == m.matchCursor {
				line = m.wrapLineStyled(gutterW, ln, seg, si == 0, matchCursorStyle)
			} else {
				// Other hits in the results pane stay lightly highlighted.
				line = m.wrapLineStyled(gutterW, ln, seg, si == 0, matchStyle)
			}
			rows = appendRow(rows, line)
		}
	}

	return rows
}

// renderSplitBody renders the 50/50 vertical split using widths from buildLayout.
// Pane cells are clipped independently (gutter-safe) so truncate never eats the
// start of a log line, and RTL is isolated so it cannot cross the divider.
func (m model) renderSplitBody() string {
	l := m.buildLayout()
	leftW, rightW := l.leftW, l.rightW
	if leftW < 1 {
		leftW = 1
	}
	if rightW < 1 {
		rightW = 1
	}

	left := m.logRows(leftW)
	right := m.resultRows(rightW)

	// Divider color shows focus: blue = LOG active, yellow = RESULTS active.
	// No triangles (user request) — color + status bar is enough.
	var div string
	if m.splitFocus == 1 {
		div = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#f9e2af")).
			Render("│")
	} else {
		div = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#89b4fa")).
			Render("│")
	}

	var b strings.Builder
	for i := range m.height {
		ls, rs := "", ""
		if i < len(left) {
			ls = left[i]
		}
		if i < len(right) {
			rs = right[i]
		}
		lp := clipPaneCell(ls, leftW)
		rp := clipPaneCell(rs, rightW)
		b.WriteString(lp)
		b.WriteByte(' ')
		b.WriteString(div)
		b.WriteByte(' ')
		b.WriteString(rp)
		if i < m.height-1 {
			b.WriteByte('\n')
		}
	}
	return b.String()
}

// clipPaneCell truncates s to at most `width` cells and pads with spaces so the
// split divider stays in a fixed column. Styles (match highlight) are preserved.
func clipPaneCell(s string, width int) string {
	if width <= 0 {
		return ""
	}
	s = isolateLTR(s)
	// Truncate without stripping styles when possible.
	if displayWidthNoANSI(s) > width {
		s = truncateANSIAware(s, width)
	}
	w := displayWidthNoANSI(s)
	if w < width {
		// Reset before pad so match/selection background does not fill the pad.
		if hasANSIIntroducer(s) {
			s += ansiReset
		}
		s += strings.Repeat(" ", width-w)
	}
	if displayWidthNoANSI(s) > width {
		return strings.Repeat(" ", width)
	}
	return s
}

// isComplexLTRRune reports whether r belongs to complex LTR scripts (CTL)
// that break terminal grid layout due to ambiguous cell widths.
func isComplexLTRRune(r rune) bool {
	switch {
	case r >= 0x0900 && r <= 0x109F: // Devanagari, Bengali, Gurmukhi, Gujarati, Oriya, Tamil, Telugu, Kannada, Malayalam, Sinhala, Thai, Lao, Tibetan, Myanmar
		return true
	case r >= 0x1780 && r <= 0x17FF: // Khmer
		return true
	default:
		return false
	}
}

// isUnsafePaneRune reports whether r can break fixed-pane geometry.
func isUnsafePaneRune(r rune) bool {
	return isRTLRune(r) || isComplexLTRRune(r)
}

// containsUnsafeRunes reports whether s contains any RTL or complex CTL runes.
func containsUnsafeRunes(s string) bool {
	for _, r := range s {
		if isUnsafePaneRune(r) {
			return true
		}
	}
	return false
}

// isolateLTR neutralizes unsafe characters (RTL + CTL) in a fixed pane by
// replacing them with '?' to preserve vertical divider alignment.
func isolateLTR(s string) string {
	if s == "" || !containsUnsafeRunes(s) {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if isUnsafePaneRune(r) {
			b.WriteByte('?')
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

func isRTLRune(r rune) bool {
	switch {
	case r >= 0x0590 && r <= 0x05FF:
		return true
	case r >= 0x0600 && r <= 0x06FF:
		return true
	case r >= 0x0700 && r <= 0x074F:
		return true
	case r >= 0x0750 && r <= 0x077F:
		return true
	case r >= 0x0780 && r <= 0x07BF:
		return true
	case r >= 0x07C0 && r <= 0x07FF:
		return true
	case r >= 0x08A0 && r <= 0x08FF:
		return true
	case r >= 0xFB50 && r <= 0xFDFF:
		return true
	case r >= 0xFE70 && r <= 0xFEFF:
		return true
	default:
		return false
	}
}

// containsRTL reports whether s has characters from common RTL scripts.
func containsRTL(s string) bool {
	for _, r := range s {
		if isRTLRune(r) {
			return true
		}
	}
	return false
}

// displayWidth returns the number of terminal cells a string occupies.
// It implements a simplified East Asian Width algorithm: most characters are
// width 1, but CJK unified ideographs, fullwidth forms, and related blocks
// are width 2. This matches what most terminals do.
func displayWidth(s string) int {
	// Fast path: pure-ASCII content (the common case for log lines) is exactly
	// one cell per byte, so a single byte scan computes the width with no rune
	// decoding or allocation (MED render-alloc reduction). Only multi-byte
	// content falls through to the full rune-wise width pass.
	for i := range len(s) {
		if s[i] >= 0x80 {
			goto slow
		}
	}

	return len(s)

slow:
	w := 0

	for _, r := range s {
		w += runeWidth(r)
	}

	return w
}

// displayWidthNoANSI returns the display width of s after stripping ANSI.
// Uses go-runewidth so CJK/emoji match terminal cell counts.
func displayWidthNoANSI(s string) int {
	return runewidth.StringWidth(stripANSI(s))
}

// ansiRe matches ANSI escape sequences (CSI/OSC): ESC [ … [@-~] or ESC ] … .
// It is compiled ONCE at package init, not per call: stripANSI runs at least
// once per rendered row (displayWidthNoANSI → padRuneDisplay/truncateToWidth),
// and the former per-call regexp.MustCompile dominated rendering — 63% of all
// allocations in a split-view render (1.9 GB over a BenchmarkViewSplit run).
var ansiRe = regexp.MustCompile(`[\x1b\x9b][[()#;?]*(?:[0-9]{1,4}(?:;[0-9]{0,4})*)?[0-9A-ORZcf-nqry=><~]`)

// stripANSI removes ANSI escape sequences (CSI, OSC, etc.) from s.
func stripANSI(s string) string {
	if !hasANSIIntroducer(s) {
		// Nothing can match, so skip the regexp entirely and return s as-is:
		// unstyled rows (the common case in the plain log pane) then measure
		// their width without a single allocation.
		return s
	}

	return ansiRe.ReplaceAllString(s, "")
}

// hasANSIIntroducer reports whether s contains a byte sequence that could start
// an ANSI escape sequence, i.e. either alternative of ansiRe's leading class:
// ESC (0x1B), or the C1 CSI rune U+009B — which the regexp engine only ever
// sees as its valid UTF-8 encoding 0xC2 0x9B (a lone 0x9B byte decodes to
// RuneError and does not match the class either).
func hasANSIIntroducer(s string) bool {
	for i := range len(s) {
		if s[i] == 0x1b {
			return true
		}

		if s[i] == 0xc2 && i+1 < len(s) && s[i+1] == 0x9b {
			return true
		}
	}

	return false
}

// wideRuneRanges lists the inclusive rune ranges that terminals render two cells
// wide: a simplified East Asian Width table (CJK unified ideographs, fullwidth
// forms, Hangul and related blocks). It is a sorted, non-overlapping table so
// runeWidth can binary-search it — as a switch this was one 56-branch function,
// while the data it encodes is a plain lookup table.
var wideRuneRanges = [...][2]rune{
	{0x1100, 0x11FF},   // Hangul Jamo
	{0x2E80, 0x2EFF},   // CJK Radicals Supplement
	{0x2F00, 0x2FDF},   // Kangxi Radicals
	{0x3000, 0x303F},   // CJK Symbols and Punctuation
	{0x3040, 0x309F},   // Hiragana
	{0x30A0, 0x30FF},   // Katakana
	{0x3100, 0x312F},   // Bopomofo
	{0x3130, 0x318F},   // Hangul Compatibility Jamo
	{0x3190, 0x319F},   // Kanbun
	{0x31A0, 0x31BF},   // Bopomofo Extended
	{0x31C0, 0x31EF},   // CJK Strokes
	{0x31F0, 0x31FF},   // Katakana Phonetic Extensions
	{0x3200, 0x32FF},   // Enclosed CJK Letters and Months
	{0x3300, 0x33FF},   // CJK Compatibility
	{0x3400, 0x4DBF},   // CJK Unified Ideographs Extension A
	{0x4E00, 0x9FFF},   // CJK Unified Ideographs
	{0xA960, 0xA97F},   // Hangul Jamo Extended-A
	{0xAC00, 0xD7A3},   // Hangul Syllables (modern Korean) — fullwidth in terminals
	{0xD7B0, 0xD7FF},   // Hangul Jamo Extended-B
	{0xF900, 0xFAFF},   // CJK Compatibility Ideographs
	{0xFE10, 0xFE1F},   // Vertical Forms
	{0xFE30, 0xFE4F},   // CJK Compatibility Forms
	{0xFE50, 0xFE6F},   // Small Form Variants
	{0xFF00, 0xFFEF},   // Halfwidth and Fullwidth Forms
	{0x1F000, 0x1FFFF}, // symbols and emoji
	{0x20000, 0x2FFFF}, // CJK Unified Ideographs Extension B+
	{0x30000, 0x3FFFF}, // CJK Unified Ideographs Extension G+
}

// runeWidth returns the display width of a single rune (terminal cells).
func runeWidth(r rune) int {
	return runewidth.RuneWidth(r)
}

// wrapSegments prepares one visual row per logical log line.
//
// Soft-wrapping was disabled on purpose: multi-segment wraps inserted extra
// terminal rows between line numbers (vertical gaps / "shifted" rows). Long
// lines are hard-truncated to `width` cells instead — one logical line always
// occupies exactly one screen row.
func wrapSegments(content string, width int, segs []string) []string {
	// Drop file-embedded ANSI first — our own styles are applied later.
	content = sanitizeLogDisplay(stripANSI(content))
	// Neutralize RTL / complex scripts so bidi cannot disturb layout.
	content = isolateLTR(content)
	if width <= 0 {
		return append(segs[:0], content)
	}
	if runewidth.StringWidth(content) > width {
		content = runewidth.Truncate(content, width, "")
	}
	return append(segs[:0], content)
}

// ansiReset restores default SGR attributes. Appended after any hard truncate
// of a styled string so a cut-off background/foreground cannot bleed into the
// rest of the row (the dark "fill" that appeared in split view).
const ansiReset = "\x1b[0m"

// padRuneDisplay returns a string that occupies exactly `width` terminal cells
// (pad with spaces or hard-truncate). Always force-fits after truncation so
// Unicode/RTL edge cases cannot overflow a split pane.
func padRuneDisplay(s string, width int) string {
	if width <= 0 {
		return ""
	}
	s = forceFitWidth(s, width)
	w := displayWidthNoANSI(s)
	if w >= width {
		return s
	}
	// Reset before padding spaces so a lingering background from match/
	// selection highlight does not paint the pad cells.
	if hasANSIIntroducer(s) {
		return s + ansiReset + strings.Repeat(" ", width-w)
	}
	return s + strings.Repeat(" ", width-w)
}

// forceFitWidth truncates s until its display width is <= width.
func forceFitWidth(s string, width int) string {
	if width <= 0 {
		return ""
	}
	if displayWidthNoANSI(s) <= width {
		return s
	}
	// Prefer runewidth on the plain text; re-apply trailing reset if styled.
	plain := stripANSI(s)
	cut := runewidth.Truncate(plain, width, "")
	if hasANSIIntroducer(s) {
		return cut + ansiReset
	}
	return cut
}

// truncateToWidth truncates s to at most width display cells. ANSI-aware:
// escape sequences do not consume width, and a trailing reset prevents
// attribute bleed when a styled run is cut mid-sequence.
func truncateToWidth(s string, width int) string {
	if width <= 0 {
		return ""
	}
	if displayWidthNoANSI(s) <= width {
		return s
	}
	return truncateANSIAware(s, width)
}

// truncateANSIAware walks s, copying ANSI sequences without charging width,
// and stops once `width` display cells of visible content have been emitted.
// Unknown ESC sequences are skipped entirely (not emitted) so a partial CSI
// can never leak a final letter (e.g. 'm','w','t') into the visible text.
// Always ends with ansiReset when any escape was present or skipped.
func truncateANSIAware(s string, width int) string {
	var b strings.Builder
	b.Grow(len(s) + len(ansiReset))
	w := 0
	i := 0
	hadANSI := false
	for i < len(s) {
		if s[i] == 0x1b {
			hadANSI = true
			j := i + 1
			if j < len(s) && s[j] == '[' {
				// CSI: ESC [ ... final @-~
				j++
				for j < len(s) {
					c := s[j]
					j++
					if c >= 0x40 && c <= 0x7e {
						break
					}
				}
				b.WriteString(s[i:j])
				i = j
				continue
			}
			if j < len(s) && s[j] == ']' {
				// OSC: ESC ] ... BEL or ST (ESC \)
				j++
				for j < len(s) {
					if s[j] == 0x07 {
						j++
						break
					}
					if s[j] == 0x1b && j+1 < len(s) && s[j+1] == '\\' {
						j += 2
						break
					}
					j++
				}
				b.WriteString(s[i:j])
				i = j
				continue
			}
			// Other ESC sequences (charset, etc.): skip whole sequence, emit nothing.
			// ESC ( B  /  ESC ) 0  /  ESC c  …
			if j < len(s) {
				c := s[j]
				j++
				// Two-byte intermediate forms: ESC SP F, ESC ( B, …
				if (c >= '(' && c <= '/') || c == ' ' {
					if j < len(s) {
						j++
					}
				}
			}
			i = j
			continue
		}

		// Decode one UTF-8 rune for visible content.
		r, size := utf8.DecodeRuneInString(s[i:])
		if size <= 0 {
			break
		}
		if r == utf8.RuneError && size == 1 {
			// Invalid byte — skip rather than emit replacement garbage.
			i++
			continue
		}
		rw := runeWidth(r)
		if w+rw > width {
			break
		}
		b.WriteString(s[i : i+size])
		w += rw
		i += size
	}
	if hadANSI {
		b.WriteString(ansiReset)
	}
	return b.String()
}

// searchFlagString renders the active search-option toggles for the prompt.
func (m model) searchFlagString() string {
	var b strings.Builder
	b.WriteString("[")

	if m.searchOpts.IsRegex {
		b.WriteString("R")
	}

	if m.searchOpts.InvertMatch {
		b.WriteString("I")
	}

	if m.searchOpts.CaseSensitive {
		b.WriteString("CS")
	} else {
		b.WriteString("CI")
	}

	b.WriteString("]")

	return b.String()
}

// navHint returns the navigation footer line for the current width + language.
func (m model) navHint() string {
	w := m.width
	switch {
	case w > 0 && w < 70:
		return tr(m, "hint_mini")
	case w > 0 && w < 110:
		return tr(m, "hint_short")
	default:
		return tr(m, "hint_full")
	}
}

// helpKey formats one help row: fixed-width key column + description.
func helpKey(keys, desc string) string {
	return "  " + keyStyle.Render(fmt.Sprintf("%-22s", keys)) + "  " + desc
}

// helpView renders the full-screen help overlay with a clear two-column layout.
func (m model) helpView() string {
	lines := []string{
		headerStyle.Render("GoFLE  ·  keyboard shortcuts"),
		"",
		keyStyle.Render("▸ Navigation"),
		helpKey("↑ ↓   j k", "line up / down"),
		helpKey("PgUp PgDn", "page up / down"),
		helpKey("Ctrl+U / Ctrl+D", "page up / down (alt)"),
		helpKey("Home / End   G", "top / bottom of file"),
		helpKey("g", "go to line number"),
		"",
		keyStyle.Render("▸ Search"),
		helpKey("/", "search  (Enter run, Esc cancel)"),
		helpKey("\\", "AND-filter inside current results"),
		helpKey("Tab", "swap primary ↔ AND prompt"),
		helpKey("Alt+R  Alt+I  Alt+C", "regex · invert · match-case"),
		helpKey("/pat  !pat  i:pat", "prefix: regex · invert · ignore-case"),
		helpKey("n / p    F3 / ⇧F3", "next / previous match"),
		helpKey("[ / ]", "first / last match"),
		helpKey("x   Esc", "clear search"),
		"",
		keyStyle.Render("▸ View"),
		helpKey("m", "cycle  normal → results-only → split"),
		helpKey("s", "split view on/off  (log | results)"),
		helpKey("r", "results-only on/off"),
		helpKey("Tab", "focus other pane  (in split)"),
		helpKey("f", "follow / live-tail on/off"),
		"",
		keyStyle.Render("▸ Select & export"),
		helpKey("v", "start or cancel line selection"),
		helpKey("c", "copy selection to clipboard"),
		helpKey("e", "export file or matches to path"),
		"",
		keyStyle.Render("▸ Other"),
		helpKey("o", "open recent file"),
		helpKey("S", "remote connect  (SSH / FTP)"),
		helpKey("?   F1", "this help"),
		helpKey("q   Ctrl+C", "quit"),
		"",
		dimStyle.Render("Examples"),
		dimStyle.Render("  gofle /var/log/app.log"),
		dimStyle.Render("  gofle ssh://user@host/var/log/sys.log"),
		"",
		infoStyle.Render("Close help:  ?  ·  Esc  ·  q"),
	}

	return strings.Join(lines, "\n")
}

// styles (dark theme, tuned for clarity and hierarchy).
// Palette is Catppuccin Mocha with increased contrast on interactive / status
// elements so the interface stays easy to scan in a busy terminal.
var (
	// matchStyle: search hits — bright fg only (no Background: some terminals
	// paint background cell-height larger and it reads as extra line spacing).
	matchStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#f9e2af"))

	// matchCursorStyle: active match — reverse video, still no bold/underline.
	matchCursorStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#1e1e2e")).
				Background(lipgloss.Color("#f9e2af"))

	// statusStyle: plain text footer — no solid background fill.
	statusStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#cdd6f4"))

	// statusFileStyle: filename stands out slightly inside the bar.
	statusFileStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#cba6f7")).
				Bold(true)

	// statusSepStyle: subtle separators between status fields.
	statusSepStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#585b70"))

	// statusValueStyle highlights numeric values (line counts, match index).
	statusValueStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#f9e2af")).
				Bold(true)

	// selectionStyle: selection range — cyan on dark surface.
	selectionStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#1e1e2e")).
			Background(lipgloss.Color("#89dceb"))

	// divStyle draws the vertical divider between the split panes.
	divStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#6c7086"))

	statusOnStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#a6e3a1")).Bold(true)
	statusOffStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#f38ba8")).Bold(true)

	errorStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#f38ba8")).Bold(true)
	infoStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#a6e3a1"))
	searchStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#89b4fa")).Bold(true)
	gotoStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#f9e2af")).Bold(true)
	exportStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#94e2d5")).Bold(true)
	confirmStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#fab387")).Bold(true)

	// hintStyle: secondary navigation help — muted but still legible.
	hintStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#6c7086"))

	// headerStyle / keyStyle: strong hierarchy for help and forms.
	headerStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#cba6f7")).Bold(true)
	keyStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#89b4fa")).Bold(true)

	// dimStyle: inactive / secondary text (empty states, examples).
	dimStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#7f849c"))

	// gutterStyle: line numbers + vertical rule — low contrast so they never
	// compete with log text or match highlights.
	gutterStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#585b70"))
)
