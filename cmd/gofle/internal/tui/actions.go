package tui

import (
	"GoFLE/cmd/gofle/internal/shared"
	"GoFLE/pkg/config"
	"GoFLE/pkg/remote"
	"GoFLE/pkg/search"
	"GoFLE/pkg/ssh"
	"bufio"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbletea"
)

// remoteListedMsg carries the result of an asynchronous remote directory listing.
type remoteListedMsg struct {
	path    string
	entries []remote.DirEntry
	err     error
}

// Key names used by more than a couple of the keymap handlers below. A keymap
// repeats the same spellings across many branches, so they live in one place: a
// typo in one literal would silently disable a binding.
const (
	keyEnter = "enter"
	keyEsc   = "esc"
	keyDown  = "down"
)

// statusClearMsg clears a transient status line after a short delay.
type statusClearMsg struct{ gen int }

// Update implements tea.Model.
func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	statusBefore := m.status

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.resize(msg)
		return m, nil

	case tickMsg:
		return m.followTick()

	case statusClearMsg:
		if msg.gen == m.statusGen {
			m.status = ""
		}

	case tea.KeyMsg:
		var next tea.Model
		next, cmd = m.handleKey(msg)
		m = next.(model)

	case remoteOpenedMsg:
		m.applyRemoteOpened(msg)

	case remoteListedMsg:
		m.applyRemoteListed(msg)

	case searchProgressMsg:
		cmd = m.applySearchProgress(msg)

	case searchDoneMsg:
		m.applySearchDone(msg)
	}

	// Footer lines (status, prompts, errors) can change on any update; keep
	// the body viewport height in sync so the panes never overflow into the
	// status/hint area.
	m.recomputeHeight()

	// Auto-dismiss transient status after 2.5s so the extra footer line does
	// not permanently steal body rows (especially noticeable in split view).
	if m.status != "" && m.status != statusBefore {
		m.statusGen++
		gen := m.statusGen
		clearCmd := tea.Tick(2500*time.Millisecond, func(time.Time) tea.Msg {
			return statusClearMsg{gen: gen}
		})
		return m, tea.Batch(cmd, clearCmd)
	}

	return m, cmd
}

// resize recomputes the viewport geometry and the prompt widths for a new
// terminal size.
func (m *model) resize(msg tea.WindowSizeMsg) {
	m.width = msg.Width
	m.fullHeight = msg.Height
	m.recomputeHeight()
	m.searchInput.Width = msg.Width - 18
	m.gotoInput.Width = msg.Width - 14
	m.exportInput.Width = msg.Width - 10
}

// handleKey routes a key press to the handler that owns the current mode. The
// modal overlays (help, the prompts, the pickers, the remote form/browser)
// consume input exclusively; anything else falls through to navigation.
func (m model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case m.help:
		m.helpKey(msg)

		return m, nil
	case m.advMode:
		return m.advancedFormKey(msg)
	case m.settingsMode:
		return m.settingsFormKey(msg)
	case m.groupMode:
		return m.groupEditorKey(msg)
	case m.topMode:
		return m.topUniqueKey(msg)
	case m.presetMode:
		return m.presetKey(msg)
	case m.searchMode || m.secondaryMode:
		return m.searchPromptKey(msg)
	case m.gotoMode:
		return m.gotoPromptKey(msg)
	case m.exportConfirm:
		m.exportConfirmKey(msg)

		return m, nil
	case m.exportMode:
		return m.exportPromptKey(msg)
	case m.recentMode:
		return m.recentPickerKey(msg)
	case m.remoteMode:
		return m.remoteFormKey(msg)
	case m.remoteBrowseMode:
		return m.remoteBrowseKey(msg)
	}

	return m.navKey(msg)
}

// helpKey handles input while the help overlay is open. The overlay is modal:
// only the keys that close it do anything.
func (m *model) helpKey(msg tea.KeyMsg) {
	switch msg.String() {
	case "?", keyEsc, "q", "ctrl+c":
		m.help = false
	}
}

// searchPromptKey handles input while the primary search box or the AND-filter
// prompt is focused: enter runs the search, esc cancels, tab swaps the two
// prompts and Alt+R/I/C toggle the search options. Every other key edits the
// focused input.
func (m *model) searchPromptKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	active := &m.searchInput
	if m.secondaryMode {
		active = &m.secondaryInput
	}

	switch {
	case msg.Type == tea.KeyEnter:
		m.searchMode = false
		m.secondaryMode = false
		return *m, m.startSearchCmd()
	case msg.Type == tea.KeyEsc:
		m.searchMode = false
		m.secondaryMode = false

		return *m, nil
	case msg.Type == tea.KeyTab:
		return m.swapSearchPrompt()
	case msg.Alt && len(msg.Runes) == 1:
		m.toggleSearchOption(msg.Runes[0])

		return *m, nil
	}

	var cmd tea.Cmd

	ni, upd := active.Update(msg)
	*active = ni
	_ = upd

	return *m, cmd
}

// swapSearchPrompt moves focus between the primary search box and the AND-filter
// prompt without either losing its text.
func (m *model) swapSearchPrompt() (tea.Model, tea.Cmd) {
	if m.secondaryMode {
		m.secondaryMode = false
		m.searchMode = true

		cmd := m.searchInput.Focus()

		return *m, cmd
	}

	m.searchMode = false
	m.secondaryMode = true

	cmd := m.secondaryInput.Focus()

	return *m, cmd
}

// toggleSearchOption applies one Alt+<r|i|c> search-option toggle.
func (m *model) toggleSearchOption(r rune) {
	switch r {
	case 'r', 'R':
		m.searchOpts.IsRegex = !m.searchOpts.IsRegex
	case 'i', 'I':
		m.searchOpts.InvertMatch = !m.searchOpts.InvertMatch
	case 'c', 'C':
		m.searchOpts.CaseSensitive = !m.searchOpts.CaseSensitive
	}
}

// gotoPromptKey handles input while the goto-line prompt is focused.
func (m *model) gotoPromptKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case keyEnter:
		m.runGoto()
		m.gotoMode = false

		return *m, nil
	case keyEsc:
		m.gotoMode = false

		return *m, nil
	}

	var cmd tea.Cmd

	m.gotoInput, cmd = m.gotoInput.Update(msg)

	return *m, cmd
}

