package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"hwops/internal/domain"
	"hwops/migrations"
)

type Store struct {
	pool  *pgxpool.Pool
	lease *pgxpool.Conn
	users bool
}

func Open(ctx context.Context, dsn string) (*Store, error) {
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, errors.New("invalid PostgreSQL configuration")
	}
	if config.MaxConns < 2 {
		return nil, errors.New("PostgreSQL pool_max_conns must be at least 2 (one connection holds the instance lock)")
	}
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, errors.New("invalid PostgreSQL configuration")
	}
	lease, err := pool.Acquire(ctx)
	if err != nil {
		pool.Close()
		return nil, errors.New("PostgreSQL unavailable")
	}
	s := &Store{pool: pool, lease: lease}
	var locked bool
	if err = lease.QueryRow(ctx, "SELECT pg_try_advisory_lock(hashtext('hwops:qa01'))").Scan(&locked); err != nil || !locked {
		s.Close()
		return nil, errors.New("hwops requires one application instance per database")
	}
	if err = migrations.Apply(ctx, pool); err != nil {
		s.Close()
		return nil, fmt.Errorf("apply hwops schema: %w", err)
	}
	return s, nil
}

func (s *Store) Pool() *pgxpool.Pool { return s.pool }

// EnableUsers must run before any application workers start.
func (s *Store) EnableUsers() error { s.users = true; return nil }

func (s *Store) PutDevice(ctx context.Context, d domain.DeviceContext) error {
	raw, err := json.Marshal(d)
	if err != nil {
		return err
	}
	tag, err := s.pool.Exec(ctx, `INSERT INTO device_snapshots (id,observed_at,payload) VALUES ($1,$2,$3)
		ON CONFLICT (id) DO UPDATE SET observed_at=EXCLUDED.observed_at,payload=EXCLUDED.payload
		WHERE device_snapshots.observed_at < EXCLUDED.observed_at`, d.DeviceID, d.ObservedAt, raw)
	if err == nil && tag.RowsAffected() == 0 {
		return fmt.Errorf("%w: device observation must be newer than the saved snapshot", domain.ErrConflict)
	}
	return err
}

func (s *Store) CreateVersionPolicy(ctx context.Context, p domain.VersionPolicy) error {
	return s.insert(ctx, "version_policies", p.ID, p, false)
}

func (s *Store) GetVersionPolicy(ctx context.Context, id string) (p domain.VersionPolicy, err error) {
	err = s.get(ctx, "version_policies", id, &p)
	return
}

func (s *Store) GetDevice(ctx context.Context, id string) (d domain.DeviceContext, err error) {
	err = s.get(ctx, "device_snapshots", id, &d)
	return
}

func (s *Store) ListDevices(ctx context.Context) ([]domain.DeviceContext, error) {
	rows, err := s.pool.Query(ctx, "SELECT payload FROM device_snapshots ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.DeviceContext
	for rows.Next() {
		var raw []byte
		var d domain.DeviceContext
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(raw, &d); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func (s *Store) Close() error {
	s.lease.Release()
	s.pool.Close()
	return nil
}

func (s *Store) insert(ctx context.Context, table, id string, value any, upsert bool) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	// table is always a package constant, never input from a request.
	query := "INSERT INTO " + table + " (id,payload) VALUES ($1,$2)"
	if upsert {
		query += " ON CONFLICT (id) DO UPDATE SET payload=EXCLUDED.payload"
	}
	_, err = s.pool.Exec(ctx, query, id, raw)
	return err
}

func (s *Store) get(ctx context.Context, table, id string, out any) error {
	var raw []byte
	err := s.pool.QueryRow(ctx, "SELECT payload FROM "+table+" WHERE id=$1", id).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ErrNotFound
	}
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, out)
}

func (s *Store) CreateRevision(ctx context.Context, r domain.Revision) error {
	return s.insert(ctx, "knowledge_revisions", r.ID, r, false)
}

