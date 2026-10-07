package acceptance_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/cloudwego/eino/schema"
	"github.com/jackc/pgx/v5"

	"hwops/internal/adapters/chatmodel"
	knowledgeagent "hwops/internal/agents/knowledge"
	"hwops/internal/domain"
)

func wbStreamFrame(w http.ResponseWriter, value any) {
	raw, _ := json.Marshal(value)
	_, _ = fmt.Fprintf(w, "data: %s\n\n", raw)
	w.(http.Flusher).Flush()
}

func wbStreamDone(w http.ResponseWriter) {
	_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
	w.(http.Flusher).Flush()
}

type wbSSE struct {
	response *http.Response
	events   <-chan domain.ResponseEvent
}

func (c *wbClient) openEvents(id string, after int64) *wbSSE {
	c.t.Helper()
	req, _ := http.NewRequest(http.MethodGet, c.base+"/v1/responses/"+id+"/events", nil)
	req.Header.Set("Last-Event-ID", strconv.FormatInt(after, 10))
	response, err := c.client.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK || response.Header.Get("Content-Type") != "text/event-stream" {
		response.Body.Close()
		c.t.Fatalf("invalid event stream response: %d", response.StatusCode)
	}
	out := make(chan domain.ResponseEvent, 16)
	go func() {
		defer close(out)
		scanner := bufio.NewScanner(response.Body)
		for scanner.Scan() {
			line := scanner.Text()
			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			var event domain.ResponseEvent
			if json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &event) == nil {
				out <- event
			}
		}
	}()
	return &wbSSE{response: response, events: out}
}

func (s *wbSSE) close() {
	_ = s.response.Body.Close()
}

func waitWBEvent(t *testing.T, stream *wbSSE, kind string) domain.ResponseEvent {
	t.Helper()
	for {
		select {
		case event, ok := <-stream.events:
			if !ok {
				t.Fatalf("event stream ended before %s", kind)
			}
			if event.Type == kind {
				return event
			}
		case <-time.After(3 * time.Second):
			t.Fatalf("timed out waiting for %s", kind)
		}
	}
}

func closeGate(gate chan struct{}) {
	select {
	case <-gate:
	default:
		close(gate)
	}
}

func wb05KnowledgeModel(final http.HandlerFunc) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			Messages []*schema.Message `json:"messages"`
			Stream   bool              `json:"stream"`
		}
		if json.NewDecoder(r.Body).Decode(&input) != nil || !input.Stream || len(input.Messages) < 2 {
			http.Error(w, "native stream required", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		var toolMessage *schema.Message
		for _, message := range input.Messages {
			if message.Role == schema.Tool {
				toolMessage = message
			}
		}
		if toolMessage != nil {
			raw, _ := json.Marshal(input)
			r.Body = io.NopCloser(bytes.NewReader(raw))
			final(w, r)
			return
		}
		var payload chatmodel.ContextInput
		_ = json.Unmarshal([]byte(input.Messages[1].Content), &payload)
		arguments, _ := json.Marshal(map[string]string{"request": payload.Question})
		index := 0
		wbStreamFrame(w, map[string]any{"choices": []any{map[string]any{
			"index": 0, "delta": map[string]any{"tool_calls": []schema.ToolCall{{
				Index: &index, ID: "wb05-call", Type: "function",
				Function: schema.FunctionCall{Name: knowledgeagent.ToolName, Arguments: string(arguments)},
			}}},
		}}})
		wbStreamFrame(w, map[string]any{"choices": []any{map[string]any{
			"index": 0, "delta": map[string]any{}, "finish_reason": "tool_calls",
		}}})
		wbStreamDone(w)
	}))
}

func wb05ModelInput(r *http.Request) ([]*schema.Message, *schema.Message) {
	var input struct {
		Messages []*schema.Message `json:"messages"`
	}
	_ = json.NewDecoder(r.Body).Decode(&input)
	var toolMessage *schema.Message
	for _, message := range input.Messages {
		if message.Role == schema.Tool {
			toolMessage = message
		}
	}
	return input.Messages, toolMessage
}