// exportConfirmKey answers the "the directory does not exist, create it?"
// confirmation raised by an export path with a missing parent.
func (m *model) exportConfirmKey(msg tea.KeyMsg) {
	switch msg.String() {
	case "y", "Y":
		m.createExportDir()
	case "n", "N", keyEsc:
		m.exportConfirm = false
		m.exportTarget = ""
		m.status = "export cancelled"
	}
}

// createExportDir creates the confirmed export directory and writes the export
// into it, reporting a mkdir failure instead of exporting.
func (m *model) createExportDir() {
	dir := filepath.Dir(m.exportTarget)
	if mkErr := os.MkdirAll(dir, 0o750); mkErr != nil {
		m.status = fmt.Sprintf("failed to create directory: %v", mkErr)
	} else {
		m.doExport(m.exportTarget)
	}

	m.exportConfirm = false
	m.exportTarget = ""
}

// exportPromptKey handles input while the export-path prompt is focused.
func (m *model) exportPromptKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case keyEnter:
		if resolved := m.submitExportPath(); resolved {
			return *m, nil
		}
	case keyEsc:
		m.exportMode = false
	}

	var cmd tea.Cmd

	m.exportInput, cmd = m.exportInput.Update(msg)

	return *m, cmd
}

// submitExportPath resolves the typed export path and then either exports right
// away, asks for confirmation to create a missing directory, or reports a bad
// path. resolved is true when the prompt is finished and the key must not also
// reach the text input.
func (m *model) submitExportPath() (resolved bool) {
	abs, needCreate, perr := resolveExportPath(m.exportInput.Value())
	if perr != nil {
		m.status = fmt.Sprintf("export: %v", perr)
		m.exportMode = false

		return true
	}

	if needCreate {
		m.exportConfirm = true
		m.exportTarget = abs
		m.exportMode = false

		return true
	}

	m.doExport(abs)
	m.exportMode = false

	return false
}

// recentPickerKey handles input in the in-viewer recent-files picker overlay.
func (m *model) recentPickerKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		if len(m.recentList) > 0 {
			m.recentCursor = (m.recentCursor + len(m.recentList) - 1) % len(m.recentList)
		}
	case keyDown, "j":
		if len(m.recentList) > 0 {
			m.recentCursor = (m.recentCursor + 1) % len(m.recentList)
		}
	case keyEnter:
		if m.openRecentAsTab {
			m.openSelectedRecentAsTab()
		} else {
			m.openSelectedRecent()
		}
		m.openRecentAsTab = false
	case "t":
		// Open selected recent entry in a new tab (keep current file open).
		m.openSelectedRecentAsTab()
		m.openRecentAsTab = false
	case "b":
		// Browse another file on the same remote server (SSH/FTP recent only).
		if len(m.recentList) == 0 {
			return *m, nil
		}
		item := m.recentList[m.recentCursor]
		if !item.IsSSH || item.SSH == nil {
			m.status = "not a remote entry — open only"
			return *m, nil
		}
		m.recentMode = false
		m.recentList = nil
		return *m, m.browseFromRecent(item)
	case keyEsc:
		m.recentMode = false
		m.recentList = nil
	}
	return *m, nil
}

// openSelectedRecent opens the highlighted recent entry in place and closes the
// picker. A failed open is reported in the status line.
func (m *model) openSelectedRecent() {
	if len(m.recentList) == 0 {
		return
	}

	item := m.recentList[m.recentCursor]

	target := item.Path
	if item.IsSSH && item.SSH != nil {
		target = remoteTargetFromRecent(item)
	}

	if rerr := m.reopen(target); rerr != nil {
		m.status = rerr.Error()
	}

	m.recentMode = false
	m.recentList = nil
}

// navKey handles keys in normal navigation mode. The bindings are grouped into
// small handlers (results pane, prompts, scrolling, view modes, match
// navigation, selection) that each report whether they owned the key, so no
// single function carries the whole keymap.
func (m *model) navKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	if m.searching && (key == keyEsc || key == "ctrl+c") {
		if m.searchCancel != nil {
			m.searchCancel()
			m.searchCancel = nil
		}
		m.searching = false
		m.status = "search cancelled"
		return *m, nil
	}
	if key == "q" || key == "ctrl+c" {
		// RunView tears the target down asynchronously after the program has
		// left the alternate screen. Doing it here on the event loop would
		// block: Streamer.Close waits for the tail supervisor to release a
		// possibly stuck SSH connection, which made the app look like it
		// never closes on open SSH logs.
		if m.cancel != nil {
			m.cancel()
		}

		return *m, tea.Quit
	}

	// Tab / Shift+Tab switch split panes — handle before other routes so the
	// key is never swallowed by scroll/match handlers. Use KeyTab type as well
	// as the "tab" string (terminals differ).
	if m.effectiveSplit() && (msg.Type == tea.KeyTab || key == "tab" || key == "shift+tab") {
		if key == "shift+tab" {
			// Cycle opposite direction (same 2-pane toggle in practice).
			m.splitFocus ^= 1
		} else {
			m.splitFocus ^= 1
		}
		if m.splitFocus == 1 {
			m.status = "pane: RESULTS  (↑↓ matches · Enter → log)"
		} else {
			m.status = "pane: LOG  (↑↓ scroll · Tab → results)"
		}
		return *m, nil
	}

	if m.resultsPaneKey(key) {
		return *m, nil
	}

	if key == "ctrl+w" {
		return *m, m.closeActiveSession()
	}

	if m.tabKey(key) {
		return *m, nil
	}

	if cmd, ok := m.promptKey(key); ok {
		return *m, cmd
	}

	if m.scrollKey(key) || m.viewModeKey(key) || m.matchKey(key) || m.selectionKey(key) {
		return *m, nil
	}

	if key == "?" {
		m.help = true
	}

	return *m, nil
}

// resultsPaneKey moves the selected match (and lets the log pane follow) while
// the results pane owns focus in split view. It reports false when that pane is
// not focused or the key is not one it owns, so the caller falls through to the
// ordinary navigation keys.
func (m *model) resultsPaneKey(key string) bool {
	// Only when the results pane is actually visible and focused.
	if !m.effectiveSplit() || m.splitFocus != 1 {
		return false
	}

	switch key {
	case "up", "k":
		m.stepMatch(-1)
		m.keepMatchVisible()
	case keyDown, "j":
		m.stepMatch(1)
		m.keepMatchVisible()
	case "pgup", "ctrl+u":
		m.matchCursor = max(0, m.matchCursor-m.height)
		m.keepMatchVisible()
		m.ensureResVisible()
	case "pgdown", "pgdn", "ctrl+d":
		m.matchCursor = min(len(m.matches)-1, m.matchCursor+m.height)
		m.keepMatchVisible()
		m.ensureResVisible()
	case "home":
		m.gotoMatch(0)
		m.keepMatchVisible()
	case "end", "G":
		m.gotoMatch(len(m.matches) - 1)
		m.keepMatchVisible()
	case keyEnter:
		// Jump back to the log pane with the selected match at the top.
		// Reset topWrap so no stale wrapped-row offset hides the first
		// visual rows of the target line.
		m.splitFocus = 0
		if !m.onlyMatches {
			m.top = m.matches[m.matchCursor]
			m.topWrap = 0
			m.clampViewport()
		}
	default:
		return false
	}

	return true
}

