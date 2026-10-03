package acceptance_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"hwops/internal/adapters/chatmodel"
	"hwops/internal/domain"
)

func TestKnowledgeConflictWithholdsAffectedAdviceAndPreservesSources(t *testing.T) {
	for _, forged := range []bool{false, true} {
		t.Run(map[bool]string{false: "conflict", true: "forged_source"}[forged], func(t *testing.T) {
			external := httptest.NewServer(replayKnowledgeChoice(func(w http.ResponseWriter, r *http.Request) {
				input := readAgentRequest(t, r)
				var payload chatmodel.ContextInput
				_ = json.Unmarshal([]byte(input.Messages[1].Content), &payload)
				if len(payload.Documents) != 2 {
					t.Errorf("expected both conflicting sources: %v", payload.Documents)
					http.Error(w, "missing sources", 400)
					return
				}
				ids := []string{payload.Documents[0].ID, payload.Documents[1].ID}
				if forged {
					ids[1] = "invented"
				}
				raw, _ := json.Marshal(domain.Draft{
					Claims: []domain.Claim{{Text: "立即重启设备。", FragmentIDs: ids[:1]}},
					Conflicts: []domain.KnowledgeConflict{{
						Subject: "同型号版本的E42资料分别要求立即重启与禁止重启", FragmentIDs: ids,
					}},
				})
				modelReply(w, string(raw))
			}))
			defer external.Close()
			cm, err := chatmodel.NewOpenAI(external.URL, "fixture", "")
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(tempDir(t), "state.json")
			s, stop := startServer(t, path, cm, "REPLAY")
			for _, content := range []string{"E42 蓝灯故障要求立即重启。", "E42 蓝灯故障禁止重启。"} {
				publishFor(t, s, map[string]any{"scope": "GENERAL"}, content)
			}
			answer := ask(t, s)
			stop()
			if forged {
				if answer.Status != "FAILED" || len(answer.Citations) != 0 || len(answer.Conflicts) != 0 {
					t.Fatalf("forged conflict accepted: %+v", answer)
				}
				return
			}
			if answer.Status != "UNRESOLVED" || answer.Answer != "" || len(answer.Claims) != 0 ||
				len(answer.Conflicts) != 1 || len(answer.Citations) != 2 || len(answer.Gaps) == 0 {
				t.Fatalf("conflicting advice was not blocked: %+v", answer)
			}
			s, stop = startServer(t, path, cm, "REPLAY")
			defer stop()
			saved := awaitResponse(t, s, answer.ID)
			if len(saved.Conflicts) != 1 || len(saved.Citations) != 2 {
				t.Fatal("conflict provenance lost on restart")
			}
			for _, citation := range saved.Citations {
				request(t, s.Client(), "GET", s.URL+citation.URL, nil, http.StatusOK)
			}
		})
	}
}

func TestPublicSearchExplainsMissingEvidenceAndCorpusGeneration(t *testing.T) {
	s, stop := startServer(t, filepath.Join(tempDir(t), "state.json"), &chatmodel.Replay{}, "REPLAY")
	defer stop()
	search := func(query, device string) map[string]any {
		return request(t, s.Client(), "POST", s.URL+"/v1/knowledge/search",
			map[string]any{"query": query, "device_id": device}, http.StatusOK)
	}
	empty := search("E42", "")
	if empty["gap_reason"] != "NO_PUBLISHED_KNOWLEDGE" {
		t.Fatalf("empty corpus not diagnosed: %v", empty)
	}
	putDevice(t, s, "target", "fixture/Atlas", "R2")
	publishFor(t, s, map[string]any{"scope": "DEVICE", "model": "fixture/Atlas", "firmware": "R10"}, "E42 表示风扇阻塞。")
	if got := search("E42", ""); got["gap_reason"] != "UNKNOWN_APPLICABILITY" {
		t.Fatalf("unknown context not diagnosed: %v", got)
	}
	if got := search("E42", "target"); got["gap_reason"] != "VERSION_MISMATCH" {
		t.Fatalf("version mismatch not diagnosed: %v", got)
	}
	publishFor(t, s, map[string]any{"scope": "DEVICE", "model": "fixture/Atlas", "firmware": "R2"}, "E42 表示风扇转速低。")
	if got := search("zxqvnomatch", "target"); got["gap_reason"] != "NO_RETRIEVAL_HIT" {
		t.Fatalf("retrieval miss not diagnosed: %v", got)
	}
	matched := search("E42", "target")
	if matched["gap_reason"] != nil || len(matched["documents"].([]any)) != 1 ||
		matched["corpus_generation"] == empty["corpus_generation"] {
		t.Fatalf("error-code match or generation missing: %v", matched)
	}
	raw, _ := json.Marshal(matched["documents"])
	if !strings.Contains(string(raw), "风扇转速低") || strings.Contains(string(raw), "风扇阻塞") {
		t.Fatalf("wrong version evidence: %s", raw)
	}
}