func TestWB05PublicSSEReceivesRealDraftBeforeModelFinishAndReplays(t *testing.T) {
	firstUpstream, secondUpstream := make(chan struct{}), make(chan struct{})
	releaseSecond, releaseFinish := make(chan struct{}), make(chan struct{})
	defer closeGate(releaseSecond)
	defer closeGate(releaseFinish)
	modelServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			Messages   []*schema.Message `json:"messages"`
			ToolChoice any               `json:"tool_choice"`
			Stream     bool              `json:"stream"`
		}
		if json.NewDecoder(r.Body).Decode(&input) != nil || !input.Stream {
			http.Error(w, "native stream required", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		var toolMessage *schema.Message
		for _, message := range input.Messages {
			if message.Role == schema.Tool {
				toolMessage = message
			}
		}
		if toolMessage == nil {
			index := 0
			wbStreamFrame(w, map[string]any{"choices": []any{map[string]any{
				"index": 0, "delta": map[string]any{"tool_calls": []schema.ToolCall{{
					Index: &index, ID: "wb05-", Type: "function",
					Function: schema.FunctionCall{Name: "retrieve_", Arguments: `{"request":"蓝灯`},
				}}},
			}}})
			wbStreamFrame(w, map[string]any{"choices": []any{map[string]any{
				"index": 0, "delta": map[string]any{"tool_calls": []schema.ToolCall{{
					Index: &index, ID: "tool",
					Function: schema.FunctionCall{Name: "hardware_knowledge", Arguments: `含义"}`},
				}}},
			}}})
			wbStreamFrame(w, map[string]any{"choices": []any{map[string]any{
				"index": 0, "delta": map[string]any{}, "finish_reason": "tool_calls",
			}}})
			wbStreamDone(w)
			return
		}
		var evidence chatmodel.ContextInput
		if json.Unmarshal([]byte(toolMessage.Content), &evidence) != nil || len(evidence.Documents) == 0 {
			http.Error(w, "missing tool evidence", http.StatusBadRequest)
			return
		}
		fragmentID, _ := json.Marshal(evidence.Documents[0].ID)
		parts := []string{
			`{"claims":[{"text":"真实`,
			`增量一，`,
			`增量二。","fragment_ids":[` + string(fragmentID) + `]}],"gaps":[],"conflicts":[]}`,
		}
		wbStreamFrame(w, map[string]any{"choices": []any{map[string]any{
			"index": 0, "delta": map[string]any{"content": parts[0]},
		}}})
		close(firstUpstream)
		<-releaseSecond
		wbStreamFrame(w, map[string]any{"choices": []any{map[string]any{
			"index": 0, "delta": map[string]any{"content": parts[1]},
		}}})
		close(secondUpstream)
		<-releaseFinish
		wbStreamFrame(w, map[string]any{"choices": []any{map[string]any{
			"index": 0, "delta": map[string]any{"content": parts[2]},
		}}})
		wbStreamFrame(w, map[string]any{
			"choices": []any{map[string]any{"index": 0, "delta": map[string]any{}, "finish_reason": "stop"}},
			"usage":   map[string]int{"prompt_tokens": 31, "completion_tokens": 15, "total_tokens": 46},
		})
		wbStreamDone(w)
	}))
	defer modelServer.Close()
	cm, _ := chatmodel.NewOpenAI(modelServer.URL, "wb05-stream-replay", "")
	server, _, stop := wbStart(t, workbenchDatabase(t), cm, true)
	defer stop()
	client := wbNewClient(t, server)
	client.login("admin", wbPassword)
	wbPublish(client)
	conversationID := client.request("POST", "/v1/conversations", map[string]any{}, 201)["id"].(string)
	submitted := client.request("POST", "/v1/conversations/"+conversationID+"/messages",
		map[string]string{"text": "蓝灯含义"}, 202)
	responseID := submitted["id"].(string)
	stream := client.openEvents(responseID, 0)

	<-firstUpstream
	first := waitWBEvent(t, stream, "answer_delta")
	if first.Delta != "真实" || first.DraftVersion != 1 {
		t.Fatalf("first real delta mismatch: %+v", first)
	}
	closeGate(releaseSecond)
	<-secondUpstream
	second := waitWBEvent(t, stream, "answer_delta")
	if second.Delta != "增量一，" || second.ID <= first.ID || second.DraftVersion != first.DraftVersion {
		t.Fatalf("second real delta mismatch: %+v", second)
	}
	if running := client.request("GET", "/v1/responses/"+responseID, nil, 200); running["status"] != "RUNNING" ||
		running["answer"] != "" {
		t.Fatalf("draft was promoted before upstream finished: %v", running)
	}
	stream.close()
	closeGate(releaseFinish)
	final := client.answer(responseID)
	if final["status"] != "ANSWERED" || !strings.Contains(final["answer"].(string), "真实增量一，增量二。") {
		t.Fatalf("final answer missing: %v", final)
	}

	replay := client.openEvents(responseID, first.ID)
	var replayed []domain.ResponseEvent
	for event := range replay.events {
		replayed = append(replayed, event)
	}
	if len(replayed) < 4 {
		t.Fatalf("reconnect omitted persisted events: %+v", replayed)
	}
	lastID := first.ID
	var secondCount int
	var retracted, answered bool
	for _, event := range replayed {
		if event.ID <= lastID {
			t.Fatalf("event sequence duplicated or regressed: %+v", replayed)
		}
		lastID = event.ID
		if event.ID == second.ID {
			secondCount++
		}
		if event.Type == "answer_delta" &&
			(strings.Contains(event.Delta, "fragment_ids") || strings.Contains(event.Delta, knowledgeagent.ToolName)) {
			t.Fatalf("internal model structure leaked: %+v", event)
		}
		if event.Type == "draft_retracted" && event.DraftVersion == 1 &&
			event.Reason != nil && event.Reason.Code == "FINALIZED" {
			retracted = true
		}
		if event.Type == "answer" && event.Response != nil && event.Response.Status == "ANSWERED" {
			answered = true
		}
	}
	if secondCount != 1 || !retracted || !answered {
		t.Fatalf("replay did not converge: second=%d retracted=%v answered=%v events=%+v",
			secondCount, retracted, answered, replayed)
	}
	t.Log("REPLAY: public HTTPS/SSE + real PostgreSQL + external streaming adapter delivered multiple persisted deltas before finish, then retracted and atomically finalized")
}

