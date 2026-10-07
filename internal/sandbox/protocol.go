// Package sandbox is the control-plane protocol for a trusted Linux runner.
// No host path, runtime, environment variable or mount is accepted in a job.
package sandbox

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"

	"hwops/internal/domain"
)

var (
	ErrUnavailable = errors.New("isolated Linux runner unavailable")
	ErrConflict    = errors.New("execution identity conflict")
	ErrMissing     = errors.New("execution identity unknown")
	safeID         = regexp.MustCompile(`^[A-Za-z0-9_-]{16,80}$`)
	digest         = regexp.MustCompile(`^sha256:[a-f0-9]{64}$`)
)

type Input struct {
	domain.ExecutionInput
	Data []byte `json:"data,omitempty"`
}

type Request struct {
	ID       string              `json:"id"`
	Code     string              `json:"code"`
	Inputs   []Input             `json:"inputs"`
	Image    string              `json:"image"`
	Budget   domain.PythonBudget `json:"budget"`
	Deadline time.Time           `json:"deadline"`
}

func (r Request) Hash() string {
	r.Inputs = append([]Input{}, r.Inputs...)
	for i := range r.Inputs {
		r.Inputs[i].Data = nil
	}
	raw, _ := json.Marshal(r)
	return Hash(raw)
}

func Hash(raw []byte) string { return fmt.Sprintf("%x", sha256.Sum256(raw)) }

func (r Request) Validate() error {
	if !safeID.MatchString(r.ID) || len(r.Code) == 0 || len(r.Code) > 64<<10 ||
		!digest.MatchString(r.Image) || r.Budget != domain.PythonLimits() || len(r.Inputs) > 20 ||
		r.Deadline.IsZero() {
		return errors.New("invalid execution request or limits")
	}
	seen, total := map[string]bool{}, int64(0)
	for _, input := range r.Inputs {
		total += input.Bytes
		if !safeID.MatchString(input.ID) || seen[input.ID] || input.Bytes < 0 ||
			input.Bytes != int64(len(input.Data)) || Hash(input.Data) != input.SHA256 ||
			input.Name == "" || len(input.Name) > 255 || strings.ContainsAny(input.Name, "\x00\r\n") ||
			(input.Kind != "ATTACHMENT" && input.Kind != "ARTIFACT") {
			return errors.New("invalid execution input manifest")
		}
		seen[input.ID] = true
	}
	if total > 100_000_000 {
		return errors.New("execution input exceeds 100 MB")
	}
	return nil
}

type Status struct {
	ID            string              `json:"id"`
	RequestSHA256 string              `json:"request_sha256"`
	State         string              `json:"state"`
	Result        domain.PythonResult `json:"result"`
}

type Capability struct {
	Ready       bool                `json:"ready"`
	Image       string              `json:"image"`
	Runtime     string              `json:"runtime"`
	Linux       string              `json:"linux"`
	Budget      domain.PythonBudget `json:"budget"`
	Packages    map[string]string   `json:"packages"`
	CheckedAt   time.Time           `json:"checked_at"`
	ProbeSHA256 string              `json:"probe_sha256"`
}

type Runner interface {
	Capability(context.Context) (Capability, error)
	Submit(context.Context, Request) (Status, error)
	Get(context.Context, string) (Status, error)
	Cancel(context.Context, string) (Status, error)
	Artifact(context.Context, string, string) (io.ReadCloser, error)
	Forget(context.Context, string) error
}
