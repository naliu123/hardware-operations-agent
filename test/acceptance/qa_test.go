package acceptance_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"

	"hwops/internal/adapters/chatmodel"
	"hwops/internal/adapters/filestore"
	"hwops/internal/application"
	"hwops/internal/domain"
	"hwops/internal/transport/httpapi"
)

const token = "acceptance-token"

func tempDir(t *testing.T) string {
	t.Helper()
	if os.Getenv("HWOPS_KEEP_TEST_ARTIFACTS") != "1" {
		return t.TempDir()
	}
	path, err := os.MkdirTemp("", "hwops-test-")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("preserved test artifacts: %s", path)
	return path
}

func request(t *testing.T, client *http.Client, method, url string, body any, expected int) map[string]any {
	t.Helper()
	var data io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		data = bytes.NewReader(encoded)
	}
	req, err := http.NewRequest(method, url, data)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	res, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var result map[string]any
	if err := json.NewDecoder(res.Body).Decode(&result); err != nil {
		t.Fatalf("decode %s: %v", url, err)
	}
	if res.StatusCode != expected {
		t.Fatalf("%s %s: status %d, want %d: %v", method, url, res.StatusCode, expected, result)
	}
	return result
}

func awaitResponse(t *testing.T, server *httptest.Server, id string) domain.Response {
	t.Helper()
	return awaitResponseWithin(t, server, id, 5*time.Second)
}

