package acceptance_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cloudwego/eino/schema"

	"hwops/internal/adapters/chatmodel"
	knowledgeagent "hwops/internal/agents/knowledge"
	"hwops/internal/application"
	"hwops/internal/blobstore"
	"hwops/internal/domain"
)

func TestWB06AttachmentKnowledgeConflictAndForgedSource(t *testing.T) {
	modelServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			Messages []*schema.Message `json:"messages"`
			Stream   bool              `json:"stream"`
		}
		if json.NewDecoder(r.Body).Decode(&input) != nil || !input.Stream || len(input.Messages) < 2 {
			http.Error(w, "invalid model request", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		var payload chatmodel.ContextInput
		_ = json.Unmarshal([]byte(input.Messages[1].Content), &payload)
		var toolMessages []*schema.Message
		for _, message := range input.Messages {
			if message.Role == schema.Tool {
				toolMessages = append(toolMessages, message)
			}
		}
		if strings.Contains(payload.Question, "伪造私有来源") {
			wb06Final(w, domain.Draft{Claims: []domain.Claim{{
				Text: "伪造执行结论", SourceIDs: []string{"execution:forged:0000000000000000"},
			}}, Gaps: []string{}})
			return
		}
		switch len(toolMessages) {
		case 0:
			wb06ToolCall(w, "wb06-conflict-attachment", einofflowRead(), map[string]any{
				"attachment_id": payload.Attachments[0].ID, "page": 1,
				"start_line": 1, "end_line": 1,
			})
		case 1:
			wb06ToolCall(w, "wb06-conflict-knowledge", knowledgeagent.ToolName,
				map[string]string{"request": "蓝灯表示什么？"})
		default:
			var attachment domain.AttachmentReadResult
			_ = json.Unmarshal([]byte(toolMessages[0].Content), &attachment)
			var knowledgeResult chatmodel.ContextInput
			_ = json.Unmarshal([]byte(toolMessages[1].Content), &knowledgeResult)
			if attachment.Source == nil || len(knowledgeResult.Documents) == 0 {
				http.Error(w, "source evidence missing", http.StatusBadRequest)
				return
			}
			wb06Final(w, domain.Draft{
				Claims: []domain.Claim{},
				Conflicts: []domain.KnowledgeConflict{{
					Subject:     "本会话附件称蓝灯为故障模式，已发布手册称蓝灯为维护模式。",
					FragmentIDs: []string{knowledgeResult.Documents[0].ID},
					SourceIDs:   []string{attachment.Source.SourceID},
				}},
				Gaps: []string{},
			})
		}
	}))
	defer modelServer.Close()
	cm, _ := chatmodel.NewOpenAI(modelServer.URL, "wb06-source-replay", "")
	files, err := blobstore.Open(filepath.Join(t.TempDir(), "private"), 500_000_000, 100_000_000)
	if err != nil {
		t.Fatal(err)
	}
	server, _, stop := wbStartOptions(t, workbenchDatabase(t), cm, true, application.Options{
		UsersMode: true, AttachmentRunner: newAttachmentReplayRunner(), Files: files,
	})
	defer stop()
	client := wbNewClient(t, server)
	client.login("admin", wbPassword)
	wbPublish(client)
	conversationID := client.request("POST", "/v1/conversations", map[string]any{}, 201)["id"].(string)
	uploaded := uploadAttachment(t, client, conversationID, "local.log", "text/plain",
		[]byte("蓝灯表示故障模式。\n"), 201)
	attachment := waitAttachment(t, client, uploaded["id"].(string))
	if attachment["status"] != "READY" {
		t.Fatalf("attachment unavailable: %v", attachment)
	}
	submitted := client.request("POST", "/v1/conversations/"+conversationID+"/messages", map[string]any{
		"text": "比较附件和手册中的蓝灯含义", "attachment_ids": []string{uploaded["id"].(string)},
	}, 202)
	answer := client.answer(submitted["id"].(string), 30*time.Second)
	if answer["status"] != "UNRESOLVED" ||
		!strings.Contains(answer["gaps"].([]any)[0].(string), "资料冲突") ||
		len(answer["conflicts"].([]any)) != 1 || len(answer["citations"].([]any)) != 1 ||
		len(answer["sources"].([]any)) != 1 {
		t.Fatalf("attachment/knowledge conflict lost provenance: %v", answer)
	}

	forged := client.request("POST", "/v1/conversations/"+conversationID+"/messages",
		map[string]string{"text": "伪造私有来源"}, 202)
	failed := client.answer(forged["id"].(string), 30*time.Second)
	if failed["status"] != "FAILED" ||
		failed["error"].(map[string]any)["code"] != "INVALID_MODEL_OUTPUT" {
		t.Fatalf("forged private source was accepted: %v", failed)
	}
	t.Log("REPLAY: attachment/manual conflict retained both real sources; forged private source failed validation")
}
