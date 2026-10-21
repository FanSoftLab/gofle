package main

import (
	"GoFLE/pkg/indexer"
	"GoFLE/pkg/search"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// benchExportApp builds an App with one tab over a 100k-line log where every
// line matches, so export-search walks the full per-match write path.
func benchExportApp(b *testing.B) (*webAPI, string) {
	b.Helper()

	dir := b.TempDir()
	p := filepath.Join(dir, "bench.log")

	var sb strings.Builder

	for i := range 100_000 {
		sb.WriteString("ERROR line ")
		sb.WriteString(strconv.Itoa(i))
		sb.WriteString(" something went wrong\n")
	}

	if err := os.WriteFile(p, []byte(sb.String()), 0o644); err != nil {
		b.Fatalf("write log: %v", err)
	}

	idx, err := indexer.NewFileIndexer(p)
	if err != nil {
		b.Fatal(err)
	}

	b.Cleanup(func() { idx.Close() })

	matches := make([]int64, 0, 100_000)
	for i := range int64(100_000) {
		matches = append(matches, i)
	}

	tab := &TabState{
		ID:           "tab_bench",
		FilePath:     p,
		FileName:     "bench.log",
		Indexer:      idx,
		Searcher:     search.NewSearchEngine(idx),
		MatchedLines: matches,
	}

	api := &webAPI{app: &App{tabs: map[string]*TabState{tab.ID: tab}}}

	return api, tab.ID
}

// BenchmarkExportSearchHTTP pins the /api/tabs/<id>/export-search handler
// (M3): the baseline writes one pread + one Fprintf per match straight into
// the (unbuffered) ResponseWriter; the fixed path uses bulk ScanLines runs
// plus a bufio.Writer.
func BenchmarkExportSearchHTTP(b *testing.B) {
	api, tabID := benchExportApp(b)

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		w := httptest.NewRecorder()
		api.handleExportSearch(w, tabID)

		if w.Code != 200 {
			b.Fatalf("status %d", w.Code)
		}
	}
}
