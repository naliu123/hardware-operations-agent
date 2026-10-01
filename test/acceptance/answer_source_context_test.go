package acceptance_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cloudwego/eino/schema"

	"hwops/internal/adapters/chatmodel"
)

// The historical scope belongs to the document header, not the matched body
// section. It must survive generation and remain visible through the citation.
func TestAnswerAndCitationRetainHistoricalDirectory(t *testing.T) {
	testHistoricalDirectory(t, false)
}

func TestAnswerRetainsSourceScopeEvenWhenModelOmitsIt(t *testing.T) {
	testHistoricalDirectory(t, true)
}

func testHistoricalDirectory(t *testing.T, omitScope bool) {
	t.Helper()
	modelServer := httptest.NewServer(replayKnowledgeChoice(func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			Messages []*schema.Message `json:"messages"`
		}
		_ = json.NewDecoder(r.Body).Decode(&input)
		var payload chatmodel.ContextInput
		_ = json.Unmarshal([]byte(input.Messages[1].Content), &payload)
		claims := []map[string]any{}
		for _, doc := range payload.Documents {
			if strings.Contains(doc.Content, "蓝灯") {
				scope, _ := doc.MetaData["document_context"].(string)
				if omitScope {
					scope = ""
				}
				claims = append(claims, map[string]any{
					"text":         scope + "\n蓝灯表示维护模式。",
					"fragment_ids": []string{doc.ID},
				})
			}
		}
		content, _ := json.Marshal(map[string]any{"claims": claims, "gaps": []string{}})
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
	revision := request(t, s.Client(), "POST", s.URL+"/v1/knowledge/revisions", map[string]any{
		"title": "设备指示灯", "source": "fixture://historical-manual",
		"content":       "# 设备指示灯\n\n来源：fixture://historical-manual\n\n目录：用户指南 > 旧设备（已弃用） > 设备指示灯\n\n更新时间：2020-01-02\n\n## 指示规则\n蓝灯表示维护模式。",
		"applicability": map[string]any{"scope": "GENERAL"},
	}, http.StatusCreated)
	request(t, s.Client(), "POST", s.URL+"/v1/knowledge/revisions/"+revision["id"].(string)+"/publication",
		map[string]any{"decision": "PUBLISH"}, http.StatusOK)
	conversation := request(t, s.Client(), "POST", s.URL+"/v1/conversations", map[string]any{}, http.StatusCreated)
	pending := request(t, s.Client(), "POST", s.URL+"/v1/conversations/"+conversation["id"].(string)+"/messages",
		map[string]any{"text": "蓝灯规则是什么？"}, http.StatusAccepted)
	answer := awaitResponse(t, s, pending["id"].(string))
	for _, want := range []string{"旧设备（已弃用）", "2020-01-02"} {
		if !strings.Contains(answer.Answer, want) {
			t.Fatalf("lost historical scope %q: %s", want, answer.Answer)
		}
	}
	if len(answer.Citations) != 1 || answer.DataMode != "REPLAY" {
		t.Fatalf("expected one traceable REPLAY source: %+v", answer)
	}
	source := request(t, s.Client(), "GET", s.URL+answer.Citations[0].URL, nil, http.StatusOK)
	if !strings.Contains(source["document_context"].(string), "旧设备（已弃用）") ||
		strings.Contains(source["content"].(string), "旧设备（已弃用）") {
		t.Fatalf("citation must expose document context separately from immutable fragment: %+v", source)
	}
}
