package acceptance_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/cloudwego/eino/schema"

	"hwops/internal/adapters/chatmodel"
	"hwops/internal/adapters/postgres"
	knowledgeagent "hwops/internal/agents/knowledge"
	"hwops/internal/domain"
)

func wb10Delta(w http.ResponseWriter, field, text string) {
	wbStreamFrame(w, map[string]any{"choices": []any{map[string]any{
		"index": 0, "delta": map[string]any{field: text},
	}}})
}

func wb10Finish(w http.ResponseWriter, reason string) {
	wbStreamFrame(w, map[string]any{"choices": []any{map[string]any{
		"index": 0, "delta": map[string]any{}, "finish_reason": reason,
	}}})
	wbStreamDone(w)
}

func wb10BrowserResponse(w http.ResponseWriter, r *http.Request, payload chatmodel.ContextInput) bool {
	if !strings.Contains(payload.Question, "WB10_BROWSER") {
		return false
	}
	w.Header().Set("Content-Type", "text/event-stream")
	wb10Delta(w, "reasoning_content", "REPLAY 思考第一段。")
	pause := func(duration time.Duration) bool {
		timer := time.NewTimer(duration)
		defer timer.Stop()
		select {
		case <-timer.C:
			return true
		case <-r.Context().Done():
			return false
		}
	}
	if strings.Contains(payload.Question, "STOP") {
		<-r.Context().Done()
		return true
	}
	if !pause(3 * time.Second) {
		return true
	}
	wb10Delta(w, "reasoning_content", "\n\nREPLAY 思考第二段。\n\n![隐藏像素](https://wb10.invalid/pixel.png)\n<script>window.__wb10_xss=true</script>")
	if !pause(6 * time.Second) {
		return true
	}
	draft := domain.Draft{Claims: []domain.Claim{}, Gaps: []string{"未找到合成测试资料。"}}
	if len(payload.Documents) > 0 {
		draft = domain.Draft{Claims: []domain.Claim{{
			Text: "REPLAY 正文独立完整。", FragmentIDs: []string{payload.Documents[0].ID},
		}}}
	}
	raw, _ := json.Marshal(draft)
	wb10Delta(w, "content", string(raw))
	wb10Finish(w, "stop")
	return true
}

