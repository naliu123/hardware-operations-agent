// Package attachments owns private upload validation, durable parsing and
// bounded source reads. Browser/model inputs never choose parser code, images,
// runtime settings, storage keys or ownership.
package attachments

import (
	"archive/zip"
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"mime"
	"net/http"
	"os"
	"path"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode"

	"hwops/internal/blobstore"
	"hwops/internal/domain"
	"hwops/internal/sandbox"
)

const (
	parseQueueLimit = 20
	maxZipEntries   = 601
	maxPageText     = 2_000_000
)

var assetPath = regexp.MustCompile(`^assets/[A-Za-z0-9][A-Za-z0-9_.-]{0,179}$`)

type Service struct {
	repo   domain.AttachmentRepository
	runner sandbox.Runner
	files  *blobstore.Store
	cap    sandbox.Capability
	ctx    context.Context
	cancel context.CancelFunc
	wake   chan struct{}
	wg     sync.WaitGroup
}

func New(repo domain.AttachmentRepository, runner sandbox.Runner, files *blobstore.Store) (*Service, error) {
	if repo == nil || runner == nil || files == nil {
		return nil, domain.ErrUnavailable
	}
	ctx, cancel := context.WithCancel(context.Background())
	probe, stop := context.WithTimeout(ctx, 20*time.Second)
	capability, err := runner.Capability(probe)
	stop()
	if err != nil || !capability.Ready || capability.Runtime == "" ||
		capability.Budget != domain.PythonLimits() ||
		capability.Packages["PyMuPDF"] != "1.26.4" ||
		capability.Packages["Pillow"] != "11.2.1" {
		cancel()
		return nil, domain.ErrUnavailable
	}
	s := &Service{
		repo: repo, runner: runner, files: files, cap: capability,
		ctx: ctx, cancel: cancel, wake: make(chan struct{}, 1),
	}
	s.wg.Add(1)
	go s.work()
	return s, nil
}

func (s *Service) Close() {
	s.cancel()
	s.wg.Wait()
}

