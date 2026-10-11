package acceptance_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cloudwego/eino/schema"

	"hwops/internal/adapters/chatmodel"
	knowledgeagent "hwops/internal/agents/knowledge"
	"hwops/internal/application"
	"hwops/internal/blobstore"
	"hwops/internal/domain"
	"hwops/internal/einoflow"
)

func TestOnlineGroundingWorkbenchDraftWaitsForReviewAndRetractsOnFailure(t *testing.T) {
	reviewStarted, release := make(chan struct{}), make(chan struct{})
	defer closeGate(release)
	reviewer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(reviewStarted)
		select {
		case <-release:
		case <-r.Context().Done():
			return
		}
		http.Error(w, "review failure", 400)
	}))
	defer reviewer.Close()
	generator := wb05KnowledgeModel(func(w http.ResponseWriter, r *http.Request) {
		_, tool := wb05ModelInput(r)
		var evidence chatmodel.ContextInput
		_ = json.Unmarshal([]byte(tool.Content), &evidence)
		wb06Final(w, domain.Draft{Claims: []domain.Claim{{
			Text: "蓝灯表示维护模式。", FragmentIDs: []string{evidence.Documents[0].ID},
		}}})
	})
	defer generator.Close()
	cm, _ := chatmodel.NewOpenAI(generator.URL, "stream-fixture", "")
	review, _ := chatmodel.NewOpenAI(reviewer.URL, "review-fixture", "")
	server, _, stop := wbStartOptions(t, workbenchDatabase(t), cm, true,
		application.Options{UsersMode: true, GroundingModel: review})
	defer stop()
	client := wbNewClient(t, server)
	client.login("admin", wbPassword)
	wbPublish(client)
	cid := client.request("POST", "/v1/conversations", map[string]any{}, 201)["id"].(string)
	id := client.request("POST", "/v1/conversations/"+cid+"/messages",
		map[string]string{"text": "蓝灯含义"}, 202)["id"].(string)
	stream := client.openEvents(id, 0)
	defer stream.close()
	delta := waitWBEvent(t, stream, "answer_delta")
	select {
	case <-reviewStarted:
	case <-time.After(3 * time.Second):
		t.Fatal("review not started")
	}
	current := client.request("GET", "/v1/responses/"+id, nil, 200)
	if current["status"] != "RUNNING" || current["answer"] != "" {
		t.Fatalf("draft became final before review: %v", current)
	}
	closeGate(release)
	final := client.answer(id)
	if final["status"] != "FAILED" || final["answer"] != "" ||
		final["error"].(map[string]any)["code"] != "GROUNDING_REVIEW_FAILED" {
		t.Fatalf("failed semantic review published draft: %v", final)
	}
	retracted := waitWBEvent(t, stream, "draft_retracted")
	if retracted.DraftVersion != delta.DraftVersion || retracted.Reason == nil ||
		retracted.Reason.Code != "GROUNDING_REVIEW_FAILED" {
		t.Fatalf("semantic failure did not retract draft: %+v", retracted)
	}
	for event := range stream.events {
		if event.Type == "answer" && event.Response != nil && event.Response.Answer != "" {
			t.Fatal("answer event bypassed semantic gate")
		}
	}
	t.Log("REPLAY: real PostgreSQL/SSE kept draft provisional and atomically retracted it on review failure")
}