func TestWB05InvalidCitationRetractsOldDraftBeforeCorrection(t *testing.T) {
	modelServer := wb05KnowledgeModel(func(w http.ResponseWriter, r *http.Request) {
		messages, toolMessage := wb05ModelInput(r)
		var evidence chatmodel.ContextInput
		_ = json.Unmarshal([]byte(toolMessage.Content), &evidence)
		correction := messages[len(messages)-1].Content
		correcting := strings.Contains(correction, "公开知识只能放 fragment_ids") &&
			strings.Contains(correction, "未调用私有工具时必须省略 source_ids")
		fragmentID, text := evidence.Documents[0].ID, "错误草稿不应保留"
		sourceIDs := []string{"fixture://not-a-private-source"}
		if correcting {
			text, sourceIDs = "复核修正后的有效结论", nil
		}
		raw, _ := json.Marshal(domain.Draft{Claims: []domain.Claim{{
			Text: text, FragmentIDs: []string{fragmentID}, SourceIDs: sourceIDs,
		}}, Gaps: []string{}, Conflicts: []domain.KnowledgeConflict{}})
		wbStreamFrame(w, map[string]any{"choices": []any{map[string]any{
			"index": 0, "delta": map[string]any{"content": string(raw)},
		}}})
		wbStreamFrame(w, map[string]any{"choices": []any{map[string]any{
			"index": 0, "delta": map[string]any{}, "finish_reason": "stop",
		}}})
		wbStreamDone(w)
	})
	defer modelServer.Close()
	cm, _ := chatmodel.NewOpenAI(modelServer.URL, "wb05-correction-replay", "")
	server, _, stop := wbStart(t, workbenchDatabase(t), cm, true)
	defer stop()
	client := wbNewClient(t, server)
	client.login("admin", wbPassword)
	wbPublish(client)
	conversationID := client.request("POST", "/v1/conversations", map[string]any{}, 201)["id"].(string)
	submitted := client.request("POST", "/v1/conversations/"+conversationID+"/messages",
		map[string]string{"text": "蓝灯含义"}, 202)
	responseID := submitted["id"].(string)
	final := client.answer(responseID)
	if final["status"] != "ANSWERED" || !strings.Contains(final["answer"].(string), "复核修正") ||
		strings.Contains(final["answer"].(string), "错误草稿") {
		t.Fatalf("correction was not finalized cleanly: %v", final)
	}
	stream := client.openEvents(responseID, 0)
	var events []domain.ResponseEvent
	for event := range stream.events {
		events = append(events, event)
	}
	var starts, superseded, finalized int
	for _, event := range events {
		switch {
		case event.Type == "draft_started":
			starts++
		case event.Type == "draft_retracted" && event.DraftVersion == 1 &&
			event.Reason != nil && event.Reason.Code == "SUPERSEDED":
			superseded++
		case event.Type == "draft_retracted" && event.DraftVersion == 2 &&
			event.Reason != nil && event.Reason.Code == "FINALIZED":
			finalized++
		}
	}
	if starts != 2 || superseded != 1 || finalized != 1 {
		t.Fatalf("draft correction lifecycle mismatch: %+v", events)
	}
	t.Log("REPLAY: invalid citation draft was retracted before corrected draft version 2 and final commit")
}

