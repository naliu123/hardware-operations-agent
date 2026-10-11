package acceptance_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/cloudwego/eino/schema"

	"hwops/internal/adapters/chatmodel"
	"hwops/internal/application"
	"hwops/internal/blobstore"
	"hwops/internal/domain"
	"hwops/internal/einoflow"
)

func TestToolLimitWorkbenchFinalizationKeepsGroundingAndDraftLifecycle(t *testing.T) {
	var modelCalls atomic.Int32
	generator := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := modelCalls.Add(1)
		input := readAgentRequest(t, r)
		w.Header().Set("Content-Type", "text/event-stream")
		if n == 1 || n == 2 {
			index := 0
			call := knowledgeCall("first", "蓝灯含义")
			if n == 2 {
				call = knowledgeCall("duplicate", "蓝灯含义")
			}
			call.Index = &index
			wbStreamFrame(w, map[string]any{"choices": []any{map[string]any{
				"index": 0, "delta": map[string]any{"tool_calls": []schema.ToolCall{call}},
			}}})
			wbStreamFrame(w, map[string]any{
				"choices": []any{map[string]any{"index": 0, "delta": map[string]any{}, "finish_reason": "tool_calls"}},
				"usage":   map[string]int{"prompt_tokens": 7, "completion_tokens": 3, "total_tokens": 10},
			})
			wbStreamDone(w)
			return
		}
		if input.ToolChoice != "none" {
			t.Error("tool-limit finalization or correction allowed more tools")
		}
		var evidence chatmodel.ContextInput
		for _, message := range input.Messages {
			if message.Role == schema.Tool {
				var result chatmodel.ContextInput
				_ = json.Unmarshal([]byte(message.Content), &result)
				evidence.Documents = append(evidence.Documents, result.Documents...)
			}
		}
		if len(evidence.Documents) == 0 {
			http.Error(w, "missing collected evidence", 400)
			return
		}
		text := "蓝灯表示故障模式。"
		if n == 4 {
			text = "蓝灯表示维护模式。"
		}
		wb06Final(w, domain.Draft{Claims: []domain.Claim{{
			Text: text, FragmentIDs: []string{evidence.Documents[0].ID},
		}}, Gaps: []string{"拆装步骤缺少对应型号手册，请补充。"}})
	}))
	defer generator.Close()
	var reviews atomic.Int32
	reviewer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reviews.Add(1)
		input := readAgentRequest(t, r)
		if len(input.Tools) != 0 {
			t.Error("grounding review inherited tools")
		}
		var payload groundingPayload
		_ = json.Unmarshal([]byte(input.Messages[1].Content), &payload)
		if len(payload.Claims) != 1 {
			http.Error(w, "missing claim", 400)
			return
		}
		verdict := "SUPPORTED"
		if strings.Contains(payload.Claims[0].Text, "故障") {
			verdict = "UNSUPPORTED"
		}
		raw, _ := json.Marshal(map[string]any{
			"claims": []domain.GroundingVerdict{groundingResponse(payload, 0, verdict)},
		})
		modelReply(w, string(raw))
	}))
	defer reviewer.Close()
	cm, _ := chatmodel.NewOpenAI(generator.URL, "stream-fixture", "")
	review, _ := chatmodel.NewOpenAI(reviewer.URL, "review-fixture", "")
	server, _, stop := wbStartOptions(t, workbenchDatabase(t), cm, true,
		application.Options{UsersMode: true, GroundingModel: review})
	defer stop()
	client := wbNewClient(t, server)
	client.login("admin", wbPassword)
	wbPublish(client)
	cid := client.request("POST", "/v1/conversations", map[string]any{}, 201)["id"].(string)
	id := client.request("POST", "/v1/conversations/"+cid+"/messages",
		map[string]string{"text": "蓝灯含义与拆装步骤"}, 202)["id"].(string)
	final := client.answer(id)
	if final["status"] != "PARTIAL" || !strings.Contains(final["answer"].(string), "维护模式") ||
		strings.Contains(final["answer"].(string), "故障模式") || final["error"] != nil ||
		modelCalls.Load() != 4 || reviews.Load() != 2 {
		t.Fatalf("tool limit bypassed grounding or lost partial answer: %v", final)
	}
	stream := client.openEvents(id, 0)
	defer stream.close()
	var starts, superseded, finalized, answers int
	for event := range stream.events {
		if event.Type == "draft_started" {
			starts++
		}
		if event.Type == "draft_retracted" && event.Reason != nil {
			switch event.Reason.Code {
			case "SUPERSEDED":
				superseded++
			case "FINALIZED":
				finalized++
			}
		}
		if event.Type == "failed" {
			t.Error("tool limit emitted a failed response")
		}
		if event.Type == "answer" {
			answers++
			if event.Response == nil || event.Response.Status != "PARTIAL" ||
				strings.Contains(event.Response.Answer, "故障模式") {
				t.Error("unverified draft became final")
			}
		}
	}
	if starts != 2 || superseded != 1 || finalized != 1 || answers != 1 {
		t.Fatalf("finalization draft lifecycle: starts=%d superseded=%d finalized=%d answers=%d",
			starts, superseded, finalized, answers)
	}
	t.Log("REPLAY: real PostgreSQL/SSE, tool-free finalization, unsupported draft corrected and reviewed before PARTIAL")
}

func TestToolLimitWorkbenchPrivateReadsFinishWithoutMoreTools(t *testing.T) {
	var requests atomic.Int32
	generator := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := requests.Add(1)
		input := readAgentRequest(t, r)
		if input.ToolChoice == "none" {
			if n != 13 {
				t.Errorf("private read budget changed: model call %d", n)
			}
			modelReply(w, `{"claims":[],"gaps":["未找到所需附件，请上传日志或操作手册。"]}`)
			return
		}
		if n > 12 {
			http.Error(w, "unbounded private reads", 400)
			return
		}
		args, _ := json.Marshal(map[string]string{"attachment_id": "unavailable"})
		modelReply(w, "", schema.ToolCall{ID: strings.Repeat("a", int(n)), Type: "function",
			Function: schema.FunctionCall{Name: einoflow.ReadAttachmentToolName, Arguments: string(args)}})
	}))
	defer generator.Close()
	files, err := blobstore.Open(filepath.Join(t.TempDir(), "files"), 1<<30, 1<<28)
	if err != nil {
		t.Fatal(err)
	}
	cm, _ := chatmodel.NewOpenAI(generator.URL, "private-fixture", "")
	server, _, stop := wbStartOptions(t, workbenchDatabase(t), cm, true,
		application.Options{UsersMode: true, Files: files, AttachmentRunner: newAttachmentReplayRunner()})
	defer stop()
	client := wbNewClient(t, server)
	client.login("admin", wbPassword)
	cid := client.request("POST", "/v1/conversations", map[string]any{}, 201)["id"].(string)
	id := client.request("POST", "/v1/conversations/"+cid+"/messages",
		map[string]string{"text": "读取附件"}, 202)["id"].(string)
	final := client.answer(id)
	if final["status"] != "UNRESOLVED" || final["error"] != nil || requests.Load() != 13 ||
		len(final["gaps"].([]any)) != 1 {
		t.Fatalf("private tool budget did not finish usefully: %v", final)
	}
	t.Log("REPLAY: twelve bounded private reads followed by tool-free missing-input response")
}
