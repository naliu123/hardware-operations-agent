package acceptance_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"hwops/internal/adapters/chatmodel"
	"hwops/internal/adapters/filestore"
	"hwops/internal/application"
	"hwops/internal/domain"
	"hwops/internal/knowledge"
	"hwops/internal/modelbudget"
	"hwops/internal/transport/httpapi"
)

func TestDiagnosticCancelDuringPlanningDiscardsProposal(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	var once atomic.Bool
	plan := func(in diagnosisInput) domain.PlanProposal {
		if once.CompareAndSwap(false, true) {
			close(entered)
			<-release
		}
		return diagnoseFixture(in)
	}
	h := newDiagnosticHarness(t, filepath.Join(tempDir(t), "state.json"), plan, "500", domain.RunBudget{})
	putObservedDevice(t, h.server, "target")
	id := submitDiagnostic(t, h.server)
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("model did not start")
	}
	r := getDiagnostic(t, h.server, id)
	request(t, h.server.Client(), "POST", h.server.URL+"/v1/runs/"+id+"/cancel", domain.RunResumeInput{StateVersion: r.StateVersion, Reason: "取消模型规划"}, http.StatusOK)
	close(release)
	r = awaitDiagnostic(t, h.server, id)
	if r.Status != "CANCELED" || len(r.Plans) != 0 || len(r.Executions) != 0 || r.Budget.ModelCalls != 1 {
		t.Fatalf("late plan escaped cancellation: %+v", r)
	}
}

func TestDiagnosticSnapshotChangeDiscardsAndReplans(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	var once atomic.Bool
	plan := func(in diagnosisInput) domain.PlanProposal {
		p := diagnoseFixture(in)
		if len(in.Run.Knowledge) > 0 && once.CompareAndSwap(false, true) {
			close(entered)
			<-release
		}
		if len(in.Run.Knowledge) == 0 && len(in.Run.Executions) > 0 {
			s := diagnosticKnowledgeStep(in, "refreshed-manual")
			p.Steps = []domain.DiagnosticStep{s}
		}
		return p
	}
	h := newDiagnosticHarness(t, filepath.Join(tempDir(t), "state.json"), plan, "500", domain.RunBudget{})
	putObservedDevice(t, h.server, "target")
	publishDiagnosticManual(t, h.server)
	id := submitDiagnostic(t, h.server)
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("model did not reach observation proposal")
	}
	old := getDiagnostic(t, h.server, id)
	putObservedDevice(t, h.server, "target")
	close(release)
	r := awaitDiagnostic(t, h.server, id)
	if r.Status != "COMPLETED" || r.Device.SnapshotID == old.Device.SnapshotID || r.Budget.ModelCalls != 6 {
		t.Fatalf("snapshot conflict did not replan: %+v", r)
	}
	for _, ev := range r.Evidence {
		if ev.SnapshotID != r.Device.SnapshotID {
			t.Fatal("old snapshot observation dispatched")
		}
	}
	discarded := false
	for _, ev := range r.Events {
		discarded = discarded || ev.Kind == "PROPOSAL_DISCARDED"
	}
	if !discarded {
		t.Fatal("discarded proposal not recorded")
	}
}

func TestDiagnosticActiveTimeAndToolBudgets(t *testing.T) {
	for _, kind := range []string{"active", "tool"} {
		t.Run(kind, func(t *testing.T) {
			budget := domain.DefaultRunBudget()
			plan := diagnoseFixture
			if kind == "active" {
				budget.ActiveLimitMS = 80
				plan = func(in diagnosisInput) domain.PlanProposal {
					time.Sleep(150 * time.Millisecond)
					return diagnoseFixture(in)
				}
			} else {
				budget.ToolLimit = 1
			}
			h := newDiagnosticHarness(t, filepath.Join(tempDir(t), "state.json"), plan, "500", budget)
			putObservedDevice(t, h.server, "target")
			publishDiagnosticManual(t, h.server)
			r := awaitDiagnostic(t, h.server, submitDiagnostic(t, h.server))
			if r.Status != "PAUSED" || r.Error == nil || r.Error.Code != "BUDGET_EXHAUSTED" || h.monitorCalls.Load() != 0 {
				t.Fatalf("budget allowed dispatch: %+v", r)
			}
		})
	}
}

