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

type rowQuerier interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func findRequest(ctx context.Context, db rowQuerier, conversationID, key, hash string) (r domain.Response, err error) {
	if key == "" {
		return r, domain.ErrNotFound
	}
	var raw []byte
	err = db.QueryRow(ctx, `SELECT r.payload FROM responses r JOIN conversations c ON c.id=r.payload->>'conversation_id'
		WHERE c.id=$1 AND c.deleted_at IS NULL AND r.payload->>'request_key'=$2 AND ($3='' OR c.owner_id=$3)`, conversationID, key, domain.Owner(ctx)).Scan(&raw)
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
	err = tx.QueryRow(ctx, "SELECT payload FROM conversations WHERE id=$1 AND deleted_at IS NULL AND ($2='' OR owner_id=$2) FOR UPDATE",
		r.ConversationID, domain.Owner(ctx)).Scan(&raw)
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
	if s.users {
		var active bool
		err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM responses WHERE payload->>'conversation_id'=$1
			AND payload->>'status' IN ('QUEUED','RUNNING','CANCELING'))`, c.ID).Scan(&active)
		if err != nil {
			return r, err
		}
		if active {
			return r, fmt.Errorf("%w: 当前会话正在处理，请等待或停止后再发送。", domain.ErrConflict)
		}
		if r.RetryOf != "" {
			var original domain.Response
			var previous []byte
			err = tx.QueryRow(ctx, `SELECT payload FROM responses WHERE id=$1 AND payload->>'conversation_id'=$2`, r.RetryOf, c.ID).Scan(&previous)
			if errors.Is(err, pgx.ErrNoRows) {
				return r, domain.ErrNotFound
			}
			if err != nil {
				return r, err
			}
			if err = json.Unmarshal(previous, &original); err != nil {
				return r, err
			}
			if (original.Status != "FAILED" && original.Status != "CANCELED" && original.Status != "INTERRUPTED") ||
				original.Question != r.Question || !sameAttachmentIDs(original.Attachments, r.Attachments) {
				return r, fmt.Errorf("%w: only an unchanged failed or interrupted question can be retried", domain.ErrInvalid)
			}
		}
		r.Workbench = true
	}
	if err = validateMessageAttachments(ctx, tx, &r, c.Owner); err != nil {
		return r, err
	}
	c.LastSequence++
	r.Sequence = c.LastSequence
	c.StateVersion++
	c.UpdatedAt = time.Now().UTC()
	if c.TitleSource != "MANUAL" && c.LastSequence == 1 {
		// Deterministic naming needs no model call and cannot block chatting.
		value := strings.Join(strings.Fields(r.Question), " ")
		if value == "" && len(r.Attachments) > 0 {
			value = r.Attachments[0].Name
		}
		title := []rune(value)
		if len(title) > 28 {
			title = append(title[:28], '…')
		}
		if len(title) > 0 {
			c.Title, c.TitleSource = string(title), "AUTO"
		}
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
	for i, attachment := range r.Attachments {
		if _, err = tx.Exec(ctx, `INSERT INTO message_attachments(response_id,attachment_id,ordinal)
			VALUES ($1,$2,$3)`, r.ID, attachment.ID, i); err != nil {
			return r, err
		}
	}
	return r, tx.Commit(ctx)
}

func sameAttachmentIDs(left, right []domain.AttachmentReference) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i].ID != right[i].ID {
			return false
		}
	}
	return true
}

func validateMessageAttachments(ctx context.Context, tx pgx.Tx, r *domain.Response, owner string) error {
	if len(r.Attachments) > domain.AttachmentMessageCount {
		return fmt.Errorf("%w: 每条消息最多引用 5 个附件。", domain.ErrInvalid)
	}
	seen := map[string]bool{}
	var total int64
	refs := make([]domain.AttachmentReference, 0, len(r.Attachments))
	for _, requested := range r.Attachments {
		if requested.ID == "" || seen[requested.ID] {
			return fmt.Errorf("%w: attachment_ids 必须非空且不能重复。", domain.ErrInvalid)
		}
		seen[requested.ID] = true
		var raw []byte
		err := tx.QueryRow(ctx, `SELECT payload FROM attachments
			WHERE id=$1 AND owner_id=$2 AND conversation_id=$3
			AND status IN ('READY','PARTIAL') FOR SHARE`, requested.ID, owner, r.ConversationID).Scan(&raw)
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrNotFound
		}
		if err != nil {
			return err
		}
		var attachment domain.Attachment
		if err = json.Unmarshal(raw, &attachment); err != nil {
			return err
		}
		total += attachment.Bytes
		if total > domain.AttachmentMessageLimit {
			return fmt.Errorf("%w: 每条消息附件总大小不能超过 100,000,000 字节。", domain.ErrInvalid)
		}
		refs = append(refs, attachment.Reference())
	}
	r.Attachments = refs
	return nil
}

func saveResponse(ctx context.Context, tx pgx.Tx, r *domain.Response) error {
	if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1,0))", "response:"+r.ID); err != nil {
		return err
	}
	var previousRaw []byte
	err := tx.QueryRow(ctx, "SELECT payload FROM responses WHERE id=$1 FOR UPDATE", r.ID).Scan(&previousRaw)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	var saved domain.Response
	if len(previousRaw) > 0 {
		if err = json.Unmarshal(previousRaw, &saved); err != nil {
			return err
		}
	}
	previous := saved.Status
	// The worker began with an older snapshot. Stream writes own reasoning.
	r.Reasoning = saved.Reasoning
	if r.Workbench && previous != "" && domain.ResponseTerminal(previous) {
		return domain.ErrConflict
	}
	if r.Workbench && previous == "CANCELING" && r.Status != "CANCELED" && r.Status != "INTERRUPTED" && r.Status != "CANCELING" {
		return domain.ErrConflict
	}
	if (r.Status == "ANSWERED" || r.Status == "PARTIAL") && r.DeviceContext != nil {
		var snapshotID string
		err = tx.QueryRow(ctx, "SELECT payload->>'snapshot_id' FROM device_snapshots WHERE id=$1 FOR UPDATE", r.DeviceContext.DeviceID).Scan(&snapshotID)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		r.CheckDeviceSnapshot(snapshotID)
	}
	sequence, _, activeDraft, err := responseEventState(ctx, tx, r.ID)
	if err != nil {
		return err
	}
	if r.Reasoning != nil && r.Reasoning.Status == "RUNNING" &&
		(r.Status == "CANCELING" || domain.ResponseTerminal(r.Status)) {
		sequence++
		r.Reasoning.Status, r.Reasoning.EventID = "INTERRUPTED", sequence
		if err = insertResponseEvent(ctx, tx,
			domain.NewReasoningEvent(*r, sequence, "", r.Reasoning.Status)); err != nil {
			return err
		}
	}
	raw, err := json.Marshal(r)
	if err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO responses (id,payload) VALUES ($1,$2)
		ON CONFLICT (id) DO UPDATE SET payload=EXCLUDED.payload`, r.ID, raw); err != nil {
		return err
	}
	if activeDraft > 0 && (r.Status == "CANCELING" || domain.ResponseTerminal(r.Status)) {
		sequence++
		event := domain.NewDraftEvent("draft_retracted", *r, activeDraft, sequence, "", domain.DraftRetractionReason(*r))
		if err = insertResponseEvent(ctx, tx, event); err != nil {
			return err
		}
	}
	for _, event := range domain.NewResponseEvents(previous, *r, sequence) {
		if err = insertResponseEvent(ctx, tx, event); err != nil {
			return err
		}
	}
	return nil
}

