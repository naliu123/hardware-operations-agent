package acceptance_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cloudwego/eino/schema"

	"hwops/internal/adapters/chatmodel"
	"hwops/internal/application"
	"hwops/internal/blobstore"
	"hwops/internal/domain"
	"hwops/internal/einoflow"
)

func wb06ToolCall(w http.ResponseWriter, id, name string, arguments any) {
	raw, _ := json.Marshal(arguments)
	index := 0
	wbStreamFrame(w, map[string]any{"choices": []any{map[string]any{
		"index": 0, "delta": map[string]any{"tool_calls": []schema.ToolCall{{
			Index: &index, ID: id, Type: "function",
			Function: schema.FunctionCall{Name: name, Arguments: string(raw)},
		}}},
	}}})
	wbStreamFrame(w, map[string]any{"choices": []any{map[string]any{
		"index": 0, "delta": map[string]any{}, "finish_reason": "tool_calls",
	}}})
	wbStreamDone(w)
}

func wb06Final(w http.ResponseWriter, draft domain.Draft) {
	raw, _ := json.Marshal(draft)
	runes := []rune(string(raw))
	cut := len(runes) / 2
	for _, part := range []string{string(runes[:cut]), string(runes[cut:])} {
		wbStreamFrame(w, map[string]any{"choices": []any{map[string]any{
			"index": 0, "delta": map[string]any{"content": part},
		}}})
	}
	wbStreamFrame(w, map[string]any{
		"choices": []any{map[string]any{"index": 0, "delta": map[string]any{}, "finish_reason": "stop"}},
		"usage":   map[string]int{"prompt_tokens": 80, "completion_tokens": 30, "total_tokens": 110},
	})
	wbStreamDone(w)
}

