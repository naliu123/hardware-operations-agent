package acceptance_test

import (
	"os"
	"testing"
	"time"

	"hwops/internal/adapters/chatmodel"
	"hwops/internal/domain"
)

func TestWB10LiveReasoningAndNativeTool(t *testing.T) {
	if os.Getenv("HWOPS_TEST_LIVE_MODEL") != "1" {
		t.Skip("LIVE model opt-in required")
	}
	cm, err := chatmodel.NewOpenAI(os.Getenv("HWOPS_MODEL_ENDPOINT"), os.Getenv("HWOPS_MODEL"), os.Getenv("HWOPS_MODEL_API_KEY"))
	if err != nil {
		t.Fatal(err)
	}
	server, _, stop := wbStart(t, workbenchDatabase(t), cm, true, "LIVE")
	defer stop()
	client := wbNewClient(t, server)
	client.login("admin", wbPassword)
	wbPublish(client)
	for _, question := range []string{"你是谁？", "请查阅知识库中 WB02 合成手册，蓝灯表示什么？"} {
		cid := client.request("POST", "/v1/conversations", map[string]any{}, 201)["id"].(string)
		id := client.request("POST", "/v1/conversations/"+cid+"/messages", map[string]string{"text": question}, 202)["id"].(string)
		stream := client.openEvents(id, 0)
		timer := time.NewTimer(2 * time.Minute)
		var final *domain.Response
		reasoningDeltas, bodyDeltas := 0, 0
		started := time.Now()
		var firstReasoning time.Duration
	reading:
		for {
			select {
			case event, ok := <-stream.events:
				if !ok {
					break reading
				}
				switch event.Type {
				case "reasoning_delta":
					reasoningDeltas++
					if reasoningDeltas == 1 {
						firstReasoning = time.Since(started)
						if bodyDeltas != 0 {
							t.Error("body arrived before reasoning")
						}
					}
				case "answer_delta":
					bodyDeltas++
				}
				if event.Response != nil {
					final = event.Response
				}
			case <-timer.C:
				stream.close()
				t.Fatal("LIVE model timed out")
			}
		}
		timer.Stop()
		stream.close()
		if final == nil || final.Status != "ANSWERED" || final.Answer == "" || final.Reasoning == nil ||
			final.Reasoning.Status != "COMPLETED" || final.Reasoning.Text == "" || reasoningDeltas < 1 || bodyDeltas < 1 {
			t.Fatalf("LIVE reasoning/body incomplete: final=%v reasoning_deltas=%d body_deltas=%d", final != nil, reasoningDeltas, bodyDeltas)
		}
		if question != "你是谁？" && len(final.KnowledgeToolCalls) == 0 {
			t.Fatal("LIVE model omitted requested native knowledge tool")
		}
		t.Logf("LIVE model=%s response=%s status=%s reasoning_deltas=%d reasoning_bytes=%d body_deltas=%d tool_calls=%d first_reasoning=%s total=%s",
			os.Getenv("HWOPS_MODEL"), id, final.Status, reasoningDeltas, len(final.Reasoning.Text),
			bodyDeltas, len(final.KnowledgeToolCalls), firstReasoning.Round(time.Millisecond), time.Since(started).Round(time.Millisecond))
	}
}
