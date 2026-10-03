package chatmodel

import (
	"encoding/json"
	"github.com/cloudwego/eino/schema"
	"hwops/internal/domain"
)

func replayDiagnostic(raw string) (*schema.Message, error) {
	var in struct {
		Run      domain.DiagnosticRun `json:"diagnostic_run"`
		Incident domain.Incident      `json:"incident"`
	}
	if err := json.Unmarshal([]byte(raw), &in); err != nil {
		return nil, err
	}
	p := domain.PlanProposal{BaseStateVersion: in.Run.StateVersion, BasePlanVersion: in.Run.PlanVersion,
		Decision: "PAUSE", Reason: "REPLAY 摘录模型不生成实际设备诊断。", Gaps: []string{"需要支持结构化规划的模型及只读观测，或专用外部回放场景。"}}
	if in.Run.PlanVersion == 0 {
		p.Decision, p.Reason, p.Gaps = "CONTINUE", "REPLAY 检索事件相关知识。", nil
		p.Steps = []domain.DiagnosticStep{{ID: "replay-knowledge", Kind: "KNOWLEDGE", TargetRef: in.Run.Device.SnapshotID,
			Query: in.Incident.ErrorCode + " " + in.Incident.Description, Purpose: "取得适用手册", ExpectedObservation: "已发布的适用知识和规则"}}
	}
	data, err := json.Marshal(p)
	if err != nil {
		return nil, err
	}
	return schema.AssistantMessage(string(data), nil), nil
}
