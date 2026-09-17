package main

import (
	"GoFLE/pkg/indexer"
	"GoFLE/pkg/search"
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// OpenFile opens a local log file and registers a tab. Opening an already-open
// path focuses the existing tab instead of duplicating it. Returns file
// statistics for the frontend.
func (a *App) OpenFile(filePath string) (*FileStatsResponse, error) {
	return a.openFileWithID(filePath, "")
}

// OpenFiles opens multiple local log files, skipping any that fail to open so a
// single bad path does not abort the batch. Returns stats for the files that
// opened successfully; every failure is emitted to the UI as a file_error event
// (with the offending path) so skipped entries are never silent.
func (a *App) OpenFiles(paths []string) ([]*FileStatsResponse, error) {
	responses := make([]*FileStatsResponse, 0, len(paths))
	for _, p := range paths {
		res, err := a.OpenFile(p)
		if err != nil {
			if a.ctx != nil {
				runtime.EventsEmit(a.ctx, "file_error", map[string]any{
					"path":  p,
					"error": err.Error(),
				})
			}

			continue
		}

		responses = append(responses, res)
	}

	return responses, nil
}

// openFileWithID indexes a local file and registers a tab. tabID may be empty
// (generate a fresh id) or an explicit id coming from a restored session.
func (a *App) openFileWithID(filePath, tabID string) (*FileStatsResponse, error) {
	// 1 tab = 1 file: reuse the existing tab instead of opening a duplicate.
	if existing := a.findOpenTab(filePath); existing != nil {
		return a.reuseOpenTab(existing)
	}

	indexPath, gzCleanup, err := indexer.EnsureDecompressed(filePath)
	if err != nil {
		return nil, err
	}

	idx, err := indexer.NewFileIndexer(indexPath)

	if gzCleanup != nil {
		defer func() {
			if idx == nil {
				gzCleanup() // failed before the tab took ownership
			} else if t := a.findOpenTab(filePath); t != nil {
				t.gzCleanup = gzCleanup
			}
		}()
	}

	if err != nil {
		return nil, err
	}

	tabID = newTabID("tab", tabID)
	fileName := filepath.Base(filePath)

	tab := &TabState{
		ID:           tabID,
		FilePath:     filePath,
		FileName:     fileName,
		Indexer:      idx,
		Searcher:     search.NewSearchEngine(idx),
		MatchedLines:    make([]int64, 0),
		PrimaryMatches:  make([]int64, 0),
		AutoScroll:      true,
	}

	a.mu.Lock()
	a.tabs[tabID] = tab
	a.activeTab = tabID
	a.mu.Unlock()

	// AddRecentFile already persists recent.json; settings are unchanged, so a
	// full Save() (rewriting both settings.json and recent.json) is redundant.
	// The open succeeded, so a failed recent-list persist is logged, not
	// returned — it must not turn into a phantom open error in the UI.
	if err := a.cfg.AddRecentFile(filePath, idx.FileSize(), false, nil); err != nil {
		slog.Warn("failed to persist recent entry", "path", filePath, "err", err)
	}

	return &FileStatsResponse{
		TabID:        tabID,
		FilePath:     filePath,
		FileName:     fileName,
		FileSize:     idx.FileSize(),
		TotalLines:   idx.TotalLines(),
		MatchedLines: 0,
	}, nil
}

// findOpenTab returns the already-open tab for a given path (local file path or
// the ssh:// display path), or nil. One path maps to at most one tab.
func (a *App) findOpenTab(filePath string) *TabState {
	a.mu.RLock()
	defer a.mu.RUnlock()

	for _, tab := range a.tabs {
		if tab.FilePath == filePath {
			return tab
		}
	}

	return nil
}

func (a *App) reuseOpenTab(tab *TabState) (*FileStatsResponse, error) {
	a.mu.Lock()
	tab.mu.Lock()
	a.activeTab = tab.ID
	tab.mu.Unlock()
	a.mu.Unlock()

	return a.tabStats(tab), nil
}

// tabStats builds the frontend response from a live tab without opening
// anything new (used for dedup reuse and id stability).
func (a *App) tabStats(tab *TabState) *FileStatsResponse {
	tab.mu.RLock()
	defer tab.mu.RUnlock()

	return &FileStatsResponse{
		TabID:        tab.ID,
		FilePath:     tab.FilePath,
		FileName:     tab.FileName,
		FileSize:     tab.Indexer.FileSize(),
		TotalLines:   tab.Indexer.TotalLines(),
		MatchedLines: int64(len(tab.MatchedLines)),
		IsSSH:        tab.IsSSH,
		IsFTP:        tab.IsFTP,
		SSH:          tab.SSHConfigSummary,
	}
}

// CloseTab closes a tab, releasing its indexer, SSH streamer, and temp file.
// Returns false if the tab did not exist.
func (a *App) CloseTab(tabID string) (bool, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	tab, exists := a.tabs[tabID]
	if !exists {
		return false, nil
	}

	tab.mu.Lock()

	if err := tab.Indexer.Close(); err != nil {
		slog.Warn("CloseTab: failed to close indexer", "tab_id", tabID, "err", err)
	}

	if tab.gzCleanup != nil {
		tab.gzCleanup()
		tab.gzCleanup = nil
	}

	if tab.SSHStreamer != nil {
		// Close asynchronously: a stuck SSH tail can make Streamer.Close block up
		// to its close timeout, which must not freeze the UI while closing a tab.
		s := tab.SSHStreamer
		tab.SSHStreamer = nil

		closeRemoteStreamerAsync(s)
	}

	if tab.FTPStreamer != nil {
		// Same rationale as SSH: a stuck FTP tail can make Streamer.Close block.
		s := tab.FTPStreamer
		tab.FTPStreamer = nil

		closeRemoteStreamerAsync(s)
	}

	tab.mu.Unlock()

	delete(a.tabs, tabID)

	if a.activeTab == tabID {
		a.activeTab = ""
	}

	return true, nil
}

// SetActiveTab makes the given tab the active one. Returns false if the tab does
// not exist.
func (a *App) SetActiveTab(tabID string) (bool, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if _, exists := a.tabs[tabID]; !exists {
		return false, nil
	}

	a.activeTab = tabID

	return true, nil
}

// RenameTab changes the display name of a tab. Returns false if the tab does not
// exist. The underlying file path is unchanged.
func (a *App) RenameTab(tabID, newName string) (bool, error) {
	a.mu.RLock()
	tab, exists := a.tabs[tabID]
	a.mu.RUnlock()

	if !exists {
		return false, nil
	}

	tab.mu.Lock()
	tab.FileName = newName
	tab.mu.Unlock()

	return true, nil
}

// GetLogLines returns a page of visible log lines [start, start+count) for the
// virtualized scroller. Unknown tabs yield an empty slice (not an error).
func (a *App) GetLogLines(tabID string, start, count int64) ([]LogLineResponse, error) {
	a.mu.RLock()
	tab, exists := a.tabs[tabID]
	a.mu.RUnlock()

	if !exists {
		return []LogLineResponse{}, nil
	}

	tab.mu.RLock()
	lines, err := tab.Indexer.GetLines(start, count)
	tab.mu.RUnlock()

	if err != nil {
		return nil, err
	}

	resp := make([]LogLineResponse, len(lines))
	for i, l := range lines {
		resp[i] = LogLineResponse{
			Index:   l.Index,
			Content: l.Content,
		}
	}

	return resp, nil
}

// looksLikeRegex reports whether q contains characters that only make sense in
// a regular expression. When the Regex toggle is on but the query is a plain
// literal, we demote to substring search (~20× faster on large files).
func looksLikeRegex(q string) bool {
	for i := 0; i < len(q); i++ {
		switch q[i] {
		case '.', '*', '+', '?', '|', '(', ')', '[', ']', '{', '}', '^', '$', '\\':
			return true
		}
	}
	return false
}

// SearchLines runs a search over a tab and stores the matched-line indices. An
// empty query clears the current matches. Regex longer than 256 chars is rejected
// (ReDoS hardening). Returns the match count; unknown tabs yield 0.
//
// Optimisations:
//   - literal queries with Regex enabled are demoted to substring matching;
//   - an in-flight search for the same tab is cancelled when a new one starts;
//   - changing only SecondaryQuery refines PrimaryMatches via FilterLines
//     (O(matches) instead of a full-file rescan);
//   - search_progress events stream partial counts to the UI while scanning.
func (a *App) SearchLines(tabID, query string, isRegex, caseSensitive, invertMatch bool, secondaryQuery string) (int64, error) {
	a.mu.RLock()
	tab, exists := a.tabs[tabID]
	a.mu.RUnlock()

	if !exists {
		return 0, nil
	}

	// Demote regex → substring when the pattern has no metacharacters.
	primaryRegex := isRegex && looksLikeRegex(query)
	// Secondary inherits the toggle but is demoted independently so a literal
	// AND-filter stays on the fast path even when primary is a real regex.
	secondaryRegex := isRegex && secondaryQuery != "" && looksLikeRegex(secondaryQuery)

	opts := search.SearchOptions{
		Query:          query,
		SecondaryQuery: secondaryQuery,
		IsRegex:        primaryRegex,
		CaseSensitive:  caseSensitive,
		InvertMatch:    invertMatch,
	}

	// security: bound regex complexity (ReDoS hardening).
	if primaryRegex && len(opts.Query) > maxRegexLen {
		return 0, fmt.Errorf("regular expression too long (max %d characters)", maxRegexLen)
	}

	if secondaryQuery != "" && secondaryRegex && len(secondaryQuery) > maxRegexLen {
		return 0, fmt.Errorf("secondary regular expression too long (max %d characters)", maxRegexLen)
	}

	// Cancel any in-flight search for this tab (typing / debounce).
	tab.searchCancelMu.Lock()
	if tab.searchCancel != nil {
		tab.searchCancel()
		tab.searchCancel = nil
	}
	parent := a.ctx
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithCancel(parent)
	tab.searchCancel = cancel
	tab.searchCancelMu.Unlock()

	tab.mu.Lock()
	prev := tab.LastSearchOpts
	tab.LastSearchOpts = opts
	// Preserve the caller's secondary in LastSearchOpts for refine detection even
	// though primary scans run without SecondaryQuery (applied via FilterLines).
	tab.LastSearchOpts.SecondaryQuery = secondaryQuery
	if query == "" {
		tab.MatchedLines = make([]int64, 0)
		tab.PrimaryMatches = make([]int64, 0)
		tab.searchGen++
		gen := tab.searchGen
		tab.mu.Unlock()
		cancel()
		if a.ctx != nil {
			runtime.EventsEmit(a.ctx, "search_progress", map[string]any{
				"tabId": tabID, "gen": gen, "count": int64(0), "done": true,
			})
		}
		return 0, nil
	}

	// Secondary-only change: refine cached primary hits in O(matches).
	onlySecondary := prev.Query == query &&
		prev.IsRegex == primaryRegex &&
		prev.CaseSensitive == caseSensitive &&
		prev.InvertMatch == invertMatch &&
		prev.SecondaryQuery != secondaryQuery &&
		len(tab.PrimaryMatches) > 0
	primarySnap := slices.Clone(tab.PrimaryMatches)
	gen := tab.searchGen + 1
	tab.searchGen = gen
	tab.mu.Unlock()

	emit := func(count int64, done bool) {
		if a.ctx == nil {
			return
		}
		runtime.EventsEmit(a.ctx, "search_progress", map[string]any{
			"tabId": tabID,
			"gen":   gen,
			"count": count,
			"done":  done,
		})
	}

	var matches []int64
	var err error

	if onlySecondary {
		// FilterLines honors SecondaryQuery; force its regex flag independently.
		fopts := opts
		fopts.SecondaryQuery = secondaryQuery
		fopts.IsRegex = secondaryRegex
		matches, err = tab.Searcher.FilterLines(ctx, fopts, primarySnap)
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return 0, err
			}
			return 0, err
		}
	} else {
		// Full primary scan without secondary (one fast pass), then optional refine.
		popts := opts
		popts.SecondaryQuery = ""
		popts.IsRegex = primaryRegex
		matches, err = tab.Searcher.SearchWithProgress(ctx, popts, func(partial int64) {
			emit(partial, false)
		})
		if err != nil {
			return 0, err
		}

		tab.mu.Lock()
		tab.PrimaryMatches = slices.Clone(matches)
		tab.mu.Unlock()

		if secondaryQuery != "" {
			fopts := opts
			fopts.SecondaryQuery = secondaryQuery
			fopts.IsRegex = secondaryRegex
			matches, err = tab.Searcher.FilterLines(ctx, fopts, matches)
			if err != nil {
				return 0, err
			}
		}
	}

	tab.mu.Lock()
	// Ignore stale results if a newer search already started.
	if tab.searchGen != gen {
		tab.mu.Unlock()
		return 0, context.Canceled
	}
	tab.MatchedLines = slices.Clone(matches)
	matchCount := int64(len(matches))
	tab.mu.Unlock()

	emit(matchCount, true)
	return matchCount, nil
}

