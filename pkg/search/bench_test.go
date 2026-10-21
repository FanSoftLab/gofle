package search

import (
	"GoFLE/pkg/indexer"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// benchFile builds a temp log file of nLines lines, each ~120 bytes of mixed
// content, with roughly one-in-every k lines containing the token "Exception".
// The pattern is deterministic so benchmarks are reproducible.
func benchFile(b *testing.B, nLines, k int) *indexer.FileIndexer { //nolint:dupl // benchmark helpers intentionally share setup
	b.Helper()
	dir := b.TempDir()
	p := filepath.Join(dir, "bench.log")

	f, err := os.Create(p)
	if err != nil {
		b.Fatalf("create bench file: %v", err)
	}

	var sb strings.Builder
	for i := range nLines {
		sb.Reset()
		fmt.Fprintf(&sb, "2026-08-15T10:%02d:%02d.000Z level=info service=svc%d user=%08d ", i%60, i%60, i%7, i)

		if i%k == 0 {
			sb.WriteString("Exception: something failed in request handler trace=abc123\n")
		} else {
			sb.WriteString("handled request ok latency=12ms status=200 trace=def456\n")
		}

		if _, err := f.WriteString(sb.String()); err != nil {
			b.Fatalf("write bench file: %v", err)
		}
	}

	if err := f.Close(); err != nil {
		b.Fatalf("close bench file: %v", err)
	}

	idx, err := indexer.NewFileIndexer(p)
	if err != nil {
		b.Fatalf("create indexer: %v", err)
	}

	return idx
}

// BenchmarkSearchCaseInsensitive exercises the dominant hot path: a full-file,
// case-insensitive substring search. This is where the per-line ToLower
// allocation and the map+O(n) resort live.
func BenchmarkSearchCaseInsensitive(b *testing.B) {
	const nLines = 200_000

	const k = 50 // ~4000 matches

	idx := benchFile(b, nLines, k)
	se := NewSearchEngine(idx)
	opts := SearchOptions{Query: "exception", CaseSensitive: false}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if _, err := se.SearchWithContext(b.Context(), opts); err != nil {
			b.Fatalf("search: %v", err)
		}
	}
}

// BenchmarkSearchRegex exercises the regex path over the whole index.
func BenchmarkSearchRegex(b *testing.B) {
	const (
		nLines = 200_000
		k      = 50
	)

	idx := benchFile(b, nLines, k)
	se := NewSearchEngine(idx)
	opts := SearchOptions{Query: "Exception|timeout", IsRegex: true, CaseSensitive: false}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if _, err := se.SearchWithContext(b.Context(), opts); err != nil {
			b.Fatalf("search: %v", err)
		}
	}
}

// BenchmarkGetLines exercises the per-chunk read path used by every search
// worker. 3000 lines is the chunk size used by matchRange.
func BenchmarkGetLines(b *testing.B) {
	const nLines = 200_000

	idx := benchFile(b, nLines, 50)

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if _, err := idx.GetLines(0, 3000); err != nil {
			b.Fatalf("GetLines: %v", err)
		}
	}
}

// benchFileCyrillic is the Cyrillic twin of benchFile: same shape and size,
// but the content (and the sparse token) are non-ASCII, so the case-insensitive
// search falls into the non-ASCII fold path instead of containsFoldASCIIBytes.
// Roughly one-in-every k lines contains "Виняток" (capitalized, so a lowercase
// query only matches when folding works).
func benchFileCyrillic(b *testing.B, nLines, k int) *indexer.FileIndexer { //nolint:dupl // benchmark helpers intentionally share setup
	b.Helper()
	dir := b.TempDir()
	p := filepath.Join(dir, "bench_uk.log")

	f, err := os.Create(p)
	if err != nil {
		b.Fatalf("create bench file: %v", err)
	}

	var sb strings.Builder
	for i := range nLines {
		sb.Reset()
		fmt.Fprintf(&sb, "2026-08-15Т10:%02d:%02d.000Z рівень=інфо служба=svc%d користувач=%08d ", i%60, i%60, i%7, i)

		if i%k == 0 {
			sb.WriteString("Виняток: збій у обробнику запитів трасування=abc123\n")
		} else {
			sb.WriteString("запит оброблено успішно затримка=12мс статус=200 трасування=def456\n")
		}

		if _, err := f.WriteString(sb.String()); err != nil {
			b.Fatalf("write bench file: %v", err)
		}
	}

	if err := f.Close(); err != nil {
		b.Fatalf("close bench file: %v", err)
	}

	idx, err := indexer.NewFileIndexer(p)
	if err != nil {
		b.Fatalf("create indexer: %v", err)
	}

	return idx
}

// BenchmarkSearchNonASCII exercises the non-ASCII case-insensitive path: the
// query is Cyrillic, so queryIsASCII is false and matching cannot use the
// ASCII fast path. This is where the per-line bytes.ToLower allocation lives.
func BenchmarkSearchNonASCII(b *testing.B) {
	const nLines = 200_000

	const k = 50 // ~4000 matches

	idx := benchFileCyrillic(b, nLines, k)
	se := NewSearchEngine(idx)
	opts := SearchOptions{Query: "виняток", CaseSensitive: false}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if _, err := se.SearchWithContext(b.Context(), opts); err != nil {
			b.Fatalf("search: %v", err)
		}
	}
}

