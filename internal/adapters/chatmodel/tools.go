package chatmodel

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
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
	Role             schema.RoleType   `json:"role"`
	Content          any               `json:"content"`
	ReasoningContent string            `json:"reasoning_content,omitempty"`
	ToolCalls        []schema.ToolCall `json:"tool_calls,omitempty"`
	ToolCallID       string            `json:"tool_call_id,omitempty"`
}

type wireContentPart struct {
	Type     string        `json:"type"`
	Text     string        `json:"text,omitempty"`
	ImageURL *wireImageURL `json:"image_url,omitempty"`
}

type wireImageURL struct {
	URL    string                `json:"url"`
	Detail schema.ImageURLDetail `json:"detail,omitempty"`
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

func (m *OpenAI) requestBody(input []*schema.Message, stream bool, opts ...model.Option) ([]byte, error) {
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
		content, err := encodeContent(item)
		if err != nil {
			return nil, err
		}
		messages = append(messages, wireMessage{
			Role: item.Role, Content: content, ToolCalls: item.ToolCalls, ToolCallID: item.ToolCallID,
			ReasoningContent: item.ReasoningContent,
		})
	}
	name := m.name
	if options.Model != nil {
		name = *options.Model
	}
	var thinking map[string]string
	if strings.HasPrefix(name, "deepseek-") {
		thinking = map[string]string{"type": "enabled"}
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
	}{name, messages, stream, thinking, temperature, tools, choice, parallel,
		options.MaxTokens, options.TopP, options.Stop})
}

func encodeContent(message *schema.Message) (any, error) {
	if len(message.UserInputMultiContent) == 0 {
		return message.Content, nil
	}
	if message.Role != schema.User || message.Content != "" {
		return nil, fmt.Errorf("multimodal content requires a user message without duplicate content")
	}
	parts := make([]wireContentPart, 0, len(message.UserInputMultiContent))
	for _, part := range message.UserInputMultiContent {
		switch part.Type {
		case schema.ChatMessagePartTypeText:
			if strings.TrimSpace(part.Text) == "" {
				return nil, fmt.Errorf("multimodal text part is empty")
			}
			parts = append(parts, wireContentPart{Type: "text", Text: part.Text})
		case schema.ChatMessagePartTypeImageURL:
			if part.Image == nil || part.Image.Base64Data == nil || part.Image.URL != nil ||
				(part.Image.MIMEType != "image/png" && part.Image.MIMEType != "image/jpeg" &&
					part.Image.MIMEType != "image/webp") ||
				len(*part.Image.Base64Data) > 67_000_000 {
				return nil, fmt.Errorf("image input must be bounded inline PNG, JPEG or WebP bytes")
			}
			raw, err := base64.StdEncoding.DecodeString(*part.Image.Base64Data)
			if err != nil || len(raw) == 0 || len(raw) > 50_000_000 || !imageSignature(raw, part.Image.MIMEType) {
				return nil, fmt.Errorf("image input bytes do not match the declared type")
			}
			parts = append(parts, wireContentPart{Type: "image_url", ImageURL: &wireImageURL{
				URL:    "data:" + part.Image.MIMEType + ";base64," + *part.Image.Base64Data,
				Detail: part.Image.Detail,
			}})
		default:
			return nil, fmt.Errorf("unsupported multimodal input part")
		}
	}
	if len(parts) == 0 {
		return nil, fmt.Errorf("multimodal content is empty")
	}
	return parts, nil
}

func imageSignature(raw []byte, mediaType string) bool {
	if mediaType == "image/webp" {
		return len(raw) >= 12 && string(raw[:4]) == "RIFF" && string(raw[8:12]) == "WEBP"
	}
	return http.DetectContentType(raw) == mediaType
}
