package domain

import (
	"context"
	"errors"
	"time"
)

var (
	ErrNotFound = errors.New("not found")
	ErrInvalid  = errors.New("invalid input")
	ErrConflict = errors.New("conflict")
)

type Applicability struct {
	Scope            string        `json:"scope"`
	Model            string        `json:"model,omitempty"`
	Firmware         string        `json:"firmware,omitempty"`
	Driver           string        `json:"driver,omitempty"`
	HardwareRevision string        `json:"hardware_revision,omitempty"`
	PolicyID         string        `json:"policy_id,omitempty"`
	FirmwareRange    *VersionRange `json:"firmware_range,omitempty"`
	DriverRange      *VersionRange `json:"driver_range,omitempty"`
}

type VersionRange struct {
	Min string `json:"min"`
	Max string `json:"max"`
}

type VersionPolicyInput struct {
	Model         string   `json:"model"`
	Source        string   `json:"source"`
	FirmwareOrder []string `json:"firmware_order,omitempty"`
	DriverOrder   []string `json:"driver_order,omitempty"`
}

type VersionPolicy struct {
	VersionPolicyInput
	SchemaVersion int       `json:"schema_version"`
	ID            string    `json:"id"`
	CreatedAt     time.Time `json:"created_at"`
}

type DeviceInput struct {
	Name             string    `json:"name"`
	Aliases          []string  `json:"aliases,omitempty"`
	Model            string    `json:"model"`
	Firmware         string    `json:"firmware,omitempty"`
	Driver           string    `json:"driver,omitempty"`
	HardwareRevision string    `json:"hardware_revision,omitempty"`
	Source           string    `json:"source"`
	ObservedAt       time.Time `json:"observed_at"`
	DataMode         string    `json:"data_mode"`
	MonitoringID     string    `json:"monitoring_id,omitempty"`
}

type DeviceContext struct {
	DeviceInput
	SchemaVersion int    `json:"schema_version"`
	DeviceID      string `json:"device_id"`
	SnapshotID    string `json:"snapshot_id"`
}

type MessageInput struct {
	Text            string `json:"text"`
	DeviceID        string `json:"device_id,omitempty"`
	DeviceQuery     string `json:"device_query,omitempty"`
	ContextRevision string `json:"context_revision,omitempty"`
}

type DeviceResolution struct {
	Status     string          `json:"status"`
	Candidates []DeviceContext `json:"candidates"`
}

type FieldCheck struct {
	Field  string `json:"field"`
	Status string `json:"status"`
	Reason string `json:"reason"`
}

type Assessment struct {
	Status string       `json:"status"`
	Checks []FieldCheck `json:"checks"`
}

type KnowledgeCheck struct {
	RevisionID string `json:"revision_id"`
	Assessment
}

type RetrievalRepresentation struct {
	Kind      string `json:"kind"`
	Text      string `json:"text"`
	StartLine int    `json:"start_line,omitempty"`
	EndLine   int    `json:"end_line,omitempty"`
}

type Fragment struct {
	ID              string                    `json:"id"`
	RevisionID      string                    `json:"revision_id"`
	Section         string                    `json:"section"`
	StartLine       int                       `json:"start_line"`
	EndLine         int                       `json:"end_line"`
	Content         string                    `json:"content"`
	ContentHash     string                    `json:"content_hash"`
	Representations []RetrievalRepresentation `json:"representations,omitempty"`
}

type FragmentInput struct {
	Section         string                    `json:"section"`
	StartLine       int                       `json:"start_line"`
	EndLine         int                       `json:"end_line"`
	Content         string                    `json:"content"`
	Representations []RetrievalRepresentation `json:"representations"`
}

type Revision struct {
	SchemaVersion int           `json:"schema_version"`
	ID            string        `json:"id"`
	DocumentID    string        `json:"document_id"`
	Title         string        `json:"title"`
	Source        string        `json:"source"`
	Content       string        `json:"content"`
	Applicability Applicability `json:"applicability"`
	Status        string        `json:"status"`
	Fragments     []Fragment    `json:"fragments"`
	CreatedAt     time.Time     `json:"created_at"`
}

type RevisionInput struct {
	Title         string          `json:"title"`
	Source        string          `json:"source"`
	Content       string          `json:"content"`
	Applicability Applicability   `json:"applicability"`
	Fragments     []FragmentInput `json:"fragments,omitempty"`
}

type Conversation struct {
	SchemaVersion   int       `json:"schema_version"`
	ID              string    `json:"id"`
	Owner           string    `json:"owner"`
	CreatedAt       time.Time `json:"created_at"`
	DeviceID        string    `json:"device_id,omitempty"`
	ContextRevision string    `json:"context_revision,omitempty"`
	ContextVersion  int64     `json:"context_version,omitempty"`
}

type Claim struct {
	Text        string   `json:"text"`
	FragmentIDs []string `json:"fragment_ids"`
}

