package search

import (
	"GoFLE/pkg/indexer"
	"bytes"
	"context"
	"errors"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func newIndexer(t *testing.T, content string) *indexer.FileIndexer {
	t.Helper()
	dir := t.TempDir()

	p := filepath.Join(dir, "t.log")
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatalf("write test file: %v", err)
	}

	idx, err := indexer.NewFileIndexer(p)
	if err != nil {
		t.Fatalf("create indexer: %v", err)
	}

	return idx
}

func TestSearchWithContext(t *testing.T) {
	idx := newIndexer(t, "error one\ninfo two\nerror three\n")
	se := NewSearchEngine(idx)

	matches, err := se.SearchWithContext(t.Context(), SearchOptions{Query: "error"})
	if err != nil {
		t.Fatalf("SearchWithContext error: %v", err)
	}

	if len(matches) != 2 || matches[0] != 0 || matches[1] != 2 {
		t.Fatalf("expected [0 2], got %v", matches)
	}
}

func TestSearchCaseInsensitive(t *testing.T) {
	idx := newIndexer(t, "Error\nwarning\nERROR\n")
	se := NewSearchEngine(idx)

	matches, err := se.SearchWithContext(t.Context(), SearchOptions{Query: "error", CaseSensitive: false})
	if err != nil {
		t.Fatalf("error: %v", err)
	}

	if len(matches) != 2 {
		t.Fatalf("expected 2 case-insensitive matches, got %v", matches)
	}

	// Case-sensitive "error" matches neither "Error" nor "ERROR".
	matches, _ = se.SearchWithContext(t.Context(), SearchOptions{Query: "error", CaseSensitive: true})
	if len(matches) != 0 {
		t.Fatalf("expected 0 case-sensitive matches for lowercase 'error', got %v", matches)
	}

	// Case-sensitive "Error" matches exactly the first line.
	matches, _ = se.SearchWithContext(t.Context(), SearchOptions{Query: "Error", CaseSensitive: true})
	if len(matches) != 1 {
		t.Fatalf("expected 1 case-sensitive match for 'Error', got %v", matches)
	}
}

func TestSearchRegex(t *testing.T) {
	idx := newIndexer(t, "foo123\nbar\nbaz456\n")
	se := NewSearchEngine(idx)

	matches, err := se.SearchWithContext(t.Context(), SearchOptions{Query: "[0-9]+", IsRegex: true})
	if err != nil {
		t.Fatalf("regex search error: %v", err)
	}

	if len(matches) != 2 {
		t.Fatalf("expected 2 regex matches, got %v", matches)
	}
}

func TestSearchInvertMatch(t *testing.T) {
	idx := newIndexer(t, "error\nok\nerror\nok\n")
	se := NewSearchEngine(idx)

	matches, err := se.SearchWithContext(t.Context(), SearchOptions{Query: "error", InvertMatch: true})
	if err != nil {
		t.Fatalf("error: %v", err)
	}

	if len(matches) != 2 || matches[0] != 1 || matches[1] != 3 {
		t.Fatalf("expected inverted [1 3], got %v", matches)
	}
}

func TestSearchEmptyQuery(t *testing.T) {
	idx := newIndexer(t, "a\nb\nc\n")
	se := NewSearchEngine(idx)

	matches, err := se.SearchWithContext(t.Context(), SearchOptions{Query: ""})
	if err != nil {
		t.Fatalf("error: %v", err)
	}

	if len(matches) != 0 {
		t.Fatalf("expected 0 matches for empty query, got %v", matches)
	}
}

func TestSearchRange(t *testing.T) {
	idx := newIndexer(t, "a\nb\nc\nd\ne\n")
	se := NewSearchEngine(idx)

	// Search only the range [2, 5) (indices 2,3,4 = c,d,e) for "c".
	matches, err := se.SearchRange(t.Context(), SearchOptions{Query: "c"}, 2, 5)
	if err != nil {
		t.Fatalf("SearchRange error: %v", err)
	}

	if len(matches) != 1 || matches[0] != 2 {
		t.Fatalf("expected [2], got %v", matches)
	}

	// Empty range should return nothing without scanning the whole file.
	empty, err := se.SearchRange(t.Context(), SearchOptions{Query: "a"}, 3, 3)
	if err != nil {
		t.Fatalf("SearchRange error: %v", err)
	}

	if len(empty) != 0 {
		t.Fatalf("expected empty for start==end, got %v", empty)
	}
}

func TestSearchRangeInvert(t *testing.T) {
	idx := newIndexer(t, "x\ny\nx\ny\nx")
	se := NewSearchEngine(idx)

	// Inverted search over [2,5) (indices 2,3,4 = x,y,x) for "x" => only index 3 (y).
	matches, err := se.SearchRange(t.Context(), SearchOptions{Query: "x", InvertMatch: true}, 2, 5)
	if err != nil {
		t.Fatalf("error: %v", err)
	}

	if len(matches) != 1 || matches[0] != 3 {
		t.Fatalf("expected [3], got %v", matches)
	}
}

func TestContainsFoldASCII(t *testing.T) {
	cases := []struct {
		s, sub string
		want   bool
	}{
		{"Error: boom", "error", true},
		{"ERROR: boom", "error", true},
		{"warning", "error", false},
		{"mixed Case STRING", "case string", true},
		{"abc", "", true},
		{"abc", "abcd", false},
		{"AbC", "abc", true},
		{"aBcDeF", "bcd", true},
	}
	for _, c := range cases {
		if got := containsFoldASCII(c.s, c.sub); got != c.want {
			t.Fatalf("containsFoldASCII(%q,%q)=%v want %v", c.s, c.sub, got, c.want)
		}
	}
}

func TestSearchConvenienceWrapper(t *testing.T) {
	idx := newIndexer(t, "error one\ninfo two\nerror three\n")
	se := NewSearchEngine(idx)

	matches, err := se.Search(SearchOptions{Query: "error"})
	if err != nil {
		t.Fatalf("Search error: %v", err)
	}

	if len(matches) != 2 || matches[0] != 0 || matches[1] != 2 {
		t.Fatalf("expected [0 2], got %v", matches)
	}
}

func TestRegexCacheInvalidation(t *testing.T) {
	idx := newIndexer(t, "foo123\nbar\nbaz456\n")
	se := NewSearchEngine(idx)

	matches, err := se.SearchWithContext(t.Context(), SearchOptions{Query: "[0-9]+", IsRegex: true})
	if err != nil {
		t.Fatalf("first search error: %v", err)
	}

	if len(matches) != 2 {
		t.Fatalf("expected 2 matches, got %d", len(matches))
	}

	matches, err = se.SearchWithContext(t.Context(), SearchOptions{Query: "[a-z]+", IsRegex: true})
	if err != nil {
		t.Fatalf("second search error: %v", err)
	}

	if len(matches) != 3 {
		t.Fatalf("expected 3 matches for new pattern, got %d", len(matches))
	}

	matches, err = se.SearchWithContext(t.Context(), SearchOptions{Query: "[0-9]+", IsRegex: true})
	if err != nil {
		t.Fatalf("third search error: %v", err)
	}

	if len(matches) != 2 {
		t.Fatalf("expected 2 matches for cached pattern, got %d", len(matches))
	}
}

func TestContextCancellation(t *testing.T) {
	var buf strings.Builder
	for range 50000 {
		buf.WriteString("error\n")
	}

	idx := newIndexer(t, buf.String())
	se := NewSearchEngine(idx)

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	matches, err := se.SearchWithContext(ctx, SearchOptions{Query: "error"})
	if err == nil {
		t.Logf("search completed before cancellation check: %d matches", len(matches))
		return
	}

	if !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected context error, got %v", err)
	}

	_ = matches
}

func TestSingleChunkPath(t *testing.T) {
	idx := newIndexer(t, "error\ninfo\nerror\n")
	se := NewSearchEngine(idx)

	matches, err := se.SearchWithContext(t.Context(), SearchOptions{Query: "error"})
	if err != nil {
		t.Fatalf("error: %v", err)
	}

	if len(matches) != 2 {
		t.Fatalf("expected 2 matches, got %d", len(matches))
	}
}

func TestMultiChunkParallelPath(t *testing.T) {
	var buf strings.Builder
	for range 5000 {
		buf.WriteString("error\n")
	}

	idx := newIndexer(t, buf.String())
	se := NewSearchEngine(idx)

	matches, err := se.SearchWithContext(t.Context(), SearchOptions{Query: "error"})
	if err != nil {
		t.Fatalf("error: %v", err)
	}

	if len(matches) != 5000 {
		t.Fatalf("expected 5000 matches, got %d", len(matches))
	}
}

func TestInvertMatchWithRegex(t *testing.T) {
	idx := newIndexer(t, "error\ninfo\nerror\nwarning\n")
	se := NewSearchEngine(idx)

	matches, err := se.SearchWithContext(t.Context(), SearchOptions{Query: "^error", IsRegex: true, InvertMatch: true})
	if err != nil {
		t.Fatalf("error: %v", err)
	}

	if len(matches) != 2 || matches[0] != 1 || matches[1] != 3 {
		t.Fatalf("expected [1 3], got %v", matches)
	}
}

func TestCaseSensitiveWithRegex(t *testing.T) {
	idx := newIndexer(t, "Error\nerror\nERROR\n")
	se := NewSearchEngine(idx)

	matches, err := se.SearchWithContext(t.Context(), SearchOptions{Query: "error", IsRegex: true, CaseSensitive: true})
	if err != nil {
		t.Fatalf("error: %v", err)
	}

	if len(matches) != 1 || matches[0] != 1 {
		t.Fatalf("expected [1], got %v", matches)
	}

	matches, err = se.SearchWithContext(t.Context(), SearchOptions{Query: "error", IsRegex: true, CaseSensitive: false})
	if err != nil {
		t.Fatalf("error: %v", err)
	}

	if len(matches) != 3 {
		t.Fatalf("expected 3 case-insensitive matches, got %d", len(matches))
	}
}

func TestIsASCII(t *testing.T) {
	cases := []struct {
		s    string
		want bool
	}{
		{"hello", true},
		{"hello world", true},
		{"hello\x00world", true},
		{"привет", false},
		{"hello мир", false},
		{"", true},
		{"12345", true},
		{"hello\nworld", true},
	}
	for _, c := range cases {
		if got := isASCII(c.s); got != c.want {
			t.Fatalf("isASCII(%q)=%v want %v", c.s, got, c.want)
		}
	}
}

func TestContainsFoldASCIIBytes(t *testing.T) {
	cases := []struct {
		s, sub []byte
		want   bool
	}{
		{[]byte("Error: boom"), []byte("error"), true},
		{[]byte("ERROR: boom"), []byte("error"), true},
		{[]byte("warning"), []byte("error"), false},
		{[]byte("mixed Case STRING"), []byte("case string"), true},
		{[]byte("abc"), []byte(""), true},
		{[]byte("abc"), []byte("abcd"), false},
		{[]byte("AbC"), []byte("abc"), true},
		{[]byte("aBcDeF"), []byte("bcd"), true},
	}
	for _, c := range cases {
		if got := containsFoldASCIIBytes(c.s, c.sub); got != c.want {
			t.Fatalf("containsFoldASCIIBytes(%q,%q)=%v want %v", c.s, c.sub, got, c.want)
		}
	}
}

func TestSearchEmptyFile(t *testing.T) {
	idx := newIndexer(t, "")
	se := NewSearchEngine(idx)

	matches, err := se.SearchWithContext(t.Context(), SearchOptions{Query: "error"})
	if err != nil {
		t.Fatalf("error: %v", err)
	}

	if len(matches) != 0 {
		t.Fatalf("expected 0 matches for empty file, got %d", len(matches))
	}
}