func TestWB06ActualLogAnalysisArtifactsAndFollowup(t *testing.T) {
	runner, parser := wbRunner(t), actualAttachmentParser(t)
	var mu sync.Mutex
	var firstArtifact string
	modelServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			Messages []*schema.Message `json:"messages"`
			Tools    []struct {
				Function struct {
					Name string `json:"name"`
				} `json:"function"`
			} `json:"tools"`
			Stream bool `json:"stream"`
		}
		if json.NewDecoder(r.Body).Decode(&input) != nil || !input.Stream || len(input.Messages) < 2 {
			http.Error(w, "invalid stream request", http.StatusBadRequest)
			return
		}
		names := map[string]bool{}
		for _, candidate := range input.Tools {
			names[candidate.Function.Name] = true
		}
		if !names[einofflowRead()] || !names[einofflowPython()] {
			http.Error(w, "private tools missing", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		var payload chatmodel.ContextInput
		_ = json.Unmarshal([]byte(input.Messages[1].Content), &payload)
		var toolMessages []*schema.Message
		for _, message := range input.Messages {
			if message.Role == schema.Tool {
				toolMessages = append(toolMessages, message)
			}
		}
		followup := strings.Contains(payload.Question, "重新分组")
		crossConversation := strings.Contains(payload.Question, "跨会话产物")
		forged := strings.Contains(payload.Question, "伪造执行参数")
		budget := strings.Contains(payload.Question, "三次失败预算")
		if forged {
			if len(toolMessages) == 0 {
				wb06ToolCall(w, "wb06-forged", einofflowPython(), map[string]any{
					"code": "print('must not run')", "input_ids": []string{},
					"owner_id": "forged", "host_path": "/etc/passwd", "network": true,
					"image": "attacker/image", "cpus": 64,
				})
				return
			}
			http.Error(w, "forged call unexpectedly executed", http.StatusBadRequest)
			return
		}
		if crossConversation {
			if len(toolMessages) == 0 {
				mu.Lock()
				artifactID := firstArtifact
				mu.Unlock()
				wb06ToolCall(w, "wb06-cross", einofflowPython(), map[string]any{
					"code":      fmt.Sprintf("print(open('/inputs/%s').read())", artifactID),
					"input_ids": []string{artifactID},
				})
				return
			}
			var rejected domain.PythonAnalysisResult
			_ = json.Unmarshal([]byte(toolMessages[len(toolMessages)-1].Content), &rejected)
			if rejected.Status != "REJECTED" || rejected.Execution.ID != "" {
				http.Error(w, "cross conversation artifact was accepted", http.StatusBadRequest)
				return
			}
			wb06Final(w, domain.Draft{Claims: []domain.Claim{},
				Gaps: []string{"前轮产物不属于当前会话，未执行分析。"}})
			return
		}
		if budget {
			if len(toolMessages) < 4 {
				wb06ToolCall(w, fmt.Sprintf("wb06-budget-%d", len(toolMessages)+1), einofflowPython(), map[string]any{
					"code": fmt.Sprintf("raise RuntimeError('attempt-%d')", len(toolMessages)+1),
				})
				return
			}
			var rejected domain.PythonAnalysisResult
			_ = json.Unmarshal([]byte(toolMessages[len(toolMessages)-1].Content), &rejected)
			if rejected.Status != "REJECTED" {
				http.Error(w, "fourth Python attempt was accepted", http.StatusBadRequest)
				return
			}
			wb06Final(w, domain.Draft{Claims: []domain.Claim{},
				Gaps: []string{"三次 Python 执行均失败，已达到本轮预算。"}})
			return
		}
		if !followup {
			switch len(toolMessages) {
			case 0:
				if len(payload.Attachments) != 1 {
					http.Error(w, "attachment context missing", http.StatusBadRequest)
					return
				}
				wb06ToolCall(w, "wb06-read", einofflowRead(), map[string]any{
					"attachment_id": payload.Attachments[0].ID, "page": 1,
					"start_line": 1, "end_line": 5,
				})
			case 1:
				var read domain.AttachmentReadResult
				_ = json.Unmarshal([]byte(toolMessages[0].Content), &read)
				if read.Status != "OK" || read.Source == nil {
					http.Error(w, "attachment read failed", http.StatusBadRequest)
					return
				}
				id := payload.Attachments[0].ID
				code := fmt.Sprintf(`import json
import time
from pathlib import Path
import pandas as pd
import matplotlib
matplotlib.use("Agg")
import matplotlib.pyplot as plt

time.sleep(0.7)
frame = pd.read_csv(Path("/inputs/%s"))
counts = frame.groupby("category").size().sort_values(ascending=False)
counts.rename("count").to_csv("/work/output/errors.csv")
counts.plot(kind="bar", color=["#476f62", "#b36a4e", "#707070"])
plt.ylabel("count")
plt.tight_layout()
plt.savefig("/work/output/errors.png")
print(json.dumps(counts.to_dict(), ensure_ascii=False, sort_keys=True))
`, id)
				wb06ToolCall(w, "wb06-python", einofflowPython(), map[string]any{
					"code": code, "input_ids": []string{id},
				})
			default:
				var result domain.PythonAnalysisResult
				_ = json.Unmarshal([]byte(toolMessages[len(toolMessages)-1].Content), &result)
				if result.Status != "SUCCEEDED" || result.Source == nil ||
					!strings.Contains(result.Execution.Result.Stdout, `"disk": 2`) {
					http.Error(w, "python result missing", http.StatusBadRequest)
					return
				}
				for _, artifact := range result.Execution.Result.Artifacts {
					if artifact.Name == "errors.csv" {
						mu.Lock()
						firstArtifact = artifact.ID
						mu.Unlock()
					}
				}
				wb06Final(w, domain.Draft{Claims: []domain.Claim{{
					Text:      "实际日志统计结果：disk 2 条，network 1 条，power 1 条；已生成 CSV 和柱状图。",
					SourceIDs: []string{result.Source.SourceID},
				}}, Gaps: []string{}, Conflicts: []domain.KnowledgeConflict{}})
			}
			return
		}

		switch len(toolMessages) {
		case 0:
			var artifactID string
			if payload.History != nil {
				for _, turn := range payload.History.Recent {
					for _, execution := range turn.Executions {
						for _, artifact := range execution.Artifacts {
							if artifact.Name == "errors.csv" {
								artifactID = artifact.ID
							}
						}
					}
				}
			}
			mu.Lock()
			expected := firstArtifact
			mu.Unlock()
			if artifactID == "" || artifactID != expected {
				http.Error(w, "history artifact identity missing", http.StatusBadRequest)
				return
			}
			code := fmt.Sprintf(`from pathlib import Path
import pandas as pd
import matplotlib
matplotlib.use("Agg")
import matplotlib.pyplot as plt

frame = pd.read_csv(Path("/inputs/%s"))
frame = frame.sort_values(["count"], ascending=False)
frame.to_csv("/work/output/regrouped.csv")
frame.plot(x="category", y="count", kind="bar", legend=False, color="#476f62")
plt.tight_layout()
plt.savefig("/work/output/regrouped.png")
print("rows=%%d total=%%d" %% (len(frame), int(frame["count"].sum())))
`, artifactID)
			wb06ToolCall(w, "wb06-followup-python", einofflowPython(), map[string]any{
				"code": code, "input_ids": []string{artifactID},
			})
		default:
			var result domain.PythonAnalysisResult
			_ = json.Unmarshal([]byte(toolMessages[len(toolMessages)-1].Content), &result)
			if result.Status != "SUCCEEDED" || result.Source == nil ||
				!strings.Contains(result.Execution.Result.Stdout, "rows=3 total=4") {
				http.Error(w, "followup result missing", http.StatusBadRequest)
				return
			}
			wb06Final(w, domain.Draft{Claims: []domain.Claim{{
				Text:      "已在新 Python 进程中读取前轮 errors.csv，按 count 重新排序并生成新 CSV 与图表。",
				SourceIDs: []string{result.Source.SourceID},
			}}, Gaps: []string{}, Conflicts: []domain.KnowledgeConflict{}})
		}
	}))
	defer modelServer.Close()
	cm, _ := chatmodel.NewOpenAI(modelServer.URL, "wb06-analysis-replay", "")
	files, err := blobstore.Open(filepath.Join(t.TempDir(), "private"), 2_000_000_000, 100_000_000)
	if err != nil {
		t.Fatal(err)
	}
	server, _, stop := wbStartOptions(t, workbenchDatabase(t), cm, true, application.Options{
		UsersMode: true, Runner: runner, AttachmentRunner: parser, Files: files,
	})
	defer stop()
	client := wbNewClient(t, server)
	client.login("admin", wbPassword)
	conversationID := client.request("POST", "/v1/conversations", map[string]any{}, 201)["id"].(string)
	logData := []byte("timestamp,category\n2026-10-04T01:00:00Z,disk\n2026-10-04T01:01:00Z,network\n2026-10-04T01:02:00Z,disk\n2026-10-04T01:03:00Z,power\n")
	uploaded := uploadAttachment(t, client, conversationID, "events.csv", "text/plain", logData, 201)
	attachment := waitAttachment(t, client, uploaded["id"].(string))
	if attachment["status"] != "READY" {
		t.Fatalf("log attachment unavailable: %v", attachment)
	}
	submitted := client.request("POST", "/v1/conversations/"+conversationID+"/messages", map[string]any{
		"text": "分析日志类别分布并生成 CSV 和图表", "attachment_ids": []string{uploaded["id"].(string)},
	}, 202)
	responseID := submitted["id"].(string)
	liveEvents := client.openEvents(responseID, 0)
	firstProgress := waitWBEvent(t, liveEvents, "tool_progress")
	if firstProgress.Execution == nil || firstProgress.Execution.Status == "SUCCEEDED" {
		t.Fatalf("execution did not expose an in-flight state: %+v", firstProgress)
	}
	liveEvents.close()
	answer := client.answer(responseID, 90*time.Second)
	if answer["status"] != "ANSWERED" || !strings.Contains(answer["answer"].(string), "disk 2") {
		debug := client.openEvents(responseID, 0)
		for event := range debug.events {
			if event.Execution != nil {
				t.Logf("WB06 execution event type=%s status=%s version=%d", event.Type,
					event.Execution.Status, event.Execution.Version)
			}
		}
		t.Fatalf("analysis answer failed: %v", answer)
	}
	executions := answer["executions"].([]any)
	if len(executions) != 1 {
		t.Fatalf("execution record missing: %v", answer)
	}
	execution := executions[0].(map[string]any)
	if execution["status"] != "SUCCEEDED" || execution["code_sha256"] == "" {
		t.Fatalf("execution provenance missing: %v", execution)
	}
	result := execution["result"].(map[string]any)
	artifacts := result["artifacts"].([]any)
	if len(artifacts) != 2 {
		t.Fatalf("artifacts missing: %v", artifacts)
	}
	for _, value := range artifacts {
		artifact := value.(map[string]any)
		req, _ := http.NewRequest(http.MethodGet, client.base+"/v1/artifacts/"+artifact["id"].(string)+"/content", nil)
		res, requestErr := client.client.Do(req)
		if requestErr != nil || res.StatusCode != http.StatusOK {
			t.Fatalf("artifact download failed: %v status=%v", requestErr, res.StatusCode)
		}
		raw, _ := io.ReadAll(res.Body)
		res.Body.Close()
		if len(raw) == 0 {
			t.Fatal("artifact was empty")
		}
	}
	events := client.openEvents(responseID, firstProgress.ID)
	var progress []string
	for event := range events.events {
		if event.Type == "tool_progress" && event.Execution != nil {
			progress = append(progress, event.Execution.Status)
		}
	}
	if len(progress) < 1 || progress[len(progress)-1] != "SUCCEEDED" {
		t.Fatalf("execution progress was not durable: %v", progress)
	}

	followup := client.request("POST", "/v1/conversations/"+conversationID+"/messages",
		map[string]string{"text": "使用前轮 errors.csv 重新分组并绘图"}, 202)
	next := client.answer(followup["id"].(string), 90*time.Second)
	if next["status"] != "ANSWERED" || !strings.Contains(next["answer"].(string), "新 Python 进程") {
		t.Fatalf("followup analysis failed: %v", next)
	}
	nextExecution := next["executions"].([]any)[0].(map[string]any)
	if nextExecution["id"] == execution["id"] ||
		nextExecution["inputs"].([]any)[0].(map[string]any)["kind"] != "ARTIFACT" {
		t.Fatalf("followup reused process state or lost artifact identity: %v", nextExecution)
	}

	otherConversation := client.request("POST", "/v1/conversations", map[string]any{}, 201)["id"].(string)
	cross := client.request("POST", "/v1/conversations/"+otherConversation+"/messages",
		map[string]string{"text": "尝试使用跨会话产物"}, 202)
	crossAnswer := client.answer(cross["id"].(string), 30*time.Second)
	if crossAnswer["status"] != "UNRESOLVED" || !strings.Contains(crossAnswer["gaps"].([]any)[0].(string), "不属于当前会话") {
		t.Fatalf("cross-conversation artifact was not rejected: %v", crossAnswer)
	}

	forged := client.request("POST", "/v1/conversations/"+conversationID+"/messages",
		map[string]string{"text": "伪造执行参数"}, 202)
	forgedAnswer := client.answer(forged["id"].(string), 30*time.Second)
	if forgedAnswer["status"] != "FAILED" ||
		forgedAnswer["error"].(map[string]any)["code"] != "INVALID_MODEL_OUTPUT" {
		t.Fatalf("forged trusted parameters were accepted: %v", forgedAnswer)
	}

	budgeted := client.request("POST", "/v1/conversations/"+conversationID+"/messages",
		map[string]string{"text": "验证三次失败预算"}, 202)
	budgetAnswer := client.answer(budgeted["id"].(string), 60*time.Second)
	if budgetAnswer["status"] != "UNRESOLVED" ||
		!strings.Contains(budgetAnswer["gaps"].([]any)[0].(string), "三次 Python") ||
		len(budgetAnswer["executions"].([]any)) != 3 {
		t.Fatalf("Python attempt budget failed: %v", budgetAnswer)
	}
	t.Log("ACTUAL + REPLAY: public upload -> model attachment read -> actual Linux/gVisor Python -> CSV/PNG -> artifact followup in a fresh process passed")
}

