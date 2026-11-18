package tui

import (
	"GoFLE/cmd/gofle/internal/shared"
	"GoFLE/pkg/config"
	"GoFLE/pkg/indexer"
	"GoFLE/pkg/remote"
	"GoFLE/pkg/search"
	"GoFLE/pkg/ssh"
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbletea"
)

// tickMsg is emitted every followInterval by the follow ticker to poll the
// indexer for appended lines (tail) and to extend an active search.
type tickMsg struct{}

// followInterval is how often the viewer polls the file for growth.
const followInterval = 500 * time.Millisecond

// model is the Bubbletea model for the interactive viewer.
type model struct {
	idx     *indexer.FileIndexer
	se      *search.SearchEngine
	cleanup func()
	cfg     *config.AppConfig

	top        int64 // first visible (0-based) line, OR match index in results-only mode
	height     int   // viewport rows (body only)
	fullHeight int   // full terminal height from last WindowSizeMsg
	width      int   // terminal columns (0 until WindowSizeMsg)

	// statusGen is bumped every time status is set; statusClearMsg carries the
	// gen it was scheduled for so a newer message is not wiped by an older tick.
	statusGen int

	matches     []int64 // matched line indices (ascending), AND-filtered by secondaryQuery
	matchCursor int     // index into matches of the current match
	// primaryMatches is the result of the primary query only (without the
	// secondary AND-filter). Storing it lets a secondary-query change refine the
	// cached primary matches in place instead of re-scanning the whole file
	// (filter-in-filter, H2). primaryKey captures the primary query's identity
	// (query + regex/case/invert flags) so runSearch can tell whether the
	// primary changed or only the secondary did.
	primaryMatches []int64
	primaryKey     string

	query string // raw search query (incl. modifiers) or "" when none
	// secondaryQuery is the AND-level filter applied on top of query results
	// (filter-in-filter). Edited via the `\` prompt.
	secondaryQuery string

	searchMode    bool   // search box focused
	secondaryMode bool   // secondary AND-filter prompt focused
	gotoMode      bool   // goto-line prompt focused
	exportMode    bool   // export path prompt focused
	exportConfirm bool   // awaiting y/N to create a missing directory
	exportTarget  string // resolved (absolute) export path pending confirmation

	// Advanced filters (GUI parity). Applied via pkg/search.ApplyAdvancedFilters;
	// the TUI only collects values into SearchOptions — no duplicated match logic.
	advMode         bool // advanced filter form overlay open
	advFocus        int  // focused field in advForm
	advForm         []textinput.Model
	advExclude      []string
	advExcludeRegex bool
	advNearbyQuery  string
	advNearbyMode   string // before | after | either
	advNearbyWindow int
	advLevels       []string
	advTimeFrom     string
	advTimeTo       string
	advTimeRequire  bool
	advLineStart    int64
	advLineEnd      int64
	advCtxBefore    int
	advCtxAfter     int

	// Top unique overlay (GUI "Top" feature).
	topMode    bool
	topEntries []topUniqueEntry
	topCursor  int

	// Search presets (shared with GUI via pkg/config search_presets.json).
	presetMode      bool
	presetList      []config.SearchPreset
	presetCursor    int
	presetSaveMode  bool
	presetNameInput textinput.Model

	// Multi-tab MVP: inactive sessions stored here; active state lives on model.
	sessions      []*fileSession
	activeSession int
	// openRecentAsTab: when true, Enter in recent picker opens a new tab.
	openRecentAsTab bool
	// Tab group for the active session (persisted into fileSession on snapshot).
	tabGroupID    string
	tabGroupTitle string
	tabGroupColor string
	// Group edit overlay.
	groupMode  bool
	groupInput textinput.Model

	// Settings overlay (shared settings.json).
	settingsMode  bool
	settingsFocus int
	settingsForm  []textinput.Model

	// Async search state (progress in status bar).
	searching    bool
	searchGen    int
	searchCancel context.CancelFunc
	searchProg   *atomic.Int64

	onlyMatches  bool // results-only filter view
	help         bool // help overlay open
	recentMode   bool // recent-files picker overlay open
	recentList   []config.RecentFileItem
	recentCursor int
	recentTop    int // first visible entry in the recent picker (windowed list)

	// Remote interactive connect + optional-path remote directory browser.
	// Protocol-agnostic: the form carries a protocol field and the lister /
	// streamer are dispatched on it (ssh or ftp). See remote.go.
	remoteMode           bool                     // connect form open
	remoteBrowseMode     bool                     // remote directory listing overlay open
	remoteForm           []textinput.Model        // protocol, host, port, user, auth, password, keyPath, remotePath
	remoteFormFocus      int                      // focused form field index
	remoteEntries        []remote.DirEntry        // current directory listing
	remoteCursor         int                      // highlighted entry index
	remoteTop            int                      // first visible entry in the remote browser (windowed list)
	remoteCurrentPath    string                   // path being browsed (""/"~" = home)
	remoteLoading        bool                     // listing in flight
	remoteOpening        bool                     // stream open (dial+index) in flight
	remoteProto          string                   // protocol of the active connection (ssh/ftp)
	remoteBrowseErr      string                   // last listing/browse error
	remoteHostPrompt     *ssh.HostKeyUnknownError // pending unknown-host trust prompt (ssh only)
	remoteAfterTrustOpen bool                     // after trusting host: open stream (vs re-list)
	// lastRemoteCfg holds the connection used for the currently open remote
	// file so "browse other file on this server" can reuse credentials without
	// re-prompting. Nil when the open target is local.
	lastRemoteCfg *remote.Config

	// Visual line selection (v to start, c to copy). selAnchor is the line the
	// selection was anchored on; the moving end is m.top, so scrolling with
	// ↑/↓/pgup/pgdn/home/end extends the selection across the gap.
	selActive bool
	selAnchor int64

	// Split view (s to toggle): a two-pane layout showing the log on the left
	// and the search-result list on the right. splitFocus picks which pane the
	// arrow keys drive; resTop is the first visible match in the right pane.
	splitView  bool
	splitFocus int // 0 = log pane, 1 = results pane
	resTop     int // first visible match index in the results pane

	// topWrap is the visual-row offset (>=0) of the top line: when the line at
	// m.top wraps to more than one visual row, this is which wrapped row sits at
	// the very top of the viewport. Only meaningful for the log pane (i.e. when
	// m.top is a logical line, not a match index).
	topWrap int
	// wordWrap soft-wraps long lines to the pane width. When false, long lines
	// are truncated and hOffset provides horizontal scrolling.
	wordWrap bool
	hOffset  int // horizontal column offset when !wordWrap (display cells)

	tailOn bool

	// searchOpts holds the live search-option toggle state edited with
	// Alt+R / Alt+I / Alt+C while the search box is focused. It is merged with
	// the prefix mini-syntax parsed from the query (see searchOptionsFromQuery).
	searchOpts search.SearchOptions

	searchInput    textinput.Model
	secondaryInput textinput.Model // AND-filter prompt (filter-in-filter)
	gotoInput      textinput.Model
	exportInput    textinput.Model

	err    error
	status string // transient confirmation/info line

	// ctx is the viewer's cancellable context, derived from the command context
	// in RunView. It has to live on the model: tea.Model's Init/Update/View
	// signatures carry no context, yet the work they start (a search over a
	// growing file, an ssh:// reopen, a remote dial) must observe the
	// interrupt cancellation installed by main.
	ctx    context.Context //nolint:containedctx // bubbletea's tea.Model methods take no context, so the cancellable viewer context must be stored
	cancel context.CancelFunc
}

