// Package search provides a parallel search engine over an indexed log file.
//
// SearchEngine fans a line range across a worker pool (up to runtime.NumCPU
// goroutines) and merges the matched-line indices back in order. It supports
// substring and regex matching, case sensitivity, and invert-match, and is
// safe for concurrent use from the tail poller and the UI.
package search

import (
	"GoFLE/pkg/indexer"
	"bytes"
	"context"
	"errors"
	"fmt"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"
)

// reCacheEntry pairs a compiled regex with the time it was cached, so entries
// can be expired by TTL (M1) in addition to the count-based LRU eviction.
type reCacheEntry struct {
	re       *regexp.Regexp
	cachedAt time.Time
}

// SearchOptions configures one search: the query, whether it is a regex, whether matching is case-sensitive, and whether matches are inverted.
type SearchOptions struct { //nolint:revive // renaming would break the public search API used across the app
	Query string `json:"query"`
	// SecondaryQuery, when non-empty, narrows the result set with AND
	// semantics: a line counts only if it matches BOTH Query (after
	// InvertMatch) and SecondaryQuery. Flags IsRegex/CaseSensitive apply to
	// both patterns.
	SecondaryQuery string `json:"secondary_query,omitempty"`
	// SecondaryMode is "and" (default) or "or" for combining primary and secondary.
	SecondaryMode string `json:"secondary_mode,omitempty"`
	IsRegex        bool   `json:"is_regex"`
	CaseSensitive  bool   `json:"case_sensitive"`
	InvertMatch    bool   `json:"invert_match"`

	// --- Advanced filters (optional; empty = disabled) ---

	// ExcludePatterns drop lines that match any pattern (after primary/secondary).
	ExcludePatterns []string `json:"exclude_patterns,omitempty"`
	ExcludeIsRegex  bool     `json:"exclude_is_regex,omitempty"`

	// NearbyQuery requires another pattern within NearbyWindow lines.
	// NearbyMode: "before" | "after" | "either" (default either).
	NearbyQuery  string `json:"nearby_query,omitempty"`
	NearbyMode   string `json:"nearby_mode,omitempty"`
	NearbyWindow int    `json:"nearby_window,omitempty"`

	// Levels keeps only lines whose content looks like one of ERROR/WARN/INFO/DEBUG/OTHER.
	Levels []string `json:"levels,omitempty"`

	// LineStart/LineEnd limit the primary scan to [LineStart, LineEnd).
	// Zero LineEnd means "until EOF". LineStart is 0-based.
	LineStart int64 `json:"line_start,omitempty"`
	LineEnd   int64 `json:"line_end,omitempty"`

	// TimeFrom/TimeTo are inclusive wall-clock bounds (RFC3339 or "2006-01-02 15:04:05").
	// Lines without a parseable timestamp are kept unless TimeRequireStamp is set.
	TimeFrom         string `json:"time_from,omitempty"`
	TimeTo           string `json:"time_to,omitempty"`
	TimeRequireStamp bool   `json:"time_require_stamp,omitempty"`

	// MatchAll returns every line in the search range without content matching
	// (used for empty primary + advanced post-filters).
	MatchAll bool `json:"match_all,omitempty"`

	// ContextBefore / ContextAfter expand each match with neighboring lines in results.
	ContextBefore int `json:"context_before,omitempty"`
	ContextAfter  int `json:"context_after,omitempty"`
}

// Options is an alias for SearchOptions.
type Options = SearchOptions

// SearchEngine runs parallel, chunked searches over a bound FileIndexer. It
// is stateless with respect to search results: every Search/SearchRange call
// returns a freshly allocated, caller-owned slice and retains nothing, so the
// engine never pins a second copy of potentially huge match sets (F1). The
// only shared mutable state is the compiled-regex cache below.
type SearchEngine struct { //nolint:revive // renaming would break the public search API used across the app
	indexer *indexer.FileIndexer

	// mu guards reCache, which after result-retention removal (F1) is the
	// engine's single shared mutable field. It is held only for the small
	// cache lookup/insert critical sections inside getCachedRegex/
	// putCachedRegex — never across the seconds-long worker-pool scan — so
	// concurrent searches on one engine (UI + tail poller) no longer
	// serialize behind a full-search-duration lock (F2).
	mu sync.Mutex

	// reCache is an LRU cache for compiled regexes. The key is the raw query
	// pattern string (with (?i) prefix for case-insensitive). This prevents
	// recompiling the same regex on every search invocation, which is the
	// single most costly CPU operation in the search path. Entries expire by
	// TTL (reCacheTTL) as well as by the count cap, so a long-running session
	// that sweeps many distinct patterns cannot accumulate stale entries
	// indefinitely (M1).
	reCache map[string]reCacheEntry
	// reCacheMaxSize limits the number of cached regexes to prevent memory
	// growth. When the cache is full, the least-recently-inserted entry is evicted.
	reCacheMaxSize int
	// reCacheTTL is how long a compiled regex stays usable before it is evicted
	// on next access. It bounds memory under sustained, varying search traffic.
	reCacheTTL time.Duration
}