func awaitResponseWithin(t *testing.T, server *httptest.Server, id string, timeout time.Duration) domain.Response {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		body := request(t, server.Client(), "GET", server.URL+"/v1/responses/"+id, nil, http.StatusOK)
		raw, _ := json.Marshal(body)
		var response domain.Response
		if err := json.Unmarshal(raw, &response); err != nil {
			t.Fatal(err)
		}
		if response.Status != "QUEUED" && response.Status != "RUNNING" {
			return response
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("response did not complete")
	return domain.Response{}
}

func TestPublishedKnowledgeAnswersWithRetrievableCitation(t *testing.T) {
	path := filepath.Join(tempDir(t), "state.json")
	store, err := filestore.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	app, err := application.New(store, &chatmodel.Replay{}, "REPLAY")
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(httpapi.New(app, token))
	defer func() {
		server.Close()
		app.Close()
		store.Close()
	}()
	revision := request(t, server.Client(), "POST", server.URL+"/v1/knowledge/revisions", map[string]any{
		"title": "测试运维手册", "source": "fixture://manual",
		"content":       "# 指示灯\n测试设备的蓝灯表示维护模式。本资料为合成测试内容。",
		"applicability": map[string]any{"scope": "GENERAL"},
	}, http.StatusCreated)
	id := revision["id"].(string)
	request(t, server.Client(), "POST", server.URL+"/v1/knowledge/revisions/"+id+"/publication",
		map[string]any{"decision": "PUBLISH"}, http.StatusOK)
	conversation := request(t, server.Client(), "POST", server.URL+"/v1/conversations", map[string]any{}, http.StatusCreated)
	pending := request(t, server.Client(), "POST", server.URL+"/v1/conversations/"+conversation["id"].(string)+"/messages",
		map[string]any{"text": "蓝灯表示什么？"}, http.StatusAccepted)
	answer := awaitResponse(t, server, pending["id"].(string))
	if answer.Status != "ANSWERED" || answer.DataMode != "REPLAY" || len(answer.Citations) != 1 {
		t.Fatalf("unexpected answer: %+v", answer)
	}
	citation := answer.Citations[0]
	fragment := request(t, server.Client(), "GET", server.URL+citation.URL, nil, http.StatusOK)
	if fragment["content"] != "测试设备的蓝灯表示维护模式。本资料为合成测试内容。" {
		t.Fatalf("wrong cited content: %v", fragment)
	}
	if citation.RevisionID != id || citation.Section != "指示灯" || len(citation.ContentHash) != 64 {
		t.Fatalf("citation lost provenance: %+v", citation)
	}
}

func startServer(t *testing.T, path string, cm model.BaseChatModel, mode string) (*httptest.Server, func()) {
	t.Helper()
	store, err := filestore.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	app, err := application.New(store, cm, mode)
	if err != nil {
		store.Close()
		t.Fatal(err)
	}
	server := httptest.NewServer(httpapi.New(app, token))
	return server, func() {
		server.Close()
		app.Close()
		if err := store.Close(); err != nil {
			t.Error(err)
		}
	}
}

func addRevision(t *testing.T, s *httptest.Server, scope, decision string) string {
	t.Helper()
	applicability := map[string]any{"scope": scope}
	if scope == "DEVICE" {
		applicability["model"] = "test-device"
	}
	revision := request(t, s.Client(), "POST", s.URL+"/v1/knowledge/revisions", map[string]any{
		"title": "合成手册", "source": "fixture://manual",
		"content":       "# 指示灯\n测试设备的蓝灯表示维护模式。本资料为合成测试内容。",
		"applicability": applicability,
	}, http.StatusCreated)
	id := revision["id"].(string)
	if decision != "" {
		request(t, s.Client(), "POST", s.URL+"/v1/knowledge/revisions/"+id+"/publication",
			map[string]any{"decision": decision}, http.StatusOK)
	}
	return id
}

func ask(t *testing.T, s *httptest.Server) domain.Response {
	t.Helper()
	conversation := request(t, s.Client(), "POST", s.URL+"/v1/conversations", map[string]any{}, http.StatusCreated)
	pending := request(t, s.Client(), "POST", s.URL+"/v1/conversations/"+conversation["id"].(string)+"/messages",
		map[string]any{"text": "蓝灯表示什么？"}, http.StatusAccepted)
	return awaitResponse(t, s, pending["id"].(string))
}

func TestWithdrawnCitationRemainsTraceableAfterRestart(t *testing.T) {
	path := filepath.Join(tempDir(t), "state.json")
	id, answer := func() (string, domain.Response) {
		s, closeServer := startServer(t, path, &chatmodel.Replay{}, "REPLAY")
		defer closeServer()
		id := addRevision(t, s, "GENERAL", "PUBLISH")
		answer := ask(t, s)
		if answer.Status != "ANSWERED" || len(answer.Citations) != 1 {
			t.Fatalf("unexpected answer: %+v", answer)
		}
		request(t, s.Client(), "POST", s.URL+"/v1/knowledge/revisions/"+id+"/publication",
			map[string]any{"decision": "WITHDRAW"}, http.StatusOK)
		return id, answer
	}()
	s, closeServer := startServer(t, path, &chatmodel.Replay{}, "REPLAY")
	defer closeServer()
	persisted := awaitResponse(t, s, answer.ID)
	if persisted.Answer != answer.Answer || len(persisted.Citations) != 1 || persisted.Citations[0].RevisionID != id {
		t.Fatalf("answer not preserved: %+v", persisted)
	}
	fragment := request(t, s.Client(), "GET", s.URL+persisted.Citations[0].URL, nil, http.StatusOK)
	if fragment["publication_status"] != "WITHDRAWN" || fragment["start_line"] != float64(2) ||
		fragment["source"] != "fixture://manual" || fragment["content"] == "" {
		t.Fatalf("historical citation lacks original location or current publication state: %v", fragment)
	}
	if response := ask(t, s); response.Status != "UNRESOLVED" || len(response.Citations) != 0 {
		t.Fatalf("withdrawn material used in a new answer: %+v", response)
	}
}

func TestOnlyPublishedGeneralKnowledgeCanAnswer(t *testing.T) {
	for _, scenario := range []struct{ name, scope, decision, status string }{
		{"draft", "GENERAL", "", "UNRESOLVED"},
		{"withdrawn", "GENERAL", "WITHDRAW", "UNRESOLVED"},
		{"device_specific", "DEVICE", "PUBLISH", "NEEDS_CLARIFICATION"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			s, closeServer := startServer(t, filepath.Join(tempDir(t), "state.json"), &chatmodel.Replay{}, "REPLAY")
			defer closeServer()
			addRevision(t, s, scenario.scope, scenario.decision)
			answer := ask(t, s)
			if answer.Status != scenario.status || answer.Answer != "" || len(answer.Citations) != 0 || len(answer.Gaps) == 0 {
				t.Fatalf("ineligible knowledge was used: %+v", answer)
			}
		})
	}
}

func TestUnavailableModelDoesNotProduceReplayAnswer(t *testing.T) {
	s, closeServer := startServer(t, filepath.Join(tempDir(t), "state.json"), &chatmodel.Unconfigured{}, "LIVE")
	defer closeServer()
	addRevision(t, s, "GENERAL", "PUBLISH")
	answer := ask(t, s)
	if answer.Status != "FAILED" || answer.DataMode != "LIVE" || answer.Error == nil ||
		answer.Error.Code != "MODEL_UNAVAILABLE" || answer.Answer != "" || len(answer.Citations) != 0 {
		t.Fatalf("model failure was hidden: %+v", answer)
	}
}

