package acceptance_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"hwops/internal/adapters/chatmodel"
	"hwops/internal/adapters/filestore"
	"hwops/internal/adapters/monitor"
	"hwops/internal/application"
	"hwops/internal/domain"
	"hwops/internal/transport/httpapi"
)

type diagnosisInput struct {
	Run      domain.DiagnosticRun `json:"diagnostic_run"`
	Incident domain.Incident      `json:"incident"`
	Now      time.Time            `json:"now"`
}

func diagnosticProposal(in diagnosisInput) domain.PlanProposal {
	return domain.PlanProposal{BaseStateVersion: in.Run.StateVersion, BasePlanVersion: in.Run.PlanVersion,
		Decision: "CONTINUE", Reason: "根据 REPLAY 实际观测继续诊断。"}
}

func diagnosticKnowledgeStep(in diagnosisInput, id string) domain.DiagnosticStep {
	return domain.DiagnosticStep{ID: id, Kind: "KNOWLEDGE", TargetRef: in.Run.Device.SnapshotID,
		Query: "FAN-001 风扇转速故障排障", Purpose: "查阅适用方案", ExpectedObservation: "错误码的排障步骤与确认规则"}
}

func diagnosticObservationStep(in diagnosisInput, id, field string) domain.DiagnosticStep {
	q := observationRequest()
	q.Fields = []string{field}
	return domain.DiagnosticStep{ID: id, Kind: "OBSERVE", TargetRef: in.Run.Device.SnapshotID,
		Observation: &q, KnowledgeRefs: []string{in.Run.Knowledge[0].Citation.FragmentID},
		Purpose: "测量 " + field, ExpectedObservation: "实际监控值与单位"}
}

func diagnoseFixture(in diagnosisInput) domain.PlanProposal {
	p := diagnosticProposal(in)
	if len(in.Run.Knowledge) == 0 {
		p.Steps = []domain.DiagnosticStep{diagnosticKnowledgeStep(in, "manual")}
		return p
	}
	if len(in.Run.Evidence) == 0 {
		p.Hypotheses = []domain.Hypothesis{{ID: "fan", Description: "风扇故障", Status: "CANDIDATE", Reason: "依据适用手册待验证。"}}
		p.Steps = []domain.DiagnosticStep{diagnosticObservationStep(in, "fan", "rpm")}
		return p
	}
	first := in.Run.Evidence[0]
	if first.Status != "OK" {
		p.Decision, p.Reason, p.Gaps = "PAUSE", "查询失败不能判断设备故障。", []string{"需要恢复监控查询。"}
		return p
	}
	low := first.Values[0].Value == "500"
	if len(in.Run.Evidence) == 1 {
		field := "temperature"
		if low {
			field = "voltage"
		}
		s := diagnosticObservationStep(in, field, field)
		s.DependsOn = []string{"fan"}
		p.Steps = []domain.DiagnosticStep{s}
		return p
	}
	p.Decision, p.Reason = "COMPLETE", "已取得当前只读排查结果。"
	ids := []string{first.ID, in.Run.Evidence[1].ID}
	p.Result = &domain.DiagnosticResult{Summary: "REPLAY 验证结果，根因和恢复分别记录。", RootCauseStatus: "UNKNOWN", RecoveryStatus: "UNKNOWN", EvidenceIDs: ids}
	if low {
		p.Hypotheses = []domain.Hypothesis{{ID: "fan", Description: "风扇故障", Status: "CONFIRMED",
			Reason: "低转速且供电正常，满足已发布判据。", EvidenceIDs: ids,
			Rule: &domain.RuleRef{RevisionID: in.Run.Knowledge[0].Citation.RevisionID, RuleID: "fan-fault"}}}
		p.Result.RootCauseStatus = "CONFIRMED"
	} else {
		p.Result.RecoveryStatus = "RECOVERED"
		p.Result.RecoveryRule = &domain.RuleRef{RevisionID: in.Run.Knowledge[0].Citation.RevisionID, RuleID: "fan-recovered"}
	}
	return p
}

