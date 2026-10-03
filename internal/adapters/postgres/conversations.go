package postgres

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"

	"hwops/internal/domain"
)

type rowQuerier interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func findRequest(ctx context.Context, db rowQuerier, conversationID, key, hash string) (r domain.Response, err error) {
	if key == "" {
		return r, domain.ErrNotFound
	}
	var raw []byte
	err = db.QueryRow(ctx, "SELECT payload FROM responses WHERE payload->>'conversation_id'=$1 AND payload->>'request_key'=$2", conversationID, key).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return r, domain.ErrNotFound
	}
	if err != nil {
		return r, err
	}
	if err = json.Unmarshal(raw, &r); err == nil && r.RequestHash != hash {
		err = domain.ErrConflict
	}
	return
}

func (s *Store) FindResponseRequest(ctx context.Context, conversationID, key, hash string) (domain.Response, error) {
	return findRequest(ctx, s.pool, conversationID, key, hash)
}

func (s *Store) CreateResponse(ctx context.Context, r domain.Response, expectedVersion int64) (domain.Response, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return r, err
	}
	defer tx.Rollback(ctx)
	var raw []byte
	err = tx.QueryRow(ctx, "SELECT payload FROM conversations WHERE id=$1 FOR UPDATE", r.ConversationID).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return r, domain.ErrNotFound
	}
	if err != nil {
		return r, err
	}
	existing, err := findRequest(ctx, tx, r.ConversationID, r.RequestKey, r.RequestHash)
	if err == nil {
		return existing, nil
	}
	if !errors.Is(err, domain.ErrNotFound) {
		return r, err
	}
	var c domain.Conversation
	if err := json.Unmarshal(raw, &c); err != nil {
		return r, err
	}
	if expectedVersion >= 0 && c.ContextVersion != expectedVersion {
		return r, domain.ErrConflict
	}
	c.ApplyResponseContext(r)
	raw, err = json.Marshal(c)
	if err != nil {
		return r, err
	}
	if _, err = tx.Exec(ctx, "UPDATE conversations SET payload=$2 WHERE id=$1", c.ID, raw); err != nil {
		return r, err
	}
	if err = saveResponse(ctx, tx, &r); err != nil {
		return r, err
	}
	return r, tx.Commit(ctx)
}

func saveResponse(ctx context.Context, tx pgx.Tx, r *domain.Response) error {
	if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1,0))", "response:"+r.ID); err != nil {
		return err
	}
	var previous string
	err := tx.QueryRow(ctx, "SELECT payload->>'status' FROM responses WHERE id=$1 FOR UPDATE", r.ID).Scan(&previous)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	if (r.Status == "ANSWERED" || r.Status == "PARTIAL") && r.DeviceContext != nil {
		var snapshotID string
		err = tx.QueryRow(ctx, "SELECT payload->>'snapshot_id' FROM device_snapshots WHERE id=$1 FOR UPDATE", r.DeviceContext.DeviceID).Scan(&snapshotID)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		r.CheckDeviceSnapshot(snapshotID)
	}
	raw, err := json.Marshal(r)
	if err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO responses (id,payload) VALUES ($1,$2)
		ON CONFLICT (id) DO UPDATE SET payload=EXCLUDED.payload`, r.ID, raw); err != nil {
		return err
	}
	var sequence int64
	if err = tx.QueryRow(ctx, "SELECT COALESCE(MAX(sequence),0) FROM response_events WHERE response_id=$1", r.ID).Scan(&sequence); err != nil {
		return err
	}
	for _, event := range domain.NewResponseEvents(previous, *r, sequence) {
		raw, err := json.Marshal(event)
		if err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, "INSERT INTO response_events (response_id,sequence,payload) VALUES ($1,$2,$3)", r.ID, event.ID, raw); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) ResponseEvents(ctx context.Context, id string, after int64) ([]domain.ResponseEvent, error) {
	if _, err := s.GetResponse(ctx, id); err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, "SELECT payload FROM response_events WHERE response_id=$1 AND sequence>$2 ORDER BY sequence", id, after)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.ResponseEvent{}
	for rows.Next() {
		var raw []byte
		var event domain.ResponseEvent
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(raw, &event); err != nil {
			return nil, err
		}
		out = append(out, event)
	}
	return out, rows.Err()
}
