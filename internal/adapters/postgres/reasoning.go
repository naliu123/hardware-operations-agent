package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"

	"hwops/internal/domain"
)

var _ domain.ReasoningEventRepository = (*Store)(nil)

func (s *Store) AppendResponseReasoning(ctx context.Context, id, delta string) (domain.ResponseEvent, error) {
	if delta == "" || len(delta) > 16*1024 || !utf8.ValidString(delta) {
		return domain.ResponseEvent{}, domain.ErrInvalid
	}
	return s.writeReasoning(ctx, id, delta)
}

func (s *Store) FinishResponseReasoning(ctx context.Context, id string) error {
	_, err := s.writeReasoning(ctx, id, "")
	return err
}

// Snapshot and event share the response lock and commit. Lock conversations
// first, as cancellation/deletion do, then re-read the latest response.
func (s *Store) writeReasoning(ctx context.Context, id, delta string) (event domain.ResponseEvent, err error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return event, err
	}
	defer tx.Rollback(ctx)
	var conversationID string
	err = tx.QueryRow(ctx, `SELECT payload->>'conversation_id' FROM responses WHERE id=$1`, id).Scan(&conversationID)
	if errors.Is(err, pgx.ErrNoRows) {
		return event, domain.ErrNotFound
	}
	if err != nil {
		return event, err
	}
	err = tx.QueryRow(ctx, `SELECT id FROM conversations WHERE id=$1 AND deleted_at IS NULL
		AND ($2='' OR owner_id=$2) FOR UPDATE`, conversationID, domain.Owner(ctx)).Scan(&conversationID)
	if errors.Is(err, pgx.ErrNoRows) {
		return event, domain.ErrNotFound
	}
	if err != nil {
		return event, err
	}
	var raw []byte
	if err = tx.QueryRow(ctx, "SELECT payload FROM responses WHERE id=$1 FOR UPDATE", id).Scan(&raw); err != nil {
		return event, err
	}
	var r domain.Response
	if err = json.Unmarshal(raw, &r); err != nil {
		return event, err
	}
	if r.Status != "RUNNING" {
		return event, domain.ErrConflict
	}
	if delta == "" && (r.Reasoning == nil || r.Reasoning.Status != "RUNNING") {
		return event, nil
	}
	if r.Reasoning == nil {
		r.Reasoning = &domain.ResponseReasoning{}
	}
	if delta != "" && r.Reasoning.Status == "COMPLETED" {
		delta = "\n\n" + delta
	}
	if len(r.Reasoning.Text)+len(delta) > domain.MaxReasoningBytes {
		return event, domain.ErrResourceExhausted
	}
	var sequence int64
	if err = tx.QueryRow(ctx, `SELECT COALESCE(MAX(sequence),0)+1 FROM response_events WHERE response_id=$1`, id).Scan(&sequence); err != nil {
		return event, err
	}
	status := "RUNNING"
	if delta == "" {
		status = "COMPLETED"
	}
	r.Reasoning.Text += delta
	r.Reasoning.Status, r.Reasoning.EventID = status, sequence
	event = domain.NewReasoningEvent(r, sequence, delta, status)
	if err = insertResponseEvent(ctx, tx, event); err != nil {
		return event, err
	}
	raw, err = json.Marshal(r)
	if err != nil {
		return event, err
	}
	if _, err = tx.Exec(ctx, "UPDATE responses SET payload=$2 WHERE id=$1", id, raw); err != nil {
		return event, err
	}
	return event, tx.Commit(ctx)
}
