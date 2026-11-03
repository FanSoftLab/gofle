package tui

import (
	"GoFLE/pkg/search"
	"fmt"
	"slices"
	"strings"
	"testing"
)

// TestStripANSIFastPathMatchesRegexp guards the allocation-free fast path in
// stripANSI: strings that cannot contain an escape introducer are returned
// verbatim instead of going through ansiRe. The fast path must therefore agree
// with the regexp on every input, including the awkward ones — a bare ESC, a
// truncated CSI, a lone 0x9B byte (invalid UTF-8, decodes to RuneError and does
// NOT match the pattern's \x9b alternative) and the properly encoded U+009B.
func TestStripANSIFastPathMatchesRegexp(t *testing.T) {
	inputs := []string{
		"",
		"plain text",
		"\x1b[31mred\x1b[0m",
		"\x1b[1;31;42mstyled\x1b[0m tail",
		"\x1b[38;5;208mextended\x1b[0m",
		matchStyle.Render("selected row 42"),
		selectionStyle.Render("日本語 row"),
		"\x1b[12",
		"\x1b[123456",
		"\x1b[;H",
		"\x1b(B\x1b[m",
		"\x1b]0;title\x07rest",
		"esc at end \x1b",
		"csi rune \u009b31m x",
		"lone 0x9b byte: " + string([]byte{0x9b}) + " tail",
		"invalid utf8 \xff\xfe tail",
		strings.Repeat("\x1b[1mA\x1b[0m", 50),
	}
	for _, in := range inputs {
		want := ansiRe.ReplaceAllString(in, "")
		if got := stripANSI(in); got != want {
			t.Errorf("stripANSI(%q) = %q, want %q", in, got, want)
		}

		if got := displayWidthNoANSI(in); got != displayWidth(want) {
			t.Errorf("displayWidthNoANSI(%q) = %d, want %d", in, got, displayWidth(want))
		}
	}
}

// TestRenderRowBudget guards the incremental row counter that replaced the
// per-row countNewlines(b.String()) rescan: every renderer must still stop at
// exactly m.height rows and never overshoot, including when long lines wrap into
// several rows (so one logical line contributes many rows) and when the viewport
// starts mid-line via topWrap.
func TestRenderRowBudget(t *testing.T) {
	lines := make([]string, 60)
	for i := range lines {
		if i%3 == 0 {
			// Long enough to wrap into several rows at every tested width.
			lines[i] = fmt.Sprintf("%02d MATCH ", i) + strings.Repeat("payload ", 30)
		} else {
			lines[i] = fmt.Sprintf("%02d short", i)
		}
	}

	base := newTestModel(t, lines)

	res, err := base.se.Search(search.SearchOptions{Query: "MATCH"})
	if err != nil {
		t.Fatal(err)
	}

	if len(res) == 0 {
		t.Fatal("expected matches for the row-budget fixture")
	}

	for _, height := range []int{1, 2, 5, 17, 40} {
		for _, width := range []int{20, 40, 120} {
			for _, topWrap := range []int{0, 1, 4} {
				for _, onlyMatches := range []bool{false, true} {
					m := base
					m.height = height
					m.width = width
					m.topWrap = topWrap
					m.top = 2
					m.matches = res
					m.onlyMatches = onlyMatches

					if rows := renderedRows(m.renderLogLines(width)); rows > height {
						t.Errorf("renderLogLines rows = %d, want <= %d (h=%d w=%d wrap=%d only=%v)",
							rows, height, height, width, topWrap, onlyMatches)
					}

					if rows := renderedRows(m.renderResultsLines(width)); rows > height {
						t.Errorf("renderResultsLines rows = %d, want <= %d (h=%d w=%d)",
							rows, height, height, width)
					}
					// The split body is padded to a fixed height, so it must be
					// exactly m.height rows — one row too many wraps the terminal.
					m.splitView = true
					if rows := renderedRows(m.renderSplitBody()); rows != height {
						t.Errorf("renderSplitBody rows = %d, want exactly %d (w=%d)", rows, height, width)
					}
				}
			}
		}
	}
}

// renderedRows counts the terminal rows in a rendered body (each row is written
// with a trailing newline, except possibly the last).
func renderedRows(s string) int {
	if s == "" {
		return 0
	}
	return len(strings.Split(s, "\n"))
}

// TestRowRenderersMatchJoinedBodies pins the equivalence the split view relies on
// (M2): the row slices it consumes are exactly the rows of the joined body string
// the non-split renderers return. If the two ever diverge, the split panes would
// silently show different content from the normal view.
func TestRowRenderersMatchJoinedBodies(t *testing.T) {
	lines := make([]string, 40)
	for i := range lines {
		if i%4 == 0 {
			lines[i] = fmt.Sprintf("%02d MATCH ", i) + strings.Repeat("payload ", 20)
		} else {
			lines[i] = fmt.Sprintf("%02d short line", i)
		}
	}

	base := newTestModel(t, lines)

	res, err := base.se.Search(search.SearchOptions{Query: "MATCH"})
	if err != nil {
		t.Fatal(err)
	}

	for _, height := range []int{1, 3, 12, 60} {
		for _, width := range []int{20, 60, 140} {
			for _, splitView := range []bool{false, true} {
				for _, matches := range [][]int64{nil, res} {
					m := base
					m.height = height
					m.width = width
					m.top = 1
					m.splitView = splitView
					m.matches = matches
					m.onlyMatches = matches != nil

					// strings.Split of a newline-terminated body yields a
					// trailing empty element, which is not a row.
					wantLog := splitTrailing(m.renderLogLines(width))
					if got := m.logRows(width); !slices.Equal(got, wantLog) {
						t.Fatalf("logRows != rows of renderLogLines (h=%d w=%d split=%v matches=%d):\n got %q\nwant %q",
							height, width, splitView, len(matches), got, wantLog)
					}

					wantRes := splitTrailing(m.renderResultsLines(width))
					if got := m.resultRows(width); !slices.Equal(got, wantRes) {
						t.Fatalf("resultRows != rows of renderResultsLines (h=%d w=%d split=%v matches=%d):\n got %q\nwant %q",
							height, width, splitView, len(matches), got, wantRes)
					}
				}
			}
		}
	}
}

