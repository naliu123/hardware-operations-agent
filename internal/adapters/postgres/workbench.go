package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"hwops/internal/domain"
)

func (s *Store) ListConversations(ctx context.Context, query, after string, limit int) ([]domain.Conversation, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if domain.Owner(ctx) == "" || len(query) > 256 || limit < 1 || limit > 100 {
		return nil, domain.ErrInvalid
	}
	stamp, id, _ := strings.Cut(after, "|")
	if after != "" {
		if _, err := time.Parse(time.RFC3339Nano, stamp); err != nil || id == "" {
			return nil, domain.ErrInvalid
		}
	}
	rows, err := s.pool.Query(ctx, `SELECT c.payload FROM conversations c
		WHERE c.owner_id=$1 AND c.deleted_at IS NULL
		AND ($2='' OR strpos(lower(c.payload->>'title'),lower($2))>0 OR EXISTS (
			SELECT 1 FROM responses r WHERE r.payload->>'conversation_id'=c.id
			AND (strpos(lower(r.payload->>'question'),lower($2))>0 OR (
				r.payload->>'status' IN ('ANSWERED','PARTIAL')
				AND strpos(lower(r.payload->>'answer'),lower($2))>0))))
		AND ($3='' OR ((c.payload->>'created_at')::timestamptz,c.id)<(nullif($3,'')::timestamptz,$4))
		ORDER BY (c.payload->>'created_at')::timestamptz DESC,c.id DESC LIMIT $5`, domain.Owner(ctx), query, stamp, id, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.Conversation{}
	for rows.Next() {
		var raw []byte
		var c domain.Conversation
		if err = rows.Scan(&raw); err != nil {
			return nil, err
		}
		if err = json.Unmarshal(raw, &c); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *Store) RenameConversation(ctx context.Context, id, title string, version int64) (c domain.Conversation, err error) {
	title = strings.TrimSpace(title)
	if title == "" || len([]rune(title)) > 100 || version < 1 || domain.Owner(ctx) == "" {
		return c, domain.ErrInvalid
	}
	rawTitle, _ := json.Marshal(title)
	var raw []byte
	err = s.pool.QueryRow(ctx, `UPDATE conversations SET payload=payload || jsonb_build_object(
		'title',$3::jsonb,'title_source','MANUAL','state_version',$4::bigint+1,'updated_at',$5::text)
		WHERE id=$1 AND owner_id=$2 AND deleted_at IS NULL AND (payload->>'state_version')::bigint=$4
		RETURNING payload`, id, domain.Owner(ctx), rawTitle, version, time.Now().UTC().Format(time.RFC3339Nano)).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		if _, err = s.GetConversation(ctx, id); err == nil {
			err = fmt.Errorf("%w: 会话已更新，请刷新标题后重试。", domain.ErrConflict)
		}
	} else if err == nil {
		err = json.Unmarshal(raw, &c)
	}
	return
}

func (s *Store) ConversationMessages(ctx context.Context, id string, after, before int64, limit int) ([]domain.Response, error) {
	if after < 0 || before < 0 || limit < 1 || limit > 100 {
		return nil, domain.ErrInvalid
	}
	if _, err := s.GetConversation(ctx, id); err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, `SELECT r.payload FROM responses r JOIN conversations c ON c.id=r.payload->>'conversation_id'
		WHERE c.id=$1 AND c.deleted_at IS NULL AND ($2='' OR c.owner_id=$2)
		AND (r.payload->>'sequence')::bigint>$3 AND ($4=0 OR (r.payload->>'sequence')::bigint<$4)
		ORDER BY (r.payload->>'sequence')::bigint LIMIT $5`, id, domain.Owner(ctx), after, before, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.Response{}
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
	return out, rows.Err()
}

func (s *Store) CancelResponse(ctx context.Context, id string) (r domain.Response, err error) {
	r, err = s.GetResponse(ctx, id)
	if err != nil {
		return r, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return r, err
	}
	defer tx.Rollback(ctx)
	var raw []byte
	err = tx.QueryRow(ctx, `SELECT payload FROM conversations WHERE id=$1 AND owner_id=$2
		AND deleted_at IS NULL FOR UPDATE`, r.ConversationID, domain.Owner(ctx)).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return r, domain.ErrNotFound
	}
	if err != nil {
		return r, err
	}
	if err = tx.QueryRow(ctx, "SELECT payload FROM responses WHERE id=$1", id).Scan(&raw); err != nil {
		return r, err
	}
	if err = json.Unmarshal(raw, &r); err != nil {
		return r, err
	}
	if !domain.ResponseTerminal(r.Status) {
		if err = cancelPython(ctx, tx, id, "CANCELED"); err != nil {
			return r, err
		}
		if r.Status == "QUEUED" {
			r.Status = "CANCELED"
			r.Error = &domain.Failure{Code: "CANCELED", Message: "本轮已停止。"}
		} else {
			r.Status = "CANCELING"
		}
		if err = saveResponse(ctx, tx, &r); err != nil {
			return r, err
		}
	}
	return r, tx.Commit(ctx)
}

