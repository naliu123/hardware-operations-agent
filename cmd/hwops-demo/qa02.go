package main

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"hwops/internal/domain"
)

func runQA02(ctx context.Context, call func(string, string, any, any) error) error {
	raw, err := os.ReadFile("testdata/qa02-devices.json")
	if err != nil {
		return err
	}
	var fixtures []struct {
		Model    string `json:"model"`
		Firmware string `json:"firmware"`
		Answer   string `json:"answer"`
	}
	if err := json.Unmarshal(raw, &fixtures); err != nil {
		return err
	}
	// Unique fixture namespaces avoid mixing revisions from previous demos.
	namespace := "fixture/demo-" + rand.Text() + "/"
	var conversation domain.Conversation
	if err := call("POST", "/v1/conversations", struct{}{}, &conversation); err != nil {
		return err
	}
	for i, fixture := range fixtures {
		id := fmt.Sprintf("demo-%s-%d", conversation.ID, i)
		var device domain.DeviceContext
		if err := call("PUT", "/v1/devices/"+id, domain.DeviceInput{
			Name: id, Model: namespace + fixture.Model, Firmware: fixture.Firmware,
			Source: "fixture://qa02-devices.json", ObservedAt: time.Now().UTC(), DataMode: "REPLAY",
		}, &device); err != nil {
			return err
		}
		var revision domain.Revision
		if err := call("POST", "/v1/knowledge/revisions", domain.RevisionInput{
			Title: "QA-02 合成设备手册", Source: "fixture://qa02-devices.json", Content: "# 蓝灯\n" + fixture.Answer,
			Applicability: domain.Applicability{Scope: "DEVICE", Model: device.Model, Firmware: device.Firmware},
		}, &revision); err != nil {
			return err
		}
		if err := call("POST", "/v1/knowledge/revisions/"+revision.ID+"/publication",
			map[string]string{"decision": "PUBLISH"}, &revision); err != nil {
			return err
		}
	}
	for i, fixture := range fixtures {
		var response domain.Response
		if err := call("POST", "/v1/conversations/"+conversation.ID+"/messages", domain.MessageInput{
			Text: "蓝灯表示什么？", DeviceID: fmt.Sprintf("demo-%s-%d", conversation.ID, i),
		}, &response); err != nil {
			return err
		}
		ticker := time.NewTicker(100 * time.Millisecond)
		for response.Status == "QUEUED" || response.Status == "RUNNING" {
			select {
			case <-ctx.Done():
				ticker.Stop()
				return ctx.Err()
			case <-ticker.C:
			}
			if err := call("GET", "/v1/responses/"+response.ID, nil, &response); err != nil {
				ticker.Stop()
				return err
			}
		}
		ticker.Stop()
		if response.DataMode != "REPLAY" || response.Status != "ANSWERED" || len(response.Citations) != 1 ||
			!strings.Contains(response.Answer, fixture.Answer) || response.DeviceContext == nil ||
			response.DeviceContext.Firmware != fixture.Firmware || response.DeviceContext.Model != namespace+fixture.Model ||
			response.Citations[0].DeviceSnapshotID != response.ContextRevision {
			return fmt.Errorf("QA-02 fixture mismatch: %+v", response)
		}
		var original domain.Fragment
		if err := call("GET", response.Citations[0].URL, nil, &original); err != nil {
			return err
		}
		if original.Content != fixture.Answer || original.ContentHash != response.Citations[0].ContentHash {
			return fmt.Errorf("QA-02 citation mismatch: %s", response.ID)
		}
		fmt.Printf("REPLAY %s / %s → %s\nresponse_id=%s snapshot=%s\n引用：%s\n\n",
			fixture.Model, fixture.Firmware, response.Answer, response.ID, response.ContextRevision, response.Citations[0].URL)
	}
	return nil
}
