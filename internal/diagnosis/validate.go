package diagnosis

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"

	"hwops/internal/domain"
	"hwops/internal/evidence"
	"hwops/internal/knowledge"
)

var (
	ErrProposal   = errors.New("invalid diagnostic proposal")
	ErrBudget     = errors.New("diagnostic budget exhausted")
	ErrNoProgress = errors.New("diagnosis made no new progress")
)

func invalid(reason string) error { return fmt.Errorf("%w: %s", ErrProposal, reason) }
func text(s string, max int) bool { return strings.TrimSpace(s) != "" && len(s) <= max }

func (e *Engine) validate(ctx context.Context, t *turn) error {
	p, r := t.proposal, &t.run
	if p.BaseStateVersion != t.baseVersion || p.BasePlanVersion != r.PlanVersion ||
		!slices.Contains([]string{"CONTINUE", "WAIT", "PAUSE", "COMPLETE"}, p.Decision) ||
		!text(p.Reason, 2000) || len(p.Steps) > 30 || len(p.Hypotheses) > 16 ||
		len(p.SupportingRefs) > 64 || len(p.Gaps) > 16 || len(p.WaitReasons) > 16 {
		return invalid("versions, decision or plan size")
	}
	for _, s := range append(append([]string{}, p.Gaps...), p.WaitReasons...) {
		if !text(s, 2000) {
			return invalid("empty or oversized gap/wait reason")
		}
	}
	for _, id := range p.SupportingRefs {
		if !hasEvidence(*r, id) && !hasKnowledge(*r, id) {
			return invalid("unknown supporting reference")
		}
	}
	hypotheses := map[string]bool{}
	for _, h := range p.Hypotheses {
		if !text(h.ID, 100) || hypotheses[h.ID] || !text(h.Description, 2000) || !text(h.Reason, 2000) ||
			!slices.Contains([]string{"CANDIDATE", "SUPPORTED", "EXCLUDED", "CONFIRMED"}, h.Status) || len(h.EvidenceIDs) > 30 {
			return invalid("invalid hypothesis")
		}
		hypotheses[h.ID] = true
		if h.Status != "CANDIDATE" && len(h.EvidenceIDs) == 0 {
			return invalid("hypothesis lacks actual observations")
		}
		for _, id := range h.EvidenceIDs {
			if !usableEvidence(*r, id, time.Now()) {
				return invalid("hypothesis uses failed, stale or foreign evidence")
			}
		}
		if h.Rule != nil {
			rule, err := e.rule(ctx, *r, *h.Rule)
			if err != nil {
				return err
			}
			if rule.Kind != "ROOT_CAUSE" || rule.ErrorCode != t.incident.ErrorCode || h.Description != rule.Conclusion {
				return invalid("hypothesis does not match published cause")
			}
			if h.Status == "CONFIRMED" && !conditionsHold(*r, rule.Conditions, h.EvidenceIDs, t.incident.OccurredAt) {
				return invalid("root cause confirmation conditions not met")
			}
		} else if h.Status == "CONFIRMED" {
			return invalid("confirmation requires a published rule")
		}
	}
	steps := map[string]domain.DiagnosticStep{}
	for _, s := range p.Steps {
		if !text(s.ID, 100) || s.TargetRef != r.Device.SnapshotID || !text(s.Purpose, 2000) ||
			!text(s.ExpectedObservation, 2000) || len(s.RetryReason) > 2000 ||
			len(s.DependsOn) > 30 || len(s.Preconditions) > 16 || len(s.HypothesisRefs) > 16 || len(s.KnowledgeRefs) > 16 {
			return invalid("step identity, purpose, target or size")
		}
		if _, exists := steps[s.ID]; exists {
			return invalid("duplicate step ID")
		}
		steps[s.ID] = s
		for _, h := range s.HypothesisRefs {
			if !hypotheses[h] {
				return invalid("unknown hypothesis")
			}
		}
		switch s.Kind {
		case "KNOWLEDGE":
			if !text(s.Query, 16000) || s.Observation != nil {
				return invalid("invalid knowledge step")
			}
		case "OBSERVE":
			if s.Observation == nil || s.Query != "" || len(s.KnowledgeRefs) == 0 {
				return invalid("observation lacks query or knowledge basis")
			}
			if err := evidence.ValidateRequest(*s.Observation, time.Now()); err != nil {
				return invalid(err.Error())
			}
		default:
			return invalid("only read-only KNOWLEDGE and OBSERVE steps are supported")
		}
		for _, id := range s.KnowledgeRefs {
			if err := e.checkKnowledge(ctx, *r, id); err != nil {
				return err
			}
		}
		for _, c := range s.Preconditions {
			if !hasEvidence(*r, c.EvidenceID) || knowledge.ValidateMetricCondition(c.Condition) != nil {
				return invalid("invalid precondition reference or metric")
			}
		}
		for _, old := range r.Executions {
			if old.Step.ID == s.ID && !equalJSON(old.Step, s) {
				return invalid("cannot change an executed step")
			}
			if old.Step.ID != s.ID && fingerprint(old.Step) == fingerprint(s) && !text(s.RetryReason, 2000) &&
				!reusableObservation(*r, s, old) {
				return invalid("repeated query requires new information or a retry reason")
			}
		}
	}
	visiting, visited := map[string]bool{}, map[string]bool{}
	var visit func(string) error
	visit = func(id string) error {
		if visiting[id] {
			return invalid("cyclic dependency")
		}
		if visited[id] {
			return nil
		}
		s, ok := steps[id]
		if !ok {
			if successful(*r, id) {
				return nil
			}
			return invalid("unknown or unsuccessful dependency")
		}
		visiting[id] = true
		for _, dep := range s.DependsOn {
			if err := visit(dep); err != nil {
				return err
			}
		}
		visiting[id], visited[id] = false, true
		return nil
	}
	for id := range steps {
		if err := visit(id); err != nil {
			return err
		}
	}
	ready := readySteps(*r, p.Steps, t.incident.OccurredAt)
	if p.Decision == "CONTINUE" && len(ready) == 0 {
		return invalid("CONTINUE has no ready step")
	}
	if p.Decision == "WAIT" && (len(ready) != 0 || len(p.WaitReasons) == 0) {
		return invalid("WAIT requires all branches blocked and explicit waiting conditions")
	}
	if p.Decision == "PAUSE" && len(p.Gaps) == 0 {
		return invalid("PAUSE requires a concrete gap")
	}
	if p.Decision == "COMPLETE" {
		for _, s := range p.Steps {
			if !executed(*r, s.ID) {
				return invalid("COMPLETE retains an unfinished step")
			}
		}
		if p.Result == nil || !text(p.Result.Summary, 4000) {
			return invalid("COMPLETE requires a result")
		}
	}
	if p.Result != nil {
		result := p.Result
		if !text(result.Summary, 4000) || len(result.EvidenceIDs) > 30 ||
			result.RootCauseStatus != rootStatus(p.Hypotheses) ||
			!slices.Contains([]string{"UNKNOWN", "RECOVERED"}, result.RecoveryStatus) {
			return invalid("result root cause/recovery states")
		}
		for _, id := range result.EvidenceIDs {
			if !usableEvidence(*r, id, time.Now()) {
				return invalid("result uses invalid evidence")
			}
		}
		if result.RecoveryStatus == "RECOVERED" {
			if result.RecoveryRule == nil {
				return invalid("recovery requires published criteria")
			}
			rule, err := e.rule(ctx, *r, *result.RecoveryRule)
			if err != nil {
				return err
			}
			if rule.Kind != "RECOVERY" || rule.ErrorCode != t.incident.ErrorCode ||
				!conditionsHold(*r, rule.Conditions, result.EvidenceIDs, t.incident.OccurredAt) {
				return invalid("recovery criteria not met")
			}
		}
	}
	return nil
}

