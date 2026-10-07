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

func (s *Store) CreateAttachment(ctx context.Context, a domain.Attachment, queueLimit int) error {
	if domain.Owner(ctx) == "" || a.OwnerID != domain.Owner(ctx) || a.ID == "" ||
		a.StorageKey == "" || a.Bytes < 0 || a.Bytes > domain.AttachmentFileLimit ||
		a.Status != "UPLOADED" || queueLimit < 1 {
		return domain.ErrInvalid
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var id string
	err = tx.QueryRow(ctx, `SELECT id FROM conversations WHERE id=$1 AND owner_id=$2
		AND deleted_at IS NULL FOR UPDATE`, a.ConversationID, a.OwnerID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ErrNotFound
	}
	if err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtext('hwops:attachment-queue'))"); err != nil {
		return err
	}
	var pending int
	if err = tx.QueryRow(ctx, "SELECT count(*) FROM attachment_parse_jobs WHERE status IN ('QUEUED','PROCESSING')").Scan(&pending); err != nil {
		return err
	}
	if pending >= queueLimit {
		return fmt.Errorf("%w: 附件解析队列已满，请稍后重试。", domain.ErrConflict)
	}
	a.Version = 1
	a.CreatedAt = time.Now().UTC()
	a.UpdatedAt = a.CreatedAt
	raw, err := json.Marshal(a)
	if err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO attachments(
		id,owner_id,conversation_id,storage_key,status,version,created_at,updated_at,payload)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$7,$8)`, a.ID, a.OwnerID, a.ConversationID,
		a.StorageKey, a.Status, a.Version, a.CreatedAt, raw); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO attachment_parse_jobs(
		attachment_id,status,version,deadline,updated_at) VALUES ($1,'QUEUED',1,$2,$3)`,
		a.ID, a.CreatedAt.Add(5*time.Minute), a.CreatedAt); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) GetAttachment(ctx context.Context, id string) (a domain.Attachment, err error) {
	if domain.Owner(ctx) == "" {
		return a, domain.ErrNotFound
	}
	var raw []byte
	err = s.pool.QueryRow(ctx, `SELECT a.payload,a.storage_key,a.owner_id
		FROM attachments a JOIN conversations c ON c.id=a.conversation_id
		WHERE a.id=$1 AND a.owner_id=$2 AND c.deleted_at IS NULL`,
		id, domain.Owner(ctx)).Scan(&raw, &a.StorageKey, &a.OwnerID)
	if errors.Is(err, pgx.ErrNoRows) {
		return a, domain.ErrNotFound
	}
	if err == nil {
		err = json.Unmarshal(raw, &a)
	}
	return
}

func (s *Store) GetAttachmentPage(ctx context.Context, id string, page int) (out domain.AttachmentPage, err error) {
	if domain.Owner(ctx) == "" || page < 1 {
		return out, domain.ErrNotFound
	}
	var raw []byte
	err = s.pool.QueryRow(ctx, `SELECT p.payload FROM attachment_pages p
		JOIN attachments a ON a.id=p.attachment_id JOIN conversations c ON c.id=a.conversation_id
		WHERE p.attachment_id=$1 AND p.page_number=$2 AND a.owner_id=$3 AND c.deleted_at IS NULL`,
		id, page, domain.Owner(ctx)).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return out, domain.ErrNotFound
	}
	if err == nil {
		err = json.Unmarshal(raw, &out)
	}
	return
}

func (s *Store) GetAttachmentAsset(ctx context.Context, attachmentID, assetID string) (out domain.AttachmentAsset, err error) {
	if domain.Owner(ctx) == "" {
		return out, domain.ErrNotFound
	}
	var raw []byte
	err = s.pool.QueryRow(ctx, `SELECT x.payload,x.storage_key FROM attachment_assets x
		JOIN attachments a ON a.id=x.attachment_id JOIN conversations c ON c.id=a.conversation_id
		WHERE x.id=$1 AND x.attachment_id=$2 AND a.owner_id=$3 AND c.deleted_at IS NULL`,
		assetID, attachmentID, domain.Owner(ctx)).Scan(&raw, &out.StorageKey)
	if errors.Is(err, pgx.ErrNoRows) {
		return out, domain.ErrNotFound
	}
	if err == nil {
		err = json.Unmarshal(raw, &out)
	}
	return
}

