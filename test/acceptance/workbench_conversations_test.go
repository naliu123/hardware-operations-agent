package acceptance_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cloudwego/eino/schema"
	"github.com/jackc/pgx/v5"

	"hwops/internal/adapters/chatmodel"
	"hwops/internal/adapters/monitor"
	"hwops/internal/application"
	"hwops/internal/domain"
)

func wbReply(w http.ResponseWriter, content string) {
	_ = json.NewEncoder(w).Encode(map[string]any{
		"choices": []any{map[string]any{"message": map[string]string{"content": content}, "finish_reason": "stop"}},
		"usage":   map[string]int{"prompt_tokens": 30, "completion_tokens": 12, "total_tokens": 42},
	})
}

func wbPublish(c *wbClient) string {
	r := c.request("POST", "/v1/knowledge/revisions", map[string]any{
		"title": "WB02 合成手册", "source": "fixture://wb02", "content": "# 蓝灯\n蓝灯表示维护模式。",
		"applicability": map[string]string{"scope": "GENERAL"},
	}, 201)
	id := r["id"].(string)
	c.request("POST", "/v1/knowledge/revisions/"+id+"/publication", map[string]string{"decision": "PUBLISH"}, 200)
	return id
}

func TestWB02HistorySearchSummaryAndIsolation(t *testing.T) {
	var mu sync.Mutex
	var inputs []chatmodel.ContextInput
	var failSummary atomic.Bool
	modelServer := httptest.NewServer(replayKnowledgeChoice(func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Messages []*schema.Message `json:"messages"`
		}
		if json.NewDecoder(r.Body).Decode(&in) != nil || len(in.Messages) < 2 {
			http.Error(w, "input", 400)
			return
		}
		if strings.Contains(in.Messages[0].Content, "摘要协议 wb-02") {
			if failSummary.Load() {
				wbReply(w, `{"summary":""}`)
			} else {
				wbReply(w, `{"summary":"用户在核对合成设备的蓝灯含义，手册说明为维护模式。历史不表示当前设备状态。"}`)
			}
			return
		}
		var payload chatmodel.ContextInput
		_ = json.Unmarshal([]byte(in.Messages[1].Content), &payload)
		mu.Lock()
		inputs = append(inputs, payload)
		mu.Unlock()
		d := domain.Draft{Claims: []domain.Claim{}, Gaps: []string{"没有资料"}}
		if len(payload.Documents) > 0 {
			d = domain.Draft{Claims: []domain.Claim{{Text: "蓝灯表示维护模式 FINAL_SEARCH_SENTINEL", FragmentIDs: []string{payload.Documents[0].ID}}}}
		}
		raw, _ := json.Marshal(d)
		wbReply(w, string(raw))
	}))
	defer modelServer.Close()
	cm, _ := chatmodel.NewOpenAI(modelServer.URL, "wb02-replay", "")
	dsn := workbenchDatabase(t)
	server, _, stop := wbStart(t, dsn, cm, true)
	defer func() { stop() }()
	admin := wbNewClient(t, server)
	admin.login("admin", wbPassword)
	wbPublish(admin)
	admin.request("POST", "/v1/admin/users", map[string]string{"username": "alice", "password": wbPassword, "role": "USER"}, 201)
	alice := wbNewClient(t, server)
	alice.login("alice", wbPassword)
	c := alice.request("POST", "/v1/conversations", map[string]any{}, 201)
	id := c["id"].(string)
	path := "/v1/conversations/" + id
	var last map[string]any
	for i := 1; i <= 9; i++ {
		p := alice.request("POST", path+"/messages", map[string]string{"text": "蓝灯含义 PRIVATE_QUESTION_SENTINEL"}, 202)
		last = alice.answer(p["id"].(string))
		if last["status"] != "ANSWERED" || last["sequence"] != float64(i) {
			t.Fatalf("turn %d failed: %v", i, last)
		}
	}
	mu.Lock()
	captured := append([]chatmodel.ContextInput{}, inputs...)
	mu.Unlock()
	if len(captured) != 9 || captured[1].History == nil || len(captured[1].History.Recent) != 1 ||
		!strings.Contains(captured[1].History.Recent[0].Answer, "FINAL_SEARCH_SENTINEL") {
		t.Fatal("external model did not receive the real previous answer")
	}
	h := captured[8].History
	if h.Summary == nil || h.Summary.From != 1 || h.Summary.Through != 2 ||
		len(h.Summary.Citations) != 1 || len(h.Recent) != 6 || h.Recent[0].Sequence != 3 {
		t.Fatalf("summary lost original range or citation: %+v", h)
	}
	selection := last["context_selection"].(map[string]any)
	if selection["through_sequence"] != float64(8) || len(selection["source_ids"].([]any)) != 1 {
		t.Fatal("answer omitted context provenance")
	}
	c = alice.request("GET", path, nil, 200)
	if !strings.Contains(c["title"].(string), "蓝灯") || c["title_source"] != "AUTO" {
		t.Fatal("automatic title was not persisted")
	}
	version := c["state_version"]
	alice.request("PATCH", path, map[string]any{"title": "手动标题 TITLE_SENTINEL", "state_version": version}, 200)
	alice.request("PATCH", path, map[string]any{"title": "过期标题", "state_version": version}, 409)
	for _, q := range []string{"TITLE_SENTINEL", "PRIVATE_QUESTION_SENTINEL", "FINAL_SEARCH_SENTINEL"} {
		if len(alice.request("GET", "/v1/conversations/search?q="+q, nil, 200)["conversations"].([]any)) != 1 ||
			len(admin.request("GET", "/v1/conversations/search?q="+q, nil, 200)["conversations"].([]any)) != 0 {
			t.Fatal("search either lost data or leaked another user's history")
		}
	}
	admin.request("GET", path+"/messages", nil, 404)
	admin.request("PATCH", path, map[string]any{"title": "stolen", "state_version": version}, 404)
	admin.request("DELETE", path, nil, 404)
	first := alice.request("GET", path+"/messages?limit=3", nil, 200)
	second := alice.request("GET", path+"/messages?after=3&limit=3", nil, 200)
	if len(first["messages"].([]any)) != 3 || second["messages"].([]any)[0].(map[string]any)["sequence"] != float64(4) {
		t.Fatal("history cursor is not stable")
	}
	other := alice.request("POST", "/v1/conversations", map[string]any{}, 201)["id"].(string)
	p := alice.request("POST", "/v1/conversations/"+other+"/messages", map[string]string{"text": "蓝灯"}, 202)
	alice.answer(p["id"].(string))
	mu.Lock()
	isolated := inputs[len(inputs)-1].History
	mu.Unlock()
	if isolated == nil || len(isolated.Recent) != 0 || isolated.Summary != nil {
		t.Fatal("a different conversation inherited history")
	}
	failSummary.Store(true)
	p = alice.request("POST", path+"/messages", map[string]string{"text": "蓝灯继续"}, 202)
	failed := alice.answer(p["id"].(string))
	if failed["status"] != "FAILED" || failed["error"].(map[string]any)["code"] != "CONTEXT_LIMIT" {
		t.Fatalf("summary failure was hidden: %v", failed)
	}
	if rows := alice.request("GET", path+"/messages", nil, 200)["messages"].([]any); len(rows) != 10 {
		t.Fatal("compression discarded original records")
	}
	failSummary.Store(false)
	retry := alice.request("POST", path+"/messages", map[string]string{"text": "蓝灯继续", "retry_of": p["id"].(string)}, 202)
	if alice.answer(retry["id"].(string))["status"] != "ANSWERED" || retry["retry_of"] != p["id"] {
		t.Fatal("explicit retry did not preserve its original attempt")
	}
	if alice.request("GET", path, nil, 200)["title"] != "手动标题 TITLE_SENTINEL" {
		t.Fatal("automatic naming overwrote manual title")
	}
	stop()
	server, _, stop = wbStart(t, dsn, cm, false)
	alice = wbNewClient(t, server)
	alice.login("alice", wbPassword)
	if len(alice.request("GET", path+"/messages", nil, 200)["messages"].([]any)) != 11 {
		t.Fatal("restart lost message order or retry")
	}
	t.Log("REPLAY: real PostgreSQL + external adapter; history, bounded summary/range/citations, summary failure, search isolation, pagination, title CAS and explicit retries passed")
}

