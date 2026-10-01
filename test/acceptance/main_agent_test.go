package acceptance_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/cloudwego/eino/schema"

	"hwops/internal/adapters/chatmodel"
	knowledgeagent "hwops/internal/agents/knowledge"
	"hwops/internal/domain"
)

type agentModelRequest struct {
	Messages   []*schema.Message `json:"messages"`
	Tools      []json.RawMessage `json:"tools"`
	ToolChoice string            `json:"tool_choice"`
}

func readAgentRequest(t *testing.T, r *http.Request) agentModelRequest {
	t.Helper()
	var input agentModelRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		t.Error(err)
	}
	return input
}

func modelReply(w http.ResponseWriter, content string, calls ...schema.ToolCall) {
	_ = json.NewEncoder(w).Encode(map[string]any{
		"choices": []any{map[string]any{"message": map[string]any{
			"role": "assistant", "content": content, "tool_calls": calls,
		}}},
		"usage": map[string]int{"prompt_tokens": 7, "completion_tokens": 3, "total_tokens": 10},
	})
}

func knowledgeCall(id, question string) schema.ToolCall {
	args, _ := json.Marshal(map[string]string{"request": question})
	return schema.ToolCall{ID: id, Type: "function", Function: schema.FunctionCall{
		Name: knowledgeagent.ToolName, Arguments: string(args),
	}}
}

func TestMainAgentRequeriesKnowledgeSubagentThroughPublicHTTP(t *testing.T) {
	var requests atomic.Int32
	external := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		input := readAgentRequest(t, r)
		if len(input.Tools) != 1 || input.ToolChoice != "auto" {
			t.Error("main agent did not register its knowledge tool")
		}
		var results []struct {
			chatmodel.ContextInput
			Status string `json:"status"`
		}
		for i, message := range input.Messages {
			if message.Role != schema.Tool {
				continue
			}
			if i == 0 || len(input.Messages[i-1].ToolCalls) != 1 ||
				message.ToolCallID != input.Messages[i-1].ToolCalls[0].ID {
				t.Error("tool result is not correlated to the model-selected call")
			}
			var result struct {
				chatmodel.ContextInput
				Status string `json:"status"`
			}
			_ = json.Unmarshal([]byte(message.Content), &result)
			if result.Device == nil || result.Device.DeviceID != "target" || result.Device.Firmware != "R2" {
				t.Error("trusted device snapshot missing from tool evidence")
			}
			results = append(results, result)
		}
		switch len(results) {
		case 0:
			modelReply(w, "", knowledgeCall("first", "zxqvnomatch"))
		case 1:
			if results[0].Status != "NOT_FOUND" || len(results[0].Documents) != 0 {
				t.Error("first model-selected query unexpectedly found evidence")
			}
			modelReply(w, "", knowledgeCall("second", "蓝灯表示什么？"))
		case 2:
			if results[1].Status != "FOUND" || len(results[1].Documents) != 1 {
				t.Error("revised query did not return its evidence")
				http.Error(w, "missing evidence", 400)
				return
			}
			doc := results[1].Documents[0]
			content, _ := json.Marshal(domain.Draft{Claims: []domain.Claim{{
				Text: doc.Content, FragmentIDs: []string{doc.ID},
			}}})
			modelReply(w, string(content))
		default:
			http.Error(w, "unexpected loop", 400)
		}
	}))
	defer external.Close()
	cm, err := chatmodel.NewOpenAI(external.URL, "fixture-model", "")
	if err != nil {
		t.Fatal(err)
	}
	s, closeServer := startServer(t, filepath.Join(tempDir(t), "state.json"), cm, "REPLAY")
	defer closeServer()
	putDevice(t, s, "target", "fixture/Atlas", "R2")
	publishFor(t, s, map[string]any{"scope": "DEVICE", "model": "fixture/Atlas", "firmware": "R2"}, "蓝灯表示待机。")
	publishFor(t, s, map[string]any{"scope": "DEVICE", "model": "fixture/Atlas", "firmware": "R10"}, "蓝灯表示维护。")
	answer := askDevice(t, s, "target")
	if answer.Status != "ANSWERED" || answer.DataMode != "REPLAY" || answer.Answer != "蓝灯表示待机。" ||
		len(answer.Citations) != 1 || answer.Citations[0].DeviceSnapshotID != answer.ContextRevision ||
		len(answer.KnowledgeToolCalls) != 2 || answer.KnowledgeToolCalls[0].Status != "NOT_FOUND" ||
		answer.KnowledgeToolCalls[1].Request != "蓝灯表示什么？" || answer.KnowledgeToolCalls[1].Status != "FOUND" ||
		answer.ModelUsage == nil || answer.ModelUsage.Calls != 3 || answer.ModelUsage.TotalTokens != 30 || requests.Load() != 3 {
		t.Fatalf("agent did not choose and revise its own tool query: %+v", answer)
	}
	original := request(t, s.Client(), "GET", s.URL+answer.Citations[0].URL, nil, http.StatusOK)
	if original["content_hash"] != answer.Citations[0].ContentHash {
		t.Fatal("citation does not match authoritative source")
	}
}

