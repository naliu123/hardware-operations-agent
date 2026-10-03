package application

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/retriever"
	"go.opentelemetry.io/otel/attribute"

	"hwops/internal/adapters/chatmodel"
	knowledgeagent "hwops/internal/agents/knowledge"
	"hwops/internal/devices"
	"hwops/internal/domain"
	"hwops/internal/einoflow"
	"hwops/internal/evidence"
	"hwops/internal/knowledge"
	"hwops/internal/observability"
)

type App struct {
	store        domain.Repository
	qa           *einoflow.QA
	mode         string
	ctx          context.Context
	cancel       context.CancelFunc
	wake         chan struct{}
	wg           sync.WaitGroup
	retrieval    retriever.Retriever
	observations *evidence.Service
}

type Options struct {
	Retriever         retriever.Retriever
	ContextLimit      int
	EvidenceSelection bool
	QueryRewrite      bool
	Observer          domain.Observer
}

func New(store domain.Repository, cm model.BaseChatModel, mode string, options ...Options) (*App, error) {
	if mode != "LIVE" && mode != "REPLAY" {
		return nil, errors.New("data mode must be LIVE or REPLAY")
	}
	if _, replay := cm.(*chatmodel.Replay); replay && mode != "REPLAY" {
		return nil, errors.New("replay model requires REPLAY mode")
	}
	if cm == nil {
		cm = &chatmodel.Unconfigured{}
	}
	config := Options{Retriever: &knowledge.LocalRetriever{Store: store}, ContextLimit: 8}
	if len(options) > 0 {
		if options[0].Retriever != nil {
			config.Retriever = options[0].Retriever
		}
		if options[0].ContextLimit > 0 && options[0].ContextLimit <= 8 {
			config.ContextLimit = options[0].ContextLimit
		}
		config.EvidenceSelection = options[0].EvidenceSelection
		config.QueryRewrite = options[0].QueryRewrite
		config.Observer = options[0].Observer
	}
	if config.QueryRewrite {
		if mode != "LIVE" {
			return nil, errors.New("query rewriting requires LIVE model mode")
		}
		multiQuery, err := knowledge.NewMultiQueryRetriever(config.Retriever, cm, 3)
		if err != nil {
			return nil, err
		}
		config.Retriever = multiQuery
	}
	retrievalAgent, err := knowledgeagent.New(knowledgeagent.Config{
		Store: store, Retriever: config.Retriever, Selector: cm, DataMode: mode,
		ContextLimit: config.ContextLimit, EvidenceSelection: config.EvidenceSelection,
	})
	if err != nil {
		return nil, err
	}
	knowledgeTool, err := retrievalAgent.Tool(context.Background())
	if err != nil {
		return nil, err
	}
	observations := &evidence.Service{Observer: config.Observer, Mode: mode}
	qa, err := einoflow.New(context.Background(), store, knowledgeTool, cm, observations)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	app := &App{store: store, qa: qa, mode: mode, ctx: ctx, cancel: cancel, wake: make(chan struct{}, 1), retrieval: config.Retriever}
	app.observations = observations
	app.wg.Add(1)
	go app.work()
	return app, nil
}

func (a *App) Close() {
	a.cancel()
	a.wg.Wait()
}

func (a *App) PutDevice(ctx context.Context, id string, input domain.DeviceInput) (domain.DeviceContext, error) {
	if input.DataMode != a.mode {
		return domain.DeviceContext{}, fmt.Errorf("%w: device data_mode differs from application mode", domain.ErrInvalid)
	}
	device, err := devices.NewSnapshot(id, input)
	if err == nil {
		err = a.store.PutDevice(ctx, device)
	}
	return device, err
}

func (a *App) ResolveDevice(ctx context.Context, query string) (domain.DeviceResolution, error) {
	return devices.Resolve(ctx, a.store, query)
}

func (a *App) Observe(ctx context.Context, deviceID string, query domain.ObservationRequest) (domain.Evidence, error) {
	device, err := a.store.GetDevice(ctx, deviceID)
	if err != nil {
		return domain.Evidence{}, err
	}
	return a.observations.Observe(ctx, device, query)
}