func einofflowRead() string   { return einoflow.ReadAttachmentToolName }
func einofflowPython() string { return einoflow.RunPythonToolName }

func TestWB06StopCancelsActualPythonTool(t *testing.T) {
	runner := wbRunner(t)
	var requests atomic.Int32
	modelServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		var input struct {
			Messages []*schema.Message `json:"messages"`
			Stream   bool              `json:"stream"`
		}
		if json.NewDecoder(r.Body).Decode(&input) != nil || !input.Stream {
			http.Error(w, "invalid stream request", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		for _, message := range input.Messages {
			if message.Role == schema.Tool {
				http.Error(w, "model continued after canceled tool", http.StatusBadRequest)
				return
			}
		}
		wb06ToolCall(w, "wb06-long-python", einofflowPython(), map[string]any{
			"code": "import time\nprint('STARTED', flush=True)\ntime.sleep(40)",
		})
	}))
	defer modelServer.Close()
	cm, _ := chatmodel.NewOpenAI(modelServer.URL, "wb06-stop-replay", "")
	files, err := blobstore.Open(filepath.Join(t.TempDir(), "private"), 500_000_000, 100_000_000)
	if err != nil {
		t.Fatal(err)
	}
	server, _, stop := wbStartOptions(t, workbenchDatabase(t), cm, true, application.Options{
		UsersMode: true, Runner: runner, Files: files,
	})
	defer stop()
	client := wbNewClient(t, server)
	client.login("admin", wbPassword)
	conversationID := client.request("POST", "/v1/conversations", map[string]any{}, 201)["id"].(string)
	submitted := client.request("POST", "/v1/conversations/"+conversationID+"/messages",
		map[string]string{"text": "运行长任务后停止"}, 202)
	responseID := submitted["id"].(string)
	stream := client.openEvents(responseID, 0)
	var executionID string
	for {
		event := waitWBEvent(t, stream, "tool_progress")
		if event.Execution == nil {
			t.Fatal("tool progress omitted execution")
		}
		executionID = event.Execution.ID
		if event.Execution.Status == "RUNNING" {
			break
		}
		if domain.PythonTerminal(event.Execution.Status) {
			t.Fatalf("execution ended before stop: %+v", event.Execution)
		}
	}
	client.request("POST", "/v1/responses/"+responseID+"/cancel", map[string]any{}, 202)
	answer := client.answer(responseID, 20*time.Second)
	answerExecutions, _ := answer["executions"].([]any)
	if answer["status"] != "CANCELED" || answer["answer"] != "" || len(answerExecutions) != 1 ||
		answerExecutions[0].(map[string]any)["status"] != "CANCELED" {
		t.Fatalf("stopped response was published: %v", answer)
	}
	execution := client.request("GET", "/v1/executions/"+executionID, nil, 200)
	if execution["status"] != "CANCELED" ||
		execution["result"].(map[string]any)["cleaned"] != true ||
		requests.Load() != 1 {
		t.Fatalf("actual Python tool was not stopped cleanly: execution=%v model_requests=%d",
			execution, requests.Load())
	}
	stream.close()
	t.Log("ACTUAL + REPLAY: stop canceled the running Linux/gVisor Python tool, cleaned it and prevented a follow-up model call")
}
