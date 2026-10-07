package contracts_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cloudwego/eino/schema"

	"hwops/internal/adapters/chatmodel"
)

func TestDeadlineWhileReadingModelBody(t *testing.T) {
	headers := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		close(headers)
		<-r.Context().Done()
	}))
	defer server.Close()
	cm, err := chatmodel.NewOpenAI(server.URL, "contract-model", "test-key")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	defer cancel()
	finished := make(chan error, 1)
	go func() {
		_, err := cm.Generate(ctx, []*schema.Message{schema.UserMessage("hello")})
		finished <- err
	}()
	select {
	case <-headers:
	case <-ctx.Done():
		t.Fatal("model server did not receive request")
	}
	select {
	case err := <-finished:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("deadline lost after headers: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("cancel did not interrupt model response")
	}
}

func TestOpenAIProtocolAndFailureSemantics(t *testing.T) {
	for _, scenario := range []struct {
		name   string
		status int
		body   string
		valid  bool
	}{
		{"success", 200, `{"choices":[{"message":{"content":"model response"}}]}`, true},
		{"unavailable", 503, "private upstream failure details", false},
		{"redirect", 302, "", false},
		{"malformed", 200, "not json", false},
		{"no_choice", 200, `{"choices":[]}`, false},
		{"oversized", 200, strings.Repeat("a", 1024*1024+1), false},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var input struct {
					Model    string            `json:"model"`
					Messages []*schema.Message `json:"messages"`
					Stream   bool              `json:"stream"`
				}
				if err := json.NewDecoder(r.Body).Decode(&input); err != nil || input.Model != "contract-model" ||
					input.Stream || len(input.Messages) != 1 || input.Messages[0].Content != "hello" ||
					input.Messages[0].Role != schema.User || r.Method != "POST" ||
					r.Header.Get("Authorization") != "Bearer test-key" {
					t.Error("unexpected model protocol request")
				}
				if scenario.status == 302 {
					w.Header().Set("Location", "/should-not-follow")
				}
				w.WriteHeader(scenario.status)
				_, _ = w.Write([]byte(scenario.body))
			}))
			defer server.Close()
			cm, err := chatmodel.NewOpenAI(server.URL, "contract-model", "test-key")
			if err != nil {
				t.Fatal(err)
			}
			out, err := cm.Generate(context.Background(), []*schema.Message{schema.UserMessage("hello")})
			if scenario.valid {
				if err != nil || out == nil || out.Content != "model response" {
					t.Fatalf("model result lost: %v %v", out, err)
				}
			} else if !errors.Is(err, chatmodel.ErrUnavailable) || out != nil ||
				strings.Contains(err.Error(), "private upstream") {
				t.Fatalf("unavailable model did not fail cleanly: %v %v", out, err)
			}
		})
	}
}