// promptKey opens one of the modal prompts or overlays (search, AND filter,
// goto, export, remote connect form, recent picker). ok is false when key opens
// none of them.
func (m *model) promptKey(key string) (cmd tea.Cmd, ok bool) {
	switch key {
	case "/":
		m.searchMode = true
		// Pre-fill with the applied query: opening the box is an EDIT, not
		// a reset — retyping the whole filter was the complaint.
		m.searchInput.SetValue(m.query)

		return m.searchInput.Focus(), true
	case "\\":
		m.secondaryMode = true
		m.secondaryInput.SetValue(m.secondaryQuery)

		return m.secondaryInput.Focus(), true
	case "g":
		m.gotoMode = true
		m.gotoInput.SetValue("")

		return m.gotoInput.Focus(), true
	case "e":
		m.exportMode = true
		m.exportInput.SetValue(defaultExportName())
		m.exportInput.CursorEnd()

		return m.exportInput.Focus(), true
	case "S":
		m.enterRemoteMode()

		return nil, true
	case "o":
		m.openRecentAsTab = false
		m.openRecentPicker()

		return nil, true
	case "T":
		// Open recent list with intent to open selection in a new tab.
		m.openRecentAsTab = true
		m.openRecentPicker()
		if m.recentMode {
			m.status = "recent → new tab (Enter or t)"
		}
		return nil, true
	case "b":
		// Browse another file on the same remote server (SSH/FTP).
		cmd := m.browseSameServer()
		return cmd, true
	case "A":
		// Advanced filters form (GUI parity: Nearby, Exclude, Levels, Time, Range, Context).
		return m.openAdvancedForm(), true
	case "f2", ",":
		// Settings (shared settings.json). Comma is a fallback when F2 is captured by the terminal.
		return m.openSettingsForm(), true
	case "Y":
		// Restore additional tabs from shared saved session.
		m.restoreSessionTabs()
		return nil, true
	case "G":
		// Assign / clear tab group for the active tab.
		return m.openGroupEditor(), true
	case "u":
		m.openTopUnique()
		return nil, true
	case "P":
		m.openPresetPicker()
		return nil, true
	}

	return nil, false
}

// openRecentPicker loads the recent list and opens the picker overlay, or
// reports that there is nothing to pick from.
func (m *model) openRecentPicker() {
	if m.cfg != nil {
		m.recentList = m.cfg.GetRecentFiles()
	}

	if len(m.recentList) == 0 {
		m.status = "no recent files"

		return
	}

	m.recentMode = true
	m.recentCursor = 0
}

// scrollKey handles the viewport-scrolling keys. In split view the results pane
// is re-synced afterwards so the two panes stay on the same region.
//
// Vim-style j/k/G and both "pgdown"/"pgdn" spellings are accepted so the
// bindings stay consistent with the results pane and recent/remote pickers.
// ctrl+u / ctrl+d do half-page jumps (same as page up/down here).
func (m *model) scrollKey(key string) bool {
	switch key {
	case "up", "k":
		m.scrollUp()
	case keyDown, "j":
		m.scrollDown()
	case "pgup", "ctrl+u":
		m.pageUp()
	case "pgdown", "pgdn", "ctrl+d":
		m.pageDown()
	case "home":
		m.scrollToTop()
	case "end", "G":
		m.scrollToEnd()
	case "left", "h":
		m.scrollLeft()
	case "right", "l":
		m.scrollRight()
	default:
		return false
	}

	m.clampViewport()
	if m.splitView {
		m.syncMatchToView()
	}

	return true
}

// scrollUp moves the viewport up one screen row (visual row when word-wrap is on).
func (m *model) scrollUp() {
	if m.onlyMatches {
		m.top = max(0, m.top-1)
		m.topWrap = 0
		return
	}
	if m.wordWrap {
		if m.topWrap > 0 {
			m.topWrap--
			return
		}
		if m.top > 0 {
			m.top--
			m.topWrap = max(0, m.wrapRows(m.top)-1)
		}
		return
	}
	m.topWrap = 0
	if m.top > 0 {
		m.top--
	}
}

// scrollDown moves the viewport down one screen row.
func (m *model) scrollDown() {
	last := max(m.lastLine(), 0)
	if m.onlyMatches {
		m.top = min(m.top+1, last)
		m.topWrap = 0
		return
	}
	if m.wordWrap {
		// Stop once the remaining content no longer fills the viewport.
		if m.remainingVisualRows() <= m.height {
			return
		}
		wr := m.wrapRows(m.top)
		if wr < 1 {
			wr = 1
		}
		if m.topWrap < wr-1 {
			m.topWrap++
			return
		}
		if m.top < last {
			m.top++
			m.topWrap = 0
		}
		return
	}
	m.topWrap = 0
	maxTop := m.viewableEnd()
	if m.top < maxTop {
		m.top++
	}
}

// pageUp scrolls up roughly one screenful of visual rows.
func (m *model) pageUp() {
	if m.onlyMatches || !m.wordWrap {
		m.top = max(0, m.top-int64(m.height))
		m.topWrap = 0
		return
	}
	steps := m.height
	for steps > 0 && (m.top > 0 || m.topWrap > 0) {
		m.scrollUp()
		steps--
	}
}

// pageDown scrolls down roughly one screenful of visual rows.
func (m *model) pageDown() {
	if m.onlyMatches || !m.wordWrap {
		last := max(m.viewableEnd(), 0)
		m.top = min(m.top+int64(m.height), last)
		m.topWrap = 0
		return
	}
	steps := m.height
	for steps > 0 {
		prevTop, prevWrap := m.top, m.topWrap
		m.scrollDown()
		if m.top == prevTop && m.topWrap == prevWrap {
			break
		}
		steps--
	}
}

