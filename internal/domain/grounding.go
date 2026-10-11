package domain

import "context"

type GroundingRepository interface {
	SaveGroundingReview(context.Context, string, GroundingReview, *ModelUsage) error
}

type GroundingSupport struct {
	EvidenceID string `json:"evidence_id"`
	StartLine  int    `json:"start_line"`
	EndLine    int    `json:"end_line"`
	Quote      string `json:"quote"`
}

type GroundingVerdict struct {
	ClaimIndex int                `json:"claim_index"`
	Verdict    string             `json:"verdict"`
	Reason     string             `json:"reason"`
	Supports   []GroundingSupport `json:"supports"`
}

type GroundingEvidenceRef struct {
	ID          string `json:"id"`
	RevisionID  string `json:"revision_id,omitempty"`
	ContentHash string `json:"content_hash"`
	StartLine   int    `json:"start_line"`
	EndLine     int    `json:"end_line"`
}

// GroundingReview binds a model verdict to immutable claims and evidence.
// VERIFIED means the review protocol passed, not a human quality certification.
type GroundingReview struct {
	Version      string                 `json:"version"`
	Model        string                 `json:"model"`
	Status       string                 `json:"status"`
	ClaimsSHA256 string                 `json:"claims_sha256"`
	InputSHA256  string                 `json:"input_sha256"`
	InputClaims  []Claim                `json:"input_claims"`
	Evidence     []GroundingEvidenceRef `json:"evidence"`
	Claims       []GroundingVerdict     `json:"claims,omitempty"`
	TraceID      string                 `json:"trace_id,omitempty"`
	ModelUsage   *ModelUsage            `json:"model_usage,omitempty"`
	Error        *Failure               `json:"error,omitempty"`
}