func TestWB02ConcurrentCancelDeleteAndRestart(t *testing.T) {
	started := make(chan string, 20)
	canceled := make(chan string, 20)
	var count atomic.Int32
	modelServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		count.Add(1)
		started <- r.Header.Get("x-opencode-session")
		<-r.Context().Done()
		canceled <- r.Header.Get("x-opencode-session")
		// A late model success must never restore a canceled/deleted response.
		wbReply(w, `{"claims":[],"gaps":["LATE_ANSWER_SENTINEL"]}`)
	}))
	defer modelServer.Close()
	cm, _ := chatmodel.NewOpenAI(modelServer.URL, "blocked-wb02-replay", "")
	dsn := workbenchDatabase(t)
	server, _, stop := wbStart(t, dsn, cm, true)
	defer func() { stop() }()
	c := wbNewClient(t, server)
	c.login("admin", wbPassword)
	id := c.request("POST", "/v1/conversations", map[string]any{}, 201)["id"].(string)
	path := "/v1/conversations/" + id
	type outcome struct {
		status int
		id     string
		err    error
	}
	submit := func(key, question string) outcome {
		raw, _ := json.Marshal(map[string]string{"text": question})
		req, _ := http.NewRequest("POST", c.base+path+"/messages", bytes.NewReader(raw))
		req.Header.Set("Origin", c.base)
		req.Header.Set("X-CSRF-Token", c.csrf)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotency-Key", key)
		res, err := c.client.Do(req)
		if err != nil {
			return outcome{err: err}
		}
		defer res.Body.Close()
		var body struct{ ID string }
		err = json.NewDecoder(res.Body).Decode(&body)
		return outcome{res.StatusCode, body.ID, err}
	}
	out := make(chan outcome, 2)
	for range 2 {
		go func() { out <- submit("same", "blocked question") }()
	}
	a, b := <-out, <-out
	if a.err != nil || b.err != nil || a.status != 202 || b.status != 202 || a.id != b.id {
		t.Fatalf("concurrent idempotency failed: %+v %+v", a, b)
	}
	if res := submit("different", "another"); res.status != 409 {
		t.Fatal("two tabs admitted more than one active response")
	}
	if res := submit("same", "changed"); res.status != 409 {
		t.Fatal("same key changed request")
	}
	other := c.request("POST", "/v1/conversations", map[string]any{}, 201)["id"].(string)
	op := c.request("POST", "/v1/conversations/"+other+"/messages", map[string]string{"text": "parallel question"}, 202)
	seen := map[string]bool{}
	for range 2 {
		select {
		case id := <-started:
			seen[id] = true
		case <-time.After(3 * time.Second):
			t.Fatal("different conversations did not execute in parallel")
		}
	}
	if len(seen) != 2 {
		t.Fatal("duplicate task execution")
	}
	c.request("POST", "/v1/responses/"+a.id+"/cancel", map[string]any{}, 202)
	select {
	case id := <-canceled:
		if id != strings.TrimPrefix(path, "/v1/conversations/") {
			t.Fatal("canceled wrong model request")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("cancel did not stop external model request")
	}
	if c.answer(a.id)["status"] != "CANCELED" {
		t.Fatal("late model answer overwrote cancellation")
	}
	c.request("POST", "/v1/responses/"+a.id+"/cancel", map[string]any{}, 202)
	if len(c.request("GET", "/v1/conversations/search?q=LATE_ANSWER_SENTINEL", nil, 200)["conversations"].([]any)) != 0 {
		t.Fatal("late draft was searchable")
	}
	// Restart leaves old response identity interrupted and does not call model.
	previousCalls := count.Load()
	stop()
	server, _, stop = wbStart(t, dsn, cm, false)
	c = wbNewClient(t, server)
	c.login("admin", wbPassword)
	if c.answer(op["id"].(string))["status"] != "INTERRUPTED" || count.Load() != previousCalls {
		t.Fatal("restart re-ran unfinished model work")
	}
	// Inject a real DB cleanup failure, then remove it and observe retry.
	db, err := pgx.Connect(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close(context.Background())
	_, err = db.Exec(context.Background(), `CREATE FUNCTION wb02_reject_delete() RETURNS trigger LANGUAGE plpgsql AS
		$$ BEGIN RAISE EXCEPTION 'synthetic cleanup failure'; END $$;
		CREATE TRIGGER wb02_cleanup_failure BEFORE DELETE ON responses FOR EACH ROW EXECUTE FUNCTION wb02_reject_delete()`)
	if err != nil {
		t.Fatal(err)
	}
	c.request("DELETE", path, nil, 202)
	for _, route := range []string{path, path + "/messages", "/v1/responses/" + a.id, "/v1/responses/" + a.id + "/events"} {
		c.request("GET", route, nil, 404)
	}
	c.request("POST", path+"/messages", map[string]string{"text": "blocked question"}, 404, map[string]string{"Idempotency-Key": "same"})
	for deadline := time.Now().Add(4 * time.Second); ; {
		var attempts int
		_ = db.QueryRow(context.Background(), "SELECT attempts FROM cleanup_jobs WHERE conversation_id=$1", id).Scan(&attempts)
		if attempts > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("cleanup failure was not persisted")
		}
		time.Sleep(20 * time.Millisecond)
	}
	_, err = db.Exec(context.Background(), "DROP TRIGGER wb02_cleanup_failure ON responses; DROP FUNCTION wb02_reject_delete()")
	if err != nil {
		t.Fatal(err)
	}
	for deadline := time.Now().Add(5 * time.Second); ; {
		var status string
		_ = db.QueryRow(context.Background(), "SELECT status FROM cleanup_jobs WHERE conversation_id=$1", id).Scan(&status)
		if status == "DONE" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("cleanup did not recover")
		}
		time.Sleep(20 * time.Millisecond)
	}
	var privateRows int
	_ = db.QueryRow(context.Background(), "SELECT count(*) FROM responses WHERE payload->>'conversation_id'=$1", id).Scan(&privateRows)
	if privateRows != 0 {
		t.Fatal("cleanup left private response contents")
	}
	c.request("DELETE", path, nil, 202)
	t.Log("REPLAY: PostgreSQL concurrent admission, per-conversation serialization, cross-conversation parallelism, cancel, restart without rerun, immediate deletion and durable cleanup retry passed")
}