// searchOptionsFromQuery translates a user-typed query into search options using
// a small, documented mini-syntax applied left-to-right:
//
//	plain      case-sensitive substring
//	/pattern   regex (case-sensitive)
//	!query     invert match (lines NOT matching)
//	i:query    case-insensitive substring
//
// Modifiers may be combined, e.g. "!/error" (regex, inverted) or
// "!i:warn" (case-insensitive substring, inverted). A query consisting only of
// modifiers (empty pattern) clears the match list.
func searchOptionsFromQuery(raw string) search.SearchOptions {
	opts := search.SearchOptions{CaseSensitive: true}

	q := raw
	if strings.HasPrefix(q, "!") {
		opts.InvertMatch = true
		q = q[1:]
	}

	if strings.HasPrefix(q, "/") {
		opts.IsRegex = true
		q = q[1:]
	}

	if strings.HasPrefix(q, "i:") {
		opts.CaseSensitive = false
		q = q[2:]
	}

	opts.Query = q

	return opts
}

// effectiveSearchOptions merges the prefix-derived options with the live toggle
// state. A toggle and a prefix on the same dimension combine: regex / invert are
// enabled if either source enables them; case-sensitivity is off (insensitive)
// if either source is off. This mirrors the desktop app, where Alt+R / Alt+I /
// Alt+C add options on top of whatever the query text implies.
func (m model) effectiveSearchOptions() search.SearchOptions {
	base := searchOptionsFromQuery(m.query)
	base.IsRegex = base.IsRegex || m.searchOpts.IsRegex
	base.InvertMatch = base.InvertMatch || m.searchOpts.InvertMatch
	base.CaseSensitive = base.CaseSensitive && m.searchOpts.CaseSensitive
	base.SecondaryQuery = m.secondaryQuery
	m.applyAdvancedToOpts(&base)

	return base
}