func TestMainAgentCanFinishWithoutRetrieval(t *testing.T) {
	external := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		input := readAgentRequest(t, r)
		if len(input.Tools) != 1 || len(input.Messages) != 2 {
			t.Error("unexpected initial tool context")
		}
		modelReply(w, `{"claims":[],"gaps":["缺少实时温度数据。"]}`)
	}))
	defer external.Close()
	cm, _ := chatmodel.NewOpenAI(external.URL, "fixture-model", "")
	s, closeServer := startServer(t, filepath.Join(tempDir(t), "state.json"), cm, "REPLAY")
	defer closeServer()
	addRevision(t, s, "GENERAL", "PUBLISH")
	answer := ask(t, s)
	if answer.Status != "UNRESOLVED" || len(answer.KnowledgeToolCalls) != 0 ||
		len(answer.RetrievedFragmentIDs) != 0 || len(answer.Citations) != 0 ||
		len(answer.Gaps) != 1 || answer.Gaps[0] != "缺少实时温度数据。" {
		t.Fatalf("workflow forced retrieval despite model decision: %+v", answer)
	}
}

func TestMainAgentRejectsInvalidToolCallsAndBoundsLoops(t *testing.T) {
	for _, scenario := range []string{"unknown", "forged-device", "empty", "trailing-json", "duplicate", "duplicate-id", "budget", "batch-budget", "uncited"} {
		t.Run(scenario, func(t *testing.T) {
			var requests atomic.Int32
			external := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				n := requests.Add(1)
				_ = readAgentRequest(t, r)
				call := knowledgeCall(fmt.Sprintf("call-%d", n), "蓝灯表示什么？")
				switch scenario {
				case "unknown":
					call.Function.Name = "execute_shell"
				case "forged-device":
					call.Function.Arguments = `{"request":"蓝灯","device_id":"forged"}`
				case "empty":
					call.Function.Arguments = `{"request":"  "}`
				case "trailing-json":
					call.Function.Arguments += `{}`
				case "budget":
					call = knowledgeCall(fmt.Sprintf("call-%d", n), fmt.Sprintf("nomatchquery%d", n))
				case "batch-budget":
					modelReply(w, "", knowledgeCall("a", "first"), knowledgeCall("b", "second"),
						knowledgeCall("c", "third"), knowledgeCall("d", "fourth"))
					return
				case "duplicate-id":
					call = knowledgeCall("same-id", fmt.Sprintf("nomatchquery%d", n))
				case "uncited":
					modelReply(w, `{"claims":[{"text":"伪造知识","fragment_ids":["invented"]}],"gaps":[]}`)
					return
				}
				modelReply(w, "", call)
			}))
			defer external.Close()
			cm, _ := chatmodel.NewOpenAI(external.URL, "fixture-model", "")
			s, closeServer := startServer(t, filepath.Join(tempDir(t), "state.json"), cm, "REPLAY")
			defer closeServer()
			addRevision(t, s, "GENERAL", "PUBLISH")
			answer := ask(t, s)
			code, modelCalls := "INVALID_MODEL_OUTPUT", int32(1)
			switch scenario {
			case "duplicate":
				code, modelCalls = "TOOL_BUDGET_EXCEEDED", 2
			case "budget":
				code, modelCalls = "TOOL_BUDGET_EXCEEDED", 4
			case "batch-budget":
				code = "TOOL_BUDGET_EXCEEDED"
			case "duplicate-id":
				modelCalls = 2
			case "uncited":
				modelCalls = 2
			}
			if answer.Status != "FAILED" || answer.Answer != "" || len(answer.Citations) != 0 ||
				answer.Error == nil || answer.Error.Code != code || requests.Load() != modelCalls ||
				answer.ModelUsage == nil || answer.ModelUsage.TotalTokens != int(modelCalls)*10 {
				t.Fatalf("invalid call or unbounded loop accepted: requests=%d answer=%+v", requests.Load(), answer)
			}
			if scenario == "forged-device" && len(answer.RetrievedFragmentIDs) > 0 {
				t.Error("untrusted device argument reached retrieval")
			}
			if scenario == "budget" || scenario == "batch-budget" {
				if len(answer.KnowledgeToolCalls) != 4 || answer.KnowledgeToolCalls[3].Error == nil ||
					answer.KnowledgeToolCalls[3].Error.Code != "TOOL_BUDGET_EXCEEDED" {
					t.Error("rejected tool budget attempt was not recorded")
				}
			}
		})
	}
}