func TestWB02ContextOverflowIsExplicit(t *testing.T) {
	server, _, stop := wbStart(t, workbenchDatabase(t), wbModel(t), true)
	defer stop()
	c := wbNewClient(t, server)
	c.login("admin", wbPassword)
	wbPublish(c)
	id := c.request("POST", "/v1/conversations", map[string]any{}, 201)["id"].(string)
	failed := false
	for i := 0; i < 6; i++ {
		p := c.request("POST", "/v1/conversations/"+id+"/messages",
			map[string]string{"text": "蓝灯 " + strings.Repeat("a", 15980)}, 202)
		a := c.answer(p["id"].(string))
		if a["status"] == "FAILED" {
			code := a["error"].(map[string]any)["code"]
			if code != "CONTEXT_LIMIT" && code != "MODEL_BUDGET_EXCEEDED" {
				t.Fatalf("unexpected failure: %v", a)
			}
			failed = true
			break
		}
	}
	if !failed {
		t.Fatal("oversize history was silently accepted")
	}
	t.Log("REPLAY: oversized context rejected explicitly; raw history retained")
}

func TestWB02DeleteRunningConversation(t *testing.T) {
	started := make(chan struct{}, 1)
	canceled := make(chan struct{}, 1)
	modelServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		started <- struct{}{}
		<-r.Context().Done()
		canceled <- struct{}{}
	}))
	defer modelServer.Close()
	cm, _ := chatmodel.NewOpenAI(modelServer.URL, "delete-replay", "")
	server, _, stop := wbStart(t, workbenchDatabase(t), cm, true)
	defer stop()
	c := wbNewClient(t, server)
	c.login("admin", wbPassword)
	id := c.request("POST", "/v1/conversations", map[string]any{}, 201)["id"].(string)
	p := c.request("POST", "/v1/conversations/"+id+"/messages", map[string]string{"text": "running delete"}, 202)
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("model did not start")
	}
	c.request("DELETE", "/v1/conversations/"+id, nil, 202)
	select {
	case <-canceled:
	case <-time.After(2 * time.Second):
		t.Fatal("delete left a model request running")
	}
	c.request("GET", "/v1/responses/"+p["id"].(string), nil, 404)
	q := url.QueryEscape("running delete")
	if len(c.request("GET", "/v1/conversations/search?q="+q, nil, 200)["conversations"].([]any)) != 0 {
		t.Fatal("deleted conversation searchable")
	}
}

