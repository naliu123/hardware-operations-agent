package domain

import (
	"context"
	"time"
)

// DiagnosticRule is reviewed and published with its immutable source revision.
// All conditions must hold. Only these rules can confirm a cause or recovery.
type DiagnosticRule struct {
	ID         string            `json:"id"`
	Kind       string            `json:"kind"` // ROOT_CAUSE or RECOVERY
	ErrorCode  string            `json:"error_code"`
	Conclusion string            `json:"conclusion"`
	StartLine  int               `json:"start_line"`
	EndLine    int               `json:"end_line"`
	Conditions []MetricCondition `json:"conditions"`
}

type MetricCondition struct {
	Capability    string            `json:"capability"`
	Component     string            `json:"component"`
	Parameters    map[string]string `json:"parameters,omitempty"`
	Field         string            `json:"field"`
	Operator      string            `json:"operator"` // EQ, GT, GE, LT, LE
	Value         string            `json:"value"`
	Unit          string            `json:"unit"`
	MaxAgeSeconds int               `json:"max_age_seconds"`
}

type RuleRef struct {
	RevisionID string `json:"revision_id"`
	RuleID     string `json:"rule_id"`
}

type IncidentInput struct {
	DeviceID    string    `json:"device_id"`
	ErrorCode   string    `json:"error_code"`
	Description string    `json:"description"`
	OccurredAt  time.Time `json:"occurred_at"`
}

type Incident struct {
	IncidentInput
	SchemaVersion int       `json:"schema_version"`
	ID            string    `json:"id"`
	DataMode      string    `json:"data_mode"`
	CreatedAt     time.Time `json:"created_at"`
	RequestKey    string    `json:"request_key,omitempty"`
	RequestHash   string    `json:"request_hash,omitempty"`
}

type Hypothesis struct {
	ID          string   `json:"id"`
	Description string   `json:"description"`
	Status      string   `json:"status"` // CANDIDATE, SUPPORTED, EXCLUDED, CONFIRMED
	EvidenceIDs []string `json:"evidence_ids"`
	Rule        *RuleRef `json:"rule,omitempty"`
	Reason      string   `json:"reason"`
}

type EvidenceCondition struct {
	EvidenceID string          `json:"evidence_id"`
	Condition  MetricCondition `json:"condition"`
}

type DiagnosticStep struct {
	ID                  string              `json:"id"`
	Kind                string              `json:"kind"`       // KNOWLEDGE or OBSERVE
	TargetRef           string              `json:"target_ref"` // trusted snapshot ID
	HypothesisRefs      []string            `json:"hypothesis_refs"`
	DependsOn           []string            `json:"depends_on"`
	Preconditions       []EvidenceCondition `json:"preconditions"`
	Query               string              `json:"query,omitempty"`
	Observation         *ObservationRequest `json:"observation,omitempty"`
	KnowledgeRefs       []string            `json:"knowledge_refs"`
	Purpose             string              `json:"purpose"`
	ExpectedObservation string              `json:"expected_observation"`
	RetryReason         string              `json:"retry_reason,omitempty"`
}

type DiagnosticResult struct {
	Summary         string   `json:"summary"`
	RootCauseStatus string   `json:"root_cause_status"` // UNKNOWN, SUPPORTED, CONFIRMED
	RecoveryStatus  string   `json:"recovery_status"`   // UNKNOWN, RECOVERED
	RecoveryRule    *RuleRef `json:"recovery_rule,omitempty"`
	EvidenceIDs     []string `json:"evidence_ids"`
}

// A proposal is a complete new plan view. Execution records are never model input
// fields to update; immutable history remains independent from step intent.
type PlanProposal struct {
	BaseStateVersion int64             `json:"base_state_version"`
	BasePlanVersion  int               `json:"base_plan_version"`
	Decision         string            `json:"decision"`
	Reason           string            `json:"reason"`
	SupportingRefs   []string          `json:"supporting_refs"`
	Hypotheses       []Hypothesis      `json:"hypotheses"`
	Steps            []DiagnosticStep  `json:"steps"`
	WaitReasons      []string          `json:"wait_reasons"`
	Gaps             []string          `json:"gaps"`
	Result           *DiagnosticResult `json:"result,omitempty"`
}

