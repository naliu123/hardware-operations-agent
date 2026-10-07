package domain

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sort"
	"time"
)

// Python records are separate from diagnostic device executions. Identity,
// budget and inputs are fixed before the first dispatch and never supplied by
// a model or browser.
type PythonBudget struct {
	Seconds       int   `json:"seconds"`
	CPUs          int   `json:"cpus"`
	MemoryBytes   int64 `json:"memory_bytes"`
	Processes     int   `json:"processes"`
	WorkBytes     int64 `json:"work_bytes"`
	LogBytes      int64 `json:"log_bytes"`
	ArtifactBytes int64 `json:"artifact_bytes"`
	ArtifactCount int   `json:"artifact_count"`
}

func PythonLimits() PythonBudget {
	return PythonBudget{60, 1, 1 << 30, 64, 256 << 20, 1 << 20, 100_000_000, 20}
}

type ExecutionInput struct {
	ID     string `json:"id"`
	Kind   string `json:"kind"`
	Name   string `json:"name"`
	SHA256 string `json:"sha256"`
	Bytes  int64  `json:"bytes"`
}

type Artifact struct {
	ID             string `json:"id"`
	ExecutionID    string `json:"execution_id"`
	ConversationID string `json:"conversation_id"`
	Name           string `json:"name"`
	SHA256         string `json:"sha256"`
	Bytes          int64  `json:"bytes"`
	MediaType      string `json:"media_type"`
	StorageKey     string `json:"-"`
}

type PythonResult struct {
	Stdout        string        `json:"stdout"`
	Stderr        string        `json:"stderr"`
	ExitCode      *int          `json:"exit_code,omitempty"`
	DurationMS    int64         `json:"duration_ms"`
	Complete      bool          `json:"complete"`
	Cleaned       bool          `json:"cleaned"`
	Error         string        `json:"error,omitempty"`
	ResourceUsage ResourceUsage `json:"resource_usage"`
	Artifacts     []Artifact    `json:"artifacts"`
	StartedAt     *time.Time    `json:"started_at,omitempty"`
	FinishedAt    *time.Time    `json:"finished_at,omitempty"`
}

type ResourceUsage struct {
	CPUUsec            int64 `json:"cpu_usec"`
	MemoryPeakBytes    int64 `json:"memory_peak_bytes"`
	ProcessesPeak      int64 `json:"processes_peak"`
	ProcessLimitEvents int64 `json:"process_limit_events"`
}

type PythonExecution struct {
	ID              string           `json:"id"`
	OwnerID         string           `json:"-"`
	ConversationID  string           `json:"conversation_id"`
	ResponseID      string           `json:"response_id"`
	Code            string           `json:"code"`
	CodeSHA256      string           `json:"code_sha256"`
	RequestSHA256   string           `json:"request_sha256"`
	Inputs          []ExecutionInput `json:"inputs"`
	Image           string           `json:"image"`
	Budget          PythonBudget     `json:"budget"`
	Status          string           `json:"status"`
	CancelReason    string           `json:"cancel_reason,omitempty"`
	Version         int64            `json:"state_version"`
	CreatedAt       time.Time        `json:"created_at"`
	Deadline        time.Time        `json:"deadline"`
	Result          PythonResult     `json:"result"`
	StdoutTruncated bool             `json:"stdout_truncated,omitempty"`
	StderrTruncated bool             `json:"stderr_truncated,omitempty"`
}

type PythonAnalysisResult struct {
	Status          string          `json:"status"`
	Execution       PythonExecution `json:"execution"`
	Source          *SourceRef      `json:"source,omitempty"`
	StdoutTruncated bool            `json:"stdout_truncated,omitempty"`
	StderrTruncated bool            `json:"stderr_truncated,omitempty"`
	Error           *Failure        `json:"error,omitempty"`
}

type HistoryExecution struct {
	ID         string           `json:"id"`
	Status     string           `json:"status"`
	CodeSHA256 string           `json:"code_sha256"`
	Inputs     []ExecutionInput `json:"inputs,omitempty"`
	Stdout     string           `json:"stdout,omitempty"`
	Stderr     string           `json:"stderr,omitempty"`
	Artifacts  []Artifact       `json:"artifacts,omitempty"`
}

func ExecutionSource(execution PythonExecution) SourceRef {
	inputHashes := make([]string, 0, len(execution.Inputs))
	for _, input := range execution.Inputs {
		inputHashes = append(inputHashes, input.SHA256)
	}
	sort.Strings(inputHashes)
	artifacts := make([]string, 0, len(execution.Result.Artifacts))
	for _, artifact := range execution.Result.Artifacts {
		artifacts = append(artifacts, artifact.ID+":"+artifact.SHA256)
	}
	sort.Strings(artifacts)
	raw, _ := json.Marshal(struct {
		ID        string   `json:"id"`
		Status    string   `json:"status"`
		Code      string   `json:"code_sha256"`
		Inputs    []string `json:"input_sha256"`
		Stdout    string   `json:"stdout"`
		Stderr    string   `json:"stderr"`
		Artifacts []string `json:"artifacts"`
	}{
		execution.ID, execution.Status, execution.CodeSHA256, inputHashes,
		execution.Result.Stdout, execution.Result.Stderr, artifacts,
	})
	hash := fmt.Sprintf("%x", sha256.Sum256(raw))
	return SourceRef{
		SourceKind: "EXECUTION", SourceID: "execution:" + execution.ID + ":" + hash[:16],
		ExecutionID: execution.ID, CodeSHA256: execution.CodeSHA256, InputSHA256: inputHashes,
		ContentSHA256: hash, CoverageStatus: execution.Status,
	}
}

func PythonTerminal(status string) bool {
	return status == "SUCCEEDED" || status == "FAILED" || status == "CANCELED" || status == "INTERRUPTED"
}

// Scheduler methods operate under the application's single-instance lease.
// Private reads and reservations always require a trusted owner in context.
type PythonRepository interface {
	CreatePythonExecution(context.Context, PythonExecution) (PythonExecution, error)
	GetPythonExecution(context.Context, string) (PythonExecution, error)
	PendingPythonExecutions(context.Context) ([]PythonExecution, error)
	SavePythonExecution(context.Context, PythonExecution, int64) error
	CancelPythonExecution(context.Context, string, string) error
	InterruptPythonExecutions(context.Context) error
	ResponsePythonSettled(context.Context, string) (bool, error)
	ResponsePythonExecutions(context.Context, string) ([]PythonExecution, error)
	GetArtifact(context.Context, string) (Artifact, error)
	DeletedPythonExecutions(context.Context) ([]PythonExecution, error)
	DeletePythonExecution(context.Context, string) error
}
