package knowledgeagent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/retriever"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"

	"hwops/internal/adapters/chatmodel"
	"hwops/internal/domain"
	"hwops/internal/knowledge"
	"hwops/internal/observability"
)

const (
	ToolName = "retrieve_hardware_knowledge"

	StatusFound        = "FOUND"
	StatusNeedsContext = "NEEDS_CONTEXT"
	StatusNotFound     = "NOT_FOUND"
	StatusFailed       = "FAILED"
)

var ErrInvalidResult = errors.New("invalid knowledge retrieval result")

type Config struct {
	Store             domain.Repository
	Retriever         retriever.Retriever
	Selector          model.BaseChatModel
	DataMode          string
	ContextLimit      int
	EvidenceSelection bool
}

type Result struct {
	SchemaVersion       int                        `json:"schema_version"`
	Status              string                     `json:"status"`
	DataMode            string                     `json:"data_mode"`
	Evidence            []knowledge.SearchDocument `json:"evidence"`
	ApplicabilityChecks []domain.KnowledgeCheck    `json:"applicability_checks"`
	RetrievalQueries    []string                   `json:"retrieval_queries,omitempty"`
	ExpandedFragmentIDs []string                   `json:"expanded_fragment_ids,omitempty"`
	Gaps                []string                   `json:"gaps"`
	TraceID             string                     `json:"trace_id,omitempty"`
	EvidenceSelection   *domain.EvidenceSelection  `json:"evidence_selection,omitempty"`
	ModelUsage          *domain.ModelUsage         `json:"model_usage,omitempty"`
	Error               *domain.Failure            `json:"error,omitempty"`
}

func (r Result) EinoDocuments() []*schema.Document {
	search := knowledge.SearchResult{Documents: r.Evidence}
	return search.EinoDocuments()
}

func (r Result) Validate() error {
	if r.SchemaVersion != 1 || (r.DataMode != "LIVE" && r.DataMode != "REPLAY") {
		return ErrInvalidResult
	}
	switch r.Status {
	case StatusFound:
		if len(r.Evidence) == 0 {
			return ErrInvalidResult
		}
	case StatusNeedsContext, StatusNotFound:
		if len(r.Evidence) != 0 || len(r.Gaps) == 0 {
			return ErrInvalidResult
		}
	case StatusFailed:
		if len(r.Evidence) != 0 || r.Error == nil || r.Error.Code == "" {
			return ErrInvalidResult
		}
	default:
		return ErrInvalidResult
	}
	if r.Status != StatusFailed && r.Error != nil {
		return ErrInvalidResult
	}
	for _, evidence := range r.Evidence {
		citation := evidence.Citation
		if evidence.ID == "" || evidence.RevisionID == "" || evidence.Content == "" ||
			citation.FragmentID != evidence.ID || citation.RevisionID != evidence.RevisionID ||
			citation.DocumentID == "" || citation.Title != evidence.Title || citation.Source != evidence.Source ||
			citation.Section != evidence.Section || citation.ContentHash != evidence.ContentHash ||
			citation.URL == "" || citation.Applicability.Status != "MATCH" {
			return ErrInvalidResult
		}
	}
	return nil
}

type Agent struct {
	store             domain.Repository
	retriever         retriever.Retriever
	selector          model.BaseChatModel
	dataMode          string
	contextLimit      int
	evidenceSelection bool
}

var _ adk.Agent = (*Agent)(nil)

func New(config Config) (*Agent, error) {
	if config.Store == nil || config.Retriever == nil {
		return nil, fmt.Errorf("%w: knowledge store and retriever are required", domain.ErrInvalid)
	}
	if config.DataMode != "LIVE" && config.DataMode != "REPLAY" {
		return nil, fmt.Errorf("%w: data mode must be LIVE or REPLAY", domain.ErrInvalid)
	}
	if config.ContextLimit < 1 || config.ContextLimit > 8 {
		return nil, fmt.Errorf("%w: context limit must be 1..8", domain.ErrInvalid)
	}
	if config.EvidenceSelection && config.Selector == nil {
		return nil, fmt.Errorf("%w: evidence selection requires a model", domain.ErrInvalid)
	}
	return &Agent{
		store: config.Store, retriever: config.Retriever, selector: config.Selector,
		dataMode: config.DataMode, contextLimit: config.ContextLimit,
		evidenceSelection: config.EvidenceSelection,
	}, nil
}

func (*Agent) Name(context.Context) string {
	return ToolName
}