// splitTrailing splits a body into rows. joinRows never emits a trailing \n, so
// there is no spurious trailing empty element to drop (unlike strings.Split on a
// newline-terminated string). An empty body yields no rows.
func splitTrailing(body string) []string {
	if body == "" {
		return nil
	}
	return strings.Split(body, "\n")
}

// TestRenderSplitBodyPadsEveryRow guards the padding the split view does itself
// now that it no longer pads a pre-split pane string: every row must be padded to
// the same width — including the blank rows that fill a pane shorter than the
// viewport (few matches, or a log shorter than the screen) — so the divider column
// stays continuous down the screen.
//
// Content and widths are chosen so no row is wide enough to be truncated: the
// truncation path counts ANSI escapes toward the width budget, which makes styled
// rows legitimately narrower and is not what this test is about.
func TestRenderSplitBodyPadsEveryRow(t *testing.T) {
	base := newTestModel(t, []string{"only MATCH line", "second line"})

	res, err := base.se.Search(search.SearchOptions{Query: "MATCH"})
	if err != nil {
		t.Fatal(err)
	}

	for _, height := range []int{2, 8, 30} {
		for _, width := range []int{80, 160} {
			m := base
			m.height = height
			m.width = width
			m.splitView = true
			m.matches = res

			rows := splitTrailing(m.renderSplitBody())
			if len(rows) != height {
				t.Fatalf("split body rows = %d, want %d (w=%d)", len(rows), height, width)
			}

			want := displayWidthNoANSI(rows[0])
			for i, row := range rows {
				if !strings.Contains(stripANSI(row), "│") {
					t.Fatalf("row %d has no divider (h=%d w=%d): %q", i, height, width, stripANSI(row))
				}

				if got := displayWidthNoANSI(row); got != want {
					t.Fatalf("row %d width = %d, want %d (h=%d w=%d): %q",
						i, got, want, height, width, stripANSI(row))
				}

				if want > m.width {
					t.Fatalf("rows overflow the terminal: width %d > %d", want, m.width)
				}
			}
		}
	}
}

// TestBodyKeyCoversWrapAndHOffset guards the body memoization key: wordWrap and
// hOffset change how every row is produced but used to be absent from bodyKey,
// so toggling wrap or scrolling horizontally re-served the body rendered under
// the previous mode (T1). Each state change below must invalidate the memo.
func TestBodyKeyCoversWrapAndHOffset(t *testing.T) {
	lines := make([]string, 30)
	for i := range lines {
		// Long enough to wrap into several rows at the test width.
		lines[i] = fmt.Sprintf("%02d MATCH %s", i, strings.Repeat("payload-", 30))
	}

	m := newTestModel(t, lines)
	m.top = 5

	// Warm the memo under the base state.
	base := m.bodyView()
	if base == "" {
		t.Fatal("empty body")
	}

	// Toggle word wrap: the body must change (wrapped rows vs clipped rows).
	m.wordWrap = true
	wrapped := m.bodyView()
	if wrapped == base {
		t.Error("toggling wordWrap returned the stale memoized body")
	}

	// Back to wrap off with horizontal scroll: the body must change again
	// (hOffset shifts the visible column window).
	m.wordWrap = false
	m.hOffset = 8
	shifted := m.bodyView()
	if shifted == base {
		t.Error("changing hOffset returned the stale memoized body")
	}
	if shifted == wrapped {
		t.Error("hOffset-shifted body equals the wrapped body")
	}

	// Restore the base state exactly: the memo may legitimately match base.
	m.hOffset = 0
	if m.bodyView() != base {
		t.Error("restored state should render the original body")
	}
}

// The wrap-count cache must return stable counts for repeated calls, and its
// stamp must invalidate on every input that changes the counts: width, split
// layout, totalLines, wordWrap and the indexer generation (T3).
func TestWrapCountCacheInvalidation(t *testing.T) {
	lines := make([]string, 40)
	for i := range lines {
		lines[i] = fmt.Sprintf("%02d %s", i, strings.Repeat("payload-", 30))
	}

	m := newTestModel(t, lines)
	m.wordWrap = true
	m.width = 120

	first := m.wrapRows(3)
	if first < 2 {
		t.Fatalf("wrapRows(3) = %d, want >=2 for a long line", first)
	}

	if again := m.wrapRows(3); again != first {
		t.Errorf("second wrapRows(3) = %d, want cached %d", again, first)
	}

	// Width change must invalidate.
	m.width = 40
	if got := m.wrapRows(3); got <= first {
		t.Errorf("wrapRows(3) after narrowing = %d, want > %d", got, first)
	}

	// Split toggle halves the pane width: must invalidate too.
	m.width = 120

	wrapCacheMu.Lock()
	stampBefore := wrapCacheStamp
	wrapCacheMu.Unlock()

	m.splitView = true
	_ = m.wrapRows(3)

	wrapCacheMu.Lock()
	stampAfter := wrapCacheStamp
	wrapCacheMu.Unlock()

	if stampBefore == stampAfter {
		t.Error("splitView toggle did not invalidate the wrap cache")
	}
}
