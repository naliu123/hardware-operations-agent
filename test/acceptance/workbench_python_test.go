package acceptance_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"hwops/internal/adapters/chatmodel"
	"hwops/internal/adapters/postgres"
	"hwops/internal/application"
	"hwops/internal/blobstore"
	"hwops/internal/domain"
	"hwops/internal/identity"
	"hwops/internal/sandbox"
	"hwops/internal/transport/httpapi"
)

func wbRunner(t *testing.T) *sandbox.Client {
	t.Helper()
	endpoint := os.Getenv("HWOPS_TEST_RUNNER_URL")
	if endpoint == "" {
		t.Skip("actual Linux + gVisor runner required; HWOPS_TEST_RUNNER_URL unset")
	}
	raw, err := os.ReadFile(os.Getenv("HWOPS_TEST_RUNNER_TOKEN_FILE"))
	if err != nil {
		t.Fatal("runner token file unavailable")
	}
	runner, err := sandbox.NewClient(endpoint, strings.TrimSpace(string(raw)))
	if err != nil {
		t.Fatal(err)
	}
	cap, err := runner.Capability(context.Background())
	if err != nil || !cap.Ready {
		t.Fatalf("actual runner not ready: %v", err)
	}
	t.Logf("ACTUAL SANDBOX: runtime=%s kernel=%s image=%s probe=%s", cap.Runtime, cap.Linux, cap.Image, cap.ProbeSHA256)
	return runner
}

type pythonFixture struct {
	app               *application.App
	store             *postgres.Store
	admin, alice, bob *wbClient
	owner             domain.User
	runner            *sandbox.Client
	server            *httptest.Server
	files             *blobstore.Store
	model             *chatmodel.OpenAI
	dsn               string
	stop              func()
}

func newPythonFixture(t *testing.T) *pythonFixture {
	t.Helper()
	runner := wbRunner(t)
	modelServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		<-r.Context().Done()
	}))
	t.Cleanup(modelServer.Close)
	cm, _ := chatmodel.NewOpenAI(modelServer.URL, "wb03-replay-blocked", "")
	dsn := workbenchDatabase(t)
	store, err := postgres.Open(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	accounts, _ := identity.New(store.Pool())
	_, err = accounts.CreateUser(context.Background(), "", "admin", wbPassword, "ADMIN", true)
	if err != nil {
		t.Fatal(err)
	}
	files, err := blobstore.Open(filepath.Join(t.TempDir(), "private"), 2_000_000_000, 100_000_000)
	if err != nil {
		t.Fatal(err)
	}
	app, err := application.New(store, cm, "REPLAY", application.Options{UsersMode: true, Runner: runner, Files: files})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewUnstartedServer(nil)
	server.Config.Handler, err = httpapi.NewUsers(app, accounts, httpapi.UsersConfig{Origin: "https://" + server.Listener.Addr().String()})
	if err != nil {
		t.Fatal(err)
	}
	server.StartTLS()
	f := &pythonFixture{app: app, store: store, runner: runner, server: server, files: files, model: cm, dsn: dsn,
		admin: wbNewClient(t, server), alice: wbNewClient(t, server), bob: wbNewClient(t, server)}
	f.stop = func() { f.server.Close(); f.app.Close(); f.store.Close() }
	t.Cleanup(func() {
		pending, _ := f.store.PendingPythonExecutions(context.Background())
		for _, e := range pending {
			_, _ = runner.Cancel(context.Background(), e.ID)
			_ = runner.Forget(context.Background(), e.ID)
		}
		f.stop()
	})
	f.admin.login("admin", wbPassword)
	user := f.admin.request("POST", "/v1/admin/users", map[string]string{"username": "alice", "password": wbPassword, "role": "USER"}, 201)
	f.admin.request("POST", "/v1/admin/users", map[string]string{"username": "bob", "password": wbPassword, "role": "USER"}, 201)
	f.alice.login("alice", wbPassword)
	f.bob.login("bob", wbPassword)
	f.owner = domain.User{ID: user["id"].(string), Role: "USER", Active: true}
	return f
}

