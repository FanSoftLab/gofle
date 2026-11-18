package tui

import (
	"GoFLE/pkg/search"
	"fmt"
	"strings"
	"testing"
)

// TestWordWrapMatchNavigation verifies that match navigation (n/p, [, ]) properly
// centers the match in the viewport when word wrap is enabled. Previously, matches
// were pinned to topWrap=0, which could hide the first visual rows of long wrapped
// lines when jumping to a match.
func TestWordWrapMatchNavigation(t *testing.T) {
	// Create fixture: every 5th line is LONG (wraps to multiple visual rows),
	// others are short. Matches on both long and short lines.
	lines := make([]string, 120)
	for i := range lines {
		//nolint:gocritic // simple test fixture setup
		switch i % 5 {
		case 0:
			lines[i] = fmt.Sprintf("LONG-%03d %s", i, strings.Repeat("payload-", 40))
		case 2:
			lines[i] = fmt.Sprintf("MATCH-short-%03d", i)
		default:
			lines[i] = fmt.Sprintf("short-%03d", i)
		}
	}

	m := newTestModel(t, lines)
	m.width = 100
	m.height = 10
	m.wordWrap = true

	// Search for "LONG" with regex to also match "MATCH-short" lines
	res, err := m.se.Search(search.SearchOptions{Query: "LONG|MATCH", IsRegex: true})
	if err != nil {
		t.Fatal(err)
	}
	m.matches = res

	if len(m.matches) == 0 {
		t.Fatal("expected matches for LONG|MATCH")
	}

	need := m.height / 2 // half-page of visual rows for centering

	for cursor := range m.matches {
		m.matchCursor = cursor
		m.top = m.matches[cursor]

		// Use the fixed navigation logic
		m.keepMatchVisible()

		mc := m.matches[cursor]

		// Verify: match's first visual row is on screen
		above := 0
		for ln := m.top; ln < mc; ln++ {
			above += m.wrapRows(ln)
		}
		above -= m.topWrap

		if above < 0 || above >= m.height {
			t.Errorf("cursor %d (line %d): match first row at %d of %d-row viewport — off screen (top=%d topWrap=%d)",
				cursor, mc, above, m.height, m.top, m.topWrap)
		}

		// Verify: topWrap is valid for the top line
		wr := m.wrapRows(m.top)
		if m.topWrap < 0 || m.topWrap >= wr {
			t.Errorf("cursor %d (line %d): topWrap %d outside 0..%d for line %d",
				cursor, mc, m.topWrap, wr-1, m.top)
		}

		// Verify exact centering when there's room above and below
		rowsAbove := 0
		for ln := range mc {
			rowsAbove += m.wrapRows(ln)
		}

		rowsBelow := 0
		for ln := mc; ln < m.idx.TotalLines() && rowsBelow < m.height-need; ln++ {
			rowsBelow += m.wrapRows(ln)
		}

		if rowsAbove >= need && rowsBelow >= m.height-need && above != need {
			t.Errorf("cursor %d (line %d): visual rows above match = %d, want exact center %d (top=%d topWrap=%d)",
				cursor, mc, above, need, m.top, m.topWrap)
		}

		// Verify: rendered body actually shows the matched line
		body := m.bodyView()
		hasMatch := strings.Contains(body, fmt.Sprintf("LONG-%03d", mc)) ||
			strings.Contains(body, fmt.Sprintf("MATCH-short-%03d", mc))
		if !hasMatch {
			t.Errorf("cursor %d (line %d): rendered body does not show the matched line (top=%d topWrap=%d)",
				cursor, mc, m.top, m.topWrap)
		}
	}
}