// defaultReCacheTTL bounds regex-cache lifetime (M1). A 5-minute window keeps
// interactive re-searches fast while preventing long-lived sessions from
// pinning patterns that are no longer used.
const defaultReCacheTTL = 5 * time.Minute

// NewSearchEngine creates a search engine bound to a FileIndexer. The engine
// retains no search results; callers own the returned match slices.
func NewSearchEngine(idx *indexer.FileIndexer) *SearchEngine {
	return &SearchEngine{
		indexer:        idx,
		reCache:        make(map[string]reCacheEntry),
		reCacheMaxSize: 16,
		reCacheTTL:     defaultReCacheTTL,
	}
}

// Search runs a full search over the entire index.
// It is a convenience wrapper around SearchWithContext with a background context.
//
// Parameters:
//   - opts: SearchOptions containing the query, regex flag, case sensitivity, and invert match.
//
// Returns a slice of matched line indices (0-based), or an error if the query is invalid.
//
// Play: https://go.dev/play/p/SEARCH_SEARCH
//
// Example:
//
//	idx, _ := indexer.NewFileIndexer("app.log")
//	se := search.NewSearchEngine(idx)
//	matches, _ := se.Search(search.SearchOptions{Query: "ERROR"})
//	fmt.Printf("Found %d matches\n", len(matches))
func (se *SearchEngine) Search(opts SearchOptions) ([]int64, error) {
	return se.SearchWithContext(context.Background(), opts)
}

// SearchWithContext runs a full search over the entire index and returns the
// matched line indices as a freshly allocated, caller-owned slice. The engine
// stores nothing, so concurrent searches cannot corrupt each other's results.
// Cancellation is checked between chunks via ctx; a single-chunk range is not interruptible mid-scan.
//
// Parameters:
//   - ctx: Context for cancellation. Checked between chunks.
//   - opts: SearchOptions containing the query, regex flag, case sensitivity, and invert match.
//
// Returns a slice of matched line indices (0-based), or an error if the query is invalid or context is cancelled.
//
// Example:
//
//	idx, _ := indexer.NewFileIndexer("app.log")
//	se := search.NewSearchEngine(idx)
//	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
//	defer cancel()
//	matches, _ := se.SearchWithContext(ctx, search.SearchOptions{Query: "ERROR", CaseSensitive: true})
func (se *SearchEngine) SearchWithContext(ctx context.Context, opts SearchOptions) ([]int64, error) {
	return se.matchRange(ctx, opts, 0, se.indexer.TotalLines(), nil)
}

// SearchWithProgress is SearchWithContext plus an optional progress callback
// invoked with the running match count as each chunk completes (unordered
// arrival; count is cumulative over finished chunks). onProgress may be nil.
func (se *SearchEngine) SearchWithProgress(ctx context.Context, opts SearchOptions, onProgress func(int64)) ([]int64, error) {
	return se.matchRange(ctx, opts, 0, se.indexer.TotalLines(), onProgress)
}

// SearchRange searches only the half-open line range [start, end) and returns
// the matched indices. Like SearchWithContext it is stateless — nothing is
// stored on the engine — which makes it safe to run concurrently with a full
// search while incrementally extending results for a tailed file
// (see app.pollFileUpdates).
// Cancellation is checked between chunks via ctx; a single-chunk range is not interruptible mid-scan.
//
// Parameters:
//   - ctx: Context for cancellation. Checked between chunks.
//   - opts: SearchOptions containing the query, regex flag, case sensitivity, and invert match.
//   - start: The 0-based index of the first line to search (inclusive).
//   - end: The 0-based index of the line after the last line to search (exclusive).
//
// Returns a slice of matched line indices within [start, end), or an error if the query is invalid or context is cancelled.
func (se *SearchEngine) SearchRange(ctx context.Context, opts SearchOptions, start, end int64) ([]int64, error) {
	return se.matchRange(ctx, opts, start, end, nil)
}

// regexPattern turns a user query into the pattern that is actually compiled:
// case-insensitive searches get the (?i) prefix. It is also the cache key, so
// the same text searched with and without the case flag maps to two distinct
// entries instead of colliding on one (M6).
func regexPattern(query string, caseSensitive bool) string {
	if caseSensitive {
		return query
	}

	return "(?i)" + query
}

// compileCached returns the compiled regex for an already-prepared pattern
// (see regexPattern), compiling and caching it on a miss. The cache lock is
// taken only inside the get/put helpers, never across compilation or scanning.
func (se *SearchEngine) compileCached(pattern string) (*regexp.Regexp, error) {
	if re := se.getCachedRegex(pattern); re != nil {
		return re, nil
	}

	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, err
	}

	se.putCachedRegex(pattern, re)

	return re, nil
}

