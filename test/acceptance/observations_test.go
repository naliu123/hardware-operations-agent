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

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"

	"hwops/internal/adapters/chatmodel"
	"hwops/internal/adapters/filestore"
	"hwops/internal/adapters/monitor"
	"hwops/internal/application"
	"hwops/internal/domain"
	"hwops/internal/transport/httpapi"
)

func startObservedServer(t *testing.T, path string, cm model.BaseChatModel, observer domain.Observer) (*httptest.Server, func()) {
	t.Helper()
	store, err := filestore.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	app, err := application.New(store, cm, "REPLAY", application.Options{Observer: observer})
	if err != nil {
		store.Close()
		t.Fatal(err)
	}
	s := httptest.NewServer(httpapi.New(app, token))
	return s, func() { s.Close(); app.Close(); store.Close() }
}

func observationRequest() domain.ObservationRequest {
	now := time.Now().UTC().Truncate(time.Second)
	return domain.ObservationRequest{Capability: "metrics", Component: "fan/1", Fields: []string{"rpm", "temperature"},
		WindowStart: now.Add(-time.Minute), WindowEnd: now, MaxAgeSeconds: 120}
}

func monitorData(q domain.ObservationQuery) domain.ObservationData {
	return domain.ObservationData{
		Status: "OK", DataMode: "REPLAY", DeviceID: q.Device.DeviceID, SnapshotID: q.Device.SnapshotID,
		MonitoringID: q.Device.MonitoringID, WindowStart: q.WindowStart, WindowEnd: q.WindowEnd,
		Source: "fixture://monitor", RawRef: "fixture://samples/1",
		Values: []domain.ObservationValue{
			{Field: "rpm", Value: "500", Unit: "rpm", ObservedAt: q.WindowEnd},
			{Field: "temperature", Value: "40", Unit: "C", ObservedAt: q.WindowEnd},
		},
	}
}

func putObservedDevice(t *testing.T, s *httptest.Server, id string) {
	t.Helper()
	request(t, s.Client(), "PUT", s.URL+"/v1/devices/"+id, domain.DeviceInput{
		Name: id, Model: "fixture/Atlas", Firmware: "R2", Source: "fixture://inventory",
		MonitoringID: "monitor/" + id, ObservedAt: time.Now().UTC(), DataMode: "REPLAY",
	}, http.StatusOK)
}

func TestObservationContractThroughPublicHTTP(t *testing.T) {
	for _, scenario := range []string{"OK", "NO_RECORD", "PARTIAL", "FAILED", "STALE", "UNSUPPORTED",
		"wrong_device", "wrong_snapshot", "wrong_mapping", "wrong_window", "wrong_mode", "outside_window", "missing_unit", "missing_field"} {
		t.Run(scenario, func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var q domain.ObservationQuery
				_ = json.NewDecoder(r.Body).Decode(&q)
				d := monitorData(q)
				switch scenario {
				case "NO_RECORD", "UNSUPPORTED":
					d.Status, d.Values = scenario, nil
				case "FAILED":
					http.Error(w, "fixture outage", http.StatusBadGateway)
					return
				case "PARTIAL":
					d.Status, d.Values, d.Missing = "PARTIAL", d.Values[:1], []string{"temperature"}
				case "wrong_device":
					d.DeviceID = "another"
				case "wrong_snapshot":
					d.SnapshotID = "old"
				case "wrong_mapping":
					d.MonitoringID = "another"
				case "wrong_window":
					d.WindowStart = d.WindowStart.Add(-time.Hour)
				case "wrong_mode":
					d.DataMode = "LIVE"
				case "outside_window":
					d.Values[0].ObservedAt = d.WindowEnd.Add(time.Second)
				case "missing_unit":
					d.Values[0].Unit = ""
				case "missing_field":
					d.Values = d.Values[:1]
				}
				_ = json.NewEncoder(w).Encode(d)
			}))
			defer upstream.Close()
			adapter, err := monitor.New(upstream.URL, "", "REPLAY", time.Second, 1)
			if err != nil {
				t.Fatal(err)
			}
			s, stop := startObservedServer(t, filepath.Join(tempDir(t), "state.json"), &chatmodel.Replay{}, adapter)
			defer stop()
			putObservedDevice(t, s, "target")
			q := observationRequest()
			if scenario == "STALE" {
				q.WindowStart, q.WindowEnd = q.WindowStart.Add(-time.Hour), q.WindowEnd.Add(-time.Hour)
			}
			got := request(t, s.Client(), "POST", s.URL+"/v1/devices/target/observations", q, http.StatusOK)
			want := scenario
			if scenario != strings.ToUpper(scenario) {
				want = "FAILED"
			}
			if got["status"] != want || got["data_mode"] != "REPLAY" {
				t.Fatalf("unexpected observation: %v", got)
			}
			if want == "FAILED" && len(got["values"].([]any)) != 0 {
				t.Fatal("invalid external values escaped into evidence")
			}
		})
	}
}

