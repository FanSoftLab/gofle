package tui

// layout is the single source of truth for how the terminal frame is divided.
// All width/height decisions for the normal viewer (body + footer) go through
// here so View() and recomputeHeight() cannot drift apart.
//
// Coordinate system (rows top → bottom):
//
//	[ body: height rows ]
//	[ optional prompt  ]
//	[ status bar       ]
//	[ optional message ]  // err / transient status / empty-results note
//	[ optional hint    ]  // dropped on very short terminals
type layout struct {
	termW int
	termH int

	// Body
	bodyH int
	// Split panes (0 when not in split). leftW + gap + rightW == termW.
	leftW  int
	rightW int
	gapW   int // spaces + divider cells between panes

	// Footer composition flags (computed once per frame).
	showPrompt bool
	promptRows int // 0–2
	showStatus bool
	showMsg    bool // err or transient status or empty-results note
	msgRows    int
	showHint   bool

	compact bool // short or narrow terminal: drop hint, shorten status
}

// buildLayout derives a layout from the current model and terminal size.
func (m model) buildLayout() layout {
	l := layout{
		termW:      max(m.width, 1),
		termH:      max(m.fullHeight, 1),
		showStatus: true,
	}

	// Compact when the terminal cannot comfortably host body + full footer.
	l.compact = l.termH < 18 || l.termW < 60

	switch {
	case m.searchMode, m.secondaryMode:
		l.showPrompt = true
		l.promptRows = 2 // primary + AND line
	case m.gotoMode, m.exportMode, m.exportConfirm:
		l.showPrompt = true
		l.promptRows = 1
	}

	l.msgRows = 0
	if m.err != nil {
		l.showMsg = true
		l.msgRows++
	}
	if m.status != "" {
		l.showMsg = true
		l.msgRows++
	}
	if m.onlyMatches && len(m.matches) == 0 {
		l.showMsg = true
		l.msgRows++
	}

	// Hint: always on roomy terminals; never on very short ones so body wins.
	l.showHint = !l.compact && l.termH >= 12

	footer := 0
	if l.showPrompt {
		footer += l.promptRows
	}
	if l.showStatus {
		footer++
	}
	footer += l.msgRows
	if l.showHint {
		footer++
	}
	// Minimum 1 body row.
	l.bodyH = max(l.termH-footer, 1)

	// Split pane widths only when the results pane has content.
	if m.effectiveSplit() {
		l.gapW = 3 // " │ "
		inner := max(l.termW-l.gapW, 2)
		l.leftW = inner / 2
		l.rightW = inner - l.leftW
		if l.leftW < 1 {
			l.leftW = 1
		}
		if l.rightW < 1 {
			l.rightW = 1
		}
	}

	return l
}

// footerLines returns how many rows the footer will occupy — kept as a thin
// wrapper so existing call sites stay readable.
func (m model) footerLines() int {
	l := m.buildLayout()
	n := 0
	if l.showPrompt {
		n += l.promptRows
	}
	if l.showStatus {
		n++
	}
	n += l.msgRows
	if l.showHint {
		n++
	}
	return max(n, 1)
}

// recomputeHeight applies buildLayout's body height to the model.
func (m *model) recomputeHeight() {
	if m.fullHeight <= 0 {
		return
	}
	m.height = m.buildLayout().bodyH
}