func TestOnlineGroundingWorkbenchCorrectionAndWithdrawalDuringReview(t *testing.T) {
	for _, withdraw := range []bool{false, true} {
		t.Run(map[bool]string{false: "correction", true: "withdrawal"}[withdraw], func(t *testing.T) {
			started, release := make(chan struct{}), make(chan struct{})
			defer closeGate(release)
			reviewer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var input struct {
					Messages []*schema.Message `json:"messages"`
				}
				_ = json.NewDecoder(r.Body).Decode(&input)
				var p groundingPayload
				_ = json.Unmarshal([]byte(input.Messages[1].Content), &p)
				if withdraw {
					close(started)
					select {
					case <-release:
					case <-r.Context().Done():
						return
					}
				}
				verdict := "SUPPORTED"
				if strings.Contains(p.Claims[0].Text, "故障") {
					verdict = "UNSUPPORTED"
				}
				raw, _ := json.Marshal(map[string]any{"claims": []domain.GroundingVerdict{groundingResponse(p, 0, verdict)}})
				modelReply(w, string(raw))
			}))
			defer reviewer.Close()
			generator := wb05KnowledgeModel(func(w http.ResponseWriter, r *http.Request) {
				messages, tool := wb05ModelInput(r)
				var evidence chatmodel.ContextInput
				_ = json.Unmarshal([]byte(tool.Content), &evidence)
				text := "蓝灯表示故障模式。"
				if withdraw || strings.Contains(messages[len(messages)-1].Content, "UNSUPPORTED") {
					text = "蓝灯表示维护模式。"
				}
				wb06Final(w, domain.Draft{Claims: []domain.Claim{{
					Text: text, FragmentIDs: []string{evidence.Documents[0].ID},
				}}})
			})
			defer generator.Close()
			cm, _ := chatmodel.NewOpenAI(generator.URL, "stream-fixture", "")
			review, _ := chatmodel.NewOpenAI(reviewer.URL, "review-fixture", "")
			server, _, stop := wbStartOptions(t, workbenchDatabase(t), cm, true,
				application.Options{UsersMode: true, GroundingModel: review})
			defer stop()
			client := wbNewClient(t, server)
			client.login("admin", wbPassword)
			revision := wbPublish(client)
			admin := client
			if withdraw {
				admin.request("POST", "/v1/admin/users", map[string]string{
					"username": "alice", "password": wbPassword, "role": "USER",
				}, 201)
				admin.request("POST", "/v1/admin/users", map[string]string{
					"username": "bob", "password": wbPassword, "role": "USER",
				}, 201)
				client = wbNewClient(t, server)
				client.login("alice", wbPassword)
			}
			cid := client.request("POST", "/v1/conversations", map[string]any{}, 201)["id"].(string)
			id := client.request("POST", "/v1/conversations/"+cid+"/messages",
				map[string]string{"text": "蓝灯含义"}, 202)["id"].(string)
			if withdraw {
				select {
				case <-started:
				case <-time.After(3 * time.Second):
					t.Fatal("review did not start")
				}
				admin.request("POST", "/v1/knowledge/revisions/"+revision+"/publication",
					map[string]string{"decision": "WITHDRAW"}, 200)
				closeGate(release)
			}
			final := client.answer(id)
			if withdraw {
				if final["status"] != "UNRESOLVED" || final["answer"] != "" {
					t.Fatalf("withdrawn knowledge passed final save: %v", final)
				}
				audit := final["grounding_reviews"].([]any)[0].(map[string]any)
				fragment := audit["evidence"].([]any)[0].(map[string]any)["id"].(string)
				path := "/v1/knowledge/revisions/" + revision + "/fragments/" + fragment
				client.request("GET", path, nil, 200)
				bob := wbNewClient(t, server)
				bob.login("bob", wbPassword)
				bob.request("GET", path, nil, 404)
			} else if final["status"] != "ANSWERED" || !strings.Contains(final["answer"].(string), "维护模式") {
				t.Fatalf("grounding correction did not publish: %v", final)
			}
			stream := client.openEvents(id, 0)
			var starts, superseded, finalized int
			for event := range stream.events {
				if event.Type == "draft_started" {
					starts++
				}
				if event.Type == "draft_retracted" && event.Reason != nil {
					if event.Reason.Code == "SUPERSEDED" {
						superseded++
					}
					if event.Reason.Code == "FINALIZED" {
						finalized++
					}
				}
			}
			stream.close()
			if finalized != 1 || (!withdraw && (starts != 2 || superseded != 1)) {
				t.Fatalf("grounding draft lifecycle mismatch: starts=%d superseded=%d finalized=%d", starts, superseded, finalized)
			}
		})
	}
}

