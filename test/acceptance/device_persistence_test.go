package acceptance_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/cloudwego/eino/schema"

	"hwops/internal/adapters/chatmodel"
	"hwops/internal/domain"
)

// The external endpoint deterministically cites every supplied fragment, so
// ineligible documents leaking into model input also appear in the HTTP result.
func externalExtractModel(t *testing.T) *chatmodel.OpenAI {
	t.Helper()
	server := httptest.NewServer(replayKnowledgeChoice(func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			Messages []*schema.Message `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil || len(input.Messages) < 2 {
			http.Error(w, "invalid model input", 400)
			return
		}
		var payload chatmodel.ContextInput
		if err := json.Unmarshal([]byte(input.Messages[1].Content), &payload); err != nil {
			http.Error(w, "invalid context", 400)
			return
		}
		draft := domain.Draft{Claims: []domain.Claim{}}
		for _, doc := range payload.Documents {
			draft.Claims = append(draft.Claims, domain.Claim{
				Text: "【REPLAY 外部模型桩】" + doc.Content, FragmentIDs: []string{doc.ID},
			})
		}
		raw, _ := json.Marshal(draft)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []any{map[string]any{"message": map[string]string{"content": string(raw)}}},
		})
	}))
	t.Cleanup(server.Close)
	cm, err := chatmodel.NewOpenAI(server.URL, "fixture-model", "")
	if err != nil {
		t.Fatal(err)
	}
	return cm
}

func TestConcurrentMessagesKeepIndependentDeviceSnapshots(t *testing.T) {
	s, closeServer := startServer(t, filepath.Join(tempDir(t), "state.json"), externalExtractModel(t), "REPLAY")
	defer closeServer()
	for _, id := range []string{"target-a", "target-b"} {
		putDevice(t, s, id, "fixture/"+id, "R2")
		publishFor(t, s, map[string]any{"scope": "DEVICE", "model": "fixture/" + id}, "蓝灯表示"+id+"待机。")
	}
	conversation := request(t, s.Client(), "POST", s.URL+"/v1/conversations", map[string]any{}, http.StatusCreated)
	var wg sync.WaitGroup
	for i := range 8 {
		id := []string{"target-a", "target-b"}[i%2]
		wg.Add(1)
		go func() {
			defer wg.Done()
			pending := request(t, s.Client(), "POST", s.URL+"/v1/conversations/"+conversation["id"].(string)+"/messages",
				map[string]any{"text": "蓝灯表示什么？", "device_id": id}, http.StatusAccepted)
			answer := awaitResponse(t, s, pending["id"].(string))
			if answer.Status != "ANSWERED" || answer.DeviceContext == nil || answer.DeviceContext.DeviceID != id ||
				!strings.Contains(answer.Answer, id+"待机") || len(answer.Citations) != 1 ||
				answer.Citations[0].DeviceSnapshotID != answer.ContextRevision {
				t.Errorf("concurrent question borrowed another device: %+v", answer)
			}
		}()
	}
	wg.Wait()
}

func TestDeviceSnapshotsAndImmutableVersionRulesSurviveRestart(t *testing.T) {
	path := filepath.Join(tempDir(t), "state.json")
	var policyID string
	old := func() domain.Response {
		s, closeServer := startServer(t, path, externalExtractModel(t), "REPLAY")
		defer closeServer()
		putDevice(t, s, "target", "fixture/Atlas", "R2")
		publishFor(t, s, map[string]any{"scope": "DEVICE", "model": "fixture/Atlas", "firmware": "R2"}, "蓝灯表示待机。")
		policy := request(t, s.Client(), "POST", s.URL+"/v1/knowledge/version-policies", map[string]any{
			"model": "fixture/Atlas", "source": "fixture://original-order", "firmware_order": []string{"R2", "R10", "R3"},
		}, http.StatusCreated)
		policyID = policy["id"].(string)
		publishFor(t, s, map[string]any{
			"scope": "DEVICE", "model": "fixture/Atlas", "policy_id": policyID,
			"firmware_range": map[string]string{"min": "R10", "max": "R3"},
		}, "蓝灯表示维护。")
		answer := askDevice(t, s, "target")
		if answer.Status != "ANSWERED" || !strings.Contains(answer.Answer, "待机") {
			t.Fatalf("old answer: %+v", answer)
		}
		request(t, s.Client(), "POST", s.URL+"/v1/knowledge/version-policies", map[string]any{
			"model": "fixture/Atlas", "source": "fixture://revised-order", "firmware_order": []string{"R3", "R10", "R2"},
		}, http.StatusCreated)
		putDevice(t, s, "target", "fixture/Atlas", "R10")
		return answer
	}()
	s, closeServer := startServer(t, path, externalExtractModel(t), "REPLAY")
	defer closeServer()
	history := awaitResponse(t, s, old.ID)
	if history.Answer != old.Answer || history.ContextRevision != old.ContextRevision ||
		history.DeviceContext == nil || history.DeviceContext.Firmware != "R2" ||
		len(history.Citations) != 1 || history.Citations[0].DeviceSnapshotID != old.ContextRevision {
		t.Fatalf("historical device provenance changed: %+v", history)
	}
	current := request(t, s.Client(), "GET", s.URL+"/v1/devices/resolve?q=target", nil, http.StatusOK)
	if current["candidates"].([]any)[0].(map[string]any)["firmware"] != "R10" {
		t.Fatal(current)
	}
	policy := request(t, s.Client(), "GET", s.URL+"/v1/knowledge/version-policies/"+policyID, nil, http.StatusOK)
	if policy["source"] != "fixture://original-order" {
		t.Fatal(policy)
	}
	answer := askDevice(t, s, "target")
	if answer.Status != "ANSWERED" || !strings.Contains(answer.Answer, "维护") ||
		strings.Contains(answer.Answer, "待机") || answer.ContextRevision == old.ContextRevision {
		t.Fatalf("restarted question lost current device or original policy: %+v", answer)
	}
	request(t, s.Client(), "GET", s.URL+history.Citations[0].URL, nil, http.StatusOK)
}

func TestQA01FileCanAcceptDevicesAndPoliciesAfterUpgrade(t *testing.T) {
	path := filepath.Join(tempDir(t), "state.json")
	// Seed the documented QA-01 file format; all observations use public HTTP.
	if err := os.WriteFile(path, []byte(`{"revisions":{},"conversations":{},"responses":{}}`), 0600); err != nil {
		t.Fatal(err)
	}
	s, closeServer := startServer(t, path, &chatmodel.Replay{}, "REPLAY")
	defer closeServer()
	putDevice(t, s, "target", "fixture/Atlas", "R2")
	policy := request(t, s.Client(), "POST", s.URL+"/v1/knowledge/version-policies", map[string]any{
		"model": "fixture/Atlas", "source": "fixture://order", "firmware_order": []string{"R2", "R10"},
	}, http.StatusCreated)
	publishFor(t, s, map[string]any{
		"scope": "DEVICE", "model": "fixture/Atlas", "policy_id": policy["id"],
		"firmware_range": map[string]string{"min": "R2", "max": "R10"},
	}, "蓝灯表示待机。")
	if answer := askDevice(t, s, "target"); answer.Status != "ANSWERED" {
		t.Fatalf("legacy file upgrade: %+v", answer)
	}
}
