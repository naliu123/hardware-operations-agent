package chatmodel

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
	"go.opentelemetry.io/otel/attribute"

	"hwops/internal/domain"
	"hwops/internal/observability"
)

var ErrUnavailable = errors.New("model unavailable")

const userAgent = "hwops/1.0"

type sessionIDKey struct{}

func WithSessionID(ctx context.Context, sessionID string) context.Context {
	if strings.TrimSpace(sessionID) == "" {
		return ctx
	}
	return context.WithValue(ctx, sessionIDKey{}, sessionID)
}

type ContextInput struct {
	Question  string                `json:"question"`
	Documents []*schema.Document    `json:"documents"`
	Device    *domain.DeviceContext `json:"device_context,omitempty"`
	Now       time.Time             `json:"now,omitempty"`
}

// Replay is an explicitly selected extractive demonstration, never a fallback.
type Replay struct{ toolName string }

var _ model.ToolCallingChatModel = (*Replay)(nil)

func (m *Replay) WithTools(tools []*schema.ToolInfo) (model.ToolCallingChatModel, error) {
	copy := *m
	copy.toolName = ""
	if len(tools) > 0 {
		if _, err := encodeTools(tools); err != nil {
			return nil, err
		}
		copy.toolName = tools[0].Name
	}
	return &copy, nil
}

func (m *Replay) Generate(ctx context.Context, input []*schema.Message, _ ...model.Option) (*schema.Message, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(input) == 0 {
		return nil, errors.New("missing input")
	}
	var payload ContextInput
	source := input[len(input)-1]
	foundTool := false
	for i := len(input) - 1; i >= 0; i-- {
		if input[i].Role == schema.Tool {
			source, foundTool = input[i], true
			break
		}
	}
	if err := json.Unmarshal([]byte(source.Content), &payload); err != nil {
		return nil, err
	}
	if m.toolName != "" && !foundTool {
		args, _ := json.Marshal(map[string]string{"request": payload.Question})
		return schema.AssistantMessage("", []schema.ToolCall{{
			ID: "replay-knowledge", Type: "function",
			Function: schema.FunctionCall{Name: m.toolName, Arguments: string(args)},
		}}), nil
	}
	if len(payload.Documents) == 0 {
		var result struct {
			Gaps []string `json:"gaps"`
		}
		_ = json.Unmarshal([]byte(source.Content), &result)
		if len(result.Gaps) == 0 {
			result.Gaps = []string{"没有可用于回答的知识证据。"}
		}
		content, _ := json.Marshal(domain.Draft{Claims: []domain.Claim{}, Gaps: result.Gaps})
		return schema.AssistantMessage(string(content), nil), nil
	}
	doc := payload.Documents[0]
	content, err := json.Marshal(domain.Draft{Claims: []domain.Claim{{
		Text: "【演示摘录，非模型诊断】" + doc.Content, FragmentIDs: []string{doc.ID},
	}}})
	if err != nil {
		return nil, err
	}
	return schema.AssistantMessage(string(content), nil), nil
}

func (m *Replay) Stream(ctx context.Context, in []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	out, err := m.Generate(ctx, in, opts...)
	if err != nil {
		return nil, err
	}
	return schema.StreamReaderFromArray([]*schema.Message{out}), nil
}

type Unconfigured struct{}

func (m *Unconfigured) WithTools([]*schema.ToolInfo) (model.ToolCallingChatModel, error) {
	return m, nil
}

func (*Unconfigured) Generate(context.Context, []*schema.Message, ...model.Option) (*schema.Message, error) {
	return nil, ErrUnavailable
}

func (*Unconfigured) Stream(context.Context, []*schema.Message, ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	return nil, ErrUnavailable
}

// OpenAI calls an explicitly configured chat/completions endpoint.
type OpenAI struct {
	endpoint string
	name     string
	key      string
	client   *http.Client
	tools    []wireTool
}

var _ model.BaseChatModel = (*OpenAI)(nil)

const (
	maxModelAttempts = 2
	defaultRetryWait = 100 * time.Millisecond
	maxRetryWait     = 2 * time.Second
)

func NewOpenAI(endpoint, name, key string) (*OpenAI, error) {
	u, err := url.Parse(endpoint)
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" ||
		(u.Scheme != "http" && u.Scheme != "https") || name == "" {
		return nil, errors.New("invalid model endpoint or model name")
	}
	return &OpenAI{endpoint: endpoint, name: name, key: key, client: &http.Client{
		Timeout: 30 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}}, nil
}