// GetFilteredLines returns a page of matched lines [start, start+count) from the
// current search results. Unknown tabs or out-of-range requests yield an empty
// slice (not an error).
func (a *App) GetFilteredLines(tabID string, start, count int64) ([]LogLineResponse, error) {
	a.mu.RLock()
	tab, exists := a.tabs[tabID]
	a.mu.RUnlock()

	if !exists {
		return []LogLineResponse{}, nil
	}

	tab.mu.RLock()
	defer tab.mu.RUnlock()

	totalMatches := int64(len(tab.MatchedLines))
	if start < 0 || start >= totalMatches || count <= 0 {
		return []LogLineResponse{}, nil
	}

	end := min(start+count, totalMatches)

	resp := make([]LogLineResponse, 0, end-start)
	for i := start; i < end; i++ {
		lineIdx := tab.MatchedLines[i]

		content, err := tab.Indexer.GetLine(lineIdx)
		if err != nil {
			// Same convention as search.GetFilteredLines: an unreadable match
			// becomes a visible placeholder instead of silently shrinking the
			// page below the reported match count.
			content = fmt.Sprintf("<error reading line %d>", lineIdx)
		}

		resp = append(resp, LogLineResponse{
			Index:   lineIdx,
			Content: content,
		})
	}

	return resp, nil
}