func TestWB02HistoryDoesNotReuseDeviceReadings(t *testing.T) {
	var monitorCalls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var q domain.ObservationQuery
		_ = json.NewDecoder(r.Body).Decode(&q)
		d := monitorData(q)
		if monitorCalls.Add(1) > 1 {
			d.Values[0].Value = "650"
		}
		_ = json.NewEncoder(w).Encode(d)
	}))
	defer upstream.Close()
	external := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		input := readAgentRequest(t, r)
		var payload chatmodel.ContextInput
		_ = json.Unmarshal([]byte(input.Messages[1].Content), &payload)
		if len(payload.History.Recent) > 0 {
			raw, _ := json.Marshal(payload.History)
			if strings.Contains(string(raw), "500") {
				t.Error("previous device reading was offered as history evidence")
			}
		}
		if len(input.Messages) == 2 {
			args, _ := json.Marshal(observationRequest())
			_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": schema.AssistantMessage("", []schema.ToolCall{{
				ID: "fresh-observation", Type: "function", Function: schema.FunctionCall{Name: "observe_device", Arguments: string(args)},
			}})}}})
			return
		}
		var evidence domain.Evidence
		_ = json.Unmarshal([]byte(input.Messages[len(input.Messages)-1].Content), &evidence)
		raw, _ := json.Marshal(domain.Draft{Claims: []domain.Claim{}, Observations: []domain.ObservationSelection{{EvidenceID: evidence.ID}}})
		wbReply(w, string(raw))
	}))
	defer external.Close()
	cm, _ := chatmodel.NewOpenAI(external.URL, "wb02-observation-replay", "")
	observer, _ := monitor.New(upstream.URL, "", "REPLAY", time.Second, 1)
	server, _, stop := wbStartOptions(t, workbenchDatabase(t), cm, true, application.Options{Observer: observer})
	defer stop()
	c := wbNewClient(t, server)
	c.login("admin", wbPassword)
	c.request("PUT", "/v1/devices/target", domain.DeviceInput{
		Name: "target", Model: "fixture/Atlas", Firmware: "R2", Source: "fixture://inventory",
		MonitoringID: "monitor/target", ObservedAt: time.Now().UTC(), DataMode: "REPLAY",
	}, 200)
	id := c.request("POST", "/v1/conversations", map[string]any{}, 201)["id"].(string)
	for _, value := range []string{"500", "650"} {
		p := c.request("POST", "/v1/conversations/"+id+"/messages", map[string]string{"text": "风扇转速和温度？", "device_id": "target"}, 202)
		a := c.answer(p["id"].(string))
		if a["status"] != "ANSWERED" || !strings.Contains(a["answer"].(string), "rpm = "+value) {
			t.Fatalf("observation was not refreshed: %v", a)
		}
	}
	if monitorCalls.Load() != 2 {
		t.Fatal("did not read fresh observations")
	}
}