func (a *App) CreateVersionPolicy(ctx context.Context, input domain.VersionPolicyInput) (domain.VersionPolicy, error) {
	policy, err := knowledge.NewVersionPolicy(input)
	if err == nil {
		err = a.store.CreateVersionPolicy(ctx, policy)
	}
	return policy, err
}

func (a *App) VersionPolicy(ctx context.Context, id string) (domain.VersionPolicy, error) {
	return a.store.GetVersionPolicy(ctx, id)
}

func (a *App) CreateRevision(ctx context.Context, input domain.RevisionInput) (domain.Revision, error) {
	revision, err := knowledge.NewRevision(input)
	if err == nil {
		err = a.store.CreateRevision(ctx, revision)
	}
	return revision, err
}

func (a *App) Publish(ctx context.Context, id, decision string) (domain.Revision, error) {
	status := map[string]string{"PUBLISH": "PUBLISHED", "WITHDRAW": "WITHDRAWN"}[decision]
	if status == "" {
		return domain.Revision{}, fmt.Errorf("%w: unsupported publication decision", domain.ErrInvalid)
	}
	if index, ok := a.retrieval.(interface {
		IndexRevision(context.Context, domain.Revision) error
	}); ok && status == "PUBLISHED" {
		revision, err := a.store.GetRevision(ctx, id)
		if err != nil {
			return domain.Revision{}, err
		}
		if err := index.IndexRevision(ctx, revision); err != nil {
			return domain.Revision{}, err
		}
	}
	return a.store.SetPublication(ctx, id, status)
}

func (a *App) Search(ctx context.Context, input knowledge.SearchInput) (knowledge.SearchResult, error) {
	fingerprint := sha256.Sum256([]byte(input.DeviceID + "\x00" + input.Query))
	ctx = chatmodel.WithSessionID(ctx, fmt.Sprintf("knowledge-search-%x", fingerprint[:12]))
	var device *domain.DeviceContext
	if input.DeviceID != "" {
		d, err := a.store.GetDevice(ctx, input.DeviceID)
		if err != nil {
			return knowledge.SearchResult{}, err
		}
		if d.DataMode != a.mode {
			return knowledge.SearchResult{}, fmt.Errorf("%w: device data mode mismatch", domain.ErrInvalid)
		}
		device = &d
	}
	return knowledge.Search(ctx, a.store, a.retrieval, input, device)
}

func (a *App) Revision(ctx context.Context, id string) (domain.Revision, error) {
	return a.store.GetRevision(ctx, id)
}

func (a *App) CreateConversation(ctx context.Context, owner string) (domain.Conversation, error) {
	conversation := domain.Conversation{SchemaVersion: 1, ID: rand.Text(), Owner: owner, CreatedAt: time.Now().UTC()}
	err := a.store.CreateConversation(ctx, conversation)
	return conversation, err
}