type PlanRevision struct {
	SchemaVersion int          `json:"schema_version"`
	Version       int          `json:"version"`
	Proposal      PlanProposal `json:"proposal"`
	CommittedAt   time.Time    `json:"committed_at"`
}

type DiagnosticKnowledge struct {
	Citation Citation         `json:"citation"`
	Content  string           `json:"content"`
	Rules    []DiagnosticRule `json:"rules,omitempty"`
}

type StepExecution struct {
	ID            string             `json:"id"`
	Step          DiagnosticStep     `json:"step"`
	PlanVersion   int                `json:"plan_version"`
	Status        string             `json:"status"` // RUNNING, SUCCEEDED, FAILED, REUSED, INTERRUPTED
	EvidenceIDs   []string           `json:"evidence_ids"`
	KnowledgeCall *KnowledgeToolCall `json:"knowledge_call,omitempty"`
	ReusedFrom    string             `json:"reused_from,omitempty"`
	Error         *Failure           `json:"error,omitempty"`
	StartedAt     time.Time          `json:"started_at"`
	FinishedAt    *time.Time         `json:"finished_at,omitempty"`
}

type RunBudget struct {
	ActiveLimitMS int64 `json:"active_limit_ms"`
	ActiveUsedMS  int64 `json:"active_used_ms"`
	ToolLimit     int   `json:"tool_limit"`
	ToolCalls     int   `json:"tool_calls"`
	ModelLimit    int   `json:"model_limit"`
	ModelCalls    int   `json:"model_calls"`
}

func DefaultRunBudget() RunBudget {
	return RunBudget{ActiveLimitMS: 300000, ToolLimit: 30, ModelLimit: 20}
}

type RunEvent struct {
	Sequence  int64     `json:"sequence"`
	Kind      string    `json:"kind"`
	Reason    string    `json:"reason"`
	CreatedAt time.Time `json:"created_at"`
}

type DiagnosticRun struct {
	SchemaVersion int                   `json:"schema_version"`
	ID            string                `json:"id"`
	IncidentID    string                `json:"incident_id"`
	DataMode      string                `json:"data_mode"`
	StateVersion  int64                 `json:"state_version"`
	Phase         string                `json:"phase"`
	Status        string                `json:"status"`
	Device        DeviceContext         `json:"device_context"`
	PlanVersion   int                   `json:"plan_version"`
	Plans         []PlanRevision        `json:"plans"`
	Hypotheses    []Hypothesis          `json:"hypotheses"`
	Evidence      []Evidence            `json:"evidence"`
	Knowledge     []DiagnosticKnowledge `json:"knowledge"`
	Executions    []StepExecution       `json:"executions"`
	Events        []RunEvent            `json:"events"`
	Budget        RunBudget             `json:"budget"`
	ActiveSince   *time.Time            `json:"active_since,omitempty"`
	WaitReasons   []string              `json:"wait_reasons"`
	Gaps          []string              `json:"gaps"`
	Result        *DiagnosticResult     `json:"result,omitempty"`
	Error         *Failure              `json:"error,omitempty"`
	CreatedAt     time.Time             `json:"created_at"`
	UpdatedAt     time.Time             `json:"updated_at"`
}

func (r DiagnosticRun) Terminal() bool { return r.Status == "COMPLETED" || r.Status == "CANCELED" }
func (r DiagnosticRun) Pending() bool  { return r.Status == "QUEUED" || r.Status == "RUNNING" }

func (r *DiagnosticRun) AddEvent(kind, reason string) {
	r.UpdatedAt = time.Now().UTC()
	r.Events = append(r.Events, RunEvent{Sequence: int64(len(r.Events) + 1), Kind: kind, Reason: reason, CreatedAt: r.UpdatedAt})
}

type RunResumeInput struct {
	StateVersion int64  `json:"state_version"`
	Reason       string `json:"reason"`
}

type DiagnosisRepository interface {
	CreateIncident(context.Context, Incident) (Incident, error)
	GetIncident(context.Context, string) (Incident, error)
	CreateRun(context.Context, DiagnosticRun) (DiagnosticRun, error)
	GetRun(context.Context, string) (DiagnosticRun, error)
	// CommitRun atomically checks state and optionally the current device snapshot.
	// Plans, evidence, budget reservations and events are one durable aggregate.
	CommitRun(context.Context, DiagnosticRun, int64, string, ...string) error
	PendingRuns(context.Context) ([]DiagnosticRun, error)
}
