package acceptance_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/cloudwego/eino/schema"

	"hwops/internal/adapters/chatmodel"
	"hwops/internal/adapters/elasticsearch"
	"hwops/internal/adapters/filestore"
	"hwops/internal/application"
	"hwops/internal/transport/httpapi"
)

func TestAnswerSelectsDirectEvidenceAndAdjacentOverview(t *testing.T) {
	testEvidenceSelection(t, "")
}

func TestRejectedEvidenceSelectionRetainsUsageWithoutPublishingAnswer(t *testing.T) {
	for _, invalid := range []string{"unknown", "duplicate", "too-many"} {
		t.Run(invalid, func(t *testing.T) { testEvidenceSelection(t, invalid) })
	}
}

func TestEvidenceSelectionRepairsMalformedModelOutputAndRetainsUsage(t *testing.T) {
	testEvidenceSelection(t, "malformed-once")
}

func TestEvidenceSelectionRejectsSecondMalformedOutputAndRetainsUsage(t *testing.T) {
	testEvidenceSelection(t, "malformed-twice")
}

func testEvidenceSelection(t *testing.T, invalid string) {
	t.Helper()
	var mu sync.Mutex
	var selectionCalls atomic.Int32
	indexed := map[string]map[string]any{}
	external := httptest.NewServer(replayKnowledgeChoice(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if r.URL.Path == "/v1/embeddings" {
			data := []any{}
			for i := range body["input"].([]any) {
				vector := make([]float64, 512)
				vector[0] = 1
				data = append(data, map[string]any{"index": i, "embedding": vector})
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"data": data})
			return
		}
		if r.URL.Path == "/chat" {
			raw, _ := json.Marshal(body["messages"])
			var messages []*schema.Message
			_ = json.Unmarshal(raw, &messages)
			var payload chatmodel.ContextInput
			_ = json.Unmarshal([]byte(messages[1].Content), &payload)
			ids := []string{}
			claims := []map[string]any{}
			for _, doc := range payload.Documents {
				if strings.Contains(doc.Content, "创建负载时选择命名空间") ||
					strings.Contains(doc.Content, "按环境隔离") {
					ids = append(ids, doc.ID)
					claims = append(claims, map[string]any{"text": doc.Content, "fragment_ids": []string{doc.ID}})
				}
			}
			var output any = map[string]any{"claims": claims, "gaps": []string{}}
			if strings.Contains(messages[0].Content, `"selected_fragment_ids"`) {
				call := selectionCalls.Add(1)
				switch invalid {
				case "unknown":
					ids = []string{"invented-fragment"}
				case "duplicate":
					ids = []string{ids[0], ids[0]}
				case "too-many":
					ids = []string{ids[0], ids[1], ids[0], ids[1], ids[0], ids[1]}
				case "malformed-once", "malformed-twice":
					if call == 1 || invalid == "malformed-twice" {
						_ = json.NewEncoder(w).Encode(map[string]any{
							"choices": []any{map[string]any{"message": map[string]any{"content": "not json"}}},
							"usage":   map[string]int{"prompt_tokens": 10, "completion_tokens": 5, "total_tokens": 15},
						})
						return
					}
				}
				output = map[string]any{"selected_fragment_ids": ids}
			}
			content, _ := json.Marshal(output)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"choices": []any{map[string]any{"message": map[string]any{"content": string(content)}}},
				"usage":   map[string]int{"prompt_tokens": 10, "completion_tokens": 5, "total_tokens": 15},
			})
			return
		}
		mu.Lock()
		defer mu.Unlock()
		if r.Method == "PUT" && strings.HasPrefix(r.URL.Path, "/selection-test/_doc/") {
			indexed[body["section"].(string)] = body
		}
		if strings.HasSuffix(r.URL.Path, "/_search") {
			order := []string{"背景1", "背景2", "背景3", "背景4", "背景5", "直接依据"}
			hits := []any{}
			for i, section := range order {
				doc := indexed[section]
				hits = append(hits, map[string]any{"_id": doc["fragment_id"], "_score": 100 - i, "_source": doc})
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"hits": map[string]any{"hits": hits}})
			return
		}
		_, _ = w.Write([]byte(`{}`))
	}))
	defer external.Close()
	backend, err := elasticsearch.New(external.URL, external.URL, "selection-test", "hybrid", 1)
	if err != nil {
		t.Fatal(err)
	}
	cm, err := chatmodel.NewOpenAI(external.URL+"/chat", "fixture-model", "")
	if err != nil {
		t.Fatal(err)
	}
	store, err := filestore.Open(filepath.Join(tempDir(t), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	app, err := application.New(store, cm, "REPLAY",
		application.Options{Retriever: backend, ContextLimit: 5, EvidenceSelection: true})
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close()
	s := httptest.NewServer(httpapi.New(app, token))
	defer s.Close()
	content := "# 概述\n创建负载时选择命名空间。\n"
	for i := 1; i <= 5; i++ {
		content += fmt.Sprintf("# 背景%d\n相关操作背景。\n", i)
	}
	revision := request(t, s.Client(), "POST", s.URL+"/v1/knowledge/revisions", map[string]any{
		"title": "命名空间手册", "source": "fixture://namespace", "content": content,
		"applicability": map[string]any{"scope": "GENERAL"},
	}, http.StatusCreated)
	request(t, s.Client(), "POST", s.URL+"/v1/knowledge/revisions/"+revision["id"].(string)+"/publication",
		map[string]any{"decision": "PUBLISH"}, http.StatusOK)
	direct := request(t, s.Client(), "POST", s.URL+"/v1/knowledge/revisions", map[string]any{
		"title": "环境隔离", "source": "fixture://environment", "content": "# 直接依据\n命名空间按环境隔离。\n",
		"applicability": map[string]any{"scope": "GENERAL"},
	}, http.StatusCreated)
	request(t, s.Client(), "POST", s.URL+"/v1/knowledge/revisions/"+direct["id"].(string)+"/publication",
		map[string]any{"decision": "PUBLISH"}, http.StatusOK)
	conversation := request(t, s.Client(), "POST", s.URL+"/v1/conversations", map[string]any{}, http.StatusCreated)
	pending := request(t, s.Client(), "POST", s.URL+"/v1/conversations/"+conversation["id"].(string)+"/messages",
		map[string]any{"text": "如何按环境隔离工作负载，并在创建时指定环境？"}, http.StatusAccepted)
	answer := awaitResponse(t, s, pending["id"].(string))
	if invalid != "" && invalid != "malformed-once" {
		expectedTokens, expectedCalls := 15, 1
		if invalid == "malformed-twice" {
			expectedTokens, expectedCalls = 30, 2
		}
		if answer.Status != "FAILED" || answer.Answer != "" || len(answer.Citations) != 0 ||
			answer.ModelUsage == nil || answer.ModelUsage.TotalTokens != expectedTokens ||
			answer.ModelUsage.Calls != expectedCalls || answer.EvidenceSelection == nil ||
			len(answer.ApplicabilityChecks) != 2 {
			t.Fatalf("rejected selection must publish no answer and retain consumed usage: %+v", answer)
		}
		return
	}
	for _, want := range []string{"按环境隔离", "创建负载时选择命名空间"} {
		if !strings.Contains(answer.Answer, want) {
			t.Fatalf("missing direct evidence %q: status=%s answer=%q", want, answer.Status, answer.Answer)
		}
	}
	expectedTokens, expectedCalls := 30, 2
	if invalid == "malformed-once" {
		expectedTokens, expectedCalls = 45, 3
	}
	if len(answer.Citations) != 2 || len(answer.RetrievedFragmentIDs) > 5 ||
		answer.ModelUsage == nil || answer.ModelUsage.TotalTokens != expectedTokens ||
		answer.ModelUsage.Calls != expectedCalls {
		t.Fatalf("selection must retain citations, final limit and total model usage: %+v", answer)
	}
	// Candidate expansion is internal to the knowledge sub-agent; the public search contract is unchanged.
	search := request(t, s.Client(), "POST", s.URL+"/v1/knowledge/search",
		map[string]any{"query": "环境隔离", "top_k": 5}, http.StatusOK)
	if len(search["documents"].([]any)) != 5 {
		t.Fatalf("public search changed its limit: %+v", search)
	}
}