// andFilterLine renders the always-visible secondary (AND) filter line:
// the live input while editing, otherwise the applied value.
func (m model) andFilterLine() string {
	line := "AND in results: "
	if m.secondaryMode {
		return line + m.secondaryInput.View()
	}

	if m.secondaryQuery != "" {
		return line + m.secondaryQuery
	}

	return line + "(none) — \\ to edit"
}

// selectionBounds returns the inclusive [lo, hi] line range of the active
// selection, normalizing so lo <= hi regardless of scroll direction. ok is false
// when no selection is active.
func (m model) selectionBounds() (lo, hi int64, ok bool) {
	if !m.selActive {
		return 0, 0, false
	}

	lo, hi = m.selAnchor, m.top
	if hi < lo {
		lo, hi = hi, lo
	}

	return lo, hi, true
}

// keepMatchVisible scrolls the log pane (m.top/topWrap) so the current match
// is centered in the viewport when possible. When navigating the results list
// in split view this keeps the match centered with context above/below instead
// of pinning it to the top. It is a no-op in results-only mode, where m.top is
// a match index.
//
// With word wrap the centering happens in visual rows, not logical lines: a
// long line can occupy several screen rows, so positioning purely by line
// count would leave the match below the viewport. clampViewport finishes by
// enforcing the filled-viewport / EOF bounds (match sits lower than center
// near the end of the file).
func (m *model) keepMatchVisible() {
	if m.onlyMatches || len(m.matches) == 0 || m.height <= 0 {
		return
	}

	mc := m.matches[m.matchCursor]

	if m.wordWrap {
		m.top, m.topWrap = m.topWrapForCentered(mc)
	} else {
		m.topWrap = 0
		m.top = min(max(mc-int64(m.height)/2, 0), m.viewableEnd())
	}

	m.clampViewport()
}

// topWrapForCentered finds the (top, topWrap) pair that leaves roughly half a
// viewport of visual rows above the match line mc, walking back through the
// wrap-row counts. wrapRows is cache-backed, so the walk is bounded by the
// half-page of lines it visits.
func (m model) topWrapForCentered(mc int64) (int64, int) {
	need := m.height / 2
	acc := 0

	for ln := mc; ln > 0 && acc < need; ln-- {
		wr := m.wrapRows(ln - 1)
		if acc+wr >= need {
			// Line ln-1 straddles the boundary: show only its last
			// need-acc visual rows above the match.
			return ln - 1, wr - (need - acc)
		}
		acc += wr
	}

	return 0, 0
}

// ensureResVisible keeps the current match cursor inside the results pane's
// visible window by adjusting resTop. When word wrap is off the window is
// measured in match indices (1 row = 1 match). When word wrap is on the pane
// fills visual rows, so fewer matches fit; we walk forward from candidate
// starts until the cursor's first visual row is on screen.
// It is a no-op when there are no matches or the height is unknown.
func (m *model) ensureResVisible() {
	if len(m.matches) == 0 || m.height <= 0 {
		return
	}
	if m.matchCursor < 0 {
		m.matchCursor = 0
	}
	if m.matchCursor >= len(m.matches) {
		m.matchCursor = len(m.matches) - 1
	}

	if !m.wordWrap {
		if m.matchCursor < m.resTop {
			m.resTop = m.matchCursor
		} else if m.matchCursor >= m.resTop+m.height {
			m.resTop = m.matchCursor - m.height + 1
		}
		if m.resTop < 0 {
			m.resTop = 0
		}
		if m.resTop > len(m.matches)-1 {
			m.resTop = max(0, len(m.matches)-m.height)
		}
		return
	}

	// Word-wrap path: ensure matchCursor's first visual row is within the
	// body height when rendering from resTop.
	if m.matchCursorVisibleFrom(m.resTop) {
		return
	}
	// Cursor above the window — pin it to the top.
	if m.matchCursor < m.resTop {
		m.resTop = m.matchCursor
		return
	}
	// Cursor below the window: find the largest start index such that the
	// cursor still fits in the visual row budget.
	start := m.matchCursor
	for start > 0 && m.matchCursorVisibleFrom(start-1) {
		start--
	}
	m.resTop = start
}

