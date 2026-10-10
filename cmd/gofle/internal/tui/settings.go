package tui

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

// Settings form field indices.
const (
	setFieldViewMode = iota
	setFieldWordWrap
	setFieldFollow
	setFieldCtxBefore
	setFieldCtxAfter
	setFieldNearbyMode
	setFieldNearbyWin
	setFieldExcludeRegex
	setFieldTimeReq
	setFieldTopLimit
	setFieldSearchCase
	setFieldSearchRegex
	setFieldSearchInvert
	setFieldSaveTabs
	setFieldHlPrimary
	setFieldHlAnd
	setFieldHlNearby
	setFieldHlExclude
	setFieldCount
)

var setFieldLabels = []string{
	"View mode",
	"Word wrap",
	"Default follow",
	"Context before",
	"Context after",
	"Nearby mode",
	"Nearby window",
	"Exclude as regex",
	"Require timestamp",
	"Top unique limit",
	"Default match-case",
	"Default regex",
	"Default invert",
	"Save tabs on exit",
	"Highlight primary",
	"Highlight AND",
	"Highlight nearby",
	"Highlight exclude",
}

var setFieldHints = []string{
	"normal | results | split",
	"y/n — wrap long lines (else ←→ scroll)",
	"y/n — tail on when opening a file",
	"default context lines before match (0–50)",
	"default context lines after match (0–50)",
	"before | after | either",
	"nearby window in lines (1–200)",
	"y/n — treat exclude patterns as regex by default",
	"y/n — drop lines without parseable timestamp",
	"max unique rows in Top (u)",
	"y/n — case-sensitive search by default",
	"y/n — regex search by default",
	"y/n — invert match by default",
	"y/n — persist open tabs to saved_session.json",
	"y/n — color primary search hits",
	"y/n — color secondary AND hits",
	"y/n — color nearby pattern hits",
	"y/n — color exclude patterns (often off)",
}

func newSettingsForm() []textinput.Model {
	forms := make([]textinput.Model, setFieldCount)
	for i := range forms {
		ti := textinput.New()
		ti.CharLimit = 32
		forms[i] = ti
	}
	return forms
}

func (m *model) fillSettingsForm() {
	if m.cfg == nil {
		return
	}
	if len(m.settingsForm) < setFieldCount {
		m.settingsForm = newSettingsForm()
	}
	vm := m.cfg.ViewMode
	if vm == "" {
		vm = "split"
	}
	m.settingsForm[setFieldViewMode].SetValue(vm)
	m.settingsForm[setFieldWordWrap].SetValue(boolYN(m.cfg.WordWrap))
	m.settingsForm[setFieldFollow].SetValue(boolYN(m.cfg.DefaultFollow))
	m.settingsForm[setFieldCtxBefore].SetValue(strconv.Itoa(m.cfg.CtxBefore))
	m.settingsForm[setFieldCtxAfter].SetValue(strconv.Itoa(m.cfg.CtxAfter))
	nm := m.cfg.NearbyMode
	if nm == "" {
		nm = "either"
	}
	m.settingsForm[setFieldNearbyMode].SetValue(nm)
	win := m.cfg.NearbyWindow
	if win <= 0 {
		win = 10
	}
	m.settingsForm[setFieldNearbyWin].SetValue(strconv.Itoa(win))
	m.settingsForm[setFieldExcludeRegex].SetValue(boolYN(m.cfg.ExcludeRegexDefault))
	m.settingsForm[setFieldTimeReq].SetValue(boolYN(m.cfg.TimeRequireStamp))
	top := m.cfg.TopLimit
	if top <= 0 {
		top = 100
	}
	m.settingsForm[setFieldTopLimit].SetValue(strconv.Itoa(top))
	m.settingsForm[setFieldSearchCase].SetValue(boolYN(m.cfg.DefaultSearchCase))
	m.settingsForm[setFieldSearchRegex].SetValue(boolYN(m.cfg.DefaultSearchRegex))
	m.settingsForm[setFieldSearchInvert].SetValue(boolYN(m.cfg.DefaultSearchInvert))
	m.settingsForm[setFieldSaveTabs].SetValue(boolYN(m.cfg.SaveTabsExit))
	m.settingsForm[setFieldHlPrimary].SetValue(boolYN(m.cfg.HlPrimary))
	m.settingsForm[setFieldHlAnd].SetValue(boolYN(m.cfg.HlSecondary))
	m.settingsForm[setFieldHlNearby].SetValue(boolYN(m.cfg.HlNearby))
	m.settingsForm[setFieldHlExclude].SetValue(boolYN(m.cfg.HlExclude))
}

func (m *model) openSettingsForm() tea.Cmd {
	if m.cfg == nil {
		m.status = "settings: no config loaded"
		return nil
	}
	m.fillSettingsForm()
	m.settingsMode = true
	m.settingsFocus = 0
	for i := range m.settingsForm {
		m.settingsForm[i].Blur()
	}
	return m.settingsForm[0].Focus()
}

