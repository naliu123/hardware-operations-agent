package acceptance_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cloudwego/eino/schema"

	"hwops/internal/adapters/chatmodel"
	"hwops/internal/domain"
)

func putDevice(t *testing.T, s *httptest.Server, id, model, firmware string) map[string]any {
	t.Helper()
	return request(t, s.Client(), "PUT", s.URL+"/v1/devices/"+id, map[string]any{
		"name": id, "model": model, "firmware": firmware, "source": "fixture://inventory",
		"observed_at": time.Now().UTC(), "data_mode": "REPLAY",
	}, http.StatusOK)
}

func publishFor(t *testing.T, s *httptest.Server, applicability map[string]any, text string) string {
	t.Helper()
	revision := request(t, s.Client(), "POST", s.URL+"/v1/knowledge/revisions", map[string]any{
		"title": "合成设备手册", "source": "fixture://device-manual",
		"content": "# 蓝灯\n" + text, "applicability": applicability,
	}, http.StatusCreated)
	id := revision["id"].(string)
	request(t, s.Client(), "POST", s.URL+"/v1/knowledge/revisions/"+id+"/publication",
		map[string]any{"decision": "PUBLISH"}, http.StatusOK)
	return id
}

func TestSameQuestionMatchesTwoModelsAndTwoVersions(t *testing.T) {
	s, closeServer := startServer(t, filepath.Join(tempDir(t), "state.json"), externalExtractModel(t), "REPLAY")
	defer closeServer()
	cases := []struct{ device, model, firmware, answer string }{
		{"atlas-old", "fixture/Atlas", "R2", "蓝灯表示待机。"},
		{"atlas-new", "fixture/Atlas", "R10", "蓝灯表示维护。"},
		{"boreal-old", "fixture/Boreal", "B-9", "蓝灯表示升级。"},
		{"boreal-new", "fixture/Boreal", "B-10", "蓝灯表示自检。"},
	}
	revisions := map[string]string{}
	for _, c := range cases {
		putDevice(t, s, c.device, c.model, c.firmware)
		revisions[c.device] = publishFor(t, s, map[string]any{
			"scope": "DEVICE", "model": c.model, "firmware": c.firmware,
		}, c.answer)
	}
	conversation := request(t, s.Client(), "POST", s.URL+"/v1/conversations", map[string]any{}, http.StatusCreated)
	for _, c := range cases {
		t.Run(c.device, func(t *testing.T) {
			pending := request(t, s.Client(), "POST", s.URL+"/v1/conversations/"+conversation["id"].(string)+"/messages",
				map[string]any{"text": "蓝灯表示什么？", "device_id": c.device}, http.StatusAccepted)
			answer := awaitResponse(t, s, pending["id"].(string))
			if answer.Status != "ANSWERED" || !strings.Contains(answer.Answer, c.answer) ||
				len(answer.Citations) != 1 || answer.Citations[0].RevisionID != revisions[c.device] {
				t.Fatalf("wrong model/version knowledge: %+v", answer)
			}
			raw := request(t, s.Client(), "GET", s.URL+"/v1/responses/"+answer.ID, nil, http.StatusOK)
			device, ok := raw["device_context"].(map[string]any)
			if !ok || device["device_id"] != c.device || device["firmware"] != c.firmware ||
				raw["context_revision"] != device["snapshot_id"] {
				t.Fatalf("answer lost device provenance: %v", raw)
			}
		})
	}
}

