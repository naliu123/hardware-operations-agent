package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"hwops/internal/domain"
)

func (s *Store) HasCitedRevision(ctx context.Context, revisionID string) (bool, error) {
	var exists bool
	err := s.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM responses r
		JOIN conversations c ON c.id=r.payload->>'conversation_id'
		WHERE c.owner_id=$1 AND c.deleted_at IS NULL AND r.payload->'citations' @> $2::jsonb)`,
		domain.Owner(ctx), fmt.Sprintf(`[{"revision_id":%q}]`, revisionID)).Scan(&exists)
	return exists, err
}

// MapLegacyOwner is an explicit, offline migration operation. It never assigns
// records just because a new account happens to have a matching username.
func (s *Store) MapLegacyOwner(ctx context.Context, sourceOwner, targetUsername string) (int64, error) {
	if sourceOwner != "local-operator" {
		return 0, fmt.Errorf("%w: only local-operator history can be mapped", domain.ErrInvalid)
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)
	var target string
	if err = tx.QueryRow(ctx, "SELECT id FROM users WHERE username=$1 AND active FOR UPDATE", targetUsername).Scan(&target); errors.Is(err, pgx.ErrNoRows) {
		return 0, domain.ErrNotFound
	} else if err != nil {
		return 0, err
	}
	var previous string
	err = tx.QueryRow(ctx, "SELECT target_user_id FROM ownership_mappings WHERE source_owner=$1", sourceOwner).Scan(&previous)
	if err == nil {
		if previous != target {
			return 0, domain.ErrConflict
		}
		return 0, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return 0, err
	}
	ownerJSON, _ := json.Marshal(target)
	tag, err := tx.Exec(ctx, `UPDATE conversations SET owner_id=$2,payload=jsonb_set(payload,'{owner}',$3::jsonb)
		WHERE owner_id IS NULL AND payload->>'owner'=$1`, sourceOwner, target, ownerJSON)
	if err != nil {
		return 0, err
	}
	if _, err = tx.Exec(ctx, "INSERT INTO ownership_mappings(source_owner,target_user_id,mapped_count) VALUES ($1,$2,$3)",
		sourceOwner, target, tag.RowsAffected()); err != nil {
		return 0, err
	}
	return tag.RowsAffected(), tx.Commit(ctx)
}
