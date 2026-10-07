package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"

	"hwops/internal/domain"
	"hwops/internal/einoflow"
	"hwops/internal/sandbox"
)

const toolLogLimit = 32 * 1024

type workbenchTurn struct {
	ResponseID     string
	ConversationID string
}

type workbenchTurnKey struct{}

func withWorkbenchTurn(ctx context.Context, response domain.Response) context.Context {
	return context.WithValue(ctx, workbenchTurnKey{}, workbenchTurn{
		ResponseID: response.ID, ConversationID: response.ConversationID,
	})
}

type workbenchTool struct {
	info   *schema.ToolInfo
	invoke func(context.Context, string) (any, error)
}

func (t *workbenchTool) Info(context.Context) (*schema.ToolInfo, error) {
	return t.info, nil
}

func (t *workbenchTool) InvokableRun(ctx context.Context, arguments string, _ ...tool.Option) (string, error) {
	result, err := t.invoke(ctx, arguments)
	if err != nil {
		return "", err
	}
	raw, err := json.Marshal(result)
	if err != nil || len(raw) > 256*1024 {
		return "", domain.ErrResourceExhausted
	}
	return string(raw), nil
}

func (a *App) workbenchAgentTools() []tool.InvokableTool {
	var tools []tool.InvokableTool
	if a.attachments != nil {
		tools = append(tools, &workbenchTool{
			info: &schema.ToolInfo{
				Name: einoflow.ReadAttachmentToolName,
				Desc: "读取当前私有会话中的附件页、日志行或静态图片。只提供附件ID和所需范围；大文本必须按行缩小范围。",
				ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
					"attachment_id": {Type: schema.String, Required: true},
					"page":          {Type: schema.Integer},
					"start_line":    {Type: schema.Integer},
					"end_line":      {Type: schema.Integer},
					"prompt":        {Type: schema.String},
				}),
			},
			invoke: a.readAttachmentTool,
		})
	}
	if a.python != nil {
		tools = append(tools, &workbenchTool{
			info: &schema.ToolInfo{
				Name: einoflow.RunPythonToolName,
				Desc: "在固定Linux/gVisor Python环境中分析当前会话附件或前轮产物。input_ids只能使用上下文中的实际ID，代码从/inputs/<ID>读取并向/work/output写产物。",
				ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
					"code": {Type: schema.String, Required: true},
					"input_ids": {
						Type: schema.Array, ElemInfo: &schema.ParameterInfo{Type: schema.String},
					},
				}),
			},
			invoke: a.runPythonTool,
		})
	}
	return tools
}

type attachmentToolInput struct {
	AttachmentID string `json:"attachment_id"`
	Page         int    `json:"page,omitempty"`
	StartLine    int    `json:"start_line,omitempty"`
	EndLine      int    `json:"end_line,omitempty"`
	Prompt       string `json:"prompt,omitempty"`
}