func (s *Store) GetRevision(ctx context.Context, id string) (r domain.Revision, err error) {
	err = s.get(ctx, "knowledge_revisions", id, &r)
	return
}

func (s *Store) SetPublication(ctx context.Context, id, status string) (r domain.Revision, err error) {
	var raw []byte
	err = s.pool.QueryRow(ctx,
		"UPDATE knowledge_revisions SET payload=jsonb_set(payload,'{status}',to_jsonb($2::text)) WHERE id=$1 RETURNING payload",
		id, status).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return r, domain.ErrNotFound
	}
	if err == nil {
		err = json.Unmarshal(raw, &r)
	}
	return
}

func (s *Store) ListPublished(ctx context.Context) ([]domain.Revision, error) {
	rows, err := s.pool.Query(ctx, "SELECT payload FROM knowledge_revisions WHERE payload->>'status'='PUBLISHED' ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Revision
	for rows.Next() {
		var raw []byte
		var r domain.Revision
		if err = rows.Scan(&raw); err != nil {
			return nil, err
		}
		if err = json.Unmarshal(raw, &r); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) CreateConversation(ctx context.Context, c domain.Conversation) error {
	raw, err := json.Marshal(c)
	if err != nil {
		return err
	}
	var owner any
	if s.users {
		if c.Owner != domain.Owner(ctx) || c.Owner == "" {
			return domain.ErrForbidden
		}
		owner = c.Owner
	}
	_, err = s.pool.Exec(ctx, "INSERT INTO conversations(id,payload,owner_id) VALUES ($1,$2,$3)", c.ID, raw, owner)
	return err
}

func (s *Store) GetConversation(ctx context.Context, id string) (c domain.Conversation, err error) {
	var raw []byte
	err = s.pool.QueryRow(ctx, "SELECT payload FROM conversations WHERE id=$1 AND deleted_at IS NULL AND ($2='' OR owner_id=$2)", id, domain.Owner(ctx)).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		err = domain.ErrNotFound
	} else if err == nil {
		err = json.Unmarshal(raw, &c)
	}
	return
}

func (s *Store) SaveResponse(ctx context.Context, r domain.Response) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	// Always lock the conversation before the response, including cancellation
	// and cleanup. A late worker cannot recreate a deleted conversation.
	var id string
	err = tx.QueryRow(ctx, `SELECT id FROM conversations WHERE id=$1 AND deleted_at IS NULL
		AND ($2='' OR owner_id=$2) FOR UPDATE`, r.ConversationID, domain.Owner(ctx)).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ErrNotFound
	}
	if err != nil {
		return err
	}
	if r.Workbench {
		var exists bool
		if err = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM responses WHERE id=$1)", r.ID).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return domain.ErrNotFound
		}
	}
	if err := saveResponse(ctx, tx, &r); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) GetResponse(ctx context.Context, id string) (r domain.Response, err error) {
	var raw []byte
	err = s.pool.QueryRow(ctx, `SELECT r.payload FROM responses r JOIN conversations c ON c.id=r.payload->>'conversation_id'
		WHERE r.id=$1 AND c.deleted_at IS NULL AND ($2='' OR c.owner_id=$2)`, id, domain.Owner(ctx)).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		err = domain.ErrNotFound
	} else if err == nil {
		err = json.Unmarshal(raw, &r)
	}
	return
}

func (s *Store) PendingResponses(ctx context.Context) ([]domain.Response, error) {
	rows, err := s.pool.Query(ctx, `SELECT r.payload FROM responses r JOIN conversations c ON c.id=r.payload->>'conversation_id'
		WHERE r.payload->>'status' IN ('QUEUED','RUNNING','CANCELING') AND c.deleted_at IS NULL
		AND (NOT $1 OR c.owner_id IS NOT NULL) ORDER BY r.payload->>'created_at' LIMIT 100`, s.users)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Response
	for rows.Next() {
		var raw []byte
		var r domain.Response
		if err = rows.Scan(&raw); err != nil {
			return nil, err
		}
		if err = json.Unmarshal(raw, &r); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out, rows.Err()
}