func (f *pythonFixture) response(t *testing.T) (string, string, context.Context) {
	t.Helper()
	c := f.alice.request("POST", "/v1/conversations", map[string]any{}, 201)
	cid := c["id"].(string)
	r := f.alice.request("POST", "/v1/conversations/"+cid+"/messages", map[string]string{"text": "WB03 执行验收"}, 202)
	rid := r["id"].(string)
	for end := time.Now().Add(3 * time.Second); time.Now().Before(end); {
		if f.alice.request("GET", "/v1/responses/"+rid, nil, 200)["status"] == "RUNNING" {
			return cid, rid, domain.WithUser(context.Background(), f.owner)
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("response did not start")
	return "", "", nil
}

func (f *pythonFixture) restart(t *testing.T) {
	t.Helper()
	f.stop()
	store, err := postgres.Open(context.Background(), f.dsn)
	if err != nil {
		t.Fatal(err)
	}
	accounts, err := identity.New(store.Pool())
	if err != nil {
		t.Fatal(err)
	}
	app, err := application.New(store, f.model, "REPLAY", application.Options{
		UsersMode: true, Runner: f.runner, Files: f.files,
	})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewUnstartedServer(nil)
	server.Config.Handler, err = httpapi.NewUsers(app, accounts, httpapi.UsersConfig{
		Origin: "https://" + server.Listener.Addr().String(),
	})
	if err != nil {
		t.Fatal(err)
	}
	server.StartTLS()
	f.store, f.app, f.server = store, app, server
	f.admin, f.alice, f.bob = wbNewClient(t, server), wbNewClient(t, server), wbNewClient(t, server)
	f.alice.login("alice", wbPassword)
}

func (f *pythonFixture) run(t *testing.T, ctx context.Context, rid, code string, inputs ...string) domain.PythonExecution {
	t.Helper()
	e, err := f.app.StartPython(ctx, rid, code, inputs)
	if err != nil {
		t.Fatal(err)
	}
	wait, cancel := context.WithTimeout(ctx, 80*time.Second)
	defer cancel()
	e, err = f.app.WaitPython(wait, e.ID)
	if err != nil {
		t.Fatal(err)
	}
	preview := e.Result.Stdout
	if len(preview) > 600 {
		preview = preview[:600]
	}
	t.Logf("ACTUAL SANDBOX execution=%s state=%s code_hash=%s duration_ms=%d exit=%v error=%s usage=%+v stdout_bytes=%d stdout_preview=%q",
		e.ID, e.Status, e.CodeSHA256, e.Result.DurationMS, e.Result.ExitCode, e.Result.Error,
		e.Result.ResourceUsage, len(e.Result.Stdout), preview)
	return e
}

func TestWB03ActualStatisticsArtifactsAndPrivacy(t *testing.T) {
	f := newPythonFixture(t)
	cid, rid, ctx := f.response(t)
	e := f.run(t, ctx, rid, `import pandas as pd
import matplotlib.pyplot as plt
d=pd.DataFrame({"hour":[0,1,2],"errors":[2,5,3]})
d.to_csv("/work/output/errors.csv",index=False)
d.plot(x="hour",y="errors")
plt.savefig("/work/output/chart.png")
print("TOTAL="+str(int(d.errors.sum())))
`)
	if e.Status != "SUCCEEDED" || !strings.Contains(e.Result.Stdout, "TOTAL=10") || len(e.Result.Artifacts) != 2 ||
		!e.Result.Cleaned || !e.Result.Complete {
		t.Fatalf("statistics failed: %+v", e.Result)
	}
	var csv domain.Artifact
	for _, a := range e.Result.Artifacts {
		t.Logf("ACTUAL ARTIFACT execution=%s id=%s name=%s sha256=%s bytes=%d media_type=%s",
			e.ID, a.ID, a.Name, a.SHA256, a.Bytes, a.MediaType)
		if a.Name == "errors.csv" {
			csv = a
		}
		req, _ := http.NewRequest("GET", f.alice.base+"/v1/artifacts/"+a.ID+"/content", nil)
		req.Header.Set("Range", "bytes=0-7")
		res, err := f.alice.client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		data, _ := io.ReadAll(res.Body)
		res.Body.Close()
		if res.StatusCode != 206 || len(data) != 8 {
			t.Fatalf("artifact range failed: %d", res.StatusCode)
		}
		for _, other := range []*wbClient{f.bob, f.admin} {
			other.request("GET", "/v1/artifacts/"+a.ID+"/content", nil, 404)
		}
	}
	f.alice.request("GET", "/v1/executions/"+e.ID, nil, 200)
	f.bob.request("GET", "/v1/executions/"+e.ID, nil, 404)
	f.admin.request("GET", "/v1/executions/"+e.ID, nil, 404)
	next := f.run(t, ctx, rid, fmt.Sprintf("import pandas as pd\nprint('MEAN='+str(pd.read_csv('/inputs/%s').errors.mean()))", csv.ID), csv.ID)
	if next.Status != "SUCCEEDED" || len(next.Inputs) != 1 || next.Inputs[0].SHA256 != csv.SHA256 ||
		!strings.Contains(next.Result.Stdout, "3.3333") {
		t.Fatalf("artifact input failed: %+v", next)
	}
	t.Logf("ACTUAL INPUT execution=%s id=%s sha256=%s bytes=%d", next.ID,
		next.Inputs[0].ID, next.Inputs[0].SHA256, next.Inputs[0].Bytes)
	_, otherRID, _ := f.response(t)
	if _, err := f.app.StartPython(ctx, otherRID, "print('forbidden')", []string{csv.ID}); err == nil {
		t.Fatal("same owner's other conversation accessed artifact")
	}
	f.alice.request("DELETE", "/v1/conversations/"+cid, nil, 202)
	f.alice.request("GET", "/v1/executions/"+e.ID, nil, 404)
	f.alice.request("GET", "/v1/artifacts/"+csv.ID+"/content", nil, 404)
	for end := time.Now().Add(10 * time.Second); time.Now().Before(end); {
		var count int
		_ = f.store.Pool().QueryRow(context.Background(), "SELECT count(*) FROM python_executions WHERE conversation_id=$1", cid).Scan(&count)
		if count == 0 {
			t.Log("actual runner journals/files and application artifacts cleaned after deletion")
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("execution cleanup did not finish")
}

func TestWB03UnavailableNeverFallsBackToHostPython(t *testing.T) {
	store, err := postgres.Open(context.Background(), workbenchDatabase(t))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	app, err := application.New(store, wbModel(t), "REPLAY", application.Options{UsersMode: true})
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close()
	marker := filepath.Join(t.TempDir(), "must-not-exist")
	_, err = app.StartPython(domain.WithUser(context.Background(), domain.User{ID: "untrusted"}),
		"missing-response", "open("+fmt.Sprintf("%q", marker)+",'w').write('unsafe')", nil)
	if !errors.Is(err, sandbox.ErrUnavailable) {
		t.Fatalf("missing isolation did not fail closed: %v", err)
	}
	if _, statErr := os.Stat(marker); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatal("user code ran on the application host")
	}
}

func TestWB03ActualRunnerIdentityAndCancellation(t *testing.T) {
	runner := wbRunner(t)
	cap, _ := runner.Capability(context.Background())
	request := sandbox.Request{ID: fmt.Sprintf("WB03ID%020d", time.Now().UnixNano()), Code: "import time\nprint('started',flush=True)\ntime.sleep(30)",
		Image: cap.Image, Budget: domain.PythonLimits(), Deadline: time.Now().Add(time.Minute)}
	first, err := runner.Submit(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = runner.Cancel(context.Background(), request.ID)
		_ = runner.Forget(context.Background(), request.ID)
	})
	duplicate, err := runner.Submit(context.Background(), request)
	if err != nil || duplicate.RequestSHA256 != first.RequestSHA256 {
		t.Fatal("same identity did not deduplicate")
	}
	conflicting := request
	conflicting.Code = "print('different')"
	if _, err = runner.Submit(context.Background(), conflicting); err != sandbox.ErrConflict {
		t.Fatalf("identity conflict: %v", err)
	}
	time.Sleep(300 * time.Millisecond)
	stopped, err := runner.Cancel(context.Background(), request.ID)
	if err != nil || stopped.State != "CANCELED" || !stopped.Result.Cleaned || stopped.Result.Complete {
		t.Fatalf("cancellation not confirmed: %+v %v", stopped, err)
	}
	again, err := runner.Submit(context.Background(), request)
	if err != nil || again.State != "CANCELED" {
		t.Fatal("canceled identity started again")
	}
	request.ID = fmt.Sprintf("WB03PRE%020d", time.Now().UnixNano())
	t.Cleanup(func() { _ = runner.Forget(context.Background(), request.ID) })
	_, err = runner.Cancel(context.Background(), request.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = runner.Submit(context.Background(), request); err != sandbox.ErrConflict {
		t.Fatal("late submit bypassed cancel tombstone")
	}
}

func TestWB03ActualResourceLimits(t *testing.T) {
	f := newPythonFixture(t)
	tests := []struct{ name, code, want, errorCode string }{
		{"cpu", `import os,time,json
children=[]
begin=time.monotonic()
for _ in range(4):
 p=os.fork()
 if p==0:
  until=time.monotonic()+4
  while time.monotonic()<until: pass
  os._exit(0)
 children.append(p)
for p in children: os.waitpid(p,0)
elapsed=time.monotonic()-begin
used=os.times().children_user+os.times().children_system
assert used/elapsed<1.25,(used,elapsed)
print(json.dumps({"cpu_seconds":used,"wall_seconds":elapsed,"cores":used/elapsed}))`, "SUCCEEDED", ""},
		{"processes", `import os,time
pids=[]
try:
 for _ in range(100):
  p=os.fork()
  if p==0:
   time.sleep(8)
   os._exit(0)
  pids.append(p)
except OSError as e:
 print("LIMIT",len(pids),e.errno,flush=True)
finally:
 for p in pids:
  try: os.kill(p,9)
  except ProcessLookupError: pass
 for p in pids: os.waitpid(p,0)
assert 0<len(pids)<64`, "FAILED", "PROCESS_LIMIT_EXCEEDED"},
		{"workspace", `import os
written=0
try:
 with open("/work/tmp/fill","wb",buffering=0) as f:
  while True:
   f.write(b"x"*(1024*1024))
   written+=1024*1024
except OSError as e:
 assert e.errno==28
assert 250*1024*1024<=written<=256*1024*1024
print("ENOSPC",written)`, "SUCCEEDED", ""},
		{"memory", "x=bytearray(1500*1024*1024)\nprint('UNBOUNDED')", "FAILED", "MEMORY_LIMIT_EXCEEDED"},
		{"logs", "print('x'*(2*1024*1024),flush=True)\nimport time\ntime.sleep(30)", "FAILED", "OUTPUT_LIMIT_EXCEEDED"},
		{"symlink", "import os\nos.symlink('/etc/passwd','/work/output/leak')", "FAILED", "ARTIFACT_VALIDATION_FAILED"},
		{"hardlink", "import os\nopen('/work/tmp/data','w').write('x')\nos.link('/work/tmp/data','/work/output/link')", "FAILED", "ARTIFACT_VALIDATION_FAILED"},
		{"artifact-count", "for i in range(21): open('/work/output/'+str(i),'w').write('x')", "FAILED", "ARTIFACT_VALIDATION_FAILED"},
		{"artifact-bytes", "open('/work/output/large','wb').truncate(100000001)", "FAILED", "ARTIFACT_VALIDATION_FAILED"},
		{"wall-time", "import time\ntime.sleep(90)", "FAILED", "TIME_LIMIT_EXCEEDED"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cid, rid, ctx := f.response(t)
			e := f.run(t, ctx, rid, test.code)
			if e.Status != test.want || e.Result.Error != test.errorCode || !e.Result.Cleaned {
				t.Fatalf("resource limit mismatch: %+v", e.Result)
			}
			if test.name == "wall-time" && (e.Result.DurationMS < 59000 || e.Result.DurationMS > 70000) {
				t.Fatal("wall limit incorrect")
			}
			if test.name == "cpu" && (e.Result.ResourceUsage.CPUUsec <= 0 ||
				e.Result.ResourceUsage.CPUUsec > e.Result.DurationMS*1250) {
				t.Fatalf("actual cgroup CPU quota was not observed: %+v", e.Result.ResourceUsage)
			}
			if test.name == "processes" && (e.Result.ResourceUsage.ProcessesPeak != 64 ||
				e.Result.ResourceUsage.ProcessLimitEvents < 1) {
				t.Fatalf("actual pids limit was not observed: %+v", e.Result.ResourceUsage)
			}
			if test.name == "memory" && (e.Result.ResourceUsage.MemoryPeakBytes < 900_000_000 ||
				e.Result.ResourceUsage.MemoryPeakBytes > 1<<30) {
				t.Fatalf("actual memory limit was not observed: %+v", e.Result.ResourceUsage)
			}
			f.alice.request("DELETE", "/v1/conversations/"+cid, nil, 202)
		})
	}
}

func TestWB03ActualQueueAndPerResponseBudget(t *testing.T) {
	f := newPythonFixture(t)
	cid, rid, ctx := f.response(t)
	var runs []domain.PythonExecution
	for range 3 {
		e, err := f.app.StartPython(ctx, rid, "import time\ntime.sleep(40)", nil)
		if err != nil {
			t.Fatal(err)
		}
		runs = append(runs, e)
	}
	if _, err := f.app.StartPython(ctx, rid, "print(4)", nil); err == nil {
		t.Fatal("fourth attempt accepted")
	}
	var running, queued int
	for end := time.Now().Add(5 * time.Second); time.Now().Before(end); {
		running, queued = 0, 0
		for _, e := range runs {
			state := f.alice.request("GET", "/v1/executions/"+e.ID, nil, 200)
			if state["status"] == "RUNNING" {
				running++
			}
			if state["status"] == "QUEUED" {
				queued++
			}
		}
		if running == 2 && queued == 1 {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if running != 2 || queued != 1 {
		t.Fatalf("queue incorrect: running=%d queued=%d", running, queued)
	}
	f.alice.request("POST", "/v1/responses/"+rid+"/cancel", map[string]any{}, 202)
	if answer := f.alice.answer(rid, 10*time.Second); answer["status"] != "CANCELED" {
		t.Fatal("response cancellation did not finish")
	}
	for _, e := range runs {
		state := f.alice.request("GET", "/v1/executions/"+e.ID, nil, 200)
		raw, _ := json.Marshal(state)
		if state["status"] != "CANCELED" || state["result"].(map[string]any)["cleaned"] != true {
			t.Fatalf("execution was not reaped: %s", raw)
		}
	}
	nextCID, nextRID, nextCtx := f.response(t)
	next := f.run(t, nextCtx, nextRID, "print('CAPACITY_RELEASED')")
	if next.Status != "SUCCEEDED" || !strings.Contains(next.Result.Stdout, "CAPACITY_RELEASED") {
		t.Fatalf("capacity was not released after cleanup: %+v", next)
	}
	f.alice.request("POST", "/v1/responses/"+nextRID+"/cancel", map[string]any{}, 202)
	_ = f.alice.answer(nextRID, 10*time.Second)
	f.alice.request("DELETE", "/v1/conversations/"+nextCID, nil, 202)
	f.alice.request("DELETE", "/v1/conversations/"+cid, nil, 202)
	t.Log("ACTUAL SANDBOX: three durable requests, two running, one queued; fourth rejected; stop reaped both processes and canceled queue")
}

func TestWB03ApplicationRestartInterruptsWithoutRerun(t *testing.T) {
	f := newPythonFixture(t)
	_, rid, ctx := f.response(t)
	e, err := f.app.StartPython(ctx, rid, "import time\nprint('STARTED',flush=True)\ntime.sleep(40)", nil)
	if err != nil {
		t.Fatal(err)
	}
	for end := time.Now().Add(5 * time.Second); time.Now().Before(end); {
		state := f.alice.request("GET", "/v1/executions/"+e.ID, nil, 200)
		if state["status"] == "RUNNING" {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	before, err := f.runner.Get(context.Background(), e.ID)
	if err != nil || before.State != "RUNNING" || before.Result.StartedAt == nil {
		t.Fatalf("execution did not start before application restart: %+v %v", before, err)
	}
	started := *before.Result.StartedAt
	f.restart(t)
	for end := time.Now().Add(10 * time.Second); time.Now().Before(end); {
		state := f.alice.request("GET", "/v1/executions/"+e.ID, nil, 200)
		if state["status"] == "INTERRUPTED" {
			result := state["result"].(map[string]any)
			if result["cleaned"] != true || result["complete"] != false {
				t.Fatalf("restarted application published incomplete output: %v", result)
			}
			response := f.alice.request("GET", "/v1/responses/"+rid, nil, 200)
			if response["status"] != "INTERRUPTED" {
				t.Fatalf("response was not interrupted: %v", response["status"])
			}
			executions, _ := response["executions"].([]any)
			if len(executions) != 1 || executions[0].(map[string]any)["status"] != "INTERRUPTED" {
				t.Fatalf("interrupted response lost its execution record: %v", response)
			}
			after, getErr := f.runner.Get(context.Background(), e.ID)
			if getErr != nil || after.Result.StartedAt == nil || !after.Result.StartedAt.Equal(started) ||
				after.State != "CANCELED" || !after.Result.Cleaned {
				t.Fatalf("execution was rerun or not reaped: %+v %v", after, getErr)
			}
			t.Log("ACTUAL SANDBOX: application restart preserved execution identity, interrupted without rerun, and reaped the original container")
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("application restart did not reconcile execution")
}

func runnerControl(t *testing.T, profile, action, image string) {
	t.Helper()
	var cmd *exec.Cmd
	switch action {
	case "stop":
		cmd = exec.Command("colima", "ssh", "--profile", profile, "--", "sudo", "systemctl", "stop", "hwops-runner-test.service")
	case "start":
		cmd = exec.Command("colima", "ssh", "--profile", profile, "--", "sudo", "systemd-run",
			"--unit=hwops-runner-test", "--property=Restart=on-failure", "/usr/local/bin/hwops-runner",
			"-root", "/var/lib/hwops-runner", "-image", image, "-token-file", "/var/lib/hwops-runner/control.key")
	default:
		t.Fatal("unknown runner control action")
	}
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("runner %s failed: %v: %s", action, err, out)
	}
}

func TestWB03RunnerLossAndRestartReapsOrphan(t *testing.T) {
	profile := os.Getenv("HWOPS_TEST_RUNNER_COLIMA_PROFILE")
	if profile == "" {
		t.Skip("runner controller restart requires HWOPS_TEST_RUNNER_COLIMA_PROFILE")
	}
	runner := wbRunner(t)
	capability, _ := runner.Capability(context.Background())
	request := sandbox.Request{
		ID:    fmt.Sprintf("WB03LOSS%018d", time.Now().UnixNano()),
		Code:  "import time\nprint('STARTED',flush=True)\ntime.sleep(40)",
		Image: capability.Image, Budget: domain.PythonLimits(), Deadline: time.Now().Add(time.Minute),
	}
	if _, err := runner.Submit(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = runner.Cancel(context.Background(), request.ID)
		_ = runner.Forget(context.Background(), request.ID)
	})
	for end := time.Now().Add(5 * time.Second); time.Now().Before(end); {
		state, err := runner.Get(context.Background(), request.ID)
		if err == nil && state.State == "RUNNING" {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	runnerControl(t, profile, "stop", capability.Image)
	t.Cleanup(func() {
		if _, err := runner.Capability(context.Background()); err != nil {
			runnerControl(t, profile, "start", capability.Image)
		}
	})
	if _, err := runner.Get(context.Background(), request.ID); !errors.Is(err, sandbox.ErrUnavailable) {
		t.Fatalf("runner loss was reported as a known execution result: %v", err)
	}
	runnerControl(t, profile, "start", capability.Image)
	for end := time.Now().Add(15 * time.Second); time.Now().Before(end); {
		state, err := runner.Get(context.Background(), request.ID)
		if err == nil && state.State == "INTERRUPTED" && state.Result.Cleaned && !state.Result.Complete {
			t.Log("ACTUAL SANDBOX: runner loss stayed unknown; restart reconciled original ID and reaped the orphan")
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("runner restart did not reconcile the orphan")
}