func TestOnlineGroundingMixedClaimUsesKnowledgeAndPrivateOriginalText(t *testing.T) {
	generator := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			Messages []*schema.Message `json:"messages"`
		}
		_ = json.NewDecoder(r.Body).Decode(&input)
		w.Header().Set("Content-Type", "text/event-stream")
		var p chatmodel.ContextInput
		_ = json.Unmarshal([]byte(input.Messages[1].Content), &p)
		var tools []*schema.Message
		for _, message := range input.Messages {
			if message.Role == schema.Tool {
				tools = append(tools, message)
			}
		}
		switch len(tools) {
		case 0:
			wb06ToolCall(w, "private", einoflow.ReadAttachmentToolName,
				map[string]any{"attachment_id": p.Attachments[0].ID, "page": 1, "start_line": 1, "end_line": 1})
		case 1:
			wb06ToolCall(w, "knowledge", knowledgeagent.ToolName, map[string]string{"request": "蓝灯含义"})
		default:
			var attachment domain.AttachmentReadResult
			var evidence chatmodel.ContextInput
			_ = json.Unmarshal([]byte(tools[0].Content), &attachment)
			_ = json.Unmarshal([]byte(tools[1].Content), &evidence)
			wb06Final(w, domain.Draft{Claims: []domain.Claim{{
				Text:        "手册说明蓝灯表示维护模式；本次日志记录蓝灯亮起。",
				FragmentIDs: []string{evidence.Documents[0].ID}, SourceIDs: []string{attachment.Source.SourceID},
			}}})
		}
	}))
	defer generator.Close()
	reviewer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			Messages []*schema.Message `json:"messages"`
		}
		_ = json.NewDecoder(r.Body).Decode(&input)
		var p groundingPayload
		_ = json.Unmarshal([]byte(input.Messages[1].Content), &p)
		verdict := domain.GroundingVerdict{ClaimIndex: 0, Verdict: "SUPPORTED",
			Reason: "手册说明含义，原始日志记录亮起。", Supports: []domain.GroundingSupport{}}
		foundManual, foundPrivate := false, false
		for _, source := range p.Evidence {
			if strings.HasPrefix(source.ID, "attachment:") {
				foundPrivate = source.Content == "本次日志：蓝灯亮起。"
				if len(p.Claims[0].SourceIDs) != 1 || source.ID != p.Claims[0].SourceIDs[0] {
					t.Error("private source identity changed")
				}
			} else {
				foundManual = strings.Contains(source.Content, "蓝灯表示维护模式")
			}
			verdict.Supports = append(verdict.Supports, domain.GroundingSupport{
				EvidenceID: source.ID, StartLine: source.StartLine, EndLine: source.EndLine, Quote: source.Content,
			})
		}
		if !foundManual || !foundPrivate {
			t.Error("review did not receive both original sources")
		}
		raw, _ := json.Marshal(map[string]any{"claims": []domain.GroundingVerdict{verdict}})
		modelReply(w, string(raw))
	}))
	defer reviewer.Close()
	cm, _ := chatmodel.NewOpenAI(generator.URL, "mixed-fixture", "")
	review, _ := chatmodel.NewOpenAI(reviewer.URL, "review-fixture", "")
	files, err := blobstore.Open(filepath.Join(t.TempDir(), "files"), 500_000_000, 100_000_000)
	if err != nil {
		t.Fatal(err)
	}
	server, _, stop := wbStartOptions(t, workbenchDatabase(t), cm, true, application.Options{
		UsersMode: true, GroundingModel: review, Files: files, AttachmentRunner: newAttachmentReplayRunner(),
	})
	defer stop()
	client := wbNewClient(t, server)
	client.login("admin", wbPassword)
	wbPublish(client)
	cid := client.request("POST", "/v1/conversations", map[string]any{}, 201)["id"].(string)
	uploaded := uploadAttachment(t, client, cid, "original.log", "text/plain", []byte("本次日志：蓝灯亮起。\n"), 201)
	waitAttachment(t, client, uploaded["id"].(string))
	id := client.request("POST", "/v1/conversations/"+cid+"/messages",
		map[string]any{"text": "结合日志与手册说明蓝灯含义", "attachment_ids": []string{uploaded["id"].(string)}}, 202)["id"].(string)
	final := client.answer(id)
	if final["status"] != "ANSWERED" || len(final["citations"].([]any)) != 1 ||
		len(final["sources"].([]any)) != 1 || len(final["grounding_reviews"].([]any)) != 1 {
		t.Fatalf("mixed claim lost grounded provenance: %v", final)
	}
}

func TestOnlineGroundingCompletedReviewSurvivesRestartDuringCorrection(t *testing.T) {
	correcting := make(chan struct{})
	generator := wb05KnowledgeModel(func(w http.ResponseWriter, r *http.Request) {
		messages, tool := wb05ModelInput(r)
		if strings.Contains(messages[len(messages)-1].Content, "UNSUPPORTED") {
			close(correcting)
			<-r.Context().Done()
			return
		}
		var evidence chatmodel.ContextInput
		_ = json.Unmarshal([]byte(tool.Content), &evidence)
		wb06Final(w, domain.Draft{Claims: []domain.Claim{{
			Text: "蓝灯表示故障模式。", FragmentIDs: []string{evidence.Documents[0].ID},
		}}})
	})
	defer generator.Close()
	reviewer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		modelReply(w, `{"claims":[{"claim_index":0,"verdict":"UNSUPPORTED","reason":"原文是维护模式。","supports":[]}]}`)
	}))
	defer reviewer.Close()
	cm, _ := chatmodel.NewOpenAI(generator.URL, "stream-fixture", "")
	review, _ := chatmodel.NewOpenAI(reviewer.URL, "review-fixture", "")
	dsn := workbenchDatabase(t)
	server, _, stop := wbStartOptions(t, dsn, cm, true, application.Options{GroundingModel: review})
	defer func() { stop() }()
	client := wbNewClient(t, server)
	client.login("admin", wbPassword)
	wbPublish(client)
	cid := client.request("POST", "/v1/conversations", map[string]any{}, 201)["id"].(string)
	id := client.request("POST", "/v1/conversations/"+cid+"/messages",
		map[string]string{"text": "蓝灯含义"}, 202)["id"].(string)
	select {
	case <-correcting:
	case <-time.After(4 * time.Second):
		t.Fatal("correction not started")
	}
	running := client.request("GET", "/v1/responses/"+id, nil, 200)
	reviews, ok := running["grounding_reviews"].([]any)
	if !ok || len(reviews) != 1 || running["answer"] != "" {
		t.Fatalf("completed review was not durably saved before correction: reviews=%v", ok)
	}
	stop()
	server, _, stop = wbStartOptions(t, dsn, cm, false, application.Options{GroundingModel: review})
	client = wbNewClient(t, server)
	client.login("admin", wbPassword)
	final := client.answer(id)
	if final["status"] != "INTERRUPTED" || len(final["grounding_reviews"].([]any)) != 1 ||
		final["model_usage"].(map[string]any)["total_tokens"].(float64) == 0 {
		t.Fatalf("restart lost completed grounding: status=%v", final["status"])
	}
}

