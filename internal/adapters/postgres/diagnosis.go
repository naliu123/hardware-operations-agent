package postgres

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"

	"hwops/internal/domain"
)

func (s *Store) CreateIncident(ctx context.Context, in domain.Incident) (out domain.Incident, err error) {
	raw, err := json.Marshal(in)
	if err != nil {
		return out, err
	}
	var key any
	if in.RequestKey != "" {
		key = in.RequestKey
	}
	var saved []byte
	err = s.pool.QueryRow(ctx, `INSERT INTO incidents(id, request_key, payload) VALUES($1,$2,$3)
		ON CONFLICT(request_key) DO UPDATE SET request_key=EXCLUDED.request_key RETURNING payload`, in.ID, key, raw).Scan(&saved)
	if err != nil {
		return out, err
	}
	if err = json.Unmarshal(saved, &out); err == nil && out.RequestHash != in.RequestHash {
		err = domain.ErrConflict
	}
	return
}

func (s *Store) GetIncident(ctx context.Context, id string) (out domain.Incident, err error) {
	err = s.get(ctx, "incidents", id, &out)
	return
}

func (s *Store) CreateRun(ctx context.Context, in domain.DiagnosticRun) (out domain.DiagnosticRun, err error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return out, err
	}
	defer tx.Rollback(ctx)
	var id string
	if err = tx.QueryRow(ctx, "SELECT id FROM incidents WHERE id=$1 FOR UPDATE", in.IncidentID).Scan(&id); errors.Is(err, pgx.ErrNoRows) {
		return out, domain.ErrNotFound
	} else if err != nil {
		return out, err
	}
	var raw []byte
	err = tx.QueryRow(ctx, "SELECT payload FROM diagnostic_runs WHERE incident_id=$1 AND status NOT IN ('COMPLETED','CANCELED')", in.IncidentID).Scan(&raw)
	if err == nil {
		err = json.Unmarshal(raw, &out)
		return out, err
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return out, err
	}
	var snapshot string
	err = tx.QueryRow(ctx, "SELECT payload->>'snapshot_id' FROM device_snapshots WHERE id=$1 FOR SHARE", in.Device.DeviceID).Scan(&snapshot)
	if err != nil {
		return out, err
	}
	if snapshot != in.Device.SnapshotID {
		return out, domain.ErrConflict
	}
	raw, err = json.Marshal(in)
	if err != nil {
		return out, err
	}
	_, err = tx.Exec(ctx, "INSERT INTO diagnostic_runs(id,incident_id,state_version,status,payload) VALUES($1,$2,$3,$4,$5)",
		in.ID, in.IncidentID, in.StateVersion, in.Status, raw)
	if err == nil {
		err = tx.Commit(ctx)
	}
	return in, err
}

func (s *Store) GetRun(ctx context.Context, id string) (out domain.DiagnosticRun, err error) {
	err = s.get(ctx, "diagnostic_runs", id, &out)
	return
}

func (s *Store) CommitRun(ctx context.Context, in domain.DiagnosticRun, expected int64, snapshot string, revisions ...string) error {
	if in.StateVersion != expected+1 {
		return domain.ErrConflict
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if snapshot != "" {
		var current string
		if err = tx.QueryRow(ctx, "SELECT payload->>'snapshot_id' FROM device_snapshots WHERE id=$1 FOR SHARE", in.Device.DeviceID).Scan(&current); err != nil {
			return err
		}
		if current != snapshot {
			return domain.ErrConflict
		}
	}
	for _, id := range revisions {
		var status string
		if err := tx.QueryRow(ctx, "SELECT payload->>'status' FROM knowledge_revisions WHERE id=$1 FOR SHARE", id).Scan(&status); err != nil {
			return err
		}
		if status != "PUBLISHED" {
			return domain.ErrConflict
		}
	}
	raw, err := json.Marshal(in)
	if err != nil {
		return err
	}
	tag, err := tx.Exec(ctx, `UPDATE diagnostic_runs SET state_version=$1,status=$2,payload=$3
		WHERE id=$4 AND state_version=$5 AND incident_id=$6`, in.StateVersion, in.Status, raw, in.ID, expected, in.IncidentID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return domain.ErrConflict
	}
	return tx.Commit(ctx)
}

func (s *Store) PendingRuns(ctx context.Context) ([]domain.DiagnosticRun, error) {
	rows, err := s.pool.Query(ctx, "SELECT payload FROM diagnostic_runs WHERE status IN ('QUEUED','RUNNING') ORDER BY payload->>'updated_at'")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.DiagnosticRun
	for rows.Next() {
		var raw []byte
		var r domain.DiagnosticRun
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(raw, &r); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
