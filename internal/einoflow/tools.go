package einoflow

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"

	"hwops/internal/adapters/chatmodel"
	knowledgeagent "hwops/internal/agents/knowledge"
	"hwops/internal/domain"
)

var ErrToolBudget = errors.New("knowledge tool budget exhausted")

const maxKnowledgeCalls = 3
const maxEvidenceBytes = 48 * 1024

func invokeKnowledge(ctx context.Context, t *turn, knowledgeTool tool.InvokableTool) error {
	if len(t.Last.ToolCalls) > maxKnowledgeCalls-t.Calls {
		for _, call := range t.Last.ToolCalls {
			t.Response.KnowledgeToolCalls = append(t.Response.KnowledgeToolCalls, domain.KnowledgeToolCall{
				ID: call.ID, Name: call.Function.Name, Status: "FAILED",
				Error: &domain.Failure{Code: "TOOL_BUDGET_EXCEEDED", Message: "知识检索调用超过三次预算。"},
			})
		}
		return ErrToolBudget
	}
	for _, call := range t.Last.ToolCalls {
		record := domain.KnowledgeToolCall{ID: call.ID, Name: call.Function.Name, Status: "FAILED"}
		t.Response.KnowledgeToolCalls = append(t.Response.KnowledgeToolCalls, record)
		current := &t.Response.KnowledgeToolCalls[len(t.Response.KnowledgeToolCalls)-1]
		var args struct {
			Request string `json:"request"`
		}
		decoder := json.NewDecoder(strings.NewReader(call.Function.Arguments))
		decoder.DisallowUnknownFields()
		if call.Type != "function" || strings.TrimSpace(call.ID) == "" || t.CallIDs[call.ID] ||
			call.Function.Name != knowledgeagent.ToolName || decoder.Decode(&args) != nil ||
			decoder.Decode(new(any)) != io.EOF || strings.TrimSpace(args.Request) == "" || len(args.Request) > 16000 {
			current.Error = &domain.Failure{Code: "INVALID_TOOL_CALL", Message: "知识工具名称、调用标识或参数无效。"}
			return ErrInvalidAnswer
		}
		current.Request = strings.TrimSpace(args.Request)
		queryKey := strings.ToLower(strings.Join(strings.Fields(args.Request), " "))
		if t.Queries[queryKey] {
			current.Error = &domain.Failure{Code: "REPEATED_TOOL_CALL", Message: "相同设备上下文下不得重复知识查询。"}
			return ErrToolBudget
		}
		t.Queries[queryKey], t.CallIDs[call.ID] = true, true
		t.Calls++
		result, err := knowledgeagent.Invoke(ctx, knowledgeTool, current.Request,
			t.Response.DeviceContext, t.Response.DataMode)
		current.Status, current.Error = result.Status, result.Error
		current.RetrievalQueries, current.Gaps = result.RetrievalQueries, result.Gaps
		current.TraceID, current.ModelUsage = result.TraceID, result.ModelUsage
		current.EvidenceSelection = result.EvidenceSelection
		t.Response.ApplicabilityChecks = append(t.Response.ApplicabilityChecks, result.ApplicabilityChecks...)
		t.Response.EvidenceSelection = result.EvidenceSelection
		mergeUsage(t, result.ModelUsage)
		if err != nil {
			if current.Status == "" {
				current.Status = "FAILED"
			}
			return err
		}
		t.NeedsContext = t.NeedsContext || result.Status == knowledgeagent.StatusNeedsContext
		seen := map[string]bool{}
		for _, doc := range t.Docs {
			seen[doc.ID] = true
		}
		docs := []*schema.Document{}
		known := []string{}
		budgetLimited := false
		for _, doc := range result.EinoDocuments() {
			if seen[doc.ID] {
				known = append(known, doc.ID)
				current.FragmentIDs = append(current.FragmentIDs, doc.ID)
				continue
			}
			next := append(append([]*schema.Document{}, t.Docs...), doc)
			raw, err := json.Marshal(next)
			if err != nil {
				return err
			}
			if len(raw) > maxEvidenceBytes {
				budgetLimited = true
				continue
			}
			t.Docs = next
			seen[doc.ID] = true
			docs = append(docs, doc)
			current.FragmentIDs = append(current.FragmentIDs, doc.ID)
			t.Response.RetrievedFragmentIDs = append(t.Response.RetrievedFragmentIDs, doc.ID)
		}
		gaps := append([]string{}, result.Gaps...)
		if budgetLimited {
			gaps = append(gaps, "累计证据达到48 KiB预算，部分新片段未提供；只能引用已提供的片段。")
		}
		current.Gaps = gaps
		if len(t.Docs) == 0 {
			t.Response.Gaps = gaps
		}
		// Only trusted, budgeted documents enter the model conversation. Repeated
		// evidence is referenced by ID instead of duplicating its context bytes.
		payload, err := json.Marshal(struct {
			chatmodel.ContextInput
			Status string   `json:"status"`
			Gaps   []string `json:"gaps"`
			Known  []string `json:"previously_returned_fragment_ids,omitempty"`
		}{
			ContextInput: chatmodel.ContextInput{Question: current.Request, Documents: docs, Device: t.Response.DeviceContext},
			Status:       result.Status, Gaps: gaps, Known: known,
		})
		if err != nil {
			return err
		}
		t.Messages = append(t.Messages, schema.ToolMessage(string(payload), call.ID))
	}
	return nil
}