func TestWB05WithdrawnKnowledgeRetractsDraftInsteadOfPublishingIt(t *testing.T) {
	draftSent, release := make(chan struct{}), make(chan struct{})
	defer closeGate(release)
	modelServer := wb05KnowledgeModel(func(w http.ResponseWriter, r *http.Request) {
		_, toolMessage := wb05ModelInput(r)
		var evidence chatmodel.ContextInput
		_ = json.Unmarshal([]byte(toolMessage.Content), &evidence)
		raw, _ := json.Marshal(domain.Draft{Claims: []domain.Claim{{
			Text: "撤回前生成的临时结论", FragmentIDs: []string{evidence.Documents[0].ID},
		}}, Gaps: []string{}, Conflicts: []domain.KnowledgeConflict{}})
		wbStreamFrame(w, map[string]any{"choices": []any{map[string]any{
			"index": 0, "delta": map[string]any{"content": string(raw)},
		}}})
		close(draftSent)
		<-release
		wbStreamFrame(w, map[string]any{"choices": []any{map[string]any{
			"index": 0, "delta": map[string]any{}, "finish_reason": "stop",
		}}})
		wbStreamDone(w)
	})
	defer modelServer.Close()
	cm, _ := chatmodel.NewOpenAI(modelServer.URL, "wb05-withdraw-replay", "")
	server, _, stop := wbStart(t, workbenchDatabase(t), cm, true)
	defer stop()
	client := wbNewClient(t, server)
	client.login("admin", wbPassword)
	revisionID := wbPublish(client)
	conversationID := client.request("POST", "/v1/conversations", map[string]any{}, 201)["id"].(string)
	submitted := client.request("POST", "/v1/conversations/"+conversationID+"/messages",
		map[string]string{"text": "蓝灯含义"}, 202)
	responseID := submitted["id"].(string)
	stream := client.openEvents(responseID, 0)
	<-draftSent
	delta := waitWBEvent(t, stream, "answer_delta")
	if !strings.Contains(delta.Delta, "临时结论") {
		t.Fatalf("expected provisional text: %+v", delta)
	}
	client.request("POST", "/v1/knowledge/revisions/"+revisionID+"/publication",
		map[string]string{"decision": "WITHDRAW"}, 200)
	closeGate(release)
	final := client.answer(responseID)
	if final["status"] != "UNRESOLVED" || final["answer"] != "" ||
		!strings.Contains(final["gaps"].([]any)[0].(string), "资料已撤回") {
		t.Fatalf("withdrawn source was published: %v", final)
	}
	retraction := waitWBEvent(t, stream, "draft_retracted")
	if retraction.DraftVersion != delta.DraftVersion || retraction.Reason == nil ||
		retraction.Reason.Code != "FINALIZED" {
		t.Fatalf("withdrawn draft was not retracted: %+v", retraction)
	}
	stream.close()
	t.Log("REPLAY: knowledge withdrawn during generation invalidated the draft before an unresolved final response")
}

