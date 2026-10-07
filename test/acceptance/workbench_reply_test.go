package acceptance_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cloudwego/eino/schema"

	"hwops/internal/adapters/chatmodel"
)

func TestWorkbenchConversationalReplyStreamAndHistory(t *testing.T) {
	t.Log("REPLAY: external model stream, public HTTP and real temporary PostgreSQL")
	release := make(chan struct{})
	modelServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			Messages []*schema.Message `json:"messages"`
			Stream   bool              `json:"stream"`
		}
		if json.NewDecoder(r.Body).Decode(&input) != nil || !input.Stream || len(input.Messages) < 2 {
			http.Error(w, "invalid streaming request", 400)
			return
		}
		var payload chatmodel.ContextInput
		_ = json.Unmarshal([]byte(input.Messages[1].Content), &payload)
		kind, text := "INTRODUCTION", "我是知维，你的硬件运维助手。"
		switch payload.Question {
		case "？":
			if payload.History == nil || len(payload.History.Recent) != 1 ||
				payload.History.Recent[0].Answer != text {
				http.Error(w, "introduction lost from history", 400)
				return
			}
			kind, text = "CLARIFICATION", "你是想了解我能帮你做什么吗？"
		case "你好":
			if payload.History == nil || len(payload.History.Recent) != 2 ||
				payload.History.Recent[1].Answer != "你是想了解我能帮你做什么吗？" {
				http.Error(w, "clarification lost from history", 400)
				return
			}
			kind, text = "GREETING", "你好，有什么需要我帮忙的？"
		}
		w.Header().Set("Content-Type", "text/event-stream")
		// Emit an unfinished JSON string and hold the model open. This must
		// reach public SSE before the rest of the response becomes available.
		runes := []rune(text)
		first := `{"analysis":"INTERNAL","reply":{"kind":"` + kind + `","text":"` + string(runes[:4])
		wbStreamFrame(w, map[string]any{"choices": []any{map[string]any{
			"index": 0, "delta": map[string]any{"content": first},
		}}})
		if payload.Question == "你是谁" {
			select {
			case <-release:
			case <-r.Context().Done():
				return
			}
		}
		wbStreamFrame(w, map[string]any{"choices": []any{map[string]any{
			"index": 0, "delta": map[string]any{"content": string(runes[4:]) + `"},"claims":[],"gaps":[]}`},
		}}})
		wbStreamFrame(w, map[string]any{"choices": []any{map[string]any{
			"index": 0, "delta": map[string]any{}, "finish_reason": "stop",
		}}})
		wbStreamDone(w)
	}))
	defer modelServer.Close()
	cm, _ := chatmodel.NewOpenAI(modelServer.URL, "reply-replay", "")
	server, _, stop := wbStart(t, workbenchDatabase(t), cm, true)
	defer stop()
	defer closeGate(release)
	client := wbNewClient(t, server)
	client.login("admin", wbPassword)
	conversation := client.request("POST", "/v1/conversations", map[string]any{}, 201)["id"].(string)
	for index, tc := range []struct{ question, answer, status, event string }{
		{"你是谁", "我是知维，你的硬件运维助手。", "ANSWERED", "answer"},
		{"？", "你是想了解我能帮你做什么吗？", "NEEDS_CLARIFICATION", "clarification_required"},
		{"你好", "你好，有什么需要我帮忙的？", "ANSWERED", "answer"},
	} {
		pending := client.request("POST", "/v1/conversations/"+conversation+"/messages",
			map[string]string{"text": tc.question}, 202)
		id := pending["id"].(string)
		stream := client.openEvents(id, 0)
		defer stream.close()
		first := waitWBEvent(t, stream, "answer_delta")
		if first.Delta != string([]rune(tc.answer)[:4]) {
			t.Fatalf("missing conversational draft: %+v", first)
		}
		if index == 0 {
			current := client.request("GET", "/v1/responses/"+id, nil, 200)
			if current["status"] != "RUNNING" || current["answer"] != "" {
				t.Fatalf("draft was not delivered before model completion: %+v", current)
			}
			closeGate(release)
		}
		final := waitWBEvent(t, stream, tc.event)
		if final.Response == nil || final.Response.Status != tc.status || final.Response.Answer != tc.answer ||
			len(final.Response.Claims) != 0 || len(final.Response.Citations) != 0 || final.Response.DataMode != "REPLAY" {
			t.Fatalf("conversational final response missing: %+v", final.Response)
		}
		saved := client.request("GET", "/v1/responses/"+id, nil, 200)
		if saved["answer"] != tc.answer || saved["status"] != tc.status {
			t.Fatalf("saved response differs from SSE: %+v", saved)
		}
		replayed := client.openEvents(id, first.ID)
		event := waitWBEvent(t, replayed, tc.event)
		replayed.close()
		if event.Response == nil || event.Response.Answer != tc.answer {
			t.Fatal("resumed stream lost final reply")
		}
	}
}

