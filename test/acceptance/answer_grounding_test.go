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

	"github.com/cloudwego/eino/schema"

	"hwops/internal/adapters/chatmodel"
	"hwops/internal/adapters/filestore"
	"hwops/internal/adapters/monitor"
	"hwops/internal/application"
	"hwops/internal/domain"
	"hwops/internal/transport/httpapi"
)

func TestOnlineGroundingRepairsUnsupportedAnswerWithRealCitation(t *testing.T) {
	var corrected atomic.Bool
	generator := httptest.NewServer(replayKnowledgeChoice(func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			Messages []*schema.Message `json:"messages"`
		}
		_ = json.NewDecoder(r.Body).Decode(&input)
		var evidence chatmodel.ContextInput
		_ = json.Unmarshal([]byte(input.Messages[1].Content), &evidence)
		text := "蓝灯表示故障模式。"
		if strings.Contains(input.Messages[len(input.Messages)-1].Content, "UNSUPPORTED") {
			corrected.Store(true)
			text = "蓝灯表示维护模式。"
		}
		raw, _ := json.Marshal(domain.Draft{Claims: []domain.Claim{{
			Text: text, FragmentIDs: []string{evidence.Documents[0].ID},
		}}})
		modelReply(w, string(raw))
	}))
	defer generator.Close()
	reviewer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			Messages []*schema.Message `json:"messages"`
			Tools    []any             `json:"tools"`
			Stream   bool              `json:"stream"`
		}
		if json.NewDecoder(r.Body).Decode(&input) != nil || len(input.Messages) != 2 ||
			len(input.Tools) != 0 || input.Stream {
			http.Error(w, "isolated review required", 400)
			return
		}
		var payload struct {
			Claims   []domain.Claim `json:"claims"`
			Evidence []struct {
				ID        string `json:"id"`
				Content   string `json:"content"`
				StartLine int    `json:"start_line"`
				EndLine   int    `json:"end_line"`
			} `json:"evidence"`
		}
		if json.Unmarshal([]byte(input.Messages[1].Content), &payload) != nil ||
			len(payload.Claims) != 1 || len(payload.Evidence) != 1 {
			http.Error(w, "missing evidence", 400)
			return
		}
		evidence := payload.Evidence[0]
		if evidence.Content != "测试设备的蓝灯表示维护模式。本资料为合成测试内容。" {
			t.Error("review did not receive authoritative source")
		}
		verdict, reason := "UNSUPPORTED", "原文说明维护模式，不支持故障模式。"
		supports := []any{}
		if payload.Claims[0].Text == "蓝灯表示维护模式。" {
			verdict, reason = "SUPPORTED", "原文直接支持维护模式。"
			supports = append(supports, map[string]any{"evidence_id": evidence.ID,
				"start_line": evidence.StartLine, "end_line": evidence.EndLine, "quote": evidence.Content})
		}
		raw, _ := json.Marshal(map[string]any{"claims": []any{map[string]any{
			"claim_index": 0, "verdict": verdict, "reason": reason, "supports": supports,
		}}})
		modelReply(w, string(raw))
	}))
	defer reviewer.Close()
	cm, _ := chatmodel.NewOpenAI(generator.URL, "generator-fixture", "")
	review, _ := chatmodel.NewOpenAI(reviewer.URL, "review-fixture", "")
	path := filepath.Join(tempDir(t), "grounding.json")
	store, err := filestore.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	app, err := application.New(store, cm, "REPLAY", application.Options{GroundingModel: review})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(httpapi.New(app, token))
	defer func() { server.Close(); app.Close(); store.Close() }()
	addRevision(t, server, "GENERAL", "PUBLISH")
	answer := ask(t, server)
	if !corrected.Load() || answer.Status != "ANSWERED" || answer.Answer != "蓝灯表示维护模式。" {
		t.Fatalf("unsupported claim with real citation was published: %+v", answer)
	}
	raw := request(t, server.Client(), "GET", server.URL+"/v1/responses/"+answer.ID, nil, 200)
	reviews, ok := raw["grounding_reviews"].([]any)
	if !ok || len(reviews) != 2 {
		t.Fatalf("review audit missing: %v", raw)
	}
	source := request(t, server.Client(), "GET", server.URL+answer.Citations[0].URL, nil, 200)
	if source["content_hash"] != answer.Citations[0].ContentHash {
		t.Fatal("grounded citation cannot be retrieved")
	}
	t.Log("REPLAY: wrong assertion with a real citation was corrected and reviewed before publication")
}