func TestApplicabilityOutcomesAndMissingVersions(t *testing.T) {
	for _, c := range []struct{ name, model, firmware, status, outcome string }{
		{"match", "fixture/Atlas", "R2", "ANSWERED", "MATCH"},
		{"wrong_firmware", "fixture/Atlas", "R10", "UNRESOLVED", "MISMATCH"},
		{"wrong_model", "fixture/Boreal", "R2", "UNRESOLVED", "MISMATCH"},
		{"missing_firmware", "fixture/Atlas", "", "NEEDS_CLARIFICATION", "UNKNOWN"},
		{"missing_model", "", "R2", "NEEDS_CLARIFICATION", "UNKNOWN"},
		{"known_mismatch_wins_over_missing_version", "fixture/Boreal", "", "UNRESOLVED", "MISMATCH"},
	} {
		t.Run(c.name, func(t *testing.T) {
			s, closeServer := startServer(t, filepath.Join(tempDir(t), "state.json"), &chatmodel.Replay{}, "REPLAY")
			defer closeServer()
			putDevice(t, s, "target", c.model, c.firmware)
			revision := publishFor(t, s, map[string]any{"scope": "DEVICE", "model": "fixture/Atlas", "firmware": "R2"},
				"蓝灯表示待机。")
			answer := askDevice(t, s, "target")
			if answer.Status != c.status || len(answer.ApplicabilityChecks) != 1 ||
				answer.ApplicabilityChecks[0].Status != c.outcome || answer.ApplicabilityChecks[0].RevisionID != revision ||
				len(answer.ApplicabilityChecks[0].Checks) == 0 {
				t.Fatalf("incorrect applicability outcome: %+v", answer)
			}
			if c.outcome != "MATCH" && (answer.Answer != "" || len(answer.Citations) != 0) {
				t.Fatalf("ineligible version leaked: %+v", answer)
			}
		})
	}
}

func askDevice(t *testing.T, s *httptest.Server, id string) domain.Response {
	t.Helper()
	conversation := request(t, s.Client(), "POST", s.URL+"/v1/conversations", map[string]any{}, http.StatusCreated)
	pending := request(t, s.Client(), "POST", s.URL+"/v1/conversations/"+conversation["id"].(string)+"/messages",
		map[string]any{"text": "蓝灯表示什么？", "device_id": id}, http.StatusAccepted)
	return awaitResponse(t, s, pending["id"].(string))
}

func TestAmbiguousDeviceAliasRequiresSelection(t *testing.T) {
	s, closeServer := startServer(t, filepath.Join(tempDir(t), "state.json"), &chatmodel.Replay{}, "REPLAY")
	defer closeServer()
	for _, id := range []string{"rack-a", "rack-b"} {
		request(t, s.Client(), "PUT", s.URL+"/v1/devices/"+id, map[string]any{
			"name": id, "aliases": []string{"机柜设备"}, "model": "fixture/Atlas", "firmware": "R2",
			"observed_at": time.Now().UTC(), "source": "fixture://inventory", "data_mode": "REPLAY",
		}, http.StatusOK)
	}
	publishFor(t, s, map[string]any{"scope": "DEVICE", "model": "fixture/Atlas", "firmware": "R2"}, "蓝灯表示待机。")
	resolution := request(t, s.Client(), "GET", s.URL+"/v1/devices/resolve?q="+url.QueryEscape("机柜设备"), nil, http.StatusOK)
	if resolution["status"] != "AMBIGUOUS" || len(resolution["candidates"].([]any)) != 2 {
		t.Fatalf("alias silently picked a device: %v", resolution)
	}
	conversation := request(t, s.Client(), "POST", s.URL+"/v1/conversations", map[string]any{}, http.StatusCreated)
	path := s.URL + "/v1/conversations/" + conversation["id"].(string) + "/messages"
	pending := request(t, s.Client(), "POST", path,
		map[string]any{"text": "蓝灯表示什么？", "device_query": "机柜设备"}, http.StatusAccepted)
	answer := awaitResponse(t, s, pending["id"].(string))
	if answer.Status != "NEEDS_CLARIFICATION" || answer.Answer != "" || len(answer.Citations) != 0 {
		t.Fatalf("ambiguous device generated an answer: %+v", answer)
	}
	selected := request(t, s.Client(), "POST", path,
		map[string]any{"text": "蓝灯表示什么？", "device_id": "rack-b"}, http.StatusAccepted)
	answer = awaitResponse(t, s, selected["id"].(string))
	if answer.Status != "ANSWERED" || answer.DeviceContext == nil || answer.DeviceContext.DeviceID != "rack-b" {
		t.Fatalf("explicit selection did not resolve ambiguity: %+v", answer)
	}
}

