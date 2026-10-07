package contracts_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cloudwego/eino/schema"

	"hwops/internal/adapters/chatmodel"
)

func streamFrame(t *testing.T, w http.ResponseWriter, value any) {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = fmt.Fprintf(w, "data: %s\n\n", raw)
	w.(http.Flusher).Flush()
}

func streamDone(w http.ResponseWriter) {
	_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
	w.(http.Flusher).Flush()
}

func TestOpenAINativeStreamDeliversBeforeFinishAndPreservesUsage(t *testing.T) {
	firstSent, release := make(chan struct{}), make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			Stream bool `json:"stream"`
		}
		if json.NewDecoder(r.Body).Decode(&input) != nil || !input.Stream ||
			r.Header.Get("Accept") != "text/event-stream" {
			http.Error(w, "stream protocol required", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		streamFrame(t, w, map[string]any{"choices": []any{map[string]any{
			"index": 0, "delta": map[string]any{"content": `{"claims":[{"text":"first `},
		}}})
		close(firstSent)
		<-release
		streamFrame(t, w, map[string]any{"choices": []any{map[string]any{
			"index": 0, "delta": map[string]any{"content": `second"}],"gaps":[]}`},
		}}})
		streamFrame(t, w, map[string]any{
			"choices": []any{map[string]any{"index": 0, "delta": map[string]any{}, "finish_reason": "stop"}},
		})
		streamFrame(t, w, map[string]any{
			"choices": []any{},
			"usage":   map[string]int{"prompt_tokens": 13, "completion_tokens": 8, "total_tokens": 21},
		})
		streamDone(w)
	}))
	defer server.Close()

	cm, err := chatmodel.NewOpenAI(server.URL, "stream-model", "")
	if err != nil {
		t.Fatal(err)
	}
	stream, err := cm.Stream(context.Background(), []*schema.Message{schema.UserMessage("hello")})
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	<-firstSent
	first, err := stream.Recv()
	if err != nil || first.Content != `{"claims":[{"text":"first ` {
		t.Fatalf("first live chunk missing: %+v %v", first, err)
	}
	close(release)
	chunks := []*schema.Message{first}
	for {
		chunk, recvErr := stream.Recv()
		if errors.Is(recvErr, io.EOF) {
			break
		}
		if recvErr != nil {
			t.Fatal(recvErr)
		}
		chunks = append(chunks, chunk)
	}
	out, err := schema.ConcatMessages(chunks)
	if err != nil || out.Content != `{"claims":[{"text":"first second"}],"gaps":[]}` ||
		out.ResponseMeta == nil || out.ResponseMeta.FinishReason != "stop" ||
		out.ResponseMeta.Usage == nil || out.ResponseMeta.Usage.TotalTokens != 21 {
		t.Fatalf("stream result lost content or usage: %+v %v", out, err)
	}
}

func TestOpenAIStreamAssemblesToolFragmentsBeforeExposure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
		index := 0
		streamFrame(t, w, map[string]any{"choices": []any{map[string]any{
			"index": 0, "delta": map[string]any{"tool_calls": []schema.ToolCall{{
				Index: &index, ID: "call-", Type: "function",
				Function: schema.FunctionCall{Name: "retrieve_", Arguments: `{"request":"`},
			}}},
		}}})
		streamFrame(t, w, map[string]any{"choices": []any{map[string]any{
			"index": 0, "delta": map[string]any{"tool_calls": []schema.ToolCall{{
				Index: &index, ID: "1",
				Function: schema.FunctionCall{Name: "hardware_knowledge", Arguments: `blue"}`},
			}}},
		}}})
		streamFrame(t, w, map[string]any{"choices": []any{map[string]any{
			"index": 0, "delta": map[string]any{}, "finish_reason": "tool_calls",
		}}})
		streamDone(w)
	}))
	defer server.Close()
	cm, _ := chatmodel.NewOpenAI(server.URL, "stream-model", "")
	stream, err := cm.Stream(context.Background(), []*schema.Message{schema.UserMessage("tool")})
	if err != nil {
		t.Fatal(err)
	}
	out, err := schema.ConcatMessageStream(stream)
	if err != nil || len(out.ToolCalls) != 1 || out.ToolCalls[0].ID != "call-1" ||
		out.ToolCalls[0].Function.Name != "retrieve_hardware_knowledge" ||
		out.ToolCalls[0].Function.Arguments != `{"request":"blue"}` {
		t.Fatalf("tool fragments were not assembled: %+v %v", out, err)
	}
}

func TestOpenAIStreamReportsAbruptEndAndCancellation(t *testing.T) {
	t.Run("abrupt_end", func(t *testing.T) {
		var requests atomic.Int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			requests.Add(1)
			w.Header().Set("Content-Type", "text/event-stream")
			streamFrame(t, w, map[string]any{"choices": []any{map[string]any{
				"index": 0, "delta": map[string]any{"content": "partial"},
			}}})
		}))
		defer server.Close()
		cm, _ := chatmodel.NewOpenAI(server.URL, "stream-model", "")
		stream, err := cm.Stream(context.Background(), []*schema.Message{schema.UserMessage("hello")})
		if err != nil {
			t.Fatal(err)
		}
		defer stream.Close()
		if chunk, err := stream.Recv(); err != nil || chunk.Content != "partial" {
			t.Fatalf("partial chunk missing: %+v %v", chunk, err)
		}
		if _, err := stream.Recv(); !errors.Is(err, chatmodel.ErrUnavailable) {
			t.Fatalf("abrupt end was accepted: %v", err)
		}
		if requests.Load() != 1 {
			t.Fatalf("partially delivered stream was retried: %d", requests.Load())
		}
	})

	t.Run("cancellation", func(t *testing.T) {
		received := make(chan struct{})
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)
			w.(http.Flusher).Flush()
			close(received)
			<-r.Context().Done()
		}))
		defer server.Close()
		cm, _ := chatmodel.NewOpenAI(server.URL, "stream-model", "")
		ctx, cancel := context.WithCancel(context.Background())
		stream, err := cm.Stream(ctx, []*schema.Message{schema.UserMessage("hello")})
		if err != nil {
			t.Fatal(err)
		}
		defer stream.Close()
		<-received
		cancel()
		done := make(chan error, 1)
		go func() {
			_, recvErr := stream.Recv()
			done <- recvErr
		}()
		select {
		case err := <-done:
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("stream cancellation lost: %v", err)
			}
		case <-time.After(time.Second):
			t.Fatal("stream cancellation did not unblock Recv")
		}
	})
}

func TestOpenAIStreamRetriesOnlyBeforeDelivery(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if requests.Add(1) == 1 {
			w.Header().Set("Retry-After", "0")
			http.Error(w, "temporary", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		streamFrame(t, w, map[string]any{"choices": []any{map[string]any{
			"index": 0, "delta": map[string]any{"content": "recovered"},
		}}})
		streamFrame(t, w, map[string]any{"choices": []any{map[string]any{
			"index": 0, "delta": map[string]any{}, "finish_reason": "stop",
		}}})
		streamDone(w)
	}))
	defer server.Close()
	cm, _ := chatmodel.NewOpenAI(server.URL, "stream-model", "")
	stream, err := cm.Stream(context.Background(), []*schema.Message{schema.UserMessage("hello")})
	if err != nil {
		t.Fatal(err)
	}
	out, err := schema.ConcatMessageStream(stream)
	if err != nil || out.Content != "recovered" || requests.Load() != 2 {
		t.Fatalf("pre-delivery retry failed: requests=%d out=%+v err=%v", requests.Load(), out, err)
	}
}