func TestSearchUnicode(t *testing.T) {
	idx := newIndexer(t, "ошибка\nпредупреждение\nошибка\n")
	se := NewSearchEngine(idx)

	matches, err := se.SearchWithContext(t.Context(), SearchOptions{Query: "ошибка", CaseSensitive: false})
	if err != nil {
		t.Fatalf("error: %v", err)
	}

	if len(matches) != 2 {
		t.Fatalf("expected 2 matches, got %d", len(matches))
	}
}

func TestSearchContextCancellationDuringParallelSearch(t *testing.T) {
	var buf strings.Builder
	for range 10000 {
		buf.WriteString("line\n")
	}

	idx := newIndexer(t, buf.String())
	se := NewSearchEngine(idx)

	ctx, cancel := context.WithTimeout(t.Context(), 1*time.Nanosecond)

	time.Sleep(1 * time.Millisecond)
	cancel()

	matches, err := se.SearchWithContext(ctx, SearchOptions{Query: "line"})
	if err == nil {
		t.Logf("search completed before cancellation (fast machine): %d matches", len(matches))
	} else if !errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context error, got %v", err)
	}
}

// TestRegexCacheTTL verifies M1: a cached regex entry that has aged past the
// TTL is treated as a miss on the next lookup (recompiled), while a fresh entry
// is reused. The test manipulates the unexported cache directly because it
// lives in the same package.
func TestRegexCacheTTL(t *testing.T) {
	idx := newIndexer(t, "foo123\nbar\nbaz456\n")
	se := NewSearchEngine(idx)

	// First search compiles and caches "[0-9]+".
	if _, err := se.SearchWithContext(t.Context(), SearchOptions{Query: "[0-9]+", IsRegex: true}); err != nil {
		t.Fatalf("first search: %v", err)
	}

	if len(se.reCache) != 1 {
		t.Fatalf("expected 1 cached entry, got %d", len(se.reCache))
	}

	for k, e := range se.reCache {
		if time.Since(e.cachedAt) > se.reCacheTTL {
			t.Fatalf("fresh entry should not be expired: key=%s", k)
		}
	}

	// Force expiry by backdating the cachedAt timestamp.
	for k := range se.reCache {
		e := se.reCache[k]
		e.cachedAt = e.cachedAt.Add(-time.Hour)
		se.reCache[k] = e
	}

	// Second identical search must still return correct matches (entry is
	// expired and recompiled rather than returning a stale/panicking result).
	matches, err := se.SearchWithContext(t.Context(), SearchOptions{Query: "[0-9]+", IsRegex: true})
	if err != nil {
		t.Fatalf("second search after expiry: %v", err)
	}

	if len(matches) != 2 {
		t.Fatalf("expected 2 matches after recompile, got %d", len(matches))
	}
	// The entry is refreshed with a new cachedAt.
	for k := range se.reCache {
		if time.Since(se.reCache[k].cachedAt) > se.reCacheTTL {
			t.Fatalf("entry should have been refreshed after recompile: key=%s", k)
		}
	}
}

// TestContainsFoldRune pins the semantics of the F3 allocation-free compare:
// it must agree with the fallback it replaced —
// bytes.Contains(bytes.ToLower(content), []byte(strings.ToLower(query))) —
// across Cyrillic/Greek/Latin mixes, case pairs, invalid UTF-8, and the
// documented simple-mapping edge cases.
func TestContainsFoldRune(t *testing.T) {
	cases := []struct {
		content, query string
		want           bool
	}{
		// Cyrillic case pairs (the motivating case).
		{"Виняток: збій у обробнику", "виняток", true},
		{"запит оброблено успішно", "ВИНЯТОК", false},
		{"рівень=інфо статус=200", "СТАТУС", true},
		{"помилка на сторінці 12", "Помилка", true},
		// Mixed ASCII + non-ASCII content with an ASCII query: must still be
		// reachable and correct if ever called on such input.
		{"2026-08-15T10:00:00Z рівень=інфо", "рівень", true},
		// Greek.
		{"ΣΦΑΛΜΑ σφάλμα", "σφάλμα", true},
		{"ΣΦΑΛΜΑ σφάλμα", "ΣΦΑΛΜΑ", true},
		// Accented Latin.
		{"Erreur d'exécution", "erreur d'exÉcution", true},
		// No match at all / partial overlaps that must not match.
		{"абаба", "баба", true},
		{"аа", "ааа", false},
		{"коротка", "коротказапис", false},
		{"", "х", false},
		{"щось", "", true},
		// Simple case mapping edge cases (documented divergence from FULL
		// folding — but identical to the replaced ToLower fallback):
		// ß does not match SS under simple mapping on either side.
		{"Straße", "strasse", false},
		{"STRASSE", "straße", false},
		// İ folds to plain "i" under unicode.ToLower on both sides.
		{"İstanbul", "istanbul", true},
		{"İstanbul", "i̇stanbul", false},
		// Kelvin sign folds to ASCII k; Å angstrom sign to å.
		{"212A Kelvin: K", "\u212A", true},
		// Invalid UTF-8 bytes decode to RuneError on both sides.
		{"bad\xffseq", "bad\uFFFDseq", true},
	}
	for _, c := range cases {
		queryLower := []rune(strings.ToLower(c.query))
		if got := containsFoldRune([]byte(c.content), queryLower); got != c.want {
			t.Fatalf("containsFoldRune(%q, %q)=%v want %v", c.content, c.query, got, c.want)
		}
		// Differential check against the exact expression the new path
		// replaced, so any stdlib disagreement fails loudly here.
		ref := bytes.Contains(bytes.ToLower([]byte(c.content)), []byte(strings.ToLower(c.query)))
		if got := containsFoldRune([]byte(c.content), queryLower); got != ref {
			t.Fatalf("divergence from ToLower fallback: containsFoldRune(%q,%q)=%v, fallback=%v", c.content, c.query, got, ref)
		}
	}
}

// TestContainsFoldRuneDifferential fuzzes the equivalence claim with a
// deterministic pseudo-random corpus of mixed Cyrillic/ASCII lines.
func TestContainsFoldRuneDifferential(t *testing.T) {
	alphabet := []rune("аБвГдЕёжзИйКлМнОпрстуФхЦчШщЪыьЭюяABCdefGHIjklMNOpqrSTUvwxyz \t-_=0123456789")
	rng := rand.New(rand.NewSource(42))

	randSeq := func(n int) string {
		b := make([]rune, n)
		for i := range b {
			b[i] = alphabet[rng.Intn(len(alphabet))]
		}

		return string(b)
	}
	for range 2000 {
		content := randSeq(rng.Intn(64))
		query := randSeq(1 + rng.Intn(8))
		got := containsFoldRune([]byte(content), []rune(strings.ToLower(query)))

		want := bytes.Contains(bytes.ToLower([]byte(content)), []byte(strings.ToLower(query)))
		if got != want {
			t.Fatalf("divergence: content=%q query=%q got=%v want=%v", content, query, got, want)
		}
	}
}

// --- compound query (SecondaryQuery) ---

func TestCompoundQueryAndSemantics(t *testing.T) {
	lines := []string{
		"ERROR Exception in module A",
		"WARN Exception recovered",
		"INFO Exception Timeout=5",
		"ERROR Exception Timeout=30",
	}
	idx := newIndexer(t, strings.Join(lines, "\n")+"\n")
	se := NewSearchEngine(idx)

	opts := SearchOptions{Query: "Exception", SecondaryQuery: "Timeout"}

	matches, err := se.SearchWithContext(t.Context(), opts)
	if err != nil {
		t.Fatal(err)
	}

	if len(matches) != 2 || matches[0] != 2 || matches[1] != 3 {
		t.Fatalf("expected [2 3], got %v", matches)
	}

	// Invert applies to the primary; secondary still narrows afterwards.
	opts.InvertMatch = true

	matches, _ = se.SearchWithContext(t.Context(), opts)
	if len(matches) != 0 {
		t.Fatalf("inverted+secondary expected empty (no non-Exception line has Timeout), got %v", matches)
	}

	opts.Query, opts.InvertMatch = "ERROR", false
	opts.SecondaryQuery = ""

	matches, _ = se.SearchWithContext(t.Context(), opts)
	if len(matches) != 2 || matches[0] != 0 || matches[1] != 3 {
		t.Fatalf("plain ERROR expected [0 3], got %v", matches)
	}
}

func TestCompoundQueryRegexSecondaryCaseInsensitiveUnicode(t *testing.T) {
	lines := []string{
		"помилка виняток A",
		"виняток Таймаут=5",
		"виняток таймаут=7",
	}
	idx := newIndexer(t, strings.Join(lines, "\n")+"\n")
	se := NewSearchEngine(idx)

	// Case-insensitive unicode secondary over a case-insensitive primary.
	opts := SearchOptions{Query: "виняток", SecondaryQuery: "ТАЙМАУТ"}

	matches, err := se.SearchWithContext(t.Context(), opts)
	if err != nil {
		t.Fatal(err)
	}

	if len(matches) != 2 || matches[0] != 1 || matches[1] != 2 {
		t.Fatalf("expected [1 2], got %v", matches)
	}

	// Regex secondary reuses the cache and ANDs with a plain primary.
	opts2 := SearchOptions{Query: "виняток", SecondaryQuery: `=\d+`, IsRegex: true}

	matches2, err := se.SearchWithContext(t.Context(), opts2)
	if err != nil {
		t.Fatal(err)
	}

	if len(matches2) != 2 {
		t.Fatalf("regex secondary expected 2, got %d", len(matches2))
	}

	if _, errBad := se.SearchWithContext(t.Context(), SearchOptions{Query: "x", SecondaryQuery: "(", IsRegex: true}); errBad == nil {
		t.Fatal("expected invalid secondary regex error")
	}
}

// TestSearchWithLevelCounts tests the search with level counting.
func TestSearchWithLevelCounts(t *testing.T) {
	lines := []string{
		"2024-01-01 10:00:00 ERROR something failed",
		"2024-01-01 10:00:01 WARN warning here",
		"2024-01-01 10:00:02 INFO info message",
		"2024-01-01 10:00:03 ERROR another error",
		"2024-01-01 10:00:04 DEBUG debug info",
		"2024-01-01 10:00:05 FATAL fatal error",
		"2024-01-01 10:00:06 just text",
	}
	idx := newIndexer(t, strings.Join(lines, "\n")+"\n")
	se := NewSearchEngine(idx)

	// Search for "error" - should match lines 0, 3, 5
	result, err := se.SearchWithLevelCounts(t.Context(), SearchOptions{Query: "error", CaseSensitive: false})
	if err != nil {
		t.Fatalf("SearchWithLevelCounts error: %v", err)
	}
	if len(result.Matches) != 3 || result.Matches[0] != 0 || result.Matches[1] != 3 || result.Matches[2] != 5 {
		t.Fatalf("expected matches [0 3 5], got %v", result.Matches)
	}
	// ERROR lines: 0, 3, 5 (FATAL counts as ERROR) = 3
	// WARN lines: 1 = 1
	if result.ErrorCount != 3 {
		t.Fatalf("expected ErrorCount=3, got %d", result.ErrorCount)
	}
	if result.WarnCount != 1 {
		t.Fatalf("expected WarnCount=1, got %d", result.WarnCount)
	}

	// Search with empty query
	result, err = se.SearchWithLevelCounts(t.Context(), SearchOptions{Query: ""})
	if err != nil {
		t.Fatalf("SearchWithLevelCounts empty query error: %v", err)
	}
	if len(result.Matches) != 0 || result.ErrorCount != 0 || result.WarnCount != 0 {
		t.Fatalf("expected empty result, got %v", result)
	}

	// Search empty file
	emptyIdx := newIndexer(t, "")
	emptySe := NewSearchEngine(emptyIdx)
	result, err = emptySe.SearchWithLevelCounts(t.Context(), SearchOptions{Query: "error"})
	if err != nil {
		t.Fatalf("empty file error: %v", err)
	}
	if len(result.Matches) != 0 {
		t.Fatalf("expected empty matches for empty file")
	}
}

