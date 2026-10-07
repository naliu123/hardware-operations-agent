package contracts_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cloudwego/eino/schema"

	"hwops/internal/adapters/chatmodel"
	"hwops/internal/observability"
)

func TestReasoningBufferedAndToolRoundTrip(t *testing.T) {
	for _, streaming := range []bool{false, true} {
		t.Run(map[bool]string{false: "generate", true: "buffered_stream"}[streaming], func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var input struct {
					Messages []struct {
						Reasoning string `json:"reasoning_content"`
					} `json:"messages"`
					Thinking map[string]string `json:"thinking"`
				}
				_ = json.NewDecoder(r.Body).Decode(&input)
				if input.Thinking["type"] != "enabled" || len(input.Messages) != 3 || input.Messages[1].Reasoning != "REPLAY tool reasoning" {
					t.Error("thinking configuration or tool reasoning was lost")
					http.Error(w, "missing reasoning", 400)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, `{"choices":[{"message":{"reasoning_content":"REPLAY final reasoning","content":"answer"},"finish_reason":"stop"}]}`)
			}))
			defer server.Close()
			cm, _ := chatmodel.NewOpenAI(server.URL, "deepseek-flash", "")
			assistant := schema.AssistantMessage("", []schema.ToolCall{{ID: "call", Type: "function",
				Function: schema.FunctionCall{Name: "tool", Arguments: `{}`}}})
			assistant.ReasoningContent = "REPLAY tool reasoning"
			input := []*schema.Message{schema.UserMessage("hello"), assistant, schema.ToolMessage("result", "call")}
			var result *schema.Message
			var err error
			if streaming {
				stream, streamErr := cm.Stream(context.Background(), input)
				if streamErr != nil {
					t.Fatal(streamErr)
				}
				result, err = schema.ConcatMessageStream(stream)
			} else {
				result, err = cm.Generate(context.Background(), input)
			}
			if err != nil || result.ReasoningContent != "REPLAY final reasoning" || result.Content != "answer" {
				t.Fatalf("buffered reasoning missing: %+v %v", result, err)
			}
		})
	}
}

func TestReasoningNativeStreamArrivesBeforeAnswer(t *testing.T) {
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		streamFrame(t, w, map[string]any{"choices": []any{map[string]any{
			"index": 0, "delta": map[string]any{"reasoning_content": "REPLAY thinking"},
		}}})
		select {
		case <-release:
		case <-r.Context().Done():
			return
		}
		streamFrame(t, w, map[string]any{"choices": []any{map[string]any{
			"index": 0, "delta": map[string]any{"content": "answer"}, "finish_reason": "stop",
		}}})
		streamDone(w)
	}))
	defer server.Close()
	defer close(release)
	cm, _ := chatmodel.NewOpenAI(server.URL, "deepseek-flash", "")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stream, err := cm.Stream(ctx, []*schema.Message{schema.UserMessage("hello")})
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	chunk, err := stream.Recv()
	if err != nil || chunk.ReasoningContent != "REPLAY thinking" || chunk.Content != "" {
		t.Fatalf("reasoning was buffered or mixed into answer: %+v %v", chunk, err)
	}
}

func TestReasoningOnlyAndOversizeStreamsFail(t *testing.T) {
	for _, size := range []int{10, 2 * 1024 * 1024} {
		t.Run(map[bool]string{true: "oversize", false: "reasoning_only"}[size > 10], func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				for n := 0; n < size; n += 4096 {
					streamFrame(t, w, map[string]any{"choices": []any{map[string]any{
						"index": 0, "delta": map[string]any{"reasoning_content": strings.Repeat("r", 4096)},
					}}})
				}
				streamFrame(t, w, map[string]any{"choices": []any{map[string]any{
					"index": 0, "delta": map[string]any{}, "finish_reason": "stop",
				}}})
				streamDone(w)
			}))
			defer server.Close()
			cm, _ := chatmodel.NewOpenAI(server.URL, "deepseek-flash", "")
			stream, err := cm.Stream(context.Background(), []*schema.Message{schema.UserMessage("hello")})
			if err != nil {
				t.Fatal(err)
			}
			if _, err = schema.ConcatMessageStream(stream); !errors.Is(err, chatmodel.ErrUnavailable) {
				t.Fatalf("reasoning-only/oversize stream accepted: %v", err)
			}
		})
	}
}

func TestReasoningOmittedFromTracing(t *testing.T) {
	message := schema.AssistantMessage("visible answer", nil)
	message.ReasoningContent = "PRIVATE_REASONING"
	result := observability.JSON(map[string]any{
		"messages": []*schema.Message{message},
		"response": map[string]any{"reasoning": map[string]string{"text": "PRIVATE_REASONING"}},
	})
	if strings.Contains(result, "PRIVATE_REASONING") || !strings.Contains(result, "visible answer") {
		t.Fatalf("trace did not separate reasoning: %s", result)
	}
}