func insertResponseEvent(ctx context.Context, tx pgx.Tx, event domain.ResponseEvent) error {
	raw, err := json.Marshal(event)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, "INSERT INTO response_events (response_id,sequence,payload) VALUES ($1,$2,$3)",
		event.ResponseID, event.ID, raw)
	return err
}

func responseEventState(ctx context.Context, tx pgx.Tx, id string) (sequence, maxVersion, activeVersion int64, err error) {
	err = tx.QueryRow(ctx, `SELECT COALESCE(MAX(sequence),0),
		COALESCE(MAX(CASE WHEN payload ? 'draft_version' THEN (payload->>'draft_version')::bigint ELSE 0 END),0)
		FROM response_events WHERE response_id=$1`, id).Scan(&sequence, &maxVersion)
	if err != nil {
		return
	}
	var kind string
	err = tx.QueryRow(ctx, `SELECT payload->>'type',COALESCE((payload->>'draft_version')::bigint,0)
		FROM response_events WHERE response_id=$1
		AND payload->>'type' IN ('draft_started','draft_retracted')
		ORDER BY sequence DESC LIMIT 1`, id).Scan(&kind, &activeVersion)
	if errors.Is(err, pgx.ErrNoRows) {
		err, activeVersion = nil, 0
	} else if err == nil && kind == "draft_retracted" {
		activeVersion = 0
	}
	return
}