func (a *App) Submit(ctx context.Context, conversationID string, input domain.MessageInput, keys ...string) (domain.Response, error) {
	text := input.Text
	if err := a.ctx.Err(); err != nil {
		return domain.Response{}, err
	}
	if strings.TrimSpace(text) == "" || len(text) > 16000 {
		return domain.Response{}, fmt.Errorf("%w: text is required (16 KiB max)", domain.ErrInvalid)
	}
	key := ""
	if len(keys) > 0 {
		key = keys[0]
	}
	if len(key) > 128 || strings.TrimSpace(key) != key {
		return domain.Response{}, fmt.Errorf("%w: Idempotency-Key must be at most 128 bytes without surrounding whitespace", domain.ErrInvalid)
	}
	raw, _ := json.Marshal(input)
	hash := fmt.Sprintf("%x", sha256.Sum256(raw))
	if key != "" {
		existing, err := a.store.FindResponseRequest(ctx, conversationID, key, hash)
		if !errors.Is(err, domain.ErrNotFound) {
			return existing, err
		}
	}
	conversation, err := a.store.GetConversation(ctx, conversationID)
	if err != nil {
		return domain.Response{}, err
	}
	expectedVersion := int64(-1)
	if input.ContextRevision != "" {
		if input.ContextRevision != conversation.ContextRevision {
			return domain.Response{}, fmt.Errorf("%w: conversation context changed", domain.ErrConflict)
		}
		expectedVersion = conversation.ContextVersion
	}
	response := domain.Response{
		SchemaVersion: 1, ID: rand.Text(), ConversationID: conversationID,
		Question: text, Status: "QUEUED", DataMode: a.mode, CreatedAt: time.Now().UTC(),
		Claims: []domain.Claim{}, Citations: []domain.Citation{}, Gaps: []string{},
		RequestKey: key, RequestHash: hash,
	}
	if input.DeviceID != "" && input.DeviceQuery != "" {
		return domain.Response{}, fmt.Errorf("%w: choose device_id or device_query", domain.ErrInvalid)
	}
	var selected *domain.DeviceContext
	if input.DeviceQuery != "" {
		resolution, err := a.ResolveDevice(ctx, input.DeviceQuery)
		if err != nil {
			return domain.Response{}, err
		}
		response.DeviceResolution = &resolution
		if resolution.Status == "RESOLVED" {
			selected = &resolution.Candidates[0]
		} else {
			response.Status = "NEEDS_CLARIFICATION"
			response.Gaps = []string{"设备查询没有唯一结果，请根据候选设备选择 device_id，或核对设备标识。"}
		}
	} else if input.DeviceID != "" {
		device, err := a.store.GetDevice(ctx, input.DeviceID)
		if errors.Is(err, domain.ErrNotFound) {
			response.Status = "NEEDS_CLARIFICATION"
			response.Gaps = []string{"未找到设备，请核对 device_id。"}
			response.DeviceResolution = &domain.DeviceResolution{Status: "NO_RECORD", Candidates: []domain.DeviceContext{}}
		} else if err != nil {
			return domain.Response{}, err
		} else {
			selected = &device
		}
	}
	mentioned, err := devices.ResolveText(ctx, a.store, text)
	if err != nil {
		return domain.Response{}, err
	}
	if input.DeviceID == "" && input.DeviceQuery == "" && mentioned.Status != "NO_RECORD" {
		response.DeviceResolution = &mentioned
		if mentioned.Status == "RESOLVED" {
			selected = &mentioned.Candidates[0]
		} else {
			response.Status = "NEEDS_CLARIFICATION"
			response.Gaps = []string{"问题中出现多个设备候选，请明确目标设备。"}
		}
	} else if selected != nil {
		for _, candidate := range mentioned.Candidates {
			if candidate.DeviceID != selected.DeviceID {
				mentioned.Status = "CONFLICT"
				found := false
				for _, d := range mentioned.Candidates {
					found = found || d.DeviceID == selected.DeviceID
				}
				if !found {
					mentioned.Candidates = append(mentioned.Candidates, *selected)
				}
				response.DeviceResolution = &mentioned
				selected = nil
				response.Status = "NEEDS_CLARIFICATION"
				response.Gaps = []string{"设备选择与问题中的设备标识冲突，请确认目标后重新提问。"}
				break
			}
		}
	}
	if selected == nil && response.Status == "QUEUED" && input.DeviceID == "" && input.DeviceQuery == "" &&
		mentioned.Status == "NO_RECORD" && devicePronoun.MatchString(text) {
		expectedVersion = conversation.ContextVersion
		if conversation.DeviceID == "" {
			response.Status = "NEEDS_CLARIFICATION"
			response.Gaps = []string{"当前没有唯一确认的设备，无法解析设备指代，请选择 device_id。"}
		} else {
			device, err := a.store.GetDevice(ctx, conversation.DeviceID)
			if err != nil {
				return domain.Response{}, err
			}
			selected = &device
		}
	}
	if selected != nil {
		if selected.DataMode != a.mode {
			return domain.Response{}, fmt.Errorf("%w: device data_mode differs from application mode", domain.ErrInvalid)
		}
		response.DeviceContext = selected
		response.ContextRevision = selected.SnapshotID
	}
	response, err = a.store.CreateResponse(ctx, response, expectedVersion)
	if err != nil {
		return domain.Response{}, err
	}
	select {
	case a.wake <- struct{}{}:
	default:
	}
	return response, nil
}

