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
	"sync/atomic"
	"time"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/retriever"
	"go.opentelemetry.io/otel/attribute"

	"hwops/internal/adapters/chatmodel"
	knowledgeagent "hwops/internal/agents/knowledge"
	"hwops/internal/attachments"
	"hwops/internal/blobstore"
	"hwops/internal/devices"
	"hwops/internal/diagnosis"
	"hwops/internal/domain"
	"hwops/internal/einoflow"
	"hwops/internal/evidence"
	"hwops/internal/execution"
	"hwops/internal/knowledge"
	"hwops/internal/modelbudget"
	"hwops/internal/observability"
	"hwops/internal/sandbox"
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
	diagnosis    *diagnosis.Engine
	workbench    domain.WorkbenchRepository
	draftEvents  domain.DraftEventRepository
	model        model.BaseChatModel
	runningMu    sync.Mutex
	running      map[string]context.CancelFunc
	inputLimit   int
	python       *execution.Service
	attachments  *attachments.Service
}

type Options struct {
	UsersMode           bool
	TracePrivateContent bool
	Retriever           retriever.Retriever
	ContextLimit        int
	EvidenceSelection   bool
	QueryRewrite        bool
	Observer            domain.Observer
	DiagnosticBudget    domain.RunBudget
	ModelContextTokens  int
	InputTokenBudget    int
	Runner              sandbox.Runner
	AttachmentRunner    sandbox.Runner
	Files               *blobstore.Store
	ExecutionInputs     execution.InputResolver
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
	counted, err := modelbudget.Wrap(cm)
	if err != nil {
		return nil, err
	}
	cm = counted
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
		config.DiagnosticBudget = options[0].DiagnosticBudget
		config.UsersMode = options[0].UsersMode
		config.TracePrivateContent = options[0].TracePrivateContent
		config.ModelContextTokens = options[0].ModelContextTokens
		config.InputTokenBudget = options[0].InputTokenBudget
		config.Runner = options[0].Runner
		config.AttachmentRunner = options[0].AttachmentRunner
		config.Files = options[0].Files
		config.ExecutionInputs = options[0].ExecutionInputs
	}
	if config.UsersMode {
		team, ok := store.(interface{ EnableUsers() error })
		if !ok {
			return nil, errors.New("users mode requires the PostgreSQL repository")
		}
		if err := team.EnableUsers(); err != nil {
			return nil, err
		}
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
	var dx *diagnosis.Engine
	if !config.UsersMode {
		dx, err = diagnosis.New(store, cm, knowledgeTool, observations, mode, config.DiagnosticBudget)
		if err != nil {
			return nil, err
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	if config.UsersMode && !config.TracePrivateContent {
		ctx = observability.WithoutContent(ctx)
	}
	app := &App{store: store, mode: mode, ctx: ctx, cancel: cancel, wake: make(chan struct{}, 1), retrieval: config.Retriever,
		model: cm, running: map[string]context.CancelFunc{}}
	if config.UsersMode {
		app.workbench, _ = store.(domain.WorkbenchRepository)
		if app.workbench == nil {
			cancel()
			return nil, errors.New("workbench repository is required")
		}
		app.draftEvents, _ = store.(domain.DraftEventRepository)
		if app.draftEvents == nil {
			cancel()
			return nil, errors.New("workbench draft event repository is required")
		}
		if config.ModelContextTokens == 0 {
			config.ModelContextTokens = 131072
		}
		if config.InputTokenBudget == 0 {
			config.InputTokenBudget = 98304
		}
		if config.InputTokenBudget < 4096 || config.InputTokenBudget+8192 >= config.ModelContextTokens {
			cancel()
			return nil, errors.New("input budget plus 8192 output tokens must be below the configured model context capacity")
		}
		app.inputLimit = config.InputTokenBudget
		pythonRepo, ok := store.(execution.Repository)
		if !ok {
			cancel()
			return nil, errors.New("Python execution repository is required")
		}
		attachmentRepo, ok := store.(domain.AttachmentRepository)
		if !ok {
			cancel()
			return nil, errors.New("attachment repository is required")
		}
		if config.AttachmentRunner != nil {
			app.attachments, err = attachments.New(attachmentRepo, config.AttachmentRunner, config.Files)
			if err != nil {
				cancel()
				return nil, err
			}
			if config.ExecutionInputs == nil {
				config.ExecutionInputs = app.attachments.ResolveInputs
			}
		} else if pending, readErr := attachmentRepo.PendingAttachmentParses(ctx); readErr != nil || len(pending) != 0 {
			cancel()
			return nil, errors.New("pending attachment parses require the configured parser runner for reconciliation")
		}
		if config.Runner != nil {
			app.python, err = execution.New(pythonRepo, config.Runner, config.Files, config.ExecutionInputs)
			if err != nil {
				cancel()
				if app.attachments != nil {
					app.attachments.Close()
				}
				return nil, err
			}
		} else if pending, readErr := pythonRepo.PendingPythonExecutions(ctx); readErr != nil || len(pending) != 0 {
			cancel()
			if app.attachments != nil {
				app.attachments.Close()
			}
			return nil, errors.New("pending Python executions require the configured runner for reconciliation")
		}
	}
	privateTools := app.workbenchAgentTools()
	if !config.UsersMode {
		privateTools = nil
	}
	app.qa, err = einoflow.New(context.Background(), store, knowledgeTool, cm, privateTools, observations)
	if err == nil && config.UsersMode {
		err = app.interruptWorkbenchResponses(ctx)
	}
	if err != nil {
		cancel()
		if app.python != nil {
			app.python.Close()
		}
		if app.attachments != nil {
			app.attachments.Close()
		}
		return nil, err
	}
	app.observations = observations
	app.diagnosis = dx
	app.wg.Add(1)
	go app.work()
	return app, nil
}

func (a *App) Close() {
	a.cancel()
	if a.python != nil {
		a.python.Close()
	}
	if a.attachments != nil {
		a.attachments.Close()
	}
	if a.diagnosis != nil {
		a.diagnosis.Close()
	}
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
	r, err := a.store.GetRevision(ctx, id)
	if err == nil && domain.Owner(ctx) != "" && !domain.IsAdmin(ctx) && r.Status != "PUBLISHED" {
		reader, ok := a.store.(interface {
			HasCitedRevision(context.Context, string) (bool, error)
		})
		if !ok {
			return domain.Revision{}, domain.ErrNotFound
		}
		allowed, err := reader.HasCitedRevision(ctx, id)
		if err != nil {
			return domain.Revision{}, err
		}
		if !allowed {
			return domain.Revision{}, domain.ErrNotFound
		}
	}
	return r, err
}

func (a *App) Conversation(ctx context.Context, id string) (domain.Conversation, error) {
	return a.store.GetConversation(ctx, id)
}

func (a *App) CreateConversation(ctx context.Context, owner string) (domain.Conversation, error) {
	now := time.Now().UTC()
	conversation := domain.Conversation{SchemaVersion: 1, ID: rand.Text(), Owner: owner, CreatedAt: now,
		UpdatedAt: now, Title: "新会话", TitleSource: "AUTO", StateVersion: 1}
	err := a.store.CreateConversation(ctx, conversation)
	return conversation, err
}

func (a *App) Submit(ctx context.Context, conversationID string, input domain.MessageInput, keys ...string) (domain.Response, error) {
	text := input.Text
	if err := a.ctx.Err(); err != nil {
		return domain.Response{}, err
	}
	if (strings.TrimSpace(text) == "" && len(input.AttachmentIDs) == 0) || len(text) > 16000 {
		return domain.Response{}, fmt.Errorf("%w: text or an attachment is required (text is 16 KiB max)", domain.ErrInvalid)
	}
	if len(input.AttachmentIDs) > domain.AttachmentMessageCount {
		return domain.Response{}, fmt.Errorf("%w: each message accepts at most 5 attachments", domain.ErrInvalid)
	}
	seenAttachments := map[string]bool{}
	for _, id := range input.AttachmentIDs {
		if id == "" || seenAttachments[id] {
			return domain.Response{}, fmt.Errorf("%w: attachment_ids must be non-empty and unique", domain.ErrInvalid)
		}
		seenAttachments[id] = true
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
	if input.RetryOf != "" {
		original, err := a.store.GetResponse(ctx, input.RetryOf)
		if err != nil {
			return domain.Response{}, err
		}
		if original.ConversationID != conversationID {
			return domain.Response{}, domain.ErrNotFound
		}
		if original.DeviceContext != nil {
			if input.DeviceQuery != "" || (input.DeviceID != "" && input.DeviceID != original.DeviceContext.DeviceID) {
				return domain.Response{}, fmt.Errorf("%w: retry must retain the original device", domain.ErrInvalid)
			}
			// Re-resolve the device below to obtain a fresh snapshot.
			input.DeviceID = original.DeviceContext.DeviceID
		}
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
		RequestKey: key, RequestHash: hash, RetryOf: input.RetryOf,
	}
	for _, id := range input.AttachmentIDs {
		response.Attachments = append(response.Attachments, domain.AttachmentReference{ID: id})
	}
	if a.workbench != nil {
		response.Workbench = true
		response.Deadline = response.CreatedAt.Add(5 * time.Minute)
	} else if input.RetryOf != "" {
		return domain.Response{}, fmt.Errorf("%w: retry_of requires users mode", domain.ErrInvalid)
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
				if a.workbench != nil {
					switch response.Status {
					case "CANCELING":
						// Cancellation cleanup may outlive the model worker. Poll
						// only the durable cleanup state; never rerun the graph.
						a.finishCancellation(response)
						continue
					case "RUNNING":
						// If a worker cannot commit a terminal result, preserve
						// the identity for restart reconciliation.
						continue
					}
				}
				if a.workbench == nil {
					a.answer(a.ctx, response)
					continue
				}
				a.runningMu.Lock()
				_, busy := a.running[response.ID]
				if busy || len(a.running) >= 10 {
					a.runningMu.Unlock()
					continue
				}
				ctx, cancel := context.WithCancel(a.ctx)
				a.running[response.ID] = cancel
				a.wg.Add(1)
				a.runningMu.Unlock()
				go func() {
					defer a.wg.Done()
					defer cancel()
					a.answer(ctx, response)
					a.runningMu.Lock()
					delete(a.running, response.ID)
					a.runningMu.Unlock()
					a.notify()
				}()
			}
		} else if a.ctx.Err() == nil {
			log.Print("response queue could not be read")
		}
		if a.workbench != nil {
			a.runningMu.Lock()
			ids := make([]string, 0, len(a.running))
			for id := range a.running {
				ids = append(ids, id)
			}
			a.runningMu.Unlock()
			if err = a.workbench.CleanupConversations(a.ctx, ids); err != nil && a.ctx.Err() == nil {
				log.Print("conversation cleanup will retry")
			}
		}
		select {
		case <-a.ctx.Done():
			return
		case <-a.wake:
		case <-ticker.C:
		}
	}
}

func (a *App) answer(parent context.Context, response domain.Response) {
	if response.Status == "CANCELING" {
		a.finishCancellation(response)
		return
	}
	// Queued time and retries count toward the same request deadline.
	deadline := response.CreatedAt.Add(time.Minute)
	if a.workbench != nil {
		deadline = response.Deadline
	}
	ctx, cancel := context.WithDeadline(parent, deadline)
	defer cancel()
	if a.workbench != nil {
		// Use the scheduler lifetime for the authority read so a request that
		// expired while queued can still be persisted as a terminal timeout.
		c, err := a.store.GetConversation(a.ctx, response.ConversationID)
		if err != nil {
			return
		}
		ctx = domain.WithUser(ctx, domain.User{ID: c.Owner, Role: "USER", Active: true})
		ctx = modelbudget.WithContextLimit(ctx, a.inputLimit)
		var calls atomic.Int32
		ctx = modelbudget.WithReservation(ctx, func(ctx context.Context) error {
			saved, err := a.store.GetResponse(ctx, response.ID)
			if err != nil {
				return err
			}
			if domain.ResponseTerminal(saved.Status) || saved.Status == "CANCELING" {
				return context.Canceled
			}
			if calls.Add(1) > 20 {
				return modelbudget.ErrExceeded
			}
			return nil
		})
	}
	ctx = chatmodel.WithSessionID(ctx, response.ConversationID)
	if a.draftEvents != nil {
		ctx = withWorkbenchTurn(ctx, response)
		ctx = einoflow.WithDraftEmitter(ctx, &responseDraftEmitter{
			repo: a.draftEvents, responseID: response.ID,
		})
		if repo, ok := a.store.(domain.ReasoningEventRepository); ok {
			ctx = einoflow.WithReasoningEmitter(ctx, &responseReasoningEmitter{
				repo: repo, responseID: response.ID,
			})
		}
	}
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
		if a.workbench != nil {
			ctx, err = a.prepareHistory(ctx, &response)
		}
		if err == nil {
			result, err = a.qa.Answer(ctx, response)
		}
	}
	// Leave the durable task pending when shutdown interrupts execution.
	if a.ctx.Err() != nil {
		return
	}
	if a.workbench != nil {
		saved, readErr := a.store.GetResponse(a.ctx, response.ID)
		if readErr != nil || domain.ResponseTerminal(saved.Status) {
			return
		}
		if saved.Status == "CANCELING" {
			saved.ModelUsage = result.ModelUsage
			saved.KnowledgeToolCalls = result.KnowledgeToolCalls
			saved.Evidence = result.Evidence
			saved.Sources = result.Sources
			saved.Executions = result.Executions
			if saved.ModelUsage == nil {
				saved.ModelUsage = response.ModelUsage
			}
			a.finishCancellation(saved)
			return
		}
	}
	if err != nil {
		traceErr = err
		if result.ModelUsage != nil {
			response.ModelUsage = result.ModelUsage
		}
		response.EvidenceSelection = result.EvidenceSelection
		response.ApplicabilityChecks = result.ApplicabilityChecks
		response.RetrievedFragmentIDs = result.RetrievedFragmentIDs
		response.KnowledgeToolCalls = result.KnowledgeToolCalls
		response.Evidence = result.Evidence
		response.Sources = result.Sources
		response.Executions = result.Executions
		result = response
		result.Status = "FAILED"
		code, message := "PROCESSING_FAILED", "问答处理失败，请稍后重试。"
		switch {
		case errors.Is(err, context.DeadlineExceeded):
			code, message = "DEADLINE_EXCEEDED", "请求总时限已到，未生成有效答案。"
		case errors.Is(err, ErrHistory):
			code, message = "CONTEXT_LIMIT", "历史压缩失败或超出上下文预算；原始记录已保留，本轮未省略历史继续生成。"
		case errors.Is(err, chatmodel.ErrUnavailable):
			code, message = "MODEL_UNAVAILABLE", "模型未配置或调用失败，未使用模拟答案替代。"
		case errors.Is(err, modelbudget.ErrExceeded):
			code, message = "MODEL_BUDGET_EXCEEDED", "模型上下文或调用预算已用尽，未继续生成。"
		case errors.Is(err, einoflow.ErrToolBudget):
			code, message = "TOOL_BUDGET_EXCEEDED", "知识或分析工具重复调用或已达到次数上限，未发布待校验答案。"
		case errors.Is(err, einoflow.ErrInvalidAnswer), errors.Is(err, knowledgeagent.ErrInvalidResult):
			code, message = "INVALID_MODEL_OUTPUT", "模型输出或引用未通过校验，未发布该答案。"
		}
		result.Error = &domain.Failure{Code: code, Message: message}
	}
	if a.python != nil {
		ticker := time.NewTicker(250 * time.Millisecond)
		defer ticker.Stop()
		for {
			settled, checkErr := a.store.(domain.PythonRepository).ResponsePythonSettled(a.ctx, response.ID)
			if checkErr == nil && settled {
				break
			}
			select {
			case <-a.ctx.Done():
				return
			case <-ticker.C:
			}
		}
		executions, listErr := a.store.(domain.PythonRepository).ResponsePythonExecutions(
			context.WithoutCancel(ctx), response.ID)
		if listErr != nil {
			traceErr = listErr
			log.Print("response executions could not be read")
			return
		}
		result.Executions = compactExecutions(executions)
	}
	if err := a.store.SaveResponse(a.ctx, result); err != nil {
		traceErr = err
		log.Print("response result could not be persisted")
	} else if saved, err := a.store.GetResponse(a.ctx, result.ID); err == nil {
		response = saved
	}
}

func (a *App) finishCancellation(r domain.Response) {
	if repo, ok := a.store.(domain.PythonRepository); ok {
		settled, err := repo.ResponsePythonSettled(a.ctx, r.ID)
		if err != nil || !settled {
			return
		}
		executions, err := repo.ResponsePythonExecutions(a.ctx, r.ID)
		if err != nil {
			return
		}
		r.Executions = compactExecutions(executions)
	}
	r.Status = "CANCELED"
	r.Error = &domain.Failure{Code: "CANCELED", Message: "本轮已停止。"}
	if err := a.store.SaveResponse(a.ctx, r); err != nil && a.ctx.Err() == nil {
		log.Print("response cancellation could not be persisted")
	}
}

func compactExecutions(executions []domain.PythonExecution) []domain.PythonExecution {
	out := make([]domain.PythonExecution, 0, len(executions))
	for _, execution := range executions {
		compact, _, _ := compactExecution(execution)
		out = append(out, compact)
	}
	return out
}
