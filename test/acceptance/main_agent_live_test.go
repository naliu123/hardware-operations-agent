package acceptance_test

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"hwops/internal/adapters/chatmodel"
)

// Opt-in integration: real remote model, public HTTP, isolated file store.
// The published manual is synthetic; this does not measure RAG answer quality.
func TestLiveMainAgentKnowledgeTool(t *testing.T) {
	if os.Getenv("HWOPS_TEST_LIVE_MODEL") != "1" {
		t.Skip("LIVE model integration requires HWOPS_TEST_LIVE_MODEL=1")
	}
	endpoint, name := os.Getenv("HWOPS_MODEL_ENDPOINT"), os.Getenv("HWOPS_MODEL")
	if endpoint == "" || name == "" {
		t.Fatal("LIVE integration requires HWOPS_MODEL_ENDPOINT and HWOPS_MODEL")
	}
	cm, err := chatmodel.NewOpenAI(endpoint, name, os.Getenv("HWOPS_MODEL_API_KEY"))
	if err != nil {
		t.Fatal(err)
	}
	s, closeServer := startServer(t, filepath.Join(tempDir(t), "live-state.json"), cm, "LIVE")
	defer closeServer()
	revisionID := addRevision(t, s, "GENERAL", "PUBLISH")
	conversation := request(t, s.Client(), "POST", s.URL+"/v1/conversations", map[string]any{}, http.StatusCreated)
	pending := request(t, s.Client(), "POST", s.URL+"/v1/conversations/"+conversation["id"].(string)+"/messages",
		map[string]any{"text": "根据合成手册，测试设备的蓝灯表示什么？"}, http.StatusAccepted)
	answer := awaitResponseWithin(t, s, pending["id"].(string), 65*time.Second)
	if answer.DataMode != "LIVE" || answer.Status != "ANSWERED" || len(answer.Citations) != 1 ||
		answer.Citations[0].RevisionID != revisionID || !strings.Contains(answer.Answer, "维护模式") ||
		len(answer.KnowledgeToolCalls) == 0 || len(answer.KnowledgeToolCalls) > 3 ||
		answer.ModelUsage == nil || answer.ModelUsage.Calls < 2 {
		t.Fatalf("LIVE tool flow failed: status=%s error=%+v tool_calls=%+v answer=%s",
			answer.Status, answer.Error, answer.KnowledgeToolCalls, answer.Answer)
	}
	for _, call := range answer.KnowledgeToolCalls {
		if call.ID == "" || call.Request == "" || call.Error != nil {
			t.Fatalf("LIVE tool call lacks successful audit: %+v", call)
		}
	}
	original := request(t, s.Client(), "GET", s.URL+answer.Citations[0].URL, nil, http.StatusOK)
	if original["content_hash"] != answer.Citations[0].ContentHash {
		t.Fatal("LIVE answer citation differs from authoritative source")
	}
	t.Logf("LIVE model=%s status=%s knowledge_calls=%d model_calls=%d total_tokens=%d",
		name, answer.Status, len(answer.KnowledgeToolCalls), answer.ModelUsage.Calls, answer.ModelUsage.TotalTokens)
}