// getCachedRegex returns the cached compiled regex for pattern, or nil when
// absent or expired past reCacheTTL. The lock is held only for this map
// access, never across scanning or compilation (F2).
func (se *SearchEngine) getCachedRegex(pattern string) *regexp.Regexp {
	now := time.Now()

	se.mu.Lock()
	defer se.mu.Unlock()

	entry, exists := se.reCache[pattern]
	if !exists {
		return nil
	}
	// Aged entries are dropped on access rather than swept: no goroutine has
	// to keep time for the cache.
	if now.Sub(entry.cachedAt) > se.reCacheTTL {
		delete(se.reCache, pattern)
		return nil
	}

	return entry.re
}

// putCachedRegex inserts a freshly compiled regex into the cache. It
// lazy-initializes a zero-value engine's map (nil map write would panic) and
// evicts the first (least-recently-inserted) entry when at capacity. Like
// getCachedRegex, the critical section is just the map mutation.
func (se *SearchEngine) putCachedRegex(pattern string, re *regexp.Regexp) {
	se.mu.Lock()
	defer se.mu.Unlock()

	if se.reCache == nil {
		se.reCache = make(map[string]reCacheEntry)
	}

	if len(se.reCache) >= se.reCacheMaxSize {
		for key := range se.reCache {
			delete(se.reCache, key)
			break
		}
	}

	se.reCache[pattern] = reCacheEntry{re: re, cachedAt: time.Now()}
}

type chunkResult struct {
	seq     int
	matches []int64
	err     error
}

// SearchResult contains the results of a search, optionally including level counts.
type SearchResult struct {
	Matches    []int64
	ErrorCount int64
	WarnCount  int64
}

// SearchWithLevelCounts runs a search and simultaneously counts log level markers
// (ERROR/FATAL/PANIC = E, WARN = W) in the scanned range. This fuses the search
// pass and level stats pass into a single ScanLines call, avoiding a full-file
// re-read when both are needed (e.g., UI showing search results + level badges).
//
// Parameters:
//   - ctx: Context for cancellation. Checked between chunks.
//   - opts: SearchOptions containing the query, regex flag, case sensitivity, and invert match.
//
// Returns a SearchResult containing matched line indices, error count, and warn count, or an error if the query is invalid or context is cancelled.
func (se *SearchEngine) SearchWithLevelCounts(ctx context.Context, opts SearchOptions) (SearchResult, error) {
	if opts.Query == "" {
		return SearchResult{Matches: []int64{}}, nil
	}

	totalLines := se.indexer.TotalLines()
	if totalLines == 0 {
		return SearchResult{Matches: []int64{}}, nil
	}

	// Run the search with level counting enabled
	matches, errCount, warnCount, err := se.matchRangeWithLevels(ctx, opts, 0, totalLines)
	return SearchResult{
		Matches:    matches,
		ErrorCount: errCount,
		WarnCount:  warnCount,
	}, err
}