func TestOnlineGroundingPDFPageTextWithoutSourceLineNumbers(t *testing.T) {
	generator := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			Messages []*schema.Message `json:"messages"`
		}
		_ = json.NewDecoder(r.Body).Decode(&input)
		w.Header().Set("Content-Type", "text/event-stream")
		var p chatmodel.ContextInput
		_ = json.Unmarshal([]byte(input.Messages[1].Content), &p)
		var source *schema.Message
		for _, message := range input.Messages {
			if message.Role == schema.Tool {
				source = message
			}
		}
		if source == nil {
			wb06ToolCall(w, "pdf", einoflow.ReadAttachmentToolName, map[string]any{
				"attachment_id": p.Attachments[0].ID, "page": 1,
			})
			return
		}
		var result domain.AttachmentReadResult
		_ = json.Unmarshal([]byte(source.Content), &result)
		wb06Final(w, domain.Draft{Claims: []domain.Claim{{
			Text: "第一页包含 Synthetic PDF page one。", SourceIDs: []string{result.Source.SourceID},
		}}})
	}))
	defer generator.Close()
	reviewer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			Messages []*schema.Message `json:"messages"`
		}
		_ = json.NewDecoder(r.Body).Decode(&input)
		var p groundingPayload
		_ = json.Unmarshal([]byte(input.Messages[1].Content), &p)
		if len(p.Evidence) != 1 || p.Evidence[0].Content != "Synthetic PDF page one\n" ||
			p.Evidence[0].StartLine != 1 || !strings.HasPrefix(p.Evidence[0].ID, "attachment:") {
			t.Error("PDF page text or identity lost")
			http.Error(w, "missing page text", 400)
			return
		}
		s := p.Evidence[0]
		raw, _ := json.Marshal(map[string]any{"claims": []domain.GroundingVerdict{{
			ClaimIndex: 0, Verdict: "SUPPORTED", Reason: "第一页原文直接支持。",
			Supports: []domain.GroundingSupport{{EvidenceID: s.ID, StartLine: 1, EndLine: s.EndLine, Quote: s.Content}},
		}}})
		modelReply(w, string(raw))
	}))
	defer reviewer.Close()
	cm, _ := chatmodel.NewOpenAI(generator.URL, "pdf-fixture", "")
	review, _ := chatmodel.NewOpenAI(reviewer.URL, "review-fixture", "")
	files, err := blobstore.Open(filepath.Join(t.TempDir(), "files"), 500_000_000, 100_000_000)
	if err != nil {
		t.Fatal(err)
	}
	server, _, stop := wbStartOptions(t, workbenchDatabase(t), cm, true, application.Options{
		GroundingModel: review, Files: files, AttachmentRunner: newAttachmentReplayRunner(),
	})
	defer stop()
	client := wbNewClient(t, server)
	client.login("admin", wbPassword)
	cid := client.request("POST", "/v1/conversations", map[string]any{}, 201)["id"].(string)
	uploaded := uploadAttachment(t, client, cid, "page.pdf", "application/pdf", []byte("%PDF-FAKE-WB04"), 201)
	waitAttachment(t, client, uploaded["id"].(string))
	id := client.request("POST", "/v1/conversations/"+cid+"/messages", map[string]any{
		"text": "第一页说什么", "attachment_ids": []string{uploaded["id"].(string)},
	}, 202)["id"].(string)
	final := client.answer(id)
	if final["status"] != "ANSWERED" || len(final["grounding_reviews"].([]any)) != 1 {
		t.Fatalf("valid PDF text was rejected: status=%v", final["status"])
	}
}
