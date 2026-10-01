package knowledge

import (
	"context"
	"fmt"
	"strings"

	"github.com/cloudwego/eino/components/retriever"
	"github.com/cloudwego/eino/schema"
	"go.opentelemetry.io/otel/attribute"

	"hwops/internal/domain"
	"hwops/internal/observability"
)

type SearchInput struct {
	Query    string `json:"query"`
	DeviceID string `json:"device_id,omitempty"`
	Strategy string `json:"strategy,omitempty"`
	TopK     int    `json:"top_k,omitempty"`
}

type SearchDocument struct {
	domain.Fragment
	Source          string          `json:"source"`
	Title           string          `json:"title"`
	DocumentContext string          `json:"document_context,omitempty"`
	Score           float64         `json:"score"`
	Citation        domain.Citation `json:"citation"`
}

type SearchResult struct {
	Documents           []SearchDocument        `json:"documents"`
	Checks              []domain.KnowledgeCheck `json:"applicability_checks"`
	TraceID             string                  `json:"trace_id,omitempty"`
	ExpandedFragmentIDs []string                `json:"expanded_fragment_ids,omitempty"`
	RetrievalQueries    []string                `json:"retrieval_queries,omitempty"`
}

func (r SearchResult) EinoDocuments() []*schema.Document {
	docs := make([]*schema.Document, 0, len(r.Documents))
	for _, d := range r.Documents {
		docs = append(docs, &schema.Document{ID: d.ID, Content: RetrievalContext(d.Fragment), MetaData: map[string]any{
			"revision_id": d.RevisionID, "section": d.Section, "source": d.Source, "score": d.Score,
			"title": d.Title, "document_context": d.DocumentContext,
			"source_content": d.Content, "content_hash": d.ContentHash,
		}})
	}
	return docs
}

// Search applies eligibility before backend candidate limits, then verifies each
// hit against the authoritative immutable fragment and current publication.
func Search(ctx context.Context, store domain.Repository, backend retriever.Retriever, in SearchInput, device *domain.DeviceContext) (result SearchResult, err error) {
	return search(ctx, store, backend, in, device, false)
}

// Candidates is used only by the explicitly enabled QA evidence selector.
// At most 40 route hits and 8 adjacent immutable fragments are exposed.
func Candidates(ctx context.Context, store domain.Repository, backend retriever.Retriever, in SearchInput, device *domain.DeviceContext) (SearchResult, error) {
	return search(ctx, store, backend, in, device, true)
}