func TestVersionRangeUsesRegisteredProductOrder(t *testing.T) {
	s, closeServer := startServer(t, filepath.Join(tempDir(t), "state.json"), &chatmodel.Replay{}, "REPLAY")
	defer closeServer()
	policy := request(t, s.Client(), "POST", s.URL+"/v1/knowledge/version-policies", map[string]any{
		"model": "fixture/Atlas", "source": "fixture://Atlas/version-order",
		"firmware_order": []string{"R1", "R2", "R10", "R3", "R20"},
	}, http.StatusCreated)
	policyID := policy["id"].(string)
	request(t, s.Client(), "GET", s.URL+"/v1/knowledge/version-policies/"+policyID, nil, http.StatusOK)
	for _, c := range []struct{ name, model, version, policy, min, max, status, outcome string }{
		{"lower_boundary", "fixture/Atlas", "R2", policyID, "R2", "R3", "ANSWERED", "MATCH"},
		{"product_order_not_lexical_or_semver", "fixture/Atlas", "R10", policyID, "R2", "R3", "ANSWERED", "MATCH"},
		{"upper_boundary", "fixture/Atlas", "R3", policyID, "R2", "R3", "ANSWERED", "MATCH"},
		{"below", "fixture/Atlas", "R1", policyID, "R2", "R3", "UNRESOLVED", "MISMATCH"},
		{"above", "fixture/Atlas", "R20", policyID, "R2", "R3", "UNRESOLVED", "MISMATCH"},
		{"unregistered_version", "fixture/Atlas", "R9", policyID, "R2", "R3", "NEEDS_CLARIFICATION", "UNKNOWN"},
		{"missing_rule", "fixture/Atlas", "R2", "", "R2", "R3", "NEEDS_CLARIFICATION", "UNKNOWN"},
		{"unknown_rule", "fixture/Atlas", "R2", "missing", "R2", "R3", "NEEDS_CLARIFICATION", "UNKNOWN"},
		{"other_product_rule", "fixture/Boreal", "R2", policyID, "R2", "R3", "NEEDS_CLARIFICATION", "UNKNOWN"},
		{"unknown_boundary", "fixture/Atlas", "R2", policyID, "R0", "R3", "NEEDS_CLARIFICATION", "UNKNOWN"},
		{"reversed_range", "fixture/Atlas", "R2", policyID, "R3", "R2", "NEEDS_CLARIFICATION", "UNKNOWN"},
	} {
		t.Run(c.name, func(t *testing.T) {
			putDevice(t, s, "target", c.model, c.version)
			id := publishFor(t, s, map[string]any{
				"scope": "DEVICE", "model": c.model, "policy_id": c.policy,
				"firmware_range": map[string]string{"min": c.min, "max": c.max},
			}, "蓝灯表示待机。")
			defer request(t, s.Client(), "POST", s.URL+"/v1/knowledge/revisions/"+id+"/publication",
				map[string]any{"decision": "WITHDRAW"}, http.StatusOK)
			answer := askDevice(t, s, "target")
			if answer.Status != c.status || len(answer.ApplicabilityChecks) != 1 ||
				answer.ApplicabilityChecks[0].Status != c.outcome {
				t.Fatalf("wrong product range decision: %+v", answer)
			}
			if c.outcome != "MATCH" && (answer.Answer != "" || len(answer.Citations) != 0) {
				t.Fatalf("unknown/mismatched range answered: %+v", answer)
			}
		})
	}
}