func (s *Store) StartResponseDraft(ctx context.Context, id string) (version int64, err error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)
	var raw []byte
	if err = tx.QueryRow(ctx, "SELECT payload FROM responses WHERE id=$1", id).Scan(&raw); errors.Is(err, pgx.ErrNoRows) {
		return 0, domain.ErrNotFound
	} else if err != nil {
		return 0, err
	}
	var response domain.Response
	if err = json.Unmarshal(raw, &response); err != nil {
		return 0, err
	}
	var conversationID string
	err = tx.QueryRow(ctx, `SELECT id FROM conversations WHERE id=$1 AND deleted_at IS NULL
		AND ($2='' OR owner_id=$2) FOR UPDATE`, response.ConversationID, domain.Owner(ctx)).Scan(&conversationID)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, domain.ErrNotFound
	}
	if err != nil {
		return 0, err
	}
	if err = tx.QueryRow(ctx, "SELECT payload FROM responses WHERE id=$1 FOR UPDATE", id).Scan(&raw); err != nil {
		return 0, err
	}
	if err = json.Unmarshal(raw, &response); err != nil {
		return 0, err
	}
	if response.Status != "RUNNING" {
		return 0, domain.ErrConflict
	}
	sequence, maxVersion, activeVersion, err := responseEventState(ctx, tx, id)
	if err != nil {
		return 0, err
	}
	if activeVersion > 0 {
		sequence++
		reason := &domain.Failure{Code: "SUPERSEDED", Message: "模型正在修正上一版草稿。"}
		if err = insertResponseEvent(ctx, tx,
			domain.NewDraftEvent("draft_retracted", response, activeVersion, sequence, "", reason)); err != nil {
			return 0, err
		}
	}
	version = maxVersion + 1
	sequence++
	if err = insertResponseEvent(ctx, tx,
		domain.NewDraftEvent("draft_started", response, version, sequence, "", nil)); err != nil {
		return 0, err
	}
	return version, tx.Commit(ctx)
}

func (s *Store) AppendResponseDelta(ctx context.Context, id string, version int64, delta string) (event domain.ResponseEvent, err error) {
	if version < 1 || delta == "" || len(delta) > 16*1024 {
		return event, domain.ErrInvalid
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return event, err
	}
	defer tx.Rollback(ctx)
	var raw []byte
	if err = tx.QueryRow(ctx, "SELECT payload FROM responses WHERE id=$1", id).Scan(&raw); errors.Is(err, pgx.ErrNoRows) {
		return event, domain.ErrNotFound
	} else if err != nil {
		return event, err
	}
	var response domain.Response
	if err = json.Unmarshal(raw, &response); err != nil {
		return event, err
	}
	var conversationID string
	err = tx.QueryRow(ctx, `SELECT id FROM conversations WHERE id=$1 AND deleted_at IS NULL
		AND ($2='' OR owner_id=$2) FOR UPDATE`, response.ConversationID, domain.Owner(ctx)).Scan(&conversationID)
	if errors.Is(err, pgx.ErrNoRows) {
		return event, domain.ErrNotFound
	}
	if err != nil {
		return event, err
	}
	if err = tx.QueryRow(ctx, "SELECT payload FROM responses WHERE id=$1 FOR UPDATE", id).Scan(&raw); err != nil {
		return event, err
	}
	if err = json.Unmarshal(raw, &response); err != nil {
		return event, err
	}
	if response.Status != "RUNNING" {
		return event, domain.ErrConflict
	}
	sequence, _, activeVersion, err := responseEventState(ctx, tx, id)
	if err != nil {
		return event, err
	}
	if activeVersion != version {
		return event, domain.ErrConflict
	}
	var bytes int
	if err = tx.QueryRow(ctx, `SELECT COALESCE(SUM(octet_length(payload->>'delta')),0)
		FROM response_events WHERE response_id=$1 AND COALESCE((payload->>'draft_version')::bigint,0)=$2`,
		id, version).Scan(&bytes); err != nil {
		return event, err
	}
	if bytes+len(delta) > 512*1024 {
		return event, domain.ErrResourceExhausted
	}
	event = domain.NewDraftEvent("answer_delta", response, version, sequence+1, delta, nil)
	if err = insertResponseEvent(ctx, tx, event); err != nil {
		return event, err
	}
	return event, tx.Commit(ctx)
}