type groundingPayload struct {
	Question string         `json:"question"`
	Claims   []domain.Claim `json:"claims"`
	Evidence []struct {
		ID        string `json:"id"`
		Content   string `json:"content"`
		StartLine int    `json:"start_line"`
		EndLine   int    `json:"end_line"`
	} `json:"evidence"`
}

func groundingResponse(p groundingPayload, index int, verdict string) domain.GroundingVerdict {
	result := domain.GroundingVerdict{ClaimIndex: index, Verdict: verdict,
		Reason: "原文只说明蓝灯为维护模式，无法支持故障模式。", Supports: []domain.GroundingSupport{}}
	if verdict == "SUPPORTED" {
		for _, source := range p.Evidence {
			for _, id := range p.Claims[index].FragmentIDs {
				if id == source.ID {
					result.Supports = append(result.Supports, domain.GroundingSupport{
						EvidenceID: source.ID, StartLine: source.StartLine, EndLine: source.EndLine, Quote: source.Content,
					})
				}
			}
		}
	}
	return result
}

func groundingHarness(t *testing.T, text []string, reviewHandler http.HandlerFunc) (*httptest.Server, string, func()) {
	t.Helper()
	generator := httptest.NewServer(replayKnowledgeChoice(func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			Messages []*schema.Message `json:"messages"`
		}
		_ = json.NewDecoder(r.Body).Decode(&input)
		var context chatmodel.ContextInput
		_ = json.Unmarshal([]byte(input.Messages[1].Content), &context)
		claims := []domain.Claim{}
		for _, value := range text {
			claims = append(claims, domain.Claim{Text: value, FragmentIDs: []string{context.Documents[0].ID}})
		}
		raw, _ := json.Marshal(domain.Draft{Claims: claims})
		modelReply(w, string(raw))
	}))
	t.Cleanup(generator.Close)
	reviewer := httptest.NewServer(reviewHandler)
	t.Cleanup(reviewer.Close)
	cm, _ := chatmodel.NewOpenAI(generator.URL, "generator-fixture", "")
	review, _ := chatmodel.NewOpenAI(reviewer.URL, "review-fixture", "")
	path := filepath.Join(tempDir(t), "grounding.json")
	store, err := filestore.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	app, err := application.New(store, cm, "REPLAY", application.Options{GroundingModel: review})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(httpapi.New(app, token))
	return server, path, func() { server.Close(); app.Close(); store.Close() }
}