// scrollToTop jumps to the first line (first match in results-only mode).
func (m *model) scrollToTop() {
	m.top = 0
	m.topWrap = 0
	m.hOffset = 0
}

// scrollToEnd jumps to the last screenful (last match in results-only mode).
func (m *model) scrollToEnd() {
	if m.onlyMatches {
		m.top = max(m.viewableEnd(), 0)
		m.topWrap = 0
		return
	}
	if m.wordWrap {
		m.top = m.viewableEndWrapped()
		m.topWrap = 0
		// Advance topWrap so the last visual row sits near the bottom.
		for m.remainingVisualRows() > m.height {
			wr := m.wrapRows(m.top)
			if m.topWrap < wr-1 {
				m.topWrap++
			} else {
				break
			}
		}
		m.clampViewport()
		return
	}
	m.top = max(m.viewableEnd(), 0)
	m.topWrap = 0
}

// scrollLeft moves the horizontal viewport left (no-op when word-wrap is on).
func (m *model) scrollLeft() {
	if m.wordWrap {
		return
	}
	step := 8
	if m.hOffset < step {
		m.hOffset = 0
	} else {
		m.hOffset -= step
	}
}

// scrollRight moves the horizontal viewport right (no-op when word-wrap is on).
func (m *model) scrollRight() {
	if m.wordWrap {
		return
	}
	m.hOffset += 8
}

// toggleWordWrap switches soft-wrap ↔ horizontal scroll and persists the preference.
func (m *model) toggleWordWrap() {
	m.wordWrap = !m.wordWrap
	m.topWrap = 0
	m.hOffset = 0
	if m.wordWrap {
		m.status = "word wrap ON"
	} else {
		m.status = "word wrap OFF · ←→ horizontal scroll"
	}
	if m.cfg != nil {
		m.cfg.Mu.Lock()
		m.cfg.WordWrap = m.wordWrap
		m.cfg.Mu.Unlock()
		m.cfg.SaveDebounced()
	}
}

// viewModeKey handles the keys that change how the log is presented: follow/tail,
// the mode cycle, split view, the results-only filter and the pane focus.
func (m *model) viewModeKey(key string) bool {
	switch key {
	case "f":
		m.toggleTail()
	case "w":
		m.toggleWordWrap()
	case "m":
		m.cycleViewMode()
		m.persistViewMode()
	case "s":
		m.toggleSplitView()
		m.persistViewMode()
	case "r":
		m.toggleOnlyMatches()
		m.persistViewMode()
	case "tab", "shift+tab":
		// Prefer navKey's Tab handler (effectiveSplit + status feedback).
		// Keep a fallback here in case viewModeKey is reached first.
		if m.effectiveSplit() {
			m.splitFocus ^= 1
			if m.splitFocus == 1 {
				m.status = "pane: RESULTS  (↑↓ matches · Enter → log)"
			} else {
				m.status = "pane: LOG  (↑↓ scroll · Tab → results)"
			}
		}
	default:
		return false
	}

	return true
}

// effectiveSplit is true when split mode is preferred AND there are matches
// for the right pane. Without matches we keep a full-width log (no divider).
func (m model) effectiveSplit() bool {
	// Split preference is independent of whether a search has hits; an empty
	// results pane shows a placeholder (see resultRows).
	return m.splitView
}

// applyViewModeFromConfig sets splitView / onlyMatches from the shared
// settings.json field view_mode (used by both CLI and desktop).
func (m *model) applyViewModeFromConfig() {
	mode := "split"
	if m.cfg != nil {
		m.cfg.Mu.RLock()
		if m.cfg.ViewMode != "" {
			mode = m.cfg.ViewMode
		}
		m.cfg.Mu.RUnlock()
	}
	switch mode {
	case "results":
		m.onlyMatches = true
		m.splitView = false
	case "normal":
		m.onlyMatches = false
		m.splitView = false
	default: // "split" and anything unknown
		m.onlyMatches = false
		m.splitView = true // preference; actual layout uses effectiveSplit()
	}
}

// persistViewMode writes the current layout into the shared AppConfig so the
// next CLI (or desktop) session restores the same mode.
func (m *model) persistViewMode() {
	if m.cfg == nil {
		return
	}
	mode := "normal"
	switch {
	case m.splitView:
		mode = "split"
	case m.onlyMatches:
		mode = "results"
	}
	m.cfg.Mu.Lock()
	m.cfg.ViewMode = mode
	m.cfg.Mu.Unlock()
	// Debounced so rapid m/s/r toggles don't thrash settings.json.
	m.cfg.SaveDebounced()
}

// toggleTail switches follow/tail mode, jumping to the end of the view when it
// is enabled.
func (m *model) toggleTail() {
	m.tailOn = !m.tailOn
	if !m.tailOn {
		return
	}

	m.top = max(m.viewableEnd(), 0)
	if !m.onlyMatches {
		m.topWrap = 0
	}
}

// cycleViewMode cycles normal → results-only → split → normal.
func (m *model) cycleViewMode() {
	switch {
	case m.splitView:
		// split → normal
		m.splitView = false
		m.splitFocus = 0
		m.onlyMatches = false
		m.status = tr(*m, "mode_normal")
	case m.onlyMatches:
		// results-only → split (requires matches)
		m.enableSplitView()
	default:
		// normal → results-only (requires matches)
		m.enableOnlyMatches()
	}
}

// toggleSplitView is the direct split-view toggle ('s'). Unlike the mode cycle
// it leaves the results-only filter untouched when switching split off.
// Enabling split without any matches is refused — there would be nothing to show
// in the results pane.
func (m *model) toggleSplitView() {
	if m.splitView {
		m.splitView = false
		m.splitFocus = 0
		m.status = tr(*m, "mode_normal")

		return
	}
	if len(m.matches) == 0 {
		m.status = tr(*m, "no_matches_mode")

		return
	}

	m.enableSplitView()
}

// enableSplitView turns on the two-pane log|results layout.
// Matches are optional — without them the results pane shows a placeholder.
func (m *model) enableSplitView() {
	m.onlyMatches = false
	m.splitView = true
	m.splitFocus = 0
	if len(m.matches) > 0 {
		m.ensureResVisible()
		m.keepMatchVisible()
	}
	m.status = tr(*m, "mode_split")
}

// enableOnlyMatches turns on the results-only filter view, which likewise needs
// an active search.
func (m *model) enableOnlyMatches() {
	if len(m.matches) == 0 {
		m.status = tr(*m, "no_matches_mode")

		return
	}

	m.onlyMatches = true
	m.matchCursor = 0
	m.top = 0
	m.status = tr(*m, "mode_results")
}