// matchRangeWithLevels runs a search over [start, end) and simultaneously
// counts log level markers. It returns matches, error count, warn count, and error.
func (se *SearchEngine) matchRangeWithLevels(ctx context.Context, opts SearchOptions, start, end int64) (matches []int64, errCount, warnCount int64, err error) {
	if opts.Query == "" {
		return []int64{}, 0, 0, nil
	}

	totalLines := se.indexer.TotalLines()
	if end > totalLines {
		end = totalLines
	}

	if start < 0 || start >= end {
		return []int64{}, 0, 0, nil
	}

	var re *regexp.Regexp

	if opts.IsRegex {
		compiled, err := se.compileCached(regexPattern(opts.Query, opts.CaseSensitive))
		if err != nil {
			return nil, 0, 0, fmt.Errorf("invalid regular expression: %w", err)
		}

		re = compiled
	}

	queryLower := strings.ToLower(opts.Query)
	queryIsASCII := isASCII(opts.Query)

	queryBytes := []byte(opts.Query)
	queryLowerBytes := []byte(queryLower)
	var queryLowerRunes []rune

	secMatch, err := se.buildSecondaryMatcher(opts)
	if err != nil {
		return nil, 0, 0, err
	}

	if !opts.IsRegex && !opts.CaseSensitive && !queryIsASCII {
		queryLowerRunes = []rune(queryLower)
	}

	chunkSize := int64(3000)
	numChunks := (end - start + chunkSize - 1) / chunkSize
	numWorkers := 1
	if end-start > chunkSize {
		numWorkers = max(runtime.NumCPU(), 1)
	}

	type chunkResultWithLevels struct {
		seq      int
		matches  []int64
		errCount int64
		warnCount int64
		err      error
	}

	jobs := make(chan int64, numChunks)
	results := make(chan chunkResultWithLevels, numChunks)

	for i := start; i < end; i += chunkSize {
		jobs <- i
	}

	close(jobs)

	var wg sync.WaitGroup
	for range numWorkers {
		wg.Go(func() {
			defer func() {
				if r := recover(); r != nil {
					results <- chunkResultWithLevels{err: fmt.Errorf("search worker panicked: %v", r)}
				}
			}()

			for startIdx := range jobs {
				qseq := int((startIdx - start) / chunkSize)

				select {
				case <-ctx.Done():
					results <- chunkResultWithLevels{seq: qseq, err: ctx.Err()}
					return
				default:
				}

				count := min(chunkSize, end-startIdx)

				localMatches := make([]int64, 0, min(int(count), 64))
				localErrCount := int64(0)
				localWarnCount := int64(0)

				err := se.indexer.ScanLinesRetry(startIdx, count, func(idx int64, content []byte) bool {
					isMatch := false
					if opts.IsRegex {
						isMatch = re.Match(content)
					} else if opts.CaseSensitive {
						isMatch = bytes.Contains(content, queryBytes)
					} else if queryIsASCII {
						isMatch = containsFoldASCIIBytes(content, queryLowerBytes)
					} else {
						isMatch = containsFoldRune(content, queryLowerRunes)
					}

					if opts.InvertMatch {
						isMatch = !isMatch
					}

					if isMatch && secMatch != nil {
						isMatch = secMatch(content)
					}

					if isMatch {
						localMatches = append(localMatches, idx)
					}

					// Count level markers in the first levelScanWindow bytes
					window := content
					if len(window) > 160 { // levelScanWindow from indexer
						window = window[:160]
					}
					if bytes.Contains(window, []byte("ERROR")) ||
						bytes.Contains(window, []byte("FATAL")) ||
						bytes.Contains(window, []byte("PANIC")) {
						localErrCount++
					} else if bytes.Contains(window, []byte("WARN")) {
						localWarnCount++
					}

					return true
				})
				if err != nil {
					results <- chunkResultWithLevels{seq: qseq, err: err}
					return
				}

				results <- chunkResultWithLevels{
					seq:       qseq,
					matches:   localMatches,
					errCount:  localErrCount,
					warnCount: localWarnCount,
				}
			}
		})
	}

	wg.Wait()
	close(results)

	ordered := make([][]int64, numChunks)
	var totalMatches int
	var totalErrCount, totalWarnCount int64
	var errs []error
	var canceled bool

	for res := range results {
		if res.err != nil {
			if errors.Is(res.err, context.Canceled) || errors.Is(res.err, context.DeadlineExceeded) {
				canceled = true
				continue
			}
			errs = append(errs, res.err)
			continue
		}

		ordered[res.seq] = res.matches
		totalMatches += len(res.matches)
		totalErrCount += res.errCount
		totalWarnCount += res.warnCount
	}

	if canceled {
		return nil, 0, 0, fmt.Errorf("search canceled: %w", context.Canceled)
	}

	if len(errs) > 0 {
		return nil, 0, 0, errors.Join(errs...)
	}

	matches = slices.Concat(ordered...)
	return matches, totalErrCount, totalWarnCount, nil
}

