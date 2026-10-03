package acceptance_test

import (
	"path/filepath"
	"testing"
	"time"

	"hwops/internal/domain"
)

func TestDiagnosticCriteriaRejectUnitsAndUncitedCounterevidence(t *testing.T) {
	for _, kind := range []string{"ROOT_CAUSE", "RECOVERY"} {
		for _, scenario := range []string{"wrong_unit", "newer_conflict", "same_time_conflict"} {
			t.Run(kind+"/"+scenario, func(t *testing.T) {
				anchor := time.Now().UTC().Add(-10 * time.Second)
				plan := func(in diagnosisInput) domain.PlanProposal {
					if len(in.Run.Knowledge) == 0 {
						return diagnoseFixture(in)
					}
					p := diagnosticProposal(in)
					n := len(in.Run.Evidence)
					if n == 0 || (n == 1 && scenario != "wrong_unit") {
						s := diagnosticObservationStep(in, "initial", "rpm")
						s.Observation.WindowStart, s.Observation.WindowEnd = anchor.Add(-time.Minute), anchor
						if n == 0 {
							s.Observation.Fields = []string{"rpm", "voltage", "temperature"}
						} else {
							s.ID = "refresh-rpm"
							if scenario == "newer_conflict" {
								s.Observation.WindowEnd = anchor.Add(time.Second)
							}
						}
						p.Steps = []domain.DiagnosticStep{s}
						return p
					}
					// Deliberately omit the contradicting second observation.
					ids := []string{in.Run.Evidence[0].ID}
					ref := &domain.RuleRef{RevisionID: in.Run.Knowledge[0].Citation.RevisionID, RuleID: "fan-fault"}
					p.Decision = "COMPLETE"
					p.Result = &domain.DiagnosticResult{Summary: "REPLAY 尝试只选支持证据", RootCauseStatus: "UNKNOWN", RecoveryStatus: "UNKNOWN", EvidenceIDs: ids}
					if kind == "ROOT_CAUSE" {
						p.Hypotheses = []domain.Hypothesis{{ID: "fan", Description: "风扇故障", Status: "CONFIRMED",
							Reason: "REPLAY 选择性引用", Rule: ref, EvidenceIDs: ids}}
						p.Result.RootCauseStatus = "CONFIRMED"
					} else {
						ref.RuleID = "fan-recovered"
						p.Result.RecoveryStatus, p.Result.RecoveryRule = "RECOVERED", ref
					}
					return p
				}
				hook := func(q domain.ObservationQuery) domain.ObservationData {
					d := monitorData(q)
					d.Values = nil
					rpm := "500"
					if kind == "RECOVERY" {
						rpm = "2000"
					}
					if len(q.Fields) == 1 {
						rpm = map[string]string{"500": "2000", "2000": "500"}[rpm]
					}
					for _, field := range q.Fields {
						unit := map[string]string{"rpm": "rpm", "voltage": "V", "temperature": "C"}[field]
						if scenario == "wrong_unit" && field == "rpm" {
							unit = "Hz"
						}
						d.Values = append(d.Values, domain.ObservationValue{Field: field,
							Value: map[string]string{"rpm": rpm, "voltage": "12", "temperature": "40"}[field],
							Unit:  unit, ObservedAt: q.WindowEnd})
					}
					return d
				}
				h := newDiagnosticHarness(t, filepath.Join(tempDir(t), "state.json"), plan, "500", domain.RunBudget{}, hook)
				putObservedDevice(t, h.server, "target")
				publishDiagnosticManual(t, h.server)
				r := awaitDiagnostic(t, h.server, submitDiagnostic(t, h.server))
				wantObservations := 2
				if scenario == "wrong_unit" {
					wantObservations = 1
				}
				if r.Status != "PAUSED" || r.Error == nil || r.Error.Code != "INVALID_PLAN" ||
					len(r.Evidence) != wantObservations || r.Result.RootCauseStatus == "CONFIRMED" || r.Result.RecoveryStatus == "RECOVERED" {
					t.Fatalf("invalid confirmation accepted: %+v", r)
				}
				for _, ev := range r.Evidence {
					if ev.Status != "OK" {
						t.Fatalf("test did not reach rule validation: %+v", ev)
					}
				}
			})
		}
	}
}