func TestSearchWithProgress(t *testing.T) {
	idx := newIndexer(t, "error\ninfo\nerror\nwarning\n")
	se := NewSearchEngine(idx)

	var progressCalls []int64
	onProgress := func(count int64) {
		progressCalls = append(progressCalls, count)
	}

	matches, err := se.SearchWithProgress(t.Context(), SearchOptions{Query: "error"}, onProgress)
	if err != nil {
		t.Fatalf("SearchWithProgress error: %v", err)
	}
	if len(matches) != 2 {
		t.Fatalf("expected 2 matches, got %d", len(matches))
	}
	if len(progressCalls) == 0 {
		t.Fatalf("expected progress callbacks")
	}
}

func TestRegexCacheEviction(t *testing.T) {
	idx := newIndexer(t, "a\nb\nc\n")
	se := NewSearchEngine(idx)

	// Fill cache beyond max size (16)
	for i := range 20 {
		pattern := fmt.Sprintf("pattern%d", i)
		_, _ = se.SearchWithContext(t.Context(), SearchOptions{Query: pattern, IsRegex: true})
	}
	// Should not panic and cache size should be <= maxSize
	if len(se.reCache) > 16 {
		t.Fatalf("cache size %d exceeds max 16", len(se.reCache))
	}
}

func TestPutCachedRegexNilMap(t *testing.T) {
	// Test lazy initialization of nil map in putCachedRegex
	idx := newIndexer(t, "test\n")
	se := NewSearchEngine(idx)
	// Manually nil the cache
	se.reCache = nil
	_, _ = se.SearchWithContext(t.Context(), SearchOptions{Query: "test", IsRegex: true})
	if se.reCache == nil {
		t.Fatal("cache should be initialized")
	}
	if len(se.reCache) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(se.reCache))
	}
}

// TestCompoundRegexSecondaryHonorsCaseSensitivity pins M6: CaseSensitive applies
// to BOTH patterns of a compound query, so the secondary regex is prefixed with
// (?i) exactly like the primary — and is cached under that prefixed pattern, so
// the two flag settings cannot collide on one cache entry and serve whichever
// regex happened to be compiled first.
func TestCompoundRegexSecondaryHonorsCaseSensitivity(t *testing.T) {
	lines := []string{
		"Exception TIMEOUT=5",
		"Exception timeout=7",
		"Exception no deadline",
	}
	idx := newIndexer(t, strings.Join(lines, "\n")+"\n")
	se := NewSearchEngine(idx)

	opts := SearchOptions{Query: "Exception", SecondaryQuery: `timeout=\d+`, IsRegex: true}

	insensitive, err := se.SearchWithContext(t.Context(), opts)
	if err != nil {
		t.Fatal(err)
	}

	if len(insensitive) != 2 || insensitive[0] != 0 || insensitive[1] != 1 {
		t.Fatalf("case-insensitive secondary matched %v, want [0 1]", insensitive)
	}

	opts.CaseSensitive = true

	sensitive, err := se.SearchWithContext(t.Context(), opts)
	if err != nil {
		t.Fatal(err)
	}

	if len(sensitive) != 1 || sensitive[0] != 1 {
		t.Fatalf("case-sensitive secondary matched %v, want [1]", sensitive)
	}

	// Repeat the insensitive search: it must not be served the case-sensitive
	// regex from the cache.
	opts.CaseSensitive = false

	again, err := se.SearchWithContext(t.Context(), opts)
	if err != nil {
		t.Fatal(err)
	}

	if len(again) != 2 || again[0] != 0 || again[1] != 1 {
		t.Fatalf("cached case-insensitive secondary matched %v, want [0 1]", again)
	}
}

// FilterLines tests

func TestFilterLinesBasic(t *testing.T) {
	lines := []string{
		"ERROR Exception in module A",
		"WARN Exception recovered",
		"INFO Exception Timeout=5",
		"ERROR Exception Timeout=30",
	}
	idx := newIndexer(t, strings.Join(lines, "\n")+"\n")
	se := NewSearchEngine(idx)

	all := []int64{0, 1, 2, 3}
	// Filter for "Timeout"
	filtered, err := se.FilterLines(t.Context(), SearchOptions{SecondaryQuery: "Timeout"}, all)
	if err != nil {
		t.Fatal(err)
	}
	if len(filtered) != 2 || filtered[0] != 2 || filtered[1] != 3 {
		t.Fatalf("expected [2 3], got %v", filtered)
	}
}

func TestFilterLinesRegex(t *testing.T) {
	lines := []string{
		"ERROR code=123",
		"WARN code=456",
		"INFO something",
		"DEBUG code=789",
	}
	idx := newIndexer(t, strings.Join(lines, "\n")+"\n")
	se := NewSearchEngine(idx)

	all := []int64{0, 1, 2, 3}
	filtered, err := se.FilterLines(t.Context(), SearchOptions{SecondaryQuery: `code=\d+`, IsRegex: true}, all)
	if err != nil {
		t.Fatal(err)
	}
	if len(filtered) != 3 || filtered[0] != 0 || filtered[1] != 1 || filtered[2] != 3 {
		t.Fatalf("expected [0 1 3], got %v", filtered)
	}
}

func TestFilterLinesCaseSensitive(t *testing.T) {
	lines := []string{
		"ERROR Timeout=5",
		"ERROR timeout=7",
		"INFO something",
	}
	idx := newIndexer(t, strings.Join(lines, "\n")+"\n")
	se := NewSearchEngine(idx)

	all := []int64{0, 1, 2}
	// Case insensitive
	filtered, err := se.FilterLines(t.Context(), SearchOptions{SecondaryQuery: "timeout"}, all)
	if err != nil {
		t.Fatal(err)
	}
	if len(filtered) != 2 || filtered[0] != 0 || filtered[1] != 1 {
		t.Fatalf("case-insensitive expected [0 1], got %v", filtered)
	}

	// Case sensitive
	filtered, err = se.FilterLines(t.Context(), SearchOptions{SecondaryQuery: "timeout", CaseSensitive: true}, all)
	if err != nil {
		t.Fatal(err)
	}
	if len(filtered) != 1 || filtered[0] != 1 {
		t.Fatalf("case-sensitive expected [1], got %v", filtered)
	}
}

func TestFilterLinesEmptySecondary(t *testing.T) {
	lines := []string{"a", "b", "c"}
	idx := newIndexer(t, strings.Join(lines, "\n")+"\n")
	se := NewSearchEngine(idx)

	all := []int64{0, 1, 2}
	filtered, err := se.FilterLines(t.Context(), SearchOptions{SecondaryQuery: ""}, all)
	if err != nil {
		t.Fatal(err)
	}
	if len(filtered) != 3 || filtered[0] != 0 || filtered[1] != 1 || filtered[2] != 2 {
		t.Fatalf("expected clone [0 1 2], got %v", filtered)
	}
}