// matchRange runs a goroutine-pooled, chunked scan over [start, end) of the indexer, applies opts, and returns matched indices in ascending order. It honors ctx cancellation between chunks (not mid-chunk), caches compiled regexes, and does not store the result on the engine.
func (se *SearchEngine) matchRange(ctx context.Context, opts SearchOptions, start, end int64, onProgress func(int64)) ([]int64, error) { //nolint:gocyclo,funlen // chunked pooled scan over the index is inherently branchy
	if opts.Query == "" && !opts.MatchAll {
		return []int64{}, nil
	}

	totalLines := se.indexer.TotalLines()
	if end > totalLines {
		end = totalLines
	}

	if start < 0 || start >= end {
		return []int64{}, nil
	}

	// Fast path: materialize every line index in range (no content I/O).
	// Skip when secondary is set — need per-line AND/OR evaluation.
	if opts.MatchAll && opts.SecondaryQuery == "" {
		n := end - start
		out := make([]int64, n)
		for i := int64(0); i < n; i++ {
			out[i] = start + i
		}
		if onProgress != nil {
			onProgress(n)
		}
		return out, nil
	}

	var re *regexp.Regexp

	if opts.IsRegex {
		// LRU + TTL cache (M1): the lock is held only inside the cache
		// helpers, so compilation and scanning run without it (F2).
		compiled, err := se.compileCached(regexPattern(opts.Query, opts.CaseSensitive))
		if err != nil {
			return nil, fmt.Errorf("invalid regular expression: %w", err)
		}

		re = compiled
	}

	queryLower := strings.ToLower(opts.Query)
	// When the query is pure ASCII we can do a case-insensitive substring test
	// without allocating a lowered copy of every line (the dominant cost of a
	// full-file case-insensitive search). Non-ASCII queries take the rune-wise
	// fold compare so Unicode folding semantics are preserved exactly.
	queryIsASCII := isASCII(opts.Query)

	// Hoist the needle conversions out of the per-line closure (O1): the query
	// never changes during a search, so converting it to a byte slice once —
	// instead of []byte(opts.Query)/[]byte(queryLower) on every line — removes
	// ~one allocation per scanned line (~10M on a full 10M-line search).
	queryBytes := []byte(opts.Query)
	queryLowerBytes := []byte(queryLower)
	// Rune-wise fold path needs the query as pre-lowered runes; decoding them
	// once here keeps the per-line compare fully allocation-free (F3).
	var queryLowerRunes []rune
	// Secondary (AND) matcher, built once per search via the shared builder so
	// its semantics exactly match FilterLines' refine path (M6). Returns nil
	// when there is no secondary query, matching the previous inlined behavior.
	secMatch, err := se.buildSecondaryMatcher(opts)
	if err != nil {
		return nil, err
	}

	if !opts.IsRegex && !opts.CaseSensitive && !queryIsASCII {
		queryLowerRunes = []rune(queryLower)
	}

	chunkSize := int64(3000)
	numChunks := (end - start + chunkSize - 1) / chunkSize
	// Spawning NumCPU goroutines only pays off for a large range. The 500ms
	// tail poll re-searches a single new chunk, so a full worker pool would
	// leave N-1 goroutines idle and immediately exiting — gate parallelism
	// behind a multi-chunk range.
	numWorkers := 1
	if end-start > chunkSize {
		numWorkers = max(runtime.NumCPU(), 1)
	}

	jobs := make(chan int64, numChunks)
	results := make(chan chunkResult, numChunks)

	for i := start; i < end; i += chunkSize {
		jobs <- i
	}

	close(jobs)

	var wg sync.WaitGroup
	for range numWorkers {
		wg.Go(func() {
			// A panicking worker must not take down the whole process: convert
			// the panic into the chunk's error path. seq is unknown at recover
			// time, so it stays unset — the drain below only reads seq for
			// non-error results.
			defer func() {
				if r := recover(); r != nil {
					results <- chunkResult{err: fmt.Errorf("search worker panicked: %v", r)}
				}
			}()

			for startIdx := range jobs {
				qseq := int((startIdx - start) / chunkSize)

				select {
				case <-ctx.Done():
					results <- chunkResult{seq: qseq, err: ctx.Err()}
					return
				default:
				}

				count := min(chunkSize, end-startIdx)

				// Cap the per-chunk accumulator. Preallocating the full chunk
				// size (up to 3000) wastes ~1.6 MB per 200k-line search in GC
				// churn even though most chunks match few lines. A small cap
				// keeps sparse matches realloc-free while dense chunks (e.g. an
				// always-matching query) grow via amortised append.
				localMatches := make([]int64, 0, min(int(count), 64))
				// ScanLines reads the whole searched range in a single ReadAt and
				// hands each line to fn as a []byte, so the match test runs
				// allocation-free on the (common) non-matching lines — previously
				// GetLines allocated a string per line (one per search hit too).
				err := se.indexer.ScanLinesRetry(startIdx, count, func(idx int64, content []byte) bool {
					isMatch := false
					if opts.IsRegex { //nolint:gocritic // distinct boolean flags; a switch is not applicable
						isMatch = re.Match(content)
					} else if opts.CaseSensitive {
						isMatch = bytes.Contains(content, queryBytes)
					} else if queryIsASCII {
						isMatch = containsFoldASCIIBytes(content, queryLowerBytes)
					} else {
						isMatch = containsFoldRune(content, queryLowerRunes)
					}

					if opts.MatchAll {
						isMatch = true
					}
					if opts.InvertMatch {
						isMatch = !isMatch
					}
					// Secondary combines with primary: AND (narrow) or OR (union on same line).
					if secMatch != nil {
						secHit := secMatch(content)
						if strings.EqualFold(opts.SecondaryMode, "or") {
							isMatch = isMatch || secHit
						} else if isMatch {
							isMatch = secHit
						}
					}

					if isMatch {
						localMatches = append(localMatches, idx)
					}

					return true
				})
				if err != nil {
					results <- chunkResult{seq: qseq, err: err}
					return
				}

				results <- chunkResult{seq: qseq, matches: localMatches}
			}
		})
	}

	wg.Wait()
	close(results)

	// Chunks are disjoint, internally ascending, and emitted in increasing
	// startIdx order, so indexing each result by its chunk ordinal and
	// concatenating yields a globally sorted, de-duplicated result with no map
	// and no O(range) resort pass.
	ordered := make([][]int64, numChunks)

	var (
		totalMatches int
		errs         []error
		canceled     bool
	)

	for res := range results {
		if res.err != nil {
			if errors.Is(res.err, context.Canceled) || errors.Is(res.err, context.DeadlineExceeded) {
				canceled = true
				continue
			}
			// Collect every worker failure: returning on the first one used to
			// discard the rest and made identical retries look flaky.
			errs = append(errs, res.err)

			continue
		}

		ordered[res.seq] = res.matches
		totalMatches += len(res.matches)
		if onProgress != nil {
			onProgress(int64(totalMatches))
		}
	}

	// A canceled search must not masquerade as a completed partial result:
	// callers present (matches, nil) as truth, so cancellation is surfaced.
	if canceled {
		return nil, fmt.Errorf("search canceled: %w", context.Canceled)
	}

	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}

	matches := slices.Concat(ordered...)

	return matches, nil
}