func TestWorkbenchConversationalReplyCannotBypassAnswerValidation(t *testing.T) {
	t.Log("REPLAY: malformed replies and unsourced claims must fail through public HTTP")
	cases := map[string]string{
		"unknown kind": `{"reply":{"kind":"HARDWARE","text":"设备正常"},"claims":[]}`,
		"empty text":   `{"reply":{"kind":"INTRODUCTION","text":" "},"claims":[]}`,
		"long text":    `{"reply":{"kind":"INTRODUCTION","text":"` + strings.Repeat("x", 16001) + `"},"claims":[]}`,
		"mixed claim":  `{"reply":{"kind":"INTRODUCTION","text":"我是知维"},"claims":[{"text":"设备正常"}]}`,
		"mixed observation": `{"reply":{"kind":"GREETING","text":"你好"},"claims":[],
			"observations":[{"evidence_id":"invented","fields":["temperature"]}]}`,
		"mixed conflict": `{"reply":{"kind":"CLARIFICATION","text":"请补充型号"},"claims":[],
			"conflicts":[{"subject":"冲突","fragment_ids":["invented-a","invented-b"]}]}`,
		"unsourced claim": `{"claims":[{"text":"设备正常"}],"gaps":[]}`,
	}
	modelServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			Messages []*schema.Message `json:"messages"`
		}
		_ = json.NewDecoder(r.Body).Decode(&input)
		var payload chatmodel.ContextInput
		_ = json.Unmarshal([]byte(input.Messages[1].Content), &payload)
		w.Header().Set("Content-Type", "text/event-stream")
		wbStreamFrame(w, map[string]any{"choices": []any{map[string]any{
			"index": 0, "delta": map[string]any{"content": cases[payload.Question]},
		}}})
		wbStreamFrame(w, map[string]any{"choices": []any{map[string]any{
			"index": 0, "delta": map[string]any{}, "finish_reason": "stop",
		}}})
		wbStreamDone(w)
	}))
	defer modelServer.Close()
	cm, _ := chatmodel.NewOpenAI(modelServer.URL, "reply-invalid-replay", "")
	server, _, stop := wbStart(t, workbenchDatabase(t), cm, true)
	defer stop()
	client := wbNewClient(t, server)
	client.login("admin", wbPassword)
	for name := range cases {
		t.Run(name, func(t *testing.T) {
			conversation := client.request("POST", "/v1/conversations", map[string]any{}, 201)["id"].(string)
			pending := client.request("POST", "/v1/conversations/"+conversation+"/messages",
				map[string]string{"text": name}, 202)
			final := client.answer(pending["id"].(string))
			if final["status"] != "FAILED" || final["answer"] != "" ||
				final["error"].(map[string]any)["code"] != "INVALID_MODEL_OUTPUT" {
				t.Fatalf("invalid output accepted: %+v", final)
			}
		})
	}
}