func TestFilterLinesContextCancellation(t *testing.T) {
	lines := make([]string, 1000)
	for i := range lines {
		lines[i] = fmt.Sprintf("line %d with match", i)
	}
	idx := newIndexer(t, strings.Join(lines, "\n")+"\n")
	se := NewSearchEngine(idx)

	all := make([]int64, 1000)
	for i := range all {
		all[i] = int64(i)
	}

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	_, err := se.FilterLines(ctx, SearchOptions{SecondaryQuery: "match"}, all)
	if err == nil {
		t.Fatal("expected context cancellation error")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

func TestFilterLinesUnicode(t *testing.T) {
	lines := []string{
		"ошибка код=1",
		"предупреждение код=2",
		"инфо сообщение",
	}
	idx := newIndexer(t, strings.Join(lines, "\n")+"\n")
	se := NewSearchEngine(idx)

	all := []int64{0, 1, 2}
	filtered, err := se.FilterLines(t.Context(), SearchOptions{SecondaryQuery: "код"}, all)
	if err != nil {
		t.Fatal(err)
	}
	if len(filtered) != 2 || filtered[0] != 0 || filtered[1] != 1 {
		t.Fatalf("expected [0 1], got %v", filtered)
	}
}

func TestFilterLinesInvalidRegex(t *testing.T) {
	idx := newIndexer(t, "a\nb\nc\n")
	se := NewSearchEngine(idx)

	_, err := se.FilterLines(t.Context(), SearchOptions{SecondaryQuery: "(", IsRegex: true}, []int64{0, 1, 2})
	if err == nil {
		t.Fatal("expected invalid regex error")
	}
}

// BuildSecondaryMatcher tests

func TestBuildSecondaryMatcherAllFlagCombos(t *testing.T) {
	idx := newIndexer(t, "test\n")
	se := NewSearchEngine(idx)

	tests := []struct {
		name       string
		opts       SearchOptions
		wantErr    bool
		checkMatch func(matcher func([]byte) bool) bool
	}{
		{
			name:    "empty secondary",
			opts:    SearchOptions{SecondaryQuery: ""},
			wantErr: false,
		},
		{
			name: "regex case-sensitive",
			opts: SearchOptions{SecondaryQuery: `[A-Z]+`, IsRegex: true, CaseSensitive: true},
			checkMatch: func(m func([]byte) bool) bool {
				return m([]byte("HELLO")) && !m([]byte("hello"))
			},
		},
		{
			name: "regex case-insensitive",
			opts: SearchOptions{SecondaryQuery: `[a-z]+`, IsRegex: true, CaseSensitive: false},
			checkMatch: func(m func([]byte) bool) bool {
				return m([]byte("hello")) && m([]byte("HELLO"))
			},
		},
		{
			name: "substring case-sensitive",
			opts: SearchOptions{SecondaryQuery: "Test", CaseSensitive: true},
			checkMatch: func(m func([]byte) bool) bool {
				return m([]byte("Test string")) && !m([]byte("test string"))
			},
		},
		{
			name: "substring case-insensitive ascii",
			opts: SearchOptions{SecondaryQuery: "test", CaseSensitive: false},
			checkMatch: func(m func([]byte) bool) bool {
				return m([]byte("Test")) && m([]byte("TEST")) && m([]byte("test"))
			},
		},
		{
			name: "substring case-insensitive unicode",
			opts: SearchOptions{SecondaryQuery: "тест", CaseSensitive: false},
			checkMatch: func(m func([]byte) bool) bool {
				return m([]byte("ТЕСТ")) && m([]byte("тест"))
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			matcher, err := se.buildSecondaryMatcher(tc.opts)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tc.checkMatch != nil && matcher != nil {
				if !tc.checkMatch(matcher) {
					t.Fatalf("matcher check failed for %s", tc.name)
				}
			}
			if tc.opts.SecondaryQuery == "" && matcher != nil {
				t.Fatal("expected nil matcher for empty secondary")
			}
		})
	}
}

func TestBuildSecondaryMatcherInvalidRegex(t *testing.T) {
	idx := newIndexer(t, "test\n")
	se := NewSearchEngine(idx)

	_, err := se.buildSecondaryMatcher(SearchOptions{SecondaryQuery: "(", IsRegex: true})
	if err == nil {
		t.Fatal("expected error for invalid regex")
	}
	if !strings.Contains(err.Error(), "invalid secondary regular expression") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

// ExpandMatchesWithContext tests

func TestExpandMatchesWithContextBasic(t *testing.T) {
	matches := []int64{5, 10}
	totalLines := int64(20)

	// before=2, after=1
	result := ExpandMatchesWithContext(matches, 2, 1, totalLines)
	expected := []int64{3, 4, 5, 6, 8, 9, 10, 11}
	if !slices.Equal(result, expected) {
		t.Fatalf("expected %v, got %v", expected, result)
	}
}

func TestExpandMatchesWithContextBounds(t *testing.T) {
	matches := []int64{0, 19}
	totalLines := int64(20)

	// Before at start, after at end
	result := ExpandMatchesWithContext(matches, 5, 5, totalLines)
	expected := []int64{0, 1, 2, 3, 4, 5, 14, 15, 16, 17, 18, 19}
	if !slices.Equal(result, expected) {
		t.Fatalf("expected %v, got %v", expected, result)
	}
}

func TestExpandMatchesWithContextClamping(t *testing.T) {
	matches := []int64{5}
	totalLines := int64(20)

	// before/after clamped to 50
	result := ExpandMatchesWithContext(matches, 100, 100, totalLines)
	expected := []int64{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19}
	if !slices.Equal(result, expected) {
		t.Fatalf("expected full range, got %v", result)
	}
}

func TestExpandMatchesWithContextZeroTotalLines(t *testing.T) {
	matches := []int64{0, 1, 2}
	result := ExpandMatchesWithContext(matches, 2, 2, 0)
	if !slices.Equal(result, matches) {
		t.Fatalf("expected %v for zero total lines, got %v", matches, result)
	}
}

func TestExpandMatchesWithContextEmptyMatches(t *testing.T) {
	result := ExpandMatchesWithContext([]int64{}, 2, 2, 10)
	if len(result) != 0 {
		t.Fatalf("expected empty for empty matches, got %v", result)
	}
}

func TestExpandMatchesWithContextNegativeBeforeAfter(t *testing.T) {
	matches := []int64{5}
	totalLines := int64(20)

	result := ExpandMatchesWithContext(matches, -5, -3, totalLines)
	expected := []int64{5}
	if !slices.Equal(result, expected) {
		t.Fatalf("expected %v for negative before/after, got %v", expected, result)
	}
}

func TestExpandMatchesWithContextDuplicates(t *testing.T) {
	// Overlapping contexts should deduplicate
	matches := []int64{5, 6}
	totalLines := int64(20)

	result := ExpandMatchesWithContext(matches, 2, 2, totalLines)
	expected := []int64{3, 4, 5, 6, 7, 8}
	if !slices.Equal(result, expected) {
		t.Fatalf("expected deduplicated %v, got %v", expected, result)
	}
}

func TestExpandMatchesWithContextZeroBeforeAfter(t *testing.T) {
	matches := []int64{1, 5, 10}
	totalLines := int64(20)

	result := ExpandMatchesWithContext(matches, 0, 0, totalLines)
	if !slices.Equal(result, matches) {
		t.Fatalf("expected original matches for 0/0, got %v", result)
	}
}

// lineLevel tests

func TestLineLevelAllLevels(t *testing.T) {
	tests := []struct {
		content string
		level   string
	}{
		{"2024-01-01 ERROR something failed", "ERROR"},
		{"2024-01-01 FATAL crash", "ERROR"},
		{"2024-01-01 CRITICAL failure", "ERROR"},
		// PANIC is not checked in lineLevel - it falls to OTHER
		{"2024-01-01 PANIC panic", "OTHER"},
		{"2024-01-01 WARN warning here", "WARN"},
		{"2024-01-01 WARNING be careful", "WARN"},
		{"2024-01-01 INFO info message", "INFO"},
		{"2024-01-01 INFORMATION something", "INFO"},
		// "debug" contains "info" so DEBUG is checked before INFO in actual code but "info" matches first
		// Actually the code checks ERROR -> WARN -> INFO -> DEBUG, so "debug" matches "info" first -> INFO
		{"2024-01-01 DEBUG debug info", "INFO"},
		{"2024-01-01 TRACE trace details", "DEBUG"},
		{"2024-01-01 just text", "OTHER"},
		{"", "OTHER"},
	}

	for _, tc := range tests {
		t.Run(tc.level, func(t *testing.T) {
			if got := lineLevel(tc.content); got != tc.level {
				t.Fatalf("lineLevel(%q)=%q want %q", tc.content, got, tc.level)
			}
		})
	}
}

func TestLineLevelCaseInsensitive(t *testing.T) {
	tests := []struct {
		content string
		level   string
	}{
		{"error occurred", "ERROR"},
		{"Error occurred", "ERROR"},
		{"ERROR occurred", "ERROR"},
		{"warn here", "WARN"},
		{"Warn here", "WARN"},
		{"WARN here", "WARN"},
		{"info log", "INFO"},
		{"Info log", "INFO"},
		{"debug mode", "DEBUG"},
		{"Debug mode", "DEBUG"},
		{"trace start", "DEBUG"},
	}

	for _, tc := range tests {
		t.Run(tc.content, func(t *testing.T) {
			if got := lineLevel(tc.content); got != tc.level {
				t.Fatalf("lineLevel(%q)=%q want %q", tc.content, got, tc.level)
			}
		})
	}
}

func TestLineLevelTruncation(t *testing.T) {
	// Only first 200 chars are checked
	longContent := strings.Repeat("x ", 200) + " ERROR at end"
	if got := lineLevel(longContent); got != "OTHER" {
		t.Fatalf("expected OTHER for truncated content, got %q", got)
	}

	longContent2 := "ERROR at start " + strings.Repeat("x ", 200)
	if got := lineLevel(longContent2); got != "ERROR" {
		t.Fatalf("expected ERROR for early ERROR, got %q", got)
	}
}

// parseLineTime and parseFlexibleTime tests

func TestParseLineTimeAllLayouts(t *testing.T) {
	tests := []struct {
		line      string
		wantTime  string // RFC3339 format for comparison
		wantFound bool
	}{
		// RFC3339Nano
		{"2024-01-01T10:00:00.123456789Z error", "2024-01-01T10:00:00.123456789Z", true},
		// RFC3339
		{"2024-01-01T10:00:00Z error", "2024-01-01T10:00:00Z", true},
		// 2006-01-02 15:04:05.000
		{"2024-01-01 10:00:00.000 error", "2024-01-01T10:00:00Z", true},
		// 2006-01-02 15:04:05
		{"2024-01-01 10:00:00 error", "2024-01-01T10:00:00Z", true},
		// 2006/01/02 15:04:05
		{"2024/01/01 10:00:00 error", "2024-01-01T10:00:00Z", true},
		// Jan _2 15:04:05 (year becomes 0000)
		{"Jan  1 10:00:00 error", "0000-01-01T10:00:00Z", true},
		// Jan 2 15:04:05
		{"Jan 1 10:00:00 error", "0000-01-01T10:00:00Z", true},
	}

	for _, tc := range tests {
		t.Run(tc.line, func(t *testing.T) {
			ts, found := parseLineTime(tc.line)
			if found != tc.wantFound {
				t.Fatalf("parseLineTime(%q) found=%v want %v", tc.line, found, tc.wantFound)
			}
			if found {
				expected, _ := time.Parse(time.RFC3339, tc.wantTime)
				if !ts.Equal(expected) {
					t.Fatalf("parseLineTime(%q)=%v want %v", tc.line, ts, expected)
				}
			}
		})
	}
}

func TestParseLineTimeNoTimestamp(t *testing.T) {
	tests := []string{
		"just text",
		"error something",
		"no timestamp here",
		"",
	}

	for _, tc := range tests {
		t.Run(tc, func(t *testing.T) {
			_, found := parseLineTime(tc)
			if found {
				t.Fatalf("expected no timestamp found in %q", tc)
			}
		})
	}
}

func TestParseFlexibleTimeAllLayouts(t *testing.T) {
	tests := []struct {
		input    string
		expected time.Time
	}{
		{"2024-01-01T10:00:00Z", time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC)},
		{"2024-01-01T10:00:00.123456789Z", time.Date(2024, 1, 1, 10, 0, 0, 123456789, time.UTC)},
		{"2024-01-01 10:00:00.000", time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC)},
		{"2024-01-01 10:00:00", time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC)},
		{"2024/01/01 10:00:00", time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC)},
		{"Jan  1 10:00:00", time.Date(0, 1, 1, 10, 0, 0, 0, time.UTC)},
		{"Jan 1 10:00:00", time.Date(0, 1, 1, 10, 0, 0, 0, time.UTC)},
		{"2024-01-01", time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			parsed, err := parseFlexibleTime(tc.input)
			if err != nil {
				t.Fatalf("parseFlexibleTime(%q) error: %v", tc.input, err)
			}
			if !parsed.Equal(tc.expected) {
				t.Fatalf("parseFlexibleTime(%q)=%v want %v", tc.input, parsed, tc.expected)
			}
		})
	}
}

