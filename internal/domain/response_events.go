package domain

import "time"

type ResponseEvent struct {
	ID         int64     `json:"id"`
	Type       string    `json:"type"`
	ResponseID string    `json:"response_id"`
	Status     string    `json:"status"`
	DataMode   string    `json:"data_mode"`
	CreatedAt  time.Time `json:"created_at"`
	Response   *Response `json:"response,omitempty"`
}

func ResponseTerminal(status string) bool {
	return status != "QUEUED" && status != "RUNNING"
}

// NewResponseEvents runs in the same transaction as the response write. Only
// validated terminal answers are included; no raw model output is streamed.
func NewResponseEvents(previous string, r Response, sequence int64) []ResponseEvent {
	var events []ResponseEvent
	add := func(kind, status string, response *Response) {
		sequence++
		events = append(events, ResponseEvent{ID: sequence, Type: kind, ResponseID: r.ID,
			Status: status, DataMode: r.DataMode, CreatedAt: time.Now().UTC(), Response: response})
	}
	if sequence == 0 {
		add("accepted", "QUEUED", nil)
	}
	if previous == r.Status || r.Status == "QUEUED" {
		return events
	}
	switch r.Status {
	case "RUNNING":
		add("progress", r.Status, nil)
	case "NEEDS_CLARIFICATION":
		add("clarification_required", r.Status, &r)
	case "FAILED":
		add("failed", r.Status, &r)
	default:
		add("answer", r.Status, &r)
	}
	return events
}

func (c *Conversation) ApplyResponseContext(r Response) {
	if r.Status == "NEEDS_CLARIFICATION" {
		c.DeviceID, c.ContextRevision = "", ""
		c.ContextVersion++
	} else if r.DeviceContext != nil {
		c.DeviceID, c.ContextRevision = r.DeviceContext.DeviceID, r.ContextRevision
		c.ContextVersion++
	}
}