func TestOnlineGroundingKeepsIndependentSupportedClaimsAndAuditAfterRestart(t *testing.T) {
	s, path, stop := groundingHarness(t, []string{"蓝灯表示维护模式。", "蓝灯一定表示故障模式。"},
		func(w http.ResponseWriter, r *http.Request) {
			var input struct {
				Messages []*schema.Message `json:"messages"`
			}
			_ = json.NewDecoder(r.Body).Decode(&input)
			var payload groundingPayload
			_ = json.Unmarshal([]byte(input.Messages[1].Content), &payload)
			raw, _ := json.Marshal(map[string]any{"claims": []domain.GroundingVerdict{
				groundingResponse(payload, 0, "SUPPORTED"), groundingResponse(payload, 1, "CONTRADICTED"),
			}})
			modelReply(w, string(raw))
		})
	addRevision(t, s, "GENERAL", "PUBLISH")
	answer := ask(t, s)
	if answer.Status != "PARTIAL" || answer.Answer != "蓝灯表示维护模式。" || len(answer.Claims) != 1 ||
		len(answer.Gaps) == 0 || len(answer.GroundingReviews) != 2 {
		t.Fatalf("unsupported content survived grounding: %+v", answer)
	}
	stop()
	s, stop = startServer(t, path, &chatmodel.Replay{}, "REPLAY")
	defer stop()
	persisted := awaitResponse(t, s, answer.ID)
	before, _ := json.Marshal(answer.GroundingReviews)
	after, _ := json.Marshal(persisted.GroundingReviews)
	if string(before) != string(after) || persisted.Answer != answer.Answer {
		t.Fatal("grounding audit was lost after restart")
	}
	for _, review := range persisted.GroundingReviews {
		if review.Version != "answer-grounding-v1" || review.Model != "review-fixture" ||
			len(review.ClaimsSHA256) != 64 || len(review.InputSHA256) != 64 || review.Status != "VERIFIED" {
			t.Fatalf("review not bound to immutable input: %+v", review)
		}
	}
}

func TestOnlineGroundingRejectsMalformedOrInventedReviewEvidence(t *testing.T) {
	for _, scenario := range []string{"quote", "range", "source", "missing_index", "missing_verdict",
		"missing_review", "duplicate", "unknown_field", "tool_call", "service_failure", "gateway_timeout", "incomplete", "empty"} {
		t.Run(scenario, func(t *testing.T) {
			s, _, stop := groundingHarness(t, []string{"蓝灯表示维护模式。"},
				func(w http.ResponseWriter, r *http.Request) {
					var input struct {
						Messages []*schema.Message `json:"messages"`
					}
					_ = json.NewDecoder(r.Body).Decode(&input)
					var payload groundingPayload
					_ = json.Unmarshal([]byte(input.Messages[1].Content), &payload)
					verdict := groundingResponse(payload, 0, "SUPPORTED")
					raw, _ := json.Marshal(verdict)
					var value map[string]any
					_ = json.Unmarshal(raw, &value)
					switch scenario {
					case "quote":
						verdict.Supports[0].Quote = "伪造的原文"
						raw, _ = json.Marshal(verdict)
					case "range":
						verdict.Supports[0].EndLine++
						raw, _ = json.Marshal(verdict)
					case "source":
						verdict.Supports[0].EvidenceID = "invented"
						raw, _ = json.Marshal(verdict)
					case "missing_index":
						delete(value, "claim_index")
						raw, _ = json.Marshal(value)
					case "missing_verdict":
						delete(value, "verdict")
						raw, _ = json.Marshal(value)
					case "unknown_field":
						value["confidence"] = 1
						raw, _ = json.Marshal(value)
					case "service_failure":
						http.Error(w, "review unavailable", http.StatusBadRequest)
						return
					case "gateway_timeout":
						w.Header().Set("Retry-After", "0")
						http.Error(w, "review timed out", http.StatusGatewayTimeout)
						return
					case "tool_call":
						modelReply(w, `{"claims":[]}`, knowledgeCall("forged-review-tool", "蓝灯"))
						return
					case "incomplete", "empty":
						content := `{"claims":[` + string(raw) + `]}`
						if scenario == "empty" {
							content = ""
						}
						_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{
							"message": map[string]any{"content": content}, "finish_reason": "length",
						}}, "usage": map[string]int{"prompt_tokens": 6, "completion_tokens": 4, "total_tokens": 10}})
						return
					}
					content := `{"claims":[` + string(raw) + `]}`
					if scenario == "missing_review" {
						content = `{"claims":[]}`
					}
					if scenario == "duplicate" {
						content = `{"claims":[` + string(raw) + `,` + string(raw) + `]}`
					}
					modelReply(w, content)
				})
			defer stop()
			addRevision(t, s, "GENERAL", "PUBLISH")
			answer := ask(t, s)
			code := "GROUNDING_REVIEW_FAILED"
			if scenario == "gateway_timeout" {
				code = "GROUNDING_REVIEW_TIMEOUT"
			}
			if answer.Status != "FAILED" || answer.Answer != "" || len(answer.Citations) != 0 ||
				answer.Error == nil || answer.Error.Code != code ||
				len(answer.GroundingReviews) != 1 || answer.GroundingReviews[0].Status != "FAILED" {
				t.Fatalf("invalid semantic review published an answer: %+v", answer)
			}
			if scenario != "service_failure" && scenario != "gateway_timeout" && answer.GroundingReviews[0].ModelUsage == nil {
				t.Fatal("consumed review usage was discarded")
			}
		})
	}
}