func TestOpenAIClientIdentityHeaders(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") != "hwops/1.0" {
			t.Errorf("unexpected user agent: %q", r.Header.Get("User-Agent"))
		}
		if r.Header.Get("x-opencode-session") != "conversation-123" {
			t.Errorf("unexpected OpenCode session: %q", r.Header.Get("x-opencode-session"))
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"model response"}}]}`))
	}))
	defer server.Close()
	cm, err := chatmodel.NewOpenAI(server.URL, "contract-model", "")
	if err != nil {
		t.Fatal(err)
	}
	ctx := chatmodel.WithSessionID(context.Background(), "conversation-123")
	if _, err := cm.Generate(ctx, []*schema.Message{schema.UserMessage("hello")}); err != nil {
		t.Fatal(err)
	}
}

func TestDeepSeekDefaultThinkingAndActualUsage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		thinking, _ := body["thinking"].(map[string]any)
		if thinking["type"] != "enabled" || body["model"] != "deepseek-flash" {
			t.Errorf("DeepSeek thinking configuration missing: %v", body)
		}
		w.Write([]byte(`{"choices":[{"message":{"content":"真实适配器协议测试 REPLAY"}}],"usage":{"prompt_tokens":17,"completion_tokens":9,"total_tokens":26}}`))
	}))
	defer server.Close()
	cm, err := chatmodel.NewOpenAI(server.URL, "deepseek-flash", "")
	if err != nil {
		t.Fatal(err)
	}
	out, err := cm.Generate(context.Background(), []*schema.Message{schema.UserMessage("hello")})
	if err != nil || out == nil || out.ResponseMeta == nil || out.ResponseMeta.Usage == nil ||
		out.ResponseMeta.Usage.PromptTokens != 17 || out.ResponseMeta.Usage.CompletionTokens != 9 ||
		out.ResponseMeta.Usage.TotalTokens != 26 {
		t.Fatalf("provider usage lost: %+v %v", out, err)
	}
}

func TestOpenAIRetriesTransientFailureAndReturnsSuccessfulUsage(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if requests.Add(1) == 1 {
			w.Header().Set("Retry-After", "0")
			http.Error(w, "temporary overload", http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte(`{
			"choices":[{"message":{"content":"recovered response"}}],
			"usage":{"prompt_tokens":11,"completion_tokens":7,"total_tokens":18}
		}`))
	}))
	defer server.Close()

	cm, err := chatmodel.NewOpenAI(server.URL, "contract-model", "test-key")
	if err != nil {
		t.Fatal(err)
	}
	out, err := cm.Generate(context.Background(), []*schema.Message{schema.UserMessage("hello")})
	if err != nil || out == nil || out.Content != "recovered response" ||
		out.ResponseMeta == nil || out.ResponseMeta.Usage == nil ||
		out.ResponseMeta.Usage.TotalTokens != 18 {
		t.Fatalf("transient model failure was not recovered: %+v %v", out, err)
	}
}

func TestOpenAIRetriesSupportedTransientStatuses(t *testing.T) {
	for _, status := range []int{
		http.StatusTooManyRequests,
		http.StatusInternalServerError,
		http.StatusBadGateway,
		http.StatusServiceUnavailable,
		http.StatusGatewayTimeout,
	} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if requests.Add(1) == 1 {
					w.Header().Set("Retry-After", "0")
					http.Error(w, "temporary failure", status)
					return
				}
				_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"recovered"}}]}`))
			}))
			defer server.Close()

			cm, err := chatmodel.NewOpenAI(server.URL, "contract-model", "")
			if err != nil {
				t.Fatal(err)
			}
			out, err := cm.Generate(context.Background(), []*schema.Message{schema.UserMessage("hello")})
			if err != nil || out == nil || out.Content != "recovered" {
				t.Fatalf("HTTP %d was not recovered: %+v %v", status, out, err)
			}
		})
	}
}

func TestOpenAIRetriesTransportFailure(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if requests.Add(1) == 1 {
			conn, _, err := w.(http.Hijacker).Hijack()
			if err != nil {
				t.Error(err)
				return
			}
			_ = conn.Close()
			return
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"recovered"}}]}`))
	}))
	defer server.Close()

	cm, err := chatmodel.NewOpenAI(server.URL, "contract-model", "")
	if err != nil {
		t.Fatal(err)
	}
	out, err := cm.Generate(context.Background(), []*schema.Message{schema.UserMessage("hello")})
	if err != nil || out == nil || out.Content != "recovered" {
		t.Fatalf("transport failure was not recovered: %+v %v", out, err)
	}
}

func TestOpenAIDoesNotRetryNonTransientClientFailure(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		http.Error(w, "invalid request", http.StatusBadRequest)
	}))
	defer server.Close()

	cm, err := chatmodel.NewOpenAI(server.URL, "contract-model", "")
	if err != nil {
		t.Fatal(err)
	}
	out, err := cm.Generate(context.Background(), []*schema.Message{schema.UserMessage("hello")})
	if out != nil || !errors.Is(err, chatmodel.ErrUnavailable) || requests.Load() != 1 {
		t.Fatalf("non-transient client failure was retried: requests=%d out=%+v err=%v", requests.Load(), out, err)
	}
}

func TestOpenAIRetryRespectsContextDuringRetryAfter(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.Header().Set("Retry-After", "1")
		http.Error(w, "rate limited", http.StatusTooManyRequests)
	}))
	defer server.Close()

	cm, err := chatmodel.NewOpenAI(server.URL, "contract-model", "")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	out, err := cm.Generate(ctx, []*schema.Message{schema.UserMessage("hello")})
	if out != nil || !errors.Is(err, context.DeadlineExceeded) || requests.Load() != 1 {
		t.Fatalf("retry ignored context deadline: requests=%d out=%+v err=%v", requests.Load(), out, err)
	}
}

func TestOpenAIRetryAttemptsAreBounded(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.Header().Set("Retry-After", "0")
		http.Error(w, "temporary failure", http.StatusServiceUnavailable)
	}))
	defer server.Close()

	cm, err := chatmodel.NewOpenAI(server.URL, "contract-model", "")
	if err != nil {
		t.Fatal(err)
	}
	out, err := cm.Generate(context.Background(), []*schema.Message{schema.UserMessage("hello")})
	if out != nil || !errors.Is(err, chatmodel.ErrUnavailable) || requests.Load() != maxExpectedModelAttempts {
		t.Fatalf("retry attempts were not bounded: requests=%d out=%+v err=%v", requests.Load(), out, err)
	}
}

const maxExpectedModelAttempts = 2
