// Package diagnosis owns the read-only Plan–Execute–Replan lifecycle.
package diagnosis

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"

	"hwops/internal/adapters/chatmodel"
	"hwops/internal/domain"
	"hwops/internal/evidence"
	"hwops/internal/modelbudget"
	"hwops/internal/observability"
)

type Engine struct {
	store     domain.Repository
	model     model.BaseChatModel
	knowledge tool.InvokableTool
	observer  *evidence.Service
	mode      string
	budget    domain.RunBudget
	graph     compose.Runnable[*turn, *turn]
	ctx       context.Context
	cancel    context.CancelFunc
	wake      chan struct{}
	wg        sync.WaitGroup
	mu        sync.Mutex
	running   map[string]context.CancelFunc
}

type turn struct {
	run         domain.DiagnosticRun
	incident    domain.Incident
	proposal    domain.PlanProposal
	baseVersion int64
	mu          sync.Mutex // nested retrieval model reservations may run in an agent goroutine
}

func New(store domain.Repository, cm model.BaseChatModel, kt tool.InvokableTool, observer *evidence.Service, mode string, budget domain.RunBudget) (*Engine, error) {
	defaults := domain.DefaultRunBudget()
	if budget.ActiveLimitMS == 0 && budget.ToolLimit == 0 && budget.ModelLimit == 0 {
		budget = defaults
	}
	if budget.ActiveLimitMS < 1 || budget.ActiveLimitMS > defaults.ActiveLimitMS ||
		budget.ToolLimit < 1 || budget.ToolLimit > defaults.ToolLimit || budget.ModelLimit < 1 || budget.ModelLimit > defaults.ModelLimit ||
		budget.ActiveUsedMS != 0 || budget.ToolCalls != 0 || budget.ModelCalls != 0 {
		return nil, domain.ErrInvalid
	}
	ctx, cancel := context.WithCancel(context.Background())
	e := &Engine{store: store, model: cm, knowledge: kt, observer: observer, mode: mode, budget: budget,
		ctx: ctx, cancel: cancel, wake: make(chan struct{}, 1), running: map[string]context.CancelFunc{}}
	var err error
	e.graph, err = e.buildGraph(ctx)
	if err != nil {
		cancel()
		return nil, err
	}
	e.wg.Add(1)
	go e.work()
	return e, nil
}

func (e *Engine) Close() { e.cancel(); e.wg.Wait() }
func (e *Engine) notify() {
	select {
	case e.wake <- struct{}{}:
	default:
	}
}
func (e *Engine) Run(ctx context.Context, id string) (domain.DiagnosticRun, error) {
	return e.store.GetRun(ctx, id)
}

func (e *Engine) CreateIncident(ctx context.Context, in domain.IncidentInput, key string) (domain.Incident, error) {
	if !text(in.DeviceID, 200) || !text(in.ErrorCode, 200) || !text(in.Description, 16000) ||
		in.OccurredAt.IsZero() || in.OccurredAt.After(time.Now().Add(5*time.Second)) || len(key) > 128 {
		return domain.Incident{}, fmt.Errorf("%w: incident requires device_id, error_code, description and a past occurred_at", domain.ErrInvalid)
	}
	device, err := e.store.GetDevice(ctx, in.DeviceID)
	if err != nil {
		return domain.Incident{}, err
	}
	if device.DataMode != e.mode {
		return domain.Incident{}, domain.ErrInvalid
	}
	raw, _ := json.Marshal(in)
	return e.store.CreateIncident(ctx, domain.Incident{IncidentInput: in, SchemaVersion: 1, ID: rand.Text(),
		DataMode: e.mode, CreatedAt: time.Now().UTC(), RequestKey: key, RequestHash: fmt.Sprintf("%x", sha256.Sum256(raw))})
}