func search(ctx context.Context, store domain.Repository, backend retriever.Retriever, in SearchInput, device *domain.DeviceContext, candidates bool) (result SearchResult, err error) {
	ctx, span := observability.Start(ctx, "knowledge.search", "RETRIEVER", in)
	defer func() {
		for i, doc := range result.Documents {
			prefix := fmt.Sprintf("retrieval.documents.%d.document.", i)
			span.SetAttributes(attribute.String(prefix+"id", doc.ID),
				attribute.String(prefix+"content", doc.Content),
				attribute.Float64(prefix+"score", doc.Score),
				attribute.String(prefix+"metadata", observability.JSON(map[string]any{
					"revision_id": doc.RevisionID, "section": doc.Section, "source": doc.Source,
				})))
		}
		observability.End(span, result, err)
	}()
	result = SearchResult{Documents: []SearchDocument{}, Checks: []domain.KnowledgeCheck{}, TraceID: observability.TraceID(ctx)}
	if strings.TrimSpace(in.Query) == "" || len(in.Query) > 16000 || in.TopK < 0 || in.TopK > 8 {
		return result, fmt.Errorf("%w: query required (16 KiB max), top_k must be 1..8", domain.ErrInvalid)
	}
	if in.Strategy != "" && in.Strategy != "bm25" && in.Strategy != "dense" && in.Strategy != "hybrid" {
		return result, fmt.Errorf("%w: unsupported retrieval strategy", domain.ErrInvalid)
	}
	if in.TopK == 0 {
		in.TopK = 5
	}
	revisions, err := store.ListPublished(ctx)
	if err != nil {
		return result, err
	}
	eligible := []string{}
	published := make(map[string]domain.Revision, len(revisions))
	for _, rev := range revisions {
		published[rev.ID] = rev
		check, err := Assess(ctx, store, rev.Applicability, device)
		if err != nil {
			return result, err
		}
		result.Checks = append(result.Checks, domain.KnowledgeCheck{RevisionID: rev.ID, Assessment: check})
		if check.Status == "MATCH" {
			eligible = append(eligible, rev.ID)
		}
	}
	if len(eligible) == 0 {
		return result, nil
	}
	retrieve := backend.Retrieve
	if candidates {
		if candidateBackend, ok := backend.(interface {
			RetrieveCandidates(context.Context, string, ...retriever.Option) ([]*schema.Document, error)
		}); ok {
			retrieve = candidateBackend.RetrieveCandidates
		}
	}
	docs, err := retrieve(ctx, in.Query, retriever.WithTopK(in.TopK),
		retriever.WithDSLInfo(map[string]any{"revision_ids": eligible, "strategy": in.Strategy}))
	if err != nil {
		return result, err
	}
	total := 0
	limit := in.TopK
	if candidates {
		limit = 40
	}
	seen := map[string]bool{}
	for _, doc := range docs {
		if doc == nil {
			return result, fmt.Errorf("nil retrieval hit")
		}
		id, ok := doc.MetaData["revision_id"].(string)
		if !ok {
			return result, fmt.Errorf("retrieval hit lacks revision identity")
		}
		if len(result.RetrievalQueries) == 0 {
			if queries, ok := doc.MetaData["retrieval_queries"].([]string); ok {
				result.RetrievalQueries = append([]string(nil), queries...)
			}
		}
		_, ok = published[id]
		if !ok {
			continue
		}
		rev, err := store.GetRevision(ctx, id)
		if err != nil {
			return result, err
		}
		check, err := Assess(ctx, store, rev.Applicability, device)
		if err != nil {
			return result, err
		}
		if rev.Status != "PUBLISHED" || check.Status != "MATCH" {
			continue
		}
		for _, fragment := range rev.Fragments {
			if fragment.ID != doc.ID {
				continue
			}
			if fragment.Content != doc.Content {
				return result, fmt.Errorf("retrieval hit differs from source fragment")
			}
			contextBytes := len(RetrievalContext(fragment))
			if !seen[fragment.ID] && len(result.Documents) < limit &&
				(candidates || total+contextBytes+len(DocumentContext(rev)) <= 48*1024) {
				score, _ := doc.MetaData["score"].(float64)
				result.Documents = append(result.Documents, SearchDocument{Fragment: fragment, Source: rev.Source, Title: rev.Title,
					DocumentContext: DocumentContext(rev), Score: score,
					Citation: BuildCitation(rev, fragment, check, device)})
				total += contextBytes + len(DocumentContext(rev))
				seen[fragment.ID] = true
			}
		}
	}
	if candidates {
		// The first eight ranked hits can contribute immediate siblings. This
		// recovers a preceding overview without silently fetching whole manuals.
		roots := append([]SearchDocument(nil), result.Documents[:min(8, len(result.Documents))]...)
		for _, root := range roots {
			rev, err := store.GetRevision(ctx, root.RevisionID)
			if err != nil {
				return result, err
			}
			check, err := Assess(ctx, store, rev.Applicability, device)
			if err != nil {
				return result, err
			}
			if rev.Status != "PUBLISHED" || check.Status != "MATCH" {
				continue
			}
			for i, fragment := range rev.Fragments {
				if fragment.ID != root.ID {
					continue
				}
				for _, j := range []int{i - 1, i + 1} {
					if j < 0 || j >= len(rev.Fragments) || len(result.ExpandedFragmentIDs) >= 8 {
						continue
					}
					adjacent := rev.Fragments[j]
					if seen[adjacent.ID] {
						continue
					}
					seen[adjacent.ID] = true
					result.Documents = append(result.Documents, SearchDocument{Fragment: adjacent, Source: rev.Source,
						Title: rev.Title, DocumentContext: DocumentContext(rev),
						Citation: BuildCitation(rev, adjacent, check, device)})
					result.ExpandedFragmentIDs = append(result.ExpandedFragmentIDs, adjacent.ID)
				}
			}
		}
	}
	return result, nil
}

func BuildCitation(revision domain.Revision, fragment domain.Fragment, assessment domain.Assessment, device *domain.DeviceContext) domain.Citation {
	snapshotID := ""
	if device != nil && revision.Applicability.Scope == "DEVICE" {
		snapshotID = device.SnapshotID
	}
	return domain.Citation{
		DocumentID:       revision.DocumentID,
		RevisionID:       revision.ID,
		FragmentID:       fragment.ID,
		Title:            revision.Title,
		Source:           revision.Source,
		Section:          fragment.Section,
		DocumentContext:  DocumentContext(revision),
		ContentHash:      fragment.ContentHash,
		URL:              fmt.Sprintf("/v1/knowledge/revisions/%s/fragments/%s", revision.ID, fragment.ID),
		DeviceSnapshotID: snapshotID,
		Applicability:    assessment,
	}
}

// DocumentContext retains provenance embedded in the immutable source header.
// Only bounded header fields are added; the cited fragment and its hash stay
// unchanged. Metadata in body examples must not become document-level scope.
func DocumentContext(revision domain.Revision) string {
	var fields []string
	size := 0
	for _, line := range strings.Split(revision.Content, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "##") {
			break
		}
		if strings.HasPrefix(line, "目录：") || strings.HasPrefix(line, "更新时间：") {
			if size+len(line)+1 > 4096 {
				break
			}
			fields = append(fields, line)
			size += len(line) + 1
		} else if line != "" && !strings.HasPrefix(line, "# ") && !strings.HasPrefix(line, "来源：") {
			break
		}
	}
	return strings.Join(fields, "\n")
}
