package tui

import (
	"regexp"
	"sync"
	"time"
)

// tuiReCacheTTL bounds the highlight-path regex cache lifetime, matching the
// search engine's defaultReCacheTTL (M1): a 5-minute window keeps repeated
// renders fast without pinning abandoned patterns for the whole session.
const tuiReCacheTTL = 5 * time.Minute

// tuiReCacheMax caps the cached patterns; on insert the first-inserted entry
// is evicted when full, so sweeping many distinct highlight patterns (e.g. a
// long exclude list) cannot grow the cache indefinitely.
const tuiReCacheMax = 32

// tuiReCacheEntry pairs a compiled regex with the time it was cached, so
// entries can be expired by TTL in addition to the count cap.
type tuiReCacheEntry struct {
	re       *regexp.Regexp
	cachedAt time.Time
}

var (
	// tuiReCacheMu guards the cache below. The lock is held only inside the
	// get/put helpers, never across compilation or matching, so concurrent
	// renders never serialize behind it.
	tuiReCacheMu sync.Mutex

	// tuiReCache caches compiled regexes for the TUI highlight path, keyed by
	// the flag-prefixed pattern (see regexPattern).
	tuiReCache = make(map[string]tuiReCacheEntry)

	// tuiReCacheOrder records insertion order so eviction can remove the
	// first-inserted entry deterministically instead of relying on Go's
	// randomized map iteration.
	tuiReCacheOrder []string
)

// regexPattern builds the compiled-pattern key exactly like the search
// engine's regexPattern: case-insensitive searches get the "(?i)" prefix so
// the two flag settings map to distinct cache entries instead of colliding.
func regexPattern(pattern string, caseSensitive bool) string {
	if caseSensitive {
		return pattern
	}

	return "(?i)" + pattern
}

// cachedRegex returns the compiled regex for pattern under the given case
// sensitivity, serving repeated calls from a small package-level cache.
// Compiling per rendered row — findSpans runs on every visible line, every
// frame, via paintSearchTerms — cost ~25x the match itself (~30 allocs and
// ~34µs per row), so the compiled pattern is served from the cache instead
// and the compile happens once per distinct pattern. Invalid patterns return
// the compile error uncached; callers keep their existing literal fallback.
func cachedRegex(pattern string, caseSensitive bool) (*regexp.Regexp, error) {
	key := regexPattern(pattern, caseSensitive)

	if re := getCachedRegex(key); re != nil {
		return re, nil
	}

	re, err := regexp.Compile(key)
	if err != nil {
		return nil, err
	}

	putCachedRegex(key, re)

	return re, nil
}

// getCachedRegex returns the cached compiled regex for key, or nil when
// absent or expired past tuiReCacheTTL. Aged entries are dropped on access
// rather than swept: no goroutine has to keep time for the cache.
func getCachedRegex(key string) *regexp.Regexp {
	now := time.Now()

	tuiReCacheMu.Lock()
	defer tuiReCacheMu.Unlock()

	entry, exists := tuiReCache[key]
	if !exists {
		return nil
	}

	if now.Sub(entry.cachedAt) > tuiReCacheTTL {
		delete(tuiReCache, key)

		return nil
	}

	return entry.re
}

// putCachedRegex inserts a freshly compiled regex into the cache, evicting
// the first-inserted entry when at capacity. Re-inserting a live key just
// refreshes it, so the insertion-order log never holds duplicates.
func putCachedRegex(key string, re *regexp.Regexp) {
	tuiReCacheMu.Lock()
	defer tuiReCacheMu.Unlock()

	if _, exists := tuiReCache[key]; exists {
		tuiReCache[key] = tuiReCacheEntry{re: re, cachedAt: time.Now()}

		return
	}

	if len(tuiReCache) >= tuiReCacheMax {
		for len(tuiReCacheOrder) > 0 {
			oldest := tuiReCacheOrder[0]
			tuiReCacheOrder = tuiReCacheOrder[1:]

			if _, ok := tuiReCache[oldest]; ok {
				delete(tuiReCache, oldest)

				break
			}
		}
	}

	tuiReCache[key] = tuiReCacheEntry{re: re, cachedAt: time.Now()}
	tuiReCacheOrder = append(tuiReCacheOrder, key)
}
