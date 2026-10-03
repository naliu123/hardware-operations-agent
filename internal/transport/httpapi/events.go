package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"hwops/internal/domain"
)

func (h *handler) events(w http.ResponseWriter, r *http.Request) {
	var after int64
	var err error
	if value := r.Header.Get("Last-Event-ID"); value != "" {
		after, err = strconv.ParseInt(value, 10, 64)
		if err != nil || after < 0 {
			result(w, 0, nil, fmt.Errorf("%w: Last-Event-ID must be a nonnegative sequence", domain.ErrInvalid))
			return
		}
	}
	events, err := h.app.ResponseEvents(r.Context(), r.PathValue("id"), 0)
	if err != nil {
		result(w, 0, nil, err)
		return
	}
	last := int64(0)
	if len(events) > 0 {
		last = events[len(events)-1].ID
	}
	if after > last {
		result(w, 0, nil, fmt.Errorf("%w: event sequence is ahead of this response", domain.ErrConflict))
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	controller := http.NewResponseController(w)
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	for {
		for _, event := range events {
			if event.ID <= after {
				continue
			}
			raw, err := json.Marshal(event)
			if err != nil {
				return
			}
			_ = controller.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if _, err := fmt.Fprintf(w, "id: %d\nevent: %s\ndata: %s\n\n", event.ID, event.Type, raw); err != nil {
				return
			}
			if err := controller.Flush(); err != nil {
				return
			}
			after = event.ID
		}
		saved, err := h.app.Response(r.Context(), r.PathValue("id"))
		if err != nil {
			return
		}
		if domain.ResponseTerminal(saved.Status) {
			// A final save can race the previous events read. Read once more
			// before closing so the persisted terminal event cannot be missed.
			events, err = h.app.ResponseEvents(r.Context(), saved.ID, after)
			if err != nil || len(events) == 0 {
				return
			}
			continue
		}
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
		}
		events, err = h.app.ResponseEvents(r.Context(), saved.ID, after)
		if err != nil {
			return
		}
		// Heartbeats also renew the server's finite write deadline.
		if len(events) == 0 {
			_ = controller.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if _, err = fmt.Fprint(w, ": waiting\n\n"); err != nil || controller.Flush() != nil {
				return
			}
		}
	}
}