// buildSecondaryMatcher compiles the SecondaryQuery of opts into a fast
// []byte predicate, mirroring exactly the AND-filter logic used inside
// matchRange. It is shared so FilterLines (refine) and matchRange (full scan)
// apply the identical semantics (M6).
func (se *SearchEngine) buildSecondaryMatcher(opts SearchOptions) (func([]byte) bool, error) {
	if opts.SecondaryQuery == "" {
		return nil, nil
	}

	switch {
	case opts.IsRegex:
		compiled, err := se.compileCached(regexPattern(opts.SecondaryQuery, opts.CaseSensitive))
		if err != nil {
			return nil, fmt.Errorf("invalid secondary regular expression: %w", err)
		}

		return compiled.Match, nil
	case opts.CaseSensitive:
		q2 := []byte(opts.SecondaryQuery)
		return func(c []byte) bool { return bytes.Contains(c, q2) }, nil
	default:
		lower2 := strings.ToLower(opts.SecondaryQuery)
		b2 := []byte(lower2)
		r2 := []rune(lower2)
		a2 := isASCII(opts.SecondaryQuery)

		return func(c []byte) bool {
			if a2 {
				return containsFoldASCIIBytes(c, b2)
			}

			return containsFoldRune(c, r2)
		}, nil
	}
}

// FilterLines returns the subset of lines whose content matches opts.SecondaryQuery,
// honoring opts.IsRegex/CaseSensitive. It is used to refine an existing match set
// when only the AND-filter (filter-in-filter) changed, turning an O(totalLines)
// full-file rescan into an O(matches) pass (H2). With an empty SecondaryQuery it
// returns a clone of lines unchanged.
//
// Parameters:
//   - ctx: Context for cancellation (currently unused but kept for API consistency).
//   - opts: SearchOptions containing SecondaryQuery, IsRegex, and CaseSensitive.
//   - lines: A slice of line indices to filter.
//
// Returns a filtered slice of line indices, or an error if SecondaryQuery is invalid.
func (se *SearchEngine) FilterLines(ctx context.Context, opts SearchOptions, lines []int64) ([]int64, error) {
	if opts.SecondaryQuery == "" {
		return slices.Clone(lines), nil
	}

	secMatch, err := se.buildSecondaryMatcher(opts)
	if err != nil {
		return nil, err
	}

	out := make([]int64, 0, len(lines))
	for i, ln := range lines {
		if i&0xff == 0 { // every 256 lines
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			default:
			}
		}
		content, e := se.indexer.GetLine(ln)
		if e != nil {
			return nil, e
		}

		if secMatch([]byte(content)) {
			out = append(out, ln)
		}
	}

	return out, nil
}

// isASCII reports whether s contains only ASCII bytes.
func isASCII(s string) bool {
	for i := range s {
		if s[i] >= 0x80 {
			return false
		}
	}

	return true
}

// containsFoldASCII reports whether s contains sub as a case-insensitive
// substring, without allocating. Both s and the already-lowercased sub are
// assumed ASCII; uppercase letters in s are folded to lower on the fly. This
// avoids the per-line strings.ToLower copy that dominates a full-file
// case-insensitive search over millions of lines.
func containsFoldASCII(s, sub string) bool {
	n, m := len(s), len(sub)
	if m == 0 {
		return true
	}

	if m > n {
		return false
	}

	for i := range n - m + 1 {
		ok := true

		for j := range m {
			c := s[i+j]
			if 'A' <= c && c <= 'Z' {
				c += 'a' - 'A'
			}

			if c != sub[j] {
				ok = false
				break
			}
		}

		if ok {
			return true
		}
	}

	return false
}

// containsFoldASCIIBytes is the []byte twin of containsFoldASCII, used by the
// zero-copy ScanLines search path so matching never materializes a string for
// the (common) non-matching lines.
func containsFoldASCIIBytes(s, sub []byte) bool {
	n, m := len(s), len(sub)
	if m == 0 {
		return true
	}

	if m > n {
		return false
	}

	for i := 0; i <= n-m; i++ {
		ok := true

		for j := range m {
			c := s[i+j]
			if 'A' <= c && c <= 'Z' {
				c += 'a' - 'A'
			}

			if c != sub[j] {
				ok = false
				break
			}
		}

		if ok {
			return true
		}
	}

	return false
}