type diagnosticHarness struct {
	server       *httptest.Server
	stop         func()
	modelCalls   atomic.Int32
	monitorCalls atomic.Int32
	model        *chatmodel.OpenAI
	observer     *monitor.HTTP
}

// Uses a real file repository and public HTTP for every business operation;
// only external model and monitor services are REPLAY fixtures.
func newDiagnosticHarness(t *testing.T, path string, plan func(diagnosisInput) domain.PlanProposal, rpm string, budget domain.RunBudget, hooks ...func(domain.ObservationQuery) domain.ObservationData) *diagnosticHarness {
	t.Helper()
	h := &diagnosticHarness{}
	external := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.modelCalls.Add(1)
		input := readAgentRequest(t, r)
		var in diagnosisInput
		if err := json.Unmarshal([]byte(input.Messages[len(input.Messages)-1].Content), &in); err != nil {
			t.Error(err)
			return
		}
		out := plan(in)
		raw, _ := json.Marshal(out)
		modelReply(w, string(raw))
	}))
	t.Cleanup(external.Close)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.monitorCalls.Add(1)
		if rpm == "FAILED" {
			http.Error(w, "fixture outage", http.StatusBadGateway)
			return
		}
		var q domain.ObservationQuery
		if err := json.NewDecoder(r.Body).Decode(&q); err != nil {
			t.Error(err)
			return
		}
		if len(hooks) > 0 {
			_ = json.NewEncoder(w).Encode(hooks[0](q))
			return
		}
		d := monitorData(q)
		d.Values = nil
		for _, field := range q.Fields {
			value, unit := map[string]string{"rpm": rpm, "voltage": "12", "temperature": "40"}[field], map[string]string{"rpm": "rpm", "voltage": "V", "temperature": "C"}[field]
			d.Values = append(d.Values, domain.ObservationValue{Field: field, Value: value, Unit: unit, ObservedAt: q.WindowEnd})
		}
		_ = json.NewEncoder(w).Encode(d)
	}))
	t.Cleanup(upstream.Close)
	var err error
	h.model, err = chatmodel.NewOpenAI(external.URL, "fixture", "")
	if err != nil {
		t.Fatal(err)
	}
	h.observer, err = monitor.New(upstream.URL, "", "REPLAY", time.Second, 1)
	if err != nil {
		t.Fatal(err)
	}
	h.start(t, path, budget)
	t.Cleanup(func() { h.stop() })
	return h
}

func (h *diagnosticHarness) start(t *testing.T, path string, budget domain.RunBudget) {
	t.Helper()
	store, err := filestore.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	app, err := application.New(store, h.model, "REPLAY", application.Options{Observer: h.observer, DiagnosticBudget: budget})
	if err != nil {
		store.Close()
		t.Fatal(err)
	}
	h.server = httptest.NewServer(httpapi.New(app, token))
	h.stop = func() { h.server.Close(); app.Close(); store.Close() }
}

func publishDiagnosticManual(t *testing.T, s *httptest.Server) {
	t.Helper()
	condition := func(field, op, value, unit string) domain.MetricCondition {
		return domain.MetricCondition{Capability: "metrics", Component: "fan/1", Field: field, Operator: op, Value: value, Unit: unit, MaxAgeSeconds: 120}
	}
	in := domain.RevisionInput{Title: "FAN-001 风扇排障", Source: "fixture://manual/fan",
		Content:       "FAN-001：先查询风扇转速。转速小于1000rpm时查询电压，电压至少11V且低转速可确认风扇故障。否则查询温度。转速至少1000rpm且温度低于60C表示本告警恢复。",
		Applicability: domain.Applicability{Scope: "DEVICE", Model: "fixture/Atlas", Firmware: "R2"},
		DiagnosticRules: []domain.DiagnosticRule{
			{ID: "fan-fault", Kind: "ROOT_CAUSE", ErrorCode: "FAN-001", Conclusion: "风扇故障", StartLine: 1, EndLine: 1,
				Conditions: []domain.MetricCondition{condition("rpm", "LT", "1000", "rpm"), condition("voltage", "GE", "11", "V")}},
			{ID: "fan-recovered", Kind: "RECOVERY", ErrorCode: "FAN-001", Conclusion: "风扇告警恢复", StartLine: 1, EndLine: 1,
				Conditions: []domain.MetricCondition{condition("rpm", "GE", "1000", "rpm"), condition("temperature", "LT", "60", "C")}},
		}}
	rev := request(t, s.Client(), "POST", s.URL+"/v1/knowledge/revisions", in, http.StatusCreated)
	request(t, s.Client(), "POST", s.URL+"/v1/knowledge/revisions/"+rev["id"].(string)+"/publication", map[string]string{"decision": "PUBLISH"}, http.StatusOK)
}

