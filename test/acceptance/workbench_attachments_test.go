package acceptance_test

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"hwops/internal/adapters/chatmodel"
	"hwops/internal/adapters/postgres"
	"hwops/internal/application"
	"hwops/internal/blobstore"
	"hwops/internal/domain"
	"hwops/internal/identity"
	"hwops/internal/sandbox"
	"hwops/internal/transport/httpapi"
)

const attachmentImage = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

type attachmentReplayRunner struct {
	mu        sync.Mutex
	statuses  map[string]sandbox.Status
	artifacts map[string][]byte
}

type blockingAttachmentRunner struct {
	mu        sync.Mutex
	statuses  map[string]sandbox.Status
	submitted chan string
	canceled  chan string
}

func newBlockingAttachmentRunner() *blockingAttachmentRunner {
	return &blockingAttachmentRunner{
		statuses: map[string]sandbox.Status{}, submitted: make(chan string, 1), canceled: make(chan string, 1),
	}
}

func (r *blockingAttachmentRunner) Capability(context.Context) (sandbox.Capability, error) {
	return sandbox.Capability{
		Ready: true, Image: attachmentImage, Runtime: "REPLAY blocking parser",
		Budget: domain.PythonLimits(), Packages: map[string]string{"PyMuPDF": "1.26.4", "Pillow": "11.2.1"},
	}, nil
}

func (r *blockingAttachmentRunner) Submit(_ context.Context, request sandbox.Request) (sandbox.Status, error) {
	status := sandbox.Status{ID: request.ID, RequestSHA256: request.Hash(), State: "RUNNING"}
	r.mu.Lock()
	r.statuses[request.ID] = status
	r.mu.Unlock()
	select {
	case r.submitted <- request.ID:
	default:
	}
	return status, nil
}

func (r *blockingAttachmentRunner) Get(_ context.Context, id string) (sandbox.Status, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	status, ok := r.statuses[id]
	if !ok {
		return status, sandbox.ErrMissing
	}
	return status, nil
}

func (r *blockingAttachmentRunner) Cancel(_ context.Context, id string) (sandbox.Status, error) {
	r.mu.Lock()
	status, ok := r.statuses[id]
	if !ok {
		status = sandbox.Status{ID: id}
	}
	status.State = "CANCELED"
	status.Result.Cleaned = true
	r.statuses[id] = status
	r.mu.Unlock()
	select {
	case r.canceled <- id:
	default:
	}
	return status, nil
}

func (r *blockingAttachmentRunner) Artifact(context.Context, string, string) (io.ReadCloser, error) {
	return nil, sandbox.ErrMissing
}

func (r *blockingAttachmentRunner) Forget(_ context.Context, id string) error {
	r.mu.Lock()
	delete(r.statuses, id)
	r.mu.Unlock()
	return nil
}

func (r *blockingAttachmentRunner) lateSuccess(id string) {
	exit := 0
	r.mu.Lock()
	r.statuses[id] = sandbox.Status{
		ID: id, State: "SUCCEEDED",
		Result: domain.PythonResult{ExitCode: &exit, Complete: true, Cleaned: true},
	}
	r.mu.Unlock()
}

func newAttachmentReplayRunner() *attachmentReplayRunner {
	return &attachmentReplayRunner{statuses: map[string]sandbox.Status{}, artifacts: map[string][]byte{}}
}

func (r *attachmentReplayRunner) Capability(context.Context) (sandbox.Capability, error) {
	return sandbox.Capability{
		Ready: true, Image: attachmentImage, Runtime: "REPLAY external parser stub",
		Budget:   domain.PythonLimits(),
		Packages: map[string]string{"PyMuPDF": "1.26.4", "Pillow": "11.2.1"},
	}, nil
}

func (r *attachmentReplayRunner) Submit(_ context.Context, request sandbox.Request) (sandbox.Status, error) {
	if err := request.Validate(); err != nil || len(request.Inputs) != 1 {
		return sandbox.Status{}, sandbox.ErrUnavailable
	}
	if bytes.Contains(request.Inputs[0].Data, []byte("PARSER_TIMEOUT_SENTINEL")) {
		status := sandbox.Status{
			ID: request.ID, RequestSHA256: request.Hash(), State: "FAILED",
			Result: domain.PythonResult{Cleaned: true, Error: "TIME_LIMIT_EXCEEDED"},
		}
		r.mu.Lock()
		r.statuses[request.ID] = status
		r.mu.Unlock()
		return status, nil
	}
	raw := replayParse(request.Inputs[0].Data)
	code := 0
	artifact := domain.Artifact{
		ID: "PARSERARTIFACT1234", ExecutionID: request.ID, Name: "result.zip",
		SHA256: sandbox.Hash(raw), Bytes: int64(len(raw)), MediaType: "application/zip",
	}
	status := sandbox.Status{
		ID: request.ID, RequestSHA256: request.Hash(), State: "SUCCEEDED",
		Result: domain.PythonResult{
			ExitCode: &code, Complete: true, Cleaned: true, Artifacts: []domain.Artifact{artifact},
		},
	}
	r.mu.Lock()
	r.statuses[request.ID] = status
	r.artifacts[request.ID+"/"+artifact.ID] = raw
	r.mu.Unlock()
	return status, nil
}