func equalJSON(a, b any) bool {
	x, _ := json.Marshal(a)
	y, _ := json.Marshal(b)
	return string(x) == string(y)
}
func fingerprint(s domain.DiagnosticStep) string {
	raw, _ := json.Marshal(struct {
		Kind, Target, Query string
		Observation         *domain.ObservationRequest
	}{s.Kind, s.TargetRef, strings.ToLower(strings.Join(strings.Fields(s.Query), " ")), s.Observation})
	return string(raw)
}
func rootStatus(hypotheses []domain.Hypothesis) string {
	status := "UNKNOWN"
	for _, h := range hypotheses {
		if h.Status == "CONFIRMED" {
			return "CONFIRMED"
		}
		if h.Status == "SUPPORTED" {
			status = "SUPPORTED"
		}
	}
	return status
}
func hasEvidence(r domain.DiagnosticRun, id string) bool {
	for _, ev := range r.Evidence {
		if ev.ID == id {
			return true
		}
	}
	return false
}
func hasKnowledge(r domain.DiagnosticRun, id string) bool {
	for _, k := range r.Knowledge {
		if k.Citation.FragmentID == id {
			return true
		}
	}
	return false
}
func usableEvidence(r domain.DiagnosticRun, id string, now time.Time) bool {
	for _, ev := range r.Evidence {
		if ev.ID == id {
			return ev.Status == "OK" && ev.SnapshotID == r.Device.SnapshotID &&
				ev.DeviceID == r.Device.DeviceID && ev.DataMode == r.DataMode && evidence.Fresh(ev, now)
		}
	}
	return false
}
func executed(r domain.DiagnosticRun, id string) bool {
	for _, ex := range r.Executions {
		if ex.Step.ID == id {
			return true
		}
	}
	return false
}
func successful(r domain.DiagnosticRun, id string) bool {
	for _, ex := range r.Executions {
		if ex.Step.ID == id && (ex.Status == "SUCCEEDED" || ex.Status == "REUSED") {
			return true
		}
	}
	return false
}
func readySteps(r domain.DiagnosticRun, steps []domain.DiagnosticStep, occurred time.Time) []domain.DiagnosticStep {
	var ready []domain.DiagnosticStep
	for _, s := range steps {
		if executed(r, s.ID) {
			continue
		}
		ok := true
		for _, dep := range s.DependsOn {
			ok = ok && successful(r, dep)
		}
		for _, c := range s.Preconditions {
			ok = ok && conditionsHold(r, []domain.MetricCondition{c.Condition}, []string{c.EvidenceID}, occurred)
		}
		if ok {
			ready = append(ready, s)
		}
	}
	return ready
}
func reusableObservation(r domain.DiagnosticRun, s domain.DiagnosticStep, old domain.StepExecution) bool {
	if s.Observation == nil || fingerprint(s) != fingerprint(old.Step) {
		return false
	}
	for _, ev := range r.Evidence {
		if slices.Contains(old.EvidenceIDs, ev.ID) && evidence.Reusable(ev, domain.ObservationQuery{
			ObservationRequest: *s.Observation, Device: r.Device,
		}, time.Now()) {
			return true
		}
	}
	return false
}

