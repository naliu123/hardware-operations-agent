package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"hwops/internal/domain"
)

func (s *Store) CreatePythonExecution(ctx context.Context, e domain.PythonExecution) (domain.PythonExecution, error) {
	if domain.Owner(ctx) == "" || e.OwnerID != domain.Owner(ctx) || e.Status != "QUEUED" ||
		e.Budget != domain.PythonLimits() {
		return e, domain.ErrForbidden
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return e, err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtext('hwops:python-queue'))"); err != nil {
		return e, err
	}
	var raw []byte
	err = tx.QueryRow(ctx, `SELECT payload FROM conversations WHERE id=$1 AND owner_id=$2
		AND deleted_at IS NULL FOR UPDATE`, e.ConversationID, e.OwnerID).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return e, domain.ErrNotFound
	}
	if err != nil {
		return e, err
	}
	err = tx.QueryRow(ctx, `SELECT payload FROM responses WHERE id=$1 AND payload->>'conversation_id'=$2 FOR UPDATE`,
		e.ResponseID, e.ConversationID).Scan(&raw)
	if err != nil {
		return e, err
	}
	var r domain.Response
	if err = json.Unmarshal(raw, &r); err != nil {
		return e, err
	}
	if r.Status != "RUNNING" || !time.Now().Before(r.Deadline) || !e.Deadline.Equal(r.Deadline) {
		return e, fmt.Errorf("%w: 本轮已结束、取消或超时。", domain.ErrConflict)
	}
	var attempts, queued int
	if err = tx.QueryRow(ctx, "SELECT count(*) FROM python_executions WHERE response_id=$1", r.ID).Scan(&attempts); err != nil {
		return e, err
	}
	if err = tx.QueryRow(ctx, `SELECT count(*) FROM python_executions WHERE status IN ('QUEUED','STARTING','RUNNING','CANCELING')`).Scan(&queued); err != nil {
		return e, err
	}
	if attempts >= 3 || queued >= 20 {
		return e, fmt.Errorf("%w: Python 每轮最多三次，等待队列最多二十次。", domain.ErrConflict)
	}
	e.Version = 1
	raw, err = json.Marshal(e)
	if err != nil {
		return e, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO python_executions(id,owner_id,conversation_id,response_id,status,version,created_at,payload)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`, e.ID, e.OwnerID, e.ConversationID, e.ResponseID, e.Status, e.Version, e.CreatedAt, raw)
	if err != nil {
		return e, err
	}
	return e, tx.Commit(ctx)
}

func (s *Store) GetPythonExecution(ctx context.Context, id string) (e domain.PythonExecution, err error) {
	if domain.Owner(ctx) == "" {
		return e, domain.ErrNotFound
	}
	var raw []byte
	err = s.pool.QueryRow(ctx, `SELECT e.payload,e.owner_id FROM python_executions e
		JOIN conversations c ON c.id=e.conversation_id WHERE e.id=$1 AND e.owner_id=$2
		AND c.deleted_at IS NULL`, id, domain.Owner(ctx)).Scan(&raw, &e.OwnerID)
	if errors.Is(err, pgx.ErrNoRows) {
		return e, domain.ErrNotFound
	}
	if err == nil {
		err = json.Unmarshal(raw, &e)
	}
	return
}

func (s *Store) PendingPythonExecutions(ctx context.Context) ([]domain.PythonExecution, error) {
	return s.pythonRows(ctx, `SELECT payload,owner_id FROM python_executions
		WHERE status IN ('QUEUED','STARTING','RUNNING','CANCELING')
		OR payload->'result'->>'cleaned'='false' ORDER BY created_at LIMIT 100`)
}

func (s *Store) pythonRows(ctx context.Context, query string) ([]domain.PythonExecution, error) {
	rows, err := s.pool.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.PythonExecution{}
	for rows.Next() {
		var raw []byte
		var e domain.PythonExecution
		if err = rows.Scan(&raw, &e.OwnerID); err != nil {
			return nil, err
		}
		if err = json.Unmarshal(raw, &e); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (s *Store) SavePythonExecution(ctx context.Context, e domain.PythonExecution, version int64) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var id string
	// Same lock order as response submission, cancellation and deletion.
	if err = tx.QueryRow(ctx, "SELECT id FROM conversations WHERE id=$1 FOR UPDATE", e.ConversationID).Scan(&id); err != nil {
		return err
	}
	var raw []byte
	err = tx.QueryRow(ctx, "SELECT payload FROM python_executions WHERE id=$1 AND version=$2 FOR UPDATE", e.ID, version).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ErrConflict
	}
	if err != nil {
		return err
	}
	var saved domain.PythonExecution
	if err = json.Unmarshal(raw, &saved); err != nil {
		return err
	}
	if saved.RequestSHA256 != e.RequestSHA256 || saved.CodeSHA256 != e.CodeSHA256 ||
		(domain.PythonTerminal(saved.Status) && saved.Result.Cleaned) ||
		(saved.CancelReason != "" && (e.CancelReason != saved.CancelReason || e.Status == "SUCCEEDED")) {
		return domain.ErrConflict
	}
	if domain.PythonTerminal(e.Status) && !e.Result.Cleaned {
		return fmt.Errorf("%w: sandbox cleanup is unconfirmed", domain.ErrConflict)
	}
	if e.Status == "SUCCEEDED" && (!e.Result.Complete || e.Result.ExitCode == nil || *e.Result.ExitCode != 0) {
		return domain.ErrInvalid
	}
	e.Version = version + 1
	raw, err = json.Marshal(e)
	if err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, "UPDATE python_executions SET status=$2,version=$3,payload=$4 WHERE id=$1", e.ID, e.Status, e.Version, raw); err != nil {
		return err
	}
	if e.Status == "SUCCEEDED" {
		for _, a := range e.Result.Artifacts {
			if a.StorageKey == "" || a.ExecutionID != e.ID || a.ConversationID != e.ConversationID {
				return domain.ErrInvalid
			}
			raw, err = json.Marshal(a)
			if err != nil {
				return err
			}
			if _, err = tx.Exec(ctx, "INSERT INTO artifacts(id,execution_id,storage_key,payload) VALUES ($1,$2,$3,$4)",
				a.ID, e.ID, a.StorageKey, raw); err != nil {
				return err
			}
		}
	}
	return tx.Commit(ctx)
}

// cancelPython is part of the same transaction that records cancellation or
// deletion. Queued jobs cannot become STARTING after this update.
func cancelPython(ctx context.Context, tx pgx.Tx, responseID, reason string) error {
	_, err := tx.Exec(ctx, `UPDATE python_executions SET status='CANCELING',version=version+1,
		payload=payload||jsonb_build_object('status','CANCELING','cancel_reason',$2::text,'state_version',version+1)
		WHERE response_id=$1 AND status IN ('QUEUED','STARTING','RUNNING','CANCELING')`, responseID, reason)
	return err
}

func (s *Store) CancelPythonExecution(ctx context.Context, id, reason string) error {
	_, err := s.pool.Exec(ctx, `UPDATE python_executions SET status='CANCELING',version=version+1,
		payload=payload||jsonb_build_object('status','CANCELING','cancel_reason',$3::text,'state_version',version+1)
		WHERE id=$1 AND owner_id=$2 AND status IN ('QUEUED','STARTING','RUNNING')
		AND ($3='CANCELED' OR $3='INTERRUPTED' OR $3='DEADLINE_EXCEEDED')`, id, domain.Owner(ctx), reason)
	return err
}

func (s *Store) InterruptPythonExecutions(ctx context.Context) error {
	_, err := s.pool.Exec(ctx, `UPDATE python_executions SET status='CANCELING',version=version+1,
		payload=payload||jsonb_build_object('status','CANCELING','cancel_reason','INTERRUPTED','state_version',version+1)
		WHERE status IN ('QUEUED','STARTING','RUNNING','CANCELING')`)
	return err
}

func (s *Store) ResponsePythonSettled(ctx context.Context, id string) (bool, error) {
	var active bool
	err := s.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM python_executions WHERE response_id=$1
		AND (status IN ('QUEUED','STARTING','RUNNING','CANCELING') OR payload->'result'->>'cleaned'='false'))`, id).Scan(&active)
	return !active, err
}

