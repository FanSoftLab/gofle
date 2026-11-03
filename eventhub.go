package main

import (
	"encoding/json"
	"sync"
)

// EventHub broadcasts named events to all registered WebSocket subscribers.
// Used by the web UI mode; desktop (Wails) continues to use runtime.EventsEmit.
type EventHub struct {
	mu   sync.RWMutex
	subs map[chan []byte]struct{}
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
		}
	}
}
