package domain

import (
	"context"
	"time"
)

// WorkbenchRepository owns ordering, visibility and cancellation in the same
// transactions that accept messages and save answers. The file replay store
// deliberately does not implement the multiuser workbench.
type WorkbenchRepository interface {
	ListConversations(context.Context, string, string, int) ([]Conversation, error)
	RenameConversation(context.Context, string, string, int64) (Conversation, error)
	ConversationMessages(context.Context, string, int64, int64, int) ([]Response, error)
	DeleteConversation(context.Context, string) ([]string, error)
	CancelResponse(context.Context, string) (Response, error)
	InterruptResponses(context.Context) error
	CleanupConversations(context.Context, []string) error
	LatestSummary(context.Context, string) (ConversationSummary, error)
	SaveSummary(context.Context, ConversationSummary) error
}

type DraftEventRepository interface {
	StartResponseDraft(context.Context, string) (int64, error)
	AppendResponseDelta(context.Context, string, int64, string) (ResponseEvent, error)
	AppendToolProgress(context.Context, string, PythonExecution) (ResponseEvent, error)
}

type HistoryTurn struct {
	ResponseID string             `json:"response_id"`
	Sequence   int64              `json:"sequence"`
	Question   string             `json:"question"`
	Status     string             `json:"status"`
	Answer     string             `json:"answer,omitempty"`
	Citations  []Citation         `json:"citations,omitempty"`
	Gaps       []string           `json:"gaps,omitempty"`
	Sources    []SourceRef        `json:"sources,omitempty"`
	Executions []HistoryExecution `json:"executions,omitempty"`
}

type ConversationSummary struct {
	ConversationID string             `json:"conversation_id"`
	Version        int64              `json:"version"`
	From           int64              `json:"from_sequence"`
	Through        int64              `json:"through_sequence"`
	Text           string             `json:"text"`
	Citations      []Citation         `json:"citations"`
	Sources        []SourceRef        `json:"sources,omitempty"`
	Executions     []HistoryExecution `json:"executions,omitempty"`
	CreatedAt      time.Time          `json:"created_at"`
}

type ConversationHistory struct {
	Summary *ConversationSummary `json:"summary,omitempty"`
	Recent  []HistoryTurn        `json:"recent"`
}

type ContextSelection struct {
	From           int64    `json:"from_sequence"`
	Through        int64    `json:"through_sequence"`
	SummaryVersion int64    `json:"summary_version"`
	Bytes          int      `json:"bytes"`
	SourceIDs      []string `json:"source_ids"`
}
