package acceptance_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/cloudwego/eino/schema"
	"github.com/jackc/pgx/v5"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"hwops/internal/adapters/chatmodel"
	"hwops/internal/adapters/postgres"
	knowledgeagent "hwops/internal/agents/knowledge"
	"hwops/internal/application"
	"hwops/internal/domain"
	"hwops/internal/einoflow"
	"hwops/internal/identity"
	"hwops/internal/transport/httpapi"
	"hwops/migrations"
)

const wbPassword = "wb01-synthetic-password"

func workbenchDatabase(t *testing.T) string {
	t.Helper()
	dsn := os.Getenv("HWOPS_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("real PostgreSQL is required; HWOPS_TEST_DATABASE_URL unset")
	}
	ctx := context.Background()
	db, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal("test PostgreSQL unavailable")
	}
	name := fmt.Sprintf("hwops_wb_%d", time.Now().UnixNano())
	if _, err = db.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{name}.Sanitize()); err != nil {
		db.Close(ctx)
		t.Fatal(err)
	}
	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	u.Path = "/" + name
	t.Cleanup(func() {
		_, err := db.Exec(ctx, "DROP DATABASE "+pgx.Identifier{name}.Sanitize()+" WITH (FORCE)")
		if err != nil {
			t.Error(err)
		}
		db.Close(ctx)
	})
	return u.String()
}

type wbClient struct {
	t      *testing.T
	client *http.Client
	base   string
	csrf   string
}

func wbNewClient(t *testing.T, server *httptest.Server) *wbClient {
	jar, _ := cookiejar.New(nil)
	return &wbClient{t: t, client: &http.Client{Transport: server.Client().Transport, Jar: jar}, base: server.URL}
}

func (c *wbClient) request(method, path string, body any, expected int, headers ...map[string]string) map[string]any {
	c.t.Helper()
	raw, _ := json.Marshal(body)
	req, _ := http.NewRequest(method, c.base+path, bytes.NewReader(raw))
	req.Header.Set("Origin", c.base)
	req.Header.Set("X-CSRF-Token", c.csrf)
	req.Header.Set("Content-Type", "application/json")
	for _, extra := range headers {
		for k, v := range extra {
			req.Header.Set(k, v)
		}
	}
	res, err := c.client.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	defer res.Body.Close()
	if path == "/v1/auth/login" && expected == 200 {
		cookies := res.Cookies()
		if len(cookies) != 1 || !cookies[0].Secure || !cookies[0].HttpOnly ||
			cookies[0].SameSite != http.SameSiteStrictMode || cookies[0].Path != "/" || cookies[0].Domain != "" {
			c.t.Fatal("login cookie does not enforce the session policy")
		}
	}
	if res.StatusCode != expected {
		value, _ := io.ReadAll(res.Body)
		c.t.Fatalf("%s %s: status %d, expected %d: %s", method, path, res.StatusCode, expected, value)
	}
	if !strings.Contains(res.Header.Get("Cache-Control"), "no-store") {
		c.t.Fatalf("private response allows caching: %s", path)
	}
	var out map[string]any
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		c.t.Fatal(err)
	}
	return out
}

func (c *wbClient) login(username, password string) {
	c.t.Helper()
	result := c.request("POST", "/v1/auth/login", map[string]string{"username": username, "password": password}, 200)
	c.csrf = result["csrf_token"].(string)
}

func (c *wbClient) answer(id string, timeouts ...time.Duration) map[string]any {
	c.t.Helper()
	timeout := 5 * time.Second
	if len(timeouts) > 0 {
		timeout = timeouts[0]
	}
	for deadline := time.Now().Add(timeout); time.Now().Before(deadline); {
		r := c.request("GET", "/v1/responses/"+id, nil, 200)
		if r["status"] != "QUEUED" && r["status"] != "RUNNING" && r["status"] != "CANCELING" {
			return r
		}
		time.Sleep(10 * time.Millisecond)
	}
	c.t.Fatal("workbench response did not complete")
	return nil
}