func TestTransientModelFailureRecoversThroughPublicHTTP(t *testing.T) {
	var requests atomic.Int32
	modelServer := httptest.NewServer(replayKnowledgeChoice(func(w http.ResponseWriter, r *http.Request) {
		if requests.Add(1) == 1 {
			w.Header().Set("Retry-After", "0")
			http.Error(w, "temporary overload", http.StatusServiceUnavailable)
			return
		}
		var input struct {
			Messages []*schema.Message `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil || len(input.Messages) < 2 {
			http.Error(w, "invalid model input", http.StatusBadRequest)
			return
		}
		var payload chatmodel.ContextInput
		if err := json.Unmarshal([]byte(input.Messages[1].Content), &payload); err != nil || len(payload.Documents) == 0 {
			http.Error(w, "missing knowledge", http.StatusBadRequest)
			return
		}
		content, _ := json.Marshal(domain.Draft{Claims: []domain.Claim{
			{Text: "蓝灯表示维护模式。", FragmentIDs: []string{payload.Documents[0].ID}},
		}})
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []any{map[string]any{"message": map[string]any{"content": string(content)}}},
			"usage":   map[string]int{"prompt_tokens": 11, "completion_tokens": 7, "total_tokens": 18},
		})
	}))
	defer modelServer.Close()
	cm, err := chatmodel.NewOpenAI(modelServer.URL, "fixture-model", "")
	if err != nil {
		t.Fatal(err)
	}
	s, closeServer := startServer(t, filepath.Join(tempDir(t), "state.json"), cm, "REPLAY")
	defer closeServer()
	addRevision(t, s, "GENERAL", "PUBLISH")

	answer := ask(t, s)
	if answer.Status != "ANSWERED" || answer.DataMode != "REPLAY" ||
		answer.Answer != "蓝灯表示维护模式。" || len(answer.Citations) != 1 ||
		answer.ModelUsage == nil || answer.ModelUsage.TotalTokens != 18 || requests.Load() != 2 {
		t.Fatalf("public answer did not recover from transient model failure: requests=%d answer=%+v",
			requests.Load(), answer)
	}
}

func TestInventedCitationsMustBeRepairedBeforeAnswering(t *testing.T) {
	for _, canRepair := range []bool{false, true} {
		name := "rejected"
		if canRepair {
			name = "repaired"
		}
		t.Run(name, func(t *testing.T) {
			var attempted atomic.Bool
			modelServer := httptest.NewServer(replayKnowledgeChoice(func(w http.ResponseWriter, r *http.Request) {
				content := `{"claims":[{"text":"无效结论","fragment_ids":["invented-fragment"]}]}`
				if attempted.Swap(true) && canRepair {
					var input struct {
						Messages []*schema.Message `json:"messages"`
					}
					if err := json.NewDecoder(r.Body).Decode(&input); err != nil || len(input.Messages) < 2 {
						t.Error("model request lacked conversation input")
						http.Error(w, "bad request", http.StatusBadRequest)
						return
					}
					var payload chatmodel.ContextInput
					if err := json.Unmarshal([]byte(input.Messages[1].Content), &payload); err != nil || len(payload.Documents) == 0 {
						t.Error("model request lacked retrieved fragments")
						http.Error(w, "bad request", http.StatusBadRequest)
						return
					}
					raw, _ := json.Marshal(domain.Draft{Claims: []domain.Claim{
						{Text: "蓝灯表示维护模式。", FragmentIDs: []string{payload.Documents[0].ID}},
					}})
					content = string(raw)
				}
				_ = json.NewEncoder(w).Encode(map[string]any{
					"choices": []any{map[string]any{"message": map[string]any{"content": content}}},
				})
			}))
			defer modelServer.Close()
			cm, err := chatmodel.NewOpenAI(modelServer.URL, "fixture-model", "")
			if err != nil {
				t.Fatal(err)
			}
			s, closeServer := startServer(t, filepath.Join(tempDir(t), "state.json"), cm, "REPLAY")
			defer closeServer()
			addRevision(t, s, "GENERAL", "PUBLISH")
			answer := ask(t, s)
			if canRepair {
				if answer.Status != "ANSWERED" || answer.Answer != "蓝灯表示维护模式。" || len(answer.Citations) != 1 {
					t.Fatalf("valid repair was not published: %+v", answer)
				}
			} else if answer.Status != "FAILED" || answer.Error == nil || answer.Error.Code != "INVALID_MODEL_OUTPUT" ||
				answer.Answer != "" || len(answer.Citations) != 0 {
				t.Fatalf("invented reference was published: %+v", answer)
			}
		})
	}
}

func TestWithdrawalDuringGenerationDropsAnswer(t *testing.T) {
	started, released := make(chan struct{}, 1), make(chan struct{})
	modelServer := httptest.NewServer(replayKnowledgeChoice(func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			Messages []*schema.Message `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil || len(input.Messages) < 2 {
			http.Error(w, "invalid model input", http.StatusBadRequest)
			return
		}
		var payload chatmodel.ContextInput
		if err := json.Unmarshal([]byte(input.Messages[1].Content), &payload); err != nil || len(payload.Documents) == 0 {
			http.Error(w, "no documents", http.StatusBadRequest)
			return
		}
		started <- struct{}{}
		select {
		case <-released:
		case <-r.Context().Done():
			return
		}
		content, _ := json.Marshal(domain.Draft{Claims: []domain.Claim{
			{Text: "蓝灯表示维护模式。", FragmentIDs: []string{payload.Documents[0].ID}},
		}})
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []any{map[string]any{"message": map[string]any{"content": string(content)}}},
		})
	}))
	defer modelServer.Close()
	cm, err := chatmodel.NewOpenAI(modelServer.URL, "fixture-model", "")
	if err != nil {
		t.Fatal(err)
	}
	s, closeServer := startServer(t, filepath.Join(tempDir(t), "state.json"), cm, "REPLAY")
	defer closeServer()
	id := addRevision(t, s, "GENERAL", "PUBLISH")
	conversation := request(t, s.Client(), "POST", s.URL+"/v1/conversations", map[string]any{}, http.StatusCreated)
	pending := request(t, s.Client(), "POST", s.URL+"/v1/conversations/"+conversation["id"].(string)+"/messages",
		map[string]any{"text": "蓝灯表示什么？"}, http.StatusAccepted)
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("generation did not start")
	}
	request(t, s.Client(), "POST", s.URL+"/v1/knowledge/revisions/"+id+"/publication",
		map[string]any{"decision": "WITHDRAW"}, http.StatusOK)
	close(released)
	answer := awaitResponse(t, s, pending["id"].(string))
	if answer.Status != "UNRESOLVED" || answer.Answer != "" || len(answer.Citations) != 0 {
		t.Fatalf("answer used knowledge withdrawn during generation: %+v", answer)
	}
}

