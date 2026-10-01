package acceptance_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"hwops/internal/adapters/chatmodel"
	"hwops/internal/adapters/elasticsearch"
	"hwops/internal/adapters/filestore"
	"hwops/internal/application"
	"hwops/internal/transport/httpapi"
)

// Reproduces a development failure: exact lexical evidence is absent from the
// dense list, while several generic passages occur in both lists. Only external
// Elasticsearch/embedding HTTP responses are simulated; the store and public
// knowledge publication/search flow remain real.
func TestHybridSearchCanRetainStrongSingleRouteEvidence(t *testing.T) {
	for _, constant := range []int{60, 1} {
		t.Run(fmt.Sprint(constant), func(t *testing.T) {
			var mu sync.Mutex
			indexed := map[string]map[string]any{}
			external := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				var body map[string]any
				_ = json.NewDecoder(r.Body).Decode(&body)
				if r.URL.Path == "/v1/embeddings" {
					texts := body["input"].([]any)
					data := make([]map[string]any, len(texts))
					for i := range texts {
						vector := make([]float64, 512)
						vector[0] = 1
						data[i] = map[string]any{"index": i, "embedding": vector}
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"data": data})
					return
				}
				mu.Lock()
				defer mu.Unlock()
				if r.Method == "PUT" && strings.HasPrefix(r.URL.Path, "/fusion-test/_doc/") {
					indexed[body["section"].(string)] = body
				}
				if strings.HasSuffix(r.URL.Path, "/_search") {
					order := []string{"精确依据", "背景1", "背景2", "背景3", "背景4", "背景5"}
					if _, dense := body["knn"]; dense {
						order = order[1:]
					}
					hits := []map[string]any{}
					for i, name := range order {
						doc := indexed[name]
						hits = append(hits, map[string]any{
							"_id": doc["fragment_id"], "_score": float64(100 - i), "_source": doc,
						})
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"hits": map[string]any{"hits": hits}})
					return
				}
				_, _ = w.Write([]byte(`{}`))
			}))
			defer external.Close()
			backend, err := elasticsearch.New(external.URL, external.URL, "fusion-test", "hybrid", constant)
			if err != nil {
				t.Fatal(err)
			}
			store, err := filestore.Open(filepath.Join(tempDir(t), "state.json"))
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			app, err := application.New(store, &chatmodel.Replay{}, "REPLAY",
				application.Options{Retriever: backend, ContextLimit: 5})
			if err != nil {
				t.Fatal(err)
			}
			defer app.Close()
			server := httptest.NewServer(httpapi.New(app, token))
			defer server.Close()
			content := "# 精确依据\n合成测试依据：CRD升级不更新，删除不卸载。\n"
			for i := 1; i <= 5; i++ {
				content += fmt.Sprintf("# 背景%d\n合成测试背景：模板可以安装和升级。\n", i)
			}
			revision := request(t, server.Client(), "POST", server.URL+"/v1/knowledge/revisions",
				map[string]any{"title": "合成融合排序资料", "source": "fixture://fusion",
					"content": content, "applicability": map[string]any{"scope": "GENERAL"}}, http.StatusCreated)
			request(t, server.Client(), "POST", server.URL+"/v1/knowledge/revisions/"+revision["id"].(string)+"/publication",
				map[string]any{"decision": "PUBLISH"}, http.StatusOK)
			result := request(t, server.Client(), "POST", server.URL+"/v1/knowledge/search",
				map[string]any{"query": "CRD升级和删除有什么限制？", "strategy": "hybrid", "top_k": 5}, http.StatusOK)
			found := false
			for _, doc := range result["documents"].([]any) {
				if doc.(map[string]any)["section"] == "精确依据" {
					found = true
				}
			}
			if found != (constant == 1) {
				t.Fatalf("RRF %d evidence retained=%v, wanted %v", constant, found, constant == 1)
			}
		})
	}
}