// toggleOnlyMatches toggles the results-only filter view, translating the
// current line position into a match index when switching into it.
// Enabling without an active search degrades to the full log and reports it
// via the status line instead of refusing.
func (m *model) toggleOnlyMatches() {
	if m.onlyMatches {
		m.onlyMatches = false
		m.status = tr(*m, "mode_normal")

		return
	}
	if len(m.matches) == 0 {
		m.onlyMatches = true
		m.matchCursor = 0
		m.top = 0
		m.status = tr(*m, "no_matches_hint")

		return
	}
	// Convert the current line position into a match index.
	i := sort.Search(len(m.matches), func(i int) bool { return m.matches[i] >= m.top })
	if i >= len(m.matches) {
		i = len(m.matches) - 1
	}

	m.onlyMatches = true
	m.matchCursor = i
	m.top = int64(i)
	m.status = tr(*m, "mode_results")
}

// matchKey handles match navigation: next/previous and first/last.
func (m *model) matchKey(key string) bool {
	switch key {
	case "n", "f3":
		m.stepMatch(1)
	case "p", "N", "shift+f3":
		m.stepMatch(-1)
	case "[":
		m.gotoMatch(0)
	case "]":
		m.gotoMatch(len(m.matches) - 1)
	default:
		return false
	}

	return true
}

// selectionKey handles the selection/copy keys plus the escape that clears the
// selection or the active search.
func (m *model) selectionKey(key string) bool {
	switch key {
	case "v":
		m.toggleSelection()
	case "c":
		m.copySelection()
	case "x":
		m.clearSearch()
	case keyEsc:
		m.escapeKey()
	default:
		return false
	}

	return true
}

// toggleSelection starts or clears the visual line selection. Anchoring at m.top
// and letting the moving end be m.top means every scroll extends or shrinks the
// range.
func (m *model) toggleSelection() {
	if m.selActive {
		m.selActive = false
		m.status = "selection cleared"

		return
	}

	m.selActive = true
	m.selAnchor = m.top
	m.status = "selecting · scroll to extend · c copy · v/esc cancel"
}

// escapeKey clears an active selection, or the active search when nothing is
// selected.
func (m *model) escapeKey() {
	if m.selActive {
		m.selActive = false
		m.status = "selection cleared"

		return
	}
	// Clear search if active and no other modal state.
	if m.query != "" || m.secondaryQuery != "" || len(m.matches) > 0 {
		m.clearSearch()
	}
}

// applyRemoteOpened swaps the viewer onto a remote target that the async open
// finished, or surfaces the trust prompt / error it produced instead.
func (m *model) applyRemoteOpened(msg remoteOpenedMsg) {
	m.remoteOpening = false

	if msg.hkErr != nil {
		m.remoteHostPrompt = msg.hkErr
		m.remoteAfterTrustOpen = true
		m.remoteBrowseErr = ""

		return
	}

	if msg.err != nil {
		m.remoteBrowseErr = msg.err.Error()

		return
	}

	m.closeCurrentTarget()
	m.idx = msg.idx
	m.se = msg.se
	m.cleanup = msg.cleanup
	m.resetViewerState()
	m.remoteBrowseMode = false
	m.remoteMode = false
	m.remoteHostPrompt = nil
	m.remoteBrowseErr = ""
	cfg := msg.cfg
	m.lastRemoteCfg = &cfg
	m.remoteProto = cfg.Protocol
	if m.remoteProto == "" {
		m.remoteProto = remote.ProtocolSSH
	}
	m.status = msg.status
}

// applyRemoteListed stores a finished directory listing, or surfaces the
// unknown-host trust prompt / error the listing failed with.
func (m *model) applyRemoteListed(msg remoteListedMsg) {
	m.remoteLoading = false

	if msg.err != nil {
		// An unknown host key surfaces a trust prompt instead of a bare error.
		var hk *ssh.HostKeyUnknownError
		if errors.As(msg.err, &hk) {
			m.remoteHostPrompt = hk
			m.remoteAfterTrustOpen = false // listing is always a browse context
			m.remoteBrowseErr = ""
		} else {
			m.remoteBrowseErr = msg.err.Error()
		}

		return
	}

	m.remoteHostPrompt = nil
	m.remoteBrowseErr = ""
	// Both backends' ListDir return entries already sorted (see
	// pkg/ssh and pkg/ftp), so no re-sort is needed here.
	m.remoteEntries = msg.entries
	m.remoteCursor = 0
	m.remoteCurrentPath = msg.path
}

// clearSearch clears the search query, matches, secondary filter, and advanced filters.
func (m *model) clearSearch() {
	m.query = ""
	m.secondaryQuery = ""
	m.searchInput.SetValue("")
	m.secondaryInput.SetValue("")
	m.matches = nil
	m.primaryMatches = nil
	m.primaryKey = ""
	m.matchCursor = 0
	m.searchOpts = search.SearchOptions{CaseSensitive: false}
	m.clearAdvancedFilters()
	m.onlyMatches = false
	m.splitFocus = 0
	// Keep split preference if user had split on; results-only drops to normal/split preference.
	if m.cfg != nil && m.cfg.ViewMode == "results" {
		m.splitView = false
	}
	// else leave m.splitView as-is so 's' state survives a search clear
	m.top = 0
	m.status = "search cleared"
}

// searchPrimaryKey returns a stable identity for the primary portion of opts —
// query text plus the regex/case/invert flags, excluding the secondary AND
// filter and advanced post-filters. runSearch compares it across calls to
// detect whether only the secondary filter changed (H2).
func searchPrimaryKey(opts search.SearchOptions) string {
	return fmt.Sprintf("%v|%v|%v|%v", opts.Query, opts.IsRegex, opts.CaseSensitive, opts.InvertMatch)
}

// applyMatchResult stores a freshly computed match set and positions the view on
// the first match, keeping matchCursor/top/status bookkeeping in one place.
func (m *model) applyMatchResult(matches []int64) {
	// Clone the returned slice so m.matches owns its backing array: the engine
	// no longer retains results (stateless), but the TUI later appends to
	// m.matches on live-tail extension, and an owned copy keeps that growth
	// from ever aliasing another holder of the same array.
	m.matches = slices.Clone(matches)

	m.matchCursor = 0
	if m.onlyMatches {
		m.top = 0
	} else if len(matches) > 0 {
		m.top = matches[0]
	}

	m.topWrap = 0
	if len(matches) > 0 {
		sum := m.advancedSummary()
		if sum != "" {
			m.status = fmt.Sprintf("found %d matches · %s", len(matches), sum)
		} else {
			m.status = fmt.Sprintf("found %d matches", len(matches))
		}
	} else {
		m.status = "no matches"
	}
}

