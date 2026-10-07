package domain

import (
	"context"
	"time"
)

const MaxReasoningBytes = 512 * 1024

// ResponseReasoning is provider output for display only, never answer evidence
// or a history turn. EventID lets clients merge snapshots with SSE replay.
type ResponseReasoning struct {
	Text    string `json:"text"`
	Status  string `json:"status"`
	EventID int64  `json:"event_id"`
}

type ReasoningEventRepository interface {
	AppendResponseReasoning(context.Context, string, string) (ResponseEvent, error)
	FinishResponseReasoning(context.Context, string) error
}

func NewReasoningEvent(r Response, sequence int64, delta, status string) ResponseEvent {
	kind := "reasoning_delta"
	if delta == "" {
		kind = "reasoning_done"
	}
	return ResponseEvent{
		ID: sequence, Type: kind, ResponseID: r.ID, Status: r.Status,
		DataMode: r.DataMode, Delta: delta, ReasoningStatus: status, CreatedAt: time.Now().UTC(),
	}
}
