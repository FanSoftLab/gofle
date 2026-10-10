package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Palette of group colors (cycled when assigning a new group name).
var groupPalette = []string{
	"#89b4fa", // blue
	"#a6e3a1", // green
	"#f9e2af", // yellow
	"#cba6f7", // mauve
	"#fab387", // peach
	"#94e2d5", // teal
	"#f38ba8", // red
}

func newGroupInput() textinput.Model {
	ti := textinput.New()
	ti.Placeholder = "group name (empty = clear)"
	ti.CharLimit = 32
	return ti
}

func (m *model) openGroupEditor() tea.Cmd {
	m.groupMode = true
	if m.groupInput.Value() == "" && m.tabGroupTitle != "" {
		m.groupInput.SetValue(m.tabGroupTitle)
	}
	return m.groupInput.Focus()
}

func (m *model) applyGroupName(name string) {
	name = strings.TrimSpace(name)
	if name == "" {
		m.tabGroupID = ""
		m.tabGroupTitle = ""
		m.tabGroupColor = ""
		m.status = "group cleared"
		return
	}
	// Reuse color if another tab already has this group title.
	color := ""
	id := "grp-" + strings.ToLower(strings.ReplaceAll(name, " ", "-"))
	for _, s := range m.sessions {
		if s != nil && strings.EqualFold(s.groupTitle, name) {
			color = s.groupColor
			id = s.groupID
			break
		}
	}
	if color == "" {
		color = groupPalette[len(name)%len(groupPalette)]
	}
	m.tabGroupID = id
	m.tabGroupTitle = name
	m.tabGroupColor = color
	m.status = fmt.Sprintf("group: %s", name)
	// Keep sessions list in sync for the active tab.
	if len(m.sessions) > 0 && m.activeSession >= 0 && m.activeSession < len(m.sessions) {
		m.sessions[m.activeSession] = m.snapshotSession()
	}
}

func (m *model) groupEditorKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	switch {
	case msg.Type == tea.KeyEsc || key == "ctrl+c":
		m.groupMode = false
		return *m, nil
	case msg.Type == tea.KeyEnter:
		m.applyGroupName(m.groupInput.Value())
		m.groupMode = false
		return *m, nil
	}
	var cmd tea.Cmd
	m.groupInput, cmd = m.groupInput.Update(msg)
	return *m, cmd
}

func (m model) groupEditorView() string {
	var b strings.Builder
	b.WriteString(headerStyle.Render("GoFLE  ·  tab group"))
	b.WriteString("\n")
	b.WriteString(dimStyle.Render("Enter name to assign · empty clears · Esc cancel"))
	b.WriteString("\n\n")
	b.WriteString(searchStyle.Render("Group: ") + m.groupInput.View())
	b.WriteString("\n\n")
	if m.tabGroupTitle != "" {
		st := lipgloss.NewStyle().Foreground(lipgloss.Color(m.tabGroupColor))
		b.WriteString(st.Render("current: " + m.tabGroupTitle))
	}
	b.WriteString("\n")
	b.WriteString(dimStyle.Render("Tabs with the same name share a color in the tab strip."))
	return b.String()
}
