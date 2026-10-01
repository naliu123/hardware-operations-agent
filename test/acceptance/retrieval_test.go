package acceptance_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cloudwego/eino/components/retriever"
	"github.com/cloudwego/eino/schema"

	"hwops/internal/adapters/chatmodel"
	"hwops/internal/adapters/filestore"
	"hwops/internal/application"
	"hwops/internal/domain"
	"hwops/internal/transport/httpapi"
)

type publicSearchRetriever struct {
	revision domain.Revision
}

func (r *publicSearchRetriever) IndexRevision(_ context.Context, revision domain.Revision) error {
	r.revision = revision
	return nil
}

func (r *publicSearchRetriever) Retrieve(
	_ context.Context,
	_ string,
	_ ...retriever.Option,
) ([]*schema.Document, error) {
	fragment := r.revision.Fragments[0]
	return []*schema.Document{{
		ID:      fragment.ID,
		Content: fragment.Content,
		MetaData: map[string]any{
			"revision_id": r.revision.ID,
		},
	}}, nil
}

func (*publicSearchRetriever) GetType() string {
	return "public-search-session-test"
}

func TestPublicRetrievalRespectsPublicationAndReturnsSource(t *testing.T) {
	s, closeServer := startServer(t, filepath.Join(tempDir(t), "state.json"), &chatmodel.Replay{}, "REPLAY")
	defer closeServer()
	id := addRevision(t, s, "GENERAL", "PUBLISH")
	input := map[string]any{"query": "蓝灯表示什么？", "top_k": 5}
	result := request(t, s.Client(), "POST", s.URL+"/v1/knowledge/search", input, http.StatusOK)
	docs, ok := result["documents"].([]any)
	if !ok || len(docs) != 1 {
		t.Fatalf("expected one published fragment: %v", result)
	}
	doc := docs[0].(map[string]any)
	if doc["revision_id"] != id || doc["source"] != "fixture://manual" || doc["content_hash"] == "" {
		t.Fatalf("source lost: %v", doc)
	}
	request(t, s.Client(), "POST", s.URL+"/v1/knowledge/revisions/"+id+"/publication",
		map[string]any{"decision": "WITHDRAW"}, http.StatusOK)
	result = request(t, s.Client(), "POST", s.URL+"/v1/knowledge/search", input, http.StatusOK)
	if len(result["documents"].([]any)) != 0 {
		t.Fatalf("withdrawn knowledge retrieved: %v", result)
	}
}

func TestPublicRetrievalProvidesStableSessionForExternalQueryRewrite(t *testing.T) {
	var sessions []string
	modelServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sessions = append(sessions, r.Header.Get("x-opencode-session"))
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []any{map[string]any{"message": map[string]any{
				"content": "QUERY: 指示灯为蓝色表示什么？\nQUERY: 蓝色指示灯对应什么状态？",
			}}},
		})
	}))
	defer modelServer.Close()
	cm, err := chatmodel.NewOpenAI(modelServer.URL, "fixture-model", "")
	if err != nil {
		t.Fatal(err)
	}
	store, err := filestore.Open(filepath.Join(tempDir(t), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	backend := &publicSearchRetriever{}
	app, err := application.New(store, cm, "LIVE", application.Options{
		Retriever: backend, QueryRewrite: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close()
	server := httptest.NewServer(httpapi.New(app, token))
	defer server.Close()
	addRevision(t, server, "GENERAL", "PUBLISH")

	input := map[string]any{"query": "蓝灯表示什么？", "top_k": 5}
	request(t, server.Client(), "POST", server.URL+"/v1/knowledge/search", input, http.StatusOK)
	request(t, server.Client(), "POST", server.URL+"/v1/knowledge/search", input, http.StatusOK)
	if len(sessions) != 2 || sessions[0] == "" || sessions[0] != sessions[1] ||
		!strings.HasPrefix(sessions[0], "knowledge-search-") {
		t.Fatalf("public search did not provide a stable model session: %v", sessions)
	}
}
