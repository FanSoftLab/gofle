package search

import (
	"GoFLE/pkg/indexer"
	"bytes"
	"context"
	"errors"
	"math/rand"
	"os"
	"path/filepath"
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