// TestStepMatchWithWrap verifies stepMatch (n/p keys) works correctly with word wrap.
func TestStepMatchWithWrap(t *testing.T) {
	lines := make([]string, 50)
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

	// Search for "LONG" -> matches at lines 0, 5, 10, 15, 20, 25, 30, 35, 40, 45
	res, err := m.se.Search(search.SearchOptions{Query: "LONG"})
	if err != nil {
		t.Fatal(err)
	}
	m.matches = res

	// Start at first match
	m.matchCursor = 0
	m.top = m.matches[0]
	m.keepMatchVisible()

	// Step forward through all matches
	for i := 1; i < len(m.matches); i++ {
		m.stepMatch(1)

		mc := m.matches[m.matchCursor]

		// Verify match is visible
		if m.top > mc || m.top+int64(m.height) <= mc {
			t.Errorf("stepMatch to %d (line %d): not visible (top=%d height=%d)",
				m.matchCursor, mc, m.top, m.height)
		}

		// Verify topWrap is valid
		wr := m.wrapRows(m.top)
		if m.topWrap < 0 || m.topWrap >= wr {
			t.Errorf("stepMatch to %d: invalid topWrap=%d for line %d (wrapRows=%d)",
				m.matchCursor, m.topWrap, m.top, wr)
		}
	}

	// Step backward through all matches
	for i := len(m.matches) - 2; i >= 0; i-- {
		m.stepMatch(-1)

		mc := m.matches[m.matchCursor]

		if m.top > mc || m.top+int64(m.height) <= mc {
			t.Errorf("stepMatch back to %d (line %d): not visible (top=%d height=%d)",
				m.matchCursor, mc, m.top, m.height)
		}

		wr := m.wrapRows(m.top)
		if m.topWrap < 0 || m.topWrap >= wr {
			t.Errorf("stepMatch back to %d: invalid topWrap=%d for line %d",
				m.matchCursor, m.topWrap, m.top)
		}
	}
}

// TestGotoMatchWithWrap verifies gotoMatch ([/] keys) works correctly with word wrap.
func TestGotoMatchWithWrap(t *testing.T) {
	lines := make([]string, 50)
	for i := range lines {
		//nolint:gocritic // simple test fixture setup
		switch i % 5 {
		case 0:
			lines[i] = fmt.Sprintf("LONG-%03d %s", i, strings.Repeat("payload-", 40))
		case 2:
			lines[i] = fmt.Sprintf("MATCH-short-%03d", i)
		default:
			lines[i] = fmt.Sprintf("short-%03d", i)
		}
	}

	m := newTestModel(t, lines)
	m.width = 100
	m.height = 10
	m.wordWrap = true

	res, err := m.se.Search(search.SearchOptions{Query: "LONG|MATCH"})
	if err != nil {
		t.Fatal(err)
	}
	m.matches = res

	// Test gotoMatch for each match index
	for i := range m.matches {
		m.gotoMatch(i)

		mc := m.matches[m.matchCursor]

		// Verify match is visible
		if m.top > mc || m.top+int64(m.height) <= mc {
			t.Errorf("gotoMatch(%d) line %d: not visible (top=%d height=%d)",
				i, mc, m.top, m.height)
		}

		// Verify topWrap is valid
		wr := m.wrapRows(m.top)
		if m.topWrap < 0 || m.topWrap >= wr {
			t.Errorf("gotoMatch(%d): invalid topWrap=%d for line %d",
				i, m.topWrap, m.top)
		}

		// Verify body shows the target line
		body := m.bodyView()
		hasMatch := strings.Contains(body, fmt.Sprintf("LONG-%03d", mc)) ||
			strings.Contains(body, fmt.Sprintf("MATCH-short-%03d", mc))
		if !hasMatch {
			t.Errorf("gotoMatch(%d) line %d: body doesn't show matched line (top=%d topWrap=%d)",
				i, mc, m.top, m.topWrap)
		}
	}
}

