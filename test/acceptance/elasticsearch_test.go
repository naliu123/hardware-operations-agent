package acceptance_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"hwops/internal/adapters/chatmodel"
	"hwops/internal/adapters/elasticsearch"
	"hwops/internal/adapters/filestore"
	"hwops/internal/application"
	"hwops/internal/transport/httpapi"
)

func TestElasticsearchPublicFlow(t *testing.T) {
	endpoint := os.Getenv("HWOPS_TEST_ES_URL")
	if endpoint == "" {
		t.Skip("real Elasticsearch integration requires HWOPS_TEST_ES_URL and HWOPS_TEST_EMBED_URL")
	}
	index, err := elasticsearch.New(endpoint, os.Getenv("HWOPS_TEST_EMBED_URL"), fmt.Sprintf("hwops-test-%d", time.Now().UnixNano()), "bm25")
	if err != nil {
		t.Fatal(err)
	}
	if err := index.Ensure(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer index.Delete(context.Background())
	store, err := filestore.Open(filepath.Join(tempDir(t), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	// Actual Elasticsearch and embedding; deterministic model remains REPLAY.
	app, err := application.New(store, &chatmodel.Replay{}, "REPLAY", application.Options{Retriever: index, ContextLimit: 5})
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close()
	s := httptest.NewServer(httpapi.New(app, token))
	defer s.Close()
	id := addRevision(t, s, "GENERAL", "PUBLISH")
	for _, strategy := range []string{"bm25", "dense", "hybrid"} {
		result := request(t, s.Client(), "POST", s.URL+"/v1/knowledge/search",
			map[string]any{"query": "蓝灯表示什么？", "strategy": strategy, "top_k": 5}, http.StatusOK)
		docs := result["documents"].([]any)
		if len(docs) != 1 || docs[0].(map[string]any)["revision_id"] != id {
			t.Fatalf("%s: expected published knowledge: %v", strategy, result)
		}
	}
	answer := ask(t, s)
	if answer.Status != "ANSWERED" || answer.DataMode != "REPLAY" || len(answer.Citations) != 1 {
		t.Fatalf("ES answer lost citation: %+v", answer)
	}
	request(t, s.Client(), "POST", s.URL+"/v1/knowledge/revisions/"+id+"/publication",
		map[string]any{"decision": "WITHDRAW"}, http.StatusOK)
	if answer := ask(t, s); answer.Status != "UNRESOLVED" {
		t.Fatalf("stale index entry used after withdrawal: %+v", answer)
	}
}

func TestElasticsearchMultiVectorPublicFlow(t *testing.T) {
	endpoint := os.Getenv("HWOPS_TEST_ES_URL")
	if endpoint == "" {
		t.Skip("real Elasticsearch integration requires HWOPS_TEST_ES_URL and HWOPS_TEST_EMBED_URL")
	}
	index, err := elasticsearch.New(endpoint, os.Getenv("HWOPS_TEST_EMBED_URL"),
		fmt.Sprintf("hwops-multi-vector-test-%d", time.Now().UnixNano()), "dense")
	if err != nil {
		t.Fatal(err)
	}
	index.EnableMultiVector()
	if err := index.Ensure(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer index.Delete(context.Background())
	store, err := filestore.Open(filepath.Join(tempDir(t), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	app, err := application.New(store, &chatmodel.Replay{}, "REPLAY",
		application.Options{Retriever: index, ContextLimit: 5})
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close()
	server := httptest.NewServer(httpapi.New(app, token))
	defer server.Close()

	revision := request(t, server.Client(), "POST", server.URL+"/v1/knowledge/revisions", map[string]any{
		"title": "超时参数说明", "source": "fixture://multi-vector",
		"content":       "# 参数\n| 名称 | 说明 |\n| --- | --- |\n| timeout | 请求超时时间 |",
		"applicability": map[string]any{"scope": "GENERAL"},
		"fragments": []any{map[string]any{
			"section": "参数", "start_line": 2, "end_line": 4,
			"content": "| 名称 | 说明 |\n| --- | --- |\n| timeout | 请求超时时间 |",
			"representations": []any{
				map[string]any{"kind": "QUESTION", "text": "请求等待多久会终止？"},
				map[string]any{"kind": "QUESTION", "text": "timeout参数控制什么？"},
				map[string]any{"kind": "QUESTION", "text": "如何配置请求超时？"},
				map[string]any{"kind": "TABLE", "text": "参数表说明timeout控制请求超时时间。"},
			},
		}},
	}, http.StatusCreated)
	id := revision["id"].(string)
	request(t, server.Client(), "POST", server.URL+"/v1/knowledge/revisions/"+id+"/publication",
		map[string]any{"decision": "PUBLISH"}, http.StatusOK)
	found := request(t, server.Client(), "POST", server.URL+"/v1/knowledge/search",
		map[string]any{"query": "请求等待多久会终止？", "strategy": "dense", "top_k": 5}, http.StatusOK)
	documents := found["documents"].([]any)
	if len(documents) != 1 || documents[0].(map[string]any)["revision_id"] != id {
		t.Fatalf("generated question vector did not return its source fragment: %v", found)
	}
}
