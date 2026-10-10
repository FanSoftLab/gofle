package tui

import (
	"strings"
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
	m := model{
		query:          "ERR",
		secondaryQuery: "timeout",
		searchOpts:     search.SearchOptions{CaseSensitive: false},
	}
	out := m.paintSearchTerms("ERR timeout in module")
	if out == "ERR timeout in module" {
		t.Fatal("expected styling applied")
	}
	// ANSI sequences should be present from lipgloss
	if !strings.Contains(out, "ERR") {
		t.Fatal("primary text lost")
	}
}