func (*Agent) Description(context.Context) string {
	return "检索已发布且适用于当前设备快照的硬件运维知识，返回可核对的原文证据、引用和缺口；不执行设备操作。"
}

func (a *Agent) Tool(ctx context.Context) (tool.InvokableTool, error) {
	base := adk.NewAgentTool(ctx, a)
	invokable, ok := base.(tool.InvokableTool)
	if !ok {
		return nil, errors.New("Eino AgentTool is not invokable")
	}
	if _, err := invokable.Info(ctx); err != nil {
		return nil, err
	}
	return invokable, nil
}

func (a *Agent) Run(ctx context.Context, input *adk.AgentInput, _ ...adk.AgentRunOption) *adk.AsyncIterator[*adk.AgentEvent] {
	iterator, generator := adk.NewAsyncIteratorPair[*adk.AgentEvent]()
	go func() {
		defer generator.Close()
		query, err := queryFromInput(input)
		if err != nil {
			result := failedResult(a.dataMode, Result{}, err)
			a.sendResult(generator, result)
			return
		}
		result, retrievalErr := a.retrieve(ctx, query, deviceContext(ctx))
		if retrievalErr != nil {
			result = failedResult(a.dataMode, result, retrievalErr)
		}
		a.sendResult(generator, result)
	}()
	return iterator
}

func (*Agent) sendResult(generator *adk.AsyncGenerator[*adk.AgentEvent], result Result) {
	payload, err := json.Marshal(result)
	if err != nil {
		generator.Send(&adk.AgentEvent{AgentName: ToolName, Err: err})
		return
	}
	event := adk.EventFromMessage(schema.AssistantMessage(string(payload), nil), nil, schema.Assistant, "")
	event.AgentName = ToolName
	generator.Send(event)
}

func failedResult(dataMode string, result Result, err error) Result {
	if result.SchemaVersion == 0 {
		result.SchemaVersion = 1
		result.DataMode = dataMode
		result.ApplicabilityChecks = []domain.KnowledgeCheck{}
		result.Gaps = []string{}
	}
	result.Status = StatusFailed
	result.Evidence = []knowledge.SearchDocument{}
	switch {
	case errors.Is(err, context.Canceled):
		result.Error = &domain.Failure{Code: "CANCELED", Message: "知识检索已取消。"}
	case errors.Is(err, context.DeadlineExceeded):
		result.Error = &domain.Failure{Code: "DEADLINE_EXCEEDED", Message: "知识检索已超过时限。"}
	case errors.Is(err, chatmodel.ErrUnavailable):
		result.Error = &domain.Failure{Code: "MODEL_UNAVAILABLE", Message: "知识检索所需模型不可用。"}
	case errors.Is(err, ErrInvalidResult):
		result.Error = &domain.Failure{Code: "INVALID_MODEL_OUTPUT", Message: "知识检索模型输出未通过校验。"}
	case errors.Is(err, domain.ErrInvalid):
		result.Error = &domain.Failure{Code: "INVALID_INPUT", Message: err.Error()}
	default:
		result.Error = &domain.Failure{Code: "RETRIEVAL_FAILED", Message: "知识检索失败。"}
	}
	return result
}

func resultError(failure *domain.Failure) error {
	if failure == nil {
		return nil
	}
	switch failure.Code {
	case "CANCELED":
		return fmt.Errorf("%w: %s", context.Canceled, failure.Message)
	case "DEADLINE_EXCEEDED":
		return fmt.Errorf("%w: %s", context.DeadlineExceeded, failure.Message)
	case "MODEL_UNAVAILABLE":
		return fmt.Errorf("%w: %s", chatmodel.ErrUnavailable, failure.Message)
	case "INVALID_MODEL_OUTPUT":
		return fmt.Errorf("%w: %s", ErrInvalidResult, failure.Message)
	case "INVALID_INPUT":
		return fmt.Errorf("%w: %s", domain.ErrInvalid, failure.Message)
	default:
		return errors.New(failure.Message)
	}
}