// BenchmarkDenseMatchAll exercises the worst case for the chunk accumulators
// and the ordered merge: every single line matches, so each per-chunk slice
// grows to its full 3000-line capacity and slices.Concat reassembles 200k
// indices instead of ~4k.
func BenchmarkDenseMatchAll(b *testing.B) {
	const nLines = 200_000

	const k = 50 // k is irrelevant here: every line contains "request"

	idx := benchFile(b, nLines, k)
	se := NewSearchEngine(idx)
	opts := SearchOptions{Query: "request", CaseSensitive: false}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if _, err := se.SearchWithContext(b.Context(), opts); err != nil {
			b.Fatalf("search: %v", err)
		}
	}
}

// BenchmarkRegexCacheHit measures the steady-state regex path with the M1
// cache warm: the same pattern repeats on every iteration, so after the first
// pass regexp.Compile never runs again and the benchmark isolates scan cost
// plus cache lookup from compile cost.
func BenchmarkRegexCacheHit(b *testing.B) {
	const (
		nLines = 200_000
		k      = 50
	)

	idx := benchFile(b, nLines, k)
	se := NewSearchEngine(idx)
	opts := SearchOptions{Query: "status=200|Exception", IsRegex: true, CaseSensitive: false}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if _, err := se.SearchWithContext(b.Context(), opts); err != nil {
			b.Fatalf("search: %v", err)
		}
	}
}

// BenchmarkSearchWithLevelCounts measures the combined search + level stats
// pass. This should be faster than running Search + GetLevelStats separately.
func BenchmarkSearchWithLevelCounts(b *testing.B) {
	const nLines = 200_000
	const k = 50

	idx := benchFile(b, nLines, k)
	se := NewSearchEngine(idx)
	opts := SearchOptions{Query: "exception", CaseSensitive: false}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if _, err := se.SearchWithLevelCounts(b.Context(), opts); err != nil {
			b.Fatalf("search with level counts: %v", err)
		}
	}
}

// BenchmarkExpandWithContext pins the memory shape of context expansion
// (H2): 20k sparse matches with a 5+5 context window overlap heavily, so the
// dedupe structure (map + full preallocation vs running pointer) dominates
// the allocation profile.
func BenchmarkExpandWithContext(b *testing.B) {
	const nLines = 200_000

	idx := benchFile(b, nLines, 10)

	total := idx.TotalLines()
	matches := make([]int64, 0, total/10)

	for i := int64(0); i < total; i += 10 {
		matches = append(matches, i)
	}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		out := ExpandMatchesWithContext(matches, 5, 5, total)
		if len(out) == 0 {
			b.Fatal("empty expansion")
		}
	}
}

// BenchmarkFilterLines pins the secondary-refine pass (M1): one indexer read
// + match per primary hit, per invocation.
func BenchmarkFilterLines(b *testing.B) {
	const nLines = 200_000

	idx := benchFile(b, nLines, 10)
	se := NewSearchEngine(idx)

	primary, err := se.SearchWithContext(b.Context(), SearchOptions{Query: "request", CaseSensitive: false})
	if err != nil {
		b.Fatalf("primary search: %v", err)
	}

	opts := SearchOptions{SecondaryQuery: "status=200", CaseSensitive: false}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		out, err := se.FilterLines(b.Context(), opts, primary)
		if err != nil {
			b.Fatalf("filter: %v", err)
		}

		if len(out) == 0 {
			b.Fatal("no lines survived the filter")
		}
	}
}

// BenchmarkAdvancedFiltersLevelsTime exercises the post-filter pass with a
// Levels filter (per-line lineLevel/hasFold) and a time window (per-line
// parseLineTime) over a dense match set.
func BenchmarkAdvancedFiltersLevelsTime(b *testing.B) {
	const nLines = 200_000

	idx := benchFile(b, nLines, 2)
	se := NewSearchEngine(idx)

	primary, err := se.SearchWithContext(b.Context(), SearchOptions{Query: "exception", CaseSensitive: false})
	if err != nil {
		b.Fatalf("primary search: %v", err)
	}

	opts := SearchOptions{
		Levels:   []string{"OTHER"},
		TimeFrom: "2026-08-15T00:00:00Z",
		TimeTo:   "2026-08-16T00:00:00Z",
	}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if _, err := se.ApplyAdvancedFilters(b.Context(), opts, primary); err != nil {
			b.Fatalf("advanced filters: %v", err)
		}
	}
}

// BenchmarkParseLineTime pins the per-line timestamp parse cost of the
// time-filter path (M6): candidates construction plus time.Parse attempts.
func BenchmarkParseLineTime(b *testing.B) {
	line := "2026-08-15T10:00:00.000Z level=info svc=1 request handled ok latency=12ms status=200"

	b.ReportAllocs()

	for b.Loop() {
		if _, ok := parseLineTime(line); !ok {
			b.Fatal("timestamp not parsed")
		}
	}
}