func (m *model) applySettingsForm() error {
	if m.cfg == nil || len(m.settingsForm) < setFieldCount {
		return nil
	}
	vm := strings.ToLower(strings.TrimSpace(m.settingsForm[setFieldViewMode].Value()))
	switch vm {
	case "normal", "results", "split":
		m.cfg.ViewMode = vm
	default:
		return fmt.Errorf("view mode must be normal|results|split, got %q", vm)
	}
	m.cfg.DefaultFollow = parseYN(m.settingsForm[setFieldFollow].Value(), m.cfg.DefaultFollow)

	cb, err := parseOptionalInt(m.settingsForm[setFieldCtxBefore].Value())
	if err != nil || cb < 0 || cb > 50 {
		return fmt.Errorf("context before must be 0..50")
	}
	ca, err := parseOptionalInt(m.settingsForm[setFieldCtxAfter].Value())
	if err != nil || ca < 0 || ca > 50 {
		return fmt.Errorf("context after must be 0..50")
	}
	m.cfg.CtxBefore = cb
	m.cfg.CtxAfter = ca

	nm := strings.ToLower(strings.TrimSpace(m.settingsForm[setFieldNearbyMode].Value()))
	switch nm {
	case "before", "after", "either", "":
		if nm == "" {
			nm = "either"
		}
		m.cfg.NearbyMode = nm
	default:
		return fmt.Errorf("nearby mode must be before|after|either")
	}
	win, err := parseOptionalInt(m.settingsForm[setFieldNearbyWin].Value())
	if err != nil || win < 1 || win > 200 {
		return fmt.Errorf("nearby window must be 1..200")
	}
	m.cfg.NearbyWindow = win

	m.cfg.ExcludeRegexDefault = parseYN(m.settingsForm[setFieldExcludeRegex].Value(), m.cfg.ExcludeRegexDefault)
	m.cfg.TimeRequireStamp = parseYN(m.settingsForm[setFieldTimeReq].Value(), m.cfg.TimeRequireStamp)

	top, err := parseOptionalInt(m.settingsForm[setFieldTopLimit].Value())
	if err != nil || top < 1 || top > 10000 {
		return fmt.Errorf("top limit must be 1..10000")
	}
	m.cfg.TopLimit = top

	m.cfg.DefaultSearchCase = parseYN(m.settingsForm[setFieldSearchCase].Value(), m.cfg.DefaultSearchCase)
	m.cfg.DefaultSearchRegex = parseYN(m.settingsForm[setFieldSearchRegex].Value(), m.cfg.DefaultSearchRegex)
	m.cfg.DefaultSearchInvert = parseYN(m.settingsForm[setFieldSearchInvert].Value(), m.cfg.DefaultSearchInvert)
	m.cfg.SaveTabsExit = parseYN(m.settingsForm[setFieldSaveTabs].Value(), m.cfg.SaveTabsExit)
	m.cfg.HlPrimary = parseYN(m.settingsForm[setFieldHlPrimary].Value(), m.cfg.HlPrimary)
	m.cfg.HlSecondary = parseYN(m.settingsForm[setFieldHlAnd].Value(), m.cfg.HlSecondary)
	m.cfg.HlNearby = parseYN(m.settingsForm[setFieldHlNearby].Value(), m.cfg.HlNearby)
	m.cfg.HlExclude = parseYN(m.settingsForm[setFieldHlExclude].Value(), m.cfg.HlExclude)

	if err := m.cfg.Save(); err != nil {
		return fmt.Errorf("save settings: %w", err)
	}

	// Apply immediately to the live viewer.
	m.applyViewModeFromConfig()
	m.seedAdvancedDefaults()
	m.searchOpts.CaseSensitive = m.cfg.DefaultSearchCase
	m.searchOpts.IsRegex = m.cfg.DefaultSearchRegex
	m.searchOpts.InvertMatch = m.cfg.DefaultSearchInvert
	return nil
}

func (m *model) settingsFormKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	switch {
	case msg.Type == tea.KeyEsc || key == "ctrl+c":
		m.settingsMode = false
		return *m, nil
	case msg.Type == tea.KeyEnter:
		if err := m.applySettingsForm(); err != nil {
			m.status = "settings: " + err.Error()
			return *m, nil
		}
		m.settingsMode = false
		m.status = "settings saved"
		return *m, nil
	case msg.Type == tea.KeyTab || key == "down" || key == "j":
		m.settingsForm[m.settingsFocus].Blur()
		m.settingsFocus = (m.settingsFocus + 1) % setFieldCount
		return *m, m.settingsForm[m.settingsFocus].Focus()
	case key == "shift+tab" || key == "up" || key == "k":
		m.settingsForm[m.settingsFocus].Blur()
		m.settingsFocus = (m.settingsFocus + setFieldCount - 1) % setFieldCount
		return *m, m.settingsForm[m.settingsFocus].Focus()
	}
	var cmd tea.Cmd
	m.settingsForm[m.settingsFocus], cmd = m.settingsForm[m.settingsFocus].Update(msg)
	return *m, cmd
}

func (m model) settingsFormView() string {
	var b strings.Builder
	b.WriteString(headerStyle.Render("GoFLE  ·  settings"))
	b.WriteString("\n")
	b.WriteString(dimStyle.Render("Shared settings.json  ·  Tab/↑↓ move  ·  Enter save  ·  Esc cancel"))
	b.WriteString("\n\n")
	for i := 0; i < setFieldCount && i < len(m.settingsForm); i++ {
		label := fmt.Sprintf("%-18s", setFieldLabels[i]+":")
		if i == m.settingsFocus {
			b.WriteString(keyStyle.Render("▸ " + label))
			b.WriteString(m.settingsForm[i].View())
			b.WriteString("\n")
			b.WriteString(dimStyle.Render("    " + setFieldHints[i]))
			b.WriteString("\n")
		} else {
			b.WriteString(dimStyle.Render("  " + label))
			val := m.settingsForm[i].Value()
			if val == "" {
				b.WriteString(dimStyle.Render("(empty)"))
			} else {
				b.WriteString(val)
			}
			b.WriteString("\n")
		}
	}
	b.WriteString("\n")
	b.WriteString(infoStyle.Render("Changes apply to CLI and desktop (same config directory)."))
	return b.String()
}