func TestInterruptedQuestionResumesAfterRestart(t *testing.T) {
	started := make(chan struct{}, 1)
	modelServer := httptest.NewServer(replayKnowledgeChoice(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		started <- struct{}{}
		<-r.Context().Done()
	}))
	defer modelServer.Close()
	cm, err := chatmodel.NewOpenAI(modelServer.URL, "fixture-model", "")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(tempDir(t), "state.json")
	responseID := func() string {
		s, closeServer := startServer(t, path, cm, "REPLAY")
		defer closeServer()
		addRevision(t, s, "GENERAL", "PUBLISH")
		conversation := request(t, s.Client(), "POST", s.URL+"/v1/conversations", map[string]any{}, http.StatusCreated)
		pending := request(t, s.Client(), "POST", s.URL+"/v1/conversations/"+conversation["id"].(string)+"/messages",
			map[string]any{"text": "蓝灯表示什么？"}, http.StatusAccepted)
		select {
		case <-started:
		case <-time.After(3 * time.Second):
			t.Fatal("generation did not start")
		}
		return pending["id"].(string)
	}()
	s, closeServer := startServer(t, path, &chatmodel.Replay{}, "REPLAY")
	defer closeServer()
	answer := awaitResponse(t, s, responseID)
	if answer.Status != "ANSWERED" || len(answer.Citations) != 1 || answer.ID != responseID {
		t.Fatalf("interrupted task was lost: %+v", answer)
	}
}

func TestHTTPAuthenticationAndInputValidation(t *testing.T) {
	s, closeServer := startServer(t, filepath.Join(tempDir(t), "state.json"), &chatmodel.Replay{}, "REPLAY")
	defer closeServer()
	res, err := s.Client().Get(s.URL + "/v1/responses/unknown")
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unauthenticated request: %d", res.StatusCode)
	}
	request(t, s.Client(), "POST", s.URL+"/v1/knowledge/revisions", map[string]any{
		"title": "missing applicability", "source": "fixture://manual", "content": "text",
	}, http.StatusBadRequest)
	request(t, s.Client(), "POST", s.URL+"/v1/conversations", map[string]any{"owner": "forged-user"}, http.StatusBadRequest)
	request(t, s.Client(), "POST", s.URL+"/v1/conversations/unknown/messages",
		map[string]any{"text": "蓝灯表示什么？"}, http.StatusNotFound)
}
