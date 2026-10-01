package elasticsearch

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/cloudwego/eino/components/retriever"

	"hwops/internal/domain"
)

func TestMultiVectorIndexBindsRepresentationsToSourceFragment(t *testing.T) {
	var mu sync.Mutex
	var indexed map[string]any
	var searchedField string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		var body map[string]any
		if r.Body != nil {
			_ = json.NewDecoder(r.Body).Decode(&body)
		}
		switch {
		case r.URL.Path == "/v1/embeddings":
			inputs := body["input"].([]any)
			data := make([]map[string]any, len(inputs))
			for i := range inputs {
				vector := make([]float64, 512)
				vector[i%512] = 1
				data[i] = map[string]any{"index": i, "embedding": vector}
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"data": data})
		case r.Method == http.MethodPut && r.URL.Path == "/multi-vector/_doc/fragment-1":
			mu.Lock()
			indexed = body
			mu.Unlock()
			_, _ = w.Write([]byte(`{}`))
		case r.Method == http.MethodPost && r.URL.Path == "/multi-vector/_search":
			searchedField = body["knn"].(map[string]any)["field"].(string)
			mu.Lock()
			source := indexed
			mu.Unlock()
			_ = json.NewEncoder(w).Encode(map[string]any{"hits": map[string]any{"hits": []any{
				map[string]any{"_id": "fragment-1", "_score": 1.0, "_source": source},
			}}})
		default:
			_, _ = w.Write([]byte(`{}`))
		}
	}))
	defer server.Close()

	index, err := New(server.URL, server.URL, "multi-vector", "dense")
	if err != nil {
		t.Fatal(err)
	}
	index.EnableMultiVector()
	revision := domain.Revision{
		ID: "revision-1", Title: "参数文档", Source: "fixture://manual",
		Fragments: []domain.Fragment{{
			ID: "fragment-1", RevisionID: "revision-1", Section: "参数",
			Content: "| 参数 | 说明 |\n| timeout | 超时 |", ContentHash: "hash",
			Representations: []domain.RetrievalRepresentation{
				{Kind: "QUESTION", Text: "timeout是什么？"},
				{Kind: "QUESTION", Text: "如何设置超时？"},
				{Kind: "QUESTION", Text: "参数表有哪些内容？"},
				{Kind: "TABLE", Text: "参数表说明timeout是超时配置。"},
				{Kind: "PASSAGE", Text: "timeout参数控制请求超时时间。"},
			},
		}},
	}
	if err := index.IndexRevision(context.Background(), revision); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	count := int(indexed["representation_count"].(float64))
	nested := indexed["representations"].([]any)
	mu.Unlock()
	if count != 5 || len(nested) != 5 {
		t.Fatalf("wanted five bounded generated vectors, got count=%d nested=%d", count, len(nested))
	}
	if context, _ := indexed["retrieval_content"].(string); !strings.Contains(context, "[表格说明]") {
		t.Fatalf("indexed retrieval context omitted table text: %q", context)
	}
	for _, value := range nested {
		if value.(map[string]any)["kind"] == "CONTENT" {
			t.Fatal("long fragment with passage fallback also stored a truncated content vector")
		}
	}
	docs, err := index.Retrieve(context.Background(), "超时参数",
		retriever.WithTopK(5),
		retriever.WithDSLInfo(map[string]any{"revision_ids": []string{"revision-1"}}))
	if err != nil {
		t.Fatal(err)
	}
	if searchedField != "representations.embedding" || len(docs) != 1 || docs[0].ID != "fragment-1" {
		t.Fatalf("multi-vector hit was not mapped to source fragment: field=%q docs=%+v", searchedField, docs)
	}
}

func TestEmbeddingTextRemovesImageAndMarkdownNoise(t *testing.T) {
	input := "步骤\n![](https://example.test/screen.png \"点击放大\")\n" +
		"[参数说明](https://example.test/manual) | `timeout` | 30"
	cleaned := embeddingText(input)
	for _, noise := range []string{"https://", "![]", "|", "`"} {
		if strings.Contains(cleaned, noise) {
			t.Fatalf("embedding input retained %q noise: %q", noise, cleaned)
		}
	}
	for _, wanted := range []string{"图片", "参数说明", "timeout", "30"} {
		if !strings.Contains(cleaned, wanted) {
			t.Fatalf("embedding input lost %q: %q", wanted, cleaned)
		}
	}
}
