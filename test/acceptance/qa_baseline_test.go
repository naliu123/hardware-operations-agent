package acceptance_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cloudwego/eino/schema"

	"hwops/internal/adapters/chatmodel"
	"hwops/internal/adapters/monitor"
	"hwops/internal/domain"
	"hwops/internal/evaluation"
)

// This is a protocol/structure baseline. The external provider deliberately
// extracts fixture text; its scores must never be presented as LLM quality.
func TestQABaseline(t *testing.T) {
	external := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/observe" {
			var q domain.ObservationQuery
			_ = json.NewDecoder(r.Body).Decode(&q)
			d := monitorData(q)
			switch q.Parameters["scenario"] {
			case "partial":
				d.Status, d.Values, d.Missing = "PARTIAL", d.Values[:1], []string{"temperature"}
			case "no-record":
				d.Status, d.Values = "NO_RECORD", nil
			case "failed":
				http.Error(w, "fixture monitor unavailable", http.StatusBadGateway)
				return
			}
			_ = json.NewEncoder(w).Encode(d)
			return
		}
		input := readAgentRequest(t, r)
		var context chatmodel.ContextInput
		_ = json.Unmarshal([]byte(input.Messages[1].Content), &context)
		if len(input.Messages) == 2 {
			if strings.HasPrefix(context.Question, "运行观测 ") {
				q := observationRequest()
				q.Parameters = map[string]string{"scenario": strings.TrimPrefix(context.Question, "运行观测 ")}
				raw, _ := json.Marshal(q)
				modelReply(w, "", schema.ToolCall{ID: "observe", Type: "function",
					Function: schema.FunctionCall{Name: "observe_device", Arguments: string(raw)}})
			} else {
				modelReply(w, "", knowledgeCall("knowledge", context.Question))
			}
			return
		}
		draft := domain.Draft{Claims: []domain.Claim{}}
		if strings.HasPrefix(context.Question, "运行观测 ") {
			var e domain.Evidence
			_ = json.Unmarshal([]byte(input.Messages[len(input.Messages)-1].Content), &e)
			draft.Observations = []domain.ObservationSelection{{EvidenceID: e.ID}}
		} else {
			for _, message := range input.Messages {
				if message.Role == schema.Tool {
					var payload chatmodel.ContextInput
					_ = json.Unmarshal([]byte(message.Content), &payload)
					context.Documents = append(context.Documents, payload.Documents...)
				}
			}
			if strings.Contains(context.Question, "C99") && len(context.Documents) == 2 {
				draft.Conflicts = []domain.KnowledgeConflict{{Subject: "C99处置要求相互冲突",
					FragmentIDs: []string{context.Documents[0].ID, context.Documents[1].ID}}}
			} else if len(context.Documents) > 0 {
				d := context.Documents[0]
				draft.Claims = []domain.Claim{{Text: d.Content, FragmentIDs: []string{d.ID}}}
			} else {
				draft.Gaps = []string{"没有适用的已检索资料。"}
			}
		}
		raw, _ := json.Marshal(draft)
		modelReply(w, string(raw))
	}))
	defer external.Close()
	cm, err := chatmodel.NewOpenAI(external.URL+"/chat", "fixture-extractive-v1", "")
	if err != nil {
		t.Fatal(err)
	}
	observer, err := monitor.New(external.URL+"/observe", "", "REPLAY", time.Second, 1)
	if err != nil {
		t.Fatal(err)
	}
	s, stop := startObservedServer(t, filepath.Join(tempDir(t), "state.json"), cm, observer)
	defer stop()
	report := evaluation.Baseline{SchemaVersion: 1, DataMode: "REPLAY", CreatedAt: time.Now().UTC(),
		Model:      "fixture-extractive-v1，经真实OpenAI HTTP适配器；本地词项检索；外部监控桩",
		LiveStatus: "本轮未评估；既有DeepSeek协议烟测与文档质量见03h/03f报告"}
	fixtures := []struct{ id, model, firmware, text string }{
		{"atlas-old", "fixture/Atlas", "R2", "E42 蓝灯表示待机；CFG8 配置前需记录原参数。"},
		{"atlas-new", "fixture/Atlas", "R10", "E42 蓝灯表示维护。"},
		{"boreal-old", "fixture/Boreal", "B-9", "E42 蓝灯表示升级。"},
		{"boreal-new", "fixture/Boreal", "B-10", "E42 蓝灯表示自检。"},
	}
	revisions := map[string]domain.Revision{}
	add := func(title, body string, applicability domain.Applicability) domain.Revision {
		created := request(t, s.Client(), "POST", s.URL+"/v1/knowledge/revisions", domain.RevisionInput{
			Title: title, Source: "fixture://qa06/" + title, Content: body, Applicability: applicability,
		}, http.StatusCreated)
		published := request(t, s.Client(), "POST", s.URL+"/v1/knowledge/revisions/"+created["id"].(string)+"/publication",
			map[string]any{"decision": "PUBLISH"}, http.StatusOK)
		raw, _ := json.Marshal(published)
		var revision domain.Revision
		_ = json.Unmarshal(raw, &revision)
		report.Revisions = append(report.Revisions, revision)
		return revision
	}
	for _, f := range fixtures {
		request(t, s.Client(), "PUT", s.URL+"/v1/devices/"+f.id, domain.DeviceInput{
			Name: f.id, Model: f.model, Firmware: f.firmware, MonitoringID: "monitor/" + f.id,
			Source: "fixture://qa06/inventory", ObservedAt: time.Now().UTC(), DataMode: "REPLAY",
		}, http.StatusOK)
		revisions[f.id] = add(f.id, f.text, domain.Applicability{Scope: "DEVICE", Model: f.model, Firmware: f.firmware})
	}
	putDevice(t, s, "unknown-version", "fixture/Atlas", "")
	for _, text := range []string{"C99要求立即重启。", "C99禁止重启。"} {
		add("conflict", text, domain.Applicability{Scope: "DEVICE", Model: "fixture/Atlas", Firmware: "R2"})
	}
	c := request(t, s.Client(), "POST", s.URL+"/v1/conversations", map[string]any{}, http.StatusCreated)
	samples := []struct{ id, text, device, resolved, status, expected, forbidden string }{
		{"atlas-r2-led", "蓝灯表示什么？", "atlas-old", "atlas-old", "ANSWERED", "待机", "维护"},
		{"atlas-r10-error", "E42表示什么？", "atlas-new", "atlas-new", "ANSWERED", "维护", "待机"},
		{"boreal-b9-led", "蓝灯表示什么？", "boreal-old", "boreal-old", "ANSWERED", "升级", "维护"},
		{"boreal-b10-led", "蓝灯表示什么？", "boreal-new", "boreal-new", "ANSWERED", "自检", "升级"},
		{"follow-up-pronoun", "它的E42表示什么？", "", "boreal-new", "ANSWERED", "自检", "维护"},
		{"switch-device-config", "CFG8配置有哪些前提？", "atlas-old", "atlas-old", "ANSWERED", "记录原参数", "自检"},
		{"unknown-version", "E42表示什么？", "unknown-version", "unknown-version", "NEEDS_CLARIFICATION", "", "待机"},
		{"retrieval-miss", "zzqqvvxx", "atlas-old", "atlas-old", "UNRESOLVED", "", "正常"},
		{"knowledge-conflict", "C99应该怎么处理？", "atlas-old", "atlas-old", "UNRESOLVED", "", "立即重启"},
		{"partial-observation", "运行观测 partial", "atlas-old", "atlas-old", "PARTIAL", "rpm = 500 rpm", "整机正常"},
		{"no-record", "运行观测 no-record", "atlas-old", "atlas-old", "ANSWERED", "此查询范围内无记录", "设备已恢复"},
		{"monitor-failure", "运行观测 failed", "atlas-old", "atlas-old", "UNRESOLVED", "", "设备故障已确认"},
	}
	for _, sample := range samples {
		start := time.Now()
		input := domain.MessageInput{Text: sample.text, DeviceID: sample.device}
		p := keyedMessage(t, s, c["id"].(string), sample.id, input, http.StatusAccepted)
		actual := awaitResponse(t, s, p.ID)
		result := evaluation.BaselineSample{
			ID: sample.id, Input: input, ExpectedStatus: sample.status, ExpectedText: sample.expected,
			ForbiddenAnswer: sample.forbidden, ExpectedDevice: sample.resolved,
			ElapsedMS: float64(time.Since(start).Microseconds()) / 1000, Actual: actual,
			Metrics: map[string]bool{
				"确定性答案规则": actual.Status == sample.status && strings.Contains(actual.Answer, sample.expected) &&
					!strings.Contains(actual.Answer, sample.forbidden),
				"设备上下文": actual.DeviceContext != nil && actual.DeviceContext.DeviceID == sample.resolved,
			},
		}
		if revision, ok := revisions[sample.resolved]; ok && sample.expected != "" && !strings.HasPrefix(sample.text, "运行观测") {
			result.ExpectedRevision = revision.ID
			result.Metrics["版本适用性"] = len(actual.Citations) == 1 && actual.Citations[0].RevisionID == revision.ID &&
				actual.Citations[0].Applicability.Status == "MATCH"
			result.Metrics["引用原文支持性"] = len(actual.Claims) == 1 && strings.Contains(actual.Claims[0].Text, revision.Content) &&
				len(actual.Citations) == 1 && actual.Citations[0].ContentHash == revision.Fragments[0].ContentHash
			result.Metrics["召回参考片段"] = false
			for _, id := range actual.RetrievedFragmentIDs {
				if id == revision.Fragments[0].ID {
					result.Metrics["召回参考片段"] = true
				}
			}
		}
		if sample.id == "knowledge-conflict" {
			result.Metrics["冲突出处与阻断"] = len(actual.Conflicts) == 1 && len(actual.Citations) == 2 && actual.Answer == ""
		}
		if sample.id == "monitor-failure" {
			result.Metrics["调用失败不是设备故障"] = len(actual.Evidence) == 1 && actual.Evidence[0].Status == "FAILED" &&
				strings.Contains(strings.Join(actual.Gaps, ""), "不能据此判断设备故障")
		}
		report.Samples = append(report.Samples, result)
	}
	if path := os.Getenv("HWOPS_QA_BASELINE_OUTPUT"); path != "" {
		raw, err := json.MarshalIndent(report, "", "  ")
		if err == nil {
			err = os.MkdirAll(filepath.Dir(path), 0755)
		}
		if err == nil {
			err = os.WriteFile(path, append(raw, '\n'), 0644)
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	for _, sample := range report.Samples {
		for metric, pass := range sample.Metrics {
			if !pass {
				t.Errorf("baseline %s failed %s: status=%s answer=%q", sample.ID, metric, sample.Actual.Status, sample.Actual.Answer)
			}
		}
	}
}
