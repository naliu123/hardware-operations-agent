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

// Sections such as "前提条件" are ambiguous without their document title.
// The external adapter echoes provenance; the real HTTP/store/retrieval flow
// must supply it even when the matched fragment does not contain the title.
func TestAnswerRetainsDocumentTitleForAmbiguousSection(t *testing.T) {
	modelServer := httptest.NewServer(replayKnowledgeChoice(func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			Messages []*schema.Message `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil || len(input.Messages) < 2 {
			http.Error(w, "invalid model request", 400)
			return
		}
		var payload chatmodel.ContextInput
		if err := json.Unmarshal([]byte(input.Messages[1].Content), &payload); err != nil {
			http.Error(w, "invalid context", 400)
			return
		}
		claims := []map[string]any{}
		for _, doc := range payload.Documents {
			if strings.Contains(doc.Content, "蓝灯") {
				title, _ := doc.MetaData["title"].(string)
				claims = append(claims, map[string]any{
					"text": title + "：蓝灯规则。",
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
	for _, title := range []string{"旧设备手册（已弃用）", "新设备手册"} {
		revision := request(t, s.Client(), "POST", s.URL+"/v1/knowledge/revisions", map[string]any{
			"title": title, "source": "fixture://title-provenance",
			"content": "# 前提条件\n蓝灯仅在该手册指定设备中表示维护模式。",
			"applicability": map[string]any{"scope": "GENERAL"},
		}, http.StatusCreated)
		request(t, s.Client(), "POST", s.URL+"/v1/knowledge/revisions/"+revision["id"].(string)+"/publication",
			map[string]any{"decision": "PUBLISH"}, http.StatusOK)
	}
	conversation := request(t, s.Client(), "POST", s.URL+"/v1/conversations", map[string]any{}, http.StatusCreated)
	pending := request(t, s.Client(), "POST", s.URL+"/v1/conversations/"+conversation["id"].(string)+"/messages",
		map[string]any{"text": "蓝灯规则在两份手册中分别是什么？"}, http.StatusAccepted)
	answer := awaitResponse(t, s, pending["id"].(string))
	for _, want := range []string{"旧设备手册（已弃用）：蓝灯规则。", "新设备手册：蓝灯规则。"} {
		if !strings.Contains(answer.Answer, want) {
			t.Fatalf("ambiguous section lost source title: status=%s answer=%q", answer.Status, answer.Answer)
		}
	}
	if len(answer.Citations) != 2 || answer.DataMode != "REPLAY" {
		t.Fatalf("expected two independently traceable REPLAY sources: %+v", answer)
	}
}
