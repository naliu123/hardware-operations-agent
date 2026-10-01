package knowledgeagent_test

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"

	"github.com/cloudwego/eino/components/retriever"
	"github.com/cloudwego/eino/schema"

	"hwops/internal/adapters/filestore"
	knowledgeagent "hwops/internal/agents/knowledge"
	"hwops/internal/domain"
	"hwops/internal/knowledge"
)

func TestAgentToolReturnsPublishedEvidenceWithCitation(t *testing.T) {
	ctx := context.Background()
	store := openStore(t)
	revision := publish(t, store, domain.Applicability{Scope: "GENERAL"})

	agent, err := knowledgeagent.New(knowledgeagent.Config{
		Store: store, Retriever: &knowledge.LocalRetriever{Store: store},
		DataMode: "REPLAY", ContextLimit: 5,
	})
	if err != nil {
		t.Fatal(err)
	}
	knowledgeTool, err := agent.Tool(ctx)
	if err != nil {
		t.Fatal(err)
	}
	info, err := knowledgeTool.Info(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if info.Name != knowledgeagent.ToolName {
		t.Fatalf("unexpected tool name: %q", info.Name)
	}
	inputSchema, err := info.ParamsOneOf.ToJSONSchema()
	if err != nil {
		t.Fatal(err)
	}
	rawSchema, err := json.Marshal(inputSchema)
	if err != nil {
		t.Fatal(err)
	}
	var schema map[string]any
	if err := json.Unmarshal(rawSchema, &schema); err != nil {
		t.Fatal(err)
	}
	properties, _ := schema["properties"].(map[string]any)
	if len(properties) != 1 || properties["request"] == nil {
		t.Fatalf("main Agent should only provide a request: %s", rawSchema)
	}

	result, err := knowledgeagent.Invoke(ctx, knowledgeTool, "蓝灯表示什么？", nil, "REPLAY")
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != knowledgeagent.StatusFound || result.DataMode != "REPLAY" ||
		len(result.Evidence) != 1 || len(result.Gaps) != 0 {
		t.Fatalf("unexpected REPLAY retrieval result: %+v", result)
	}
	evidence := result.Evidence[0]
	if evidence.Content != "蓝灯表示维护模式。" || evidence.Citation.RevisionID != revision.ID ||
		evidence.Citation.FragmentID != evidence.ID || evidence.Citation.ContentHash != evidence.ContentHash ||
		evidence.Citation.Applicability.Status != "MATCH" {
		t.Fatalf("evidence is not independently traceable: %+v", evidence)
	}
}

func TestAgentToolReportsMissingDeviceContext(t *testing.T) {
	ctx := context.Background()
	store := openStore(t)
	publish(t, store, domain.Applicability{
		Scope: "DEVICE", Model: "fixture/Atlas", Firmware: "R2",
	})
	agent, err := knowledgeagent.New(knowledgeagent.Config{
		Store: store, Retriever: &knowledge.LocalRetriever{Store: store},
		DataMode: "REPLAY", ContextLimit: 5,
	})
	if err != nil {
		t.Fatal(err)
	}
	knowledgeTool, err := agent.Tool(ctx)
	if err != nil {
		t.Fatal(err)
	}

	ctx = knowledgeagent.WithDeviceContext(ctx, &domain.DeviceContext{
		SchemaVersion: 1, DeviceID: "stale", SnapshotID: "stale-snapshot",
		DeviceInput: domain.DeviceInput{DataMode: "LIVE"},
	})
	result, err := knowledgeagent.Invoke(ctx, knowledgeTool, "蓝灯表示什么？", nil, "REPLAY")
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != knowledgeagent.StatusNeedsContext || len(result.Evidence) != 0 ||
		len(result.Gaps) < 2 || result.ApplicabilityChecks[0].Status != "UNKNOWN" {
		t.Fatalf("missing device context was not explicit: %+v", result)
	}
}

func TestAgentToolRejectsUntrustedDeviceMode(t *testing.T) {
	ctx := context.Background()
	store := openStore(t)
	agent, err := knowledgeagent.New(knowledgeagent.Config{
		Store: store, Retriever: &knowledge.LocalRetriever{Store: store},
		DataMode: "REPLAY", ContextLimit: 5,
	})
	if err != nil {
		t.Fatal(err)
	}
	knowledgeTool, err := agent.Tool(ctx)
	if err != nil {
		t.Fatal(err)
	}
	device := &domain.DeviceContext{
		SchemaVersion: 1, DeviceID: "device-1", SnapshotID: "snapshot-1",
		DeviceInput: domain.DeviceInput{DataMode: "LIVE"},
	}

	result, err := knowledgeagent.Invoke(ctx, knowledgeTool, "蓝灯表示什么？", device, "REPLAY")
	if !errors.Is(err, domain.ErrInvalid) || result.Status != knowledgeagent.StatusFailed ||
		result.Error == nil || result.Error.Code != "INVALID_INPUT" {
		t.Fatalf("mode mismatch did not fail closed: result=%+v err=%v", result, err)
	}
}

func TestAgentToolRechecksPublicationAfterRetrieval(t *testing.T) {
	ctx := context.Background()
	store := openStore(t)
	revision := publish(t, store, domain.Applicability{Scope: "GENERAL"})
	agent, err := knowledgeagent.New(knowledgeagent.Config{
		Store: store,
		Retriever: &withdrawingRetriever{
			store: store, revision: revision,
		},
		DataMode: "REPLAY", ContextLimit: 5,
	})
	if err != nil {
		t.Fatal(err)
	}
	knowledgeTool, err := agent.Tool(ctx)
	if err != nil {
		t.Fatal(err)
	}

	result, err := knowledgeagent.Invoke(ctx, knowledgeTool, "蓝灯表示什么？", nil, "REPLAY")
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != knowledgeagent.StatusNotFound || len(result.Evidence) != 0 {
		t.Fatalf("concurrently withdrawn revision escaped recheck: %+v", result)
	}
}

type withdrawingRetriever struct {
	store    domain.Repository
	revision domain.Revision
}

func (r *withdrawingRetriever) Retrieve(
	ctx context.Context,
	_ string,
	_ ...retriever.Option,
) ([]*schema.Document, error) {
	if _, err := r.store.SetPublication(ctx, r.revision.ID, "WITHDRAWN"); err != nil {
		return nil, err
	}
	fragment := r.revision.Fragments[0]
	return []*schema.Document{{
		ID: fragment.ID, Content: fragment.Content,
		MetaData: map[string]any{"revision_id": r.revision.ID, "score": 1.0},
	}}, nil
}

func openStore(t *testing.T) *filestore.Store {
	t.Helper()
	store, err := filestore.Open(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Error(err)
		}
	})
	return store
}

func publish(t *testing.T, store domain.Repository, applicability domain.Applicability) domain.Revision {
	t.Helper()
	revision, err := knowledge.NewRevision(domain.RevisionInput{
		Title: "测试运维手册", Source: "fixture://manual",
		Content: "# 指示灯\n蓝灯表示维护模式。", Applicability: applicability,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.CreateRevision(context.Background(), revision); err != nil {
		t.Fatal(err)
	}
	revision, err = store.SetPublication(context.Background(), revision.ID, "PUBLISHED")
	if err != nil {
		t.Fatal(err)
	}
	return revision
}