func submitDiagnostic(t *testing.T, s *httptest.Server) string {
	return submitDiagnosticForDevice(t, s, "target")
}

func submitDiagnosticForDevice(t *testing.T, s *httptest.Server, deviceID string) string {
	t.Helper()
	in := request(t, s.Client(), "POST", s.URL+"/v1/incidents", domain.IncidentInput{DeviceID: deviceID, ErrorCode: "FAN-001",
		Description: "风扇低速告警", OccurredAt: time.Now().Add(-2 * time.Minute)}, http.StatusCreated)
	out := request(t, s.Client(), "POST", s.URL+"/v1/incidents/"+in["id"].(string)+"/runs", map[string]any{}, http.StatusAccepted)
	return out["id"].(string)
}

func getDiagnostic(t *testing.T, s *httptest.Server, id string) domain.DiagnosticRun {
	t.Helper()
	out := request(t, s.Client(), "GET", s.URL+"/v1/runs/"+id, nil, http.StatusOK)
	raw, _ := json.Marshal(out)
	var run domain.DiagnosticRun
	if err := json.Unmarshal(raw, &run); err != nil {
		t.Fatal(err)
	}
	return run
}

func awaitDiagnostic(t *testing.T, s *httptest.Server, id string) domain.DiagnosticRun {
	return awaitDiagnosticWithin(t, s, id, 10*time.Second)
}