func (s *Store) PendingAttachmentParses(ctx context.Context) ([]domain.AttachmentParseJob, error) {
	rows, err := s.pool.Query(ctx, `SELECT a.payload,a.storage_key,a.owner_id,j.status,j.version,j.deadline,
		c.deleted_at IS NOT NULL FROM attachment_parse_jobs j
		JOIN attachments a ON a.id=j.attachment_id JOIN conversations c ON c.id=a.conversation_id
		WHERE j.status IN ('QUEUED','PROCESSING') OR c.deleted_at IS NOT NULL
		ORDER BY j.updated_at,j.attachment_id LIMIT 100`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.AttachmentParseJob
	for rows.Next() {
		var raw []byte
		var job domain.AttachmentParseJob
		if err = rows.Scan(&raw, &job.Attachment.StorageKey, &job.Attachment.OwnerID,
			&job.Status, &job.Version, &job.Deadline, &job.Deleted); err != nil {
			return nil, err
		}
		if err = json.Unmarshal(raw, &job.Attachment); err != nil {
			return nil, err
		}
		out = append(out, job)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	for i := range out {
		assets, readErr := s.attachmentAssets(ctx, out[i].Attachment.ID)
		if readErr != nil {
			return nil, readErr
		}
		out[i].Assets = assets
	}
	return out, nil
}

func (s *Store) attachmentAssets(ctx context.Context, id string) ([]domain.AttachmentAsset, error) {
	rows, err := s.pool.Query(ctx, "SELECT payload,storage_key FROM attachment_assets WHERE attachment_id=$1 ORDER BY id", id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.AttachmentAsset
	for rows.Next() {
		var raw []byte
		var asset domain.AttachmentAsset
		if err = rows.Scan(&raw, &asset.StorageKey); err != nil {
			return nil, err
		}
		if err = json.Unmarshal(raw, &asset); err != nil {
			return nil, err
		}
		out = append(out, asset)
	}
	return out, rows.Err()
}

func (s *Store) StartAttachmentParse(ctx context.Context, id string, version int64) error {
	now := time.Now().UTC()
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var raw []byte
	err = tx.QueryRow(ctx, `SELECT a.payload FROM attachments a JOIN conversations c ON c.id=a.conversation_id
		WHERE a.id=$1 AND a.version=$2 AND c.deleted_at IS NULL FOR UPDATE OF a`,
		id, version).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ErrNotFound
	}
	if err != nil {
		return err
	}
	var a domain.Attachment
	if err = json.Unmarshal(raw, &a); err != nil {
		return err
	}
	if a.Status != "UPLOADED" {
		return domain.ErrConflict
	}
	a.Status, a.Version, a.UpdatedAt = "PROCESSING", version+1, now
	raw, err = json.Marshal(a)
	if err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE attachments SET status='PROCESSING',version=$2,updated_at=$3,payload=$4
		WHERE id=$1`, id, a.Version, now, raw); err != nil {
		return err
	}
	tag, err := tx.Exec(ctx, `UPDATE attachment_parse_jobs SET status='PROCESSING',version=version+1,updated_at=$3
		WHERE attachment_id=$1 AND version=$2 AND status='QUEUED'`, id, version, now)
	if err != nil || tag.RowsAffected() != 1 {
		return domain.ErrConflict
	}
	return tx.Commit(ctx)
}

func (s *Store) FinishAttachmentParse(ctx context.Context, result domain.AttachmentParseResult, version int64) error {
	a := result.Attachment
	if !domain.AttachmentTerminal(a.Status) || a.Version != version {
		return domain.ErrInvalid
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var raw []byte
	var storage, owner string
	err = tx.QueryRow(ctx, `SELECT a.payload,a.storage_key,a.owner_id FROM attachments a
		JOIN conversations c ON c.id=a.conversation_id
		WHERE a.id=$1 AND a.version=$2 AND c.deleted_at IS NULL FOR UPDATE OF a`,
		a.ID, version).Scan(&raw, &storage, &owner)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ErrNotFound
	}
	if err != nil {
		return err
	}
	var saved domain.Attachment
	if err = json.Unmarshal(raw, &saved); err != nil {
		return err
	}
	if saved.Status != "PROCESSING" || saved.SHA256 != a.SHA256 || saved.Bytes != a.Bytes ||
		saved.ConversationID != a.ConversationID {
		return domain.ErrConflict
	}
	a.OwnerID, a.StorageKey, a.Version, a.UpdatedAt = owner, storage, version+1, time.Now().UTC()
	for _, page := range result.Pages {
		if page.AttachmentID != a.ID || page.Page < 1 {
			return domain.ErrInvalid
		}
		raw, err = json.Marshal(page)
		if err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO attachment_pages(attachment_id,page_number,status,payload)
			VALUES ($1,$2,$3,$4)`, a.ID, page.Page, page.Status, raw); err != nil {
			return err
		}
	}
	for _, asset := range result.Assets {
		if asset.AttachmentID != a.ID || asset.Page < 1 || asset.StorageKey == "" {
			return domain.ErrInvalid
		}
		raw, err = json.Marshal(asset)
		if err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO attachment_assets(id,attachment_id,page_number,storage_key,payload)
			VALUES ($1,$2,$3,$4,$5)`, asset.ID, a.ID, asset.Page, asset.StorageKey, raw); err != nil {
			return err
		}
	}
	raw, err = json.Marshal(a)
	if err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE attachments SET status=$2,version=$3,updated_at=$4,payload=$5
		WHERE id=$1`, a.ID, a.Status, a.Version, a.UpdatedAt, raw); err != nil {
		return err
	}
	tag, err := tx.Exec(ctx, `UPDATE attachment_parse_jobs SET status='DONE',version=version+1,updated_at=$3
		WHERE attachment_id=$1 AND version=$2 AND status='PROCESSING'`, a.ID, version, a.UpdatedAt)
	if err != nil || tag.RowsAffected() != 1 {
		return domain.ErrConflict
	}
	return tx.Commit(ctx)
}

func (s *Store) DeleteAttachmentData(ctx context.Context, id string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var found string
	err = tx.QueryRow(ctx, `SELECT a.id FROM attachments a JOIN conversations c ON c.id=a.conversation_id
		WHERE a.id=$1 AND c.deleted_at IS NOT NULL FOR UPDATE OF a,c`, id).Scan(&found)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ErrNotFound
	}
	if err != nil {
		return err
	}
	for _, query := range []string{
		"DELETE FROM message_attachments WHERE attachment_id=$1",
		"DELETE FROM attachment_assets WHERE attachment_id=$1",
		"DELETE FROM attachment_pages WHERE attachment_id=$1",
		"DELETE FROM attachment_parse_jobs WHERE attachment_id=$1",
		"DELETE FROM attachments WHERE id=$1",
	} {
		if _, err = tx.Exec(ctx, query, id); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}
