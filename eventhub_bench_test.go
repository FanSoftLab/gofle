package main

import (
	"encoding/json"
	"testing"
)

// BenchmarkBroadcastNoSubs pins the E1 fast path: in desktop (Wails) mode the
// hub never has a WebSocket subscriber, so Broadcast must return before the
// map build and json.Marshal. The old shape marshaled unconditionally.
func BenchmarkBroadcastNoSubs(b *testing.B) {
	h := NewEventHub()

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		h.Broadcast("file_updated", map[string]any{
			"tab_id":      "tab_1",
			"total_lines": int64(123456),
			"file_size":   int64(98765432),
			"added_lines": 7,
			"match_count": int64(42),
		})
	}
}

// BenchmarkBroadcastOneSub measures the real fan-out path for comparison: the
// marshal must still happen exactly once per event, not per subscriber.
func BenchmarkBroadcastOneSub(b *testing.B) {
	h := NewEventHub()
	ch := h.Subscribe()

	defer h.Unsubscribe(ch)

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		h.Broadcast("file_updated", map[string]any{
			"tab_id":      "tab_1",
			"total_lines": int64(123456),
			"file_size":   int64(98765432),
			"added_lines": 7,
			"match_count": int64(42),
		})
	}
}

// BenchmarkBroadcastNoSubsLegacy is the pre-E1 shape: marshal unconditionally,
// then discover there are no subscribers. Kept as the documented baseline.
func BenchmarkBroadcastNoSubsLegacy(b *testing.B) {
	h := NewEventHub()

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		// The old Broadcast body, verbatim.
		msg, err := json.Marshal(map[string]any{
			"type": "file_updated",
			"data": map[string]any{
				"tab_id":      "tab_1",
				"total_lines": int64(123456),
				"file_size":   int64(98765432),
				"added_lines": 7,
				"match_count": int64(42),
			},
		})
		if err != nil {
			b.Fatal(err)
		}

		h.mu.RLock()

		for ch := range h.subs {
			select {
			case ch <- msg:
			default:
			}
		}
		h.mu.RUnlock()
	}
}