func TestOnlineGroundingWithdrawalDuringReviewCannotPublishAnswer(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	defer closeGate(release)
	s, _, stop := groundingHarness(t, []string{"蓝灯表示维护模式。"},
		func(w http.ResponseWriter, r *http.Request) {
			var input struct {
				Messages []*schema.Message `json:"messages"`
			}
			_ = json.NewDecoder(r.Body).Decode(&input)
			var payload groundingPayload
			_ = json.Unmarshal([]byte(input.Messages[1].Content), &payload)
			close(started)
			select {
			case <-release:
			case <-r.Context().Done():
				return
			}
			raw, _ := json.Marshal(map[string]any{"claims": []domain.GroundingVerdict{
				groundingResponse(payload, 0, "SUPPORTED"),
			}})
			modelReply(w, string(raw))
		})
	defer stop()
	revision := addRevision(t, s, "GENERAL", "PUBLISH")
	cid := request(t, s.Client(), "POST", s.URL+"/v1/conversations", map[string]any{}, 201)["id"].(string)
	id := request(t, s.Client(), "POST", s.URL+"/v1/conversations/"+cid+"/messages",
		map[string]string{"text": "蓝灯含义"}, 202)["id"].(string)
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("review did not start")
	}
	request(t, s.Client(), "POST", s.URL+"/v1/knowledge/revisions/"+revision+"/publication",
		map[string]string{"decision": "WITHDRAW"}, 200)
	closeGate(release)
	answer := awaitResponse(t, s, id)
	if answer.Status != "UNRESOLVED" || answer.Answer != "" || len(answer.Citations) != 0 {
		t.Fatalf("source withdrawn during review still produced guidance: %+v", answer)
	}
}

func TestOnlineGroundingAllUnsupportedAnswersReturnSpecificGap(t *testing.T) {
	s, _, stop := groundingHarness(t, []string{"蓝灯表示故障模式。"},
		func(w http.ResponseWriter, r *http.Request) {
			var input struct {
				Messages []*schema.Message `json:"messages"`
			}
			_ = json.NewDecoder(r.Body).Decode(&input)
			var payload groundingPayload
			_ = json.Unmarshal([]byte(input.Messages[1].Content), &payload)
			raw, _ := json.Marshal(map[string]any{"claims": []domain.GroundingVerdict{
				groundingResponse(payload, 0, "UNSUPPORTED"),
			}})
			modelReply(w, string(raw))
		})
	defer stop()
	addRevision(t, s, "GENERAL", "PUBLISH")
	answer := ask(t, s)
	if answer.Status != "UNRESOLVED" || answer.Answer != "" || len(answer.Claims) != 0 ||
		len(answer.Citations) != 0 || len(answer.GroundingReviews) != 2 || len(answer.Gaps) == 0 ||
		!strings.Contains(strings.Join(answer.Gaps, " "), "故障模式") {
		t.Fatalf("unsupported answer was not reduced to a specific gap: %+v", answer)
	}
}