func (s *Service) notify() {
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

func cleanName(value string) (string, error) {
	value = path.Base(strings.ReplaceAll(strings.TrimSpace(value), `\`, "/"))
	value = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, value)
	if value == "" || value == "." || len([]byte(value)) > 255 {
		return "", fmt.Errorf("%w: 附件文件名无效。", domain.ErrInvalid)
	}
	return value, nil
}

func (s *Service) Upload(ctx context.Context, conversationID, name string, input io.Reader) (domain.Attachment, error) {
	if domain.Owner(ctx) == "" {
		return domain.Attachment{}, domain.ErrNotFound
	}
	name, err := cleanName(name)
	if err != nil {
		return domain.Attachment{}, err
	}
	id, key := rand.Text(), rand.Text()
	n, hash, err := s.files.Put(key, input, domain.AttachmentFileLimit, "")
	if err != nil {
		if errors.Is(err, blobstore.ErrCapacity) {
			return domain.Attachment{}, fmt.Errorf("%w: 私有文件存储空间不足。", domain.ErrResourceExhausted)
		}
		return domain.Attachment{}, fmt.Errorf("%w: 附件超过 50,000,000 字节或上传不完整。", domain.ErrInvalid)
	}
	discard := true
	defer func() {
		if discard {
			_ = s.files.Delete(key)
		}
	}()
	file, err := s.files.Open(key)
	if err != nil {
		return domain.Attachment{}, err
	}
	mediaType, err := inspect(file, n)
	file.Close()
	if err != nil {
		return domain.Attachment{}, err
	}
	attachment := domain.Attachment{
		ID: id, OwnerID: domain.Owner(ctx), ConversationID: conversationID, Name: name,
		MediaType: mediaType, SHA256: hash, Bytes: n, Status: "UPLOADED",
		StorageKey: key, Gaps: []domain.AttachmentGap{},
	}
	if err = s.repo.CreateAttachment(ctx, attachment, parseQueueLimit); err != nil {
		return domain.Attachment{}, err
	}
	discard = false
	s.notify()
	return s.repo.GetAttachment(ctx, id)
}

func inspect(file *os.File, size int64) (string, error) {
	if size < 1 {
		return "", fmt.Errorf("%w: 空文件不受支持。", domain.ErrInvalid)
	}
	var header [512]byte
	n, err := io.ReadFull(file, header[:])
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) {
		return "", err
	}
	head := header[:n]
	if _, err = file.Seek(0, io.SeekStart); err != nil {
		return "", err
	}
	switch {
	case bytes.HasPrefix(head, []byte("\x89PNG\r\n\x1a\n")):
		if err = inspectRaster(file); err != nil {
			return "", err
		}
		return "image/png", nil
	case bytes.HasPrefix(head, []byte("\xff\xd8\xff")):
		if err = inspectRaster(file); err != nil {
			return "", err
		}
		return "image/jpeg", nil
	case len(head) >= 12 && string(head[:4]) == "RIFF" && string(head[8:12]) == "WEBP":
		return "image/webp", nil
	case bytes.HasPrefix(head, []byte("%PDF-")):
		return "application/pdf", nil
	}
	if _, err = file.Seek(0, io.SeekStart); err != nil {
		return "", err
	}
	reader := bufio.NewReaderSize(file, 64<<10)
	for {
		r, width, readErr := reader.ReadRune()
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			return "", fmt.Errorf("%w: 仅支持严格 UTF-8 文本日志。", domain.ErrInvalid)
		}
		if r == '\x00' || (r == unicode.ReplacementChar && width == 1) {
			return "", fmt.Errorf("%w: 仅支持严格 UTF-8 文本日志。", domain.ErrInvalid)
		}
	}
	return "text/plain; charset=utf-8", nil
}

func inspectRaster(file *os.File) error {
	config, _, err := image.DecodeConfig(file)
	if err != nil || config.Width < 1 || config.Height < 1 || int64(config.Width)*int64(config.Height) > 40_000_000 {
		return fmt.Errorf("%w: 图片损坏或展开尺寸超过 40,000,000 像素。", domain.ErrInvalid)
	}
	return nil
}

func (s *Service) Get(ctx context.Context, id string) (domain.Attachment, error) {
	return s.repo.GetAttachment(ctx, id)
}

func (s *Service) Content(ctx context.Context, id string) (domain.Attachment, *os.File, error) {
	attachment, err := s.repo.GetAttachment(ctx, id)
	if err != nil {
		return attachment, nil, err
	}
	file, err := s.files.Open(attachment.StorageKey)
	return attachment, file, err
}

func (s *Service) Asset(ctx context.Context, attachmentID, assetID string) (domain.AttachmentAsset, *os.File, error) {
	asset, err := s.repo.GetAttachmentAsset(ctx, attachmentID, assetID)
	if err != nil {
		return asset, nil, err
	}
	file, err := s.files.Open(asset.StorageKey)
	return asset, file, err
}

func (s *Service) Page(ctx context.Context, id string, pageNumber int) (domain.AttachmentPageResult, error) {
	attachment, err := s.repo.GetAttachment(ctx, id)
	if err != nil {
		return domain.AttachmentPageResult{}, err
	}
	if pageNumber < 1 || !domain.AttachmentTerminal(attachment.Status) {
		return domain.AttachmentPageResult{}, domain.ErrNotFound
	}
	var page domain.AttachmentPage
	switch attachment.MediaType {
	case "text/plain; charset=utf-8":
		if pageNumber > len(attachment.TextChunks) {
			return domain.AttachmentPageResult{}, domain.ErrNotFound
		}
		chunk := attachment.TextChunks[pageNumber-1]
		if chunk.EndOffset < chunk.StartOffset || chunk.EndOffset-chunk.StartOffset > 2_000_000 {
			return domain.AttachmentPageResult{}, errors.New("stored text index is invalid")
		}
		file, err := s.files.Open(attachment.StorageKey)
		if err != nil {
			return domain.AttachmentPageResult{}, err
		}
		defer file.Close()
		raw := make([]byte, chunk.EndOffset-chunk.StartOffset)
		n, err := file.ReadAt(raw, chunk.StartOffset)
		if err != nil && !errors.Is(err, io.EOF) {
			return domain.AttachmentPageResult{}, err
		}
		if n != len(raw) {
			return domain.AttachmentPageResult{}, errors.New("stored text content is incomplete")
		}
		page = domain.AttachmentPage{
			AttachmentID: id, Page: pageNumber, Status: "READY", Text: string(raw),
			TextSHA256: sandbox.Hash(raw), StartLine: chunk.StartLine, EndLine: chunk.EndLine,
			Tables: []domain.AttachmentTable{}, Images: []domain.AttachmentAsset{},
		}
	case "image/png", "image/jpeg", "image/webp":
		if pageNumber != 1 || attachment.Status != "READY" {
			return domain.AttachmentPageResult{}, domain.ErrNotFound
		}
		page = domain.AttachmentPage{
			AttachmentID: id, Page: 1, Status: "READY",
			Tables: []domain.AttachmentTable{}, Images: []domain.AttachmentAsset{},
		}
	default:
		page, err = s.repo.GetAttachmentPage(ctx, id, pageNumber)
		if err != nil {
			return domain.AttachmentPageResult{}, err
		}
	}
	source := domain.SourceRef{
		SourceKind: "ATTACHMENT", AttachmentID: attachment.ID, AttachmentHash: attachment.SHA256,
		Page: page.Page, StartLine: page.StartLine, EndLine: page.EndLine,
		ContentSHA256: page.TextSHA256, CoverageStatus: attachment.Status,
	}
	if source.ContentSHA256 == "" {
		source.ContentSHA256 = attachment.SHA256
	}
	if len(attachment.Gaps) > 0 {
		source.UnparsedSummary = attachment.Gaps[0].Text
	}
	return domain.AttachmentPageResult{Attachment: attachment, Page: page, Source: source}, nil
}

func (s *Service) ResolveInputs(ctx context.Context, conversationID string, ids []string) ([]sandbox.Input, error) {
	if len(ids) > domain.AttachmentMessageCount {
		return nil, domain.ErrInvalid
	}
	out := make([]sandbox.Input, 0, len(ids))
	var total int64
	seen := map[string]bool{}
	for _, id := range ids {
		if seen[id] {
			return nil, domain.ErrInvalid
		}
		seen[id] = true
		attachment, file, err := s.Content(ctx, id)
		if err != nil {
			return nil, err
		}
		if attachment.ConversationID != conversationID ||
			(attachment.Status != "READY" && attachment.Status != "PARTIAL") {
			file.Close()
			return nil, domain.ErrNotFound
		}
		total += attachment.Bytes
		if total > domain.AttachmentMessageLimit {
			file.Close()
			return nil, domain.ErrInvalid
		}
		data, readErr := io.ReadAll(io.LimitReader(file, attachment.Bytes+1))
		file.Close()
		if readErr != nil || int64(len(data)) != attachment.Bytes || sandbox.Hash(data) != attachment.SHA256 {
			return nil, errors.New("authorized attachment content mismatch")
		}
		out = append(out, sandbox.Input{
			ExecutionInput: domain.ExecutionInput{
				ID: attachment.ID, Kind: "ATTACHMENT", Name: attachment.Name,
				SHA256: attachment.SHA256, Bytes: attachment.Bytes,
			},
			Data: data,
		})
	}
	return out, nil
}

func (s *Service) work() {
	defer s.wg.Done()
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		jobs, err := s.repo.PendingAttachmentParses(s.ctx)
		if err == nil {
			occupied := 0
			for _, job := range jobs {
				if job.Deleted {
					s.cleanup(job)
					continue
				}
				if job.Status == "PROCESSING" {
					occupied++
					s.reconcile(job)
				}
			}
			for _, job := range jobs {
				if job.Deleted || job.Status != "QUEUED" {
					continue
				}
				if !time.Now().Before(job.Deadline) {
					s.fail(job, "PARSE_TIMEOUT", "附件在解析队列中超时。")
					continue
				}
				if occupied >= 2 {
					continue
				}
				occupied++
				s.dispatch(job)
			}
		}
		select {
		case <-s.ctx.Done():
			return
		case <-s.wake:
		case <-ticker.C:
		}
	}
}

func (s *Service) request(job domain.AttachmentParseJob) (sandbox.Request, error) {
	file, err := s.files.Open(job.Attachment.StorageKey)
	if err != nil {
		return sandbox.Request{}, err
	}
	data, err := io.ReadAll(io.LimitReader(file, job.Attachment.Bytes+1))
	file.Close()
	if err != nil || int64(len(data)) != job.Attachment.Bytes || sandbox.Hash(data) != job.Attachment.SHA256 {
		return sandbox.Request{}, errors.New("attachment content mismatch")
	}
	request := sandbox.Request{
		ID: job.Attachment.ID, Code: parserCode, Image: s.cap.Image,
		Budget: domain.PythonLimits(), Deadline: job.Deadline,
		Inputs: []sandbox.Input{{
			ExecutionInput: domain.ExecutionInput{
				ID: job.Attachment.ID, Kind: "ATTACHMENT", Name: job.Attachment.Name,
				SHA256: job.Attachment.SHA256, Bytes: job.Attachment.Bytes,
			},
			Data: data,
		}},
	}
	return request, request.Validate()
}

func (s *Service) dispatch(job domain.AttachmentParseJob) {
	if err := s.repo.StartAttachmentParse(s.ctx, job.Attachment.ID, job.Version); err != nil {
		return
	}
	job.Status = "PROCESSING"
	job.Version++
	job.Attachment.Status = "PROCESSING"
	job.Attachment.Version++
	request, err := s.request(job)
	if err != nil {
		s.fail(job, "ORIGINAL_MISMATCH", "附件原始字节校验失败。")
		return
	}
	state, err := s.runner.Submit(s.ctx, request)
	if err == nil {
		s.accept(job, request.Hash(), state)
	}
}

func (s *Service) reconcile(job domain.AttachmentParseJob) {
	request, err := s.request(job)
	if err != nil {
		s.fail(job, "ORIGINAL_MISMATCH", "附件原始字节校验失败。")
		return
	}
	state, err := s.runner.Get(s.ctx, job.Attachment.ID)
	if errors.Is(err, sandbox.ErrMissing) {
		s.fail(job, "PARSER_INTERRUPTED", "解析执行记录丢失，未自动重跑。")
		return
	}
	if err == nil {
		s.accept(job, request.Hash(), state)
	}
}

func (s *Service) accept(job domain.AttachmentParseJob, requestHash string, state sandbox.Status) {
	if state.ID != job.Attachment.ID || state.RequestSHA256 != requestHash {
		return
	}
	if !domain.PythonTerminal(state.State) {
		return
	}
	if !state.Result.Cleaned {
		return
	}
	if state.State != "SUCCEEDED" || !state.Result.Complete || state.Result.ExitCode == nil ||
		*state.Result.ExitCode != 0 || len(state.Result.Artifacts) != 1 ||
		state.Result.Artifacts[0].Name != "result.zip" {
		s.fail(job, "PARSER_"+state.Result.Error, "隔离解析程序未成功完成。")
		return
	}
	artifact := state.Result.Artifacts[0]
	stream, err := s.runner.Artifact(s.ctx, state.ID, artifact.ID)
	if err != nil {
		return
	}
	raw, readErr := io.ReadAll(io.LimitReader(stream, domain.AttachmentMessageLimit+1))
	stream.Close()
	if readErr != nil || int64(len(raw)) != artifact.Bytes || sandbox.Hash(raw) != artifact.SHA256 {
		return
	}
	result, staged, err := s.decode(job.Attachment, raw)
	if err != nil {
		s.discard(staged)
		s.fail(job, "PARSER_OUTPUT_INVALID", "解析输出未通过完整性校验。")
		return
	}
	result.Attachment.Version = job.Attachment.Version
	if err = s.repo.FinishAttachmentParse(s.ctx, result, job.Attachment.Version); err != nil {
		s.discard(staged)
		return
	}
	_ = s.runner.Forget(s.ctx, job.Attachment.ID)
}

func (s *Service) fail(job domain.AttachmentParseJob, code, message string) {
	if job.Status == "QUEUED" {
		if s.repo.StartAttachmentParse(s.ctx, job.Attachment.ID, job.Version) != nil {
			return
		}
		job.Status = "PROCESSING"
		job.Version++
		job.Attachment.Status = "PROCESSING"
		job.Attachment.Version++
	}
	job.Attachment.Status = "FAILED"
	job.Attachment.Gaps = []domain.AttachmentGap{{Code: code, Text: message}}
	result := domain.AttachmentParseResult{Attachment: job.Attachment}
	if s.repo.FinishAttachmentParse(s.ctx, result, job.Attachment.Version) == nil {
		_, _ = s.runner.Cancel(s.ctx, job.Attachment.ID)
		_ = s.runner.Forget(s.ctx, job.Attachment.ID)
	}
}

func (s *Service) cleanup(job domain.AttachmentParseJob) {
	_, err := s.runner.Cancel(s.ctx, job.Attachment.ID)
	if err != nil && !errors.Is(err, sandbox.ErrMissing) {
		return
	}
	if err = s.runner.Forget(s.ctx, job.Attachment.ID); err != nil && !errors.Is(err, sandbox.ErrMissing) {
		return
	}
	keys := []string{job.Attachment.StorageKey}
	for _, asset := range job.Assets {
		keys = append(keys, asset.StorageKey)
	}
	for _, key := range keys {
		if err = s.files.Delete(key); err != nil {
			return
		}
	}
	_ = s.repo.DeleteAttachmentData(s.ctx, job.Attachment.ID)
}

func (s *Service) discard(keys []string) {
	for _, key := range keys {
		_ = s.files.Delete(key)
	}
}

type parserManifest struct {
	SchemaVersion  int                    `json:"schema_version"`
	Kind           string                 `json:"kind"`
	MediaType      string                 `json:"media_type"`
	OriginalSHA256 string                 `json:"original_sha256"`
	Status         string                 `json:"status"`
	Width          int                    `json:"width"`
	Height         int                    `json:"height"`
	PageCount      int                    `json:"page_count"`
	LineCount      int                    `json:"line_count"`
	AvailablePages []int                  `json:"available_pages"`
	AvailableLines []int                  `json:"available_lines"`
	Gaps           []domain.AttachmentGap `json:"gaps"`
	TextChunks     []domain.TextChunk     `json:"text_chunks"`
	Pages          []parserPage           `json:"pages"`
}

type parserPage struct {
	Page       int                      `json:"page"`
	Status     string                   `json:"status"`
	Text       string                   `json:"text"`
	TextSHA256 string                   `json:"text_sha256"`
	Tables     []domain.AttachmentTable `json:"tables"`
	Preview    *parserAsset             `json:"preview"`
	Images     []parserAsset            `json:"images"`
	Error      *domain.Failure          `json:"error"`
}

type parserAsset struct {
	Path      string `json:"path"`
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	MediaType string `json:"media_type"`
	SHA256    string `json:"sha256"`
	Bytes     int64  `json:"bytes"`
	Width     int    `json:"width"`
	Height    int    `json:"height"`
}

func (s *Service) decode(attachment domain.Attachment, raw []byte) (domain.AttachmentParseResult, []string, error) {
	reader, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil || len(reader.File) < 1 || len(reader.File) > maxZipEntries {
		return domain.AttachmentParseResult{}, nil, errors.New("invalid parser archive")
	}
	entries := map[string][]byte{}
	var expanded uint64
	for _, file := range reader.File {
		if file.FileInfo().IsDir() || (file.Name != "manifest.json" && !assetPath.MatchString(file.Name)) {
			return domain.AttachmentParseResult{}, nil, errors.New("invalid parser path")
		}
		if _, exists := entries[file.Name]; exists {
			return domain.AttachmentParseResult{}, nil, errors.New("duplicate parser path")
		}
		expanded += file.UncompressedSize64
		if expanded > uint64(domain.AttachmentMessageLimit) {
			return domain.AttachmentParseResult{}, nil, errors.New("parser expansion limit")
		}
		stream, openErr := file.Open()
		if openErr != nil {
			return domain.AttachmentParseResult{}, nil, openErr
		}
		value, readErr := io.ReadAll(io.LimitReader(stream, int64(file.UncompressedSize64)+1))
		stream.Close()
		if readErr != nil || uint64(len(value)) != file.UncompressedSize64 {
			return domain.AttachmentParseResult{}, nil, errors.New("incomplete parser entry")
		}
		entries[file.Name] = value
	}
	manifestRaw, ok := entries["manifest.json"]
	if !ok {
		return domain.AttachmentParseResult{}, nil, errors.New("missing parser manifest")
	}
	delete(entries, "manifest.json")
	var manifest parserManifest
	decoder := json.NewDecoder(bytes.NewReader(manifestRaw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&manifest) != nil || decoder.Decode(new(any)) != io.EOF ||
		manifest.SchemaVersion != 1 || manifest.OriginalSHA256 != attachment.SHA256 ||
		!domain.AttachmentTerminal(manifest.Status) {
		return domain.AttachmentParseResult{}, nil, errors.New("invalid parser manifest")
	}
	expectedKind := map[string]string{
		"image/png": "IMAGE", "image/jpeg": "IMAGE", "image/webp": "IMAGE",
		"application/pdf": "PDF", "text/plain; charset=utf-8": "TEXT",
	}[attachment.MediaType]
	if manifest.Kind != expectedKind || manifest.MediaType != attachment.MediaType ||
		manifest.PageCount < 0 ||
		len(manifest.Gaps) > 1000 {
		return domain.AttachmentParseResult{}, nil, errors.New("parser type or coverage mismatch")
	}
	if manifest.PageCount > domain.AttachmentPDFPageLimit &&
		(manifest.Kind != "PDF" || manifest.Status != "UNSUPPORTED" || len(manifest.Pages) != 0) {
		return domain.AttachmentParseResult{}, nil, errors.New("parser page limit mismatch")
	}
	attachment.Status, attachment.Width, attachment.Height = manifest.Status, manifest.Width, manifest.Height
	attachment.PageCount, attachment.LineCount = manifest.PageCount, manifest.LineCount
	attachment.AvailablePages = append([]int{}, manifest.AvailablePages...)
	attachment.AvailableLines = append([]int{}, manifest.AvailableLines...)
	attachment.Gaps = append([]domain.AttachmentGap{}, manifest.Gaps...)
	attachment.TextChunks = append([]domain.TextChunk{}, manifest.TextChunks...)
	if err = validateCoverage(attachment, manifest); err != nil {
		return domain.AttachmentParseResult{}, nil, err
	}
	result := domain.AttachmentParseResult{Attachment: attachment}
	staged := []string{}
	referenced := map[string]bool{}
	for _, parsed := range manifest.Pages {
		if parsed.Page < 1 || parsed.Page > manifest.PageCount || len(parsed.Text) > maxPageText ||
			(parsed.Status != "READY" && parsed.Status != "FAILED" && parsed.Status != "UNSUPPORTED") ||
			(parsed.Text != "" && sandbox.Hash([]byte(parsed.Text)) != parsed.TextSHA256) {
			s.discard(staged)
			return domain.AttachmentParseResult{}, nil, errors.New("invalid parser page")
		}
		page := domain.AttachmentPage{
			AttachmentID: attachment.ID, Page: parsed.Page, Status: parsed.Status,
			Text: parsed.Text, TextSHA256: parsed.TextSHA256, Tables: parsed.Tables,
			Images: []domain.AttachmentAsset{}, Error: parsed.Error,
		}
		for _, table := range page.Tables {
			if table.Index < 1 || len(table.Rows) > 500 {
				s.discard(staged)
				return domain.AttachmentParseResult{}, nil, errors.New("invalid parser table")
			}
			canonical, encodeErr := canonicalJSON(table.Rows)
			if encodeErr != nil || sandbox.Hash(canonical) != table.SHA256 {
				s.discard(staged)
				return domain.AttachmentParseResult{}, nil, errors.New("parser table hash mismatch")
			}
		}
		if parsed.Preview != nil {
			asset, key, assetErr := s.stageAsset(attachment.ID, parsed.Page, *parsed.Preview, entries, referenced)
			if assetErr != nil {
				s.discard(staged)
				return domain.AttachmentParseResult{}, nil, assetErr
			}
			staged = append(staged, key)
			page.Preview = &asset
			result.Assets = append(result.Assets, asset)
		}
		for _, image := range parsed.Images {
			asset, key, assetErr := s.stageAsset(attachment.ID, parsed.Page, image, entries, referenced)
			if assetErr != nil {
				s.discard(staged)
				return domain.AttachmentParseResult{}, nil, assetErr
			}
			staged = append(staged, key)
			page.Images = append(page.Images, asset)
			result.Assets = append(result.Assets, asset)
		}
		result.Pages = append(result.Pages, page)
	}
	if len(referenced) != len(entries) || (manifest.Kind == "PDF" && manifest.PageCount <= 200 &&
		len(manifest.Pages) != manifest.PageCount && manifest.Status != "UNSUPPORTED") {
		s.discard(staged)
		return domain.AttachmentParseResult{}, nil, errors.New("unreferenced or missing parser output")
	}
	return result, staged, nil
}

func validateCoverage(attachment domain.Attachment, manifest parserManifest) error {
	if manifest.Kind == "TEXT" {
		if manifest.Status == "READY" {
			if manifest.LineCount < 1 || len(manifest.TextChunks) < 1 || len(manifest.TextChunks) > 10_000 ||
				manifest.PageCount != len(manifest.TextChunks) {
				return errors.New("invalid text coverage")
			}
			var previousOffset int64
			previousLine := 1
			for i, chunk := range manifest.TextChunks {
				if chunk.Page != i+1 || chunk.StartLine != previousLine || chunk.StartOffset != previousOffset ||
					chunk.EndLine < chunk.StartLine || chunk.EndOffset < chunk.StartOffset ||
					chunk.EndOffset > attachment.Bytes || chunk.EndOffset-chunk.StartOffset > 2_000_000 {
					return errors.New("invalid text index")
				}
				previousOffset, previousLine = chunk.EndOffset, chunk.EndLine+1
			}
			if previousOffset != attachment.Bytes || manifest.TextChunks[len(manifest.TextChunks)-1].EndLine != manifest.LineCount {
				return errors.New("incomplete text index")
			}
		}
		return nil
	}
	if manifest.Kind == "IMAGE" {
		if manifest.Status == "READY" && (manifest.Width < 1 || manifest.Height < 1 ||
			int64(manifest.Width)*int64(manifest.Height) > 40_000_000) {
			return errors.New("invalid image dimensions")
		}
		return nil
	}
	seen := map[int]bool{}
	for _, page := range manifest.AvailablePages {
		if page < 1 || page > manifest.PageCount || seen[page] {
			return errors.New("invalid PDF coverage")
		}
		seen[page] = true
	}
	return nil
}

func (s *Service) stageAsset(attachmentID string, pageNumber int, parsed parserAsset,
	entries map[string][]byte, referenced map[string]bool) (domain.AttachmentAsset, string, error) {
	raw, ok := entries[parsed.Path]
	if !ok || referenced[parsed.Path] || !assetPath.MatchString(parsed.Path) ||
		int64(len(raw)) != parsed.Bytes || sandbox.Hash(raw) != parsed.SHA256 ||
		(parsed.Kind != "PREVIEW" && parsed.Kind != "EMBEDDED_IMAGE") ||
		(parsed.MediaType != "image/png" && parsed.MediaType != "image/jpeg" && parsed.MediaType != "image/webp") ||
		parsed.Width < 1 || parsed.Height < 1 || int64(parsed.Width)*int64(parsed.Height) > 40_000_000 {
		return domain.AttachmentAsset{}, "", errors.New("invalid parser asset")
	}
	detected := http.DetectContentType(raw)
	if parsed.MediaType == "image/webp" {
		if len(raw) < 12 || string(raw[:4]) != "RIFF" || string(raw[8:12]) != "WEBP" {
			return domain.AttachmentAsset{}, "", errors.New("invalid WebP asset")
		}
	} else if detected != parsed.MediaType {
		return domain.AttachmentAsset{}, "", errors.New("parser asset media mismatch")
	}
	name, err := cleanName(parsed.Name)
	if err != nil {
		return domain.AttachmentAsset{}, "", err
	}
	id, key := rand.Text(), rand.Text()
	if _, _, err = s.files.Put(key, bytes.NewReader(raw), int64(len(raw)), parsed.SHA256); err != nil {
		return domain.AttachmentAsset{}, "", err
	}
	referenced[parsed.Path] = true
	return domain.AttachmentAsset{
		ID: id, AttachmentID: attachmentID, Page: pageNumber, Kind: parsed.Kind,
		Name: name, MediaType: parsed.MediaType, SHA256: parsed.SHA256, Bytes: parsed.Bytes,
		Width: parsed.Width, Height: parsed.Height, StorageKey: key,
	}, key, nil
}

func canonicalJSON(value any) ([]byte, error) {
	var out bytes.Buffer
	encoder := json.NewEncoder(&out)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		return nil, err
	}
	return bytes.TrimSuffix(out.Bytes(), []byte("\n")), nil
}

func ContentDisposition(name string, inline bool) string {
	disposition := "attachment"
	if inline {
		disposition = "inline"
	}
	return mime.FormatMediaType(disposition, map[string]string{"filename": name})
}
