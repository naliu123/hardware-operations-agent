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
	"hwops/internal/domain"
)

func TestToolLimitFinalizesThroughPublicHTTP(t *testing.T) {
	for _, scenario := range []string{"two-plus-two", "duplicate", "no-evidence", "batch-four", "complete", "ignores-stop", "forged-citation", "model-failure", "invalid-over-budget"} {
		t.Run(scenario, func(t *testing.T) {
			var requests atomic.Int32
			var stopped atomic.Bool
			external := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				n := requests.Add(1)
				input := readAgentRequest(t, r)
				docs := []*schema.Document{}
				pending := map[string]bool{}
				for _, message := range input.Messages {
					for _, call := range message.ToolCalls {
						pending[call.ID] = true
					}
					if message.Role == schema.Tool {
						if !pending[message.ToolCallID] {
							t.Error("orphan or duplicate tool result")
						}
						delete(pending, message.ToolCallID)
						var evidence chatmodel.ContextInput
						_ = json.Unmarshal([]byte(message.Content), &evidence)
						docs = append(docs, evidence.Documents...)
					}
				}
				if len(pending) != 0 {
					t.Error("finalization received unanswered tool calls")
				}
				if input.ToolChoice == "none" {
					stopped.Store(true)
					if scenario == "ignores-stop" {
						modelReply(w, "", knowledgeCall("forbidden", "more"))
						return
					}
					if scenario == "model-failure" {
						http.Error(w, "external failure", 400)
						return
					}
					draft := domain.Draft{Claims: []domain.Claim{}, Gaps: []string{"尚未找到拆装步骤，请提供设备型号和对应手册。"}}
					if scenario == "complete" {
						draft.Gaps = nil
					}
					if len(docs) > 0 {
						id := docs[0].ID
						if scenario == "forged-citation" {
							id = "never-returned"
						}
						draft.Claims = append(draft.Claims, domain.Claim{Text: docs[0].Content, FragmentIDs: []string{id}})
					}
					raw, _ := json.Marshal(draft)
					modelReply(w, string(raw))
					return
				}
				if n > 4 {
					http.Error(w, "unbounded tool loop", 400)
					return
				}
				switch scenario {
				case "invalid-over-budget":
					bad := knowledgeCall("d", "拆装步骤")
					bad.Function.Arguments = `{"request":"拆装","device_id":"forged"}`
					modelReply(w, "", knowledgeCall("a", "蓝灯"), knowledgeCall("b", "蓝灯含义"),
						knowledgeCall("c", "维护模式"), bad)
				case "batch-four":
					modelReply(w, "", knowledgeCall("a", "蓝灯"), knowledgeCall("b", "蓝灯含义"),
						knowledgeCall("c", "维护模式"), knowledgeCall("d", "拆装步骤"))
				case "duplicate":
					modelReply(w, "", knowledgeCall(fmt.Sprintf("call-%d", n), "蓝灯"))
				case "no-evidence":
					modelReply(w, "", knowledgeCall(fmt.Sprintf("call-%d", n), fmt.Sprintf("zxqvnomatch%d", n)))
				default:
					if n == 1 {
						modelReply(w, "", knowledgeCall("a", "蓝灯"), knowledgeCall("b", "蓝灯含义"))
					} else {
						modelReply(w, "", knowledgeCall("c", "维护模式"), knowledgeCall("d", "拆装步骤"))
					}
				}
			}))
			defer external.Close()
			cm, _ := chatmodel.NewOpenAI(external.URL, "fixture-model", "")
			path := filepath.Join(tempDir(t), "state.json")
			server, stop := startServer(t, path, cm, "REPLAY")
			addRevision(t, server, "GENERAL", "PUBLISH")
			answer := ask(t, server)
			stop()
			if !stopped.Load() && scenario != "invalid-over-budget" {
				t.Fatalf("tool limit aborted instead of requesting a tool-free final answer: status=%s error=%+v", answer.Status, answer.Error)
			}
			var executed, rejected int
			for _, call := range answer.KnowledgeToolCalls {
				if call.Error == nil {
					executed++
				} else {
					rejected++
					if call.Request == "" && call.Error.Code != "INVALID_TOOL_CALL" {
						t.Error("rejected query lost its actual arguments")
					}
				}
			}
			if executed > 3 {
				t.Fatal("tool execution exceeded the three-call budget")
			}
			if scenario == "ignores-stop" || scenario == "forged-citation" || scenario == "model-failure" || scenario == "invalid-over-budget" {
				code := "INVALID_MODEL_OUTPUT"
				if scenario == "model-failure" {
					code = "MODEL_UNAVAILABLE"
				}
				if answer.Status != "FAILED" || answer.Error == nil || answer.Error.Code != code || answer.Answer != "" {
					t.Fatalf("invalid finalization was published: %+v", answer)
				}
			} else {
				status := "PARTIAL"
				if scenario == "no-evidence" {
					status = "UNRESOLVED"
				}
				gaps := 1
				if scenario == "complete" {
					status, gaps = "ANSWERED", 0
				}
				if answer.Status != status || answer.Error != nil || len(answer.Gaps) != gaps ||
					answer.DataMode != "REPLAY" || requests.Load() > 4 {
					t.Fatalf("tool limit did not preserve a bounded useful response: %+v", answer)
				}
				if status == "PARTIAL" && (len(answer.Citations) != 1 || !strings.Contains(answer.Answer, "维护模式")) {
					t.Fatalf("collected evidence lost during finalization: %+v", answer)
				}
				if scenario != "no-evidence" && rejected == 0 {
					t.Error("refused call was not retained")
				}
			}
			reopened, closeReopened := startServer(t, path, cm, "REPLAY")
			defer closeReopened()
			saved := awaitResponse(t, reopened, answer.ID)
			successfulModels := int(requests.Load())
			if scenario == "model-failure" {
				successfulModels--
			}
			if saved.Status != answer.Status || len(saved.KnowledgeToolCalls) != len(answer.KnowledgeToolCalls) ||
				saved.ModelUsage == nil || saved.ModelUsage.TotalTokens != successfulModels*10 {
				t.Fatal("final response or usage did not survive restart")
			}
		})
	}
}
