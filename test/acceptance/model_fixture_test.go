package acceptance_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"

	"github.com/cloudwego/eino/schema"

	"hwops/internal/adapters/chatmodel"
	knowledgeagent "hwops/internal/agents/knowledge"
)

// replayKnowledgeChoice adds a deterministic tool decision to legacy answer
// fixtures. All requests still cross the real HTTP model adapter. New agent
// behavior tests use explicit tool-call scripts instead of this helper.
func replayKnowledgeChoice(answer http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		r.Body = io.NopCloser(bytes.NewReader(raw))
		var input struct {
			Messages   []*schema.Message `json:"messages"`
			Tools      []json.RawMessage `json:"tools"`
			ToolChoice string            `json:"tool_choice"`
		}
		if json.Unmarshal(raw, &input) != nil || len(input.Tools) == 0 || len(input.Messages) < 2 {
			answer(w, r)
			return
		}
		var context chatmodel.ContextInput
		_ = json.Unmarshal([]byte(input.Messages[1].Content), &context)
		hasTool := false
		for _, message := range input.Messages {
			if message.Role == schema.Tool {
				hasTool = true
				var result chatmodel.ContextInput
				_ = json.Unmarshal([]byte(message.Content), &result)
				context.Documents = append(context.Documents, result.Documents...)
			}
		}
		if !hasTool && input.ToolChoice != "none" {
			args, _ := json.Marshal(map[string]string{"request": context.Question})
			_ = json.NewEncoder(w).Encode(map[string]any{
				"choices": []any{map[string]any{"message": map[string]any{
					"role": "assistant", "content": nil,
					"tool_calls": []schema.ToolCall{{
						ID: "fixture-knowledge", Type: "function",
						Function: schema.FunctionCall{Name: knowledgeagent.ToolName, Arguments: string(args)},
					}},
				}}},
			})
			return
		}
		// Present accumulated tool evidence in the answer fixture's original
		// input format so existing citation and device assertions remain useful.
		payload, _ := json.Marshal(context)
		input.Messages[1].Content = string(payload)
		var body map[string]json.RawMessage
		_ = json.Unmarshal(raw, &body)
		body["messages"], _ = json.Marshal(input.Messages)
		raw, _ = json.Marshal(body)
		r.Body = io.NopCloser(bytes.NewReader(raw))
		answer(w, r)
	}
}
