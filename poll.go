package main

import (
	"errors"
	"log/slog"
	"os"
	"runtime/debug"
	"sync"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// pollFileUpdates is invoked by the 500 ms ticker. For each tab it appends newly
// written lines to the index, re-runs the active search over just the new range,
// and emits a file_updated runtime event so the UI can refresh.
//
//nolint:gocyclo,nestif,funlen // per-tab poll loop with rotation/append branches is inherently long, branchy and nested
func (a *App) pollFileUpdates() {
	a.mu.RLock()

	tabsToPoll := make([]*TabState, 0, len(a.tabs))
	for _, tab := range a.tabs {
		tabsToPoll = append(tabsToPoll, tab)
	}

	a.mu.RUnlock()

	// Process tabs in parallel, limited by semaphore to avoid over-contention.
	const maxConcurrent = 8

	sem := make(chan struct{}, maxConcurrent)

	var wg sync.WaitGroup

	for _, tab := range tabsToPoll {
		wg.Add(1)

		sem <- struct{}{} // acquire semaphore

		go func(t *TabState) {
			defer wg.Done()
			defer func() { <-sem }() // release semaphore
			// A panic while polling one tab (file reads, SSH-streamed bytes,
			// regex over arbitrary log content) must not take down the whole
			// GUI. Declared after wg.Done/sem so bookkeeping still runs.
			defer func() {
				if r := recover(); r != nil {
					slog.Error("panic while polling tab", "tab_id", t.ID, "panic", r, "stack", string(debug.Stack()))
				}
			}()

			// Snapshot the bits that need tab.mu, then release it: the (often
			// expensive) index update, search and event emit run WITHOUT the lock
			// so the UI thread (GetFilteredLines/RenameTab/close) is never
			// starved by a big search (B3). The indexer and search engine have
			// their own mutexes.
			t.mu.Lock()
			// Adaptive backoff (O3): an idle tab is skipped for several ticks so
			// the 500 ms ticker doesn't re-stat files that never change, while an
			// active tab keeps being polled every tick.
			if t.pollSkip > 0 {
				t.pollSkip--
				t.mu.Unlock()

				return
			}

			opts := t.LastSearchOpts
			gen := t.searchGen
			t.mu.Unlock()

			oldTotal := t.Indexer.TotalLines()

			added, err := t.Indexer.UpdateIndex()
			if err != nil {
				if errors.Is(err, os.ErrClosed) {
					return // tab closed mid-poll — normal teardown, nothing to report
				}

				t.mu.Lock()
				t.pollFailures++
				failures := t.pollFailures
				t.mu.Unlock()

				if failures == 1 || failures%120 == 0 {
					slog.Warn("failed to update index for tab",
						"tab_id", t.ID, "path", t.FilePath, "consecutive_failures", failures, "err", err)
				}

				if failures == 1 {
					runtime.EventsEmit(a.ctx, "file_error", map[string]any{
						"tab_id": t.ID,
						"error":  err.Error(),
					})
				}

				return
			}

			t.mu.Lock()
			t.pollFailures = 0
			t.mu.Unlock()

			// Rotation/truncation rebuilt the offset table: the previously stored
			// MatchedLines now point at invalid line indices, so re-run the full
			// search and REPLACE the set rather than appending (O6).
			if t.Indexer.TakeRebuiltFlag() {
				newTotal := t.Indexer.TotalLines()
				matchCount := 0

				if opts.Query != "" {
					matches, searchErr := t.Searcher.SearchRange(a.ctx, opts, 0, newTotal)
					if searchErr != nil {
						// The lines themselves still changed — refresh the UI
						// with a zero count rather than skipping the emit and
						// leaving the view stale; the failure is logged here.
						slog.Warn("tail re-search after rotation failed", "tab_id", t.ID, "err", searchErr)
					} else {
						t.mu.Lock()
						t.MatchedLines = matches
						matchCount = len(t.MatchedLines)
						t.mu.Unlock()
					}
				} else {
					t.mu.Lock()
					t.MatchedLines = make([]int64, 0)
					t.mu.Unlock()
				}

				runtime.EventsEmit(a.ctx, "file_updated", map[string]any{
					"tab_id":      t.ID,
					"total_lines": newTotal,
					"file_size":   t.Indexer.FileSize(),
					"added_lines": added,
					"match_count": matchCount,
				})
				t.mu.Lock()
				t.pollSkip = 0
				t.mu.Unlock()

				return
			}

			if added > 0 {
				newTotal := t.Indexer.TotalLines()
				matchCount := 0

				if opts.Query != "" {
					newMatches, searchErr := t.Searcher.SearchRange(a.ctx, opts, oldTotal, newTotal)
					if searchErr != nil {
						// Emit with the last known count (the appended lines are
						// real); the failed extension is logged, not swallowed.
						slog.Warn("tail search over appended lines failed", "tab_id", t.ID, "err", searchErr)
						t.mu.Lock()
						matchCount = len(t.MatchedLines)
						t.mu.Unlock()
					} else {
						t.mu.Lock()
						// B6: a newer UI search may have replaced MatchedLines
						// while SearchRange ran; only append if our snapshot is
						// still current, otherwise the range is already present.
						if t.searchGen == gen {
							t.MatchedLines = append(t.MatchedLines, newMatches...)
						}

						matchCount = len(t.MatchedLines)
						t.mu.Unlock()
					}
				} else {
					t.mu.Lock()
					matchCount = len(t.MatchedLines)
					t.mu.Unlock()
				}

				runtime.EventsEmit(a.ctx, "file_updated", map[string]any{
					"tab_id":      t.ID,
					"total_lines": t.Indexer.TotalLines(),
					"file_size":   t.Indexer.FileSize(),
					"added_lines": added,
					"match_count": matchCount,
				})
				// Active tab: reset backoff so it stays at the 500 ms cadence.
				t.mu.Lock()
				t.pollSkip = 0
				t.mu.Unlock()
			} else {
				// No new data this tick: back off. 9 skipped ticks at 500 ms ≈ 5 s.
				t.mu.Lock()
				t.pollSkip = 9
				t.mu.Unlock()
			}
		}(tab)
	}

	wg.Wait()
}
