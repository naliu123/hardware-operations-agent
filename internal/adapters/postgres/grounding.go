package postgres

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"

	"hwops/internal/domain"
)

func (s *Store) SaveGroundingReview(ctx context.Context, id string, review domain.GroundingReview, usage *domain.ModelUsage) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var conversationID string
	if err = tx.QueryRow(ctx, "SELECT payload->>'conversation_id' FROM responses WHERE id=$1", id).Scan(&conversationID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrNotFound
		}
		return err
	}
	if err = tx.QueryRow(ctx, `SELECT id FROM conversations WHERE id=$1 AND deleted_at IS NULL
		AND ($2='' OR owner_id=$2) FOR UPDATE`, conversationID, domain.Owner(ctx)).Scan(&conversationID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrNotFound
		}
		return err
	}
	var raw []byte
	if err = tx.QueryRow(ctx, "SELECT payload FROM responses WHERE id=$1 FOR UPDATE", id).Scan(&raw); err != nil {
		return err
	}
	var r domain.Response
	if err = json.Unmarshal(raw, &r); err != nil {
		return err
	}
	if r.Status != "RUNNING" {
		return domain.ErrConflict
	}
	r.GroundingReviews = append(r.GroundingReviews, review)
	r.ModelUsage = usage
	raw, err = json.Marshal(r)
	if err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, "UPDATE responses SET payload=$2 WHERE id=$1", id, raw); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
