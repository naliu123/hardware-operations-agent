package einoflow

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"time"

	"github.com/cloudwego/eino/schema"

	"hwops/internal/domain"
	"hwops/internal/evidence"
)

const observationToolName = "observe_device"

func observationInfo() *schema.ToolInfo {
	return &schema.ToolInfo{Name: observationToolName,
		Desc: "只读查询当前已确认设备的指标、告警或日志；必须提供实际问题所需字段、UTC时间窗口和时效要求。身份与数据模式由系统注入。FAILED不代表设备故障；部分正常指标不代表整台设备正常。",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"capability":      {Type: schema.String, Required: true, Enum: []string{"metrics", "alerts", "logs"}},
			"component":       {Type: schema.String},
			"fields":          {Type: schema.Array, Required: true, ElemInfo: &schema.ParameterInfo{Type: schema.String}},
			"parameters":      {Type: schema.Object},
			"window_start":    {Type: schema.String, Required: true, Desc: "RFC3339"},
			"window_end":      {Type: schema.String, Required: true, Desc: "RFC3339"},
			"max_age_seconds": {Type: schema.Integer, Required: true},
		})}
}

func invokeObservation(ctx context.Context, t *turn, service *evidence.Service, call schema.ToolCall) error {
	var input domain.ObservationRequest
	decoder := json.NewDecoder(strings.NewReader(call.Function.Arguments))
	decoder.DisallowUnknownFields()
	if call.Type != "function" || strings.TrimSpace(call.ID) == "" || t.CallIDs[call.ID] ||
		len(call.Function.Arguments) > 16000 || decoder.Decode(&input) != nil || decoder.Decode(new(any)) != io.EOF ||
		evidence.ValidateRequest(input, time.Now()) != nil {
		return ErrInvalidAnswer
	}
	raw, _ := json.Marshal(input)
	key := observationToolName + string(raw)
	if t.Queries[key] || t.ObservationCalls >= 3 {
		return ErrToolBudget
	}
	t.Queries[key], t.CallIDs[call.ID] = true, true
	t.ObservationCalls++
	if t.Response.DeviceContext == nil {
		t.NeedsContext = true
		t.Response.Gaps = append(t.Response.Gaps, "状态查询需要唯一确认的设备。")
		t.Messages = append(t.Messages, schema.ToolMessage(`{"status":"NEEDS_CONTEXT","gaps":["请指定唯一设备。"]}`, call.ID))
		return nil
	}
	result, err := service.Observe(ctx, *t.Response.DeviceContext, input)
	if err != nil {
		return err
	}
	t.Response.Evidence = append(t.Response.Evidence, result)
	raw, _ = json.Marshal(result)
	t.Messages = append(t.Messages, schema.ToolMessage(string(raw), call.ID))
	return nil
}

func validateObservations(t *turn, draft domain.Draft) ([]string, []string, error) {
	if len(draft.Observations) > 3 {
		return nil, nil, ErrInvalidAnswer
	}
	byID := map[string]domain.Evidence{}
	gaps := []string{}
	for i := range t.Response.Evidence {
		e := &t.Response.Evidence[i]
		if (e.Status == "OK" || e.Status == "PARTIAL" || e.Status == "NO_RECORD") && !evidence.Fresh(*e, time.Now()) {
			e.Status = "STALE"
		}
		byID[e.ID] = *e
		if gap := evidence.Gap(*e); gap != "" {
			gaps = append(gaps, gap)
		}
	}
	var texts []string
	seen := map[string]bool{}
	for _, selection := range draft.Observations {
		e, exists := byID[selection.EvidenceID]
		if !exists || seen[e.ID] {
			return nil, nil, ErrInvalidAnswer
		}
		seen[e.ID] = true
		text, err := evidence.Render(e, selection.Fields)
		if err != nil {
			return nil, nil, ErrInvalidAnswer
		}
		if e.Status != "FAILED" && e.Status != "UNSUPPORTED" {
			texts = append(texts, text)
		}
	}
	return texts, gaps, nil
}