func (r *attachmentReplayRunner) Get(_ context.Context, id string) (sandbox.Status, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	status, ok := r.statuses[id]
	if !ok {
		return status, sandbox.ErrMissing
	}
	return status, nil
}

func (r *attachmentReplayRunner) Cancel(_ context.Context, id string) (sandbox.Status, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	status, ok := r.statuses[id]
	if !ok {
		return sandbox.Status{ID: id, State: "CANCELED", Result: domain.PythonResult{Cleaned: true}}, nil
	}
	return status, nil
}

func (r *attachmentReplayRunner) Artifact(_ context.Context, id, artifact string) (io.ReadCloser, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	raw, ok := r.artifacts[id+"/"+artifact]
	if !ok {
		return nil, sandbox.ErrMissing
	}
	return io.NopCloser(bytes.NewReader(raw)), nil
}

func (r *attachmentReplayRunner) Forget(_ context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	status, ok := r.statuses[id]
	if !ok {
		return sandbox.ErrMissing
	}
	for _, artifact := range status.Result.Artifacts {
		delete(r.artifacts, id+"/"+artifact.ID)
	}
	return nil
}

func onePixelPNG() []byte {
	var out bytes.Buffer
	value := image.NewRGBA(image.Rect(0, 0, 1, 1))
	value.Set(0, 0, color.RGBA{R: 180, G: 80, B: 30, A: 255})
	_ = png.Encode(&out, value)
	return out.Bytes()
}

func replayParse(input []byte) []byte {
	kind, mediaType, status := "TEXT", "text/plain; charset=utf-8", "READY"
	pageCount, lineCount := 1, 0
	availablePages := []int{}
	availableLines := []int{}
	var textChunks []map[string]any
	pages := []any{}
	gaps := []any{}
	assets := map[string][]byte{}
	if bytes.HasPrefix(input, []byte("%PDF-")) {
		kind, mediaType, status = "PDF", "application/pdf", "PARTIAL"
		pageCount, lineCount = 2, 0
		availablePages, availableLines, textChunks = []int{1}, []int{}, nil
		pixel := onePixelPNG()
		assets["assets/page-001-preview.png"] = pixel
		assets["assets/page-001-image-001.png"] = pixel
		table := [][]string{{"hour", "errors"}, {"10", "3"}}
		tableRaw, _ := json.Marshal(table)
		pages = []any{
			map[string]any{
				"page": 1, "status": "READY", "text": "Synthetic PDF page one\n",
				"text_sha256": sandbox.Hash([]byte("Synthetic PDF page one\n")),
				"tables":      []any{map[string]any{"index": 1, "rows": table, "sha256": sandbox.Hash(tableRaw)}},
				"preview": map[string]any{
					"path": "assets/page-001-preview.png", "kind": "PREVIEW", "name": "page-1-preview.png",
					"media_type": "image/png", "sha256": sandbox.Hash(pixel), "bytes": len(pixel), "width": 1, "height": 1,
				},
				"images": []any{map[string]any{
					"path": "assets/page-001-image-001.png", "kind": "EMBEDDED_IMAGE", "name": "page-1-image-1.png",
					"media_type": "image/png", "sha256": sandbox.Hash(pixel), "bytes": len(pixel), "width": 1, "height": 1,
				}},
				"error": nil,
			},
			map[string]any{
				"page": 2, "status": "UNSUPPORTED", "text": "", "text_sha256": "", "tables": []any{},
				"preview": nil, "images": []any{},
				"error": map[string]string{"code": "SCANNED_PAGE", "message": "页面只有扫描图像，本期不执行整页 OCR。"},
			},
		}
		gaps = []any{map[string]any{"page": 2, "code": "SCANNED_PAGE", "text": "页面只有扫描图像，本期不执行整页 OCR。"}}
	} else if bytes.HasPrefix(input, []byte("\x89PNG\r\n\x1a\n")) {
		kind, mediaType, status = "IMAGE", "image/png", "READY"
		pageCount, lineCount = 1, 0
		availablePages, availableLines, textChunks = []int{1}, []int{}, nil
	} else {
		lineCount = strings.Count(string(input), "\n")
		if !bytes.HasSuffix(input, []byte("\n")) {
			lineCount++
		}
		if lineCount == 0 {
			lineCount = 1
		}
		availableLines = []int{1, lineCount}
		textChunks = []map[string]any{{
			"page": 1, "start_line": 1, "end_line": lineCount,
			"start_offset": 0, "end_offset": len(input),
		}}
	}
	manifest := map[string]any{
		"schema_version": 1, "kind": kind, "media_type": mediaType,
		"original_sha256": sandbox.Hash(input), "status": status, "width": 1, "height": 1,
		"page_count": pageCount, "line_count": lineCount, "available_pages": availablePages,
		"available_lines": availableLines, "gaps": gaps, "text_chunks": textChunks, "pages": pages,
	}
	var out bytes.Buffer
	archive := zip.NewWriter(&out)
	entry, _ := archive.Create("manifest.json")
	_ = json.NewEncoder(entry).Encode(manifest)
	for name, raw := range assets {
		entry, _ = archive.Create(name)
		_, _ = entry.Write(raw)
	}
	_ = archive.Close()
	return out.Bytes()
}

