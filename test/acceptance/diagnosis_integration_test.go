package acceptance_test

import (
	"context"
	"crypto/rand"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"hwops/internal/adapters/chatmodel"
	"hwops/internal/adapters/postgres"
	"hwops/internal/application"
	"hwops/internal/domain"
	"hwops/internal/transport/httpapi"
)

func TestPostgreSQLDiagnosticWaitResumeAndHistory(t *testing.T) {
	dsn := os.Getenv("HWOPS_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("HWOPS_TEST_DATABASE_URL is unset; PostgreSQL diagnosis integration not verified")
	}
	var resume atomic.Bool
	plan := func(in diagnosisInput) domain.PlanProposal {
		if resume.Load() {
			return diagnoseFixture(in)
		}
		p := diagnosticProposal(in)
		p.Decision, p.WaitReasons = "WAIT", []string{"等待监控恢复"}
		return p
	}
	h := newDiagnosticHarness(t, filepath.Join(tempDir(t), "adapter-fixture.json"), plan, "500", domain.RunBudget{})
	h.stop()
	h.stop = func() {}
	start := func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		store, err := postgres.Open(ctx, dsn)
		if err != nil {
			t.Fatal(err)
		}
		app, err := application.New(store, h.model, "REPLAY", application.Options{Observer: h.observer})
		if err != nil {
			store.Close()
			t.Fatal(err)
		}
		h.server = httptest.NewServer(httpapi.New(app, token))
		h.stop = func() { h.server.Close(); app.Close(); store.Close() }
	}
	start()
	deviceID := "diagnosis-" + rand.Text()
	putObservedDevice(t, h.server, deviceID)
	publishDiagnosticManual(t, h.server)
	r := awaitDiagnostic(t, h.server, submitDiagnosticForDevice(t, h.server, deviceID))
	if r.Status != "WAITING" {
		t.Fatalf("expected durable wait: %+v", r)
	}
	before := r
	h.stop()
	start()
	r = getDiagnostic(t, h.server, r.ID)
	if r.StateVersion != before.StateVersion || r.Budget != before.Budget || len(r.Plans) != 1 {
		t.Fatal("PostgreSQL restart changed committed state")
	}
	request(t, h.server.Client(), "POST", h.server.URL+"/v1/runs/"+r.ID+"/resume",
		domain.RunResumeInput{StateVersion: r.StateVersion - 1, Reason: "stale"}, http.StatusConflict)
	resume.Store(true)
	request(t, h.server.Client(), "POST", h.server.URL+"/v1/runs/"+r.ID+"/resume",
		domain.RunResumeInput{StateVersion: r.StateVersion, Reason: "监控恢复"}, http.StatusAccepted)
	r = awaitDiagnostic(t, h.server, r.ID)
	if r.Status != "COMPLETED" || r.Result.RootCauseStatus != "CONFIRMED" || r.Result.RecoveryStatus != "UNKNOWN" ||
		len(r.Plans) != 5 || len(r.Evidence) != 2 || r.Budget.ModelCalls != 5 {
		t.Fatalf("PostgreSQL diagnosis failed: %+v", r)
	}
	before = r
	h.stop()
	start()
	r = getDiagnostic(t, h.server, r.ID)
	if r.StateVersion != before.StateVersion || r.Budget != before.Budget || len(r.Events) != len(before.Events) ||
		r.Evidence[0].ID != before.Evidence[0].ID || r.Executions[0].ID != before.Executions[0].ID {
		t.Fatal("PostgreSQL lost diagnostic history")
	}
}

// Real model only. The published manual and device are synthetic. Monitoring is
// deliberately unconfigured: no fabricated device values are labeled LIVE.
func TestLiveDiagnosticPlanningAndMissingObservation(t *testing.T) {
	if os.Getenv("HWOPS_TEST_LIVE_MODEL") != "1" {
		t.Skip("LIVE diagnosis integration requires HWOPS_TEST_LIVE_MODEL=1")
	}
	endpoint, name := os.Getenv("HWOPS_MODEL_ENDPOINT"), os.Getenv("HWOPS_MODEL")
	if endpoint == "" || name == "" {
		t.Fatal("LIVE integration requires HWOPS_MODEL_ENDPOINT and HWOPS_MODEL")
	}
	cm, err := chatmodel.NewOpenAI(endpoint, name, os.Getenv("HWOPS_MODEL_API_KEY"))
	if err != nil {
		t.Fatal(err)
	}
	s, stop := startServer(t, filepath.Join(tempDir(t), "live-diagnosis.json"), cm, "LIVE")
	defer stop()
	request(t, s.Client(), "PUT", s.URL+"/v1/devices/target", domain.DeviceInput{
		Name: "合成诊断设备", Model: "fixture/Atlas", Firmware: "R2", Source: "fixture://inventory",
		MonitoringID: "fixture/target", ObservedAt: time.Now().UTC(), DataMode: "LIVE",
	}, http.StatusOK)
	publishDiagnosticManual(t, s)
	r := awaitDiagnosticWithin(t, s, submitDiagnostic(t, s), 150*time.Second)
	if r.Error != nil || r.DataMode != "LIVE" || len(r.Plans) < 3 || len(r.Knowledge) == 0 ||
		len(r.Evidence) == 0 || r.Result == nil || r.Result.RootCauseStatus == "CONFIRMED" || r.Result.RecoveryStatus != "UNKNOWN" {
		t.Fatalf("LIVE diagnostic protocol failed: status=%s error=%+v plans=%d result=%+v", r.Status, r.Error, len(r.Plans), r.Result)
	}
	for _, ev := range r.Evidence {
		if ev.Status != "UNSUPPORTED" || len(ev.Values) != 0 {
			t.Fatalf("unconfigured monitoring became a device reading: %+v", ev)
		}
	}
	if len(r.Gaps) == 0 && len(r.WaitReasons) == 0 {
		t.Fatal("LIVE model omitted the monitoring gap")
	}
	t.Logf("LIVE model=%s status=%s plan_versions=%d model_calls=%d tool_calls=%d observations=UNSUPPORTED root_cause=%s recovery=%s",
		name, r.Status, len(r.Plans), r.Budget.ModelCalls, r.Budget.ToolCalls, r.Result.RootCauseStatus, r.Result.RecoveryStatus)
}