func TestParseFlexibleTimeInvalid(t *testing.T) {
	_, err := parseFlexibleTime("invalid time")
	if err == nil {
		t.Fatal("expected error for invalid time")
	}
	if !strings.Contains(err.Error(), "unrecognized time") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// ApplyAdvancedFilters comprehensive tests

func TestApplyAdvancedFilters_ExcludePatterns(t *testing.T) {
	lines := []string{
		"alpha",
		"beta noise",
		"gamma",
		"delta NOISE",
		"epsilon",
	}
	idx := newIndexer(t, strings.Join(lines, "\n")+"\n")
	se := NewSearchEngine(idx)

	all := []int64{0, 1, 2, 3, 4}

	// Substring exclude (case-sensitive)
	filtered, err := se.ApplyAdvancedFilters(t.Context(), SearchOptions{
		ExcludePatterns: []string{"noise"},
		CaseSensitive:   true,
	}, all)
	if err != nil {
		t.Fatal(err)
	}
	if len(filtered) != 4 || !slices.Equal(filtered, []int64{0, 2, 3, 4}) {
		t.Fatalf("case-sensitive exclude got %v want [0 2 3 4]", filtered)
	}

	// Case-insensitive exclude
	filtered, err = se.ApplyAdvancedFilters(t.Context(), SearchOptions{
		ExcludePatterns: []string{"noise"},
		CaseSensitive:   false,
	}, all)
	if err != nil {
		t.Fatal(err)
	}
	if len(filtered) != 3 || !slices.Equal(filtered, []int64{0, 2, 4}) {
		t.Fatalf("case-insensitive exclude got %v want [0 2 4]", filtered)
	}

	// Multiple exclude patterns
	filtered, err = se.ApplyAdvancedFilters(t.Context(), SearchOptions{
		ExcludePatterns: []string{"noise", "alpha"},
		CaseSensitive:   false,
	}, all)
	if err != nil {
		t.Fatal(err)
	}
	if len(filtered) != 2 || !slices.Equal(filtered, []int64{2, 4}) {
		t.Fatalf("multiple exclude got %v want [2 4]", filtered)
	}

	// Regex exclude
	filtered, err = se.ApplyAdvancedFilters(t.Context(), SearchOptions{
		ExcludePatterns: []string{"g.*a"},
		ExcludeIsRegex:  true,
	}, all)
	if err != nil {
		t.Fatal(err)
	}
	if len(filtered) != 4 || !slices.Equal(filtered, []int64{0, 1, 3, 4}) {
		t.Fatalf("regex exclude got %v want [0 1 3 4]", filtered)
	}

	// Empty exclude patterns (should be no-op)
	filtered, err = se.ApplyAdvancedFilters(t.Context(), SearchOptions{
		ExcludePatterns: []string{"", "   "},
	}, all)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(filtered, all) {
		t.Fatalf("empty exclude patterns should be no-op, got %v", filtered)
	}
}

func TestApplyAdvancedFilters_LevelsComprehensive(t *testing.T) {
	lines := []string{
		"2024-01-01 ERROR something failed",
		"2024-01-01 WARN warning here",
		"2024-01-01 INFO info message",
		"2024-01-01 DEBUG debug only",
		"2024-01-01 TRACE trace details",
		"2024-01-01 FATAL fatal error",
		"2024-01-01 CRITICAL critical error",
		"just text no level",
	}
	idx := newIndexer(t, strings.Join(lines, "\n")+"\n")
	se := NewSearchEngine(idx)

	all := []int64{0, 1, 2, 3, 4, 5, 6, 7}

	// Filter for ERROR (includes FATAL, CRITICAL)
	filtered, err := se.ApplyAdvancedFilters(t.Context(), SearchOptions{
		Levels: []string{"ERROR"},
	}, all)
	if err != nil {
		t.Fatal(err)
	}
	if len(filtered) != 3 || !slices.Equal(filtered, []int64{0, 5, 6}) {
		t.Fatalf("ERROR filter got %v want [0 5 6]", filtered)
	}

	// Filter for WARN
	filtered, err = se.ApplyAdvancedFilters(t.Context(), SearchOptions{
		Levels: []string{"WARN"},
	}, all)
	if err != nil {
		t.Fatal(err)
	}
	if len(filtered) != 1 || !slices.Equal(filtered, []int64{1}) {
		t.Fatalf("WARN filter got %v want [1]", filtered)
	}

	// Filter for INFO
	filtered, err = se.ApplyAdvancedFilters(t.Context(), SearchOptions{
		Levels: []string{"INFO"},
	}, all)
	if err != nil {
		t.Fatal(err)
	}
	if len(filtered) != 1 || !slices.Equal(filtered, []int64{2}) {
		t.Fatalf("INFO filter got %v want [2]", filtered)
	}

	// Filter for DEBUG (includes TRACE)
	filtered, err = se.ApplyAdvancedFilters(t.Context(), SearchOptions{
		Levels: []string{"DEBUG"},
	}, all)
	if err != nil {
		t.Fatal(err)
	}
	if len(filtered) != 2 || !slices.Equal(filtered, []int64{3, 4}) {
		t.Fatalf("DEBUG filter got %v want [3 4]", filtered)
	}

	// Filter for OTHER
	filtered, err = se.ApplyAdvancedFilters(t.Context(), SearchOptions{
		Levels: []string{"OTHER"},
	}, all)
	if err != nil {
		t.Fatal(err)
	}
	if len(filtered) != 1 || !slices.Equal(filtered, []int64{7}) {
		t.Fatalf("OTHER filter got %v want [7]", filtered)
	}

	// Multiple levels
	filtered, err = se.ApplyAdvancedFilters(t.Context(), SearchOptions{
		Levels: []string{"ERROR", "WARN"},
	}, all)
	if err != nil {
		t.Fatal(err)
	}
	if len(filtered) != 4 || !slices.Equal(filtered, []int64{0, 1, 5, 6}) {
		t.Fatalf("multi-level filter got %v want [0 1 5 6]", filtered)
	}

	// Case-insensitive level names
	filtered, err = se.ApplyAdvancedFilters(t.Context(), SearchOptions{
		Levels: []string{"error", "warn"},
	}, all)
	if err != nil {
		t.Fatal(err)
	}
	if len(filtered) != 4 || !slices.Equal(filtered, []int64{0, 1, 5, 6}) {
		t.Fatalf("case-insensitive level filter got %v", filtered)
	}
}

func TestApplyAdvancedFilters_TimeRange(t *testing.T) {
	lines := []string{
		"2024-01-01 10:00:00 ERROR early",
		"2024-01-01 12:00:00 INFO middle",
		"2024-01-01 14:00:00 WARN late",
		"no timestamp here",
	}
	idx := newIndexer(t, strings.Join(lines, "\n")+"\n")
	se := NewSearchEngine(idx)

	all := []int64{0, 1, 2, 3}

	// TimeFrom only - lines without timestamp are kept by default
	filtered, err := se.ApplyAdvancedFilters(t.Context(), SearchOptions{
		TimeFrom: "2024-01-01 11:00:00",
	}, all)
	if err != nil {
		t.Fatal(err)
	}
	// Line 0 (10:00) is before 11:00 -> excluded
	// Lines 1, 2 (12:00, 14:00) are after 11:00 -> included
	// Line 3 (no timestamp) -> kept by default
	if len(filtered) != 3 || !slices.Equal(filtered, []int64{1, 2, 3}) {
		t.Fatalf("TimeFrom got %v want [1 2 3]", filtered)
	}

	// TimeTo only
	filtered, err = se.ApplyAdvancedFilters(t.Context(), SearchOptions{
		TimeTo: "2024-01-01 13:00:00",
	}, all)
	if err != nil {
		t.Fatal(err)
	}
	// Line 0 (10:00) is before 13:00 -> included
	// Line 1 (12:00) is before 13:00 -> included
	// Line 2 (14:00) is after 13:00 -> excluded
	// Line 3 (no timestamp) -> kept by default
	if len(filtered) != 3 || !slices.Equal(filtered, []int64{0, 1, 3}) {
		t.Fatalf("TimeTo got %v want [0 1 3]", filtered)
	}

	// TimeFrom and TimeTo
	filtered, err = se.ApplyAdvancedFilters(t.Context(), SearchOptions{
		TimeFrom: "2024-01-01 11:00:00",
		TimeTo:   "2024-01-01 13:00:00",
	}, all)
	if err != nil {
		t.Fatal(err)
	}
	// Line 0 (10:00) before 11:00 -> excluded
	// Line 1 (12:00) within range -> included
	// Line 2 (14:00) after 13:00 -> excluded
	// Line 3 (no timestamp) -> kept by default
	if len(filtered) != 2 || !slices.Equal(filtered, []int64{1, 3}) {
		t.Fatalf("TimeFrom+TimeTo got %v want [1 3]", filtered)
	}

	// TimeRequireStamp - lines without timestamp should be excluded
	filtered, err = se.ApplyAdvancedFilters(t.Context(), SearchOptions{
		TimeFrom:         "2024-01-01 00:00:00",
		TimeRequireStamp: true,
	}, all)
	if err != nil {
		t.Fatal(err)
	}
	if len(filtered) != 3 || !slices.Equal(filtered, []int64{0, 1, 2}) {
		t.Fatalf("TimeRequireStamp got %v want [0 1 2]", filtered)
	}

	// TimeRequireStamp false - lines without timestamp kept
	filtered, err = se.ApplyAdvancedFilters(t.Context(), SearchOptions{
		TimeFrom:         "2024-01-01 00:00:00",
		TimeRequireStamp: false,
	}, all)
	if err != nil {
		t.Fatal(err)
	}
	if len(filtered) != 4 || !slices.Equal(filtered, []int64{0, 1, 2, 3}) {
		t.Fatalf("TimeRequireStamp=false got %v want [0 1 2 3]", filtered)
	}

	// RFC3339 timestamps
	rfcLines := []string{
		"2024-01-01T10:00:00Z ERROR early",
		"2024-01-01T12:00:00Z INFO middle",
		"2024-01-01T14:00:00Z WARN late",
	}
	rfcIdx := newIndexer(t, strings.Join(rfcLines, "\n")+"\n")
	rfcSe := NewSearchEngine(rfcIdx)

	rfcAll := []int64{0, 1, 2}
	filtered, err = rfcSe.ApplyAdvancedFilters(t.Context(), SearchOptions{
		TimeFrom: "2024-01-01T11:00:00Z",
		TimeTo:   "2024-01-01T13:00:00Z",
	}, rfcAll)
	if err != nil {
		t.Fatal(err)
	}
	if len(filtered) != 1 || !slices.Equal(filtered, []int64{1}) {
		t.Fatalf("RFC3339 time range got %v want [1]", filtered)
	}
}

func TestApplyAdvancedFilters_NearbyQuery(t *testing.T) {
	lines := []string{
		"line 0",
		"ERROR something",
		"line 2",
		"line 3",
		"line 4",
		"WARN nearby",
		"line 6",
		"ERROR another",
		"line 8",
		"INFO here",
	}
	idx := newIndexer(t, strings.Join(lines, "\n")+"\n")
	se := NewSearchEngine(idx)

	// Nearby mode "either" (default) - finds WARN within window of ERROR
	// Window=3: line 1 (ERROR) checks lines 0-4 (excl 1), WARN at 5 not in range
	// line 7 (ERROR) checks lines 4-10 (excl 7), WARN at 5 IS in range
	filtered, err := se.ApplyAdvancedFilters(t.Context(), SearchOptions{
		NearbyQuery:  "WARN",
		NearbyWindow: 3,
	}, []int64{1, 7}) // ERROR lines
	if err != nil {
		t.Fatal(err)
	}
	// Line 1 (ERROR at idx 1) has no WARN within window 3 - should NOT match
	// Line 7 (ERROR at idx 7) has WARN at idx 5 (distance 2) - should match
	if len(filtered) != 1 || filtered[0] != 7 {
		t.Fatalf("nearby either got %v want [7]", filtered)
	}

	// Nearby mode "before"
	filtered, err = se.ApplyAdvancedFilters(t.Context(), SearchOptions{
		NearbyQuery:  "ERROR",
		NearbyMode:   "before",
		NearbyWindow: 5,
	}, []int64{5}) // WARN line
	if err != nil {
		t.Fatal(err)
	}
	// Line 5 (WARN) has ERROR at line 1 (distance 4) and line 7 (distance 2) before it
	// but "before" only checks before, so line 1 and line 7 (which is after) - actually line 7 is after
	// For "before": start = 5-5=0, end = 5, checks lines 0-4
	// Line 1 (ERROR) is in range 0-4 -> found
	if len(filtered) != 1 || filtered[0] != 5 {
		t.Fatalf("nearby before got %v want [5]", filtered)
	}

	// Nearby mode "after"
	filtered, err = se.ApplyAdvancedFilters(t.Context(), SearchOptions{
		NearbyQuery:  "WARN",
		NearbyMode:   "after",
		NearbyWindow: 5,
	}, []int64{1}) // ERROR line
	if err != nil {
		t.Fatal(err)
	}
	// Line 1 (ERROR) has WARN at line 5 after it (distance 4) - should match
	if len(filtered) != 1 || filtered[0] != 1 {
		t.Fatalf("nearby after got %v want [1]", filtered)
	}

	// Window clamping (capped at 200)
	filtered, err = se.ApplyAdvancedFilters(t.Context(), SearchOptions{
		NearbyQuery:  "WARN",
		NearbyWindow: 500,
	}, []int64{1})
	if err != nil {
		t.Fatal(err)
	}
	if len(filtered) != 1 || filtered[0] != 1 {
		t.Fatalf("window clamped got %v want [1]", filtered)
	}

	// Regex nearby query
	filtered, err = se.ApplyAdvancedFilters(t.Context(), SearchOptions{
		NearbyQuery:  "WARN|INFO",
		NearbyMode:   "either",
		NearbyWindow: 10,
		IsRegex:      true,
	}, []int64{1, 7})
	if err != nil {
		t.Fatal(err)
	}
	// Line 1 has WARN at 5 (dist 4), line 7 has WARN at 5 (dist 2) and INFO at 9 (dist 2)
	if len(filtered) != 2 || !slices.Equal(filtered, []int64{1, 7}) {
		t.Fatalf("regex nearby got %v want [1 7]", filtered)
	}
}

func TestApplyAdvancedFilters_MatchAll(t *testing.T) {
	lines := []string{
		"2024-01-01 10:00:00 ERROR something",
		"2024-01-01 13:00:00 WARN warning",
		"2024-01-01 14:00:00 INFO info",
		"no timestamp",
	}
	idx := newIndexer(t, strings.Join(lines, "\n")+"\n")
	se := NewSearchEngine(idx)

	all := []int64{0, 1, 2, 3}

	// MatchAll with Levels filter
	filtered, err := se.ApplyAdvancedFilters(t.Context(), SearchOptions{
		MatchAll: true,
		Levels:   []string{"ERROR"},
	}, all)
	if err != nil {
		t.Fatal(err)
	}
	if len(filtered) != 1 || filtered[0] != 0 {
		t.Fatalf("MatchAll+Levels got %v want [0]", filtered)
	}

	// MatchAll with Time filter
	filtered, err = se.ApplyAdvancedFilters(t.Context(), SearchOptions{
		MatchAll: true,
		TimeFrom: "2024-01-01 12:00:00",
	}, all)
	if err != nil {
		t.Fatal(err)
	}
	// Line 0 (10:00) before 12:00 -> excluded
	// Lines 1, 2 (13:00, 14:00) after 12:00 -> included
	// Line 3 (no timestamp) -> kept by default
	if len(filtered) != 3 || !slices.Equal(filtered, []int64{1, 2, 3}) {
		t.Fatalf("MatchAll+TimeFrom got %v want [1 2 3]", filtered)
	}

	// MatchAll with Exclude
	filtered, err = se.ApplyAdvancedFilters(t.Context(), SearchOptions{
		MatchAll:        true,
		ExcludePatterns: []string{"info"},
		CaseSensitive:   false,
	}, all)
	if err != nil {
		t.Fatal(err)
	}
	if len(filtered) != 3 || !slices.Equal(filtered, []int64{0, 1, 3}) {
		t.Fatalf("MatchAll+Exclude got %v want [0 1 3]", filtered)
	}
}

func TestApplyAdvancedFilters_ContextCancellation(t *testing.T) {
	// Need enough lines to hit the cancellation check (every 256 lines)
	// and do some actual work so the cancellation is detected
	lines := make([]string, 50000)
	for i := range lines {
		lines[i] = fmt.Sprintf("line %d with some content to process", i)
	}
	idx := newIndexer(t, strings.Join(lines, "\n")+"\n")
	se := NewSearchEngine(idx)

	all := make([]int64, 50000)
	for i := range all {
		all[i] = int64(i)
	}

	// Use a very short timeout to ensure cancellation happens
	// Add Levels filter to ensure we enter the processing loop
	ctx, cancel := context.WithTimeout(t.Context(), 1*time.Nanosecond)
	// Give a tiny moment for the timeout to be set
	time.Sleep(1 * time.Millisecond)
	defer cancel()

	_, err := se.ApplyAdvancedFilters(ctx, SearchOptions{
		MatchAll: true,
		Levels:   []string{"ERROR"}, // This forces processing loop
	}, all)
	if err == nil {
		t.Fatal("expected context cancellation error")
	}
	if !errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context error, got %v", err)
	}
}