// runSearch executes the current search-box query and jumps to the first match.
// Pipeline mirrors the GUI (app_tabs.SearchLines):
//  1. primary scan (optional LineStart/LineEnd range, MatchAll when only advanced)
//  2. secondary AND via FilterLines
//  3. ApplyAdvancedFilters (exclude / nearby / levels / time)
//  4. optional ExpandMatchesWithContext for display
//
// When only the secondary AND-filter changed and advanced filters are unchanged,
// it refines the cached primary matches in O(matches) instead of O(totalLines).
func (m *model) runSearch() {
	m.query = m.searchInput.Value()
	m.secondaryQuery = strings.TrimSpace(m.secondaryInput.Value())

	opts := m.effectiveSearchOptions()
	hasAdv := m.hasAdvancedFilters()

	if opts.Query == "" && !hasAdv && opts.SecondaryQuery == "" {
		m.matches = nil
		m.primaryMatches = nil
		m.primaryKey = ""
		m.matchCursor = 0
		return
	}

	// Empty primary + advanced (and/or secondary) → MatchAll indices, then post-filter.
	if opts.Query == "" {
		opts.MatchAll = true
		opts.IsRegex = false
		opts.InvertMatch = false
	}

	key := searchPrimaryKey(opts)
	// secondaryOnly refine only when advanced filters are stable (same key path as GUI).
	secondaryOnly := m.primaryMatches != nil && key == m.primaryKey &&
		!hasAdv && opts.SecondaryQuery != ""

	var res []int64
	var err error

	if secondaryOnly {
		res, err = m.se.FilterLines(m.ctx, opts, m.primaryMatches)
		if err != nil {
			m.err = err
			return
		}
		if m.hasContextExpand() {
			res = search.ExpandMatchesWithContext(res, m.advCtxBefore, m.advCtxAfter, m.idx.TotalLines())
		}
		m.applyMatchResult(res)
		return
	}

	primaryOpts := opts
	primaryOpts.SecondaryQuery = ""

	if opts.LineStart > 0 || opts.LineEnd > 0 {
		end := opts.LineEnd
		if end <= 0 {
			end = m.idx.TotalLines()
		}
		res, err = m.se.SearchRange(m.ctx, primaryOpts, opts.LineStart, end)
	} else {
		res, err = m.se.SearchWithContext(m.ctx, primaryOpts)
	}
	if err != nil {
		m.err = err
		return
	}

	m.primaryMatches = slices.Clone(res)
	m.primaryKey = key

	if opts.SecondaryQuery != "" {
		res, err = m.se.FilterLines(m.ctx, opts, res)
		if err != nil {
			m.err = err
			return
		}
	}

	// Advanced post-filters live in pkg/search — TUI never reimplements them.
	res, err = m.se.ApplyAdvancedFilters(m.ctx, opts, res)
	if err != nil {
		m.err = err
		return
	}

	if m.hasContextExpand() {
		res = search.ExpandMatchesWithContext(res, m.advCtxBefore, m.advCtxAfter, m.idx.TotalLines())
	}

	m.applyMatchResult(res)
}

// stepMatch moves the match cursor by dir (+1 forward, -1 backward) and scrolls
// the view to the new current match. In results-only mode m.top is the match
// index; otherwise it is the matched line number.
func (m *model) stepMatch(dir int) {
	if len(m.matches) == 0 {
		return
	}

	m.matchCursor = (m.matchCursor + dir) % len(m.matches)
	if m.matchCursor < 0 {
		m.matchCursor += len(m.matches)
	}

	if m.onlyMatches {
		m.top = int64(m.matchCursor)
	} else {
		m.top = m.matches[m.matchCursor]
		m.keepMatchVisible()
	}

	m.ensureResVisible()
}

// gotoMatch jumps the match cursor to logical match index i.
func (m *model) gotoMatch(i int) {
	if len(m.matches) == 0 {
		return
	}

	if i < 0 {
		i = 0
	}

	if i > len(m.matches)-1 {
		i = len(m.matches) - 1
	}

	m.matchCursor = i
	if m.onlyMatches {
		m.top = int64(i)
	} else {
		m.top = m.matches[i]
		m.keepMatchVisible()
	}

	m.ensureResVisible()
}

// runGoto jumps to the (1-based) line number typed in the goto prompt.
func (m *model) runGoto() {
	s := strings.TrimSpace(m.gotoInput.Value())

	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		m.status = fmt.Sprintf("invalid line number: %q", s)
		return
	}

	n-- // to 0-based
	if n < 0 {
		n = 0
	}

	total := m.idx.TotalLines()
	if n >= total {
		n = total - 1
	}

	if n < 0 {
		n = 0
	}

	m.top = n
	// Drop any stale wrapped-row offset and clamp so the target line is the
	// first visible row in both wrap and non-wrap modes.
	m.topWrap = 0
	m.clampViewport()
}

// copySelection copies the selected line range (inclusive) to the system
// clipboard via the OSC 52 escape sequence. The full (untruncated) line content
// is copied, not the viewport-clipped rendering. A successful copy clears the
// selection and reports the line count. In results-only mode m.top/anchor are
// match indices, so the range maps through m.matches to the real line numbers.
func (m *model) copySelection() {
	lo, hi, ok := m.selectionBounds()
	if !ok {
		m.status = "no selection — press v, scroll to extend, then c"
		return
	}

	collected, err := m.selectedLines(lo, hi)
	if err != nil {
		m.status = fmt.Sprintf("copy failed: %v", err)
		return
	}

	if len(collected) == 0 {
		m.status = "no lines in selection"
		return
	}

	if err := writeClipboardOSC52(strings.Join(collected, "\n")); err != nil {
		m.status = fmt.Sprintf("clipboard write failed: %v", err)
		return
	}

	m.status = fmt.Sprintf("Copied %d lines", len(collected))
	m.selActive = false
}

// selectedLines returns the full (untruncated) content of the selected range. In
// results-only mode lo/hi are match indices, so they are mapped through
// m.matches to the real line numbers.
func (m *model) selectedLines(lo, hi int64) ([]string, error) {
	if m.onlyMatches && len(m.matches) > 0 {
		return m.selectedMatchLines(lo, hi), nil
	}

	items, err := m.idx.GetLines(lo, hi-lo+1)
	if err != nil {
		return nil, err
	}

	collected := make([]string, 0, len(items))
	for _, it := range items {
		collected = append(collected, it.Content)
	}

	return collected, nil
}