type Draft struct {
	Claims       []Claim                `json:"claims"`
	Gaps         []string               `json:"gaps,omitempty"`
	Conflicts    []KnowledgeConflict    `json:"conflicts,omitempty"`
	Observations []ObservationSelection `json:"observations,omitempty"`
}

type KnowledgeConflict struct {
	Subject     string   `json:"subject"`
	FragmentIDs []string `json:"fragment_ids"`
}

type Citation struct {
	DocumentID       string     `json:"document_id"`
	RevisionID       string     `json:"revision_id"`
	FragmentID       string     `json:"fragment_id"`
	Title            string     `json:"title"`
	Source           string     `json:"source"`
	Section          string     `json:"section"`
	DocumentContext  string     `json:"document_context,omitempty"`
	ContentHash      string     `json:"content_hash"`
	URL              string     `json:"url"`
	DeviceSnapshotID string     `json:"device_snapshot_id,omitempty"`
	Applicability    Assessment `json:"applicability"`
}

type Failure struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type ModelUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
	Calls            int `json:"calls"`
}

type EvidenceSelection struct {
	CandidateFragmentIDs []string `json:"candidate_fragment_ids"`
	ExpandedFragmentIDs  []string `json:"expanded_fragment_ids,omitempty"`
	InputBytes           int      `json:"input_bytes"`
	ContextBytes         int      `json:"context_bytes"`
}

type KnowledgeToolCall struct {
	ID                string             `json:"id"`
	Name              string             `json:"name"`
	Request           string             `json:"request"`
	Status            string             `json:"status"`
	FragmentIDs       []string           `json:"fragment_ids,omitempty"`
	RetrievalQueries  []string           `json:"retrieval_queries,omitempty"`
	CorpusGeneration  string             `json:"corpus_generation,omitempty"`
	IndexGenerations  []string           `json:"index_generations,omitempty"`
	GapReason         string             `json:"gap_reason,omitempty"`
	Gaps              []string           `json:"gaps,omitempty"`
	TraceID           string             `json:"trace_id,omitempty"`
	EvidenceSelection *EvidenceSelection `json:"evidence_selection,omitempty"`
	ModelUsage        *ModelUsage        `json:"model_usage,omitempty"`
	Error             *Failure           `json:"error,omitempty"`
}

type Response struct {
	SchemaVersion        int                    `json:"schema_version"`
	ID                   string                 `json:"id"`
	ConversationID       string                 `json:"conversation_id"`
	Question             string                 `json:"question"`
	Status               string                 `json:"status"`
	DataMode             string                 `json:"data_mode"`
	Answer               string                 `json:"answer"`
	Claims               []Claim                `json:"claims"`
	Citations            []Citation             `json:"citations"`
	Gaps                 []string               `json:"gaps"`
	Error                *Failure               `json:"error,omitempty"`
	CreatedAt            time.Time              `json:"created_at"`
	DeviceContext        *DeviceContext         `json:"device_context,omitempty"`
	DeviceResolution     *DeviceResolution      `json:"device_resolution,omitempty"`
	ContextRevision      string                 `json:"context_revision,omitempty"`
	ApplicabilityChecks  []KnowledgeCheck       `json:"applicability_checks,omitempty"`
	TraceID              string                 `json:"trace_id,omitempty"`
	RetrievedFragmentIDs []string               `json:"retrieved_fragment_ids,omitempty"`
	ModelUsage           *ModelUsage            `json:"model_usage,omitempty"`
	EvidenceSelection    *EvidenceSelection     `json:"evidence_selection,omitempty"`
	KnowledgeToolCalls   []KnowledgeToolCall    `json:"knowledge_tool_calls,omitempty"`
	Conflicts            []KnowledgeConflict    `json:"conflicts,omitempty"`
	Evidence             []Evidence             `json:"evidence,omitempty"`
	Observations         []ObservationSelection `json:"observations,omitempty"`
	RequestKey           string                 `json:"request_key,omitempty"`
	RequestHash          string                 `json:"request_hash,omitempty"`
}

type Repository interface {
	CreateVersionPolicy(context.Context, VersionPolicy) error
	GetVersionPolicy(context.Context, string) (VersionPolicy, error)
	PutDevice(context.Context, DeviceContext) error
	GetDevice(context.Context, string) (DeviceContext, error)
	ListDevices(context.Context) ([]DeviceContext, error)
	CreateRevision(context.Context, Revision) error
	GetRevision(context.Context, string) (Revision, error)
	SetPublication(context.Context, string, string) (Revision, error)
	ListPublished(context.Context) ([]Revision, error)
	CreateConversation(context.Context, Conversation) error
	GetConversation(context.Context, string) (Conversation, error)
	FindResponseRequest(context.Context, string, string, string) (Response, error)
	CreateResponse(context.Context, Response, int64) (Response, error)
	SaveResponse(context.Context, Response) error
	GetResponse(context.Context, string) (Response, error)
	PendingResponses(context.Context) ([]Response, error)
	ResponseEvents(context.Context, string, int64) ([]ResponseEvent, error)
}