var devicePronoun = regexp.MustCompile(`(?i)(它|这台|该设备|这条告警|\bit\b|\bthat device\b|\bthis device\b)`)

func (a *App) ResponseEvents(ctx context.Context, id string, after int64) ([]domain.ResponseEvent, error) {
	return a.store.ResponseEvents(ctx, id, after)
}

func (a *App) Response(ctx context.Context, id string) (domain.Response, error) {
	return a.store.GetResponse(ctx, id)
}

func (a *App) work() {
	defer a.wg.Done()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		pending, err := a.store.PendingResponses(a.ctx)
		if err == nil {
			for _, response := range pending {
				if a.ctx.Err() != nil {
					return
				}
				a.answer(response)
			}
		} else if a.ctx.Err() == nil {
			log.Print("response queue could not be read")
		}
		select {
		case <-a.ctx.Done():
			return
		case <-a.wake:
		case <-ticker.C:
		}
	}
}

func (a *App) answer(response domain.Response) {
	// Queued time and retries count toward the same request deadline.
	deadline := response.CreatedAt.Add(time.Minute)
	ctx, cancel := context.WithDeadline(a.ctx, deadline)
	defer cancel()
	ctx = chatmodel.WithSessionID(ctx, response.ConversationID)
	ctx, span := observability.Start(ctx, "qa.answer", "CHAIN", response.Question)
	response.TraceID = observability.TraceID(ctx)
	span.SetAttributes(attribute.String("session.id", response.ConversationID),
		attribute.String("metadata", observability.JSON(map[string]any{
			"response_id": response.ID, "data_mode": response.DataMode, "context_revision": response.ContextRevision,
		})))
	var traceErr error
	defer func() { observability.End(span, response, traceErr) }()
	response.Status = "RUNNING"
	if err := a.store.SaveResponse(a.ctx, response); err != nil {
		traceErr = err
		log.Print("response could not be marked running")
		return
	}
	var result domain.Response
	var err error
	if response.DataMode != a.mode {
		err = errors.New("model mode changed during restart")
	} else {
		result, err = a.qa.Answer(ctx, response)
	}
	// Leave the durable task pending when shutdown interrupts execution.
	if a.ctx.Err() != nil {
		return
	}
	if err != nil {
		traceErr = err
		response.ModelUsage = result.ModelUsage
		response.EvidenceSelection = result.EvidenceSelection
		response.ApplicabilityChecks = result.ApplicabilityChecks
		response.RetrievedFragmentIDs = result.RetrievedFragmentIDs
		response.KnowledgeToolCalls = result.KnowledgeToolCalls
		response.Evidence = result.Evidence
		result = response
		result.Status = "FAILED"
		code, message := "PROCESSING_FAILED", "问答处理失败，请稍后重试。"
		switch {
		case errors.Is(err, context.DeadlineExceeded):
			code, message = "DEADLINE_EXCEEDED", "请求总时限已到，未生成有效答案。"
		case errors.Is(err, chatmodel.ErrUnavailable):
			code, message = "MODEL_UNAVAILABLE", "模型未配置或调用失败，未使用模拟答案替代。"
		case errors.Is(err, einoflow.ErrToolBudget):
			code, message = "TOOL_BUDGET_EXCEEDED", "知识查询重复或已达到调用次数上限，未发布待校验答案。"
		case errors.Is(err, einoflow.ErrInvalidAnswer), errors.Is(err, knowledgeagent.ErrInvalidResult):
			code, message = "INVALID_MODEL_OUTPUT", "模型输出或引用未通过校验，未发布该答案。"
		}
		result.Error = &domain.Failure{Code: code, Message: message}
	}
	if err := a.store.SaveResponse(a.ctx, result); err != nil {
		traceErr = err
		log.Print("response result could not be persisted")
	} else if saved, err := a.store.GetResponse(a.ctx, result.ID); err == nil {
		response = saved
	}
}