func TestDiagnosticNestedModelCallsAndRetriesConsumeSameBudget(t *testing.T) {
	var total, plans, rewrites, selections atomic.Int32
	external := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		total.Add(1)
		input := readAgentRequest(t, r)
		system := input.Messages[0].Content
		switch {
		case strings.Contains(system, "诊断协议版本 dx-01"):
			plans.Add(1)
			var in diagnosisInput
			_ = json.Unmarshal([]byte(input.Messages[len(input.Messages)-1].Content), &in)
			p := diagnosticProposal(in)
			if len(in.Run.Executions) == 0 {
				p.Steps = []domain.DiagnosticStep{diagnosticKnowledgeStep(in, "manual")}
			} else {
				p.Decision, p.Gaps = "PAUSE", []string{"REPLAY 已完成嵌套模型计数验证。"}
			}
			raw, _ := json.Marshal(p)
			modelReply(w, string(raw))
		case strings.Contains(system, "改写"):
			if rewrites.Add(1) == 1 {
				http.Error(w, "retry fixture", http.StatusServiceUnavailable)
				return
			}
			modelReply(w, "QUERY: FAN-001 风扇转速的排障手册\nQUERY: FAN-001 风扇低速与电源检查方案\nQUERY: FAN-001 故障确认依据")
		default:
			selections.Add(1)
			var in chatmodel.ContextInput
			_ = json.Unmarshal([]byte(input.Messages[1].Content), &in)
			raw, _ := json.Marshal(map[string]any{"selected_fragment_ids": []string{in.Documents[0].ID}})
			modelReply(w, string(raw))
		}
	}))
	defer external.Close()
	store, err := filestore.Open(filepath.Join(tempDir(t), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	cm, err := chatmodel.NewOpenAI(external.URL, "fixture", "")
	if err != nil {
		t.Fatal(err)
	}
	seed, err := application.New(store, cm, "REPLAY")
	if err != nil {
		t.Fatal(err)
	}
	seedServer := httptest.NewServer(httpapi.New(seed, token))
	putObservedDevice(t, seedServer, "target")
	publishDiagnosticManual(t, seedServer)
	seedServer.Close()
	seed.Close()
	counted, err := modelbudget.Wrap(cm)
	if err != nil {
		t.Fatal(err)
	}
	// Explicit external REPLAY rewrite fixture; production QueryRewrite wiring
	// uses this same counted model, without relabeling fixture output as LIVE.
	retrieval, err := knowledge.NewMultiQueryRetriever(&knowledge.LocalRetriever{Store: store}, counted, 3)
	if err != nil {
		t.Fatal(err)
	}
	app, err := application.New(store, cm, "REPLAY", application.Options{Retriever: retrieval, EvidenceSelection: true})
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close()
	s := httptest.NewServer(httpapi.New(app, token))
	defer s.Close()
	r := awaitDiagnostic(t, s, submitDiagnostic(t, s))
	if r.Status != "PAUSED" || r.Error != nil || plans.Load() != 2 || rewrites.Load() != 2 || selections.Load() != 1 ||
		r.Budget.ModelCalls != int(total.Load()) || r.Budget.ModelCalls != 5 {
		t.Fatalf("nested calls or retries escaped budget: plans=%d rewrites=%d selection=%d run=%+v", plans.Load(), rewrites.Load(), selections.Load(), r)
	}
}

func TestDiagnosticPublishedCriteriaRejectWrongUnitsAndStaleEvidence(t *testing.T) {
	for _, scenario := range []string{"no_voltage", "wrong_rule", "wrong_conclusion", "stale"} {
		t.Run(scenario, func(t *testing.T) {
			plan := func(in diagnosisInput) domain.PlanProposal {
				p := diagnoseFixture(in)
				if scenario == "stale" && len(in.Run.Knowledge) > 0 && len(in.Run.Evidence) == 0 {
					p.Steps[0].Observation.WindowStart = p.Steps[0].Observation.WindowStart.Add(-time.Hour)
					p.Steps[0].Observation.WindowEnd = p.Steps[0].Observation.WindowEnd.Add(-time.Hour)
				}
				if len(in.Run.Evidence) == 0 {
					return p
				}
				ids := []string{in.Run.Evidence[0].ID}
				p.Decision, p.Steps = "COMPLETE", nil
				h := domain.Hypothesis{ID: "fan", Description: "风扇故障", Status: "CONFIRMED", Reason: "REPLAY 尝试越过规则", EvidenceIDs: ids,
					Rule: &domain.RuleRef{RevisionID: in.Run.Knowledge[0].Citation.RevisionID, RuleID: "fan-fault"}}
				if scenario == "wrong_rule" {
					h.Rule.RuleID = "fan-recovered"
				}
				if scenario == "wrong_conclusion" {
					h.Description = "电源故障"
				}
				p.Hypotheses = []domain.Hypothesis{h}
				p.Result = &domain.DiagnosticResult{Summary: "未验证的模型判断", RootCauseStatus: "CONFIRMED", RecoveryStatus: "UNKNOWN", EvidenceIDs: ids}
				return p
			}
			h := newDiagnosticHarness(t, filepath.Join(tempDir(t), "state.json"), plan, "500", domain.RunBudget{})
			putObservedDevice(t, h.server, "target")
			publishDiagnosticManual(t, h.server)
			r := awaitDiagnostic(t, h.server, submitDiagnostic(t, h.server))
			if r.Status != "PAUSED" || r.Error == nil || r.Error.Code != "INVALID_PLAN" || r.Result.RootCauseStatus == "CONFIRMED" {
				t.Fatalf("unmet published criteria accepted: %+v", r)
			}
		})
	}
}

func TestDiagnosticActiveRestartKeepsExecutionAndSpentBudget(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	var once atomic.Bool
	plan := func(in diagnosisInput) domain.PlanProposal {
		if len(in.Run.Knowledge) > 0 && once.CompareAndSwap(false, true) {
			close(entered)
			<-release
		}
		return diagnoseFixture(in)
	}
	path := filepath.Join(tempDir(t), "state.json")
	h := newDiagnosticHarness(t, path, plan, "500", domain.RunBudget{})
	putObservedDevice(t, h.server, "target")
	publishDiagnosticManual(t, h.server)
	id := submitDiagnostic(t, h.server)
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("replan did not start")
	}
	h.stop()
	close(release)
	h.start(t, path, domain.RunBudget{})
	r := awaitDiagnostic(t, h.server, id)
	if r.Status != "COMPLETED" || r.Budget.ModelCalls != 5 || r.Budget.ToolCalls != 3 || len(r.Executions) != 3 || h.monitorCalls.Load() != 2 {
		t.Fatalf("active restart repeated execution or reset accounting: %+v", r)
	}
}

