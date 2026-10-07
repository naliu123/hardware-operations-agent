package domain

import (
	"context"
	"time"
)

const (
	AttachmentFileLimit    int64 = 50_000_000
	AttachmentMessageLimit int64 = 100_000_000
	AttachmentMessageCount       = 5
	AttachmentPDFPageLimit       = 200
)

type AttachmentGap struct {
	Page int    `json:"page,omitempty"`
	Code string `json:"code"`
	Text string `json:"text"`
}

type TextChunk struct {
	Page        int   `json:"page"`
	StartLine   int   `json:"start_line"`
	EndLine     int   `json:"end_line"`
	StartOffset int64 `json:"start_offset"`
	EndOffset   int64 `json:"end_offset"`
}

type Attachment struct {
	ID             string          `json:"id"`
	OwnerID        string          `json:"-"`
	ConversationID string          `json:"conversation_id"`
	Name           string          `json:"name"`
	MediaType      string          `json:"media_type"`
	SHA256         string          `json:"sha256"`
	Bytes          int64           `json:"bytes"`
	Status         string          `json:"status"`
	Width          int             `json:"width,omitempty"`
	Height         int             `json:"height,omitempty"`
	PageCount      int             `json:"page_count,omitempty"`
	LineCount      int             `json:"line_count,omitempty"`
	AvailablePages []int           `json:"available_pages,omitempty"`
	AvailableLines []int           `json:"available_lines,omitempty"`
	Gaps           []AttachmentGap `json:"gaps,omitempty"`
	StorageKey     string          `json:"-"`
	ParseID        string          `json:"-"`
	ParseHash      string          `json:"-"`
	Version        int64           `json:"state_version"`
	CreatedAt      time.Time       `json:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at"`
	TextChunks     []TextChunk     `json:"text_chunks,omitempty"`
}

type AttachmentReference struct {
	ID             string          `json:"id"`
	Name           string          `json:"name"`
	MediaType      string          `json:"media_type"`
	SHA256         string          `json:"sha256"`
	Bytes          int64           `json:"bytes"`
	Status         string          `json:"status"`
	PageCount      int             `json:"page_count,omitempty"`
	LineCount      int             `json:"line_count,omitempty"`
	AvailablePages []int           `json:"available_pages,omitempty"`
	AvailableLines []int           `json:"available_lines,omitempty"`
	Gaps           []AttachmentGap `json:"gaps,omitempty"`
}

func (a Attachment) Reference() AttachmentReference {
	return AttachmentReference{
		ID: a.ID, Name: a.Name, MediaType: a.MediaType, SHA256: a.SHA256, Bytes: a.Bytes,
		Status: a.Status, PageCount: a.PageCount, LineCount: a.LineCount,
		AvailablePages: append([]int{}, a.AvailablePages...),
		AvailableLines: append([]int{}, a.AvailableLines...),
		Gaps:           append([]AttachmentGap{}, a.Gaps...),
	}
}

type AttachmentTable struct {
	Index  int        `json:"index"`
	Rows   [][]string `json:"rows"`
	SHA256 string     `json:"sha256"`
}

type AttachmentAsset struct {
	ID           string `json:"id"`
	AttachmentID string `json:"attachment_id"`
	Page         int    `json:"page"`
	Kind         string `json:"kind"`
	Name         string `json:"name"`
	MediaType    string `json:"media_type"`
	SHA256       string `json:"sha256"`
	Bytes        int64  `json:"bytes"`
	Width        int    `json:"width,omitempty"`
	Height       int    `json:"height,omitempty"`
	StorageKey   string `json:"-"`
}

type AttachmentPage struct {
	AttachmentID string            `json:"attachment_id"`
	Page         int               `json:"page"`
	Status       string            `json:"status"`
	Text         string            `json:"text,omitempty"`
	TextSHA256   string            `json:"text_sha256,omitempty"`
	StartLine    int               `json:"start_line,omitempty"`
	EndLine      int               `json:"end_line,omitempty"`
	Tables       []AttachmentTable `json:"tables,omitempty"`
	Preview      *AttachmentAsset  `json:"preview,omitempty"`
	Images       []AttachmentAsset `json:"images,omitempty"`
	Error        *Failure          `json:"error,omitempty"`
}

type SourceRef struct {
	SourceKind      string   `json:"source_kind"`
	SourceID        string   `json:"source_id,omitempty"`
	AttachmentID    string   `json:"attachment_id,omitempty"`
	AttachmentHash  string   `json:"attachment_hash,omitempty"`
	ExecutionID     string   `json:"execution_id,omitempty"`
	CodeSHA256      string   `json:"code_sha256,omitempty"`
	InputSHA256     []string `json:"input_sha256,omitempty"`
	Page            int      `json:"page,omitempty"`
	StartLine       int      `json:"start_line,omitempty"`
	EndLine         int      `json:"end_line,omitempty"`
	ContentSHA256   string   `json:"content_sha256"`
	CoverageStatus  string   `json:"coverage_status"`
	UnparsedSummary string   `json:"unparsed_summary,omitempty"`
}

type AttachmentPageResult struct {
	Attachment Attachment     `json:"attachment"`
	Page       AttachmentPage `json:"page"`
	Source     SourceRef      `json:"source"`
}

type AttachmentReadResult struct {
	Status     string            `json:"status"`
	Name       string            `json:"name"`
	MediaType  string            `json:"media_type"`
	Text       string            `json:"text,omitempty"`
	Tables     []AttachmentTable `json:"tables,omitempty"`
	Visual     string            `json:"visual_description,omitempty"`
	Source     *SourceRef        `json:"source,omitempty"`
	Gaps       []AttachmentGap   `json:"gaps,omitempty"`
	ModelUsage *ModelUsage       `json:"model_usage,omitempty"`
}

type AttachmentParseJob struct {
	Attachment Attachment
	Assets     []AttachmentAsset
	Status     string
	Version    int64
	Deleted    bool
	Deadline   time.Time
}

type AttachmentParseResult struct {
	Attachment Attachment
	Pages      []AttachmentPage
	Assets     []AttachmentAsset
}

type AttachmentRepository interface {
	CreateAttachment(context.Context, Attachment, int) error
	GetAttachment(context.Context, string) (Attachment, error)
	GetAttachmentPage(context.Context, string, int) (AttachmentPage, error)
	GetAttachmentAsset(context.Context, string, string) (AttachmentAsset, error)
	PendingAttachmentParses(context.Context) ([]AttachmentParseJob, error)
	StartAttachmentParse(context.Context, string, int64) error
	FinishAttachmentParse(context.Context, AttachmentParseResult, int64) error
	DeleteAttachmentData(context.Context, string) error
}

func AttachmentTerminal(status string) bool {
	return status == "READY" || status == "PARTIAL" || status == "FAILED" || status == "UNSUPPORTED"
}