func (m *OpenAI) Generate(ctx context.Context, input []*schema.Message, opts ...model.Option) (messageOut *schema.Message, callErr error) {
	ctx, span := observability.Start(ctx, "chat.generate", "LLM", input)
	span.SetAttributes(attribute.String("llm.model_name", m.name))
	if strings.HasPrefix(m.name, "deepseek-") {
		span.SetAttributes(attribute.String("llm.provider", "deepseek"), attribute.String("llm.system", "deepseek"))
	}
	defer func() { observability.End(span, messageOut, callErr) }()
	body, err := m.requestBody(input, opts...)
	if err != nil {
		return nil, err
	}
	for attempt := 0; attempt < maxModelAttempts; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, m.endpoint, bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("User-Agent", userAgent)
		if m.key != "" {
			req.Header.Set("Authorization", "Bearer "+m.key)
		}
		if sessionID, ok := ctx.Value(sessionIDKey{}).(string); ok {
			req.Header.Set("x-opencode-session", sessionID)
		}
		response, err := m.client.Do(req)
		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			if attempt+1 < maxModelAttempts && waitForRetry(ctx, defaultRetryWait) {
				span.SetAttributes(attribute.Int("llm.retry_count", attempt+1))
				continue
			}
			return nil, ErrUnavailable
		}
		if response.StatusCode != http.StatusOK {
			status := response.StatusCode
			retryAfter := response.Header.Get("Retry-After")
			_ = response.Body.Close()
			if attempt+1 < maxModelAttempts && retryableStatus(status) {
				if delay, ok := retryDelay(retryAfter); ok && waitForRetry(ctx, delay) {
					span.SetAttributes(attribute.Int("llm.retry_count", attempt+1))
					continue
				}
				if ctx.Err() != nil {
					return nil, ctx.Err()
				}
			}
			return nil, fmt.Errorf("%w: HTTP %d", ErrUnavailable, status)
		}
		raw, err := io.ReadAll(io.LimitReader(response.Body, 1024*1024+1))
		_ = response.Body.Close()
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if err != nil || len(raw) > 1024*1024 {
			return nil, fmt.Errorf("%w: unreadable response", ErrUnavailable)
		}
		var result struct {
			Usage   *schema.TokenUsage `json:"usage"`
			Choices []struct {
				Message struct {
					Content   string            `json:"content"`
					ToolCalls []schema.ToolCall `json:"tool_calls"`
				} `json:"message"`
				FinishReason string `json:"finish_reason"`
			} `json:"choices"`
		}
		if err := json.Unmarshal(raw, &result); err != nil || len(result.Choices) == 0 ||
			(strings.TrimSpace(result.Choices[0].Message.Content) == "" && len(result.Choices[0].Message.ToolCalls) == 0) {
			return nil, fmt.Errorf("%w: invalid response", ErrUnavailable)
		}
		out := schema.AssistantMessage(result.Choices[0].Message.Content, result.Choices[0].Message.ToolCalls)
		out.ResponseMeta = &schema.ResponseMeta{Usage: result.Usage, FinishReason: result.Choices[0].FinishReason}
		if result.Usage != nil {
			span.SetAttributes(attribute.Int("llm.token_count.prompt", result.Usage.PromptTokens),
				attribute.Int("llm.token_count.completion", result.Usage.CompletionTokens),
				attribute.Int("llm.token_count.total", result.Usage.TotalTokens))
		}
		return out, nil
	}
	return nil, ErrUnavailable
}

func retryableStatus(status int) bool {
	return status == http.StatusTooManyRequests ||
		status == http.StatusInternalServerError ||
		status == http.StatusBadGateway ||
		status == http.StatusServiceUnavailable ||
		status == http.StatusGatewayTimeout
}

func retryDelay(value string) (time.Duration, bool) {
	if value == "" {
		return defaultRetryWait, true
	}
	if seconds, err := strconv.Atoi(value); err == nil && seconds >= 0 {
		delay := time.Duration(seconds) * time.Second
		return delay, delay <= maxRetryWait
	}
	when, err := http.ParseTime(value)
	if err != nil {
		return 0, false
	}
	delay := time.Until(when)
	if delay < 0 {
		delay = 0
	}
	return delay, delay <= maxRetryWait
}

func waitForRetry(ctx context.Context, delay time.Duration) bool {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-timer.C:
		return true
	case <-ctx.Done():
		return false
	}
}

func (m *OpenAI) Stream(ctx context.Context, in []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	// QA-01 validates a complete answer before exposing it to the client.
	out, err := m.Generate(ctx, in, opts...)
	if err != nil {
		return nil, err
	}
	return schema.StreamReaderFromArray([]*schema.Message{out}), nil
}
