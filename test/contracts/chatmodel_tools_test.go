package contracts_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"

	"hwops/internal/adapters/chatmodel"
)

func TestOpenAIToolCallingRoundTripAndImmutableBinding(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			Messages []struct {
				Role string `json:"role"`
				Content string `json:"content"`
				ToolCalls []schema.ToolCall `json:"tool_calls"`
				ToolCallID string `json:"tool_call_id"`
			} `json:"messages"`
			Tools []struct {
				Type string `json:"type"`
				Function struct {
					Name string `json:"name"`
					Parameters struct {
						Type string `json:"type"`
						Properties map[string]any `json:"properties"`
						Required []string `json:"required"`
					} `json:"parameters"`
				} `json:"function"`
			} `json:"tools"`
			ToolChoice string `json:"tool_choice"`
			Parallel *bool `json:"parallel_tool_calls"`
		}
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			t.Error(err)
			http.Error(w, "bad input", 400)
			return
		}
		if input.Messages[0].Content == "unbound" {
			if len(input.Tools) != 0 || input.ToolChoice != "" || input.Parallel != nil {
				t.Error("bound tools leaked into shared model")
			}
			_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"unbound result"}}]}`))
			return
		}
		if len(input.Tools) != 1 || input.Tools[0].Type != "function" ||
			input.Tools[0].Function.Name != "retrieve_hardware_knowledge" ||
			input.Tools[0].Function.Parameters.Type != "object" ||
			len(input.Tools[0].Function.Parameters.Properties) != 1 ||
			len(input.Tools[0].Function.Parameters.Required) != 1 ||
			input.Tools[0].Function.Parameters.Required[0] != "request" ||
			input.Parallel == nil || *input.Parallel {
			t.Error("tool schema was lost or mutated")
		}
		if len(input.Messages) == 1 {
			if input.ToolChoice != "auto" {
				t.Error("initial decision must be auto")
			}
			_, _ = w.Write([]byte(`{"choices":[{"finish_reason":"tool_calls","message":{"role":"assistant","content":null,"tool_calls":[{"id":"call-1","type":"function","function":{"name":"retrieve_hardware_knowledge","arguments":"{\"request\":\"蓝灯含义\"}"}}]}}],"usage":{"prompt_tokens":7,"completion_tokens":3,"total_tokens":10}}`))
			return
		}
		if input.ToolChoice != "none" || len(input.Messages) != 3 ||
			input.Messages[1].Role != "assistant" || len(input.Messages[1].ToolCalls) != 1 ||
			input.Messages[1].ToolCalls[0].ID != "call-1" ||
			input.Messages[2].Role != "tool" || input.Messages[2].ToolCallID != "call-1" ||
			input.Messages[2].Content != `{"status":"NOT_FOUND"}` {
			t.Error("tool response lost its assistant call correlation")
		}
		_, _ = w.Write([]byte(`{"choices":[{"finish_reason":"stop","message":{"content":"缺少知识"}}]}`))
	}))
	defer server.Close()
	base, err := chatmodel.NewOpenAI(server.URL, "fixture-model", "")
	if err != nil {
		t.Fatal(err)
	}
	info := &schema.ToolInfo{Name: "retrieve_hardware_knowledge", Desc: "知识检索", ParamsOneOf: schema.NewParamsOneOfByParams(
		map[string]*schema.ParameterInfo{"request": {Type: schema.String, Required: true}},
	)}
	bound, err := base.WithTools([]*schema.ToolInfo{info})
	if err != nil {
		t.Fatal(err)
	}
	info.Name = "mutated-after-binding"
	ctx := context.Background()
	initial := []*schema.Message{schema.UserMessage("retrieve")}
	call, err := bound.Generate(ctx, initial)
	if err != nil || call == nil || len(call.ToolCalls) != 1 ||
		call.ToolCalls[0].Function.Arguments != `{"request":"蓝灯含义"}` ||
		call.ResponseMeta == nil || call.ResponseMeta.Usage.TotalTokens != 10 ||
		call.ResponseMeta.FinishReason != "tool_calls" {
		t.Fatalf("tool-only reply or usage lost: %+v %v", call, err)
	}
	history := append(initial, call, schema.ToolMessage(`{"status":"NOT_FOUND"}`, call.ToolCalls[0].ID))
	stream, err := bound.Stream(ctx, history, model.WithToolChoice(schema.ToolChoiceForbidden))
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	final, err := stream.Recv()
	if err != nil || final.Content != "缺少知识" {
		t.Fatalf("tool round trip failed: %+v %v", final, err)
	}
	if _, err := base.Generate(ctx, []*schema.Message{schema.UserMessage("unbound")}); err != nil {
		t.Fatal(err)
	}
}