func (e *Engine) CreateRun(ctx context.Context, incidentID string) (domain.DiagnosticRun, error) {
	in, err := e.store.GetIncident(ctx, incidentID)
	if err != nil {
		return domain.DiagnosticRun{}, err
	}
	d, err := e.store.GetDevice(ctx, in.DeviceID)
	if err != nil {
		return domain.DiagnosticRun{}, err
	}
	if in.DataMode != e.mode || d.DataMode != e.mode {
		return domain.DiagnosticRun{}, domain.ErrInvalid
	}
	r := domain.DiagnosticRun{SchemaVersion: 1, ID: rand.Text(), IncidentID: in.ID, DataMode: e.mode,
		StateVersion: 1, Phase: "PLAN", Status: "QUEUED", Device: d, Budget: e.budget, CreatedAt: time.Now().UTC(),
		Plans: []domain.PlanRevision{}, Hypotheses: []domain.Hypothesis{}, Evidence: []domain.Evidence{},
		Knowledge: []domain.DiagnosticKnowledge{}, Executions: []domain.StepExecution{}, WaitReasons: []string{}, Gaps: []string{},
		Result: &domain.DiagnosticResult{Summary: "诊断尚未完成。", RootCauseStatus: "UNKNOWN", RecoveryStatus: "UNKNOWN"}}
	r.AddEvent("ACCEPTED", "诊断已受理。")
	r, err = e.store.CreateRun(ctx, r)
	if err == nil {
		e.notify()
	}
	return r, err
}

func (e *Engine) Resume(ctx context.Context, id string, in domain.RunResumeInput) (domain.DiagnosticRun, error) {
	r, err := e.store.GetRun(ctx, id)
	if err != nil {
		return r, err
	}
	if !text(in.Reason, 2000) {
		return r, domain.ErrInvalid
	}
	if r.StateVersion != in.StateVersion || (r.Status != "WAITING" && r.Status != "PAUSED") || exhausted(r.Budget) {
		return r, fmt.Errorf("%w: run is not resumable at this version or budget is exhausted", domain.ErrConflict)
	}
	d, err := e.store.GetDevice(ctx, r.Device.DeviceID)
	if err != nil {
		return r, err
	}
	if d.DataMode != e.mode || r.DataMode != e.mode {
		return r, domain.ErrInvalid
	}
	if d.SnapshotID != r.Device.SnapshotID {
		r.Knowledge = nil
	}
	r.Device, r.Status, r.ActiveSince, r.Error, r.Result = d, "QUEUED", nil, nil, nil
	r.WaitReasons, r.Gaps = nil, nil
	r.AddEvent("RESUMED", in.Reason)
	err = e.save(ctx, &r, d.SnapshotID)
	if err == nil {
		e.notify()
	}
	return r, err
}

func (e *Engine) Cancel(ctx context.Context, id string, in domain.RunResumeInput) (domain.DiagnosticRun, error) {
	r, err := e.store.GetRun(ctx, id)
	if err != nil {
		return r, err
	}
	if r.Status == "CANCELED" {
		return r, nil
	}
	if !text(in.Reason, 2000) {
		return r, domain.ErrInvalid
	}
	if r.Terminal() || r.StateVersion != in.StateVersion {
		return r, domain.ErrConflict
	}
	account(&r)
	r.Status, r.ActiveSince, r.WaitReasons = "CANCELED", nil, nil
	r.AddEvent("CANCELED", in.Reason)
	if err = e.save(ctx, &r, ""); err != nil {
		return r, err
	}
	e.mu.Lock()
	if cancel := e.running[id]; cancel != nil {
		cancel()
	}
	e.mu.Unlock()
	return r, nil
}

func exhausted(b domain.RunBudget) bool {
	return b.ActiveUsedMS >= b.ActiveLimitMS || b.ToolCalls >= b.ToolLimit || b.ModelCalls >= b.ModelLimit
}

func account(r *domain.DiagnosticRun) {
	if r.ActiveSince == nil {
		return
	}
	now := time.Now().UTC()
	r.Budget.ActiveUsedMS = min(r.Budget.ActiveLimitMS, r.Budget.ActiveUsedMS+max(0, now.Sub(*r.ActiveSince).Milliseconds()))
	r.ActiveSince = &now
}

func (e *Engine) save(ctx context.Context, r *domain.DiagnosticRun, snapshot string, revisions ...string) error {
	account(r)
	expected := r.StateVersion
	r.StateVersion++
	r.UpdatedAt = time.Now().UTC()
	if err := e.store.CommitRun(ctx, *r, expected, snapshot, revisions...); err != nil {
		r.StateVersion = expected
		return err
	}
	return nil
}