// selectedMatchLines resolves a selected range of match indices to line content,
// clamped to the match list and skipping lines that cannot be read.
func (m *model) selectedMatchLines(lo, hi int64) []string {
	first := max(int(lo), 0)
	last := min(int(hi), len(m.matches)-1)

	var collected []string

	for i := first; i <= last; i++ {
		line, err := m.idx.GetLine(m.matches[i])
		if err == nil {
			collected = append(collected, line)
		}
	}

	return collected
}

// writeClipboardOSC52 copies text to the terminal clipboard using the OSC 52
// escape sequence (works in most modern terminal emulators). The viewer runs in
// the alternate screen, so tea.Printf is suppressed; writing the sequence
// directly to stdout (the program's output) is the supported path.
func writeClipboardOSC52(text string) error {
	if text == "" {
		return nil
	}

	b64 := base64.StdEncoding.EncodeToString([]byte(text))
	seq := fmt.Sprintf("\x1b]52;c;%s\x07", b64)
	_, err := os.Stdout.WriteString(seq)

	return err
}

// resolveExportPath expands a raw export path via the shared helper.
func resolveExportPath(raw string) (string, bool, error) {
	return shared.PrepareExportTarget(raw)
}

// defaultExportName returns the timestamped default export file name.
func defaultExportName() string {
	ts := time.Now().Format("20060102-150405")
	return filepath.Join(".", "gofle-export-"+ts+".log")
}

// lineWriter funnels export rows into a bufio.Writer, tracking the first write
// error so every exporter can stop early without reaching into the file handle.
type lineWriter struct {
	w   *bufio.Writer
	err error
}

func (lw *lineWriter) writeLine(b []byte) bool {
	if lw.err != nil {
		return false
	}

	if _, e := lw.w.Write(b); e != nil {
		lw.err = e

		return false
	}

	if e := lw.w.WriteByte('\n'); e != nil {
		lw.err = e

		return false
	}

	return true
}

// doExport writes the current matches (or the whole file when none) to path.
// Write, flush and close errors are surfaced via m.err; unreadable lines are
// counted and reported instead of silently shrinking the export.
//
// Reads go through the bulk contiguous-run/chunk ScanLines path (T2) — the
// TUI twin of exports.go SaveFullLogAs/SaveSearchResults: the old per-line
// GetLoop shape issued one pread + one string + one fmt.Sprintf per line
// (~10M syscalls on a 10M-line full-file export, the shape benchmarked as
// BenchmarkExportGetLineLoop at ~15x the bulk path).
func (m *model) doExport(path string) {
	//nolint:gosec // the export path is typed by the user in the export prompt; writing where they asked is the feature
	f, err := os.Create(path)
	if err != nil {
		m.err = fmt.Errorf("failed to create export file: %w", err)
		return
	}

	lw := &lineWriter{w: bufio.NewWriter(f)}

	var written, skipped int

	if len(m.matches) > 0 {
		written, skipped = m.exportMatchedLines(lw)
	} else {
		written, skipped = m.exportAllLines(lw)
	}

	// Flush before close: the buffer holds the data, the close releases the fd.
	flushErr := lw.w.Flush()
	closeErr := f.Close()

	switch {
	case lw.err != nil:
		m.err = fmt.Errorf("failed to write export: %w", lw.err)
	case flushErr != nil:
		m.err = fmt.Errorf("failed to flush export: %w", flushErr)
	case closeErr != nil:
		m.err = fmt.Errorf("failed to close export: %w", closeErr)
	default:
		abs, _ := filepath.Abs(path)

		status := fmt.Sprintf("Exported %d lines to %s", written, abs)
		if skipped > 0 {
			status += fmt.Sprintf(" (%d unreadable lines skipped)", skipped)
		}

		m.status = status
	}
}

// exportMatchedLines writes the matched lines, prefixed with their 1-based line
// number, and reports how many were written and how many were unreadable.
// Contiguous runs of the ascending match list are read with one bulk
// ScanLinesRetry pass (C3 semantics); a run whose bulk read fails falls back
// to per-line reads for exactly that run so skip accounting stays precise.
// Numbers are appended into a reusable buffer instead of fmt.Sprintf per line
// (H1: interface boxing + reflection per line).
func (m *model) exportMatchedLines(lw *lineWriter) (written, skipped int) {
	total := m.idx.TotalLines()
	num := make([]byte, 0, 32)

	i := 0
	for i < len(m.matches) && lw.err == nil {
		if m.matches[i] >= total {
			skipped++
			i++

			continue
		}

		runEnd := i + 1
		for runEnd < len(m.matches) && m.matches[runEnd] == m.matches[runEnd-1]+1 && m.matches[runEnd] < total {
			runEnd++
		}

		start, count := m.matches[i], int64(runEnd-i)

		serr := m.idx.ScanLinesRetry(start, count, func(idx int64, line []byte) bool {
			num = strconv.AppendInt(num[:0], idx+1, 10)
			num = append(num, ':')
			num = append(num, line...)

			return lw.writeLine(num)
		})
		if serr == nil {
			written += runEnd - i
			i = runEnd

			continue
		}

		// Chunk read failed (e.g. raced a rotation): per-line fallback for
		// exactly this run so skip accounting stays precise.
		for k := i; k < runEnd && lw.err == nil; k++ {
			ln := m.matches[k]

			line, gerr := m.idx.GetLine(ln)
			if gerr != nil {
				skipped++
				continue
			}

			num = strconv.AppendInt(num[:0], ln+1, 10)
			num = append(num, ':')
			num = append(num, line...)

			if lw.writeLine(num) {
				written++
			}
		}

		i = runEnd
	}

	return written, skipped
}