// matchCursorVisibleFrom reports whether rendering the results pane from
// startIndex places matchCursor's first visual row inside the body height.
func (m model) matchCursorVisibleFrom(startIndex int) bool {
	if startIndex < 0 || m.matchCursor < startIndex || m.height <= 0 {
		return false
	}
	gutterW := shared.MaxLineNumWidth(m.idx.TotalLines())
	// Approximate content width; exact split width is not required for the
	// visibility test — we only need a consistent wrap budget.
	avail := max(m.width/2-gutterW-3, 1)
	rows := 0
	for i := startIndex; i < len(m.matches); i++ {
		ln := m.matches[i]
		content, err := m.idx.GetLine(ln)
		if err != nil {
			content = ""
		}
		wr := 1
		if m.wordWrap {
			wr = max(1, len(wrapSegmentsOpts(content, avail, nil, true, 0)))
		}
		if i == m.matchCursor {
			// First visual row of the cursor match must land on screen.
			return rows < m.height
		}
		rows += wr
		if rows >= m.height {
			return false
		}
	}
	return false
}

// recentVisibleSlots is how many recent entries fit in the current terminal
// (each entry occupies 2 rows: path + metadata). Header and footer take a few
// rows; we keep at least 1 slot.
func (m model) recentVisibleSlots() int {
	// Header (~2) + blank + footer hint (~2) ≈ 5 reserved rows.
	const reserved = 5
	h := m.fullHeight
	if h <= 0 {
		h = m.height
	}
	rows := max(h-reserved, 2)
	return max(1, rows/2)
}

// ensureRecentVisible keeps recentCursor inside the windowed recent list.
func (m *model) ensureRecentVisible() {
	if len(m.recentList) == 0 {
		m.recentTop = 0
		return
	}
	slots := m.recentVisibleSlots()
	if m.recentCursor < m.recentTop {
		m.recentTop = m.recentCursor
	} else if m.recentCursor >= m.recentTop+slots {
		m.recentTop = m.recentCursor - slots + 1
	}
	if m.recentTop < 0 {
		m.recentTop = 0
	}
	maxTop := max(0, len(m.recentList)-slots)
	if m.recentTop > maxTop {
		m.recentTop = maxTop
	}
}

// remoteVisibleSlots is how many remote directory entries fit on screen.
func (m model) remoteVisibleSlots() int {
	const reserved = 6 // header, path, blank, footer
	h := m.fullHeight
	if h <= 0 {
		h = m.height
	}
	return max(1, h-reserved)
}

// ensureRemoteVisible keeps remoteCursor inside the windowed remote browser.
func (m *model) ensureRemoteVisible() {
	if len(m.remoteEntries) == 0 {
		m.remoteTop = 0
		return
	}
	slots := m.remoteVisibleSlots()
	if m.remoteCursor < m.remoteTop {
		m.remoteTop = m.remoteCursor
	} else if m.remoteCursor >= m.remoteTop+slots {
		m.remoteTop = m.remoteCursor - slots + 1
	}
	if m.remoteTop < 0 {
		m.remoteTop = 0
	}
	maxTop := max(0, len(m.remoteEntries)-slots)
	if m.remoteTop > maxTop {
		m.remoteTop = maxTop
	}
}

// syncMatchToView synchronizes the right (results) pane with the left (log) pane
// in split view. It sets the results window (resTop) to the first match at or
// after the current log top line (m.top), and updates matchCursor to that match
// so it is highlighted at the top of the right pane.
func (m *model) syncMatchToView() {
	if len(m.matches) == 0 || !m.splitView {
		return
	}
	// Find the first match at or after the current log top line.
	i := sort.Search(len(m.matches), func(i int) bool { return m.matches[i] >= m.top })
	if i >= len(m.matches) {
		i = len(m.matches) - 1
	}

	m.resTop = i
	m.matchCursor = i
	m.ensureResVisible()
}

// lastLine is the last logical line index (or last match index in results-only).
func (m model) lastLine() int64 {
	if m.onlyMatches && len(m.matches) > 0 {
		return int64(len(m.matches) - 1)
	}
	n := m.idx.TotalLines()
	if n <= 0 {
		return 0
	}
	return n - 1
}