// SetAutoScroll toggles live-tail auto-scroll for a tab (follow new lines as
// they arrive). Unknown tabs are ignored.
func (a *App) SetAutoScroll(tabID string, enabled bool) {
	a.mu.RLock()
	tab, exists := a.tabs[tabID]
	a.mu.RUnlock()

	if exists {
		tab.mu.Lock()
		tab.AutoScroll = enabled
		tab.mu.Unlock()
	}
}

// newTabID returns the runtime id for a new tab. When explicit is non-empty it
// is used verbatim — this is what lets RestoreTab keep the ids persisted in the
// session file, so the restored group membership and strip order still match.
func newTabID(prefix, explicit string) string {
	if explicit != "" {
		return explicit
	}

	return fmt.Sprintf("%s_%d_%d", prefix, time.Now().UnixNano(), tabIDSeq.Add(1))
}

// extractFilePathArgs scans the process arguments and returns those that refer
// to existing regular files. This lets the OS "Open with" / file-association
// flow pass log paths straight to the app on launch.
func extractFilePathArgs(args []string) []string {
	files := make([]string, 0)

	for i, a := range args {
		if i == 0 {
			continue // skip the binary path
		}

		if strings.HasPrefix(a, "-") {
			continue // skip flags
		}

		if info, err := os.Stat(a); err == nil && !info.IsDir() {
			files = append(files, a)
		}
	}

	return files
}

