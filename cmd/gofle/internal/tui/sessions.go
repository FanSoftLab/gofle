package tui

import (
	"GoFLE/cmd/gofle/internal/shared"
	"GoFLE/pkg/indexer"
	"GoFLE/pkg/remote"
	"GoFLE/pkg/search"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// fileSession holds one open file's resources and view/search state.
// The active session's state lives on model; inactive sessions are stored here.
// This keeps the Bubbletea model thin: switch = snapshot + restore, no full
// model clone.
type fileSession struct {
	target      string
	idx         *indexer.FileIndexer
	se          *search.SearchEngine
	cleanup     func()
	remote      *remote.Config
	remoteProto string

	// View / search snapshot.
	top, topWrap   int64
	wordWrap       bool
	hOffset        int
	matches        []int64
	matchCursor    int
	primaryMatches []int64
	primaryKey     string
	query          string
	secondaryQuery string
	searchOpts     search.SearchOptions
	onlyMatches    bool
	splitView      bool
	splitFocus     int
	resTop         int
	tailOn         bool
	selActive      bool
	selAnchor      int64

	// Advanced filters snapshot.
	advExclude      []string
	advExcludeRegex bool
	advNearbyQuery  string
	advNearbyMode   string
	advNearbyWindow int
	advLevels       []string
	advTimeFrom     string
	advTimeTo       string
	advTimeRequire  bool
	advLineStart    int64
	advLineEnd      int64
	advCtxBefore    int
	advCtxAfter     int

	// Optional tab group (desktop parity, simplified).
	groupID    string
	groupTitle string
	groupColor string // lipgloss color name or hex, e.g. "#89b4fa"
}

// snapshotSession copies the active model state into a fileSession.
func (m *model) snapshotSession() *fileSession {
	s := &fileSession{
		target:      m.sessionTarget(),
		idx:         m.idx,
		se:          m.se,
		cleanup:     m.cleanup,
		remote:      m.lastRemoteCfg,
		remoteProto: m.remoteProto,

		top:            m.top,
		topWrap:        int64(m.topWrap),
		wordWrap:       m.wordWrap,
		hOffset:        m.hOffset,
		matches:        m.matches,
		matchCursor:    m.matchCursor,
		primaryMatches: m.primaryMatches,
		primaryKey:     m.primaryKey,
		query:          m.query,
		secondaryQuery: m.secondaryQuery,
		searchOpts:     m.searchOpts,
		onlyMatches:    m.onlyMatches,
		splitView:      m.splitView,
		splitFocus:     m.splitFocus,
		resTop:         m.resTop,
		tailOn:         m.tailOn,
		selActive:      m.selActive,
		selAnchor:      m.selAnchor,

		advExclude:      append([]string(nil), m.advExclude...),
		advExcludeRegex: m.advExcludeRegex,
		advNearbyQuery:  m.advNearbyQuery,
		advNearbyMode:   m.advNearbyMode,
		advNearbyWindow: m.advNearbyWindow,
		advLevels:       append([]string(nil), m.advLevels...),
		advTimeFrom:     m.advTimeFrom,
		advTimeTo:       m.advTimeTo,
		advTimeRequire:  m.advTimeRequire,
		advLineStart:    m.advLineStart,
		advLineEnd:      m.advLineEnd,
		advCtxBefore:    m.advCtxBefore,
		advCtxAfter:     m.advCtxAfter,
		groupID:         m.tabGroupID,
		groupTitle:      m.tabGroupTitle,
		groupColor:      m.tabGroupColor,
	}
	return s
}

// restoreSession loads a fileSession into the live model fields.
func (m *model) restoreSession(s *fileSession) {
	if s == nil {
		return
	}
	m.idx = s.idx
	m.se = s.se
	m.cleanup = s.cleanup
	m.lastRemoteCfg = s.remote
	m.remoteProto = s.remoteProto

	m.top = s.top
	m.topWrap = int(s.topWrap)
	m.wordWrap = s.wordWrap
	m.hOffset = s.hOffset
	m.matches = s.matches
	m.matchCursor = s.matchCursor
	m.primaryMatches = s.primaryMatches
	m.primaryKey = s.primaryKey
	m.query = s.query
	m.secondaryQuery = s.secondaryQuery
	m.searchInput.SetValue(s.query)
	m.secondaryInput.SetValue(s.secondaryQuery)
	m.searchOpts = s.searchOpts
	m.onlyMatches = s.onlyMatches
	m.splitView = s.splitView
	m.splitFocus = s.splitFocus
	m.resTop = s.resTop
	m.tailOn = s.tailOn
	m.selActive = s.selActive
	m.selAnchor = s.selAnchor

	m.advExclude = append([]string(nil), s.advExclude...)
	m.advExcludeRegex = s.advExcludeRegex
	m.advNearbyQuery = s.advNearbyQuery
	m.advNearbyMode = s.advNearbyMode
	m.advNearbyWindow = s.advNearbyWindow
	m.advLevels = append([]string(nil), s.advLevels...)
	m.advTimeFrom = s.advTimeFrom
	m.advTimeTo = s.advTimeTo
	m.advTimeRequire = s.advTimeRequire
	m.advLineStart = s.advLineStart
	m.advLineEnd = s.advLineEnd
	m.advCtxBefore = s.advCtxBefore
	m.advCtxAfter = s.advCtxAfter
	m.tabGroupID = s.groupID
	m.tabGroupTitle = s.groupTitle
	m.tabGroupColor = s.groupColor

	// Close any modal overlays when switching tabs.
	m.searchMode = false
	m.secondaryMode = false
	m.gotoMode = false
	m.exportMode = false
	m.exportConfirm = false
	m.help = false
	m.recentMode = false
	m.advMode = false
	m.settingsMode = false
	m.groupMode = false
	m.topMode = false
	m.presetMode = false
	m.remoteMode = false
	m.remoteBrowseMode = false
}

// sessionTarget returns a stable label for the current open file.
func (m model) sessionTarget() string {
	if m.idx == nil {
		return ""
	}
	return m.idx.FilePath()
}

// sessionLabel short display name for a session (basename).
func sessionLabel(s *fileSession) string {
	if s == nil || s.idx == nil {
		return "?"
	}
	name := filepath.Base(s.idx.FilePath())
	if name == "" || name == "." {
		name = s.target
	}
	if len(name) > 20 {
		name = name[:19] + "…"
	}
	if s.groupTitle != "" {
		g := s.groupTitle
		if len(g) > 8 {
			g = g[:7] + "…"
		}
		return g + "/" + name
	}
	return name
}

// ensureSessionsInitialized makes sure sessions[active] exists for the current open.
func (m *model) ensureSessionsInitialized() {
	if len(m.sessions) > 0 {
		return
	}
	m.sessions = []*fileSession{m.snapshotSession()}
	m.activeSession = 0
}

// saveActiveSession writes live model state into sessions[activeSession].
func (m *model) saveActiveSession() {
	m.ensureSessionsInitialized()
	if m.activeSession < 0 || m.activeSession >= len(m.sessions) {
		m.activeSession = 0
	}
	m.sessions[m.activeSession] = m.snapshotSession()
}

// switchToSession saves the current session and restores sessions[i].
func (m *model) switchToSession(i int) {
	if i < 0 || i >= len(m.sessions) || i == m.activeSession {
		return
	}
	m.saveActiveSession()
	m.activeSession = i
	m.restoreSession(m.sessions[i])
	m.status = fmt.Sprintf("tab %d/%d · %s", i+1, len(m.sessions), sessionLabel(m.sessions[i]))
}

// nextSession cycles to the next tab.
func (m *model) nextSession() {
	m.ensureSessionsInitialized()
	if len(m.sessions) < 2 {
		m.status = "only one tab open"
		return
	}
	m.switchToSession((m.activeSession + 1) % len(m.sessions))
}

// prevSession cycles to the previous tab.
func (m *model) prevSession() {
	m.ensureSessionsInitialized()
	if len(m.sessions) < 2 {
		m.status = "only one tab open"
		return
	}
	i := m.activeSession - 1
	if i < 0 {
		i = len(m.sessions) - 1
	}
	m.switchToSession(i)
}

// closeActiveSession closes the current tab. If it is the last tab, quits the viewer.
func (m *model) closeActiveSession() tea.Cmd {
	m.ensureSessionsInitialized()
	if len(m.sessions) <= 1 {
		if m.cancel != nil {
			m.cancel()
		}
		return tea.Quit
	}
	// Tear down resources for the tab being closed (async, like reopen).
	cur := m.sessions[m.activeSession]
	if cur != nil && cur.cleanup != nil {
		go cur.cleanup()
		cur.cleanup = nil
	}
	// Remove from slice.
	m.sessions = append(m.sessions[:m.activeSession], m.sessions[m.activeSession+1:]...)
	if m.activeSession >= len(m.sessions) {
		m.activeSession = len(m.sessions) - 1
	}
	m.restoreSession(m.sessions[m.activeSession])
	// Prevent double-cleanup on quit: model.cleanup is the restored session's.
	m.status = fmt.Sprintf("tab closed · %d left · %s", len(m.sessions), sessionLabel(m.sessions[m.activeSession]))
	return nil
}

// openInNewTab resolves target and appends a new session without closing the current one.
func (m *model) openInNewTab(target string) error {
	idx, se, cleanup, err := shared.ResolveTarget(m.ctx, target, m.cfg)
	if err != nil {
		return fmt.Errorf("failed to open %s: %w", target, err)
	}

	m.saveActiveSession()

	// Build a fresh session for the new file.
	m.idx = idx
	m.se = se
	m.cleanup = cleanup
	m.resetViewerState()
	m.clearAdvancedFilters()
	m.seedAdvancedDefaults()
	m.lastRemoteCfg = nil
	m.remoteProto = ""
	if m.cfg != nil {
		if sum := findRecentSummaryForTarget(m.cfg, target); sum != nil {
			cfg := remoteConfigFromSummary(sum)
			m.lastRemoteCfg = &cfg
			m.remoteProto = cfg.Protocol
			m.fillRemoteFormFromRecent(sum)
		}
	}

	newSess := m.snapshotSession()
	newSess.target = target
	m.sessions = append(m.sessions, newSess)
	m.activeSession = len(m.sessions) - 1
	m.status = fmt.Sprintf("new tab %d · %s", m.activeSession+1, sessionLabel(newSess))
	return nil
}

// openSelectedRecentAsTab opens the highlighted recent entry in a new tab.
func (m *model) openSelectedRecentAsTab() {
	if len(m.recentList) == 0 {
		return
	}
	item := m.recentList[m.recentCursor]
	target := item.Path
	if item.IsSSH && item.SSH != nil {
		target = remoteTargetFromRecent(item)
	}
	if err := m.openInNewTab(target); err != nil {
		m.status = err.Error()
	}
	m.recentMode = false
	m.recentList = nil
}

// tabsBar returns a compact one-line tab strip for the status area.
func (m model) tabsBar() string {
	if len(m.sessions) < 2 {
		return ""
	}
	var b strings.Builder
	for i, s := range m.sessions {
		label := sessionLabel(s)
		cell := fmt.Sprintf(" %d:%s ", i+1, label)
		switch {
		case i == m.activeSession:
			b.WriteString(matchCursorStyle.Render(cell))
		case s != nil && s.groupColor != "":
			st := lipgloss.NewStyle().Foreground(lipgloss.Color(s.groupColor))
			b.WriteString(st.Render(cell))
		default:
			b.WriteString(dimStyle.Render(cell))
		}
	}
	return b.String()
}

// tabKey handles multi-tab navigation keys. Returns true if consumed.
func (m *model) tabKey(key string) bool {
	switch key {
	case "ctrl+tab", "ctrl+pagedown":
		m.nextSession()
		return true
	case "ctrl+shift+tab", "ctrl+pageup":
		m.prevSession()
		return true
	case "ctrl+w":
		// close handled separately (may return tea.Quit)
		return false
	}
	// Number keys 1-9 jump to tab when multiple sessions exist.
	if len(m.sessions) >= 2 && len(key) == 1 && key[0] >= '1' && key[0] <= '9' {
		i := int(key[0] - '1')
		if i < len(m.sessions) {
			m.switchToSession(i)
			return true
		}
	}
	return false
}
