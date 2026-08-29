package experience_test

import (
	"context"
	"testing"

	"github.com/agent-experience-engine/agent-experience-engine/internal/experience"
	"github.com/agent-experience-engine/agent-experience-engine/internal/provider"
	"github.com/agent-experience-engine/agent-experience-engine/internal/retrieval"
)

func TestStoreAndRetrieveSimilarTask(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := experience.NewMemoryRepository()
	svc := experience.NewService(repo)
	embedder := &provider.MockEmbedding{Dim: 32}
	pipeline, err := experience.NewStorePipeline(svc, embedder, experience.StorePipelineConfig{})
	if err != nil {
		t.Fatalf("NewStorePipeline: %v", err)
	}
	retriever, err := retrieval.New(svc, embedder, retrieval.RankConfig{
		CandidateTopK: 10,
		DefaultTopK:   5,
	})
	if err != nil {
		t.Fatalf("retrieval.New: %v", err)
	}

	stored, err := pipeline.StoreCandidates(ctx, "tenant_a", "ep_1", []experience.Candidate{
		{
			Type: experience.TypeProcedural, Trigger: "create jira issue when project key unknown",
			Content:    "Resolve the Jira project key before calling create_issue.",
			Confidence: 0.91, Scope: experience.ScopeTool, ScopeKey: "jira",
		},
		{
			Type: experience.TypeSemantic, Trigger: "payment timeout alert rules",
			Content:    "Payment timeout alerts use severity P2.",
			Confidence: 0.9, Scope: experience.ScopeTenant,
		},
	})
	if err != nil {
		t.Fatalf("StoreCandidates: %v", err)
	}
	if len(stored.Stored) != 2 {
		t.Fatalf("stored = %d", len(stored.Stored))
	}

	results, err := retriever.Retrieve(ctx, retrieval.Query{
		TenantID: "tenant_a",
		Task:     "create jira issue when project key unknown",
		Tools:    []string{"jira"},
		TopK:     5,
	})
	if err != nil {
		t.Fatalf("Retrieve: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected retrieval hits")
	}
	found := false
	for _, r := range results {
		if r.Experience.ScopeKey == "jira" {
			found = true
			if r.Score.FinalScore <= 0 {
				t.Fatalf("expected positive final score for jira hit: %#v", r.Score)
			}
			break
		}
	}
	if !found {
		t.Fatalf("expected jira experience in results: %#v", results)
	}
}

func TestRetrievalTenantIsolation(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := experience.NewMemoryRepository()
	svc := experience.NewService(repo)
	embedder := &provider.MockEmbedding{Dim: 16}
	pipeline, _ := experience.NewStorePipeline(svc, embedder, experience.StorePipelineConfig{})
	retriever, _ := retrieval.New(svc, embedder, retrieval.RankConfig{DefaultTopK: 5})

	_, err := pipeline.StoreCandidates(ctx, "tenant_a", "ep", []experience.Candidate{{
		Type: experience.TypeProcedural, Trigger: "jira create", Content: "search project first",
		Confidence: 0.9, Scope: experience.ScopeTool, ScopeKey: "jira",
	}})
	if err != nil {
		t.Fatalf("store: %v", err)
	}

	results, err := retriever.Retrieve(ctx, retrieval.Query{
		TenantID: "tenant_b",
		Task:     "jira create issue",
	})
	if err != nil {
		t.Fatalf("retrieve: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("cross-tenant leakage: %#v", results)
	}
}

func TestStatusFromConfidenceThresholds(t *testing.T) {
	t.Parallel()
	st, ok := experience.StatusFromConfidence(0.7, 0.65, 0.4)
	if !ok || st != experience.StatusActive {
		t.Fatalf("got %v %v", st, ok)
	}
	st, ok = experience.StatusFromConfidence(0.5, 0.65, 0.4)
	if !ok || st != experience.StatusCandidate {
		t.Fatalf("got %v %v", st, ok)
	}
	_, ok = experience.StatusFromConfidence(0.2, 0.65, 0.4)
	if ok {
		t.Fatal("expected skip")
	}
}

func TestDeprecatedExcludedFromDefaultSearch(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := experience.NewMemoryRepository()
	svc := experience.NewService(repo)
	embedder := &provider.MockEmbedding{Dim: 8}

	vec, _ := embedder.Embed(ctx, []string{"same"})
	created, err := svc.Create(ctx, experience.CreateInput{
		TenantID: "t", Type: experience.TypeSemantic, Scope: experience.ScopeTenant,
		Trigger: "t", Content: "c", Confidence: 0.9, Embedding: vec[0],
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := repo.Archive(ctx, "t", created.ID); err != nil {
		t.Fatalf("archive: %v", err)
	}

	results, err := svc.Search(ctx, experience.SearchInput{
		TenantID: "t", QueryEmbedding: vec[0], TopK: 10,
	})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("archived should be excluded, got %#v", results)
	}
}
