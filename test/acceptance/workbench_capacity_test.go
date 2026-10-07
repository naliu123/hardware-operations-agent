package acceptance_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cloudwego/eino/schema"

	"hwops/internal/adapters/chatmodel"
	"hwops/internal/application"
	"hwops/internal/blobstore"
	"hwops/internal/domain"
)

type capacityMetric struct {
	accepted   time.Time
	started    time.Time
	firstDelta time.Time
	terminal   time.Time
	execution  string
	status     string
	deltas     int
}

func TestWB08TenActiveConversationsTwoPythonExecutions(t *testing.T) {
	runner := wbRunner(t)
	modelServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			Messages []*schema.Message `json:"messages"`
			Stream   bool              `json:"stream"`
		}
		if json.NewDecoder(r.Body).Decode(&input) != nil || !input.Stream || len(input.Messages) < 2 {
			http.Error(w, "native stream required", http.StatusBadRequest)
			return
		}
		var payload chatmodel.ContextInput
		_ = json.Unmarshal([]byte(input.Messages[1].Content), &payload)
		var tools []*schema.Message
		for _, message := range input.Messages {
			if message.Role == schema.Tool {
				tools = append(tools, message)
			}
		}
		w.Header().Set("Content-Type", "text/event-stream")
		if len(tools) == 0 {
			code := fmt.Sprintf("import time\ntime.sleep(1.2)\nprint(%q)\n", payload.Question)
			wb06ToolCall(w, "wb08-python-"+strings.TrimPrefix(payload.Question, "WB08_CAPACITY_"),
				einofflowPython(), map[string]any{"code": code})
			return
		}
		var result domain.PythonAnalysisResult
		_ = json.Unmarshal([]byte(tools[len(tools)-1].Content), &result)
		if result.Status != "SUCCEEDED" || result.Source == nil ||
			!strings.Contains(result.Execution.Result.Stdout, payload.Question) {
			http.Error(w, "capacity execution failed", http.StatusBadRequest)
			return
		}
		wb06Final(w, domain.Draft{Claims: []domain.Claim{{
			Text: payload.Question + " 已完成。", SourceIDs: []string{result.Source.SourceID},
		}}})
	}))
	defer modelServer.Close()
	cm, err := chatmodel.NewOpenAI(modelServer.URL, "wb08-capacity-replay", "")
	if err != nil {
		t.Fatal(err)
	}
	files, err := blobstore.Open(filepath.Join(t.TempDir(), "private"), 2_000_000_000, 100_000_000)
	if err != nil {
		t.Fatal(err)
	}
	server, _, stop := wbStartOptions(t, workbenchDatabase(t), cm, true, application.Options{
		UsersMode: true, Runner: runner, Files: files,
	})
	defer stop()
	client := wbNewClient(t, server)
	client.login("admin", wbPassword)

	const count = 10
	metrics := make([]capacityMetric, count)
	responseIDs := make([]string, count)
	streams := make([]*wbSSE, count)
	for i := range count {
		conversation := client.request("POST", "/v1/conversations", map[string]any{}, http.StatusCreated)
		label := fmt.Sprintf("WB08_CAPACITY_%02d", i)
		response := client.request("POST", "/v1/conversations/"+conversation["id"].(string)+"/messages",
			map[string]string{"text": label}, http.StatusAccepted)
		metrics[i].accepted = time.Now()
		responseIDs[i] = response["id"].(string)
		streams[i] = client.openEvents(responseIDs[i], 0)
	}
	defer func() {
		for _, stream := range streams {
			stream.close()
		}
	}()

	var mu sync.Mutex
	var collectors sync.WaitGroup
	collectors.Add(count)
	for i, stream := range streams {
		go func() {
			defer collectors.Done()
			for event := range stream.events {
				now := time.Now()
				mu.Lock()
				if event.Type == "tool_progress" && event.Execution != nil && metrics[i].execution == "" {
					metrics[i].execution = event.Execution.ID
				}
				if event.Type == "answer_delta" {
					metrics[i].deltas++
					if metrics[i].firstDelta.IsZero() {
						metrics[i].firstDelta = now
					}
				}
				if event.Response != nil && domain.ResponseTerminal(event.Response.Status) {
					metrics[i].terminal = now
					metrics[i].status = event.Response.Status
				}
				mu.Unlock()
			}
		}()
	}

	for deadline := time.Now().Add(10 * time.Second); ; {
		mu.Lock()
		known := 0
		for i := range metrics {
			if metrics[i].execution != "" {
				known++
			}
		}
		mu.Unlock()
		if known == count {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("only %d/%d capacity executions were created", known, count)
		}
		time.Sleep(50 * time.Millisecond)
	}

	maxRunning, maxStartingAndRunning, maxQueued := 0, 0, 0
	queuedSeen := map[string]bool{}
	for deadline := time.Now().Add(90 * time.Second); ; {
		running, starting, queued, terminal := 0, 0, 0, 0
		mu.Lock()
		ids := make([]string, count)
		for i := range metrics {
			ids[i] = metrics[i].execution
		}
		mu.Unlock()
		for i, id := range ids {
			execution := client.request("GET", "/v1/executions/"+id, nil, http.StatusOK)
			switch execution["status"] {
			case "QUEUED":
				queued++
				queuedSeen[id] = true
			case "STARTING":
				starting++
			case "RUNNING":
				running++
				mu.Lock()
				if metrics[i].started.IsZero() {
					metrics[i].started = time.Now()
				}
				mu.Unlock()
			case "SUCCEEDED", "FAILED", "CANCELED", "INTERRUPTED":
				terminal++
			}
		}
		maxRunning = max(maxRunning, running)
		maxStartingAndRunning = max(maxStartingAndRunning, running+starting)
		maxQueued = max(maxQueued, queued)
		if running > 2 || running+starting > 2 {
			t.Fatalf("Python concurrency exceeded: running=%d starting=%d", running, starting)
		}
		if terminal == count {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("capacity run timed out: terminal=%d running=%d starting=%d queued=%d",
				terminal, running, starting, queued)
		}
		time.Sleep(75 * time.Millisecond)
	}

	done := make(chan struct{})
	go func() {
		collectors.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("terminal SSE events did not close")
	}

	queueDurations := make([]time.Duration, 0, count)
	firstDeltas := make([]time.Duration, 0, count)
	endToEnd := make([]time.Duration, 0, count)
	for i := range count {
		answer := client.answer(responseIDs[i], 5*time.Second)
		if answer["status"] != "ANSWERED" ||
			!strings.Contains(answer["answer"].(string), fmt.Sprintf("WB08_CAPACITY_%02d", i)) {
			t.Fatalf("capacity response %d failed: %v", i, answer)
		}
		mu.Lock()
		metric := metrics[i]
		mu.Unlock()
		if metric.status != "ANSWERED" || metric.deltas < 1 || metric.started.IsZero() ||
			metric.firstDelta.IsZero() || metric.terminal.IsZero() {
			t.Fatalf("capacity metrics incomplete for %d: %+v", i, metric)
		}
		queueDurations = append(queueDurations, metric.started.Sub(metric.accepted))
		firstDeltas = append(firstDeltas, metric.firstDelta.Sub(metric.accepted))
		endToEnd = append(endToEnd, metric.terminal.Sub(metric.accepted))
	}
	sort.Slice(queueDurations, func(i, j int) bool { return queueDurations[i] < queueDurations[j] })
	sort.Slice(firstDeltas, func(i, j int) bool { return firstDeltas[i] < firstDeltas[j] })
	sort.Slice(endToEnd, func(i, j int) bool { return endToEnd[i] < endToEnd[j] })
	if maxRunning != 2 || maxStartingAndRunning != 2 || maxQueued < 6 || len(queuedSeen) < 8 {
		t.Fatalf("capacity distribution not exercised: running=%d active=%d queued=%d queued_ids=%d",
			maxRunning, maxStartingAndRunning, maxQueued, len(queuedSeen))
	}
	t.Logf("REPLAY MODEL + ACTUAL SANDBOX CAPACITY: accepted=%d answered=%d failed=0 timeout=0 "+
		"max_python_running=%d max_starting_or_running=%d max_queued=%d queued_executions=%d "+
		"queue_wait_p50=%s queue_wait_p95=%s first_delta_p50=%s first_delta_p95=%s "+
		"end_to_end_p50=%s end_to_end_p95=%s",
		count, count, maxRunning, maxStartingAndRunning, maxQueued, len(queuedSeen),
		queueDurations[4].Round(time.Millisecond), queueDurations[9].Round(time.Millisecond),
		firstDeltas[4].Round(time.Millisecond), firstDeltas[9].Round(time.Millisecond),
		endToEnd[4].Round(time.Millisecond), endToEnd[9].Round(time.Millisecond))
}
