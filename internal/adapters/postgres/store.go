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
	if _, err = pool.Exec(ctx, migrations.Core+"\n"+migrations.Devices); err != nil {
		s.Close()
		return nil, fmt.Errorf("apply hwops schema: %w", err)
	}
	return s, nil
}

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
	return s.insert(ctx, "conversations", c.ID, c, false)
}

func (s *Store) GetConversation(ctx context.Context, id string) (c domain.Conversation, err error) {
	err = s.get(ctx, "conversations", id, &c)
	return
}

func (s *Store) SaveResponse(ctx context.Context, r domain.Response) error {
	if (r.Status == "ANSWERED" || r.Status == "PARTIAL") && r.DeviceContext != nil {
		tx, err := s.pool.Begin(ctx)
		if err != nil {
			return err
		}
		defer tx.Rollback(ctx)
		var snapshotID string
		err = tx.QueryRow(ctx, "SELECT payload->>'snapshot_id' FROM device_snapshots WHERE id=$1 FOR UPDATE",
			r.DeviceContext.DeviceID).Scan(&snapshotID)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		r.CheckDeviceSnapshot(snapshotID)
		raw, err := json.Marshal(r)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `INSERT INTO responses (id,payload) VALUES ($1,$2)
			ON CONFLICT (id) DO UPDATE SET payload=EXCLUDED.payload`, r.ID, raw)
		if err != nil {
			return err
		}
		return tx.Commit(ctx)
	}
	return s.insert(ctx, "responses", r.ID, r, true)
}

func (s *Store) GetResponse(ctx context.Context, id string) (r domain.Response, err error) {
	err = s.get(ctx, "responses", id, &r)
	return
}

func (s *Store) PendingResponses(ctx context.Context) ([]domain.Response, error) {
	rows, err := s.pool.Query(ctx, "SELECT payload FROM responses WHERE payload->>'status' IN ('QUEUED','RUNNING')")
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