func TestMainAgentDeduplicatesAndBoundsCumulativeEvidence(t *testing.T) {
	for _, citeExcluded := range []bool{false, true} {
		t.Run(fmt.Sprintf("cite-excluded-%t", citeExcluded), func(t *testing.T) {
			var excludedID string
			external := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				input := readAgentRequest(t, r)
				var results []struct {
					chatmodel.ContextInput
					Gaps  []string `json:"gaps"`
					Known []string `json:"previously_returned_fragment_ids"`
				}
				var all []*schema.Document
				seen := map[string]bool{}
				for _, message := range input.Messages {
					if message.Role != schema.Tool {
						continue
					}
					var result struct {
						chatmodel.ContextInput
						Gaps  []string `json:"gaps"`
						Known []string `json:"previously_returned_fragment_ids"`
					}
					if err := json.Unmarshal([]byte(message.Content), &result); err != nil {
						t.Error(err)
					}
					results = append(results, result)
					for _, doc := range result.Documents {
						if seen[doc.ID] {
							t.Error("duplicate document was injected into the model conversation")
						}
						seen[doc.ID] = true
						all = append(all, doc)
					}
				}
				raw, _ := json.Marshal(all)
				if len(raw) > 48*1024 {
					t.Errorf("cumulative evidence exceeded budget: %d", len(raw))
				}
				switch len(results) {
				case 0:
					modelReply(w, "", knowledgeCall("first", "红灯"))
				case 1:
					if len(results[0].Documents) != 1 {
						t.Error("first retrieval must return one document")
					}
					modelReply(w, "", knowledgeCall("second", "温度"))
				case 2:
					if len(results[1].Documents) != 1 || len(results[1].Known) != 1 {
						t.Error("second retrieval must return new evidence and reference earlier evidence by ID")
					}
					modelReply(w, "", knowledgeCall("third", "风扇"))
				case 3:
					if len(all) != 2 || len(results[2].Documents) != 0 ||
						!strings.Contains(strings.Join(results[2].Gaps, " "), "48 KiB") {
						t.Error("third retrieval did not enforce the cumulative budget")
					}
					if len(all) == 0 {
						http.Error(w, "missing evidence", 400)
						return
					}
					id := all[0].ID
					if citeExcluded {
						id = excludedID
					}
					content, _ := json.Marshal(domain.Draft{Claims: []domain.Claim{{
						Text: "合成证据结论。", FragmentIDs: []string{id},
					}}})
					modelReply(w, string(content))
				default:
					http.Error(w, "unexpected loop", 400)
				}
			}))
			defer external.Close()
			cm, _ := chatmodel.NewOpenAI(external.URL, "fixture-model", "")
			path := filepath.Join(tempDir(t), "state.json")
			s, closeServer := startServer(t, path, cm, "REPLAY")
			answer := func() domain.Response {
				defer closeServer()
				for _, terms := range []string{"红灯 温度", "温度", "风扇"} {
					revision := request(t, s.Client(), "POST", s.URL+"/v1/knowledge/revisions", map[string]any{
						"title": "预算合成资料", "source": "fixture://budget",
						"content":       "# 证据\n" + terms + strings.Repeat(" payload", 1250),
						"applicability": map[string]any{"scope": "GENERAL"},
					}, http.StatusCreated)
					request(t, s.Client(), "POST", s.URL+"/v1/knowledge/revisions/"+revision["id"].(string)+"/publication",
						map[string]any{"decision": "PUBLISH"}, http.StatusOK)
					if terms == "风扇" {
						excludedID = revision["fragments"].([]any)[0].(map[string]any)["id"].(string)
					}
				}
				return ask(t, s)
			}()
			if len(answer.KnowledgeToolCalls) != 3 || len(answer.RetrievedFragmentIDs) != 2 ||
				len(answer.KnowledgeToolCalls[2].FragmentIDs) != 0 || len(answer.KnowledgeToolCalls[2].Gaps) == 0 {
				t.Fatalf("budget or tool records lost: %+v", answer)
			}
			if citeExcluded {
				if answer.Status != "FAILED" || answer.Error == nil || answer.Error.Code != "INVALID_MODEL_OUTPUT" ||
					len(answer.Citations) != 0 {
					t.Fatalf("excluded evidence was allowed as a citation: %+v", answer)
				}
			} else if answer.Status != "ANSWERED" || len(answer.Citations) != 1 {
				t.Fatalf("earlier evidence could not be reused: %+v", answer)
			}
			reopened, closeReopened := startServer(t, path, cm, "REPLAY")
			defer closeReopened()
			persisted := awaitResponse(t, reopened, answer.ID)
			if len(persisted.KnowledgeToolCalls) != 3 || len(persisted.KnowledgeToolCalls[2].Gaps) == 0 ||
				persisted.Status != answer.Status {
				t.Fatal("tool call audit did not survive a store restart")
			}
		})
	}
}

