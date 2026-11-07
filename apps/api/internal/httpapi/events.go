package httpapi

import (
	"fmt"
	"net/http"
	"time"

	"marketingops/api/internal/events"
)

// workspaceEventsHandler handles live Server-Sent Events (SSE) connections for a workspace.
func workspaceEventsHandler(hub *events.Hub) http.HandlerFunc {
	if hub == nil {
		hub = events.DefaultHub
	}

	return func(w http.ResponseWriter, r *http.Request) {
		workspaceID := r.PathValue("id")
		if workspaceID == "" {
			http.Error(w, "workspace id required", http.StatusBadRequest)
			return
		}

		flusher, ok := w.(http.Flusher)
		if !ok {
			if u, ok := w.(interface{ Unwrap() http.ResponseWriter }); ok {
				flusher, _ = u.Unwrap().(http.Flusher)
			}
		}
		if flusher == nil {
			http.Error(w, "streaming unsupported", http.StatusInternalServerError)
			return
		}

		// The server's WriteTimeout is sized for one-shot responses and would
		// cut this long-lived stream after about a minute and a half. Clear the
		// deadline for this connection only; errors just mean it is unsupported.
		_ = http.NewResponseController(w).SetWriteDeadline(time.Time{})

		// Set SSE headers
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache, no-transform")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("X-Accel-Buffering", "no") // Prevents proxy buffering in Caddy / Nginx

		// Initial connection handshake
		ch, unsubscribe := hub.Subscribe(workspaceID)
		defer unsubscribe()

		initEv := events.Event{
			WorkspaceID: workspaceID,
			Type:        "connected",
			Message:     "Real-time event stream connected",
			Timestamp:   time.Now().UTC().Format(time.RFC3339),
		}
		_, _ = w.Write(events.FormatSSE(initEv))
		flusher.Flush()

		// Keep-alive ticker every 15s to keep connections alive across NAT/proxies
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()

		ctx := r.Context()
		for {
			select {
			case <-ctx.Done():
				return
			case ev, open := <-ch:
				if !open {
					return
				}
				if _, err := w.Write(events.FormatSSE(ev)); err != nil {
					return
				}
				flusher.Flush()
			case <-ticker.C:
				if _, err := fmt.Fprintf(w, ": keep-alive\n\n"); err != nil {
					return
				}
				flusher.Flush()
			}
		}
	}
}