// containsFoldRune is the non-ASCII counterpart of containsFoldASCIIBytes: it
// reports whether content contains queryLower as a case-insensitive substring,
// decoding content rune-by-rune and folding each rune with unicode.ToLower
// before comparison. queryLower must be pre-lowered (strings.ToLower) and
// pre-decoded by the caller once per search, so the per-line compare performs
// no allocation at all — replacing the old fallback's per-line
// bytes.Contains(bytes.ToLower(content), ...) copy (~1 alloc + ~line-length
// bytes per scanned line, i.e. millions of allocations on a full search).
//
// Semantics: equivalent to the replaced fallback for all inputs. Both sides
// apply the same simple unicode.ToLower mapping per rune; a lowered rune never
// merges with or splits into neighboring runes, and UTF-8 is self-synchronizing,
// so any encoded occurrence of queryLower inside ToLower(content) must start at
// a rune boundary — which is exactly where this compare looks. Invalid UTF-8
// bytes decode to utf8.RuneError on both sides (bytes.ToLower re-encodes them
// as U+FFFD), so they compare equal the same way. Honest caveat: this — like
// the code it replaces — implements simple case mapping, NOT full Unicode case
// folding: "ß" does not match "SS", and "İ" (U+0130) folds to plain "i" without
// its combining dot. Realistic log content (Cyrillic, Greek, accented Latin)
// folds identically under both.
func containsFoldRune(content []byte, queryLower []rune) bool {
	m := len(queryLower)
	if m == 0 {
		return true
	}

	first := queryLower[0]

	for i := 0; i < len(content); {
		r, size := utf8.DecodeRune(content[i:])
		if unicode.ToLower(r) == first && foldRuneAt(content[i+size:], queryLower[1:]) {
			return true
		}

		i += size
	}

	return false
}

// foldRuneAt reports whether content starts with the folded runes of want.
func foldRuneAt(content []byte, want []rune) bool {
	i := 0
	for _, w := range want {
		if i >= len(content) {
			return false
		}

		r, size := utf8.DecodeRune(content[i:])
		if unicode.ToLower(r) != w {
			return false
		}

		i += size
	}

	return true
}



// ApplyAdvancedFilters post-processes primary match line indices in a single
// pass where possible. Priority: low memory (no extra full-file structures) and
// few indexer reads (small line cache for nearby windows).
func (se *SearchEngine) ApplyAdvancedFilters(ctx context.Context, opts SearchOptions, lines []int64) ([]int64, error) {
	needExclude := len(opts.ExcludePatterns) > 0
	needLevels := len(opts.Levels) > 0
	needTime := opts.TimeFrom != "" || opts.TimeTo != ""
	needNearby := opts.NearbyQuery != ""
	if !needExclude && !needLevels && !needTime && !needNearby {
		return lines, nil
	}

	var excludeMatchers []func([]byte) bool
	if needExclude {
		for _, p := range opts.ExcludePatterns {
			p = strings.TrimSpace(p)
			if p == "" {
				continue
			}
			m, err := se.buildSecondaryMatcher(SearchOptions{
				SecondaryQuery: p,
				IsRegex:        opts.ExcludeIsRegex,
				CaseSensitive:  opts.CaseSensitive,
			})
			if err != nil {
				return nil, fmt.Errorf("exclude pattern %q: %w", p, err)
			}
			excludeMatchers = append(excludeMatchers, m)
		}
		if len(excludeMatchers) == 0 {
			needExclude = false
		}
	}

	allowLevel := map[string]bool{}
	if needLevels {
		for _, l := range opts.Levels {
			allowLevel[strings.ToUpper(strings.TrimSpace(l))] = true
		}
	}

	var from, to time.Time
	if needTime {
		var err error
		if opts.TimeFrom != "" {
			from, err = parseFlexibleTime(opts.TimeFrom)
			if err != nil {
				return nil, fmt.Errorf("time_from: %w", err)
			}
		}
		if opts.TimeTo != "" {
			to, err = parseFlexibleTime(opts.TimeTo)
			if err != nil {
				return nil, fmt.Errorf("time_to: %w", err)
			}
		}
	}

	var nearbyMatch func([]byte) bool
	win := opts.NearbyWindow
	mode := strings.ToLower(strings.TrimSpace(opts.NearbyMode))
	if needNearby {
		if win <= 0 {
			win = 10
		}
		if win > 200 {
			win = 200 // hard cap — protects against pathological windows
		}
		if mode == "" {
			mode = "either"
		}
		var err error
		nearbyMatch, err = se.buildSecondaryMatcher(SearchOptions{
			SecondaryQuery: opts.NearbyQuery,
			IsRegex:        opts.IsRegex,
			CaseSensitive:  opts.CaseSensitive,
		})
		if err != nil {
			return nil, fmt.Errorf("nearby query: %w", err)
		}
	}

	// Tiny LRU-ish cache for nearby window reads (line index → content).
	// Bounded to keep memory flat on huge match sets.
	const cacheCap = 256
	type cacheEntry struct {
		ln  int64
		txt string
	}
	cache := make([]cacheEntry, 0, cacheCap)
	getLine := func(ln int64) (string, error) {
		for i := len(cache) - 1; i >= 0; i-- {
			if cache[i].ln == ln {
				return cache[i].txt, nil
			}
		}
		txt, err := se.indexer.GetLine(ln)
		if err != nil {
			return "", err
		}
		if len(cache) >= cacheCap {
			// drop oldest half
			cache = append(cache[:0], cache[cacheCap/2:]...)
		}
		cache = append(cache, cacheEntry{ln: ln, txt: txt})
		return txt, nil
	}

	total := se.indexer.TotalLines()
	out := make([]int64, 0, min(len(lines), 4096)) // modest growth; append grows as needed

	for i, ln := range lines {
		if i&0xff == 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			default:
			}
		}

		content, err := getLine(ln)
		if err != nil {
			return nil, err
		}
		b := []byte(content)

		if needExclude {
			drop := false
			for _, m := range excludeMatchers {
				if m(b) {
					drop = true
					break
				}
			}
			if drop {
				continue
			}
		}

		if needLevels && !allowLevel[lineLevel(content)] {
			continue
		}

		if needTime {
			ts, ok := parseLineTime(content)
			if !ok {
				if opts.TimeRequireStamp {
					continue
				}
			} else {
				if !from.IsZero() && ts.Before(from) {
					continue
				}
				if !to.IsZero() && ts.After(to) {
					continue
				}
			}
		}

		if needNearby {
			var start, end int64
			switch mode {
			case "before":
				start = ln - int64(win)
				if start < 0 {
					start = 0
				}
				end = ln
			case "after":
				start = ln + 1
				end = ln + 1 + int64(win)
			default:
				start = ln - int64(win)
				if start < 0 {
					start = 0
				}
				end = ln + 1 + int64(win)
			}
			if end > total {
				end = total
			}
			found := false
			for j := start; j < end; j++ {
				if j == ln {
					continue
				}
				nb, e := getLine(j)
				if e != nil {
					continue
				}
				if nearbyMatch([]byte(nb)) {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}

		out = append(out, ln)
	}
	return out, nil
}