// exportAllLines writes the whole file verbatim and reports how many lines were
// written and how many were unreadable. The file is streamed in 500k-line
// chunks via ScanLines (round-1 O4); a chunk that fails is re-read per-line so
// genuinely unreadable lines are still counted, not silently dropped.
func (m *model) exportAllLines(lw *lineWriter) (written, skipped int) {
	total := m.idx.TotalLines()

	const exportChunk = int64(500_000)

	for start := int64(0); start < total && lw.err == nil; start += exportChunk {
		count := min(exportChunk, total-start)

		serr := m.idx.ScanLinesRetry(start, count, func(_ int64, line []byte) bool {
			return lw.writeLine(line)
		})
		if serr == nil {
			written += int(count)
			continue
		}

		// Chunk read failed (e.g. raced a rotation): per-line fallback for
		// exactly this chunk.
		for k := start; k < start+count && lw.err == nil; k++ {
			line, gerr := m.idx.GetLine(k)
			if gerr != nil {
				skipped++
				continue
			}

			if lw.writeLine([]byte(line)) {
				written++
			}
		}
	}

	return written, skipped
}

// remoteTargetFromRecent rebuilds the ssh:// or ftp:// URL for a stored remote
// recent entry so it can be reopened (cmds.buildSSHURL lives in the other package
// and cannot be imported here without a cycle). The scheme is taken from the
// stored Protocol so ftp recent entries reopen over ftp, not ssh.
func remoteTargetFromRecent(item config.RecentFileItem) string {
	if item.SSH == nil {
		return item.Path
	}

	proto := item.SSH.Protocol
	if proto == "" {
		proto = "ssh"
	}

	var b strings.Builder
	if proto == "ftp" {
		b.WriteString("ftp://")
	} else {
		b.WriteString("ssh://")
	}

	if item.SSH.Username != "" {
		b.WriteString(item.SSH.Username)
		b.WriteString("@")
	}

	b.WriteString(item.SSH.Host)

	if item.SSH.Port != 0 {
		fmt.Fprintf(&b, ":%d", item.SSH.Port)
	}

	p := item.SSH.RemoteFilePath
	if p == "" {
		p = "/"
	}

	b.WriteString(p)

	return b.String()
}

// closeCurrentTarget tears down the currently open target (if any) without
// blocking the event loop. Streamer.Close() can wait up to its close timeout for
// a stuck SSH tail to release, so it runs in a goroutine; the reference is
// cleared so it can never be torn down twice.
func (m *model) closeCurrentTarget() {
	if m.cleanup != nil {
		go m.cleanup()

		m.cleanup = nil
	}
}

// reopen closes the current target and opens a new one in place, resetting all
// viewer state. It is used by the recent-files picker. SSH targets rely on
// cached credentials from the recent entry, so no interactive prompt is needed.
// The new target is resolved BEFORE the old one is released: closing first used
// to brick the viewer against a closed indexer when resolution failed. The old
// teardown is asynchronous so switching away from an SSH log never hangs the UI.
//
// The resolve runs synchronously inside Update, so an SSH target would otherwise
// hold the terminal for the full dial/handshake/stall watchdogs. m.ctx descends
// from the command context (RunView), which carries main's interrupt
// cancellation, so Ctrl+C aborts the open rather than being queued behind it.
func (m *model) reopen(target string) error {
	idx, se, cleanup, err := shared.ResolveTarget(m.ctx, target, m.cfg)
	if err != nil {
		return fmt.Errorf("failed to open %s: %w", target, err)
	}

	m.closeCurrentTarget()
	m.idx = idx
	m.se = se
	m.cleanup = cleanup
	m.resetViewerState()
	m.lastRemoteCfg = nil
	m.remoteProto = ""
	// If the new target is remote, remember credentials from the matching
	// recent entry so "b" can browse other files on the same server.
	if m.cfg != nil {
		if sum := findRecentSummaryForTarget(m.cfg, target); sum != nil {
			cfg := remoteConfigFromSummary(sum)
			m.lastRemoteCfg = &cfg
			m.remoteProto = cfg.Protocol
			m.fillRemoteFormFromRecent(sum)
		}
	}
	m.status = "opened " + idx.FilePath()
	// Keep multi-tab list in sync: replace the active session snapshot.
	if len(m.sessions) > 0 && m.activeSession >= 0 && m.activeSession < len(m.sessions) {
		m.sessions[m.activeSession] = m.snapshotSession()
	}

	return nil
}

// resetViewerState clears all search/selection/navigation state so the viewer
// shows a freshly opened target's matches-only (filter) view from the top. It is
// shared by reopen() and the async remote-open path (openRemoteCmd) so the open/reset logic is not
// duplicated across the two open paths.
func (m *model) resetViewerState() {
	m.matches = nil
	m.matchCursor = 0
	m.query = ""
	m.searchInput.SetValue("")
	m.searchOpts = search.SearchOptions{CaseSensitive: false}
	m.onlyMatches = true
	m.tailOn = false
	m.top = 0
	m.topWrap = 0
}

// recentPickerModel is a standalone interactive picker used by the `gofle recent`
// command: it lists recently opened files, navigates with up/down (or k/j), and
// returns the chosen target (local path or ssh:// URL) on enter. Pressing "b" on
// a remote entry requests browse-other-file-on-server (same as the in-viewer o
// picker). This mirrors the in-viewer "open recent" experience.
type recentPickerModel struct {
	items    []config.RecentFileItem
	cursor   int
	canceled bool
	// browse is set when the user presses "b" on a remote entry: the caller
	// should open the remote directory browser with that entry's credentials
	// instead of opening the file.
	browse bool
}

func (m recentPickerModel) Init() tea.Cmd { return nil }

func (m recentPickerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}

	switch key.String() {
	case "ctrl+c", "q", keyEsc:
		m.canceled = true
		return m, tea.Quit
	case "up", "k":
		if len(m.items) > 0 {
			m.cursor = (m.cursor + len(m.items) - 1) % len(m.items)
		}
	case keyDown, "j":
		if len(m.items) > 0 {
			m.cursor = (m.cursor + 1) % len(m.items)
		}
	case keyEnter:
		if len(m.items) == 0 {
			m.canceled = true
		}

		return m, tea.Quit
	case "b":
		// Browse another file on the same remote server (SSH/FTP recent only).
		if len(m.items) == 0 {
			return m, nil
		}
		item := m.items[m.cursor]
		if !item.IsSSH || item.SSH == nil {
			// Local entry: ignore (same as in-viewer status); stay in picker.
			return m, nil
		}
		m.browse = true
		return m, tea.Quit
	}

	return m, nil
}

func (m recentPickerModel) View() string {
	var b strings.Builder
	b.WriteString(headerStyle.Render("Open recent") + "\n\n")
	b.WriteString(renderRecentList(m.items, m.cursor))
	b.WriteString("\n" + hintStyle.Render("↑↓/jk move  ·  enter open  ·  b browse other file on server  ·  esc cancel"))

	return b.String()
}
