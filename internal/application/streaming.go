package application

import (
	"context"
	"unicode/utf8"

	"hwops/internal/domain"
)

type responseDraftEmitter struct {
	repo       domain.DraftEventRepository
	responseID string
	version    int64
}

func (e *responseDraftEmitter) BeginDraft(ctx context.Context) error {
	version, err := e.repo.StartResponseDraft(ctx, e.responseID)
	if err == nil {
		e.version = version
	}
	return err
}

func (e *responseDraftEmitter) EmitDraftDelta(ctx context.Context, delta string) error {
	if e.version < 1 {
		return domain.ErrConflict
	}
	_, err := e.repo.AppendResponseDelta(ctx, e.responseID, e.version, delta)
	return err
}

type responseReasoningEmitter struct {
	repo       domain.ReasoningEventRepository
	responseID string
}

func (e *responseReasoningEmitter) EmitReasoningDelta(ctx context.Context, delta string) error {
	for len(delta) > 0 {
		n := min(len(delta), 16*1024)
		for n < len(delta) && !utf8.RuneStart(delta[n]) {
			n--
		}
		if n == 0 {
			return domain.ErrInvalid
		}
		if _, err := e.repo.AppendResponseReasoning(ctx, e.responseID, delta[:n]); err != nil {
			return err
		}
		delta = delta[n:]
	}
	return nil
}

func (e *responseReasoningEmitter) FinishReasoning(ctx context.Context) error {
	return e.repo.FinishResponseReasoning(ctx, e.responseID)
}