func wbModel(t *testing.T) *chatmodel.OpenAI {
	t.Helper()
	server := httptest.NewServer(replayKnowledgeChoice(func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			Messages []*schema.Message `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil || len(input.Messages) < 2 {
			http.Error(w, "invalid model input", 400)
			return
		}
		var payload chatmodel.ContextInput
		_ = json.Unmarshal([]byte(input.Messages[1].Content), &payload)
		if wb10BrowserResponse(w, r, payload) {
			return
		}
		if strings.Contains(payload.Question, "WB02_BROWSER_BLOCK") {
			<-r.Context().Done()
			return
		}
		if strings.Contains(payload.Question, "WB06_BROWSER_ANALYSIS") && len(payload.Attachments) == 1 {
			var toolMessages []*schema.Message
			for _, message := range input.Messages {
				if message.Role == schema.Tool {
					toolMessages = append(toolMessages, message)
				}
			}
			if len(toolMessages) == 1 {
				id := payload.Attachments[0].ID
				code := fmt.Sprintf(`import pandas as pd
import matplotlib
matplotlib.use("Agg")
import matplotlib.pyplot as plt
frame = pd.read_csv("/inputs/%s")
counts = frame.groupby("category").size().rename("count")
counts.to_csv("/work/output/browser-summary.csv")
counts.plot(kind="bar", color="#476f62")
plt.tight_layout()
plt.savefig("/work/output/browser-chart.png")
print("TOTAL="+str(int(counts.sum())))
`, id)
				args, _ := json.Marshal(map[string]any{"code": code, "input_ids": []string{id}})
				_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{
					"finish_reason": "tool_calls",
					"message": map[string]any{"role": "assistant", "content": nil,
						"tool_calls": []schema.ToolCall{{
							ID: "wb06-browser-python", Type: "function",
							Function: schema.FunctionCall{Name: einoflow.RunPythonToolName, Arguments: string(args)},
						}}},
				}}})
				return
			}
			var result domain.PythonAnalysisResult
			_ = json.Unmarshal([]byte(toolMessages[len(toolMessages)-1].Content), &result)
			if result.Status != "SUCCEEDED" || result.Source == nil {
				wbReply(w, `{"claims":[],"gaps":["浏览器 Python 执行失败。"]}`)
				return
			}
			raw, _ := json.Marshal(domain.Draft{Claims: []domain.Claim{{
				Text:      "浏览器实际执行完成，共统计 4 条记录并生成 CSV 与柱状图。",
				SourceIDs: []string{result.Source.SourceID},
			}}, Gaps: []string{}})
			wbReply(w, string(raw))
			return
		}
		if strings.Contains(payload.Question, "WB05_BROWSER_STREAM") && len(payload.Documents) > 0 {
			w.Header().Set("Content-Type", "text/event-stream")
			fragmentID, _ := json.Marshal(payload.Documents[0].ID)
			wbStreamFrame(w, map[string]any{"choices": []any{map[string]any{
				"index": 0, "delta": map[string]any{"content": `{"claims":[{"text":"浏览器真实`},
			}}})
			time.Sleep(350 * time.Millisecond)
			wbStreamFrame(w, map[string]any{"choices": []any{map[string]any{
				"index": 0, "delta": map[string]any{"content": `增量正文。","fragment_ids":[` +
					string(fragmentID) + `]}],"gaps":[],"conflicts":[]}`},
			}}})
			time.Sleep(350 * time.Millisecond)
			wbStreamFrame(w, map[string]any{"choices": []any{map[string]any{
				"index": 0, "delta": map[string]any{}, "finish_reason": "stop",
			}}})
			wbStreamDone(w)
			return
		}
		if strings.Contains(payload.Question, "WB07_BROWSER_SECURITY") && len(payload.Documents) > 0 {
			text := "安全渲染检查\n\n" +
				"![远程像素](https://wb07.invalid/pixel.png)\n\n" +
				"[外部链接](https://wb07.invalid/details)\n\n" +
				"<script>window.__wb07_xss = true</script>\n\n" +
				"```text\nLONG_CODE_" + strings.Repeat("x", 1200) + "\n```"
			raw, _ := json.Marshal(domain.Draft{Claims: []domain.Claim{{
				Text: text, FragmentIDs: []string{payload.Documents[0].ID},
			}}})
			wbReply(w, string(raw))
			return
		}
		draft := domain.Draft{Claims: []domain.Claim{}, Gaps: []string{"没有合适的公开资料。"}}
		if len(payload.Documents) > 0 {
			draft = domain.Draft{Claims: []domain.Claim{{Text: "合成手册说明：蓝灯表示维护模式。", FragmentIDs: []string{payload.Documents[0].ID}}}}
		}
		raw, _ := json.Marshal(draft)
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{
			"message": map[string]string{"content": string(raw)}, "finish_reason": "stop",
		}}, "usage": map[string]int{"prompt_tokens": 30, "completion_tokens": 12, "total_tokens": 42}})
	}))
	t.Cleanup(server.Close)
	cm, err := chatmodel.NewOpenAI(server.URL, "wb01-external-replay", "")
	if err != nil {
		t.Fatal(err)
	}
	return cm
}