func uploadAttachment(t *testing.T, client *wbClient, conversationID, name, declaredType string, data []byte, expected int) map[string]any {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	header := make(textproto.MIMEHeader)
	header.Set("Content-Disposition", `form-data; name="file"; filename="`+name+`"`)
	header.Set("Content-Type", declaredType)
	part, _ := writer.CreatePart(header)
	_, _ = part.Write(data)
	_ = writer.Close()
	req, _ := http.NewRequest("POST", client.base+"/v1/conversations/"+conversationID+"/attachments", &body)
	req.Header.Set("Origin", client.base)
	req.Header.Set("X-CSRF-Token", client.csrf)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	res, err := client.client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != expected {
		raw, _ := io.ReadAll(res.Body)
		t.Fatalf("upload status %d, expected %d: %s", res.StatusCode, expected, raw)
	}
	var value map[string]any
	if err = json.NewDecoder(res.Body).Decode(&value); err != nil {
		t.Fatal(err)
	}
	return value
}

func waitAttachment(t *testing.T, client *wbClient, id string) map[string]any {
	t.Helper()
	for deadline := time.Now().Add(90 * time.Second); time.Now().Before(deadline); {
		value := client.request("GET", "/v1/attachments/"+id, nil, 200)
		switch value["status"] {
		case "READY", "PARTIAL", "FAILED", "UNSUPPORTED":
			return value
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("attachment parse did not complete")
	return nil
}

func actualAttachmentParser(t *testing.T) *sandbox.Client {
	t.Helper()
	endpoint := os.Getenv("HWOPS_TEST_PARSER_RUNNER_URL")
	if endpoint == "" {
		t.Skip("actual Linux + gVisor parser runner required; HWOPS_TEST_PARSER_RUNNER_URL unset")
	}
	raw, err := os.ReadFile(os.Getenv("HWOPS_TEST_PARSER_RUNNER_TOKEN_FILE"))
	if err != nil {
		t.Fatal("parser token file unavailable")
	}
	client, err := sandbox.NewClient(endpoint, strings.TrimSpace(string(raw)))
	if err != nil {
		t.Fatal(err)
	}
	capability, err := client.Capability(context.Background())
	if err != nil || capability.Packages["PyMuPDF"] != "1.26.4" {
		t.Fatalf("actual parser unavailable: %+v %v", capability, err)
	}
	t.Logf("ACTUAL PARSER: runtime=%s kernel=%s image=%s probe=%s packages=%v",
		capability.Runtime, capability.Linux, capability.Image, capability.ProbeSHA256, capability.Packages)
	return client
}

func syntheticPDF(streams []string, imageObject bool) []byte {
	type object struct {
		id   int
		body []byte
	}
	next := 4
	imageID := 0
	objects := []object{
		{id: 1, body: []byte("<< /Type /Catalog /Pages 2 0 R >>")},
		{id: 3, body: []byte("<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>")},
	}
	if imageObject {
		imageID = next
		next++
		objects = append(objects, object{id: imageID, body: []byte("<< /Type /XObject /Subtype /Image /Width 1 /Height 1 /ColorSpace /DeviceRGB /BitsPerComponent 8 /Length 3 >>\nstream\n\xcc\x55\x22\nendstream")})
	}
	pageIDs := make([]int, 0, len(streams))
	for _, stream := range streams {
		pageID, contentID := next, next+1
		next += 2
		pageIDs = append(pageIDs, pageID)
		resources := "<< /Font << /F1 3 0 R >>"
		if imageObject {
			resources += fmt.Sprintf(" /XObject << /Im1 %d 0 R >>", imageID)
		}
		resources += " >>"
		objects = append(objects,
			object{id: pageID, body: []byte(fmt.Sprintf("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources %s /Contents %d 0 R >>", resources, contentID))},
			object{id: contentID, body: []byte(fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(stream), stream))},
		)
	}
	var kids strings.Builder
	for _, id := range pageIDs {
		fmt.Fprintf(&kids, "%d 0 R ", id)
	}
	objects = append(objects, object{id: 2, body: []byte(fmt.Sprintf("<< /Type /Pages /Kids [%s] /Count %d >>", kids.String(), len(pageIDs)))})
	maxID := next - 1
	byID := make(map[int][]byte, len(objects))
	for _, item := range objects {
		byID[item.id] = item.body
	}
	var out bytes.Buffer
	out.WriteString("%PDF-1.4\n%\xe2\xe3\xcf\xd3\n")
	offsets := make([]int, maxID+1)
	for id := 1; id <= maxID; id++ {
		offsets[id] = out.Len()
		fmt.Fprintf(&out, "%d 0 obj\n", id)
		out.Write(byID[id])
		out.WriteString("\nendobj\n")
	}
	xref := out.Len()
	fmt.Fprintf(&out, "xref\n0 %d\n0000000000 65535 f \n", maxID+1)
	for id := 1; id <= maxID; id++ {
		fmt.Fprintf(&out, "%010d 00000 n \n", offsets[id])
	}
	fmt.Fprintf(&out, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", maxID+1, xref)
	return out.Bytes()
}

func TestWB04ActualIsolatedParser(t *testing.T) {
	parser := actualAttachmentParser(t)
	dsn := workbenchDatabase(t)
	files, err := blobstore.Open(filepath.Join(t.TempDir(), "private"), 1_000_000_000, 100_000_000)
	if err != nil {
		t.Fatal(err)
	}
	server, _, stop := wbStartOptions(t, dsn, wbModel(t), true, application.Options{
		UsersMode: true, Files: files, AttachmentRunner: parser,
	})
	defer stop()
	client := wbNewClient(t, server)
	client.login("admin", wbPassword)
	cid := client.request("POST", "/v1/conversations", map[string]any{}, 201)["id"].(string)

	pngFile := uploadAttachment(t, client, cid, "photo.bin", "application/octet-stream", onePixelPNG(), 201)
	pngFile = waitAttachment(t, client, pngFile["id"].(string))
	if pngFile["status"] != "READY" || pngFile["media_type"] != "image/png" ||
		pngFile["width"] != float64(1) || pngFile["height"] != float64(1) {
		t.Fatalf("actual image decode failed: %v", pngFile)
	}
	logRaw := []byte("alpha\nbeta ERROR\n")
	logFile := uploadAttachment(t, client, cid, "device.log", "text/plain", logRaw, 201)
	logFile = waitAttachment(t, client, logFile["id"].(string))
	if logFile["status"] != "READY" || logFile["line_count"] != float64(2) {
		t.Fatalf("actual text parse failed: %v", logFile)
	}

	textAndTable := `BT /F1 12 Tf 72 740 Td (Synthetic PDF page one) Tj ET
0.7 w 72 650 m 300 650 l S 72 620 m 300 620 l S 72 590 m 300 590 l S
72 590 m 72 650 l S 180 590 m 180 650 l S 300 590 m 300 650 l S
BT /F1 10 Tf 78 632 Td (hour) Tj 186 0 Td (errors) Tj 78 602 Td (10) Tj 108 0 Td (3) Tj ET
q 80 0 0 80 72 470 cm /Im1 Do Q`
	scanned := `q 400 0 0 400 72 250 cm /Im1 Do Q`
	mixedRaw := syntheticPDF([]string{textAndTable, scanned}, true)
	mixed := uploadAttachment(t, client, cid, "mixed.pdf", "application/pdf", mixedRaw, 201)
	mixed = waitAttachment(t, client, mixed["id"].(string))
	if mixed["status"] != "PARTIAL" || mixed["page_count"] != float64(2) ||
		len(mixed["available_pages"].([]any)) != 1 {
		t.Fatalf("mixed PDF coverage failed: %v", mixed)
	}
	page := client.request("GET", "/v1/attachments/"+mixed["id"].(string)+"/pages/1", nil, 200)["page"].(map[string]any)
	if !strings.Contains(page["text"].(string), "Synthetic PDF page one") ||
		len(page["tables"].([]any)) == 0 || len(page["images"].([]any)) == 0 ||
		page["preview"] == nil {
		t.Fatalf("actual PDF text/table/image/preview path failed: %v", page)
	}
	scanOnly := uploadAttachment(t, client, cid, "scan.pdf", "application/pdf", syntheticPDF([]string{scanned}, true), 201)
	scanOnly = waitAttachment(t, client, scanOnly["id"].(string))
	if scanOnly["status"] != "UNSUPPORTED" ||
		scanOnly["gaps"].([]any)[0].(map[string]any)["code"] != "SCANNED_PAGE" {
		t.Fatalf("pure scan was not explicit: %v", scanOnly)
	}
	damaged := uploadAttachment(t, client, cid, "damaged.pdf", "application/pdf", []byte("%PDF-not-a-document"), 201)
	damaged = waitAttachment(t, client, damaged["id"].(string))
	if damaged["status"] != "FAILED" {
		t.Fatalf("damaged PDF was hidden: %v", damaged)
	}
	manyStreams := make([]string, 201)
	for i := range manyStreams {
		manyStreams[i] = fmt.Sprintf("BT /F1 8 Tf 20 20 Td (page %d) Tj ET", i+1)
	}
	tooMany := uploadAttachment(t, client, cid, "201-pages.pdf", "application/pdf", syntheticPDF(manyStreams, false), 201)
	tooMany = waitAttachment(t, client, tooMany["id"].(string))
	if tooMany["status"] != "UNSUPPORTED" || tooMany["page_count"] != float64(201) ||
		tooMany["gaps"].([]any)[0].(map[string]any)["code"] != "PDF_PAGE_LIMIT" {
		t.Fatalf("PDF page hard limit was not explicit: %v", tooMany)
	}
	t.Log("ACTUAL: public HTTP + PostgreSQL/files + separate pinned parser image under Linux/gVisor; image, UTF-8 log, PDF text/table/embedded image/preview, scan/mixed/corrupt/201-page cases passed")
}

func TestWB04VisualBytesExternalAdapter(t *testing.T) {
	imageBytes := onePixelPNG()
	captured := make(chan []byte, 1)
	modelServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Messages []struct {
				Content json.RawMessage `json:"content"`
			} `json:"messages"`
		}
		if json.NewDecoder(r.Body).Decode(&body) != nil || len(body.Messages) != 2 {
			http.Error(w, "bad model request", 400)
			return
		}
		var parts []struct {
			Type     string `json:"type"`
			ImageURL *struct {
				URL string `json:"url"`
			} `json:"image_url"`
		}
		if json.Unmarshal(body.Messages[1].Content, &parts) != nil || len(parts) != 2 ||
			parts[1].ImageURL == nil || !strings.HasPrefix(parts[1].ImageURL.URL, "data:image/png;base64,") {
			http.Error(w, "missing inline image", 400)
			return
		}
		raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(parts[1].ImageURL.URL, "data:image/png;base64,"))
		if err != nil {
			http.Error(w, "invalid image", 400)
			return
		}
		captured <- raw
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []any{map[string]any{
				"message":       map[string]string{"content": "图中是一个合成的单像素测试图像。"},
				"finish_reason": "stop",
			}},
			"usage": map[string]int{"prompt_tokens": 20, "completion_tokens": 8, "total_tokens": 28},
		})
	}))
	defer modelServer.Close()
	model, err := chatmodel.NewOpenAI(modelServer.URL, "wb04-vision-replay", "")
	if err != nil {
		t.Fatal(err)
	}
	dsn := workbenchDatabase(t)
	store, err := postgres.Open(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	accounts, err := identity.New(store.Pool())
	if err != nil {
		t.Fatal(err)
	}
	user, err := accounts.CreateUser(context.Background(), "", "admin", wbPassword, "ADMIN", true)
	if err != nil {
		t.Fatal(err)
	}
	files, err := blobstore.Open(filepath.Join(t.TempDir(), "private"), 500_000_000, 100_000_000)
	if err != nil {
		t.Fatal(err)
	}
	app, err := application.New(store, model, "REPLAY", application.Options{
		UsersMode: true, Files: files, AttachmentRunner: newAttachmentReplayRunner(),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close()
	server := httptest.NewUnstartedServer(nil)
	server.Config.Handler, err = httpapi.NewUsers(app, accounts, httpapi.UsersConfig{
		Origin: "https://" + server.Listener.Addr().String(),
	})
	if err != nil {
		t.Fatal(err)
	}
	server.StartTLS()
	defer server.Close()
	client := wbNewClient(t, server)
	client.login("admin", wbPassword)
	cid := client.request("POST", "/v1/conversations", map[string]any{}, 201)["id"].(string)
	uploaded := uploadAttachment(t, client, cid, "vision.png", "image/png", imageBytes, 201)
	uploaded = waitAttachment(t, client, uploaded["id"].(string))
	ctx := domain.WithUser(context.Background(), user)
	answer, source, usage, err := app.AnalyzeAttachmentImage(ctx, uploaded["id"].(string), "描述图像")
	if err != nil || !strings.Contains(answer, "单像素") || source.AttachmentHash != sandbox.Hash(imageBytes) ||
		usage == nil || usage.TotalTokens != 28 {
		t.Fatalf("visual path failed: answer=%q source=%+v usage=%+v err=%v", answer, source, usage, err)
	}
	select {
	case raw := <-captured:
		if !bytes.Equal(raw, imageBytes) {
			t.Fatal("visual adapter changed the authorized image bytes")
		}
	case <-time.After(time.Second):
		t.Fatal("external visual adapter did not receive image bytes")
	}
	t.Log("REPLAY: public upload + private owner lookup + external visual adapter received the exact immutable PNG bytes as bounded inline data; real visual model is reported separately")
}

func TestWB04ReplayPrivateAttachmentHTTP(t *testing.T) {
	dsn := workbenchDatabase(t)
	files, err := blobstore.Open(filepath.Join(t.TempDir(), "private"), 500_000_000, 100_000_000)
	if err != nil {
		t.Fatal(err)
	}
	parser := newAttachmentReplayRunner()
	server, _, stop := wbStartOptions(t, dsn, wbModel(t), true, application.Options{
		UsersMode: true, Files: files, AttachmentRunner: parser,
	})
	defer stop()
	admin, alice, bob := wbNewClient(t, server), wbNewClient(t, server), wbNewClient(t, server)
	admin.login("admin", wbPassword)
	admin.request("POST", "/v1/admin/users", map[string]string{"username": "alice", "password": wbPassword, "role": "USER"}, 201)
	admin.request("POST", "/v1/admin/users", map[string]string{"username": "bob", "password": wbPassword, "role": "USER"}, 201)
	alice.login("alice", wbPassword)
	bob.login("bob", wbPassword)
	cid := alice.request("POST", "/v1/conversations", map[string]any{}, 201)["id"].(string)
	otherCID := alice.request("POST", "/v1/conversations", map[string]any{}, 201)["id"].(string)

	logRaw := []byte("2026-10-04T10:00:00Z INFO boot\n2026-10-04T10:01:00Z ERROR fan\n")
	logFile := uploadAttachment(t, alice, cid, "forged.png", "image/png", logRaw, 201)
	logID := logFile["id"].(string)
	logFile = waitAttachment(t, alice, logID)
	if logFile["media_type"] != "text/plain; charset=utf-8" || logFile["status"] != "READY" {
		t.Fatalf("real bytes did not override forged MIME: %v", logFile)
	}
	page := alice.request("GET", "/v1/attachments/"+logID+"/pages/1", nil, 200)
	pageBody := page["page"].(map[string]any)
	if pageBody["start_line"] != float64(1) || pageBody["end_line"] != float64(2) ||
		pageBody["text"] != string(logRaw) {
		t.Fatalf("line source changed: %v", pageBody)
	}
	source := page["source"].(map[string]any)
	if source["source_kind"] != "ATTACHMENT" || source["attachment_hash"] != sandbox.Hash(logRaw) ||
		source["content_sha256"] != sandbox.Hash(logRaw) {
		t.Fatalf("line SourceRef is not verifiable: %v", source)
	}

	pdfRaw := []byte("%PDF-FAKE-WB04")
	pdf := uploadAttachment(t, alice, cid, "manual.pdf", "application/octet-stream", pdfRaw, 201)
	pdfID := pdf["id"].(string)
	pdf = waitAttachment(t, alice, pdfID)
	if pdf["status"] != "PARTIAL" || pdf["page_count"] != float64(2) ||
		len(pdf["gaps"].([]any)) != 1 {
		t.Fatalf("partial PDF coverage missing: %v", pdf)
	}
	pdfPage := alice.request("GET", "/v1/attachments/"+pdfID+"/pages/1", nil, 200)["page"].(map[string]any)
	if len(pdfPage["tables"].([]any)) != 1 || len(pdfPage["images"].([]any)) != 1 {
		t.Fatalf("PDF table/image route missing: %v", pdfPage)
	}
	asset := pdfPage["images"].([]any)[0].(map[string]any)["id"].(string)
	req, _ := http.NewRequest("GET", alice.base+"/v1/attachments/"+pdfID+"/assets/"+asset, nil)
	req.Header.Set("Range", "bytes=0-7")
	res, err := alice.client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	assetBytes, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != 206 || len(assetBytes) != 8 {
		t.Fatal("derived image Range response failed")
	}

	for _, client := range []*wbClient{bob, admin} {
		client.request("GET", "/v1/attachments/"+logID, nil, 404)
		client.request("GET", "/v1/attachments/"+pdfID+"/pages/1", nil, 404)
	}
	bad := alice.request("POST", "/v1/conversations/"+otherCID+"/messages",
		map[string]any{"text": "stolen", "attachment_ids": []string{logID}}, 404)
	if bad["code"] != "NOT_FOUND" {
		t.Fatal("cross-conversation attachment existence leaked")
	}
	pending := alice.request("POST", "/v1/conversations/"+cid+"/messages",
		map[string]any{"text": "根据附件分析", "attachment_ids": []string{logID, pdfID}}, 202,
		map[string]string{"Idempotency-Key": "wb04-attachments"})
	if len(pending["attachments"].([]any)) != 2 {
		t.Fatal("message transaction did not bind attachments")
	}
	alice.request("POST", "/v1/conversations/"+cid+"/messages",
		map[string]any{"text": "根据附件分析", "attachment_ids": []string{pdfID, logID}}, 409,
		map[string]string{"Idempotency-Key": "wb04-attachments"})
	uploadAttachment(t, alice, cid, "bad.log", "text/plain", []byte{0xff, 0xfe, 0xfd}, 400)
	timeout := uploadAttachment(t, alice, cid, "timeout.log", "text/plain", []byte("PARSER_TIMEOUT_SENTINEL\n"), 201)
	timeout = waitAttachment(t, alice, timeout["id"].(string))
	if timeout["status"] != "FAILED" ||
		timeout["gaps"].([]any)[0].(map[string]any)["code"] != "PARSER_TIME_LIMIT_EXCEEDED" {
		t.Fatalf("parser timeout was not explicit: %v", timeout)
	}

	large := make([]byte, domain.AttachmentFileLimit)
	copy(large, onePixelPNG())
	boundaryCID := alice.request("POST", "/v1/conversations", map[string]any{}, 201)["id"].(string)
	firstLarge := uploadAttachment(t, alice, boundaryCID, "first.png", "image/png", large, 201)
	firstLarge = waitAttachment(t, alice, firstLarge["id"].(string))
	secondLarge := uploadAttachment(t, alice, boundaryCID, "second.png", "image/png", large, 201)
	secondLarge = waitAttachment(t, alice, secondLarge["id"].(string))
	exact := alice.request("POST", "/v1/conversations/"+boundaryCID+"/messages", map[string]any{
		"text":           "exact attachment byte boundary",
		"attachment_ids": []string{firstLarge["id"].(string), secondLarge["id"].(string)},
	}, 202)
	alice.answer(exact["id"].(string))
	overTotal := uploadAttachment(t, alice, boundaryCID, "third.png", "image/png", onePixelPNG(), 201)
	overTotal = waitAttachment(t, alice, overTotal["id"].(string))
	alice.request("POST", "/v1/conversations/"+boundaryCID+"/messages", map[string]any{
		"text": "over total attachment boundary",
		"attachment_ids": []string{
			firstLarge["id"].(string), secondLarge["id"].(string), overTotal["id"].(string),
		},
	}, 400)

	tooLarge := append(append([]byte{}, large...), 0)
	uploadAttachment(t, alice, boundaryCID, "too-large.png", "image/png", tooLarge, 400)
	large, tooLarge = nil, nil

	countCID := alice.request("POST", "/v1/conversations", map[string]any{}, 201)["id"].(string)
	var five []string
	for index := 0; index < 5; index++ {
		file := uploadAttachment(t, alice, countCID, fmt.Sprintf("%d.png", index), "image/png", onePixelPNG(), 201)
		file = waitAttachment(t, alice, file["id"].(string))
		five = append(five, file["id"].(string))
	}
	countOK := alice.request("POST", "/v1/conversations/"+countCID+"/messages",
		map[string]any{"text": "five attachments", "attachment_ids": five}, 202)
	alice.answer(countOK["id"].(string))
	sixth := uploadAttachment(t, alice, countCID, "sixth.png", "image/png", onePixelPNG(), 201)
	sixth = waitAttachment(t, alice, sixth["id"].(string))
	alice.request("POST", "/v1/conversations/"+countCID+"/messages",
		map[string]any{"text": "six attachments", "attachment_ids": append(five, sixth["id"].(string))}, 400)
	t.Log("REPLAY: real PostgreSQL/files/public HTTP; external parser stub; MIME sniffing, line hashes, PDF table/image/partial coverage, Range and owner+conversation isolation passed")
}

func TestWB04UploadAbortDeleteAndLateParserResult(t *testing.T) {
	dsn := workbenchDatabase(t)
	store, err := postgres.Open(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	accounts, err := identity.New(store.Pool())
	if err != nil {
		t.Fatal(err)
	}
	user, err := accounts.CreateUser(context.Background(), "", "admin", wbPassword, "ADMIN", true)
	if err != nil {
		t.Fatal(err)
	}
	files, err := blobstore.Open(filepath.Join(t.TempDir(), "private"), 500_000_000, 100_000_000)
	if err != nil {
		t.Fatal(err)
	}
	runner := newBlockingAttachmentRunner()
	app, err := application.New(store, wbModel(t), "REPLAY", application.Options{
		UsersMode: true, Files: files, AttachmentRunner: runner,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close()
	server := httptest.NewUnstartedServer(nil)
	server.Config.Handler, err = httpapi.NewUsers(app, accounts, httpapi.UsersConfig{
		Origin: "https://" + server.Listener.Addr().String(),
	})
	if err != nil {
		t.Fatal(err)
	}
	server.StartTLS()
	defer server.Close()
	client := wbNewClient(t, server)
	client.login("admin", wbPassword)

	abortCID := client.request("POST", "/v1/conversations", map[string]any{}, 201)["id"].(string)
	pipeReader, pipeWriter := io.Pipe()
	multipartWriter := multipart.NewWriter(pipeWriter)
	requestCtx, cancelUpload := context.WithCancel(context.Background())
	req, _ := http.NewRequestWithContext(requestCtx, "POST",
		client.base+"/v1/conversations/"+abortCID+"/attachments", pipeReader)
	req.Header.Set("Origin", client.base)
	req.Header.Set("X-CSRF-Token", client.csrf)
	req.Header.Set("Content-Type", multipartWriter.FormDataContentType())
	go func() {
		part, _ := multipartWriter.CreateFormFile("file", "aborted.log")
		_, _ = part.Write(bytes.Repeat([]byte("partial\n"), 4096))
		cancelUpload()
		_ = pipeWriter.CloseWithError(context.Canceled)
	}()
	if res, requestErr := client.client.Do(req); requestErr == nil {
		res.Body.Close()
	}
	time.Sleep(100 * time.Millisecond)
	var aborted int
	if err = store.Pool().QueryRow(context.Background(),
		"SELECT count(*) FROM attachments WHERE conversation_id=$1", abortCID).Scan(&aborted); err != nil || aborted != 0 {
		t.Fatalf("aborted upload became an attachment: count=%d err=%v", aborted, err)
	}

	cid := client.request("POST", "/v1/conversations", map[string]any{}, 201)["id"].(string)
	uploaded := uploadAttachment(t, client, cid, "deleting.png", "image/png", onePixelPNG(), 201)
	id := uploaded["id"].(string)
	select {
	case submitted := <-runner.submitted:
		if submitted != id {
			t.Fatal("parser started a different attachment")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("blocking parser did not start")
	}
	ownerCtx := domain.WithUser(context.Background(), user)
	stored, err := store.GetAttachment(ownerCtx, id)
	if err != nil {
		t.Fatal(err)
	}
	client.request("DELETE", "/v1/conversations/"+cid, nil, 202)
	client.request("GET", "/v1/attachments/"+id, nil, 404)
	rangeReq, _ := http.NewRequest("GET", client.base+"/v1/attachments/"+id+"/content", nil)
	rangeReq.Header.Set("Range", "bytes=0-7")
	rangeRes, err := client.client.Do(rangeReq)
	if err != nil {
		t.Fatal(err)
	}
	rangeRes.Body.Close()
	if rangeRes.StatusCode != 404 {
		t.Fatalf("deleted attachment Range remained visible: %d", rangeRes.StatusCode)
	}
	select {
	case canceled := <-runner.canceled:
		if canceled != id {
			t.Fatal("cleanup canceled a different parser identity")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("conversation deletion did not cancel parser")
	}
	runner.lateSuccess(id)
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); {
		var count int
		err = store.Pool().QueryRow(context.Background(), "SELECT count(*) FROM attachments WHERE id=$1", id).Scan(&count)
		_, fileErr := files.Open(stored.StorageKey)
		if err == nil && count == 0 && os.IsNotExist(fileErr) {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	var remaining int
	if err = store.Pool().QueryRow(context.Background(), "SELECT count(*) FROM attachments WHERE id=$1", id).Scan(&remaining); err != nil ||
		remaining != 0 {
		t.Fatalf("deleted attachment record survived cleanup: count=%d err=%v", remaining, err)
	}
	if _, err = files.Open(stored.StorageKey); !os.IsNotExist(err) {
		t.Fatalf("deleted original survived cleanup: %v", err)
	}
	client.request("GET", "/v1/attachments/"+id, nil, 404)
	t.Log("REPLAY: aborted multipart left no record; parsing-time deletion revoked metadata/Range immediately, canceled the isolated job, removed bytes and ignored a late terminal result")
}