func (e *Engine) reserve(ctx context.Context, t *turn, kind string) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	account(&t.run)
	b := &t.run.Budget
	if b.ActiveUsedMS >= b.ActiveLimitMS {
		return ErrBudget
	}
	if kind == "MODEL" {
		if b.ModelCalls >= b.ModelLimit {
			return ErrBudget
		}
		b.ModelCalls++
	} else {
		if b.ToolCalls >= b.ToolLimit {
			return ErrBudget
		}
		b.ToolCalls++
	}
	t.run.AddEvent(kind+"_RESERVED", "调用前预留预算，失败和重试同样计费。")
	var guards []string
	if kind == "TOOL" {
		guards = guardRevisions(t.run, t.proposal)
	}
	return e.save(ctx, &t.run, t.run.Device.SnapshotID, guards...)
}

func (e *Engine) work() {
	defer e.wg.Done()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		pending, err := e.store.PendingRuns(e.ctx)
		if err == nil {
			for _, r := range pending {
				if e.ctx.Err() != nil {
					return
				}
				e.advance(r)
			}
		} else if e.ctx.Err() == nil {
			log.Print("diagnostic queue could not be read")
		}
		select {
		case <-e.ctx.Done():
			return
		case <-e.wake:
		case <-ticker.C:
		}
	}
}

func (e *Engine) advance(r domain.DiagnosticRun) {
	// Persisted RUNNING means an interrupted invocation. Charge conservatively
	// up to its remaining active budget; never reset counters after a restart.
	account(&r)
	remaining := max(int64(1), r.Budget.ActiveLimitMS-r.Budget.ActiveUsedMS)
	ctx, cancel := context.WithTimeout(e.ctx, time.Duration(remaining)*time.Millisecond)
	e.mu.Lock()
	e.running[r.ID] = cancel
	e.mu.Unlock()
	defer func() { cancel(); e.mu.Lock(); delete(e.running, r.ID); e.mu.Unlock() }()
	t := &turn{run: r}
	ctx = chatmodel.WithSessionID(ctx, r.ID)
	ctx = modelbudget.WithReservation(ctx, func(ctx context.Context) error { return e.reserve(ctx, t, "MODEL") })
	ctx, span := observability.Start(ctx, "diagnosis.run", "CHAIN", map[string]string{"run_id": r.ID, "incident_id": r.IncidentID, "data_mode": r.DataMode})
	_, err := e.graph.Invoke(ctx, t)
	observability.End(span, t.run, err)
	if err == nil {
		return
	}
	// A stale worker must not overwrite a cancel/resume or newer state.
	saved, readErr := e.store.GetRun(context.Background(), r.ID)
	if readErr != nil {
		return
	}
	if saved.StateVersion != t.run.StateVersion || saved.Terminal() {
		return
	}
	t.run = saved
	account(&t.run)
	t.run.ActiveSince = nil
	if errors.Is(err, domain.ErrConflict) && e.ctx.Err() == nil {
		t.run.Status = "QUEUED"
		t.run.AddEvent("PROPOSAL_DISCARDED", "状态、设备快照或资料发布状态发生变化，重新加载并规划。")
		e.notify()
	} else if e.ctx.Err() != nil {
		t.run.Status = "QUEUED"
		t.run.AddEvent("INTERRUPTED", "服务停止，保留计划、证据和已消费预算。")
	} else {
		code := "DIAGNOSIS_FAILED"
		message := "诊断调用失败，需要核对模型或检索服务后恢复。"
		switch {
		case errors.Is(err, ErrProposal):
			code, message = "INVALID_PLAN", err.Error()
		case errors.Is(err, ErrBudget), errors.Is(err, context.DeadlineExceeded):
			code, message = "BUDGET_EXHAUSTED", "诊断预算耗尽，保留已取得证据；尚未完成的检查需要另行安排。"
		case errors.Is(err, ErrNoProgress):
			code, message = "NO_PROGRESS", "重复检查没有取得新信息，需要新的观察或调整方向。"
		}
		t.run.Status, t.run.Error = "PAUSED", &domain.Failure{Code: code, Message: message}
		t.run.Gaps = append(t.run.Gaps, message)
		t.run.AddEvent("PAUSE", message)
	}
	if err := e.save(context.Background(), &t.run, ""); err != nil {
		log.Print("diagnostic interruption could not be persisted")
	}
}