// viewableEnd is the maximum m.top such that the viewport is still filled when
// possible (no huge empty region below the last line).
func (m model) viewableEnd() int64 {
	if m.onlyMatches && len(m.matches) > 0 {
		return int64(len(m.matches) - 1)
	}
	if m.wordWrap {
		return m.viewableEndWrapped()
	}
	return max(m.idx.TotalLines()-int64(m.height), 0)
}

// viewableEndWrapped finds the greatest (top) so that remaining visual rows from
// (top, 0) are still at least m.height (or top=0 if the whole file is shorter).
func (m model) viewableEndWrapped() int64 {
	total := m.idx.TotalLines()
	if total <= 0 || m.height <= 0 {
		return 0
	}
	// Walk backward from EOF counting visual rows until we fill the viewport.
	need := m.height
	for ln := total - 1; ln >= 0; ln-- {
		wr := max(m.wrapRows(ln), 1)
		need -= wr
		if need <= 0 {
			// top should be this line; topWrap may be >0 for partial first line.
			return ln
		}
	}
	return 0
}

// remainingVisualRows counts display rows from (top, topWrap) to EOF (capped
// just above height so callers can test "fills the viewport").
func (m model) remainingVisualRows() int {
	if m.idx == nil {
		return 0
	}
	total := m.idx.TotalLines()
	if total <= 0 {
		return 0
	}
	rows := 0
	capN := max(m.height+1, 2)
	for ln := m.top; ln < total; ln++ {
		wr := max(m.wrapRows(ln), 1)
		start := 0
		if ln == m.top {
			start = min(m.topWrap, wr)
			if start < 0 {
				start = 0
			}
		}
		rows += wr - start
		if rows >= capN {
			return rows
		}
	}
	return rows
}

// clampViewport ensures top/topWrap stay within a range that keeps content on screen.
func (m *model) clampViewport() {
	if m.onlyMatches {
		last := m.lastLine()
		if m.top > last {
			m.top = last
		}
		if m.top < 0 {
			m.top = 0
		}
		m.topWrap = 0
		return
	}
	if m.top < 0 {
		m.top = 0
	}
	last := m.lastLine()
	if m.top > last {
		m.top = last
	}
	if m.wordWrap {
		wr := max(m.wrapRows(m.top), 1)
		if m.topWrap >= wr {
			m.topWrap = wr - 1
		}
		if m.topWrap < 0 {
			m.topWrap = 0
		}
		// If we overshot past the filled viewport, walk back.
		for m.remainingVisualRows() < m.height && (m.top > 0 || m.topWrap > 0) {
			switch {
			case m.topWrap > 0:
				m.topWrap--
			case m.top > 0:
				m.top--
				m.topWrap = max(0, m.wrapRows(m.top)-1)
			}
		}
	} else {
		m.topWrap = 0
		maxTop := m.viewableEnd()
		if m.top > maxTop {
			m.top = maxTop
		}
	}
}

// logPaneWidth returns the width (in cells) of the log/content pane that the
// scrolling logic should account for. In split view that is the left pane width;
// otherwise the full terminal width.
func (m model) logPaneWidth() int {
	if m.splitView {
		available := max(m.width-2, 4)

		leftW := max(available/2-1, 1)

		return leftW
	}

	return m.width
}

// wrapStamp captures everything that changes wrap-row counts: window
// geometry, split layout, file extent, the wrap mode itself, and the indexer
// generation (a rotation rebuild can change line content without changing
// the total line count).
type wrapStamp struct {
	width      int
	splitView  bool
	totalLines int64
	wordWrap   bool
	gen        uint64
}

// The wrap-count cache is package-scoped for the same reason as the view
// body memo (viewBody above): one tea.Program per process, View()/Update()
// single-threaded; the lock guards against concurrent callers. Without it,
// every wrap-mode keypress recomputed wrap counts with one GetLine pread +
// string per visible line, and clampViewport's walk-back loop re-walked the
// same lines through remainingVisualRows once per iteration — O(height²)
// preads per keypress (T3).
var (
	wrapCacheMu    sync.Mutex
	wrapCacheStamp wrapStamp
	wrapCacheSet   bool
	wrapCounts     map[int64]int
)

// wrapCacheMaxEntries bounds the memo (T3): ~24 bytes per entry, so 1<<18
// entries ≈ a few MB even on pathological full-file wrap scans. Exceeding it
// resets the cache; the next misses recompute.
const wrapCacheMaxEntries = 1 << 18