func TestWB05CancelRetractsDraftAndStopsUpstream(t *testing.T) {
	upstreamCanceled := make(chan struct{})
	modelServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			Stream bool `json:"stream"`
		}
		_ = json.NewDecoder(r.Body).Decode(&input)
		if !input.Stream {
			http.Error(w, "native stream required", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		wbStreamFrame(w, map[string]any{"choices": []any{map[string]any{
			"index": 0, "delta": map[string]any{"content": `{"claims":[{"text":"停止前草稿`},
		}}})
		<-r.Context().Done()
		close(upstreamCanceled)
		// A provider that tries to write after cancellation cannot restore the
		// retracted version because the response is no longer RUNNING.
		wbStreamFrame(w, map[string]any{"choices": []any{map[string]any{
			"index": 0, "delta": map[string]any{"content": `迟到内容"}],"gaps":[]}`},
		}}})
	}))
	defer modelServer.Close()
	cm, _ := chatmodel.NewOpenAI(modelServer.URL, "wb05-cancel-replay", "")
	server, _, stop := wbStart(t, workbenchDatabase(t), cm, true)
	defer stop()
	client := wbNewClient(t, server)
	client.login("admin", wbPassword)
	conversationID := client.request("POST", "/v1/conversations", map[string]any{}, 201)["id"].(string)
	submitted := client.request("POST", "/v1/conversations/"+conversationID+"/messages",
		map[string]string{"text": "不需要检索，直接回答"}, 202)
	responseID := submitted["id"].(string)
	stream := client.openEvents(responseID, 0)
	delta := waitWBEvent(t, stream, "answer_delta")
	client.request("POST", "/v1/responses/"+responseID+"/cancel", map[string]any{}, 202)
	retraction := waitWBEvent(t, stream, "draft_retracted")
	if retraction.DraftVersion != delta.DraftVersion || retraction.Reason == nil ||
		retraction.Reason.Code != "CANCELED" {
		t.Fatalf("cancel did not retract the active draft: %+v", retraction)
	}
	select {
	case <-upstreamCanceled:
	case <-time.After(2 * time.Second):
		t.Fatal("cancel did not propagate to the external stream")
	}
	final := client.answer(responseID)
	if final["status"] != "CANCELED" || final["answer"] != "" {
		t.Fatalf("canceled draft was promoted: %v", final)
	}
	var late bool
	for event := range stream.events {
		late = late || (event.Type == "answer_delta" && strings.Contains(event.Delta, "迟到"))
	}
	if late {
		t.Fatal("late upstream delta was persisted after cancellation")
	}
	t.Log("REPLAY: stop atomically retracted the draft, canceled the external model stream and rejected late output")
}