func wbStart(t *testing.T, dsn string, cm *chatmodel.OpenAI, bootstrap bool, modes ...string) (*httptest.Server, *identity.Service, func()) {
	return wbStartOptions(t, dsn, cm, bootstrap, application.Options{UsersMode: true}, modes...)
}

func wbStartOptions(t *testing.T, dsn string, cm *chatmodel.OpenAI, bootstrap bool, options application.Options, modes ...string) (*httptest.Server, *identity.Service, func()) {
	t.Helper()
	store, err := postgres.Open(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	accounts, err := identity.New(store.Pool())
	if err != nil {
		t.Fatal(err)
	}
	if bootstrap {
		if _, err = accounts.CreateUser(context.Background(), "", "admin", wbPassword, "ADMIN", true); err != nil {
			t.Fatal(err)
		}
	}
	mode := "REPLAY"
	if len(modes) > 0 {
		mode = modes[0]
	}
	options.UsersMode = true
	app, err := application.New(store, cm, mode, options)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewUnstartedServer(nil)
	handler, err := httpapi.NewUsers(app, accounts, httpapi.UsersConfig{Origin: "https://" + server.Listener.Addr().String()})
	if err != nil {
		t.Fatal(err)
	}
	server.Config.Handler = handler
	server.StartTLS()
	return server, accounts, func() { server.Close(); app.Close(); store.Close() }
}

func TestWB01IdentityPrivateHTTPAndRestart(t *testing.T) {
	dsn, cm := workbenchDatabase(t), wbModel(t)
	server, accounts, stop := wbStart(t, dsn, cm, true)
	defer func() { stop() }()
	admin, alice, bob := wbNewClient(t, server), wbNewClient(t, server), wbNewClient(t, server)
	anonymous := wbNewClient(t, server)
	anonymous.request("GET", "/v1/auth/me", nil, 401)
	anonymous.request("POST", "/v1/auth/login", map[string]string{"username": "admin", "password": wbPassword}, 403, map[string]string{"Origin": "https://foreign.test"})
	admin.login("admin", wbPassword)
	if _, err := accounts.CreateUser(context.Background(), "", "second", wbPassword, "ADMIN", true); err == nil {
		t.Fatal("bootstrap allowed a second administrator")
	}
	au := admin.request("POST", "/v1/admin/users", map[string]string{"username": "alice", "password": wbPassword, "role": "USER"}, 201)
	bu := admin.request("POST", "/v1/admin/users", map[string]string{"username": "bob", "password": wbPassword, "role": "USER"}, 201)
	alice.login("alice", wbPassword)
	bob.login("bob", wbPassword)
	alice.request("GET", "/v1/admin/users", nil, 403)
	alice.request("POST", "/v1/conversations", map[string]string{"owner": bu["id"].(string)}, 400)
	alice.request("POST", "/v1/conversations", map[string]any{}, 403, map[string]string{"X-CSRF-Token": ""})
	alice.request("POST", "/v1/conversations", map[string]any{}, 403, map[string]string{"Origin": ""})
	alice.request("GET", "/v1/auth/me", nil, 401, map[string]string{"Authorization": "Bearer " + token})
	alice.request("POST", "/v1/incidents", map[string]any{}, 404)
	admin.request("GET", "/v1/runs/unknown", nil, 404)
	alice.request("POST", "/v1/knowledge/revisions", map[string]any{}, 403)
	alice.request("PUT", "/v1/devices/unknown", map[string]any{}, 403)
	revision := admin.request("POST", "/v1/knowledge/revisions", map[string]any{
		"title": "WB01 合成手册", "source": "fixture://wb01", "content": "# 蓝灯\n合成设备蓝灯表示维护模式。",
		"applicability": map[string]string{"scope": "GENERAL"},
	}, 201)
	revisionID := revision["id"].(string)
	alice.request("GET", "/v1/knowledge/revisions/"+revisionID, nil, 404)
	admin.request("POST", "/v1/knowledge/revisions/"+revisionID+"/publication", map[string]string{"decision": "PUBLISH"}, 200)
	conversation := alice.request("POST", "/v1/conversations", map[string]any{}, 201)
	cid := conversation["id"].(string)
	if conversation["owner"] != au["id"] {
		t.Fatal("conversation owner does not match authenticated user")
	}
	pending := alice.request("POST", "/v1/conversations/"+cid+"/messages", map[string]string{"text": "蓝灯表示什么？"}, 202, map[string]string{"Idempotency-Key": "alice-key"})
	rid := pending["id"].(string)
	answer := alice.answer(rid)
	if answer["status"] != "ANSWERED" || answer["data_mode"] != "REPLAY" || len(answer["citations"].([]any)) != 1 {
		t.Fatalf("external REPLAY model did not answer: %v", answer)
	}
	for _, other := range []*wbClient{bob, admin} {
		for _, path := range []string{"/v1/conversations/" + cid, "/v1/responses/" + rid, "/v1/responses/" + rid + "/events"} {
			other.request("GET", path, nil, 404)
		}
		other.request("POST", "/v1/conversations/"+cid+"/messages", map[string]string{"text": "蓝灯表示什么？"}, 404, map[string]string{"Idempotency-Key": "alice-key"})
	}
	same := alice.request("POST", "/v1/conversations/"+cid+"/messages", map[string]string{"text": "蓝灯表示什么？"}, 202, map[string]string{"Idempotency-Key": "alice-key"})
	if same["id"] != rid {
		t.Fatal("retry created another task")
	}
	bc := bob.request("POST", "/v1/conversations", map[string]any{}, 201)
	bp := bob.request("POST", "/v1/conversations/"+bc["id"].(string)+"/messages", map[string]string{"text": "蓝灯表示什么？"}, 202)
	if bob.answer(bp["id"].(string))["status"] != "ANSWERED" {
		t.Fatal("second user did not complete their own answer")
	}
	// A withdrawn citation remains available only to an owner who actually used it.
	admin.request("POST", "/v1/knowledge/revisions/"+revisionID+"/publication", map[string]string{"decision": "WITHDRAW"}, 200)
	alice.request("GET", "/v1/knowledge/revisions/"+revisionID, nil, 200)
	admin.request("POST", "/v1/admin/users/"+au["id"].(string)+"/password-reset", map[string]string{"password": "changed-synthetic-password"}, 200)
	alice.request("GET", "/v1/auth/me", nil, 401)
	alice.request("POST", "/v1/auth/login", map[string]string{"username": "alice", "password": wbPassword}, 401)
	alice.login("alice", "changed-synthetic-password")
	admin.request("PATCH", "/v1/admin/users/"+bu["id"].(string), map[string]bool{"active": false}, 200)
	bob.request("GET", "/v1/responses/"+bp["id"].(string), nil, 401)
	bob.request("POST", "/v1/auth/login", map[string]string{"username": "bob", "password": wbPassword}, 401)
	admin.request("PATCH", "/v1/admin/users/"+bu["id"].(string), map[string]bool{"active": true}, 200)
	bob.login("bob", wbPassword)
	adminID := admin.request("GET", "/v1/auth/me", nil, 200)["user"].(map[string]any)["id"].(string)
	admin.request("PATCH", "/v1/admin/users/"+adminID, map[string]bool{"active": false}, 409)
	bob.request("POST", "/v1/auth/logout", map[string]any{}, 200)
	bob.request("GET", "/v1/auth/me", nil, 401)
	for i := 0; i < 6; i++ {
		status := 401
		if i == 5 {
			status = 429
		}
		anonymous.request("POST", "/v1/auth/login", map[string]string{"username": "unknown", "password": wbPassword}, status)
	}
	// Keep the real account/session records across a complete app + DB pool restart.
	oldURL, _ := url.Parse(server.URL)
	cookies := alice.client.Jar.Cookies(oldURL)
	stop()
	server, _, stop = wbStart(t, dsn, cm, false)
	restarted := wbNewClient(t, server)
	newURL, _ := url.Parse(server.URL)
	restarted.client.Jar.SetCookies(newURL, cookies)
	restarted.csrf = alice.csrf
	restarted.request("GET", "/v1/auth/me", nil, 200)
	saved := restarted.request("GET", "/v1/responses/"+rid, nil, 200)
	if saved["answer"] != answer["answer"] {
		t.Fatal("restart lost private answer")
	}
	t.Log("REPLAY: real temporary PostgreSQL; external model adapter; 2 users + admin; auth, CSRF, isolation, resets, revocation and restart passed")
}

func TestWB01LegacyAdoptionAndMigrationDrift(t *testing.T) {
	dsn := workbenchDatabase(t)
	ctx := context.Background()
	db, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close(ctx)
	if _, err = db.Exec(ctx, migrations.Core+migrations.Devices+migrations.ResponseEvents+migrations.Diagnosis); err != nil {
		t.Fatal(err)
	}
	legacy := domain.Conversation{SchemaVersion: 1, ID: "legacy-conversation", Owner: "local-operator", CreatedAt: time.Now().UTC()}
	raw, _ := json.Marshal(legacy)
	if _, err = db.Exec(ctx, "INSERT INTO conversations(id,payload) VALUES ($1,$2)", legacy.ID, raw); err != nil {
		t.Fatal(err)
	}
	store, err := postgres.Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	accounts, _ := identity.New(store.Pool())
	user, err := accounts.CreateUser(ctx, "", "admin", wbPassword, "ADMIN", true)
	if err != nil {
		t.Fatal(err)
	}
	scoped := domain.WithUser(ctx, user)
	if _, err = store.GetConversation(scoped, legacy.ID); err != domain.ErrNotFound {
		t.Fatalf("unmapped history became visible: %v", err)
	}
	if count, err := store.MapLegacyOwner(ctx, "local-operator", "admin"); err != nil || count != 1 {
		t.Fatalf("explicit mapping failed: %d %v", count, err)
	}
	if c, err := store.GetConversation(scoped, legacy.ID); err != nil || c.Owner != user.ID {
		t.Fatalf("mapped owner not persisted: %+v %v", c, err)
	}
	if count, err := store.MapLegacyOwner(ctx, "local-operator", "admin"); err != nil || count != 0 {
		t.Fatal("mapping is not idempotent")
	}
	store.Close()
	var versions int
	if err = db.QueryRow(ctx, "SELECT count(*) FROM hwops_migrations").Scan(&versions); err != nil || versions != 8 {
		t.Fatalf("migration adoption missing: %d %v", versions, err)
	}
	if _, err = db.Exec(ctx, "ALTER TABLE users ALTER COLUMN password_hash DROP NOT NULL"); err != nil {
		t.Fatal(err)
	}
	if invalid, err := postgres.Open(ctx, dsn); err == nil {
		invalid.Close()
		t.Fatal("schema drift was silently accepted")
	}
	var count int
	if err = db.QueryRow(ctx, "SELECT count(*) FROM conversations").Scan(&count); err != nil || count != 1 {
		t.Fatal("rejected migration changed business records")
	}
	t.Log("real PostgreSQL: legacy adoption, hidden history, explicit mapping and fail-closed schema drift passed")
}

func TestWB01RevocationClosesActiveStream(t *testing.T) {
	modelServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		<-r.Context().Done()
	}))
	defer modelServer.Close()
	cm, _ := chatmodel.NewOpenAI(modelServer.URL, "blocked-replay", "")
	server, _, stop := wbStart(t, workbenchDatabase(t), cm, true)
	defer stop()
	admin, user := wbNewClient(t, server), wbNewClient(t, server)
	admin.login("admin", wbPassword)
	u := admin.request("POST", "/v1/admin/users", map[string]string{"username": "stream-user", "password": wbPassword, "role": "USER"}, 201)
	user.login("stream-user", wbPassword)
	c := user.request("POST", "/v1/conversations", map[string]any{}, 201)
	p := user.request("POST", "/v1/conversations/"+c["id"].(string)+"/messages", map[string]string{"text": "合成问题"}, 202)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", user.base+"/v1/responses/"+p["id"].(string)+"/events", nil)
	res, err := user.client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	scanner := bufio.NewScanner(res.Body)
	if !scanner.Scan() || !strings.HasPrefix(scanner.Text(), "id: ") {
		t.Fatal("stream did not publish accepted event")
	}
	admin.request("PATCH", "/v1/admin/users/"+u["id"].(string), map[string]bool{"active": false}, 200)
	for scanner.Scan() {
	}
	if scanner.Err() != nil || ctx.Err() != nil {
		t.Fatalf("revoked stream was not closed: %v", scanner.Err())
	}
}