func TestOnlineGroundingInputBudgetFailsBeforeReviewDispatch(t *testing.T) {
	var dispatched atomic.Bool
	claims := make([]string, 20)
	for i := range claims {
		claims[i] = strings.Repeat("蓝灯表示维护模式。", 550)
	}
	s, _, stop := groundingHarness(t, claims, func(w http.ResponseWriter, r *http.Request) {
		dispatched.Store(true)
		http.Error(w, "budget should reject before dispatch", 400)
	})
	defer stop()
	addRevision(t, s, "GENERAL", "PUBLISH")
	answer := ask(t, s)
	if dispatched.Load() || answer.Status != "FAILED" || answer.Answer != "" ||
		answer.Error == nil || answer.Error.Code != "MODEL_BUDGET_EXCEEDED" ||
		len(answer.GroundingReviews) != 1 || answer.GroundingReviews[0].ModelUsage != nil {
		t.Fatalf("review input budget: dispatched=%v status=%s error=%+v reviews=%d",
			dispatched.Load(), answer.Status, answer.Error, len(answer.GroundingReviews))
	}
}

func TestOnlineGroundingManyRejectedClaimsKeepGapsBounded(t *testing.T) {
	texts := make([]string, 20)
	for i := range texts {
		texts[i] = "蓝灯表示故障模式。"
	}
	s, _, stop := groundingHarness(t, texts, func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			Messages []*schema.Message `json:"messages"`
		}
		_ = json.NewDecoder(r.Body).Decode(&input)
		var p groundingPayload
		_ = json.Unmarshal([]byte(input.Messages[1].Content), &p)
		verdicts := []domain.GroundingVerdict{}
		for i := range p.Claims {
			verdict := groundingResponse(p, i, "UNSUPPORTED")
			verdict.Reason = "故障模式缺少原文依据。" + strings.Repeat("x", 1940)
			verdicts = append(verdicts, verdict)
		}
		raw, _ := json.Marshal(map[string]any{"claims": verdicts})
		modelReply(w, string(raw))
	})
	defer stop()
	addRevision(t, s, "GENERAL", "PUBLISH")
	answer := ask(t, s)
	if answer.Status != "UNRESOLVED" || len(answer.Gaps) < 1 || len(answer.Gaps) > 12 {
		t.Fatalf("grounding gaps exceed contract: status=%s gaps=%d", answer.Status, len(answer.Gaps))
	}
	if !strings.Contains(strings.Join(answer.Gaps, "\n"), "故障模式缺少原文依据") {
		t.Fatal("bounded gaps lost the specific unsupported assertion")
	}
	for _, gap := range answer.Gaps {
		if len(gap) > 2000 {
			t.Fatalf("grounding gap exceeds byte limit: %d", len(gap))
		}
	}
}

func TestOnlineGroundingRejectedFactCannotEscapeThroughClarificationReply(t *testing.T) {
	generator := httptest.NewServer(replayKnowledgeChoice(func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			Messages []*schema.Message `json:"messages"`
		}
		_ = json.NewDecoder(r.Body).Decode(&input)
		if strings.Contains(input.Messages[len(input.Messages)-1].Content, "UNSUPPORTED") {
			modelReply(w, `{"reply":{"kind":"CLARIFICATION","text":"蓝灯表示故障模式。"},"claims":[]}`)
			return
		}
		var p chatmodel.ContextInput
		_ = json.Unmarshal([]byte(input.Messages[1].Content), &p)
		raw, _ := json.Marshal(domain.Draft{Claims: []domain.Claim{{
			Text: "蓝灯表示故障模式。", FragmentIDs: []string{p.Documents[0].ID},
		}}})
		modelReply(w, string(raw))
	}))
	defer generator.Close()
	reviewer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		modelReply(w, `{"claims":[{"claim_index":0,"verdict":"UNSUPPORTED","reason":"原文是维护模式。","supports":[]}]}`)
	}))
	defer reviewer.Close()
	cm, _ := chatmodel.NewOpenAI(generator.URL, "generator-fixture", "")
	review, _ := chatmodel.NewOpenAI(reviewer.URL, "review-fixture", "")
	store, err := filestore.Open(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	app, err := application.New(store, cm, "REPLAY", application.Options{GroundingModel: review})
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close()
	server := httptest.NewServer(httpapi.New(app, token))
	defer server.Close()
	addRevision(t, server, "GENERAL", "PUBLISH")
	answer := ask(t, server)
	if answer.Status != "FAILED" || answer.Answer != "" || answer.Reply != nil || len(answer.GroundingReviews) != 1 {
		t.Fatalf("clarification reply bypassed grounding: status=%s answer=%q", answer.Status, answer.Answer)
	}
}

