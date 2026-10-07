package httpapi

import (
	"net/http"
	"strconv"
	"time"

	"hwops/internal/domain"
)

func (h *handler) listConversations(w http.ResponseWriter, r *http.Request) {
	limit, err := pageLimit(r)
	if err != nil {
		result(w, 0, nil, err)
		return
	}
	rows, err := h.app.Conversations(r.Context(), r.URL.Query().Get("q"), r.URL.Query().Get("after"), limit)
	next := ""
	if len(rows) == limit {
		last := rows[len(rows)-1]
		next = last.CreatedAt.Format(time.RFC3339Nano) + "|" + last.ID
	}
	result(w, http.StatusOK, map[string]any{"conversations": rows, "next": next}, err)
}

func pageLimit(r *http.Request) (int, error) {
	value := r.URL.Query().Get("limit")
	if value == "" {
		return 50, nil
	}
	limit, err := strconv.Atoi(value)
	if err != nil || limit < 1 || limit > 100 {
		return 0, domain.ErrInvalid
	}
	return limit, nil
}

func (h *handler) renameConversation(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Title        string `json:"title"`
		StateVersion int64  `json:"state_version"`
	}
	if !decode(w, r, &input) {
		return
	}
	c, err := h.app.RenameConversation(r.Context(), r.PathValue("id"), input.Title, input.StateVersion)
	result(w, http.StatusOK, c, err)
}

func (h *handler) deleteConversation(w http.ResponseWriter, r *http.Request) {
	err := h.app.DeleteConversation(r.Context(), r.PathValue("id"))
	result(w, http.StatusAccepted, map[string]string{"status": "DELETING"}, err)
}

func (h *handler) messages(w http.ResponseWriter, r *http.Request) {
	var after int64
	var err error
	if r.URL.Query().Has("after") {
		after, err = strconv.ParseInt(r.URL.Query().Get("after"), 10, 64)
	}
	if err != nil || after < 0 {
		result(w, 0, nil, domain.ErrInvalid)
		return
	}
	limit, err := pageLimit(r)
	if err != nil {
		result(w, 0, nil, err)
		return
	}
	rows, err := h.app.Messages(r.Context(), r.PathValue("id"), after, limit)
	var next int64
	if len(rows) == limit {
		next = rows[len(rows)-1].Sequence
	}
	result(w, http.StatusOK, map[string]any{"messages": rows, "next": next}, err)
}

func (h *handler) cancelResponse(w http.ResponseWriter, r *http.Request) {
	response, err := h.app.CancelResponse(r.Context(), r.PathValue("id"))
	result(w, http.StatusAccepted, response, err)
}
