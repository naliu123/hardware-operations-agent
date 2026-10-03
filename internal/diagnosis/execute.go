package diagnosis

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"slices"
	"time"

	knowledgeagent "hwops/internal/agents/knowledge"
	"hwops/internal/domain"
)

func (e *Engine) execute(ctx context.Context, t *turn) (*turn, error) {
	ready := readySteps(t.run, t.proposal.Steps, t.incident.OccurredAt)
	if len(ready) == 0 {
		return t, invalid("no executable step after rechecking preconditions")
	}
	step := ready[0]
	for _, id := range step.KnowledgeRefs {
		if err := e.checkKnowledge(ctx, t.run, id); err != nil {
			return t, err
		}
	}
	ex := domain.StepExecution{ID: rand.Text(), Step: step, PlanVersion: t.run.PlanVersion, Status: "RUNNING", StartedAt: time.Now().UTC()}
	for _, old := range t.run.Executions {
		if reusableObservation(t.run, step, old) {
			n := len(t.run.Executions)
			if n >= 2 && t.run.Executions[n-1].Status == "REUSED" && t.run.Executions[n-2].Status == "REUSED" {
				return t, ErrNoProgress
			}
			now := time.Now().UTC()
			ex.Status, ex.ReusedFrom, ex.EvidenceIDs, ex.FinishedAt = "REUSED", old.ID, append([]string{}, old.EvidenceIDs...), &now
			t.run.Executions = append(t.run.Executions, ex)
			t.run.AddEvent("EVIDENCE_REUSED", "完整查询与快照一致，观测仍有效；沿用证据 "+old.ID)
			return t, e.save(ctx, &t.run, t.run.Device.SnapshotID)
		}
	}
	t.run.Phase = "EXECUTE"
	t.run.Executions = append(t.run.Executions, ex)
	index := len(t.run.Executions) - 1
	if err := e.reserve(ctx, t, "TOOL"); err != nil {
		return t, err
	}
	var callErr error
	if step.Kind == "OBSERVE" {
		var ev domain.Evidence
		ev, callErr = e.observer.Observe(ctx, t.run.Device, *step.Observation)
		if ev.ID != "" {
			t.run.Evidence = append(t.run.Evidence, ev)
			ex.EvidenceIDs = []string{ev.ID}
			if ev.Status != "OK" && ev.Status != "NO_RECORD" {
				ex.Error = &domain.Failure{Code: ev.Status, Message: "观测没有取得完整有效结果，不作设备故障结论。"}
			}
		}
	} else {
		var found knowledgeagent.Result
		found, callErr = knowledgeagent.Invoke(ctx, e.knowledge, step.Query, &t.run.Device, e.mode)
		ex.KnowledgeCall = &domain.KnowledgeToolCall{ID: ex.ID, Name: knowledgeagent.ToolName, Request: step.Query,
			Status: found.Status, RetrievalQueries: found.RetrievalQueries, Gaps: found.Gaps, TraceID: found.TraceID,
			EvidenceSelection: found.EvidenceSelection, ModelUsage: found.ModelUsage, Error: found.Error,
			CorpusGeneration: found.CorpusGeneration, IndexGenerations: found.IndexGenerations, GapReason: found.GapReason}
		if found.Status != knowledgeagent.StatusFound {
			ex.Error = &domain.Failure{Code: "KNOWLEDGE_GAP", Message: "未取得适用知识，需纠偏或补齐上下文。"}
		}
		for _, doc := range found.Evidence {
			if callErr != nil {
				break
			}
			rev, err := e.store.GetRevision(ctx, doc.RevisionID)
			if err != nil {
				callErr = err
				break
			}
			k := domain.DiagnosticKnowledge{Citation: doc.Citation, Content: doc.Content}
			for _, rule := range rev.DiagnosticRules {
				if rule.StartLine >= doc.StartLine && rule.EndLine <= doc.EndLine {
					k.Rules = append(k.Rules, rule)
				}
			}
			if !hasKnowledge(t.run, doc.ID) {
				next := append(append([]domain.DiagnosticKnowledge{}, t.run.Knowledge...), k)
				raw, _ := json.Marshal(next)
				if len(raw) > 48*1024 {
					ex.Error = &domain.Failure{Code: "KNOWLEDGE_CONTEXT_LIMIT", Message: "累计知识上下文达到48 KiB。"}
					break
				}
				t.run.Knowledge = next
			}
			ex.KnowledgeCall.FragmentIDs = append(ex.KnowledgeCall.FragmentIDs, doc.ID)
		}
	}
	now := time.Now().UTC()
	ex.Status, ex.FinishedAt = "SUCCEEDED", &now
	if callErr != nil {
		ex.Error = &domain.Failure{Code: "TOOL_FAILED", Message: "只读调用失败或被中断，不作设备故障结论。"}
	}
	if ex.Error != nil {
		ex.Status = "FAILED"
	}
	t.run.Executions[index] = ex
	t.run.AddEvent("EXECUTED", "已保存实际结果："+ex.Status+"；步骤 "+step.ID)
	// Always retain results against their original execution identity, even if
	// cancel won the state-version race while the read-only call was in flight.
	saveErr := e.save(context.Background(), &t.run, "")
	if errors.Is(saveErr, domain.ErrConflict) {
		e.archiveCanceled(t.run, ex)
	}
	if saveErr != nil {
		return t, saveErr
	}
	if callErr != nil && (ctx.Err() != nil || errors.Is(callErr, ErrBudget)) {
		return t, callErr
	}
	return t, nil
}

func (e *Engine) archiveCanceled(source domain.DiagnosticRun, ex domain.StepExecution) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	r, err := e.store.GetRun(ctx, source.ID)
	if err != nil || r.Status != "CANCELED" {
		return
	}
	for i := range r.Executions {
		if r.Executions[i].ID == ex.ID {
			r.Executions[i] = ex
			for _, ev := range source.Evidence {
				if slices.Contains(ex.EvidenceIDs, ev.ID) && !hasEvidence(r, ev.ID) {
					r.Evidence = append(r.Evidence, ev)
				}
			}
			if ex.KnowledgeCall != nil {
				for _, k := range source.Knowledge {
					if slices.Contains(ex.KnowledgeCall.FragmentIDs, k.Citation.FragmentID) && !hasKnowledge(r, k.Citation.FragmentID) {
						r.Knowledge = append(r.Knowledge, k)
					}
				}
			}
			r.AddEvent("LATE_RESULT", "已取消运行的只读调用结果归档到原步骤。")
			_ = e.save(ctx, &r, "")
			return
		}
	}
}