// ValidateRegex checks a regular expression for validity and length (capped at
// 256 chars for ReDoS hardening). An empty pattern is valid. The result carries a
// human-readable error message for the UI.
func (a *App) ValidateRegex(pattern string) (RegexValidationResult, error) {
	if pattern == "" {
		return RegexValidationResult{IsValid: true}, nil
	}
	// security: bound regex complexity (ReDoS hardening) — see maxRegexLen.
	if len(pattern) > maxRegexLen {
		return RegexValidationResult{
			IsValid:      false,
			ErrorMessage: fmt.Sprintf("regular expression too long (max %d characters)", maxRegexLen),
		}, nil
	}

	_, err := regexp.Compile(pattern)
	if err != nil {
		return RegexValidationResult{
			IsValid:      false,
			ErrorMessage: err.Error(),
		}, err
	}

	return RegexValidationResult{IsValid: true}, nil
}

// Level-marker needles, hoisted out of the per-line scan closure (M1):
// []byte("ERROR") & co. built inside the callback repeated the conversion for
// every line of the file, adding four throwaway slices per line to a pass that
// walks millions of them.
var (
	levelTokenError = []byte("ERROR")
	levelTokenFatal = []byte("FATAL")
	levelTokenPanic = []byte("PANIC")
	levelTokenWarn  = []byte("WARN")
)

