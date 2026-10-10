package tui

import (
	"GoFLE/cmd/gofle/internal/shared"
	"GoFLE/pkg/config"
	"fmt"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// buildSavedSession snapshots all open tabs into a config.SavedSession suitable
// for the shared saved_session.json used by the desktop app.
func (m *model) buildSavedSession() config.SavedSession {
	m.ensureSessionsInitialized()
	m.saveActiveSession()

	tabs := make([]config.SavedTabItem, 0, len(m.sessions))
	top := make([]config.SavedTopEntry, 0, len(m.sessions))
	activeID := ""
	groupMap := map[string]*config.SavedGroupItem{}

	for i, s := range m.sessions {
		if s == nil {
			continue
		}
		id := fmt.Sprintf("cli-%d", i)
		if i == m.activeSession {
			activeID = id
		}
		item := savedTabFromSession(s, id)
		tabs = append(tabs, item)
		if s.groupID != "" {
			g, ok := groupMap[s.groupID]
			if !ok {
				g = &config.SavedGroupItem{
					ID:    s.groupID,
					Title: s.groupTitle,
					Color: s.groupColor,
				}
				groupMap[s.groupID] = g
				top = append(top, config.SavedTopEntry{Type: "group", ID: s.groupID})
			}
			g.MemberIDs = append(g.MemberIDs, id)
		} else {
			top = append(top, config.SavedTopEntry{Type: "tab", ID: id})
		}
	}

	groups := make([]config.SavedGroupItem, 0, len(groupMap))
	for _, g := range groupMap {
		groups = append(groups, *g)
	}

	return config.SavedSession{
		ActiveTabID: activeID,
		TopLevel:    top,
		Tabs:        tabs,
		Groups:      groups,
	}
}

func savedTabFromSession(s *fileSession, id string) config.SavedTabItem {
	path := s.target
	if path == "" && s.idx != nil {
		path = s.idx.FilePath()
	}
	title := filepath.Base(path)
	isSSH := s.remote != nil || strings.HasPrefix(path, shared.SSHScheme) || strings.HasPrefix(path, shared.FTPScheme)
	var ssh *config.SSHConfigSummary
	if s.remote != nil {
		ssh = summaryFromRemoteConfig(s.remote)
	} else if isSSH {
		// Path may already be ssh:// or ftp:// URL.
		ssh = nil
	}
	return config.SavedTabItem{
		ID:      id,
		Title:   title,
		Path:    path,
		IsSSH:   isSSH,
		SSH:     ssh,
		GroupID: s.groupID,
	}
}

// targetFromSavedTab builds a local path or ssh:// / ftp:// URL for ResolveTarget.
func targetFromSavedTab(t config.SavedTabItem) string {
	if t.IsSSH && t.SSH != nil {
		proto := t.SSH.Protocol
		if proto == "" {
			proto = "ssh"
		}
		var b strings.Builder
		if proto == "ftp" {
			b.WriteString("ftp://")
		} else {
			b.WriteString("ssh://")
		}
		if t.SSH.Username != "" {
			b.WriteString(t.SSH.Username)
			b.WriteString("@")
		}
		b.WriteString(t.SSH.Host)
		if t.SSH.Port != 0 {
			fmt.Fprintf(&b, ":%d", t.SSH.Port)
		}
		p := t.SSH.RemoteFilePath
		if p == "" {
			p = t.Path
		}
		if p == "" {
			p = "/"
		}
		if !strings.HasPrefix(p, "/") {
			p = "/" + p
		}
		b.WriteString(p)
		return b.String()
	}
	if t.Path != "" {
		return t.Path
	}
	return ""
}

// persistSessionOnExit writes the tab bar when "save tabs on exit" is enabled.
func (m *model) persistSessionOnExit() {
	if m.cfg == nil || !m.cfg.SaveTabsExit {
		return
	}
	s := m.buildSavedSession()
	if len(s.Tabs) == 0 {
		return
	}
	if err := m.cfg.SaveSession(s); err != nil {
		// Best-effort: never block quit on session write failure.
		_ = err
	}
}

// restoreSessionTabs opens additional tabs from the shared saved session.
// The currently open file stays as tab 0; other saved paths are appended.
// Failures for individual tabs are skipped with a status note.
func (m *model) restoreSessionTabs() {
	if m.cfg == nil {
		m.status = "restore: no config"
		return
	}
	sess := m.cfg.GetSavedSession()
	if len(sess.Tabs) == 0 {
		m.status = "restore: no saved session"
		return
	}

	m.ensureSessionsInitialized()
	current := m.sessionTarget()
	opened := 0
	var skipped []string

	for _, tab := range sess.Tabs {
		target := targetFromSavedTab(tab)
		if target == "" {
			continue
		}
		// Skip if already open (same path/URL basename or exact match).
		if sessionAlreadyOpen(m.sessions, target, current) {
			continue
		}
		if err := m.openInNewTab(target); err != nil {
			skipped = append(skipped, filepath.Base(target))
			continue
		}
		// Apply group metadata from the saved session when present.
		if tab.GroupID != "" && len(m.sessions) > 0 {
			s := m.sessions[m.activeSession]
			s.groupID = tab.GroupID
			for _, g := range sess.Groups {
				if g.ID == tab.GroupID {
					s.groupTitle = g.Title
					s.groupColor = g.Color
					break
				}
			}
			m.tabGroupID = s.groupID
			m.tabGroupTitle = s.groupTitle
			m.tabGroupColor = s.groupColor
		}
		opened++
	}

	status := fmt.Sprintf("restored %d tab(s)", opened)
	if len(skipped) > 0 {
		status += fmt.Sprintf(" · skipped: %s", strings.Join(skipped, ", "))
	}
	if opened == 0 && len(skipped) == 0 {
		status = "restore: all saved tabs already open"
	}
	m.status = status
}

func sessionAlreadyOpen(sessions []*fileSession, target, current string) bool {
	norm := normalizeTarget(target)
	if normalizeTarget(current) == norm {
		return true
	}
	for _, s := range sessions {
		if s == nil {
			continue
		}
		if normalizeTarget(s.target) == norm {
			return true
		}
		if s.idx != nil && normalizeTarget(s.idx.FilePath()) == norm {
			return true
		}
	}
	return false
}

func normalizeTarget(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, shared.SSHScheme)
	s = strings.TrimPrefix(s, shared.FTPScheme)
	return filepath.Clean(s)
}

// restoreSessionMsg is emitted after a background restore of extra tabs (optional).
type restoreSessionMsg struct {
	opened  int
	skipped int
}

// cmdRestoreSession can be used as a tea.Cmd for deferred restore.
func cmdRestoreSession(m *model) tea.Cmd {
	return func() tea.Msg {
		before := len(m.sessions)
		m.restoreSessionTabs()
		return restoreSessionMsg{opened: len(m.sessions) - before}
	}
}
