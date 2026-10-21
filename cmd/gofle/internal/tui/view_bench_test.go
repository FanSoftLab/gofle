package tui

import (
	"GoFLE/pkg/indexer"
	"GoFLE/pkg/search"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/textinput"
)

// buildBenchModel makes a large index (nLines) with a deterministic fraction of
// lines containing the token "Exception", runs a case-insensitive search for
// "request" so most lines match (exercising matchStyle ANSI rendering), and
// returns a render-ready model.
func buildBenchModel(b *testing.B, nLines, height, width int) model {
	b.Helper()
	dir := b.TempDir()
	p := filepath.Join(dir, "bench.log")

	f, err := os.Create(p)
	if err != nil {
		b.Fatal(err)
	}

	var sb strings.Builder
	for i := range nLines {
		sb.Reset()
		fmt.Fprintf(&sb, "2026-08-15T10:%02d:%02d.000Z level=info svc=%d user=%08d ", i%60, i%60, i%7, i)

		if i%50 == 0 {
			sb.WriteString("Exception: something failed trace=abc123\n")
		} else {
			sb.WriteString("request handled ok latency=12ms status=200 trace=def456\n")
		}

		if _, err := f.WriteString(sb.String()); err != nil {
			b.Fatal(err)
		}
	}

	f.Close()

	idx, err := indexer.NewFileIndexer(p)
	if err != nil {
		b.Fatal(err)
	}

	se := search.NewSearchEngine(idx)

	res, err := se.SearchWithContext(context.Background(), search.SearchOptions{Query: "request", CaseSensitive: false})
	if err != nil {
		b.Fatal(err)
	}

	b.Cleanup(func() { idx.Close() })

	si := textinput.New()
	gi := textinput.New()
	ei := textinput.New()
	m := model{
		idx:         idx,
		se:          se,
		height:      height,
		width:       width,
		searchInput: si,
		gotoInput:   gi,
		exportInput: ei,
		searchOpts:  search.SearchOptions{CaseSensitive: false},
		ctx:         context.Background(),
		matches:     res,
		matchCursor: 0,
		top:         0,
		onlyMatches: true, // matchStyle (ANSI) rendering on every row
	}

	return m
}

// BenchmarkViewNormal renders the normal (full-log) view with a tall viewport.
func BenchmarkViewNormal(b *testing.B) {
	m := buildBenchModel(b, 200_000, 200, 120)
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_ = m.View()
	}
}

// BenchmarkViewSplit renders the 50/50 split view (exercises per-row
// padRuneDisplay/truncateToWidth -> stripANSI and strings.Split of both panes).
func BenchmarkViewSplit(b *testing.B) {
	m := buildBenchModel(b, 200_000, 200, 120)
	m.splitView = true

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_ = m.View()
	}
}

// BenchmarkViewNormalHighlight pins the case-insensitive literal highlight
// path (findSpans without regex): one rendered row per match per frame.
func BenchmarkViewNormalHighlight(b *testing.B) {
	m := buildBenchModel(b, 200_000, 200, 120)
	m.query = "i:request"

	opts := m.effectiveSearchOptions()

	res, err := m.se.SearchWithContext(context.Background(), opts)
	if err != nil {
		b.Fatal(err)
	}

	m.matches = res

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if len(m.matches) > 0 {
			m.matchCursor = (m.matchCursor + 1) % len(m.matches)
		}

		_ = m.View()
	}
}

// BenchmarkViewNormalRegex pins the regex highlight path: with a regex query
// active, every rendered row compiles the pattern (baseline) or serves it
// from the compiled-regex cache (after the fix).
func BenchmarkViewNormalRegex(b *testing.B) {
	m := buildBenchModel(b, 200_000, 200, 120)
	m.query = "/Exception|latency"

	opts := m.effectiveSearchOptions()

	res, err := m.se.SearchWithContext(context.Background(), opts)
	if err != nil {
		b.Fatal(err)
	}

	m.matches = res

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if len(m.matches) > 0 {
			m.matchCursor = (m.matchCursor + 1) % len(m.matches)
		}

		_ = m.View()
	}
}

// BenchmarkViewSplitRegex is BenchmarkViewNormalRegex in the 50/50 split view,
// which doubles the per-frame highlight work (both panes render match rows).
func BenchmarkViewSplitRegex(b *testing.B) {
	m := buildBenchModel(b, 200_000, 200, 120)
	m.query = "/Exception|latency"

	opts := m.effectiveSearchOptions()

	res, err := m.se.SearchWithContext(context.Background(), opts)
	if err != nil {
		b.Fatal(err)
	}

	m.matches = res
	m.splitView = true

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if len(m.matches) > 0 {
			m.matchCursor = (m.matchCursor + 1) % len(m.matches)
		}

		_ = m.View()
	}
}