func (s *Store) DeleteConversation(ctx context.Context, id string) ([]string, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	var raw []byte
	err = tx.QueryRow(ctx, "SELECT payload FROM conversations WHERE id=$1 AND owner_id=$2 FOR UPDATE", id, domain.Owner(ctx)).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, `SELECT payload FROM responses WHERE payload->>'conversation_id'=$1
		AND payload->>'status' IN ('QUEUED','RUNNING','CANCELING')`, id)
	if err != nil {
		return nil, err
	}
	var pending []domain.Response
	for rows.Next() {
		var r domain.Response
		if err = rows.Scan(&raw); err == nil {
			err = json.Unmarshal(raw, &r)
		}
		if err != nil {
			rows.Close()
			return nil, err
		}
		pending = append(pending, r)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	ids := []string{}
	for _, r := range pending {
		if err = cancelPython(ctx, tx, r.ID, "CANCELED"); err != nil {
			return nil, err
		}
		r.Status = "CANCELED"
		r.Error = &domain.Failure{Code: "CONVERSATION_DELETED", Message: "会话已删除。"}
		if err = saveResponse(ctx, tx, &r); err != nil {
			return nil, err
		}
		ids = append(ids, r.ID)
	}
	if _, err = tx.Exec(ctx, "UPDATE conversations SET deleted_at=COALESCE(deleted_at,now()) WHERE id=$1", id); err != nil {
		return nil, err
	}
	if _, err = tx.Exec(ctx, "INSERT INTO cleanup_jobs(conversation_id) VALUES ($1) ON CONFLICT DO NOTHING", id); err != nil {
		return nil, err
	}
	return ids, tx.Commit(ctx)
}

func (s *Store) InterruptResponses(ctx context.Context) error {
	pending, err := s.PendingResponses(ctx)
	if err != nil {
		return err
	}
	for len(pending) > 0 {
		for _, r := range pending {
			r.Status = "INTERRUPTED"
			r.Error = &domain.Failure{Code: "SERVICE_RESTARTED", Message: "服务重启中断了本轮处理，请重试。"}
			if err = s.SaveResponse(ctx, r); err != nil {
				return err
			}
		}
		pending, err = s.PendingResponses(ctx)
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) CleanupConversations(ctx context.Context, running []string) error {
	rows, err := s.pool.Query(ctx, `SELECT conversation_id FROM cleanup_jobs
		WHERE status<>'DONE' AND (attempts=0 OR updated_at<now()-interval '2 seconds')
		ORDER BY updated_at LIMIT 20`)
	if err != nil {
		return err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, id := range ids {
		if err = s.cleanupConversation(ctx, id, running); err != nil {
			// Store a stable error code, never DB detail or private content.
			_, _ = s.pool.Exec(ctx, `UPDATE cleanup_jobs SET attempts=attempts+1,last_error='CLEANUP_FAILED',updated_at=now()
				WHERE conversation_id=$1`, id)
			return err
		}
	}
	return nil
}

func (s *Store) cleanupConversation(ctx context.Context, id string, running []string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var exists string
	if err = tx.QueryRow(ctx, "SELECT id FROM conversations WHERE id=$1 AND deleted_at IS NOT NULL FOR UPDATE", id).Scan(&exists); err != nil {
		return err
	}
	var active bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM responses WHERE payload->>'conversation_id'=$1 AND id=ANY($2::text[]))`,
		id, running).Scan(&active); err != nil {
		return err
	}
	if active {
		return domain.ErrConflict
	}
	// External files and runner journals must be removed before their durable
	// identities disappear. The execution module retries those operations.
	if err = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM python_executions WHERE conversation_id=$1)", id).Scan(&active); err != nil {
		return err
	}
	if active {
		return domain.ErrConflict
	}
	if err = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM attachments WHERE conversation_id=$1)", id).Scan(&active); err != nil {
		return err
	}
	if active {
		return domain.ErrConflict
	}
	for _, sql := range []string{
		"DELETE FROM response_events WHERE response_id IN (SELECT id FROM responses WHERE payload->>'conversation_id'=$1)",
		"DELETE FROM responses WHERE payload->>'conversation_id'=$1",
		"DELETE FROM conversation_summaries WHERE conversation_id=$1",
		"UPDATE conversations SET payload=jsonb_build_object('id',id) WHERE id=$1",
		"UPDATE cleanup_jobs SET status='DONE',attempts=attempts+1,last_error='',updated_at=now() WHERE conversation_id=$1",
	} {
		if _, err = tx.Exec(ctx, sql, id); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (s *Store) LatestSummary(ctx context.Context, id string) (out domain.ConversationSummary, err error) {
	var raw []byte
	err = s.pool.QueryRow(ctx, `SELECT s.payload FROM conversation_summaries s JOIN conversations c ON c.id=s.conversation_id
		WHERE c.id=$1 AND c.deleted_at IS NULL AND c.owner_id=$2 ORDER BY s.version DESC LIMIT 1`, id, domain.Owner(ctx)).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return out, domain.ErrNotFound
	}
	if err == nil {
		err = json.Unmarshal(raw, &out)
	}
	return
}

func (s *Store) SaveSummary(ctx context.Context, summary domain.ConversationSummary) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var id string
	err = tx.QueryRow(ctx, "SELECT id FROM conversations WHERE id=$1 AND owner_id=$2 AND deleted_at IS NULL FOR UPDATE",
		summary.ConversationID, domain.Owner(ctx)).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ErrNotFound
	}
	if err != nil {
		return err
	}
	raw, err := json.Marshal(summary)
	if err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, "INSERT INTO conversation_summaries(conversation_id,version,payload) VALUES ($1,$2,$3)",
		summary.ConversationID, summary.Version, raw); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
