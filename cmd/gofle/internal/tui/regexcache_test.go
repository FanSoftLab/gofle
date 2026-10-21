package tui

import (
	"strings"
	"testing"
	"time"
)

// resetRegexCache empties the package-level regex cache so cache-behavior
// tests start from a known state and do not leak entries into other tests.
func resetRegexCache() {
	tuiReCacheMu.Lock()
	defer tuiReCacheMu.Unlock()

	tuiReCache = make(map[string]tuiReCacheEntry)
	tuiReCacheOrder = nil
}

func TestCachedRegexSamePointer(t *testing.T) {
	resetRegexCache()
	defer resetRegexCache()

	re1, err := cachedRegex("cache-hit-\\d+", false)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}

	re2, err := cachedRegex("cache-hit-\\d+", false)
	if err != nil {
		t.Fatalf("second compile: %v", err)
	}

	if re1 != re2 {
		t.Fatal("second call returned a different regex; cache miss")
	}

	if len(tuiReCache) != 1 {
		t.Fatalf("cache size = %d, want 1", len(tuiReCache))
	}
}

func TestCachedRegexCaseFlagDistinct(t *testing.T) {
	resetRegexCache()
	defer resetRegexCache()

	reCS, err := cachedRegex("flag-distinct", true)
	if err != nil {
		t.Fatalf("compile cs: %v", err)
	}

	reCI, err := cachedRegex("flag-distinct", false)
	if err != nil {
		t.Fatalf("compile ci: %v", err)
	}

	if reCS == reCI {
		t.Fatal("case-sensitive and case-insensitive keys collided on one entry")
	}

	if len(tuiReCache) != 2 {
		t.Fatalf("cache size = %d, want 2 (one entry per flag setting)", len(tuiReCache))
	}
}

func TestCachedRegexTTLExpiry(t *testing.T) {
	resetRegexCache()
	defer resetRegexCache()

	key := regexPattern("ttl-pattern", true)

	re1, err := cachedRegex("ttl-pattern", true)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}

	// Rewind cachedAt past the TTL so the entry is expired on next access.
	tuiReCacheMu.Lock()
	entry := tuiReCache[key]
	entry.cachedAt = time.Now().Add(-tuiReCacheTTL - time.Second)
	tuiReCache[key] = entry
	tuiReCacheMu.Unlock()

	re2, err := cachedRegex("ttl-pattern", true)
	if err != nil {
		t.Fatalf("recompile: %v", err)
	}

	if re1 == re2 {
		t.Fatal("expired entry was served instead of recompiled")
	}
}

func TestCachedRegexCapEviction(t *testing.T) {
	resetRegexCache()
	defer resetRegexCache()

	for i := range tuiReCacheMax + 1 {
		if _, err := cachedRegex("cap-"+strings.Repeat("x", i), true); err != nil {
			t.Fatalf("compile %d: %v", i, err)
		}
	}

	if len(tuiReCache) != tuiReCacheMax {
		t.Fatalf("cache size = %d, want %d", len(tuiReCache), tuiReCacheMax)
	}

	// First-inserted eviction: the very first pattern ("cap-", i=0) must be
	// gone, its neighbors and the newest entry still served from cache.
	if re := getCachedRegex("cap-"); re != nil {
		t.Fatal("first-inserted entry was not evicted at capacity")
	}

	if re := getCachedRegex("cap-x"); re == nil {
		t.Fatal("second-inserted entry was evicted prematurely")
	}

	if re := getCachedRegex("cap-" + strings.Repeat("x", tuiReCacheMax)); re == nil {
		t.Fatal("newest entry missing from cache")
	}
}

func TestCachedRegexInvalidNotCached(t *testing.T) {
	resetRegexCache()
	defer resetRegexCache()

	if _, err := cachedRegex("[bad", true); err == nil {
		t.Fatal("expected compile error for invalid pattern")
	}

	if len(tuiReCache) != 0 {
		t.Fatalf("invalid pattern was cached (%d entries)", len(tuiReCache))
	}
}

func TestFindSpansBadRegexFallsBackToLiteral(t *testing.T) {
	spans := findSpans("a [b] c", "[b", true, true, 0, hlPrimaryStyle)
	if len(spans) != 1 || spans[0].start != 2 || spans[0].end != 4 {
		t.Fatalf("case-sensitive fallback spans = %+v", spans)
	}

	spans = findSpans("a [B] c", "[b", true, false, 0, hlPrimaryStyle)
	if len(spans) != 1 || spans[0].start != 2 || spans[0].end != 4 {
		t.Fatalf("case-insensitive fallback spans = %+v", spans)
	}
}

