// Package elasticsearch stores source revisions and vectorized fragments. The
// application supplies eligible revision IDs; the index never owns publication.
package elasticsearch

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/cloudwego/eino/components/retriever"
	"github.com/cloudwego/eino/schema"
	"go.opentelemetry.io/otel/attribute"

	"hwops/internal/domain"
	"hwops/internal/knowledge"
	"hwops/internal/observability"
)

type Index struct {
	endpoint, embedding, name, strategy string
	client                              *http.Client
	rrfConstant                         int
	multiVector                         bool
}

// New defaults to RRF 60; an explicit constant permits reproducible experiments.
func New(endpoint, embedding, name, strategy string, rrf ...int) (*Index, error) {
	constant := 60
	if len(rrf) > 1 {
		return nil, errors.New("at most one RRF constant is supported")
	}
	if len(rrf) == 1 {
		constant = rrf[0]
	}
	if constant < 1 || constant > 1000 {
		return nil, errors.New("RRF constant must be 1..1000")
	}
	for _, address := range []string{endpoint, embedding} {
		u, err := url.Parse(address)
		if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" ||
			(u.Scheme != "http" && u.Scheme != "https") {
			return nil, errors.New("invalid Elasticsearch/embedding endpoint")
		}
	}
	if !regexp.MustCompile(`^[a-z][a-z0-9-]{0,100}$`).MatchString(name) ||
		(strategy != "bm25" && strategy != "dense" && strategy != "hybrid") {
		return nil, errors.New("invalid index name or retrieval strategy")
	}
	return &Index{endpoint: strings.TrimRight(endpoint, "/"), embedding: strings.TrimRight(embedding, "/"),
		name: name, strategy: strategy, rrfConstant: constant, client: &http.Client{Timeout: 30 * time.Second,
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}

// EnableMultiVector stores content, generated questions, and asset
// descriptions as separate vectors under one authoritative fragment.
func (x *Index) EnableMultiVector() {
	x.multiVector = true
}

var (
	imageMarkdown = regexp.MustCompile(`!\[([^\]]*)\]\([^)\n]*\)`)
	linkMarkdown  = regexp.MustCompile(`\[([^\]]+)\]\([^)\n]*\)`)
	rawURL        = regexp.MustCompile(`https?://[^\s)>]+`)
	headingPrefix = regexp.MustCompile(`(?m)^\s*#{1,6}\s*`)
	listPrefix    = regexp.MustCompile(`(?m)^\s*(?:[-+]\s+|[0-9]+[.)]\s+)`)
	markupNoise   = strings.NewReplacer("|", " ", "`", " ", "*", " ", "_", " ", "~", " ", "<", " ", ">", " ")
)

func embeddingText(text string) string {
	text = imageMarkdown.ReplaceAllString(text, " 图片 $1 ")
	text = linkMarkdown.ReplaceAllString(text, " $1 ")
	text = rawURL.ReplaceAllString(text, " ")
	text = headingPrefix.ReplaceAllString(text, "")
	text = listPrefix.ReplaceAllString(text, "")
	text = markupNoise.Replace(text)
	return strings.Join(strings.Fields(text), " ")
}

func (x *Index) request(ctx context.Context, method, endpoint string, body any, out any) error {
	var raw []byte
	var err error
	if body != nil {
		raw, err = json.Marshal(body)
		if err != nil {
			return err
		}
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	response, err := x.client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return errors.New("search dependency unavailable")
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("search dependency HTTP %d", response.StatusCode)
	}
	if out == nil {
		return nil
	}
	raw, err = io.ReadAll(io.LimitReader(response.Body, 8*1024*1024+1))
	if err != nil {
		return err
	}
	if len(raw) > 8*1024*1024 {
		return errors.New("search response too large")
	}
	return json.Unmarshal(raw, out)
}

func (x *Index) Ensure(ctx context.Context) error {
	properties := map[string]any{
		"content":      map[string]any{"type": "text", "analyzer": "cjk"},
		"retrieval_content": map[string]any{"type": "text", "analyzer": "cjk"},
		"section":      map[string]any{"type": "text", "analyzer": "cjk"},
		"title":        map[string]any{"type": "text", "analyzer": "cjk"},
		"revision_id":  map[string]any{"type": "keyword"},
		"fragment_id":  map[string]any{"type": "keyword"},
		"content_hash": map[string]any{"type": "keyword"},
		"embedding": map[string]any{"type": "dense_vector", "dims": 512, "index": true,
			"similarity": "cosine", "index_options": map[string]any{"type": "int8_hnsw"}},
	}
	if x.multiVector {
		properties["representation_count"] = map[string]any{"type": "integer"}
		properties["representations"] = map[string]any{
			"type": "nested",
			"properties": map[string]any{
				"kind": map[string]any{"type": "keyword"},
				"text": map[string]any{"type": "text", "index": false},
				"embedding": map[string]any{"type": "dense_vector", "dims": 512, "index": true,
					"similarity": "cosine", "index_options": map[string]any{"type": "int8_hnsw"}},
			},
		}
	}
	for _, spec := range []struct {
		name     string
		mappings map[string]any
	}{
		{x.name, map[string]any{"properties": properties}},
		{x.name + "-revisions", map[string]any{"enabled": false}},
	} {
		req, err := http.NewRequestWithContext(ctx, "HEAD", x.endpoint+"/"+spec.name, nil)
		if err != nil {
			return err
		}
		res, err := x.client.Do(req)
		if err != nil {
			return errors.New("Elasticsearch unavailable")
		}
		res.Body.Close()
		if res.StatusCode == 200 {
			continue
		}
		if res.StatusCode != 404 {
			return fmt.Errorf("index check HTTP %d", res.StatusCode)
		}
		if err := x.request(ctx, "PUT", x.endpoint+"/"+spec.name, map[string]any{
			"settings": map[string]any{"number_of_shards": 1, "number_of_replicas": 0},
			"mappings": spec.mappings,
		}, nil); err != nil {
			return err
		}
	}
	return nil
}

// Delete is for isolated integration-test indexes, never called during shutdown.
func (x *Index) Delete(ctx context.Context) error {
	return x.request(ctx, "DELETE", x.endpoint+"/"+x.name+","+x.name+"-revisions", nil, nil)
}

func (x *Index) embed(ctx context.Context, texts []string, inputType string) (vectors [][]float64, err error) {
	ctx, span := observability.Start(ctx, "local.embedding", "EMBEDDING", map[string]any{"texts": texts, "input_type": inputType})
	span.SetAttributes(attribute.String("embedding.model_name", "BAAI/bge-small-zh-v1.5"))
	defer func() { observability.End(span, map[string]any{"count": len(vectors), "dimensions": 512}, err) }()
	var out struct {
		Data []struct {
			Index     int       `json:"index"`
			Embedding []float64 `json:"embedding"`
		} `json:"data"`
	}
	if err := x.request(ctx, "POST", x.embedding+"/v1/embeddings",
		map[string]any{"input": texts, "input_type": inputType}, &out); err != nil {
		return nil, err
	}
	if len(out.Data) != len(texts) {
		return nil, errors.New("embedding count mismatch")
	}
	vectors = make([][]float64, len(texts))
	for _, item := range out.Data {
		if item.Index < 0 || item.Index >= len(texts) || vectors[item.Index] != nil || len(item.Embedding) != 512 {
			return nil, errors.New("invalid embedding dimensions or index")
		}
		vectors[item.Index] = item.Embedding
	}
	return vectors, nil
}

func (x *Index) IndexRevision(ctx context.Context, revision domain.Revision) error {
	if err := x.request(ctx, "PUT", x.endpoint+"/"+x.name+"-revisions/_doc/"+revision.ID,
		revision, nil); err != nil {
		return err
	}
	if x.multiVector {
		return x.indexMultiVectorRevision(ctx, revision)
	}
	// Bounded batches keep the local model's memory independent of corpus size.
	for start := 0; start < len(revision.Fragments); start += 16 {
		end := min(start+16, len(revision.Fragments))
		batch := revision.Fragments[start:end]
		texts := make([]string, len(batch))
		for i, f := range batch {
			texts[i] = embeddingText(revision.Title + "\n" + f.Section + "\n" + knowledge.RetrievalContext(f))
		}
		vectors, err := x.embed(ctx, texts, "document")
		if err != nil {
			return err
		}
		for i, f := range batch {
			if err := x.request(ctx, "PUT", x.endpoint+"/"+x.name+"/_doc/"+f.ID,
				map[string]any{"fragment_id": f.ID, "revision_id": revision.ID,
					"title": revision.Title, "section": f.Section, "content": f.Content,
					"retrieval_content": knowledge.RetrievalContext(f),
					"content_hash": f.ContentHash, "source": revision.Source, "embedding": vectors[i]}, nil); err != nil {
				return err
			}
		}
	}
	// Publication cannot succeed until both stores are searchable.
	return x.request(ctx, "POST", x.endpoint+"/"+x.name+","+x.name+"-revisions/_refresh", nil, nil)
}

func (x *Index) indexMultiVectorRevision(ctx context.Context, revision domain.Revision) error {
	type target struct {
		fragment   int
		kind, text string
	}
	texts := []string{}
	targets := []target{}
	for fragmentIndex, fragment := range revision.Fragments {
		context := knowledge.RetrievalContext(fragment)
		hasPassage := false
		for _, representation := range fragment.Representations {
			hasPassage = hasPassage || representation.Kind == "PASSAGE"
		}
		if !hasPassage {
			text := embeddingText(context)
			texts = append(texts, embeddingText(revision.Title+"\n"+fragment.Section+"\n"+text))
			targets = append(targets, target{fragment: fragmentIndex, kind: "CONTENT", text: text})
		}
		for _, representation := range fragment.Representations {
			text := embeddingText(representation.Text)
			texts = append(texts, embeddingText(revision.Title+"\n"+fragment.Section+"\n"+text))
			targets = append(targets, target{fragment: fragmentIndex, kind: representation.Kind, text: text})
		}
	}
	vectors := make([][]float64, len(texts))
	for start := 0; start < len(texts); start += 64 {
		end := min(start+64, len(texts))
		batch, err := x.embed(ctx, texts[start:end], "document")
		if err != nil {
			return err
		}
		copy(vectors[start:end], batch)
	}
	representations := make([][]map[string]any, len(revision.Fragments))
	for i, target := range targets {
		representations[target.fragment] = append(representations[target.fragment], map[string]any{
			"kind": target.kind, "text": target.text, "embedding": vectors[i],
		})
	}
	for i, fragment := range revision.Fragments {
		if err := x.request(ctx, "PUT", x.endpoint+"/"+x.name+"/_doc/"+fragment.ID,
			map[string]any{
				"fragment_id": fragment.ID, "revision_id": revision.ID,
				"title": revision.Title, "section": fragment.Section, "content": fragment.Content,
					"retrieval_content": knowledge.RetrievalContext(fragment),
				"content_hash": fragment.ContentHash, "source": revision.Source,
				"representations":      representations[i],
				"representation_count": len(representations[i]),
			}, nil); err != nil {
			return err
		}
	}
	return x.request(ctx, "POST", x.endpoint+"/"+x.name+","+x.name+"-revisions/_refresh", nil, nil)
}

func (x *Index) search(ctx context.Context, body any) ([]*schema.Document, error) {
	var result struct {
		Hits struct {
			Hits []struct {
				ID     string  `json:"_id"`
				Score  float64 `json:"_score"`
				Source struct {
					Content    string `json:"content"`
					RevisionID string `json:"revision_id"`
					FragmentID string `json:"fragment_id"`
				} `json:"_source"`
			} `json:"hits"`
		} `json:"hits"`
	}
	if err := x.request(ctx, "POST", x.endpoint+"/"+x.name+"/_search", body, &result); err != nil {
		return nil, err
	}
	docs := make([]*schema.Document, 0, len(result.Hits.Hits))
	for _, hit := range result.Hits.Hits {
		id := hit.Source.FragmentID
		if id == "" {
			id = hit.ID
		}
		docs = append(docs, &schema.Document{ID: id, Content: hit.Source.Content,
			MetaData: map[string]any{"revision_id": hit.Source.RevisionID, "score": hit.Score}})
	}
	return docs, nil
}

func (x *Index) Retrieve(ctx context.Context, query string, opts ...retriever.Option) ([]*schema.Document, error) {
	return x.retrieve(ctx, query, false, opts...)
}

// RetrieveCandidates exposes the same two bounded routes before final top-K
// truncation. It does not widen either route's 20-hit retrieval budget.
func (x *Index) RetrieveCandidates(ctx context.Context, query string, opts ...retriever.Option) ([]*schema.Document, error) {
	return x.retrieve(ctx, query, true, opts...)
}

func (x *Index) retrieve(ctx context.Context, query string, candidates bool, opts ...retriever.Option) ([]*schema.Document, error) {
	options := retriever.GetCommonOptions(nil, opts...)
	eligible, _ := options.DSLInfo["revision_ids"].([]string)
	if len(eligible) == 0 {
		return []*schema.Document{}, nil
	}
	strategy, _ := options.DSLInfo["strategy"].(string)
	if strategy == "" {
		strategy = x.strategy
	}
	if strategy != "bm25" && strategy != "dense" && strategy != "hybrid" {
		return nil, fmt.Errorf("%w: unsupported retrieval strategy", domain.ErrInvalid)
	}
	topK := 5
	if options.TopK != nil {
		topK = *options.TopK
	}
	if topK < 1 || topK > 8 {
		return nil, domain.ErrInvalid
	}
	if candidates {
		topK = 40
	}
	filter := []any{map[string]any{"terms": map[string]any{"revision_id": eligible}}}
	var lists [][]*schema.Document
	if strategy != "dense" {
		docs, err := x.search(ctx, map[string]any{"size": 20, "_source": []string{"content", "revision_id", "fragment_id"},
			"sort": []any{map[string]any{"_score": "desc"}, map[string]any{"fragment_id": "asc"}},
				"query": map[string]any{"bool": map[string]any{"filter": filter,
					"must": []any{map[string]any{"multi_match": map[string]any{"query": query,
						"fields": []string{"retrieval_content^2", "content", "section", "title"}}}}}}})
		if err != nil {
			return nil, err
		}
		lists = append(lists, docs)
	}
	if strategy != "bm25" {
		vectors, err := x.embed(ctx, []string{query}, "query")
		if err != nil {
			return nil, err
		}
		field := "embedding"
		if x.multiVector {
			field = "representations.embedding"
		}
		docs, err := x.search(ctx, map[string]any{"size": 20, "_source": []string{"content", "revision_id", "fragment_id"},
			"knn": map[string]any{"field": field, "query_vector": vectors[0],
				"k": 20, "num_candidates": 100, "filter": filter}})
		if err != nil {
			return nil, err
		}
		lists = append(lists, docs)
	}
	if len(lists) == 1 {
		return lists[0][:min(topK, len(lists[0]))], nil
	}
	// Client RRF works on the ES Basic license; no licensed native ranker.
	fused := map[string]*schema.Document{}
	scores := map[string]float64{}
	for _, docs := range lists {
		for rank, doc := range docs {
			fused[doc.ID] = doc
			scores[doc.ID] += 1.0 / float64(x.rrfConstant+rank+1)
		}
	}
	docs := make([]*schema.Document, 0, len(fused))
	for id, doc := range fused {
		doc.MetaData["score"] = scores[id]
		docs = append(docs, doc)
	}
	sort.Slice(docs, func(i, j int) bool {
		if scores[docs[i].ID] == scores[docs[j].ID] {
			return docs[i].ID < docs[j].ID
		}
		return scores[docs[i].ID] > scores[docs[j].ID]
	})
	return docs[:min(topK, len(docs))], nil
}