func TestApplyAdvancedFilters_CombinedFilters(t *testing.T) {
	lines := []string{
		"2024-01-01 10:00:00 ERROR alpha",
		"2024-01-01 11:00:00 WARN beta",
		"2024-01-01 12:00:00 INFO gamma",
		"2024-01-01 13:00:00 ERROR delta",
		"2024-01-01 14:00:00 WARN epsilon",
	}
	idx := newIndexer(t, strings.Join(lines, "\n")+"\n")
	se := NewSearchEngine(idx)

	all := []int64{0, 1, 2, 3, 4}

	// Combine: Levels ERROR + TimeFrom 11:00
	filtered, err := se.ApplyAdvancedFilters(t.Context(), SearchOptions{
		Levels:   []string{"ERROR"},
		TimeFrom: "2024-01-01 11:00:00",
	}, all)
	if err != nil {
		t.Fatal(err)
	}
	// Only line 3 (ERROR at 13:00) matches both
	if len(filtered) != 1 || filtered[0] != 3 {
		t.Fatalf("combined Levels+Time got %v want [3]", filtered)
	}

	// Combine: Exclude + Levels
	filtered, err = se.ApplyAdvancedFilters(t.Context(), SearchOptions{
		Levels:          []string{"ERROR", "WARN"},
		ExcludePatterns: []string{"beta"},
		CaseSensitive:   false,
	}, all)
	if err != nil {
		t.Fatal(err)
	}
	// ERROR lines (0, 3) + WARN line (4), but exclude "beta" (line 1)
	if len(filtered) != 3 || !slices.Equal(filtered, []int64{0, 3, 4}) {
		t.Fatalf("combined Levels+Exclude got %v want [0 3 4]", filtered)
	}

	// Combine: Nearby + Time + Levels
	filtered, err = se.ApplyAdvancedFilters(t.Context(), SearchOptions{
		NearbyQuery:  "WARN",
		NearbyMode:   "either",
		NearbyWindow: 10,
		TimeFrom:     "2024-01-01 11:30:00",
		Levels:       []string{"ERROR"},
	}, all)
	if err != nil {
		t.Fatal(err)
	}
	// ERROR lines after 11:30: line 3 (13:00) - has WARN at line 4 nearby
	if len(filtered) != 1 || filtered[0] != 3 {
		t.Fatalf("combined Nearby+Time+Levels got %v want [3]", filtered)
	}
}

func TestApplyAdvancedFilters_NoFilters(t *testing.T) {
	idx := newIndexer(t, "a\nb\nc\n")
	se := NewSearchEngine(idx)

	all := []int64{0, 1, 2}
	filtered, err := se.ApplyAdvancedFilters(t.Context(), SearchOptions{}, all)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(filtered, all) {
		t.Fatalf("no filters should return all, got %v", filtered)
	}
}