func TestTextDeviceNamesAndExplicitConflictsDoNotReuseContext(t *testing.T) {
	s, closeServer := startServer(t, filepath.Join(tempDir(t), "state.json"), &chatmodel.Replay{}, "REPLAY")
	defer closeServer()
	for _, id := range []string{"atlas", "atlas-2"} {
		putDevice(t, s, id, "fixture/"+id, "R2")
		publishFor(t, s, map[string]any{"scope": "DEVICE", "model": "fixture/" + id}, "蓝灯表示"+id+"待机。")
	}
	conversation := request(t, s.Client(), "POST", s.URL+"/v1/conversations", map[string]any{}, http.StatusCreated)
	path := s.URL + "/v1/conversations/" + conversation["id"].(string) + "/messages"
	for _, c := range []struct{ name, text, id, query, wantDevice, status string }{
		{"name_in_text", "atlas 的蓝灯表示什么？", "", "", "atlas", "ANSWERED"},
		{"whole_identifier", "atlas-2 的蓝灯表示什么？", "", "", "atlas-2", "ANSWERED"},
		{"two_names", "atlas 和 atlas-2 的蓝灯表示什么？", "", "", "", "NEEDS_CLARIFICATION"},
		{"conflicting_id", "atlas-2 的蓝灯表示什么？", "atlas", "", "", "NEEDS_CLARIFICATION"},
		{"unknown_id", "蓝灯表示什么？", "unknown", "", "", "NEEDS_CLARIFICATION"},
		{"unknown_alias", "蓝灯表示什么？", "", "不存在的设备", "", "NEEDS_CLARIFICATION"},
		{"no_implicit_carry", "它的蓝灯表示什么？", "", "", "", "NEEDS_CLARIFICATION"},
	} {
		t.Run(c.name, func(t *testing.T) {
			pending := request(t, s.Client(), "POST", path,
				map[string]any{"text": c.text, "device_id": c.id, "device_query": c.query}, http.StatusAccepted)
			answer := awaitResponse(t, s, pending["id"].(string))
			if answer.Status != c.status {
				t.Fatalf("incorrect resolution: %+v", answer)
			}
			if c.wantDevice != "" {
				if answer.DeviceContext == nil || answer.DeviceContext.DeviceID != c.wantDevice ||
					!strings.Contains(answer.Answer, c.wantDevice+"待机") {
					t.Fatalf("wrong text device: %+v", answer)
				}
			} else if answer.DeviceContext != nil || answer.Answer != "" || len(answer.Citations) != 0 {
				t.Fatalf("ambiguous question retained device context: %+v", answer)
			}
		})
	}
}