func TestOnlineGroundingAndObservationGapsStayWithinFinalLimits(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var q domain.ObservationQuery
		_ = json.NewDecoder(r.Body).Decode(&q)
		result := monitorData(q)
		result.Status, result.Values = "FAILED", nil
		result.Error = &domain.Failure{Code: "MONITOR_FAILED", Message: "测试监控失败。"}
		_ = json.NewEncoder(w).Encode(result)
	}))
	defer upstream.Close()
	observer, _ := monitor.New(upstream.URL, "", "REPLAY", time.Second, 1)
	generator := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		input := readAgentRequest(t, r)
		var tools []*schema.Message
		for _, message := range input.Messages {
			if message.Role == schema.Tool {
				tools = append(tools, message)
			}
		}
		switch len(tools) {
		case 0:
			modelReply(w, "", knowledgeCall("knowledge", "蓝灯含义"))
		case 1:
			raw, _ := json.Marshal(observationRequest())
			modelReply(w, "", schema.ToolCall{ID: "observation", Type: "function",
				Function: schema.FunctionCall{Name: "observe_device", Arguments: string(raw)}})
		default:
			var p chatmodel.ContextInput
			_ = json.Unmarshal([]byte(tools[0].Content), &p)
			gaps := make([]string, 11)
			for i := range gaps {
				gaps[i] = "测试缺口"
			}
			raw, _ := json.Marshal(domain.Draft{Claims: []domain.Claim{{
				Text: "蓝灯表示故障模式。", FragmentIDs: []string{p.Documents[0].ID},
			}}, Gaps: gaps})
			modelReply(w, string(raw))
		}
	}))
	defer generator.Close()
	reviewer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		modelReply(w, `{"claims":[{"claim_index":0,"verdict":"UNSUPPORTED","reason":"手册只支持维护模式。","supports":[]}]}`)
	}))
	defer reviewer.Close()
	cm, _ := chatmodel.NewOpenAI(generator.URL, "combined-fixture", "")
	review, _ := chatmodel.NewOpenAI(reviewer.URL, "review-fixture", "")
	store, err := filestore.Open(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	app, err := application.New(store, cm, "REPLAY", application.Options{GroundingModel: review, Observer: observer})
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close()
	server := httptest.NewServer(httpapi.New(app, token))
	defer server.Close()
	addRevision(t, server, "GENERAL", "PUBLISH")
	putObservedDevice(t, server, "target")
	cid := request(t, server.Client(), "POST", server.URL+"/v1/conversations", map[string]any{}, 201)["id"].(string)
	id := request(t, server.Client(), "POST", server.URL+"/v1/conversations/"+cid+"/messages",
		map[string]string{"text": "蓝灯含义与转速", "device_id": "target"}, 202)["id"].(string)
	answer := awaitResponse(t, server, id)
	if answer.Status != "UNRESOLVED" || len(answer.Gaps) > 12 || len(answer.Gaps) < 1 || len(answer.GroundingReviews) != 2 {
		t.Fatalf("combined final gaps not bounded: status=%s gaps=%d", answer.Status, len(answer.Gaps))
	}
	for _, gap := range answer.Gaps {
		if len(gap) > 2000 {
			t.Fatal("combined gap too long")
		}
	}
}