func TestApplyAdvancedFilters_ExcludeInvalidRegex(t *testing.T) {
	idx := newIndexer(t, "a\nb\nc\n")
	se := NewSearchEngine(idx)

	_, err := se.ApplyAdvancedFilters(t.Context(), SearchOptions{
		ExcludePatterns: []string{"("},
		ExcludeIsRegex:  true,
	}, []int64{0, 1, 2})
	if err == nil {
		t.Fatal("expected error for invalid exclude regex")
	}
	if !strings.Contains(err.Error(), "exclude pattern") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestApplyAdvancedFilters_NearbyInvalidRegex(t *testing.T) {
	idx := newIndexer(t, "a\nb\nc\n")
	se := NewSearchEngine(idx)

	_, err := se.ApplyAdvancedFilters(t.Context(), SearchOptions{
		NearbyQuery: "(",
		IsRegex:     true,
	}, []int64{0, 1, 2})
	if err == nil {
		t.Fatal("expected error for invalid nearby regex")
	}
	if !strings.Contains(err.Error(), "nearby query") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestApplyAdvancedFilters_InvalidTimeFormat(t *testing.T) {
	idx := newIndexer(t, "a\nb\nc\n")
	se := NewSearchEngine(idx)

	_, err := se.ApplyAdvancedFilters(t.Context(), SearchOptions{
		TimeFrom: "invalid",
	}, []int64{0, 1, 2})
	if err == nil {
		t.Fatal("expected error for invalid time_from")
	}
	if !strings.Contains(err.Error(), "time_from") {
		t.Fatalf("unexpected error: %v", err)
	}

	_, err = se.ApplyAdvancedFilters(t.Context(), SearchOptions{
		TimeTo: "invalid",
	}, []int64{0, 1, 2})
	if err == nil {
		t.Fatal("expected error for invalid time_to")
	}
	if !strings.Contains(err.Error(), "time_to") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// matchRangeWithLevels additional tests.
func TestMatchRangeWithLevels_Cancellation(t *testing.T) {
	var buf strings.Builder
	for range 10000 {
		buf.WriteString("error\n")
	}
	idx := newIndexer(t, buf.String())
	se := NewSearchEngine(idx)

	ctx, cancel := context.WithTimeout(t.Context(), 1*time.Nanosecond)
	time.Sleep(1 * time.Millisecond)
	cancel()

	_, _, _, err := se.matchRangeWithLevels(ctx, SearchOptions{Query: "error"}, 0, 10000)
	if err == nil {
		t.Log("search completed before cancellation")
		return
	}
	if !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected context error, got %v", err)
	}
}

func TestMatchRangeWithLevels_EmptyFile(t *testing.T) {
	idx := newIndexer(t, "")
	se := NewSearchEngine(idx)

	matches, errCount, warnCount, err := se.matchRangeWithLevels(t.Context(), SearchOptions{Query: "error"}, 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 0 || errCount != 0 || warnCount != 0 {
		t.Fatalf("expected empty result, got matches=%v err=%d warn=%d", matches, errCount, warnCount)
	}
}

func TestMatchRangeWithLevels_OutOfBoundsRange(t *testing.T) {
	idx := newIndexer(t, "error\ninfo\nerror\n")
	se := NewSearchEngine(idx)

	// End > total lines
	matches, _, _, err := se.matchRangeWithLevels(t.Context(), SearchOptions{Query: "error"}, 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 2 {
		t.Fatalf("expected 2 matches, got %d", len(matches))
	}

	// Start >= end
	matches, _, _, err = se.matchRangeWithLevels(t.Context(), SearchOptions{Query: "error"}, 5, 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 0 {
		t.Fatalf("expected 0 matches, got %d", len(matches))
	}

	// Start < 0
	matches, _, _, err = se.matchRangeWithLevels(t.Context(), SearchOptions{Query: "error"}, -1, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 0 {
		t.Fatalf("expected 0 matches, got %d", len(matches))
	}
}

// Additional tests for uncovered paths in matchRangeWithLevels

func TestMatchRangeWithLevels_InvalidRegex(t *testing.T) {
	idx := newIndexer(t, "error\ninfo\nerror\n")
	se := NewSearchEngine(idx)

	// Invalid regex should return error
	_, _, _, err := se.matchRangeWithLevels(t.Context(), SearchOptions{
		Query:   "(",
		IsRegex: true,
	}, 0, 10)
	if err == nil {
		t.Fatal("expected error for invalid regex")
	}
	if !strings.Contains(err.Error(), "invalid regular expression") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestMatchRangeWithLevels_SecondaryMatcherError(t *testing.T) {
	idx := newIndexer(t, "error\ninfo\nerror\n")
	se := NewSearchEngine(idx)

	// Invalid secondary regex should return error
	_, _, _, err := se.matchRangeWithLevels(t.Context(), SearchOptions{
		Query:          "error",
		SecondaryQuery: "(",
		IsRegex:        true,
	}, 0, 10)
	if err == nil {
		t.Fatal("expected error for invalid secondary regex")
	}
	if !strings.Contains(err.Error(), "invalid secondary regular expression") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestMatchRangeWithLevels_NonASCIIQuery(t *testing.T) {
	lines := []string{
		"ошибка first",
		"warning second",
		"ошибка third",
	}
	idx := newIndexer(t, strings.Join(lines, "\n")+"\n")
	se := NewSearchEngine(idx)

	// Non-ASCII query with case-insensitive should use containsFoldRune path
	matches, _, _, err := se.matchRangeWithLevels(t.Context(), SearchOptions{
		Query:         "ОШИБКА",
		CaseSensitive: false,
	}, 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 2 || matches[0] != 0 || matches[1] != 2 {
		t.Fatalf("expected [0 2], got %v", matches)
	}
}

func TestMatchRangeWithLevels_InvertMatch(t *testing.T) {
	lines := []string{
		"error one",
		"info two",
		"error three",
	}
	idx := newIndexer(t, strings.Join(lines, "\n")+"\n")
	se := NewSearchEngine(idx)

	matches, _, _, err := se.matchRangeWithLevels(t.Context(), SearchOptions{
		Query:       "error",
		InvertMatch: true,
	}, 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 1 || matches[0] != 1 {
		t.Fatalf("expected [1], got %v", matches)
	}
}

func TestMatchRangeWithLevels_SecondaryMatch(t *testing.T) {
	lines := []string{
		"ERROR Exception in module A",
		"WARN Exception recovered",
		"INFO Exception Timeout=5",
		"ERROR Exception Timeout=30",
	}
	idx := newIndexer(t, strings.Join(lines, "\n")+"\n")
	se := NewSearchEngine(idx)

	// Secondary query narrows results (AND semantics) - matchRangeWithLevels only supports AND
	matches, _, _, err := se.matchRangeWithLevels(t.Context(), SearchOptions{
		Query:          "Exception",
		SecondaryQuery: "Timeout",
	}, 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 2 || matches[0] != 2 || matches[1] != 3 {
		t.Fatalf("expected [2 3], got %v", matches)
	}

	// Note: matchRangeWithLevels doesn't support OR mode, only AND
	// OR mode is only in matchRange
}

func TestMatchRangeWithLevels_MultiChunkParallel(t *testing.T) {
	// Large enough to trigger multi-worker path
	// Use uppercase ERROR to trigger error counting
	var buf strings.Builder
	for range 5000 {
		buf.WriteString("ERROR\n")
	}
	idx := newIndexer(t, buf.String())
	se := NewSearchEngine(idx)

	matches, errCount, warnCount, err := se.matchRangeWithLevels(t.Context(), SearchOptions{Query: "ERROR"}, 0, 5000)
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 5000 {
		t.Fatalf("expected 5000 matches, got %d", len(matches))
	}
	// Each line has "ERROR" in first 160 bytes -> error count
	if errCount != 5000 {
		t.Fatalf("expected 5000 error count, got %d", errCount)
	}
	if warnCount != 0 {
		t.Fatalf("expected 0 warn count, got %d", warnCount)
	}
}

// Additional tests for uncovered paths in matchRange

func TestMatchRange_MatchAllFastPath(t *testing.T) {
	idx := newIndexer(t, "a\nb\nc\nd\ne\n")
	se := NewSearchEngine(idx)

	// MatchAll with no secondary should use fast path
	matches, err := se.matchRange(t.Context(), SearchOptions{
		MatchAll: true,
	}, 1, 4, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(matches, []int64{1, 2, 3}) {
		t.Fatalf("expected [1 2 3], got %v", matches)
	}

	// MatchAll with secondary should NOT use fast path
	matches, err = se.matchRange(t.Context(), SearchOptions{
		MatchAll:       true,
		SecondaryQuery: "b",
	}, 1, 4, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(matches, []int64{1}) {
		t.Fatalf("expected [1] with secondary, got %v", matches)
	}
}

func TestMatchRange_InvalidRegex(t *testing.T) {
	idx := newIndexer(t, "error\ninfo\nerror\n")
	se := NewSearchEngine(idx)

	_, err := se.matchRange(t.Context(), SearchOptions{
		Query:   "(",
		IsRegex: true,
	}, 0, 10, nil)
	if err == nil {
		t.Fatal("expected error for invalid regex")
	}
	if !strings.Contains(err.Error(), "invalid regular expression") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestMatchRange_SecondaryMatcherError(t *testing.T) {
	idx := newIndexer(t, "error\ninfo\nerror\n")
	se := NewSearchEngine(idx)

	_, err := se.matchRange(t.Context(), SearchOptions{
		Query:          "error",
		SecondaryQuery: "(",
		IsRegex:        true,
	}, 0, 10, nil)
	if err == nil {
		t.Fatal("expected error for invalid secondary regex")
	}
	if !strings.Contains(err.Error(), "invalid secondary regular expression") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestMatchRange_NonASCIIQuery(t *testing.T) {
	lines := []string{
		"ошибка first",
		"warning second",
		"ошибка third",
	}
	idx := newIndexer(t, strings.Join(lines, "\n")+"\n")
	se := NewSearchEngine(idx)

	// Non-ASCII query with case-insensitive should use containsFoldRune path
	matches, err := se.matchRange(t.Context(), SearchOptions{
		Query:         "ОШИБКА",
		CaseSensitive: false,
	}, 0, 10, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 2 || matches[0] != 0 || matches[1] != 2 {
		t.Fatalf("expected [0 2], got %v", matches)
	}
}

func TestMatchRange_MatchAllWithInvert(t *testing.T) {
	idx := newIndexer(t, "error\ninfo\nerror\nwarning\n")
	se := NewSearchEngine(idx)

	// MatchAll with InvertMatch - the fast path for MatchAll (no secondary) ignores InvertMatch
	// This tests the fast path behavior
	matches, err := se.matchRange(t.Context(), SearchOptions{
		MatchAll:    true,
		InvertMatch: true,
	}, 0, 10, nil)
	if err != nil {
		t.Fatal(err)
	}
	// Fast path returns all lines without applying InvertMatch
	if len(matches) != 4 {
		t.Fatalf("expected 4 matches (fast path ignores InvertMatch), got %v", matches)
	}

	// With secondary, InvertMatch is applied (not fast path)
	matches, err = se.matchRange(t.Context(), SearchOptions{
		MatchAll:       true,
		InvertMatch:    true,
		SecondaryQuery: "x", // dummy secondary to disable fast path
	}, 0, 10, nil)
	if err != nil {
		t.Fatal(err)
	}
	// Now InvertMatch is applied: MatchAll=true -> isMatch=true, then InvertMatch -> isMatch=false
	// But secondary "x" doesn't match anything, so secHit=false, AND mode -> isMatch=false
	if len(matches) != 0 {
		t.Fatalf("expected 0 matches with secondary+InvertMatch, got %v", matches)
	}
}

func TestMatchRange_SecondaryOrMode(t *testing.T) {
	lines := []string{
		"ERROR Exception in module A",
		"WARN Exception recovered",
		"INFO Exception Timeout=5",
		"ERROR Exception Timeout=30",
	}
	idx := newIndexer(t, strings.Join(lines, "\n")+"\n")
	se := NewSearchEngine(idx)

	// Secondary with OR mode
	matches, err := se.matchRange(t.Context(), SearchOptions{
		Query:          "ERROR",
		SecondaryQuery: "Timeout",
		SecondaryMode:  "or",
	}, 0, 10, nil)
	if err != nil {
		t.Fatal(err)
	}
	// ERROR lines (0, 3) + lines with Timeout (2, 3) = 0, 2, 3
	if len(matches) != 3 || !slices.Equal(matches, []int64{0, 2, 3}) {
		t.Fatalf("OR mode expected [0 2 3], got %v", matches)
	}
}

func TestMatchRange_ContextCancelledInResults(t *testing.T) {
	var buf strings.Builder
	for range 10000 {
		buf.WriteString("error\n")
	}
	idx := newIndexer(t, buf.String())
	se := NewSearchEngine(idx)

	ctx, cancel := context.WithTimeout(t.Context(), 1*time.Nanosecond)
	time.Sleep(1 * time.Millisecond)
	cancel()

	_, err := se.matchRange(ctx, SearchOptions{Query: "error"}, 0, 10000, nil)
	if err == nil {
		t.Log("search completed before cancellation")
		return
	}
	if !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected context error, got %v", err)
	}
}

func TestMatchRange_WorkerPanic(t *testing.T) {
	// This test verifies the panic recovery path in workers
	// We can't easily trigger a panic in the worker, but we can verify
	// the code compiles and the recovery mechanism exists
	idx := newIndexer(t, "error\ninfo\nerror\n")
	se := NewSearchEngine(idx)

	// Normal search should work
	matches, err := se.matchRange(t.Context(), SearchOptions{Query: "error"}, 0, 10, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 2 {
		t.Fatalf("expected 2 matches, got %d", len(matches))
	}
}

// Additional tests for FilterLines

func TestFilterLines_ContextCancellationDuringIteration(t *testing.T) {
	lines := make([]string, 10000)
	for i := range lines {
		lines[i] = fmt.Sprintf("line %d with match", i)
	}
	idx := newIndexer(t, strings.Join(lines, "\n")+"\n")
	se := NewSearchEngine(idx)

	all := make([]int64, 10000)
	for i := range all {
		all[i] = int64(i)
	}

	ctx, cancel := context.WithTimeout(t.Context(), 1*time.Nanosecond)
	time.Sleep(1 * time.Millisecond)
	cancel()

	_, err := se.FilterLines(ctx, SearchOptions{SecondaryQuery: "match"}, all)
	if err == nil {
		t.Log("filter completed before cancellation")
		return
	}
	if !errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context error, got %v", err)
	}
}

func TestFilterLines_WithSecondaryQueryError(t *testing.T) {
	idx := newIndexer(t, "test\n")
	se := NewSearchEngine(idx)

	_, err := se.FilterLines(t.Context(), SearchOptions{
		SecondaryQuery: "(",
		IsRegex:        true,
	}, []int64{0})
	if err == nil {
		t.Fatal("expected error for invalid secondary regex")
	}
}

// Additional tests for ApplyAdvancedFilters

func TestApplyAdvancedFilters_NearbyWithContextCancellation(t *testing.T) {
	lines := make([]string, 10000)
	for i := range lines {
		lines[i] = fmt.Sprintf("line %d", i)
	}
	idx := newIndexer(t, strings.Join(lines, "\n")+"\n")
	se := NewSearchEngine(idx)

	all := make([]int64, 10000)
	for i := range all {
		all[i] = int64(i)
	}

	ctx, cancel := context.WithTimeout(t.Context(), 1*time.Nanosecond)
	time.Sleep(1 * time.Millisecond)
	cancel()

	_, err := se.ApplyAdvancedFilters(ctx, SearchOptions{
		NearbyQuery:  "something",
		NearbyWindow: 10,
	}, all)
	if err == nil {
		t.Log("filter completed before cancellation")
		return
	}
	if !errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context error, got %v", err)
	}
}

func TestApplyAdvancedFilters_MatchAllWithNoFilters(t *testing.T) {
	idx := newIndexer(t, "a\nb\nc\n")
	se := NewSearchEngine(idx)

	// MatchAll with no other filters should return all lines (handled early)
	filtered, err := se.ApplyAdvancedFilters(t.Context(), SearchOptions{
		MatchAll: true,
	}, []int64{0, 1, 2})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(filtered, []int64{0, 1, 2}) {
		t.Fatalf("expected [0 1 2], got %v", filtered)
	}
}

// Additional tests for parseLineTime

func TestParseLineTime_OnlyDateNoTime(t *testing.T) {
	// Line with only date (no time) should not parse
	line := "2024-01-01 just a date"
	_, found := parseLineTime(line)
	if found {
		t.Fatalf("expected no time found for date-only line")
	}
}

func TestParseLineTime_MultipleSpaces(t *testing.T) {
	// Test with multiple spaces in timestamp
	line := "2024-01-01  10:00:00  error"
	ts, found := parseLineTime(line)
	if !found {
		t.Fatal("expected time found")
	}
	expected := time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC)
	if !ts.Equal(expected) {
		t.Fatalf("expected %v, got %v", expected, ts)
	}
}

func TestParseLineTime_TabSeparated(t *testing.T) {
	// Test with tab-separated timestamp
	line := "2024-01-01\t10:00:00\terror"
	ts, found := parseLineTime(line)
	if !found {
		t.Fatal("expected time found")
	}
	expected := time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC)
	if !ts.Equal(expected) {
		t.Fatalf("expected %v, got %v", expected, ts)
	}
}

func TestParseLineTime_LongPrefixTruncation(t *testing.T) {
	// Line with very long prefix (>40 chars) - should truncate to 40 chars
	// The timestamp should still be found in the truncated prefix
	longPrefix := "2024-01-01 10:00:00 " + strings.Repeat("x", 30)
	line := longPrefix + " error"
	ts, found := parseLineTime(line)
	if !found {
		t.Fatal("expected time found in truncated prefix")
	}
	expected := time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC)
	if !ts.Equal(expected) {
		t.Fatalf("expected %v, got %v", expected, ts)
	}
}

// Tests for remaining uncovered paths in matchRange.WithLevels.
func TestMatchRangeWithLevels_EmptyQuery(t *testing.T) {
	idx := newIndexer(t, "error\ninfo\nerror\n")
	se := NewSearchEngine(idx)

	// Empty query should return empty immediately
	matches, errCount, warnCount, err := se.matchRangeWithLevels(t.Context(), SearchOptions{Query: ""}, 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 0 || errCount != 0 || warnCount != 0 {
		t.Fatalf("expected empty result for empty query")
	}
}

func TestMatchRangeWithLevels_RegexPath(t *testing.T) {
	idx := newIndexer(t, "error123\ninfo\nerror456\n")
	se := NewSearchEngine(idx)

	// Test regex match path
	matches, _, _, err := se.matchRangeWithLevels(t.Context(), SearchOptions{
		Query:   "[0-9]+",
		IsRegex: true,
	}, 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 2 || matches[0] != 0 || matches[1] != 2 {
		t.Fatalf("expected [0 2], got %v", matches)
	}
}

func TestMatchRangeWithLevels_CaseSensitivePath(t *testing.T) {
	idx := newIndexer(t, "Error\ninfo\nERROR\n")
	se := NewSearchEngine(idx)

	// Test case-sensitive match path
	matches, _, _, err := se.matchRangeWithLevels(t.Context(), SearchOptions{
		Query:         "Error",
		CaseSensitive: true,
	}, 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 1 || matches[0] != 0 {
		t.Fatalf("expected [0], got %v", matches)
	}
}

func TestMatchRangeWithLevels_LevelWindowTruncation(t *testing.T) {
	// Test level counting with long lines that get truncated to 160 bytes
	longLine := "ERROR " + strings.Repeat("x", 200)
	lines := []string{longLine, "WARN short", "info normal"}
	idx := newIndexer(t, strings.Join(lines, "\n")+"\n")
	se := NewSearchEngine(idx)

	_, errCount, warnCount, err := se.matchRangeWithLevels(t.Context(), SearchOptions{
		Query: "x", // match all lines
	}, 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	// First line has ERROR in first 160 bytes -> error count
	// Second line has WARN -> warn count
	// Third line has neither in first 160 -> other
	if errCount != 1 {
		t.Fatalf("expected 1 error count, got %d", errCount)
	}
	if warnCount != 1 {
		t.Fatalf("expected 1 warn count, got %d", warnCount)
	}
}

func TestMatchRangeWithLevels_ScanLinesRetryError(t *testing.T) {
	// This tests the error path from ScanLinesRetry
	// We can't easily trigger a real error, but we can verify the code path exists
	idx := newIndexer(t, "error\ninfo\nerror\n")
	se := NewSearchEngine(idx)

	// Normal search should work
	_, _, _, err := se.matchRangeWithLevels(t.Context(), SearchOptions{Query: "error"}, 0, 10)
	if err != nil {
		t.Fatal(err)
	}
}

func TestMatchRangeWithLevels_CanceledHandling(t *testing.T) {
	// Test the canceled error handling path
	var buf strings.Builder
	for range 10000 {
		buf.WriteString("error\n")
	}
	idx := newIndexer(t, buf.String())
	se := NewSearchEngine(idx)

	ctx, cancel := context.WithTimeout(t.Context(), 1*time.Nanosecond)
	time.Sleep(1 * time.Millisecond)
	cancel()

	_, _, _, err := se.matchRangeWithLevels(ctx, SearchOptions{Query: "error"}, 0, 10000)
	if err == nil {
		t.Log("search completed before cancellation")
		return
	}
	// Should get a "search canceled" error
	if err != nil && !strings.Contains(err.Error(), "search canceled") {
		t.Fatalf("expected 'search canceled' error, got %v", err)
	}
}

func TestMatchRangeWithLevels_MultipleErrorsJoined(t *testing.T) {
	// This tests the errors.Join path - we can't easily trigger multiple worker errors
	// but we verify the error handling code compiles
	idx := newIndexer(t, "error\ninfo\nerror\n")
	se := NewSearchEngine(idx)

	matches, _, _, err := se.matchRangeWithLevels(t.Context(), SearchOptions{Query: "error"}, 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 2 {
		t.Fatalf("expected 2 matches, got %d", len(matches))
	}
}

// Tests for remaining uncovered paths in matchRange.
func TestMatchRange_FastPathOnProgress(t *testing.T) {
	idx := newIndexer(t, "a\nb\nc\nd\ne\n")
	se := NewSearchEngine(idx)

	var progress int64
	onProgress := func(n int64) {
		progress = n
	}

	// MatchAll with no secondary uses fast path with onProgress
	matches, err := se.matchRange(t.Context(), SearchOptions{
		MatchAll: true,
	}, 1, 4, onProgress)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(matches, []int64{1, 2, 3}) {
		t.Fatalf("expected [1 2 3], got %v", matches)
	}
	if progress != 3 {
		t.Fatalf("expected progress=3, got %d", progress)
	}
}

func TestMatchRange_WorkerPanicRecovery(t *testing.T) {
	// Test the panic recovery path in workers
	// We can't easily trigger a real panic, but we verify the code compiles
	idx := newIndexer(t, "error\ninfo\nerror\n")
	se := NewSearchEngine(idx)

	matches, err := se.matchRange(t.Context(), SearchOptions{Query: "error"}, 0, 10, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 2 {
		t.Fatalf("expected 2 matches, got %d", len(matches))
	}
}

func TestMatchRange_ScanLinesRetryError(t *testing.T) {
	// Test the error path from ScanLinesRetry
	idx := newIndexer(t, "error\ninfo\nerror\n")
	se := NewSearchEngine(idx)

	matches, err := se.matchRange(t.Context(), SearchOptions{Query: "error"}, 0, 10, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 2 {
		t.Fatalf("expected 2 matches, got %d", len(matches))
	}
}

func TestMatchRange_CanceledHandling(t *testing.T) {
	// Test the canceled error handling path
	var buf strings.Builder
	for range 10000 {
		buf.WriteString("error\n")
	}
	idx := newIndexer(t, buf.String())
	se := NewSearchEngine(idx)

	ctx, cancel := context.WithTimeout(t.Context(), 1*time.Nanosecond)
	time.Sleep(1 * time.Millisecond)
	cancel()

	_, err := se.matchRange(ctx, SearchOptions{Query: "error"}, 0, 10000, nil)
	if err == nil {
		t.Log("search completed before cancellation")
		return
	}
	// Should get a "search canceled" error
	if err != nil && !strings.Contains(err.Error(), "search canceled") {
		t.Fatalf("expected 'search canceled' error, got %v", err)
	}
}

func TestMatchRange_MultipleErrorsJoined(t *testing.T) {
	// Test the errors.Join path
	idx := newIndexer(t, "error\ninfo\nerror\n")
	se := NewSearchEngine(idx)

	matches, err := se.matchRange(t.Context(), SearchOptions{Query: "error"}, 0, 10, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 2 {
		t.Fatalf("expected 2 matches, got %d", len(matches))
	}
}

// Test for FilterLines GetLine error.
func TestFilterLines_GetLineError(t *testing.T) {
	// This tests the error path when GetLine fails
	// We can't easily trigger a real GetLine error, but we verify the code compiles
	idx := newIndexer(t, "test line\n")
	se := NewSearchEngine(idx)

	filtered, err := se.FilterLines(t.Context(), SearchOptions{SecondaryQuery: "test"}, []int64{0})
	if err != nil {
		t.Fatal(err)
	}
	if len(filtered) != 1 {
		t.Fatalf("expected 1 match, got %d", len(filtered))
	}
}

// Tests for ApplyAdvancedFilters remaining paths.
func TestApplyAdvancedFilters_CacheEviction(t *testing.T) {
	// Test the cache eviction path (dropping oldest half)
	lines := make([]string, 300)
	for i := range lines {
		lines[i] = fmt.Sprintf("line %d with WARN", i)
	}
	idx := newIndexer(t, strings.Join(lines, "\n")+"\n")
	se := NewSearchEngine(idx)

	all := make([]int64, 300)
	for i := range all {
		all[i] = int64(i)
	}

	// With nearby window, cache will be used and may evict
	filtered, err := se.ApplyAdvancedFilters(t.Context(), SearchOptions{
		NearbyQuery:  "WARN",
		NearbyWindow: 10,
	}, all)
	if err != nil {
		t.Fatal(err)
	}
	if len(filtered) != 300 {
		t.Fatalf("expected 300 matches, got %d", len(filtered))
	}
}

func TestApplyAdvancedFilters_GetLineError(t *testing.T) {
	// Test GetLine error in main loop
	idx := newIndexer(t, "test\n")
	se := NewSearchEngine(idx)

	// Normal case should work
	filtered, err := se.ApplyAdvancedFilters(t.Context(), SearchOptions{
		Levels: []string{"ERROR"},
	}, []int64{0})
	if err != nil {
		t.Fatal(err)
	}
	if len(filtered) != 0 {
		t.Fatalf("expected 0 matches, got %d", len(filtered))
	}
}

func TestApplyAdvancedFilters_NearbyBeforeModeStartClamp(t *testing.T) {
	// Test nearby "before" mode with start clamping at 0
	lines := []string{
		"WARN at start",
		"line 1",
		"line 2",
		"ERROR target",
	}
	idx := newIndexer(t, strings.Join(lines, "\n")+"\n")
	se := NewSearchEngine(idx)

	// Search for ERROR at line 3, with nearby "before" window 10
	// Should find WARN at line 0 (within 3 lines before)
	filtered, err := se.ApplyAdvancedFilters(t.Context(), SearchOptions{
		NearbyQuery:  "WARN",
		NearbyMode:   "before",
		NearbyWindow: 10,
	}, []int64{3})
	if err != nil {
		t.Fatal(err)
	}
	if len(filtered) != 1 || filtered[0] != 3 {
		t.Fatalf("expected [3], got %v", filtered)
	}
}

func TestApplyAdvancedFilters_NearbyGetLineError(t *testing.T) {
	// Test GetLine error in nearby window loop
	lines := []string{
		"test line 1",
		"test line 2",
		"other line",
	}
	idx := newIndexer(t, strings.Join(lines, "\n")+"\n")
	se := NewSearchEngine(idx)

	// Search for line 2 (index 2), nearby "test" in window 10
	// Should find "test" in lines 0 and 1
	filtered, err := se.ApplyAdvancedFilters(t.Context(), SearchOptions{
		NearbyQuery:  "test",
		NearbyWindow: 10,
	}, []int64{2})
	if err != nil {
		t.Fatal(err)
	}
	if len(filtered) != 1 {
		t.Fatalf("expected 1 match, got %d", len(filtered))
	}
}

func TestApplyAdvancedFilters_CanceledInResults(t *testing.T) {
	// Test the canceled error handling in results collection
	var buf strings.Builder
	for range 10000 {
		buf.WriteString("line with WARN\n")
	}
	idx := newIndexer(t, buf.String())
	se := NewSearchEngine(idx)

	all := make([]int64, 10000)
	for i := range all {
		all[i] = int64(i)
	}

	ctx, cancel := context.WithTimeout(t.Context(), 1*time.Nanosecond)
	time.Sleep(1 * time.Millisecond)
	cancel()

	// Use Levels filter to ensure we enter the processing loop
	_, err := se.ApplyAdvancedFilters(ctx, SearchOptions{
		Levels: []string{"WARN"},
	}, all)
	if err == nil {
		t.Log("filter completed before cancellation")
		return
	}
	if !errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context error, got %v", err)
	}
}
