package domain

import "time"

type ResponseEvent struct {
	ID              int64            `json:"id"`
	Type            string           `json:"type"`
	ResponseID      string           `json:"response_id"`
	Status          string           `json:"status"`
	DataMode        string           `json:"data_mode"`
	DraftVersion    int64            `json:"draft_version,omitempty"`
	Delta           string           `json:"delta,omitempty"`
	ReasoningStatus string           `json:"reasoning_status,omitempty"`
	Reason          *Failure         `json:"reason,omitempty"`
	CreatedAt       time.Time        `json:"created_at"`
	Response        *Response        `json:"response,omitempty"`
	Execution       *PythonExecution `json:"execution,omitempty"`
}

func ResponseTerminal(status string) bool {
	return status != "QUEUED" && status != "RUNNING" && status != "CANCELING"
}

// NewResponseEvents runs in the same transaction as the response write.
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
	case "CANCELING":
		add("progress", r.Status, nil)
	case "NEEDS_CLARIFICATION":
		add("clarification_required", r.Status, &r)
	case "FAILED":
		add("failed", r.Status, &r)
	case "CANCELED":
		add("canceled", r.Status, &r)
	case "INTERRUPTED":
		add("interrupted", r.Status, &r)
	default:
		add("answer", r.Status, &r)
	}
	return events
}

func NewDraftEvent(kind string, r Response, version, sequence int64, delta string, reason *Failure) ResponseEvent {
	return ResponseEvent{
		ID: sequence, Type: kind, ResponseID: r.ID, Status: r.Status, DataMode: r.DataMode,
		DraftVersion: version, Delta: delta, Reason: reason, CreatedAt: time.Now().UTC(),
	}
}

func NewToolProgressEvent(r Response, execution PythonExecution, sequence int64) ResponseEvent {
	return ResponseEvent{
		ID: sequence, Type: "tool_progress", ResponseID: r.ID, Status: r.Status,
		DataMode: r.DataMode, CreatedAt: time.Now().UTC(), Execution: &execution,
	}
}

func ActiveDraft(events []ResponseEvent) int64 {
	var active int64
	for _, event := range events {
		switch event.Type {
		case "draft_started":
			active = event.DraftVersion
		case "draft_retracted":
			if event.DraftVersion == active {
				active = 0
			}
		case "answer", "failed", "canceled", "interrupted", "clarification_required":
			active = 0
		}
	}
	return active
}

func DraftRetractionReason(r Response) *Failure {
	if r.Status == "ANSWERED" || r.Status == "PARTIAL" || r.Status == "UNRESOLVED" ||
		r.Status == "NEEDS_CLARIFICATION" {
		return &Failure{Code: "FINALIZED", Message: "草稿已由复核后的最终结果替换。"}
	}
	if r.Status == "CANCELING" {
		return &Failure{Code: "CANCELED", Message: "本轮已停止，生成中草稿已撤回。"}
	}
	if r.Error != nil {
		return &Failure{Code: r.Error.Code, Message: r.Error.Message}
	}
	return &Failure{Code: r.Status, Message: "草稿已失效。"}
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