func (e *Engine) checkKnowledge(ctx context.Context, r domain.DiagnosticRun, id string) error {
	for _, k := range r.Knowledge {
		if k.Citation.FragmentID != id {
			continue
		}
		rev, err := e.store.GetRevision(ctx, k.Citation.RevisionID)
		if err != nil {
			return err
		}
		a, err := knowledge.Assess(ctx, e.store, rev.Applicability, &r.Device)
		if err != nil {
			return err
		}
		if rev.Status != "PUBLISHED" || a.Status != "MATCH" {
			return invalid("knowledge withdrawn or no longer applicable")
		}
		for _, f := range rev.Fragments {
			if f.ID == id && f.Content == k.Content && f.ContentHash == k.Citation.ContentHash {
				return nil
			}
		}
	}
	return invalid("unknown or altered knowledge reference")
}

func (e *Engine) rule(ctx context.Context, r domain.DiagnosticRun, ref domain.RuleRef) (domain.DiagnosticRule, error) {
	for _, k := range r.Knowledge {
		if k.Citation.RevisionID != ref.RevisionID {
			continue
		}
		for _, rule := range k.Rules {
			if rule.ID != ref.RuleID {
				continue
			}
			if err := e.checkKnowledge(ctx, r, k.Citation.FragmentID); err != nil {
				return rule, err
			}
			rev, err := e.store.GetRevision(ctx, ref.RevisionID)
			if err != nil {
				return rule, err
			}
			for _, authoritative := range rev.DiagnosticRules {
				if equalJSON(authoritative, rule) {
					return rule, nil
				}
			}
		}
	}
	return domain.DiagnosticRule{}, invalid("confirmation rule not retrieved from published knowledge")
}

// A criterion applies to all returned samples for the field, with exact units,
// component and query parameters. Newer conflicting observations cannot be
// bypassed by selecting an older evidence ID.
func conditionsHold(r domain.DiagnosticRun, conditions []domain.MetricCondition, ids []string, occurred time.Time) bool {
	now := time.Now()
	for _, c := range conditions {
		var newest time.Time
		var candidates []domain.Evidence
		for _, ev := range r.Evidence {
			if !usableEvidence(r, ev.ID, now) || ev.Query.Capability != c.Capability || ev.Query.Component != c.Component ||
				!maps.Equal(ev.Query.Parameters, c.Parameters) {
				continue
			}
			for _, v := range ev.Values {
				if v.Field == c.Field && v.ObservedAt.After(newest) {
					newest = v.ObservedAt
				}
			}
			if slices.Contains(ids, ev.ID) {
				candidates = append(candidates, ev)
			}
		}
		matched := false
		for _, ev := range candidates {
			found, all := false, true
			for _, v := range ev.Values {
				if v.Field != c.Field {
					continue
				}
				found = found || v.ObservedAt.Equal(newest)
				all = all && !v.ObservedAt.Before(occurred) && !v.ObservedAt.Before(now.Add(-time.Duration(c.MaxAgeSeconds)*time.Second)) &&
					v.Unit == c.Unit && compare(v.Value, c.Operator, c.Value)
			}
			matched = matched || (found && all)
		}
		if !matched {
			return false
		}
	}
	return len(conditions) > 0
}

func compare(value, op, want string) bool {
	if op == "EQ" {
		return value == want
	}
	a, ea := strconv.ParseFloat(value, 64)
	b, eb := strconv.ParseFloat(want, 64)
	if ea != nil || eb != nil || math.IsNaN(a) || math.IsNaN(b) || math.IsInf(a, 0) || math.IsInf(b, 0) {
		return false
	}
	switch op {
	case "GT":
		return a > b
	case "GE":
		return a >= b
	case "LT":
		return a < b
	case "LE":
		return a <= b
	}
	return false
}
