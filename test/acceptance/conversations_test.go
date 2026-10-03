package acceptance_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"hwops/internal/adapters/chatmodel"
	"hwops/internal/adapters/filestore"
	"hwops/internal/domain"
)

func keyedMessage(t *testing.T, s *httptest.Server, conversationID, key string, input domain.MessageInput, status int) domain.Response {
	t.Helper()
	raw, _ := json.Marshal(input)
	req, _ := http.NewRequest("POST", s.URL+"/v1/conversations/"+conversationID+"/messages", bytes.NewReader(raw))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", key)
	res, err := s.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	raw, _ = io.ReadAll(res.Body)
	if res.StatusCode != status {
		t.Fatalf("message status %d, want %d: %s", res.StatusCode, status, raw)
	}
	var response domain.Response
	_ = json.Unmarshal(raw, &response)
	return response
}

func TestConversationIdempotencyContextAndRestart(t *testing.T) {
	var modelCalls atomic.Int32
	external := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		modelCalls.Add(1)
		modelReply(w, `{"claims":[],"gaps":["测试资料不足。"]}`)
	}))
	defer external.Close()
	cm, err := chatmodel.NewOpenAI(external.URL, "fixture", "")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(tempDir(t), "state.json")
	s, stop := startServer(t, path, cm, "REPLAY")
	putDevice(t, s, "device-a", "fixture/Atlas", "R2")
	putDevice(t, s, "device-b", "fixture/Boreal", "R10")
	c := request(t, s.Client(), "POST", s.URL+"/v1/conversations", map[string]any{}, http.StatusCreated)
	cid := c["id"].(string)
	input := domain.MessageInput{Text: "解释 E42", DeviceID: "device-a"}
	var wg sync.WaitGroup
	ids := make(chan string, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ids <- keyedMessage(t, s, cid, "same-key", input, http.StatusAccepted).ID
		}()
	}
	wg.Wait()
	close(ids)
	id := ""
	for value := range ids {
		if id != "" && id != value {
			t.Fatal("concurrent idempotency created multiple tasks")
		}
		id = value
	}
	first := awaitResponse(t, s, id)
	if modelCalls.Load() != 1 {
		t.Fatalf("duplicate model executions: %d", modelCalls.Load())
	}
	keyedMessage(t, s, cid, "same-key", domain.MessageInput{Text: "different"}, http.StatusConflict)
	next := keyedMessage(t, s, cid, "", domain.MessageInput{Text: "它的告警呢？", ContextRevision: first.ContextRevision}, http.StatusAccepted)
	if next.DeviceContext == nil || next.DeviceContext.DeviceID != "device-a" {
		t.Fatalf("pronoun lost target: %+v", next)
	}
	generic := keyedMessage(t, s, cid, "", domain.MessageInput{Text: "什么是ECC？"}, http.StatusAccepted)
	if generic.DeviceContext != nil {
		t.Fatal("generic question inherited a device")
	}
	switched := keyedMessage(t, s, cid, "", domain.MessageInput{Text: "解释 E42", DeviceID: "device-b"}, http.StatusAccepted)
	if switched.DeviceContext == nil || switched.DeviceContext.Firmware != "R10" {
		t.Fatal("switch did not select new device")
	}
	keyedMessage(t, s, cid, "", domain.MessageInput{Text: "它呢？", ContextRevision: first.ContextRevision}, http.StatusConflict)
	conflict := keyedMessage(t, s, cid, "", domain.MessageInput{Text: "device-a 怎么了？", DeviceID: "device-b"}, http.StatusAccepted)
	if conflict.Status != "NEEDS_CLARIFICATION" {
		t.Fatal("conflict did not require clarification")
	}
	ambiguous := keyedMessage(t, s, cid, "", domain.MessageInput{Text: "它呢？"}, http.StatusAccepted)
	if ambiguous.Status != "NEEDS_CLARIFICATION" || ambiguous.DeviceContext != nil {
		t.Fatal("unresolved context silently reused prior target")
	}
	for _, r := range []domain.Response{next, generic, switched} {
		awaitResponse(t, s, r.ID)
	}
	stop()
	s, stop = startServer(t, path, cm, "REPLAY")
	defer stop()
	before := modelCalls.Load()
	repeated := keyedMessage(t, s, cid, "same-key", input, http.StatusAccepted)
	if repeated.ID != first.ID || repeated.ContextRevision != first.ContextRevision || modelCalls.Load() != before {
		t.Fatal("restart failed to recover original idempotent response")
	}
}