func TestExtractMatchSnippetUsesCache(t *testing.T) {
	resetRegexCache()
	defer resetRegexCache()

	reFind, err := cachedRegex("user_\\d+", true)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}

	got := extractMatchSnippet("request from user_42 done", "user_\\d+", true, true)
	if got != "user_42" {
		t.Fatalf("extractMatchSnippet = %q, want %q", got, "user_42")
	}

	// The snippet path must serve the same compiled regex as the cache,
	// proving it no longer compiles its own copy per call.
	reCached := getCachedRegex("user_\\d+")
	if reCached != reFind {
		t.Fatal("extractMatchSnippet did not reuse the cached regex")
	}
}

func TestExtractMatchSnippetBadRegexFallsBackToLiteral(t *testing.T) {
	got := extractMatchSnippet("a [b] c", "[b", true, false)
	if got != "[b" {
		t.Fatalf("extractMatchSnippet = %q, want %q", got, "[b")
	}
}

func TestIndexFoldASCIIPositions(t *testing.T) {
	cases := []struct {
		s, sub string
		want   int
	}{
		{"Hello HELLO", "hello", 0},
		{"aaa ERROR bbb", "error", 4},
		{"warning", "error", -1},
		{"", "x", -1},
		{"abc", "", 0},
		{"ab", "abc", -1},
		{"MiXeD CaSe", "case", 6},
		{"ünïcode ERROR", "error", 10},
		{"A", "a", 0},
		{"a", "A", 0},
	}

	for _, c := range cases {
		if got := indexFoldASCII(c.s, c.sub); got != c.want {
			t.Fatalf("indexFoldASCII(%q,%q)=%d want %d", c.s, c.sub, got, c.want)
		}
	}
}

func TestFindSpansCaseInsensitiveASCIINoLowerCopy(t *testing.T) {
	s := "ERROR error Error eRRoR"
	spans := findSpans(s, "error", false, false, 0, hlPrimaryStyle)
	if len(spans) != 4 {
		t.Fatalf("got %d spans: %+v", len(spans), spans)
	}

	want := []int{0, 6, 12, 18}
	for i, sp := range spans {
		if sp.start != want[i] || sp.end != want[i]+5 {
			t.Fatalf("span %d = %+v, want start %d", i, sp, want[i])
		}
	}
}

func TestFindSpansCaseInsensitiveASCIIMixedContent(t *testing.T) {
	// Pattern is ASCII but the segment is not: the fold scan must still find
	// the ASCII needle at the right byte offset and keep rune-boundary ends.
	s := "Ünïcode ERROR line"
	spans := findSpans(s, "error", false, false, 0, hlPrimaryStyle)
	wantStart := strings.Index(s, "ERROR")
	if len(spans) != 1 || spans[0].start != wantStart || spans[0].end != wantStart+len("ERROR") {
		t.Fatalf("spans = %+v, want start %d", spans, wantStart)
	}
}

func TestFindSpansCaseInsensitiveNonASCIIPattern(t *testing.T) {
	// Non-ASCII pattern keeps the lowered-copy path, including Unicode
	// folding: "Привет" matches "привет" case-insensitively.
	s := "лог: Привет мир"
	spans := findSpans(s, "привет", false, false, 0, hlPrimaryStyle)
	if len(spans) != 1 {
		t.Fatalf("got %d spans: %+v", len(spans), spans)
	}

	if s[spans[0].start:spans[0].end] != "Привет" {
		t.Fatalf("span text = %q", s[spans[0].start:spans[0].end])
	}
}

func TestFindSpansRegexUsesCache(t *testing.T) {
	resetRegexCache()
	defer resetRegexCache()

	re1, err := cachedRegex("ERR-[0-9]+", false)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}

	spans := findSpans("x ERR-7 y err-9", "ERR-[0-9]+", true, false, 0, hlPrimaryStyle)
	if len(spans) != 2 {
		t.Fatalf("got %d spans: %+v", len(spans), spans)
	}

	if re := getCachedRegex("(?i)ERR-[0-9]+"); re != re1 {
		t.Fatal("findSpans recompiled the pattern instead of using the cache")
	}
}
