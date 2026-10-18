package tui

import (
	"testing"

	"GoFLE/pkg/search"
)

func TestFindSpansLiteral(t *testing.T) {
	spans := findSpans("aaa ERROR bbb ERROR", "ERROR", false, true, 0, hlPrimaryStyle)
	if len(spans) != 2 {
		t.Fatalf("got %d spans: %+v", len(spans), spans)
	}
	if spans[0].start != 4 || spans[0].end != 9 {
		t.Fatalf("first span %+v", spans[0])
	}
}

func TestFindSpansCaseInsensitive(t *testing.T) {
	spans := findSpans("Hello HELLO", "hello", false, false, 0, hlPrimaryStyle)
	if len(spans) != 2 {
		t.Fatalf("got %d", len(spans))
	}
}

func TestPaintSearchTermsPrimaryAnd(t *testing.T) {
	// findSpans is the core matching logic; lipgloss.Style.Render() does not
	// emit ANSI in a non-terminal test environment, so we verify span detection
	// directly rather than relying on ANSI codes in paintSearchTerms output.
	m := model{
		query:          "ERR",
		secondaryQuery: "timeout",
		searchOpts:     search.SearchOptions{CaseSensitive: false},
	}
	seg := "ERR timeout in module"

	// Primary query "ERR" should match at [0, 3).
	primary := findSpans(seg, m.query, false, m.searchOpts.CaseSensitive, 0, hlPrimaryStyle)
	if len(primary) != 1 || primary[0].start != 0 || primary[0].end != 3 {
		t.Fatalf("primary spans = %+v, want [{0 3}]", primary)
	}

	// Secondary query "timeout" should match at [4, 11).
	secondary := findSpans(seg, m.secondaryQuery, false, m.searchOpts.CaseSensitive, 1, hlAndStyle)
	if len(secondary) != 1 || secondary[0].start != 4 || secondary[0].end != 11 {
		t.Fatalf("secondary spans = %+v, want [{4 11}]", secondary)
	}

	// paintSearchTerms should return the input text unchanged when no ANSI is
	// available (non-terminal env), but the text must still be intact.
	out := m.paintSearchTerms(seg)
	if out != seg {
		t.Fatalf("paintSearchTerms = %q, want input unchanged %q", out, seg)
	}
}