const (
	// levelScanWindow bounds the per-line inspection to the conventional
	// placement of a level tag, so one pathological megabyte-long line costs
	// 160 bytes of matching instead of a megabyte.
	levelScanWindow = 160
	// levelStatsCancelStride is how many lines the counting pass processes
	// between context checks. A per-line ctx.Err() would put an atomic load in
	// the hot loop; a stride keeps the loop clean while still abandoning the
	// pass promptly when the app shuts down.
	levelStatsCancelStride = 4096
	// levelStatsMaxAttempts bounds how often a counting pass restarts because
	// the file rotated underneath it. A rebuild invalidates every line index the
	// pass had counted, so the partial counts must be dropped and the pass
	// restarted rather than retried in place.
	levelStatsMaxAttempts = 8
)

// GetLevelStats counts log-level markers (E: ERROR/FATAL/PANIC, W: WARN) in the
// active tab's file. Detection is a case-sensitive token check over the first
// levelScanWindow bytes of each line — the conventional placement of level tags.
//
// The count is cached per tab and extended incrementally (M4): the UI refreshes
// level stats on every tail update, and re-counting the whole file each time made
// a tailed multi-GB log re-read gigabytes per refresh while the caller awaited it.
// Only lines appended since the previous call are scanned; a rotation (detected
// via the indexer generation) drops the cache and recounts. Concurrent calls for
// one tab are serialized, so a burst of refreshes waits for the in-flight pass and
// then reads its result instead of starting a second full scan. The pass also
// honors the app context, so a shutdown does not leave a full-file scan running.
//
// OPTIMIZATION: Now uses indexer.GetLevelCounts() which provides pre-computed
// counts at regular intervals (every 1000 lines) during index build. Only the
// tail region after the last snapshot needs a fresh ScanLines pass.
func (a *App) GetLevelStats(tabID string) (map[string]int64, error) {
	a.mu.RLock()
	tab, exists := a.tabs[tabID]
	a.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("%w: %s", ErrTabNotFound, tabID)
	}

	ctx := a.ctx
	if ctx == nil {
		// Not started through the Wails lifecycle (tests, headless use).
		ctx = context.Background()
	}

	// A dedicated mutex, not tab.mu: the counting pass must not block renames,
	// closes or the tail poller's bookkeeping for its whole duration.
	tab.statsMu.Lock()
	defer tab.statsMu.Unlock()

	for range levelStatsMaxAttempts {
		total := tab.Indexer.TotalLines()
		gen := tab.Indexer.Generation()

		counted := tab.levels
		// A generation bump means the table was rebuilt, so every line the cache
		// counted now denotes different content; a total below what we already
		// counted means the same for a shrink we have not been told about yet.
		if counted.gen != gen || counted.scanned > total {
			counted = levelStats{gen: gen}
		}

		// Fast path: use pre-computed level counts from the indexer
		levelCounts := tab.Indexer.GetLevelCounts()
		if levelCounts != nil && len(levelCounts) > 0 {
			// Find the last snapshot that covers lines we've already counted
			var baseErrors, baseWarns int64
			var baseScanned int64

			for _, lc := range levelCounts {
				if lc.LineIndex <= counted.scanned {
					baseErrors = lc.Errors
					baseWarns = lc.Warns
					baseScanned = lc.LineIndex
				} else {
					break
				}
			}

			// If the snapshot covers up to or past our total, we're done
			if baseScanned >= total {
				counted.errors = baseErrors
				counted.warns = baseWarns
				counted.scanned = total
				tab.levels = counted
				return map[string]int64{"E": counted.errors, "W": counted.warns}, nil
			}

			// Only scan from the last snapshot to the end
			errs, warns, scanned, err := countLevelMarkers(ctx, tab.Indexer, baseScanned, total-baseScanned)
			if errors.Is(err, indexer.ErrIndexRebuilt) {
				continue
			}
			if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
				return nil, err
			}
			if tab.Indexer.Generation() != gen {
				continue
			}

			counted.errors = baseErrors + errs
			counted.warns = baseWarns + warns
			counted.scanned = scanned
			tab.levels = counted
			return map[string]int64{"E": counted.errors, "W": counted.warns}, nil
		}

		// Fallback: no level counts available (old index), do full scan
		if counted.scanned == total {
			tab.levels = counted
			return map[string]int64{"E": counted.errors, "W": counted.warns}, nil
		}

		errs, warns, scanned, err := countLevelMarkers(ctx, tab.Indexer, counted.scanned, total-counted.scanned)
		if errors.Is(err, indexer.ErrIndexRebuilt) {
			continue
		}

		if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
			return nil, err
		}
		if tab.Indexer.Generation() != gen {
			continue
		}

		counted.errors += errs
		counted.warns += warns
		counted.scanned = scanned
		tab.levels = counted
		return map[string]int64{"E": counted.errors, "W": counted.warns}, nil
	}

	return nil, fmt.Errorf("level stats for tab %s: %w", tabID, indexer.ErrIndexRebuilt)
}