func awaitDiagnosticWithin(t *testing.T, s *httptest.Server, id string, timeout time.Duration) domain.DiagnosticRun {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		r := getDiagnostic(t, s, id)
		if !r.Pending() {
			return r
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("diagnostic run did not settle")
	return domain.DiagnosticRun{}
}

func TestDiagnosticReplansFromActualObservations(t *testing.T) {
	for _, rpm := range []string{"500", "2000", "FAILED"} {
		t.Run(rpm, func(t *testing.T) {
			h := newDiagnosticHarness(t, filepath.Join(tempDir(t), "state.json"), diagnoseFixture, rpm, domain.RunBudget{})
			putObservedDevice(t, h.server, "target")
			publishDiagnosticManual(t, h.server)
			r := awaitDiagnostic(t, h.server, submitDiagnostic(t, h.server))
			if r.DataMode != "REPLAY" || len(r.Plans) < 3 || len(r.Executions) < 2 {
				t.Fatalf("missing history: %+v", r)
			}
			if rpm == "FAILED" {
				if r.Status != "PAUSED" || len(r.Hypotheses) != 0 || r.Evidence[0].Status != "FAILED" {
					t.Fatalf("failure became a diagnosis: %+v", r)
				}
				return
			}
			if r.Status != "COMPLETED" || r.Result == nil {
				t.Fatalf("not completed: %+v", r)
			}
			wantField, wantCause, wantRecovery := "voltage", "CONFIRMED", "UNKNOWN"
			if rpm == "2000" {
				wantField, wantCause, wantRecovery = "temperature", "UNKNOWN", "RECOVERED"
			}
			if r.Evidence[1].Query.Fields[0] != wantField || r.Result.RootCauseStatus != wantCause || r.Result.RecoveryStatus != wantRecovery {
				t.Fatalf("observation did not affect replan: %+v", r)
			}
			if r.Budget.ModelCalls != int(h.modelCalls.Load()) || r.Budget.ToolCalls != 3 ||
				r.Executions[2].Step.DependsOn[0] != "fan" || r.Executions[1].Step.ExpectedObservation == r.Evidence[0].Values[0].Value {
				t.Fatalf("budget/dependency/intent tracking failed: %+v", r)
			}
		})
	}
}

func TestDiagnosticRejectsInvalidPlansBeforeDispatch(t *testing.T) {
	for _, scenario := range []string{"cycle", "missing_dependency", "foreign_target", "command", "false_confirmation", "false_recovery", "unknown_citation", "bad_base", "wait_with_ready"} {
		t.Run(scenario, func(t *testing.T) {
			plan := func(in diagnosisInput) domain.PlanProposal {
				p := diagnosticProposal(in)
				s := diagnosticKnowledgeStep(in, "a")
				switch scenario {
				case "cycle":
					s.DependsOn = []string{"b"}
					b := diagnosticKnowledgeStep(in, "b")
					b.DependsOn = []string{"a"}
					p.Steps = append(p.Steps, b)
				case "missing_dependency":
					s.DependsOn = []string{"missing"}
				case "foreign_target":
					s.TargetRef = "other-snapshot"
				case "command":
					s.Kind = "CLI"
				case "unknown_citation":
					s.KnowledgeRefs = []string{"invented"}
				case "bad_base":
					p.BaseStateVersion--
				case "wait_with_ready":
					p.Decision, p.WaitReasons = "WAIT", []string{"现场等待"}
				case "false_confirmation":
					p.Hypotheses = []domain.Hypothesis{{ID: "fan", Description: "风扇故障", Status: "CONFIRMED", Reason: "模型自信"}}
				case "false_recovery":
					p.Result = &domain.DiagnosticResult{Summary: "模型宣称已恢复", RootCauseStatus: "UNKNOWN", RecoveryStatus: "RECOVERED"}
				}
				p.Steps = append(p.Steps, s)
				return p
			}
			h := newDiagnosticHarness(t, filepath.Join(tempDir(t), "state.json"), plan, "500", domain.RunBudget{})
			putObservedDevice(t, h.server, "target")
			r := awaitDiagnostic(t, h.server, submitDiagnostic(t, h.server))
			if r.Status != "PAUSED" || r.Error == nil || r.Error.Code != "INVALID_PLAN" || len(r.Plans) != 0 ||
				len(r.Executions) != 0 || h.monitorCalls.Load() != 0 {
				t.Fatalf("invalid plan dispatched: %+v", r)
			}
		})
	}
}

func TestDiagnosticWaitRestartResumeAndBudget(t *testing.T) {
	var resume atomic.Bool
	plan := func(in diagnosisInput) domain.PlanProposal {
		if !resume.Load() {
			p := diagnosticProposal(in)
			p.Decision, p.WaitReasons = "WAIT", []string{"等待操作员补充监控可用性。"}
			return p
		}
		return diagnoseFixture(in)
	}
	path := filepath.Join(tempDir(t), "state.json")
	h := newDiagnosticHarness(t, path, plan, "500", domain.RunBudget{})
	putObservedDevice(t, h.server, "target")
	publishDiagnosticManual(t, h.server)
	r := awaitDiagnostic(t, h.server, submitDiagnostic(t, h.server))
	if r.Status != "WAITING" || r.ActiveSince != nil || r.Budget.ModelCalls != 1 {
		t.Fatalf("wait did not suspend: %+v", r)
	}
	before := r.Budget.ActiveUsedMS
	h.stop()
	h.start(t, path, domain.RunBudget{})
	r = getDiagnostic(t, h.server, r.ID)
	if r.Budget.ActiveUsedMS != before || r.Status != "WAITING" {
		t.Fatal("restart reset waiting state or budget")
	}
	resume.Store(true)
	request(t, h.server.Client(), "POST", h.server.URL+"/v1/runs/"+r.ID+"/resume", domain.RunResumeInput{StateVersion: r.StateVersion - 1, Reason: "stale"}, http.StatusConflict)
	request(t, h.server.Client(), "POST", h.server.URL+"/v1/runs/"+r.ID+"/resume", domain.RunResumeInput{StateVersion: r.StateVersion, Reason: "监控恢复"}, http.StatusAccepted)
	r = awaitDiagnostic(t, h.server, r.ID)
	if r.Status != "COMPLETED" || r.Budget.ModelCalls != 5 || r.Budget.ActiveUsedMS < before {
		t.Fatalf("resume reset budget or failed: %+v", r)
	}
	kinds := []string{}
	for _, ev := range r.Events {
		kinds = append(kinds, ev.Kind)
	}
	for _, kind := range []string{"PLAN", "REPLAN", "WAIT", "RESUMED", "COMPLETE"} {
		if !slices.Contains(kinds, kind) {
			t.Fatalf("missing graph branch %s", kind)
		}
	}
}

func TestDiagnosticModelBudgetSurvivesPause(t *testing.T) {
	h := newDiagnosticHarness(t, filepath.Join(tempDir(t), "state.json"), diagnoseFixture, "500", domain.RunBudget{ActiveLimitMS: 300000, ToolLimit: 30, ModelLimit: 2})
	putObservedDevice(t, h.server, "target")
	publishDiagnosticManual(t, h.server)
	r := awaitDiagnostic(t, h.server, submitDiagnostic(t, h.server))
	if r.Status != "PAUSED" || r.Error == nil || r.Error.Code != "BUDGET_EXHAUSTED" || r.Budget.ModelCalls != 2 ||
		h.modelCalls.Load() != 2 || len(r.Gaps) == 0 {
		t.Fatalf("budget not enforced: %+v", r)
	}
	request(t, h.server.Client(), "POST", h.server.URL+"/v1/runs/"+r.ID+"/resume", domain.RunResumeInput{StateVersion: r.StateVersion, Reason: "继续"}, http.StatusConflict)
}

func TestDiagnosticReusesEvidenceAndChecksPreconditions(t *testing.T) {
	plan := func(in diagnosisInput) domain.PlanProposal {
		if len(in.Run.Evidence) == 0 {
			return diagnoseFixture(in)
		}
		p := diagnosticProposal(in)
		if len(in.Run.Executions) == 2 {
			s := in.Run.Executions[1].Step
			s.ID = "fan-again"
			s.Preconditions = []domain.EvidenceCondition{{EvidenceID: in.Run.Evidence[0].ID, Condition: domain.MetricCondition{
				Capability: "metrics", Component: "fan/1", Field: "rpm", Operator: "LT", Value: "1000", Unit: "rpm", MaxAgeSeconds: 120}}}
			p.Steps = []domain.DiagnosticStep{s}
		} else {
			s := diagnosticObservationStep(in, "blocked", "voltage")
			s.Preconditions = []domain.EvidenceCondition{{EvidenceID: in.Run.Evidence[0].ID, Condition: domain.MetricCondition{
				Capability: "metrics", Component: "fan/1", Field: "rpm", Operator: "GT", Value: "1000", Unit: "rpm", MaxAgeSeconds: 120}}}
			p.Steps, p.Decision, p.WaitReasons = []domain.DiagnosticStep{s}, "WAIT", []string{"等待转速满足分支条件。"}
		}
		return p
	}
	h := newDiagnosticHarness(t, filepath.Join(tempDir(t), "state.json"), plan, "500", domain.RunBudget{})
	putObservedDevice(t, h.server, "target")
	publishDiagnosticManual(t, h.server)
	r := awaitDiagnostic(t, h.server, submitDiagnostic(t, h.server))
	if r.Status != "WAITING" || len(r.Executions) != 3 || r.Executions[2].Status != "REUSED" ||
		r.Executions[2].ReusedFrom != r.Executions[1].ID || len(r.Evidence) != 1 || h.monitorCalls.Load() != 1 || r.Budget.ToolCalls != 2 {
		t.Fatalf("reuse or precondition failed: %+v", r)
	}
	request(t, h.server.Client(), "POST", h.server.URL+"/v1/runs/"+r.ID+"/cancel", domain.RunResumeInput{StateVersion: r.StateVersion, Reason: "停止排查"}, http.StatusOK)
	r = getDiagnostic(t, h.server, r.ID)
	if r.Status != "CANCELED" || !strings.Contains(r.Events[len(r.Events)-1].Reason, "停止") {
		t.Fatal("cancel was not durable")
	}
}