func TestDeviceUpdateDuringGenerationDropsObsoleteGuidance(t *testing.T) {
	for _, partial := range []bool{false, true} {
		name := "complete"
		if partial {
			name = "partial"
		}
		t.Run(name, func(t *testing.T) {
			started, release := make(chan chatmodel.ContextInput, 1), make(chan struct{})
			modelServer := httptest.NewServer(replayKnowledgeChoice(func(w http.ResponseWriter, r *http.Request) {
				var input struct {
					Messages []*schema.Message `json:"messages"`
				}
				if err := json.NewDecoder(r.Body).Decode(&input); err != nil || len(input.Messages) < 2 {
					http.Error(w, "invalid model input", 400)
					return
				}
				var payload chatmodel.ContextInput
				if err := json.Unmarshal([]byte(input.Messages[1].Content), &payload); err != nil || len(payload.Documents) == 0 {
					http.Error(w, "no documents", 400)
					return
				}
				started <- payload
				select {
				case <-release:
				case <-r.Context().Done():
					return
				}
				draft := domain.Draft{Claims: []domain.Claim{
					{Text: payload.Documents[0].Content, FragmentIDs: []string{payload.Documents[0].ID}},
				}}
				if partial {
					draft.Gaps = []string{"缺少该设备的实时温度读数。"}
				}
				content, _ := json.Marshal(draft)
				_ = json.NewEncoder(w).Encode(map[string]any{
					"choices": []any{map[string]any{"message": map[string]any{"content": string(content)}}},
				})
			}))
			defer modelServer.Close()
			cm, err := chatmodel.NewOpenAI(modelServer.URL, "fixture-model", "")
			if err != nil {
				t.Fatal(err)
			}
			s, closeServer := startServer(t, filepath.Join(tempDir(t), "state.json"), cm, "REPLAY")
			defer closeServer()
			old := putDevice(t, s, "target", "fixture/Atlas", "R2")
			publishFor(t, s, map[string]any{"scope": "DEVICE", "model": "fixture/Atlas", "firmware": "R2"}, "蓝灯表示待机。")
			publishFor(t, s, map[string]any{"scope": "DEVICE", "model": "fixture/Atlas", "firmware": "R10"}, "蓝灯表示维护。")
			conversation := request(t, s.Client(), "POST", s.URL+"/v1/conversations", map[string]any{}, http.StatusCreated)
			path := s.URL + "/v1/conversations/" + conversation["id"].(string) + "/messages"
			pending := request(t, s.Client(), "POST", path, map[string]any{"text": "蓝灯表示什么？", "device_id": "target"}, http.StatusAccepted)
			select {
			case payload := <-started:
				if payload.Device == nil || payload.Device.Firmware != "R2" || len(payload.Documents) != 1 ||
					payload.Documents[0].Content != "蓝灯表示待机。" {
					t.Fatalf("model received wrong device or ineligible content: %+v", payload)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("generation did not start")
			}
			putDevice(t, s, "target", "fixture/Atlas", "R10")
			close(release)
			answer := awaitResponse(t, s, pending["id"].(string))
			if answer.Status != "UNRESOLVED" || answer.Answer != "" || len(answer.Citations) != 0 ||
				answer.ContextRevision != old["snapshot_id"] || len(answer.Gaps) == 0 {
				t.Fatalf("obsolete device guidance published: %+v", answer)
			}
			next := askDevice(t, s, "target")
			wantStatus := "ANSWERED"
			if partial {
				wantStatus = "PARTIAL"
			}
			if next.Status != wantStatus || next.Answer != "蓝灯表示维护。" || next.DeviceContext.Firmware != "R10" {
				t.Fatalf("new question did not use upgraded device: %+v", next)
			}
		})
	}
}

func TestFirmwareDriverAndHardwareAreIndependentRequirements(t *testing.T) {
	s, closeServer := startServer(t, filepath.Join(tempDir(t), "state.json"), &chatmodel.Replay{}, "REPLAY")
	defer closeServer()
	policy := request(t, s.Client(), "POST", s.URL+"/v1/knowledge/version-policies", map[string]any{
		"model": "fixture/Atlas", "source": "fixture://driver-order", "driver_order": []string{"D2", "D10", "D3"},
	}, http.StatusCreated)
	publishFor(t, s, map[string]any{
		"scope": "DEVICE", "model": "fixture/Atlas", "firmware": "R2", "hardware_revision": "H1",
		"driver_range": map[string]string{"min": "D2", "max": "D10"}, "policy_id": policy["id"],
	}, "蓝灯表示待机。")
	for _, c := range []struct{ name, firmware, driver, hardware, status string }{
		{"all_match", "R2", "D10", "H1", "ANSWERED"},
		{"missing_driver", "R2", "", "H1", "NEEDS_CLARIFICATION"},
		{"missing_hardware", "R2", "D2", "", "NEEDS_CLARIFICATION"},
		{"wrong_hardware", "R2", "D2", "H2", "UNRESOLVED"},
		{"wrong_driver", "R2", "D3", "H1", "UNRESOLVED"},
		{"firmware_cannot_substitute_driver", "D2", "R2", "H1", "UNRESOLVED"},
	} {
		t.Run(c.name, func(t *testing.T) {
			request(t, s.Client(), "PUT", s.URL+"/v1/devices/target", map[string]any{
				"model": "fixture/Atlas", "firmware": c.firmware, "driver": c.driver, "hardware_revision": c.hardware,
				"source": "fixture://inventory", "observed_at": time.Now().UTC(), "data_mode": "REPLAY",
			}, http.StatusOK)
			answer := askDevice(t, s, "target")
			if answer.Status != c.status {
				t.Fatalf("version dimensions conflated: %+v", answer)
			}
		})
	}
}

func TestGeneralKnowledgeNeedsNoDeviceVersions(t *testing.T) {
	s, closeServer := startServer(t, filepath.Join(tempDir(t), "state.json"), &chatmodel.Replay{}, "REPLAY")
	defer closeServer()
	putDevice(t, s, "target", "", "")
	publishFor(t, s, map[string]any{"scope": "DEVICE", "model": "fixture/Atlas", "firmware": "R2"}, "蓝灯表示待机。")
	id := publishFor(t, s, map[string]any{"scope": "GENERAL"}, "蓝灯的颜色含义需查阅对应手册。")
	for _, device := range []string{"", "target"} {
		answer := askDevice(t, s, device)
		if answer.Status != "ANSWERED" || len(answer.Citations) != 1 || answer.Citations[0].RevisionID != id ||
			answer.Citations[0].DeviceSnapshotID != "" {
			t.Fatalf("general question required irrelevant device versions: %+v", answer)
		}
	}
}

func TestIneligibleHighRankedFragmentsDoNotConsumeContextBudget(t *testing.T) {
	s, closeServer := startServer(t, filepath.Join(tempDir(t), "state.json"), &chatmodel.Replay{}, "REPLAY")
	defer closeServer()
	putDevice(t, s, "target", "fixture/Atlas", "R2")
	for range 10 {
		publishFor(t, s, map[string]any{"scope": "DEVICE", "model": "fixture/Boreal", "firmware": "R2"},
			"蓝灯表示什么？蓝灯表示危险，应采用其他型号的配置。")
	}
	id := publishFor(t, s, map[string]any{"scope": "DEVICE", "model": "fixture/Atlas", "firmware": "R2"}, "蓝灯：待机。")
	answer := askDevice(t, s, "target")
	if answer.Status != "ANSWERED" || len(answer.Citations) != 1 || answer.Citations[0].RevisionID != id ||
		strings.Contains(answer.Answer, "危险") || len(answer.ApplicabilityChecks) != 11 {
		t.Fatalf("ranking displaced matching knowledge: %+v", answer)
	}
}

func TestDeviceObservationsRejectRollbackAndModeMixing(t *testing.T) {
	s, closeServer := startServer(t, filepath.Join(tempDir(t), "state.json"), &chatmodel.Replay{}, "REPLAY")
	defer closeServer()
	putDevice(t, s, "target", "fixture/Atlas", "R10")
	request(t, s.Client(), "PUT", s.URL+"/v1/devices/target", map[string]any{
		"model": "fixture/Atlas", "firmware": "R2", "source": "fixture://late-inventory",
		"observed_at": time.Now().Add(-time.Hour).UTC(), "data_mode": "REPLAY",
	}, http.StatusConflict)
	request(t, s.Client(), "PUT", s.URL+"/v1/devices/target", map[string]any{
		"model": "fixture/Atlas", "firmware": "R2", "source": "fixture://wrong-mode",
		"observed_at": time.Now().UTC(), "data_mode": "LIVE",
	}, http.StatusBadRequest)
	current := request(t, s.Client(), "GET", s.URL+"/v1/devices/resolve?q=target", nil, http.StatusOK)
	if current["candidates"].([]any)[0].(map[string]any)["firmware"] != "R10" {
		t.Fatalf("inventory rolled back: %v", current)
	}
}