func (a *App) readAttachmentTool(ctx context.Context, arguments string) (any, error) {
	var input attachmentToolInput
	if err := strictToolArguments(arguments, &input); err != nil ||
		input.AttachmentID == "" || input.Page < 0 || input.StartLine < 0 || input.EndLine < 0 ||
		(input.StartLine == 0) != (input.EndLine == 0) || input.EndLine < input.StartLine ||
		len(input.Prompt) > 4000 {
		return nil, domain.ErrInvalid
	}
	turn, ok := ctx.Value(workbenchTurnKey{}).(workbenchTurn)
	if !ok {
		return nil, domain.ErrForbidden
	}
	attachment, err := a.attachments.Get(ctx, input.AttachmentID)
	if errors.Is(err, domain.ErrNotFound) || (err == nil && attachment.ConversationID != turn.ConversationID) {
		return domain.AttachmentReadResult{Status: "NOT_FOUND"}, nil
	}
	if err != nil {
		return nil, err
	}
	if attachment.MediaType == "image/png" || attachment.MediaType == "image/jpeg" ||
		attachment.MediaType == "image/webp" {
		if strings.TrimSpace(input.Prompt) == "" || input.Page > 1 || input.StartLine != 0 {
			return domain.AttachmentReadResult{Status: "INVALID_RANGE", Name: attachment.Name,
				MediaType: attachment.MediaType}, nil
		}
		visual, source, usage, err := a.AnalyzeAttachmentImage(ctx, attachment.ID, input.Prompt)
		if err != nil {
			return nil, err
		}
		source.SourceID = attachmentSourceID(source)
		return domain.AttachmentReadResult{
			Status: "OK", Name: attachment.Name, MediaType: attachment.MediaType,
			Visual: visual, Source: &source, Gaps: attachment.Gaps, ModelUsage: usage,
		}, nil
	}
	page := input.Page
	if page == 0 {
		page = 1
	}
	result, err := a.attachments.Page(ctx, attachment.ID, page)
	if errors.Is(err, domain.ErrNotFound) {
		return domain.AttachmentReadResult{Status: "NOT_FOUND", Name: attachment.Name,
			MediaType: attachment.MediaType}, nil
	}
	if err != nil {
		return nil, err
	}
	text := result.Page.Text
	if input.StartLine > 0 {
		text, err = selectLines(text, result.Page.StartLine, result.Page.EndLine, input.StartLine, input.EndLine)
		if err != nil {
			return domain.AttachmentReadResult{Status: "INVALID_RANGE", Name: attachment.Name,
				MediaType: attachment.MediaType, Gaps: attachment.Gaps}, nil
		}
		result.Source.StartLine, result.Source.EndLine = input.StartLine, input.EndLine
		result.Source.ContentSHA256 = sandbox.Hash([]byte(text))
	}
	if len(text) > 48*1024 {
		return domain.AttachmentReadResult{
			Status: "NEEDS_RANGE", Name: attachment.Name, MediaType: attachment.MediaType,
			Gaps: append(append([]domain.AttachmentGap{}, attachment.Gaps...), domain.AttachmentGap{
				Page: page, Code: "OUTPUT_RANGE_REQUIRED",
				Text: fmt.Sprintf("该页内容超过工具上限，请在 %d-%d 行内指定更小范围。", result.Page.StartLine, result.Page.EndLine),
			}),
		}, nil
	}
	result.Source.SourceID = attachmentSourceID(result.Source)
	return domain.AttachmentReadResult{
		Status: "OK", Name: attachment.Name, MediaType: attachment.MediaType,
		Text: text, Tables: result.Page.Tables, Source: &result.Source, Gaps: attachment.Gaps,
	}, nil
}

func selectLines(text string, availableStart, availableEnd, start, end int) (string, error) {
	if availableStart < 1 || start < availableStart || end > availableEnd {
		return "", domain.ErrInvalid
	}
	lines := strings.Split(text, "\n")
	from, through := start-availableStart, end-availableStart+1
	if from < 0 || through > len(lines) || from >= through {
		return "", domain.ErrInvalid
	}
	return strings.Join(lines[from:through], "\n"), nil
}

func attachmentSourceID(source domain.SourceRef) string {
	return fmt.Sprintf("attachment:%s:%d:%d:%d:%s", source.AttachmentID, source.Page,
		source.StartLine, source.EndLine, source.ContentSHA256)
}

type pythonToolInput struct {
	Code     string   `json:"code"`
	InputIDs []string `json:"input_ids,omitempty"`
}

