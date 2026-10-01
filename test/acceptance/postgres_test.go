package acceptance_test

import (
	"context"
	"crypto/rand"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"hwops/internal/adapters/chatmodel"
	"hwops/internal/adapters/postgres"
	"hwops/internal/application"
	"hwops/internal/domain"
	"hwops/internal/transport/httpapi"
)

// This opt-in test creates schema and retains its records in a dedicated test DB.
func TestPostgreSQLAnswerSurvivesRestart(t *testing.T) {
	dsn := os.Getenv("HWOPS_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("HWOPS_TEST_DATABASE_URL is unset; PostgreSQL integration not verified")
	}
	start := func() (*httptest.Server, func()) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		store, err := postgres.Open(ctx, dsn)
		if err != nil {
			t.Fatal(err)
		}
		app, err := application.New(store, &chatmodel.Replay{}, "REPLAY")
		if err != nil {
			store.Close()
			t.Fatal(err)
		}
		server := httptest.NewServer(httpapi.New(app, token))
		return server, func() {
			server.Close()
			app.Close()
			store.Close()
		}
	}
	var deviceAnswer domain.Response
	var policyID string
	deviceID := "fixture-" + rand.Text()
	answer := func() domain.Response {
		s, closeServer := start()
		defer closeServer()
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		second, err := postgres.Open(ctx, dsn)
		if err == nil {
			second.Close()
			t.Fatal("database allowed a second QA-01 runtime")
		}
		query := "fixture" + rand.Text()
		revision := request(t, s.Client(), "POST", s.URL+"/v1/knowledge/revisions", map[string]any{
			"title": "PostgreSQL 合成测试", "source": "fixture://postgres",
			"content":       "# 测试片段\n" + query + " 表示合成测试通过，不代表设备状态。",
			"applicability": map[string]any{"scope": "GENERAL"},
		}, http.StatusCreated)
		id := revision["id"].(string)
		request(t, s.Client(), "POST", s.URL+"/v1/knowledge/revisions/"+id+"/publication",
			map[string]any{"decision": "PUBLISH"}, http.StatusOK)
		conversation := request(t, s.Client(), "POST", s.URL+"/v1/conversations", map[string]any{}, http.StatusCreated)
		pending := request(t, s.Client(), "POST", s.URL+"/v1/conversations/"+conversation["id"].(string)+"/messages",
			map[string]any{"text": query}, http.StatusAccepted)
		answer := awaitResponse(t, s, pending["id"].(string))
		if answer.Status != "ANSWERED" || len(answer.Citations) != 1 || answer.Citations[0].RevisionID != id {
			t.Fatalf("PostgreSQL question failed: %+v", answer)
		}
		putDevice(t, s, deviceID, deviceID, "R10")
		policy := request(t, s.Client(), "POST", s.URL+"/v1/knowledge/version-policies", map[string]any{
			"model": deviceID, "source": "fixture://postgres-order", "firmware_order": []string{"R2", "R10", "R3"},
		}, http.StatusCreated)
		policyID = policy["id"].(string)
		deviceQuery := "devicecheck" + rand.Text()
		revision = request(t, s.Client(), "POST", s.URL+"/v1/knowledge/revisions", map[string]any{
			"title": deviceQuery, "source": "fixture://postgres-device", "content": deviceQuery + " 合成设备规则",
			"applicability": map[string]any{
				"scope": "DEVICE", "model": deviceID, "policy_id": policyID,
				"firmware_range": map[string]string{"min": "R2", "max": "R3"},
			},
		}, http.StatusCreated)
		request(t, s.Client(), "POST", s.URL+"/v1/knowledge/revisions/"+revision["id"].(string)+"/publication",
			map[string]any{"decision": "PUBLISH"}, http.StatusOK)
		pending = request(t, s.Client(), "POST", s.URL+"/v1/conversations/"+conversation["id"].(string)+"/messages",
			map[string]any{"text": deviceQuery, "device_id": deviceID}, http.StatusAccepted)
		deviceAnswer = awaitResponse(t, s, pending["id"].(string))
		if deviceAnswer.Status != "ANSWERED" || deviceAnswer.DeviceContext == nil ||
			deviceAnswer.DeviceContext.Firmware != "R10" || len(deviceAnswer.Citations) != 1 ||
			deviceAnswer.Citations[0].RevisionID != revision["id"] {
			t.Fatalf("PostgreSQL device version failed: %+v", deviceAnswer)
		}
		return answer
	}()
	s, closeServer := start()
	defer closeServer()
	persisted := awaitResponse(t, s, answer.ID)
	if persisted.Answer != answer.Answer || len(persisted.Citations) != 1 ||
		persisted.Citations[0].ContentHash != answer.Citations[0].ContentHash {
		t.Fatalf("PostgreSQL result not durable: %+v", persisted)
	}
	request(t, s.Client(), "GET", s.URL+persisted.Citations[0].URL, nil, http.StatusOK)
	deviceHistory := awaitResponse(t, s, deviceAnswer.ID)
	if deviceHistory.Answer != deviceAnswer.Answer || deviceHistory.ContextRevision != deviceAnswer.ContextRevision {
		t.Fatalf("PostgreSQL lost device provenance: %+v", deviceHistory)
	}
	request(t, s.Client(), "GET", s.URL+"/v1/knowledge/version-policies/"+policyID, nil, http.StatusOK)
	device := request(t, s.Client(), "GET", s.URL+"/v1/devices/resolve?q="+deviceID, nil, http.StatusOK)
	if device["status"] != "RESOLVED" {
		t.Fatalf("PostgreSQL lost device: %v", device)
	}
}
