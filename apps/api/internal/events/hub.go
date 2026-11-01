// Package events provides an in-memory pub-sub broker for Server-Sent Events (SSE).
package events

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

// Event represents a live event broadcast to subscribed clients in a workspace.
type Event struct {
	WorkspaceID string `json:"workspaceId"`
	Type        string `json:"type"` // e.g. "lead_update", "whatsapp_status", "delivery_update"
	Message     string `json:"message,omitempty"`
	Timestamp   string `json:"timestamp"`
}

// Hub manages client subscriptions and event fan-out per workspace.
type Hub struct {
	mu          sync.RWMutex
	subscribers map[string]map[chan Event]struct{}
}

// Global Hub instance
var DefaultHub = NewHub()

func NewHub() *Hub {
	return &Hub{
		subscribers: make(map[string]map[chan Event]struct{}),
	}
}

// Subscribe returns an event channel for a workspace and an unsubscribe cleanup function.
func (h *Hub) Subscribe(workspaceID string) (<-chan Event, func()) {
	h.mu.Lock()
	defer h.mu.Unlock()

	ch := make(chan Event, 16)
	if _, ok := h.subscribers[workspaceID]; !ok {
		h.subscribers[workspaceID] = make(map[chan Event]struct{})
	}
	h.subscribers[workspaceID][ch] = struct{}{}

	unsubscribe := func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		if subs, ok := h.subscribers[workspaceID]; ok {
			delete(subs, ch)
			close(ch)
			if len(subs) == 0 {
				delete(h.subscribers, workspaceID)
			}
		}
	}

	return ch, unsubscribe
}

// Publish broadcasts an event to all active clients subscribed to the workspace.
func (h *Hub) Publish(workspaceID, eventType, message string) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	subs, ok := h.subscribers[workspaceID]
	if !ok || len(subs) == 0 {
		return
	}

	ev := Event{
		WorkspaceID: workspaceID,
		Type:        eventType,
		Message:     message,
		Timestamp:   time.Now().UTC().Format(time.RFC3339),
	}

	for ch := range subs {
		select {
		case ch <- ev:
		default:
			// Buffer full, skip slow consumer to avoid blocking
		}
	}
}

// FormatSSE converts an Event to a standard SSE wire frame.
func FormatSSE(ev Event) []byte {
	b, _ := json.Marshal(ev)
	return []byte(fmt.Sprintf("event: message\ndata: %s\n\n", string(b)))
}