func Invoke(
	ctx context.Context,
	knowledgeTool tool.InvokableTool,
	query string,
	device *domain.DeviceContext,
	dataMode string,
) (Result, error) {
	if knowledgeTool == nil {
		return Result{}, fmt.Errorf("%w: knowledge tool is required", domain.ErrInvalid)
	}
	if dataMode != "LIVE" && dataMode != "REPLAY" {
		return Result{}, fmt.Errorf("%w: data mode must be LIVE or REPLAY", domain.ErrInvalid)
	}
	ctx = WithDeviceContext(ctx, device)
	request, err := json.Marshal(map[string]string{"request": query})
	if err != nil {
		return Result{}, err
	}
	raw, err := knowledgeTool.InvokableRun(ctx, string(request))
	if err != nil {
		return Result{}, err
	}
	var result Result
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		return Result{}, fmt.Errorf("%w: %v", ErrInvalidResult, err)
	}
	if err := result.Validate(); err != nil {
		return Result{}, err
	}
	if result.DataMode != dataMode {
		return Result{}, fmt.Errorf("%w: knowledge tool data mode mismatch", ErrInvalidResult)
	}
	if result.Status == StatusFailed {
		return result, resultError(result.Error)
	}
	return result, nil
}

func (a *Agent) retrieve(ctx context.Context, query string, device *domain.DeviceContext) (result Result, err error) {
	ctx, span := observability.Start(ctx, ToolName, "TOOL", map[string]any{
		"query": query, "device_context": device, "data_mode": a.dataMode,
	})
	defer func() { observability.End(span, result, err) }()
	result = Result{
		SchemaVersion: 1,
		Status:        StatusNotFound,
		DataMode:      a.dataMode,
		Evidence:      []knowledge.SearchDocument{},
		Gaps:          []string{},
	}
	if device != nil {
		if device.SnapshotID == "" {
			return result, fmt.Errorf("%w: device snapshot_id is required", domain.ErrInvalid)
		}
		if device.DataMode != a.dataMode {
			return result, fmt.Errorf("%w: device data mode mismatch", domain.ErrInvalid)
		}
	}
	search := knowledge.Search
	if a.evidenceSelection {
		search = knowledge.Candidates
	}
	found, err := search(ctx, a.store, a.retriever, knowledge.SearchInput{
		Query: query,
		TopK:  a.contextLimit,
	}, device)
	if err != nil {
		return result, err
	}
	result.Evidence = found.Documents
	result.ApplicabilityChecks = found.Checks
	result.RetrievalQueries = found.RetrievalQueries
	result.ExpandedFragmentIDs = found.ExpandedFragmentIDs
	result.TraceID = found.TraceID
	if a.evidenceSelection && len(result.Evidence) > 0 {
		result.Evidence, result.EvidenceSelection, result.ModelUsage, err = selectEvidence(
			ctx, a.store, a.selector, query, device, result.Evidence,
			result.ExpandedFragmentIDs, a.contextLimit,
		)
		if err != nil {
			return result, err
		}
	}
	if len(result.Evidence) > 0 {
		result.Status = StatusFound
		return result, nil
	}
	result.Gaps = append(result.Gaps, "未召回已发布且适用的资料，请核对设备型号版本、知识发布状态及适用性检查。")
	seen := map[string]bool{}
	for _, assessment := range result.ApplicabilityChecks {
		if assessment.Status != "UNKNOWN" {
			continue
		}
		result.Status = StatusNeedsContext
		for _, check := range assessment.Checks {
			if check.Status != "UNKNOWN" {
				continue
			}
			reason := strings.TrimSpace(check.Reason)
			if reason != "" && !seen[reason] {
				result.Gaps = append(result.Gaps, reason)
				seen[reason] = true
			}
		}
	}
	return result, nil
}

func queryFromInput(input *adk.AgentInput) (string, error) {
	if input == nil {
		return "", fmt.Errorf("%w: knowledge request is required", domain.ErrInvalid)
	}
	for i := len(input.Messages) - 1; i >= 0; i-- {
		message := input.Messages[i]
		if message != nil && message.Role == schema.User {
			query := strings.TrimSpace(message.Content)
			if query == "" || len(query) > 16000 {
				return "", fmt.Errorf("%w: query required (16 KiB max)", domain.ErrInvalid)
			}
			return query, nil
		}
	}
	return "", fmt.Errorf("%w: knowledge request must contain a user question", domain.ErrInvalid)
}

type deviceContextKey struct{}

func WithDeviceContext(ctx context.Context, device *domain.DeviceContext) context.Context {
	if device == nil {
		return context.WithValue(ctx, deviceContextKey{}, deviceScope{})
	}
	snapshot := *device
	snapshot.Aliases = append([]string(nil), device.Aliases...)
	return context.WithValue(ctx, deviceContextKey{}, deviceScope{device: &snapshot})
}

func deviceContext(ctx context.Context) *domain.DeviceContext {
	scope, _ := ctx.Value(deviceContextKey{}).(deviceScope)
	return scope.device
}

type deviceScope struct {
	device *domain.DeviceContext
}
