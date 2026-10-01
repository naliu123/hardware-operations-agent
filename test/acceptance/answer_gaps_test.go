package acceptance_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/cloudwego/eino/schema"

	"hwops/internal/adapters/chatmodel"
)

func TestUnresolvedAnswerPreservesSpecificKnowledgeGap(t *testing.T) {
	modelServer := httptest.NewServer(replayKnowledgeChoice(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []any{map[string]any{"message": map[string]any{
				"content": `{"claims":[],"gaps":["缺少该设备的实时温度读数，无法确认当前温度。"]}`,
			}}},
		})
	}))
	defer modelServer.Close()
	cm, err := chatmodel.NewOpenAI(modelServer.URL, "fixture-model", "")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(tempDir(t), "state.json")
	s, closeServer := startServer(t, path, cm, "REPLAY")
	addRevision(t, s, "GENERAL", "PUBLISH")
	conversation := request(t, s.Client(), "POST", s.URL+"/v1/conversations", map[string]any{}, http.StatusCreated)
	pending := request(t, s.Client(), "POST", s.URL+"/v1/conversations/"+conversation["id"].(string)+"/messages",
		map[string]any{"text": "蓝灯设备当前温度是多少？"}, http.StatusAccepted)
	answer := awaitResponse(t, s, pending["id"].(string))
	closeServer()
	s, closeServer = startServer(t, path, cm, "REPLAY")
	defer closeServer()
	persisted := awaitResponse(t, s, answer.ID)
	if persisted.Status != "UNRESOLVED" || persisted.Answer != "" || len(persisted.Citations) != 0 ||
		len(persisted.Gaps) != 1 || persisted.Gaps[0] != "缺少该设备的实时温度读数，无法确认当前温度。" {
		t.Fatalf("specific knowledge gap lost through HTTP and restart: %+v", persisted)
	}
}

func TestPartialAnswerKeepsSupportedClaimAndSpecificGap(t *testing.T) {
	modelServer := httptest.NewServer(replayKnowledgeChoice(func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			Messages []*schema.Message `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil || len(input.Messages) < 2 {
			http.Error(w, "invalid model request", 400)
			return
		}
		var payload chatmodel.ContextInput
		if err := json.Unmarshal([]byte(input.Messages[1].Content), &payload); err != nil || len(payload.Documents) == 0 {
			http.Error(w, "missing knowledge", 400)
			return
		}
		content, _ := json.Marshal(map[string]any{
			"claims": []any{map[string]any{"text": "蓝灯表示维护模式。", "fragment_ids": []string{payload.Documents[0].ID}}},
			"gaps":   []string{"没有实时温度读数，不能确认当前温度。"},
		})
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
	addRevision(t, s, "GENERAL", "PUBLISH")
	conversation := request(t, s.Client(), "POST", s.URL+"/v1/conversations", map[string]any{}, http.StatusCreated)
	pending := request(t, s.Client(), "POST", s.URL+"/v1/conversations/"+conversation["id"].(string)+"/messages",
		map[string]any{"text": "蓝灯表示什么，设备当前温度是多少？"}, http.StatusAccepted)
	answer := awaitResponse(t, s, pending["id"].(string))
	if answer.Status != "PARTIAL" || answer.Answer != "蓝灯表示维护模式。" ||
		len(answer.Gaps) != 1 || answer.Gaps[0] != "没有实时温度读数，不能确认当前温度。" || len(answer.Citations) != 1 {
		t.Fatalf("partial answer lost evidence or gap: %+v", answer)
	}
	fragment := request(t, s.Client(), "GET", s.URL+answer.Citations[0].URL, nil, http.StatusOK)
	if fragment["content_hash"] != answer.Citations[0].ContentHash {
		t.Fatalf("partial answer citation is not retrievable: %v", fragment)
	}
}
