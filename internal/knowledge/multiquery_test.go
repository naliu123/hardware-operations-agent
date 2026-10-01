package knowledge

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/retriever"
	"github.com/cloudwego/eino/schema"
)

type rewriteModel struct {
	content string
}

func (m *rewriteModel) Generate(context.Context, []*schema.Message, ...model.Option) (*schema.Message, error) {
	return schema.AssistantMessage(m.content, nil), nil
}

func (m *rewriteModel) Stream(ctx context.Context, messages []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	out, err := m.Generate(ctx, messages, opts...)
	if err != nil {
		return nil, err
	}
	return schema.StreamReaderFromArray([]*schema.Message{out}), nil
}

type sequenceRewriteModel struct {
	mu        sync.Mutex
	responses []string
}

func (m *sequenceRewriteModel) Generate(context.Context, []*schema.Message, ...model.Option) (*schema.Message, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	content := m.responses[0]
	m.responses = m.responses[1:]
	return schema.AssistantMessage(content, nil), nil
}

func (m *sequenceRewriteModel) Stream(ctx context.Context, messages []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	out, err := m.Generate(ctx, messages, opts...)
	if err != nil {
		return nil, err
	}
	return schema.StreamReaderFromArray([]*schema.Message{out}), nil
}

type concurrentRetriever struct {
	mu      sync.Mutex
	queries []string
	started int
	gate    chan struct{}
}

func (r *concurrentRetriever) Retrieve(ctx context.Context, query string, opts ...retriever.Option) ([]*schema.Document, error) {
	options := retriever.GetCommonOptions(nil, opts...)
	eligible, _ := options.DSLInfo["revision_ids"].([]string)
	if len(eligible) != 1 || eligible[0] != "revision-1" {
		return nil, context.Canceled
	}
	r.mu.Lock()
	r.queries = append(r.queries, query)
	r.started++
	if r.started == 4 {
		close(r.gate)
	}
	r.mu.Unlock()
	select {
	case <-r.gate:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	return []*schema.Document{
		{ID: "shared", Content: "shared", MetaData: map[string]any{"revision_id": "revision-1"}},
		{ID: query, Content: query, MetaData: map[string]any{"revision_id": "revision-1"}},
	}, nil
}

func TestMultiQueryRetrieverRewritesInParallelAndPreservesOptions(t *testing.T) {
	model := &rewriteModel{content: `{"queries":["问题改写一","问题改写二","问题改写三"]}`}
	base := &concurrentRetriever{gate: make(chan struct{})}
	multi, err := NewMultiQueryRetriever(base, model, 3)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	docs, err := multi.Retrieve(ctx, "原问题",
		retriever.WithTopK(3),
		retriever.WithDSLInfo(map[string]any{"revision_ids": []string{"revision-1"}}))
	if err != nil {
		t.Fatal(err)
	}
	if len(base.queries) != 4 {
		t.Fatalf("wanted original plus three rewrites, got %v", base.queries)
	}
	if len(docs) != 3 || docs[0].ID != "shared" {
		t.Fatalf("query fusion did not prioritize shared evidence: %+v", docs)
	}
	queries, ok := docs[0].MetaData["retrieval_queries"].([]string)
	if !ok || len(queries) != 4 || queries[0] != "原问题" {
		t.Fatalf("retrieval queries not exposed: %#v", docs[0].MetaData)
	}
}

func TestDecodeRewritesRejectsDuplicatesAndUnknownFields(t *testing.T) {
	if _, ok := decodeRewrites(`{"queries":["相同","相同"]}`, "原问题", 3); ok {
		t.Fatal("duplicate rewrites accepted")
	}
	raw, _ := json.Marshal(map[string]any{
		"queries": []string{"问题一", "问题二"}, "explanation": "not allowed",
	})
	if _, ok := decodeRewrites(string(raw), "原问题", 3); ok {
		t.Fatal("unknown rewrite fields accepted")
	}
	if got := parseRewrites("QUERY: 第一种问法\nQUERY: 第二种问法"); len(got) != 2 {
		t.Fatalf("tagged rewrites not parsed: %v", got)
	}
	if got := parseRewrites("QUERY: 问题1\nQUERY: query 2"); len(got) != 0 {
		t.Fatalf("placeholder rewrites accepted: %v", got)
	}
}

func TestMultiQueryRetrieverAccumulatesSingleRewriteResponses(t *testing.T) {
	cm := &sequenceRewriteModel{responses: []string{
		"QUERY: 第一种具体问法",
		"QUERY: 第二种具体问法",
	}}
	multi, err := NewMultiQueryRetriever(&concurrentRetriever{gate: make(chan struct{})}, cm, 3)
	if err != nil {
		t.Fatal(err)
	}
	queries, err := multi.rewrite(context.Background(), "原问题")
	if err != nil {
		t.Fatal(err)
	}
	if len(queries) != 3 || queries[1] != "第一种具体问法" || queries[2] != "第二种具体问法" {
		t.Fatalf("single rewrite responses were not accumulated: %v", queries)
	}
}