func (s *Store) AppendToolProgress(ctx context.Context, id string, execution domain.PythonExecution) (event domain.ResponseEvent, err error) {
	if execution.ID == "" || execution.ResponseID != id ||
		len(execution.Result.Stdout)+len(execution.Result.Stderr) > 128*1024 {
		return event, domain.ErrInvalid
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return event, err
	}
	defer tx.Rollback(ctx)
	var raw []byte
	if err = tx.QueryRow(ctx, "SELECT payload FROM responses WHERE id=$1", id).Scan(&raw); errors.Is(err, pgx.ErrNoRows) {
		return event, domain.ErrNotFound
	} else if err != nil {
		return event, err
	}
	var response domain.Response
	if err = json.Unmarshal(raw, &response); err != nil {
		return event, err
	}
	var conversationID string
	err = tx.QueryRow(ctx, `SELECT id FROM conversations WHERE id=$1 AND deleted_at IS NULL
		AND ($2='' OR owner_id=$2) FOR UPDATE`, response.ConversationID, domain.Owner(ctx)).Scan(&conversationID)
	if errors.Is(err, pgx.ErrNoRows) {
		return event, domain.ErrNotFound
	}
	if err != nil {
		return event, err
	}
	if err = tx.QueryRow(ctx, "SELECT payload FROM responses WHERE id=$1 FOR UPDATE", id).Scan(&raw); err != nil {
		return event, err
	}
	if err = json.Unmarshal(raw, &response); err != nil {
		return event, err
	}
	if response.Status != "RUNNING" && response.Status != "CANCELING" {
		return event, domain.ErrConflict
	}
	var savedRaw []byte
	var owner string
	err = tx.QueryRow(ctx, `SELECT payload,owner_id FROM python_executions
		WHERE id=$1 AND response_id=$2 AND owner_id=$3`, execution.ID, id, domain.Owner(ctx)).Scan(&savedRaw, &owner)
	if errors.Is(err, pgx.ErrNoRows) {
		return event, domain.ErrNotFound
	}
	if err != nil {
		return event, err
	}
	var saved domain.PythonExecution
	if err = json.Unmarshal(savedRaw, &saved); err != nil {
		return event, err
	}
	if saved.Status != execution.Status || saved.Version != execution.Version ||
		saved.CodeSHA256 != execution.CodeSHA256 || owner != domain.Owner(ctx) {
		return event, domain.ErrConflict
	}
	sequence, _, _, err := responseEventState(ctx, tx, id)
	if err != nil {
		return event, err
	}
	event = domain.NewToolProgressEvent(response, execution, sequence+1)
	if err = insertResponseEvent(ctx, tx, event); err != nil {
		return event, err
	}
	return event, tx.Commit(ctx)
}

func (s *Store) ResponseEvents(ctx context.Context, id string, after int64) ([]domain.ResponseEvent, error) {
	if _, err := s.GetResponse(ctx, id); err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, `SELECT e.payload FROM response_events e JOIN responses r ON r.id=e.response_id
		JOIN conversations c ON c.id=r.payload->>'conversation_id'
		WHERE e.response_id=$1 AND c.deleted_at IS NULL AND e.sequence>$2 AND ($3='' OR c.owner_id=$3) ORDER BY e.sequence`, id, after, domain.Owner(ctx))
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