// TestResultsPaneEnterWithWrap verifies that pressing Enter in the results pane
// (split view) properly resets topWrap when jumping to a match.
func TestResultsPaneEnterWithWrap(t *testing.T) {
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
	m.splitView = true
	m.splitFocus = 1

	res, err := m.se.Search(search.SearchOptions{Query: "LONG"})
	if err != nil {
		t.Fatal(err)
	}
	m.matches = res
	m.matchCursor = 1 // Second match (line 5)

	// Leave the log pane mid-line: show the last visual row of line 5
	m.top = 5
	m.topWrap = m.wrapRows(5) - 1

	if m.topWrap < 1 {
		t.Fatalf("fixture: wrapRows(5)=%d, expected >=2", m.wrapRows(5))
	}

	// Simulate Enter key in results pane (resultsPaneKey handles this)
	handled := m.resultsPaneKey(keyEnter)
	if !handled {
		t.Fatal("resultsPaneKey did not handle enter")
	}

	// Verify: switched back to log pane
	if m.splitFocus != 0 {
		t.Errorf("splitFocus = %d, want 0 (back to log pane)", m.splitFocus)
	}

	// Verify: jumped to the match line
	if m.top != 5 {
		t.Errorf("top = %d, want 5 (the match line)", m.top)
	}

	// Verify: topWrap was reset (the bug was that it wasn't)
	if m.topWrap != 0 {
		t.Errorf("topWrap = %d, want 0 after enter", m.topWrap)
	}

	// Verify: body starts at the target line
	body := m.bodyView()
	if !strings.Contains(body, "LONG-005") {
		t.Error("rendered body does not start at the jumped-to match")
	}
}

// TestRunGotoWithWrap verifies the goto-line prompt (g key) resets topWrap.
func TestRunGotoWithWrap(t *testing.T) {
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

	// Leave the viewport mid-line
	m.top = 5
	m.topWrap = m.wrapRows(5) - 1

	if m.topWrap < 1 {
		t.Fatalf("fixture: wrapRows(5)=%d, expected >=2", m.wrapRows(5))
	}

	// Use goto prompt to jump to line 51 (0-based: 50)
	m.gotoInput.SetValue("51")
	m.runGoto()

	if m.top != 50 {
		t.Errorf("top = %d, want 50 (1-based line 51)", m.top)
	}

	if m.topWrap != 0 {
		t.Errorf("topWrap = %d, want 0 after goto", m.topWrap)
	}

	body := m.bodyView()
	if !strings.Contains(body, "LONG-050") {
		t.Error("rendered body does not start at the goto target")
	}
}

// TestEnsureResVisibleWordWrap_StepCycle tests that repeated stepMatch
// calls with word wrap keep the match cursor visible, and that the
// rendered resultRows contains the current match marker.
func TestEnsureResVisibleWordWrap_StepCycle(t *testing.T) {
	lines := make([]string, 40)
	for i := range lines {
		// Long lines that wrap to several visual rows at narrow width.
		lines[i] = fmt.Sprintf("MATCH-%02d %s", i, strings.Repeat("payload-", 30))
	}
	m := newTestModel(t, lines)
	m.height = 6
	m.width = 40
	m.splitView = true
	m.wordWrap = true
	m.matches = make([]int64, 40)
	for i := range m.matches {
		m.matches[i] = int64(i)
	}
	m.matchCursor = 0
	m.resTop = 0

	// Step through matches and verify visibility after each step
	for i := range 20 {
		m.matchCursor = i
		m.ensureResVisible()
		if !m.matchCursorVisibleFrom(m.resTop) {
			t.Fatalf("step %d: matchCursor %d not visible from resTop=%d (word-wrap)", i, m.matchCursor, m.resTop)
		}
	}

	// Also verify resultRows renders the current match marker
	m.matchCursor = 15
	m.ensureResVisible()
	rightW := m.width/2 - 2 // approximate results pane width
	rows := m.resultRows(rightW)
	found := false
	for _, row := range rows {
		if strings.Contains(row, "MATCH-15") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("resultRows does not contain current match marker MATCH-15: %v", rows)
	}
}