// ExpandMatchesWithContext inserts neighboring line indices around each match.
// Order is preserved; duplicates are removed. before/after are clamped to [0, 50].
func ExpandMatchesWithContext(matches []int64, before, after int, totalLines int64) []int64 {
	if len(matches) == 0 || totalLines <= 0 {
		return matches
	}
	if before < 0 {
		before = 0
	}
	if after < 0 {
		after = 0
	}
	if before > 50 {
		before = 50
	}
	if after > 50 {
		after = 50
	}
	if before == 0 && after == 0 {
		return matches
	}
	seen := make(map[int64]struct{}, len(matches)*(before+after+1))
	out := make([]int64, 0, len(matches)*(before+after+1))
	for _, m := range matches {
		start := m - int64(before)
		if start < 0 {
			start = 0
		}
		end := m + int64(after)
		if end >= totalLines {
			end = totalLines - 1
		}
		for i := start; i <= end; i++ {
			if _, ok := seen[i]; ok {
				continue
			}
			seen[i] = struct{}{}
			out = append(out, i)
		}
	}
	return out
}

func lineLevel(content string) string {
	// Case-insensitive substring without allocating ToUpper of the whole line.
	u := content
	if len(u) > 200 {
		u = u[:200]
	}
	if hasFold(u, "error") || hasFold(u, "fatal") || hasFold(u, "critical") {
		return "ERROR"
	}
	if hasFold(u, "warn") {
		return "WARN"
	}
	if hasFold(u, "info") {
		return "INFO"
	}
	if hasFold(u, "debug") || hasFold(u, "trace") {
		return "DEBUG"
	}
	return "OTHER"
}

func hasFold(s, sub string) bool {
	return strings.Contains(strings.ToLower(s), sub)
}

var timeLayouts = []string{
	time.RFC3339Nano,
	time.RFC3339,
	"2006-01-02T15:04:05.000Z07:00",
	"2006-01-02 15:04:05.000",
	"2006-01-02 15:04:05",
	"2006/01/02 15:04:05",
	"Jan _2 15:04:05",
	"Jan 2 15:04:05",
}

func parseLineTime(content string) (time.Time, bool) {
	prefix := content
	if len(prefix) > 40 {
		prefix = prefix[:40]
	}
	fields := strings.Fields(prefix)
	var candidates []string
	if len(fields) >= 1 {
		candidates = append(candidates, fields[0])
	}
	if len(fields) >= 2 {
		candidates = append(candidates, fields[0]+" "+fields[1])
	}
	if len(fields) >= 3 {
		candidates = append(candidates, fields[0]+" "+fields[1]+" "+fields[2])
	}
	for _, c := range candidates {
		for _, layout := range timeLayouts {
			if t, err := time.Parse(layout, c); err == nil {
				return t, true
			}
		}
	}
	return time.Time{}, false
}

func parseFlexibleTime(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	for _, layout := range timeLayouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t, nil
		}
	}
	if t, err := time.Parse("2006-01-02", s); err == nil {
		return t, nil
	}
	return time.Time{}, fmt.Errorf("unrecognized time %q", s)
}
