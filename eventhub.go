package main

import (
	"encoding/json"
	"log/slog"
	"sync"
	"sync/atomic"
)

// EventHub broadcasts named events to all registered WebSocket subscribers.
// Used by the web UI mode; desktop (Wails) continues to use runtime.EventsEmit.
type EventHub struct {
	mu   sync.RWMutex
	subs map[chan []byte]struct{}
	// drops counts events that could not be delivered (W3): a subscriber's
	// 64-slot buffer was full, either at Broadcast time or when the WS
	// dispatch loop forwarded into a stalled client's outbox. The first drop
	// is logged once; the counter stays available for tests/diagnostics.
	drops    atomic.Int64
	dropOnce sync.Once
}

func NewEventHub() *EventHub {
	return &EventHub{subs: make(map[chan []byte]struct{})}
}

// Subscribe returns a buffered channel that receives JSON-encoded events.
// Caller must Unsubscribe when done.
func (h *EventHub) Subscribe() chan []byte {
	ch := make(chan []byte, 64)
	h.mu.Lock()
	h.subs[ch] = struct{}{}
	h.mu.Unlock()
	return ch
}

func (h *EventHub) Unsubscribe(ch chan []byte) {
	h.mu.Lock()
	if _, ok := h.subs[ch]; ok {
		delete(h.subs, ch)
		close(ch)
	}
	h.mu.Unlock()
}

// recordDrop counts one undeliverable event and logs the first one (W3):
// silent event loss made a slow WebSocket client indistinguishable from a
// frozen live view.
func (h *EventHub) recordDrop() {
	h.drops.Add(1)
	h.dropOnce.Do(func() {
		slog.Warn("event dropped: WebSocket client too slow to consume", "total_dropped", h.drops.Load())
	})
}

// Drops returns the number of events dropped for slow subscribers so far.
func (h *EventHub) Drops() int64 {
	return h.drops.Load()
}

// Broadcast sends an event of the given type to all subscribers.
func (h *EventHub) Broadcast(eventType string, data any) {
	// Skip the marshal entirely when nobody is listening (E1): desktop
	// (Wails) mode never registers a WebSocket subscriber, so without this
	// check every file_updated (per active tab per 500 ms poll tick), every
	// search_progress partial and every ssh:progress stage paid a map build
	// plus a json.Marshal for nothing.
	h.mu.RLock()
	n := len(h.subs)
	h.mu.RUnlock()
	if n == 0 {
		return
	}

	msg, err := json.Marshal(map[string]any{
		"type": eventType,
		"data": data,
	})
	if err != nil {
		return
	}

	h.mu.RLock()
	defer h.mu.RUnlock()
	for ch := range h.subs {
		select {
		case ch <- msg:
		default:
			h.recordDrop()
		}
	}
}