func TestMainAgentObservationOnlyAnswerPreservesScopeAndRestart(t *testing.T) {
	var knowledgeCalls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var q domain.ObservationQuery
		_ = json.NewDecoder(r.Body).Decode(&q)
		d := monitorData(q)
		d.Status, d.Values, d.Missing = "PARTIAL", d.Values[:1], []string{"temperature"}
		_ = json.NewEncoder(w).Encode(d)
	}))
	defer upstream.Close()
	external := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		input := readAgentRequest(t, r)
		if len(input.Messages) == 2 {
			args, _ := json.Marshal(observationRequest())
			_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": schema.AssistantMessage("", []schema.ToolCall{{
				ID: "observe-1", Type: "function", Function: schema.FunctionCall{Name: "observe_device", Arguments: string(args)},
			}})}}})
			return
		}
		var evidence domain.Evidence
		for _, msg := range input.Messages {
			if msg.Role == schema.Tool {
				_ = json.Unmarshal([]byte(msg.Content), &evidence)
			}
			for _, call := range msg.ToolCalls {
				if call.Function.Name == "retrieve_hardware_knowledge" {
					knowledgeCalls.Add(1)
				}
			}
		}
		raw, _ := json.Marshal(domain.Draft{Claims: []domain.Claim{}, Observations: []domain.ObservationSelection{{EvidenceID: evidence.ID}}})
		modelReply(w, string(raw))
	}))
	defer external.Close()
	adapter, err := monitor.New(upstream.URL, "", "REPLAY", time.Second, 1)
	if err != nil {
		t.Fatal(err)
	}
	cm, err := chatmodel.NewOpenAI(external.URL, "fixture", "")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(tempDir(t), "state.json")
	s, stop := startObservedServer(t, path, cm, adapter)
	putObservedDevice(t, s, "target")
	c := request(t, s.Client(), "POST", s.URL+"/v1/conversations", map[string]any{}, http.StatusCreated)
	p := request(t, s.Client(), "POST", s.URL+"/v1/conversations/"+c["id"].(string)+"/messages",
		map[string]any{"text": "风扇转速和温度是多少？", "device_id": "target"}, http.StatusAccepted)
	got := awaitResponse(t, s, p["id"].(string))
	if got.Status != "PARTIAL" || len(got.Evidence) != 1 || knowledgeCalls.Load() != 0 ||
		!strings.Contains(got.Answer, "rpm = 500 rpm") || !strings.Contains(strings.Join(got.Gaps, ""), "temperature") ||
		strings.Contains(got.Answer, "整台设备正常") {
		t.Fatalf("observation answer lost scope: %+v", got)
	}
	stop()
	s, stop = startObservedServer(t, path, cm, adapter)
	defer stop()
	saved := awaitResponse(t, s, got.ID)
	if len(saved.Evidence) != 1 || saved.Evidence[0].Query.Device.MonitoringID != "monitor/target" || saved.Evidence[0].RawRef == "" {
		t.Fatal("observation provenance not durable")
	}
}