// countLevelMarkers counts level markers over the lines [start, start+count) and
// returns the counts together with the exclusive line index it got through, so a
// cancelled pass can be resumed from where it stopped. On a read failure it
// reports no progress: the caller must not fold half a span into a cached count
// it cannot verify.
func countLevelMarkers(ctx context.Context, idx *indexer.FileIndexer, start, count int64) (errs, warns, scannedTo int64, err error) {
	scannedTo = start
	seen := int64(0)
	cancelled := false

	// ScanLines, not ScanLinesRetry: a transparent restart would replay lines
	// that were already added to errs/warns and double-count them. The caller
	// restarts instead, from a zeroed base.
	if serr := idx.ScanLines(start, count, func(i int64, line []byte) bool {
		seen++
		if seen%levelStatsCancelStride == 0 && ctx.Err() != nil {
			cancelled = true
			return false
		}
		window := line
		if len(window) > levelScanWindow {
			window = window[:levelScanWindow]
		}
		switch {
		case bytes.Contains(window, levelTokenError),
			bytes.Contains(window, levelTokenFatal),
			bytes.Contains(window, levelTokenPanic):
			errs++
		case bytes.Contains(window, levelTokenWarn):
			warns++
		}
		scannedTo = i + 1
		return true
	}); serr != nil {
		return 0, 0, start, serr
	}

	if cancelled {
		return errs, warns, scannedTo, ctx.Err()
	}

	return errs, warns, scannedTo, nil
}