func (a *App) runPythonTool(ctx context.Context, arguments string) (any, error) {
	var input pythonToolInput
	if err := strictToolArguments(arguments, &input); err != nil ||
		len(input.Code) == 0 || len(input.Code) > 64*1024 || len(input.InputIDs) > 20 {
		return nil, domain.ErrInvalid
	}
	seen := map[string]bool{}
	for _, id := range input.InputIDs {
		if id == "" || seen[id] {
			return nil, domain.ErrInvalid
		}
		seen[id] = true
	}
	turn, ok := ctx.Value(workbenchTurnKey{}).(workbenchTurn)
	if !ok {
		return nil, domain.ErrForbidden
	}
	execution, err := a.StartPython(ctx, turn.ResponseID, input.Code, input.InputIDs)
	if errors.Is(err, domain.ErrNotFound) || errors.Is(err, domain.ErrConflict) ||
		errors.Is(err, domain.ErrInvalid) {
		return domain.PythonAnalysisResult{Status: "REJECTED", Error: &domain.Failure{
			Code: "INPUT_OR_BUDGET_REJECTED",
			Message: "输入文件不属于当前会话、调用预算已用尽或执行参数无效。",
		}}, nil
	}
	if err != nil {
		return nil, err
	}
	executionID := execution.ID
	execution, err = a.emitToolProgress(ctx, turn.ResponseID, executionID)
	if err != nil {
		a.cancelPythonTool(ctx, executionID)
		return nil, err
	}
	lastVersion := execution.Version
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		execution, err = a.PythonExecution(ctx, execution.ID)
		if err != nil {
			return nil, err
		}
		if execution.Version != lastVersion {
			execution, err = a.emitToolProgress(ctx, turn.ResponseID, execution.ID)
			if err != nil {
				a.cancelPythonTool(ctx, execution.ID)
				return nil, err
			}
			lastVersion = execution.Version
		}
		if domain.PythonTerminal(execution.Status) && execution.Result.Cleaned {
			break
		}
		select {
		case <-ctx.Done():
			_, _ = a.WaitPython(ctx, execution.ID)
			return nil, ctx.Err()
		case <-ticker.C:
		}
	}
	compact, stdoutTruncated, stderrTruncated := compactExecution(execution)
	result := domain.PythonAnalysisResult{
		Status: execution.Status, Execution: compact,
		StdoutTruncated: stdoutTruncated, StderrTruncated: stderrTruncated,
	}
	if execution.Status == "SUCCEEDED" {
		source := domain.ExecutionSource(execution)
		result.Source = &source
	}
	return result, nil
}

func (a *App) emitToolProgress(ctx context.Context, responseID, executionID string) (domain.PythonExecution, error) {
	var execution domain.PythonExecution
	for range 5 {
		current, err := a.PythonExecution(ctx, executionID)
		if err != nil {
			return current, err
		}
		execution = current
		compact, _, _ := compactExecution(current)
		if _, err = a.draftEvents.AppendToolProgress(ctx, responseID, compact); err == nil {
			return current, nil
		}
		if !errors.Is(err, domain.ErrConflict) {
			return current, err
		}
	}
	return execution, domain.ErrConflict
}

func (a *App) cancelPythonTool(ctx context.Context, id string) {
	repo, ok := a.store.(domain.PythonRepository)
	if ok {
		_ = repo.CancelPythonExecution(context.WithoutCancel(ctx), id, "CANCELED")
	}
}

func compactExecution(execution domain.PythonExecution) (domain.PythonExecution, bool, bool) {
	out := execution
	var stdoutTruncated, stderrTruncated bool
	if len(out.Result.Stdout) > toolLogLimit {
		out.Result.Stdout = out.Result.Stdout[:toolLogLimit]
		stdoutTruncated = true
	}
	if len(out.Result.Stderr) > toolLogLimit {
		out.Result.Stderr = out.Result.Stderr[:toolLogLimit]
		stderrTruncated = true
	}
	out.StdoutTruncated, out.StderrTruncated = stdoutTruncated, stderrTruncated
	return out, stdoutTruncated, stderrTruncated
}

func strictToolArguments(raw string, out any) error {
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(out); err != nil {
		return err
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return domain.ErrInvalid
	}
	return nil
}