// wrapRows returns how many visual rows the given logical line occupies once
// hard-wrapped to the current log pane width, serving repeat counts from the
// wrap cache.
func (m model) wrapRows(ln int64) int {
	if !m.wordWrap {
		return 1
	}

	wrapCacheMu.Lock()
	defer wrapCacheMu.Unlock()

	st := wrapStamp{
		width:      m.width,
		splitView:  m.splitView,
		totalLines: m.idx.TotalLines(),
		wordWrap:   m.wordWrap,
		gen:        m.idx.Generation(),
	}

	if !wrapCacheSet || wrapCacheStamp != st {
		wrapCacheStamp = st
		wrapCacheSet = true
		wrapCounts = make(map[int64]int, 1024)
	}

	if n, ok := wrapCounts[ln]; ok {
		return n
	}

	n := max(m.computeWrapRows(ln), 1)

	if len(wrapCounts) >= wrapCacheMaxEntries {
		wrapCounts = make(map[int64]int, 1024)
	}

	wrapCounts[ln] = n

	return n
}

// computeWrapRows reads the line and counts its wrapped rows (uncached core).
func (m model) computeWrapRows(ln int64) int {
	content, e := m.idx.GetLine(ln)
	if e != nil {
		content = ""
	}

	gutterW := shared.MaxLineNumWidth(m.idx.TotalLines())

	avail := max(m.logPaneWidth()-gutterW-3, 1)

	n := len(wrapSegmentsOpts(content, avail, nil, true, 0))
	if n < 1 {
		return 1
	}

	return n
}

// Init implements tea.Model: focus the search box off and start the follow tick.
func (m model) Init() tea.Cmd {
	return tick()
}

// tick returns a command that fires a follow tick after the follow interval.
func tick() tea.Cmd {
	return tea.Tick(followInterval, func(time.Time) tea.Msg { return tickMsg{} })
}

// followTick polls the indexer for new lines and extends an active search. A
// successful poll clears a previous poll error: transient failures (rotation
// races, brief read errors) self-heal, and pinning the stale error forever made
// a healthy file look broken.
func (m model) followTick() (tea.Model, tea.Cmd) {
	prevTotal := m.idx.TotalLines()

	n, err := m.idx.UpdateIndex()
	if err != nil {
		m.err = err
	} else {
		m.err = nil
		if n > 0 && (len(m.query) > 0 || m.hasAdvancedFilters() || m.secondaryQuery != "") {
			m.extendSearch(prevTotal)
		}
	}

	if n > 0 && m.tailOn {
		last := max(m.lastLine(), 0)
		m.top = last
	}

	return m, tick()
}

// extendSearch scans the lines appended since prevTotal and appends their
// matches to the current result set. The cached primary matches are extended
// too, so the set stays in sync for future filter refinement (H2); secondary
// AND and advanced post-filters are then applied via pkg/search.
func (m *model) extendSearch(prevTotal int64) {
	opts := m.effectiveSearchOptions()
	if opts.Query == "" && !m.hasAdvancedFilters() && opts.SecondaryQuery == "" {
		return
	}
	if opts.Query == "" {
		opts.MatchAll = true
		opts.IsRegex = false
		opts.InvertMatch = false
	}

	primaryOpts := opts
	primaryOpts.SecondaryQuery = ""

	newPrimary, err := m.se.SearchRange(m.ctx, primaryOpts, prevTotal, m.idx.TotalLines())
	if err != nil {
		m.err = fmt.Errorf("failed to extend search over appended lines: %w", err)
		return
	}

	m.primaryMatches = append(m.primaryMatches, newPrimary...)

	newMatches := newPrimary
	if opts.SecondaryQuery != "" {
		newMatches, err = m.se.FilterLines(m.ctx, opts, newPrimary)
		if err != nil {
			m.err = fmt.Errorf("failed to extend search over appended lines: %w", err)
			return
		}
	}

	newMatches, err = m.se.ApplyAdvancedFilters(m.ctx, opts, newMatches)
	if err != nil {
		m.err = fmt.Errorf("failed to extend search over appended lines: %w", err)
		return
	}

	// Context expansion for live appends: expand only the new hits so we do not
	// re-expand the whole result set on every tail tick.
	if m.hasContextExpand() {
		newMatches = search.ExpandMatchesWithContext(newMatches, m.advCtxBefore, m.advCtxAfter, m.idx.TotalLines())
	}

	m.matches = append(m.matches, newMatches...)
}