func readEvents(t *testing.T, s *httptest.Server, responseID string, after int64) ([]domain.ResponseEvent, string) {
	t.Helper()
	req, _ := http.NewRequest("GET", s.URL+"/v1/responses/"+responseID+"/events", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Last-Event-ID", strconv.FormatInt(after, 10))
	res, err := s.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK || res.Header.Get("Content-Type") != "text/event-stream" {
		t.Fatalf("invalid SSE response: %d", res.StatusCode)
	}
	raw, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	var events []domain.ResponseEvent
	for _, line := range strings.Split(string(raw), "\n") {
		if strings.HasPrefix(line, "data: ") {
			var event domain.ResponseEvent
			if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &event); err != nil {
				t.Fatal(err)
			}
			events = append(events, event)
		}
	}
	return events, string(raw)
}

func TestSSEDisconnectReplaysPersistedValidatedEvents(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	external := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			close(started)
		}
		select {
		case <-release:
			modelReply(w, `{"claims":[],"gaps":["未校验的建议不会流出。"]}`)
		case <-r.Context().Done():
		}
	}))
	defer external.Close()
	cm, err := chatmodel.NewOpenAI(external.URL, "fixture", "")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(tempDir(t), "state.json")
	s, stop := startServer(t, path, cm, "REPLAY")
	c := request(t, s.Client(), "POST", s.URL+"/v1/conversations", map[string]any{}, http.StatusCreated)
	r := keyedMessage(t, s, c["id"].(string), "one-task", domain.MessageInput{Text: "请检查状态"}, http.StatusAccepted)
	<-started
	req, _ := http.NewRequest("GET", s.URL+"/v1/responses/"+r.ID+"/events", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	res, err := s.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	scanner := bufio.NewScanner(res.Body)
	first := ""
	for scanner.Scan() {
		first += scanner.Text() + "\n"
		if scanner.Text() == "" {
			break
		}
	}
	res.Body.Close()
	if !strings.Contains(first, "event: accepted") || strings.Contains(first, "未校验") {
		t.Fatalf("unexpected early stream: %s", first)
	}
	close(release)
	awaitResponse(t, s, r.ID)
	all, _ := readEvents(t, s, r.ID, 0)
	if len(all) != 3 || all[0].Type != "accepted" || all[1].Type != "progress" ||
		all[2].Type != "answer" || all[2].Response == nil || calls.Load() != 1 {
		t.Fatalf("event lifecycle incorrect: %+v calls=%d", all, calls.Load())
	}
	stop()
	s, stop = startServer(t, path, cm, "REPLAY")
	defer stop()
	remaining, raw := readEvents(t, s, r.ID, 1)
	if len(remaining) != 2 || remaining[0].ID != 2 || remaining[1].ID != 3 ||
		strings.Contains(raw, "event: accepted") || remaining[1].Response.ID != r.ID {
		t.Fatalf("SSE replay incorrect: %s", raw)
	}
}

func TestExpiredQueuedMessageReportsTotalDeadline(t *testing.T) {
	path := filepath.Join(tempDir(t), "state.json")
	store, err := filestore.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := store.CreateConversation(ctx, domain.Conversation{SchemaVersion: 1, ID: "expired-conversation"}); err != nil {
		t.Fatal(err)
	}
	_, err = store.CreateResponse(ctx, domain.Response{SchemaVersion: 1, ID: "expired-response",
		ConversationID: "expired-conversation", Question: "E42", Status: "QUEUED", DataMode: "REPLAY",
		CreatedAt: time.Now().Add(-2 * time.Minute)}, -1)
	if err != nil {
		t.Fatal(err)
	}
	store.Close()
	s, stop := startServer(t, path, &chatmodel.Replay{}, "REPLAY")
	defer stop()
	got := awaitResponse(t, s, "expired-response")
	if got.Status != "FAILED" || got.Error == nil || got.Error.Code != "DEADLINE_EXCEEDED" {
		t.Fatalf("queue time reset the request budget: %+v", got)
	}
	events, _ := readEvents(t, s, got.ID, 0)
	if events[len(events)-1].Type != "failed" {
		t.Fatal("deadline failure not in replayable event stream")
	}
}