func TestMainAgentFailureAfterToolRetainsEvidenceAndUsage(t *testing.T) {
	external := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		input := readAgentRequest(t, r)
		if len(input.Messages) == 2 {
			modelReply(w, "", knowledgeCall("call-1", "蓝灯表示什么？"))
			return
		}
		if input.Messages[len(input.Messages)-1].Role != schema.Tool {
			t.Error("model did not receive knowledge tool result")
		}
		http.Error(w, "external failure", http.StatusBadRequest)
	}))
	defer external.Close()
	cm, _ := chatmodel.NewOpenAI(external.URL, "fixture-model", "")
	s, closeServer := startServer(t, filepath.Join(tempDir(t), "state.json"), cm, "REPLAY")
	defer closeServer()
	addRevision(t, s, "GENERAL", "PUBLISH")
	answer := ask(t, s)
	if answer.Status != "FAILED" || answer.Error == nil || answer.Error.Code != "MODEL_UNAVAILABLE" ||
		len(answer.KnowledgeToolCalls) != 1 || answer.KnowledgeToolCalls[0].Status != "FOUND" ||
		len(answer.RetrievedFragmentIDs) != 1 || answer.ModelUsage == nil || answer.ModelUsage.TotalTokens != 10 ||
		answer.Answer != "" || strings.Contains(answer.Error.Message, "external failure") {
		t.Fatalf("failure discarded tool evidence or leaked upstream body: %+v", answer)
	}
}