func (s *Store) ResponsePythonExecutions(ctx context.Context, id string) ([]domain.PythonExecution, error) {
	rows, err := s.pool.Query(ctx, `SELECT e.payload,e.owner_id FROM python_executions e
		JOIN responses r ON r.id=e.response_id
		JOIN conversations c ON c.id=e.conversation_id
		WHERE e.response_id=$1 AND c.deleted_at IS NULL
		AND ($2='' OR e.owner_id=$2) ORDER BY e.created_at,e.id`, id, domain.Owner(ctx))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.PythonExecution{}
	for rows.Next() {
		var raw []byte
		var execution domain.PythonExecution
		if err = rows.Scan(&raw, &execution.OwnerID); err != nil {
			return nil, err
		}
		if err = json.Unmarshal(raw, &execution); err != nil {
			return nil, err
		}
		out = append(out, execution)
	}
	return out, rows.Err()
}

func (s *Store) GetArtifact(ctx context.Context, id string) (a domain.Artifact, err error) {
	var raw []byte
	err = s.pool.QueryRow(ctx, `SELECT a.payload,a.storage_key FROM artifacts a
		JOIN python_executions e ON e.id=a.execution_id JOIN conversations c ON c.id=e.conversation_id
		WHERE a.id=$1 AND e.owner_id=$2 AND e.status='SUCCEEDED' AND c.deleted_at IS NULL`, id, domain.Owner(ctx)).Scan(&raw, &a.StorageKey)
	if errors.Is(err, pgx.ErrNoRows) {
		return a, domain.ErrNotFound
	}
	if err == nil {
		err = json.Unmarshal(raw, &a)
	}
	return
}

func (s *Store) DeletedPythonExecutions(ctx context.Context) ([]domain.PythonExecution, error) {
	return s.pythonRows(ctx, `SELECT e.payload,e.owner_id FROM python_executions e
		JOIN conversations c ON c.id=e.conversation_id WHERE c.deleted_at IS NOT NULL ORDER BY e.created_at LIMIT 100`)
}

func (s *Store) DeletePythonExecution(ctx context.Context, id string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var cid string
	if err = tx.QueryRow(ctx, `SELECT c.id FROM conversations c JOIN python_executions e ON e.conversation_id=c.id
		WHERE e.id=$1 AND c.deleted_at IS NOT NULL FOR UPDATE OF c`, id).Scan(&cid); err != nil {
		return err
	}
	var pending bool
	if err = tx.QueryRow(ctx, `SELECT status IN ('QUEUED','STARTING','RUNNING','CANCELING') OR payload->'result'->>'cleaned'='false'
		FROM python_executions WHERE id=$1`, id).Scan(&pending); err != nil || pending {
		return domain.ErrConflict
	}
	if _, err = tx.Exec(ctx, "DELETE FROM artifacts WHERE execution_id=$1", id); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, "DELETE FROM python_executions WHERE id=$1", id); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