func TestWB01LiveModel(t *testing.T) {
	if os.Getenv("HWOPS_TEST_LIVE_MODEL") != "1" {
		t.Skip("real model integration is separate; set HWOPS_TEST_LIVE_MODEL=1")
	}
	cm, err := chatmodel.NewOpenAI(os.Getenv("HWOPS_MODEL_ENDPOINT"), os.Getenv("HWOPS_MODEL"), os.Getenv("HWOPS_MODEL_API_KEY"))
	if err != nil {
		t.Fatal(err)
	}
	server, _, stop := wbStart(t, workbenchDatabase(t), cm, true, "LIVE")
	defer stop()
	admin := wbNewClient(t, server)
	admin.login("admin", wbPassword)
	r := admin.request("POST", "/v1/knowledge/revisions", map[string]any{
		"title": "LIVE 合成协议测试", "source": "fixture://wb01-live", "content": "# 蓝灯\n合成测试设备的蓝灯表示维护模式。",
		"applicability": map[string]string{"scope": "GENERAL"},
	}, 201)
	admin.request("POST", "/v1/knowledge/revisions/"+r["id"].(string)+"/publication", map[string]string{"decision": "PUBLISH"}, 200)
	c := admin.request("POST", "/v1/conversations", map[string]any{}, 201)
	p := admin.request("POST", "/v1/conversations/"+c["id"].(string)+"/messages", map[string]string{"text": "根据合成手册，测试设备的蓝灯表示什么？"}, 202)
	responseID := p["id"].(string)
	stream := admin.openEvents(responseID, 0)
	var events []domain.ResponseEvent
	for event := range stream.events {
		events = append(events, event)
	}
	a := admin.answer(responseID, 65*time.Second)
	if a["status"] != "ANSWERED" || a["data_mode"] != "LIVE" || !strings.Contains(a["answer"].(string), "维护模式") ||
		len(a["knowledge_tool_calls"].([]any)) == 0 || len(a["citations"].([]any)) != 1 {
		t.Fatalf("LIVE flow failed: status=%v error=%v", a["status"], a["error"])
	}
	var deltas int
	var retracted bool
	for _, event := range events {
		if event.Type == "answer_delta" {
			deltas++
			if strings.Contains(event.Delta, "fragment_ids") || strings.Contains(event.Delta, knowledgeagent.ToolName) {
				t.Fatal("LIVE stream leaked internal model structure")
			}
		}
		retracted = retracted || event.Type == "draft_retracted"
	}
	if deltas < 2 || !retracted {
		t.Fatalf("LIVE model did not provide verifiable native increments: deltas=%d retracted=%v", deltas, retracted)
	}
	t.Logf("LIVE: real PostgreSQL + username login + native model stream/tools + cited answer; model=%s deltas=%d usage=%v",
		os.Getenv("HWOPS_MODEL"), deltas, a["model_usage"])
}

func TestWB01PrivateContentDoesNotEnterTraces(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	previous := otel.GetTracerProvider()
	otel.SetTracerProvider(provider)
	defer func() { otel.SetTracerProvider(previous); _ = provider.Shutdown(context.Background()) }()
	server, _, stop := wbStart(t, workbenchDatabase(t), wbModel(t), true)
	admin := wbNewClient(t, server)
	admin.login("admin", wbPassword)
	c := admin.request("POST", "/v1/conversations", map[string]any{}, 201)
	p := admin.request("POST", "/v1/conversations/"+c["id"].(string)+"/messages", map[string]string{"text": "PRIVATE_TRACE_SENTINEL 蓝灯"}, 202)
	admin.answer(p["id"].(string))
	stop()
	spans := exporter.GetSpans()
	if len(spans) < 3 {
		t.Fatal("trace capture did not exercise the model and knowledge flow")
	}
	raw, _ := json.Marshal(spans)
	if strings.Contains(string(raw), "PRIVATE_TRACE_SENTINEL") || strings.Contains(string(raw), wbPassword) ||
		strings.Contains(string(raw), `"input.value"`) || strings.Contains(string(raw), `"output.value"`) {
		t.Fatal("users mode exported private content")
	}
}