func TestDiagnosticCancelInFlightReadRetainsOriginalExecution(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	hook := func(q domain.ObservationQuery) domain.ObservationData {
		close(entered)
		<-release
		d := monitorData(q)
		d.Values = d.Values[:1]
		return d
	}
	h := newDiagnosticHarness(t, filepath.Join(tempDir(t), "state.json"), diagnoseFixture, "500", domain.RunBudget{}, hook)
	putObservedDevice(t, h.server, "target")
	publishDiagnosticManual(t, h.server)
	id := submitDiagnostic(t, h.server)
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("monitor did not start")
	}
	r := getDiagnostic(t, h.server, id)
	exID := r.Executions[1].ID
	request(t, h.server.Client(), "POST", h.server.URL+"/v1/runs/"+id+"/cancel", domain.RunResumeInput{StateVersion: r.StateVersion, Reason: "停止只读查询"}, http.StatusOK)
	close(release)
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		r = getDiagnostic(t, h.server, id)
		if r.Executions[1].Status != "RUNNING" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if r.Status != "CANCELED" || len(r.Executions) != 2 || r.Executions[1].ID != exID ||
		r.Executions[1].Status == "RUNNING" || h.monitorCalls.Load() != 1 {
		t.Fatalf("canceled read lost tracking: %+v", r)
	}
}

func TestDiagnosticWaitingReleasesWorkerAndActiveRunIsReused(t *testing.T) {
	plan := func(in diagnosisInput) domain.PlanProposal {
		p := diagnosticProposal(in)
		p.Decision, p.WaitReasons = "WAIT", []string{"等待外部条件"}
		return p
	}
	h := newDiagnosticHarness(t, filepath.Join(tempDir(t), "state.json"), plan, "500", domain.RunBudget{})
	putObservedDevice(t, h.server, "target")
	first := awaitDiagnostic(t, h.server, submitDiagnostic(t, h.server))
	out := request(t, h.server.Client(), "POST", h.server.URL+"/v1/incidents/"+first.IncidentID+"/runs", map[string]any{}, http.StatusAccepted)
	if out["id"] != first.ID {
		t.Fatal("waiting incident received another active run")
	}
	second := awaitDiagnostic(t, h.server, submitDiagnostic(t, h.server))
	if first.Status != "WAITING" || second.Status != "WAITING" || h.modelCalls.Load() != 2 {
		t.Fatal("waiting run occupied the worker")
	}
}