func TestWB05FinalEventFailureDoesNotPromiseOrRerunAnswer(t *testing.T) {
	finish := make(chan struct{})
	modelCalls := make(chan struct{}, 4)
	modelServer := wb05KnowledgeModel(func(w http.ResponseWriter, r *http.Request) {
		modelCalls <- struct{}{}
		_, toolMessage := wb05ModelInput(r)
		var evidence chatmodel.ContextInput
		_ = json.Unmarshal([]byte(toolMessage.Content), &evidence)
		raw, _ := json.Marshal(domain.Draft{Claims: []domain.Claim{{
			Text: "只能在原子提交后显示的答案", FragmentIDs: []string{evidence.Documents[0].ID},
		}}, Gaps: []string{}, Conflicts: []domain.KnowledgeConflict{}})
		wbStreamFrame(w, map[string]any{"choices": []any{map[string]any{
			"index": 0, "delta": map[string]any{"content": string(raw)},
		}}})
		<-finish
		wbStreamFrame(w, map[string]any{"choices": []any{map[string]any{
			"index": 0, "delta": map[string]any{}, "finish_reason": "stop",
		}}})
		wbStreamDone(w)
	})
	defer modelServer.Close()
	defer closeGate(finish)
	cm, _ := chatmodel.NewOpenAI(modelServer.URL, "wb05-atomic-replay", "")
	dsn := workbenchDatabase(t)
	server, _, stop := wbStart(t, dsn, cm, true)
	defer func() { stop() }()
	client := wbNewClient(t, server)
	client.login("admin", wbPassword)
	wbPublish(client)
	conversationID := client.request("POST", "/v1/conversations", map[string]any{}, 201)["id"].(string)
	submitted := client.request("POST", "/v1/conversations/"+conversationID+"/messages",
		map[string]string{"text": "蓝灯含义"}, 202)
	responseID := submitted["id"].(string)
	stream := client.openEvents(responseID, 0)
	delta := waitWBEvent(t, stream, "answer_delta")

	db, err := pgx.Connect(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close(context.Background())
	_, err = db.Exec(context.Background(), `CREATE FUNCTION wb05_reject_answer_event() RETURNS trigger LANGUAGE plpgsql AS $$
		BEGIN
			IF NEW.payload->>'type'='answer' THEN
				RAISE EXCEPTION 'synthetic final event failure';
			END IF;
			RETURN NEW;
		END $$;
		CREATE TRIGGER wb05_answer_event_failure BEFORE INSERT ON response_events
		FOR EACH ROW EXECUTE FUNCTION wb05_reject_answer_event()`)
	if err != nil {
		t.Fatal(err)
	}
	closeGate(finish)
	select {
	case <-modelCalls:
	case <-time.After(time.Second):
		t.Fatal("final model request was not observed")
	}
	time.Sleep(300 * time.Millisecond)
	current := client.request("GET", "/v1/responses/"+responseID, nil, 200)
	if current["status"] != "RUNNING" || current["answer"] != "" {
		t.Fatalf("failed transaction partially published response: %v", current)
	}
	var answerEvents, retractEvents int
	if err = db.QueryRow(context.Background(), `SELECT
		count(*) FILTER (WHERE payload->>'type'='answer'),
		count(*) FILTER (WHERE payload->>'type'='draft_retracted')
		FROM response_events WHERE response_id=$1`, responseID).Scan(&answerEvents, &retractEvents); err != nil {
		t.Fatal(err)
	}
	if answerEvents != 0 || retractEvents != 0 {
		t.Fatalf("final transaction was not atomic: answers=%d retracts=%d", answerEvents, retractEvents)
	}
	select {
	case <-modelCalls:
		t.Fatal("RUNNING response was automatically rerun after final storage failure")
	case <-time.After(300 * time.Millisecond):
	}
	stream.close()
	stop()
	server, _, stop = wbStart(t, dsn, cm, false)
	client = wbNewClient(t, server)
	client.login("admin", wbPassword)
	interrupted := client.answer(responseID)
	if interrupted["status"] != "INTERRUPTED" || interrupted["answer"] != "" {
		t.Fatalf("restart did not reconcile failed final save: %v", interrupted)
	}
	replay := client.openEvents(responseID, delta.ID)
	var tail []domain.ResponseEvent
	for event := range replay.events {
		tail = append(tail, event)
	}
	if len(tail) != 2 || tail[0].Type != "draft_retracted" || tail[1].Type != "interrupted" ||
		tail[0].DraftVersion != delta.DraftVersion {
		t.Fatalf("restart reconciliation was not atomic: %+v", tail)
	}
	t.Log("REPLAY: injected PostgreSQL final-event failure rolled back retraction and answer, did not rerun, then restart atomically interrupted the draft")
}

func TestWB05DeviceSnapshotChangeRetractsGeneratedGuidance(t *testing.T) {
	draftSent, release := make(chan struct{}), make(chan struct{})
	defer closeGate(release)
	modelServer := wb05KnowledgeModel(func(w http.ResponseWriter, r *http.Request) {
		_, toolMessage := wb05ModelInput(r)
		var evidence chatmodel.ContextInput
		_ = json.Unmarshal([]byte(toolMessage.Content), &evidence)
		raw, _ := json.Marshal(domain.Draft{Claims: []domain.Claim{{
			Text: "基于旧设备快照生成的建议", FragmentIDs: []string{evidence.Documents[0].ID},
		}}, Gaps: []string{}, Conflicts: []domain.KnowledgeConflict{}})
		wbStreamFrame(w, map[string]any{"choices": []any{map[string]any{
			"index": 0, "delta": map[string]any{"content": string(raw)},
		}}})
		close(draftSent)
		<-release
		wbStreamFrame(w, map[string]any{"choices": []any{map[string]any{
			"index": 0, "delta": map[string]any{}, "finish_reason": "stop",
		}}})
		wbStreamDone(w)
	})
	defer modelServer.Close()
	cm, _ := chatmodel.NewOpenAI(modelServer.URL, "wb05-device-replay", "")
	server, _, stop := wbStart(t, workbenchDatabase(t), cm, true)
	defer stop()
	client := wbNewClient(t, server)
	client.login("admin", wbPassword)
	wbPublish(client)
	observed := time.Now().UTC().Add(-time.Minute)
	put := func(firmware string, at time.Time) {
		client.request("PUT", "/v1/devices/wb05-device", map[string]any{
			"name": "WB05 device", "model": "fixture/Atlas", "firmware": firmware,
			"source": "fixture://wb05-device", "observed_at": at, "data_mode": "REPLAY",
		}, 200)
	}
	put("R1", observed)
	conversationID := client.request("POST", "/v1/conversations", map[string]any{}, 201)["id"].(string)
	submitted := client.request("POST", "/v1/conversations/"+conversationID+"/messages",
		map[string]string{"text": "蓝灯含义", "device_id": "wb05-device"}, 202)
	responseID := submitted["id"].(string)
	stream := client.openEvents(responseID, 0)
	<-draftSent
	delta := waitWBEvent(t, stream, "answer_delta")
	put("R2", observed.Add(time.Minute))
	closeGate(release)
	final := client.answer(responseID)
	if final["status"] != "UNRESOLVED" || final["answer"] != "" ||
		!strings.Contains(final["gaps"].([]any)[0].(string), "设备快照已更新") {
		t.Fatalf("stale device guidance was published: %v", final)
	}
	retraction := waitWBEvent(t, stream, "draft_retracted")
	if retraction.DraftVersion != delta.DraftVersion {
		t.Fatalf("stale device draft was not retracted: %+v", retraction)
	}
	stream.close()
	t.Log("REPLAY: a newer device snapshot prevented generated guidance from becoming final and retracted its draft")
}
