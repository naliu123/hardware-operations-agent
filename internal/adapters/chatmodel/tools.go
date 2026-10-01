package chatmodel

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

type wireTool struct {
	Type     string       `json:"type"`
	Function wireFunction `json:"function"`
}

type wireFunction struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

type wireMessage struct {
	Role       schema.RoleType   `json:"role"`
	Content    string            `json:"content"`
	ToolCalls  []schema.ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string            `json:"tool_call_id,omitempty"`
}

var _ model.ToolCallingChatModel = (*OpenAI)(nil)

// WithTools snapshots the schema without changing the shared model used by RAG.
func (m *OpenAI) WithTools(infos []*schema.ToolInfo) (model.ToolCallingChatModel, error) {
	tools, err := encodeTools(infos)
	if err != nil {
		return nil, err
	}
	copy := *m
	copy.tools = tools
	return &copy, nil
}

func encodeTools(infos []*schema.ToolInfo) ([]wireTool, error) {
	tools := make([]wireTool, 0, len(infos))
	seen := map[string]bool{}
	for _, info := range infos {
		if info == nil || strings.TrimSpace(info.Name) == "" || seen[info.Name] || info.ParamsOneOf == nil {
			return nil, fmt.Errorf("invalid tool definition")
		}
		params, err := info.ParamsOneOf.ToJSONSchema()
		if err != nil {
			return nil, err
		}
		raw, err := json.Marshal(params)
		if err != nil {
			return nil, err
		}
		seen[info.Name] = true
		tools = append(tools, wireTool{Type: "function", Function: wireFunction{
			Name: info.Name, Description: info.Desc, Parameters: raw,
		}})
	}
	return tools, nil
}

func (m *OpenAI) requestBody(input []*schema.Message, opts ...model.Option) ([]byte, error) {
	options := model.GetCommonOptions(nil, opts...)
	tools := m.tools
	var err error
	if options.Tools != nil {
		tools, err = encodeTools(options.Tools)
		if err != nil {
			return nil, err
		}
	}
	messages := make([]wireMessage, 0, len(input))
	for _, item := range input {
		if item == nil {
			return nil, fmt.Errorf("nil model message")
		}
		messages = append(messages, wireMessage{
			Role: item.Role, Content: item.Content, ToolCalls: item.ToolCalls, ToolCallID: item.ToolCallID,
		})
	}
	name := m.name
	if options.Model != nil {
		name = *options.Model
	}
	var thinking map[string]string
	if strings.HasPrefix(name, "deepseek-") {
		thinking = map[string]string{"type": "disabled"}
	}
	var choice any
	if len(tools) > 0 {
		choice = "auto"
	}
	if options.ToolChoice != nil {
		switch *options.ToolChoice {
		case schema.ToolChoiceForbidden:
			choice = "none"
		case schema.ToolChoiceAllowed:
			choice = "auto"
		case schema.ToolChoiceForced:
			choice = "required"
		default:
			return nil, fmt.Errorf("unsupported tool choice")
		}
	}
	if len(options.AllowedToolNames) > 0 {
		if len(options.AllowedToolNames) != 1 || options.ToolChoice == nil ||
			*options.ToolChoice != schema.ToolChoiceForced {
			return nil, fmt.Errorf("named tool choice requires one forced tool")
		}
		found := false
		for _, t := range tools {
			found = found || t.Function.Name == options.AllowedToolNames[0]
		}
		if !found {
			return nil, fmt.Errorf("named tool is not registered")
		}
		choice = map[string]any{"type": "function", "function": map[string]string{"name": options.AllowedToolNames[0]}}
	}
	var temperature float32
	if options.Temperature != nil {
		temperature = *options.Temperature
	}
	// Execute tool calls serially so a follow-up query can use previous evidence.
	var parallel *bool
	if len(tools) > 0 {
		value := false
		parallel = &value
	}
	return json.Marshal(struct {
		Model       string            `json:"model"`
		Messages    []wireMessage     `json:"messages"`
		Stream      bool              `json:"stream"`
		Thinking    map[string]string `json:"thinking,omitempty"`
		Temperature float32           `json:"temperature"`
		Tools       []wireTool        `json:"tools,omitempty"`
		ToolChoice  any               `json:"tool_choice,omitempty"`
		Parallel    *bool             `json:"parallel_tool_calls,omitempty"`
		MaxTokens   *int              `json:"max_tokens,omitempty"`
		TopP        *float32          `json:"top_p,omitempty"`
		Stop        []string          `json:"stop,omitempty"`
	}{name, messages, false, thinking, temperature, tools, choice, parallel,
		options.MaxTokens, options.TopP, options.Stop})
}