func TestWB02LiveHistory(t *testing.T) {
	if os.Getenv("HWOPS_TEST_LIVE_MODEL") != "1" {
		t.Skip("real model integration is separate")
	}
	cm, err := chatmodel.NewOpenAI(os.Getenv("HWOPS_MODEL_ENDPOINT"), os.Getenv("HWOPS_MODEL"), os.Getenv("HWOPS_MODEL_API_KEY"))
	if err != nil {
		t.Fatal(err)
	}
	server, _, stop := wbStart(t, workbenchDatabase(t), cm, true, "LIVE")
	defer stop()
	c := wbNewClient(t, server)
	c.login("admin", wbPassword)
	wbPublish(c)
	id := c.request("POST", "/v1/conversations", map[string]any{}, 201)["id"].(string)
	for i, question := range []string{"根据合成手册，蓝灯代表什么？", "刚才讨论的指示灯是什么颜色？仍按刚才的手册回答。"} {
		p := c.request("POST", "/v1/conversations/"+id+"/messages", map[string]string{"text": question}, 202)
		a := c.answer(p["id"].(string), 65*time.Second)
		if a["status"] != "ANSWERED" || !strings.Contains(a["answer"].(string), "蓝") || len(a["citations"].([]any)) != 1 {
			t.Fatalf("LIVE follow-up failed: status=%v error=%v gaps=%v", a["status"], a["error"], a["gaps"])
		}
		if i > 0 && a["context_selection"].(map[string]any)["through_sequence"] != float64(1) {
			t.Fatal("live follow-up did not select previous history")
		}
		t.Logf("LIVE turn %d: model=%s usage=%v", i+1, os.Getenv("HWOPS_MODEL"), a["model_usage"])
	}
}