func TestWB10ReasoningToolLoopPersistenceReplayAndHistory(t *testing.T) {
	t.Log("REPLAY: public HTTP/SSE, external model adapter and real temporary PostgreSQL")
	release := make(chan struct{})
	const first, second = "REPLAY_THINK_ONE", "REPLAY_THINK_TWO"
	modelServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var input struct {
			Messages []*schema.Message `json:"messages"`
			Stream   bool              `json:"stream"`
			Thinking map[string]string `json:"thinking"`
		}
		if json.Unmarshal(raw, &input) != nil || !input.Stream || input.Thinking["type"] != "enabled" {
			http.Error(w, "thinking stream required", 400)
			return
		}
		var payload chatmodel.ContextInput
		_ = json.Unmarshal([]byte(input.Messages[1].Content), &payload)
		w.Header().Set("Content-Type", "text/event-stream")
		if payload.Question == "你好" {
			if strings.Contains(string(raw), "REPLAY_THINK") || payload.History == nil || len(payload.History.Recent) != 1 {
				t.Error("reasoning leaked into subsequent history or history was lost")
			}
			wb10Delta(w, "content", `{"reply":{"kind":"GREETING","text":"你好。"},"claims":[],"gaps":[]}`)
			wb10Finish(w, "stop")
			return
		}
		var tool *schema.Message
		var toolReasoning string
		for _, message := range input.Messages {
			if message.Role == schema.Tool {
				tool = message
			}
			if message.Role == schema.Assistant {
				toolReasoning = message.ReasoningContent
			}
		}
		if tool == nil {
			wb10Delta(w, "reasoning_content", first)
			wbStreamFrame(w, map[string]any{"choices": []any{map[string]any{
				"index": 0, "delta": map[string]any{"tool_calls": []schema.ToolCall{{
					ID: "wb10-tool", Type: "function", Function: schema.FunctionCall{
						Name: knowledgeagent.ToolName, Arguments: `{"request":"蓝灯含义"}`,
					},
				}}},
			}}})
			wb10Finish(w, "tool_calls")
			return
		}
		if toolReasoning != first {
			t.Error("tool follow-up omitted assistant reasoning_content")
		}
		wb10Delta(w, "reasoning_content", second)
		select {
		case <-release:
		case <-r.Context().Done():
			return
		}
		var evidence chatmodel.ContextInput
		_ = json.Unmarshal([]byte(tool.Content), &evidence)
		if len(evidence.Documents) == 0 {
			t.Error("missing actual evidence")
			return
		}
		draft, _ := json.Marshal(domain.Draft{Claims: []domain.Claim{{
			Text: "蓝灯表示维护模式。", FragmentIDs: []string{evidence.Documents[0].ID},
		}}})
		wb10Delta(w, "content", string(draft))
		wb10Finish(w, "stop")
	}))
	defer modelServer.Close()
	dsn := workbenchDatabase(t)
	cm, _ := chatmodel.NewOpenAI(modelServer.URL, "deepseek-wb10-replay", "")
	server, _, stop := wbStart(t, dsn, cm, true)
	defer stop()
	defer closeGate(release)
	client := wbNewClient(t, server)
	client.login("admin", wbPassword)
	wbPublish(client)
	cid := client.request("POST", "/v1/conversations", map[string]any{}, 201)["id"].(string)
	path := "/v1/conversations/" + cid + "/messages"
	id := client.request("POST", path, map[string]string{"text": "蓝灯含义"}, 202)["id"].(string)
	stream := client.openEvents(id, 0)
	defer stream.close()
	one := waitWBEvent(t, stream, "reasoning_delta")
	done := waitWBEvent(t, stream, "reasoning_done")
	two := waitWBEvent(t, stream, "reasoning_delta")
	if one.Delta != first || done.ReasoningStatus != "COMPLETED" || two.Delta != "\n\n"+second {
		t.Fatalf("reasoning segments mismatch: %+v %+v %+v", one, done, two)
	}
	running := client.request("GET", "/v1/responses/"+id, nil, 200)
	reasoning := running["reasoning"].(map[string]any)
	if running["status"] != "RUNNING" || running["answer"] != "" ||
		reasoning["text"] != first+"\n\n"+second || reasoning["event_id"] != float64(two.ID) {
		t.Fatal("reasoning was not persisted before body/finish")
	}
	stream.close()
	replay := client.openEvents(id, one.ID)
	defer replay.close()
	if next := waitWBEvent(t, replay, "reasoning_delta"); next.ID != two.ID || next.Delta != two.Delta {
		t.Fatal("reconnect lost or changed persisted reasoning")
	}
	closeGate(release)
	finalEvent := waitWBEvent(t, replay, "answer")
	final := client.answer(id)
	if finalEvent.Response == nil || finalEvent.Response.Reasoning == nil ||
		finalEvent.Response.Reasoning.Text != first+"\n\n"+second || finalEvent.Response.Reasoning.Status != "COMPLETED" ||
		final["answer"] == "" || final["reasoning"].(map[string]any)["text"] != first+"\n\n"+second {
		t.Fatal("final worker save overwrote reasoning")
	}
	if rows := client.request("GET", path, nil, 200)["messages"].([]any); rows[0].(map[string]any)["reasoning"] == nil {
		t.Fatal("history UI cannot restore reasoning")
	}
	if rows := client.request("GET", "/v1/conversations/search?q=REPLAY_THINK", nil, 200)["conversations"].([]any); len(rows) != 0 {
		t.Fatal("reasoning leaked into conversation search")
	}
	client.request("POST", "/v1/admin/users", map[string]string{"username": "other", "password": wbPassword, "role": "USER"}, 201)
	other := wbNewClient(t, server)
	other.login("other", wbPassword)
	other.request("GET", "/v1/responses/"+id, nil, 404)
	other.request("GET", "/v1/responses/"+id+"/events", nil, 404)
	next := client.request("POST", path, map[string]string{"text": "你好"}, 202)
	nextFinal := client.answer(next["id"].(string))
	if nextFinal["status"] != "ANSWERED" || nextFinal["reasoning"] != nil {
		t.Fatal("model with no reasoning acquired fabricated reasoning")
	}
}

