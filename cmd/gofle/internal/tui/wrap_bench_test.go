package tui

import (
	"testing"
)

// BenchmarkWrapGeometry pins the T3 wrap-count cache against the legacy
// per-keypress cost. In wrap mode clampViewport walks back through
// remainingVisualRows, and both call wrapRows — which used to issue one
// GetLine pread + string per visible line with no memoization, O(height²)
// preads per keypress.
//
//   - cold-cache clears the memo every iteration: this approximates the
//     pre-T3 cost of every keypress (nothing was cached).
//   - warm-cache keeps the memo: steady-state scrolling over the same
//     screenfuls.
//
// top varies per iteration so no other memoization can hide the work.
func BenchmarkWrapGeometry(b *testing.B) {
	m := buildBenchModel(b, 200_000, 200, 120)
	m.wordWrap = true
	m.onlyMatches = false
	m.matches = nil
	m.matchCursor = 0

	b.Run("cold-cache", func(b *testing.B) {
		b.ReportAllocs()

		for b.Loop() {
			wrapCacheMu.Lock()
			wrapCacheSet = false // force the full recompute every iteration
			wrapCacheMu.Unlock()

			m.top = int64(b.N % 2000)
			m.clampViewport()
		}
	})

	b.Run("warm-cache", func(b *testing.B) {
		b.ReportAllocs()

		for b.Loop() {
			m.top = int64(b.N % 2000)
			m.clampViewport()
		}
	})
}
