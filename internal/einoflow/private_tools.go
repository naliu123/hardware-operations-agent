package einoflow

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"

	"hwops/internal/domain"
)

const (
	ReadAttachmentToolName = "read_private_attachment"
	RunPythonToolName      = "run_python_analysis"
	maxPrivateToolCalls    = 12
)

func invokePrivate(ctx context.Context, t *turn, tools map[string]tool.InvokableTool) error {
	if len(t.Last.ToolCalls) == 0 ||
		t.PrivateCalls+len(t.Last.ToolCalls) > maxPrivateToolCalls {
		return ErrToolBudget
	}
	for _, call := range t.Last.ToolCalls {
		if err := invokeOnePrivate(ctx, t, tools, call); err != nil {
			return err
		}
	}
	return nil
}

func invokeOnePrivate(ctx context.Context, t *turn, tools map[string]tool.InvokableTool, call schema.ToolCall) error {
	selected := tools[call.Function.Name]
	if selected == nil || call.Type != "function" || strings.TrimSpace(call.ID) == "" ||
		t.CallIDs[call.ID] || len(call.Function.Arguments) > 80*1024 {
		return ErrInvalidAnswer
	}
	t.CallIDs[call.ID] = true
	t.PrivateCalls++
	raw, err := selected.InvokableRun(ctx, call.Function.Arguments)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return err
		}
		return ErrInvalidAnswer
	}
	if len(raw) == 0 || len(raw) > 256*1024 {
		return ErrInvalidAnswer
	}
	switch call.Function.Name {
	case ReadAttachmentToolName:
		var result domain.AttachmentReadResult
		if strictToolResult(raw, &result) != nil || result.Status == "" {
			return ErrInvalidAnswer
		}
		if result.Source != nil {
			if result.Status != "OK" || result.Source.SourceKind != "ATTACHMENT" ||
				result.Source.SourceID == "" {
				return ErrInvalidAnswer
			}
			if err = appendSource(&t.Response, *result.Source); err != nil {
				return err
			}
		}
		mergeUsage(t, result.ModelUsage)
	case RunPythonToolName:
		var result domain.PythonAnalysisResult
		if strictToolResult(raw, &result) != nil || result.Status == "" {
			return ErrInvalidAnswer
		}
		if result.Execution.ID != "" {
			if result.Execution.ResponseID != t.Response.ID ||
				result.Execution.ConversationID != t.Response.ConversationID {
				return ErrInvalidAnswer
			}
			t.Response.Executions = append(t.Response.Executions, result.Execution)
		}
		if result.Source != nil {
			if result.Execution.Status != "SUCCEEDED" || result.Source.SourceKind != "EXECUTION" ||
				result.Source.SourceID == "" || result.Source.ExecutionID != result.Execution.ID {
				return ErrInvalidAnswer
			}
			if err = appendSource(&t.Response, *result.Source); err != nil {
				return err
			}
		}
	default:
		return ErrInvalidAnswer
	}
	t.Messages = append(t.Messages, schema.ToolMessage(raw, call.ID))
	return nil
}

func strictToolResult(raw string, out any) error {
	decoder := json.NewDecoder(strings.NewReader(raw))
	if err := decoder.Decode(out); err != nil {
		return err
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return domain.ErrInvalid
	}
	return nil
}

func appendSource(response *domain.Response, source domain.SourceRef) error {
	for _, saved := range response.Sources {
		if saved.SourceID == source.SourceID {
			if saved.ContentSHA256 != source.ContentSHA256 || saved.SourceKind != source.SourceKind {
				return ErrInvalidAnswer
			}
			return nil
		}
	}
	response.Sources = append(response.Sources, source)
	return nil
}