func TestWB10ReasoningSurvivesFailureCancellationAndRestart(t *testing.T) {
	for _, mode := range []string{"disconnect", "reasoning_only", "cancel", "restart", "limit"} {
		t.Run(mode, func(t *testing.T) {
			t.Log("REPLAY: public response lifecycle with actual PostgreSQL")
			release := make(chan struct{})
			modelServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				wb10Delta(w, "reasoning_content", "REPLAY retained thought")
				select {
				case <-release:
				case <-r.Context().Done():
					return
				}
				switch mode {
				case "reasoning_only":
					wb10Finish(w, "stop")
				case "limit":
					for n := 0; n < 34; n++ {
						wb10Delta(w, "reasoning_content", strings.Repeat("r", 16*1024))
					}
					wb10Finish(w, "stop")
				}
			}))
			defer modelServer.Close()
			dsn := workbenchDatabase(t)
			cm, _ := chatmodel.NewOpenAI(modelServer.URL, "deepseek-wb10-replay", "")
			server, _, stop := wbStart(t, dsn, cm, true)
			defer func() { stop() }()
			defer closeGate(release)
			client := wbNewClient(t, server)
			client.login("admin", wbPassword)
			cid := client.request("POST", "/v1/conversations", map[string]any{}, 201)["id"].(string)
			id := client.request("POST", "/v1/conversations/"+cid+"/messages", map[string]string{"text": "你是谁"}, 202)["id"].(string)
			stream := client.openEvents(id, 0)
			delta := waitWBEvent(t, stream, "reasoning_delta")
			stream.close()
			expected := "FAILED"
			switch mode {
			case "cancel":
				client.request("POST", "/v1/responses/"+id+"/cancel", map[string]any{}, 202)
				expected = "CANCELED"
			case "restart":
				stop()
				server, _, stop = wbStart(t, dsn, cm, false)
				client = wbNewClient(t, server)
				client.login("admin", wbPassword)
				expected = "INTERRUPTED"
			default:
				closeGate(release)
			}
			final := client.answer(id)
			reasoning, ok := final["reasoning"].(map[string]any)
			if !ok || final["status"] != expected || final["answer"] != "" || reasoning["status"] != "INTERRUPTED" ||
				!strings.HasPrefix(reasoning["text"].(string), delta.Delta) ||
				len(reasoning["text"].(string)) > domain.MaxReasoningBytes {
				t.Fatalf("partial reasoning lifecycle failed: status=%v reasoning_present=%v", final["status"], ok)
			}
			if mode != "limit" {
				replay := client.openEvents(id, delta.ID)
				done := waitWBEvent(t, replay, "reasoning_done")
				replay.close()
				if done.ReasoningStatus != "INTERRUPTED" {
					t.Fatal("replay lost interrupted reasoning status")
				}
			}
			stop()
			stop = func() {}
			store, err := postgres.Open(context.Background(), dsn)
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			if _, err = store.AppendResponseReasoning(context.Background(), id, "late"); !errors.Is(err, domain.ErrConflict) {
				t.Fatalf("late reasoning accepted: %v", err)
			}
			if err = store.FinishResponseReasoning(context.Background(), id); !errors.Is(err, domain.ErrConflict) {
				t.Fatalf("late completion accepted: %v", err)
			}
		})
	}
}
