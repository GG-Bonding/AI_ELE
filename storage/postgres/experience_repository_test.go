package postgres_test

import (
	"context"
	"testing"

	"github.com/agent-experience-engine/agent-experience-engine/internal/experience"
	"github.com/agent-experience-engine/agent-experience-engine/internal/provider"
	"github.com/agent-experience-engine/agent-experience-engine/internal/retrieval"
	"github.com/agent-experience-engine/agent-experience-engine/storage/postgres"
)

func TestExperienceRepositoryVectorSearch(t *testing.T) {
	db := openTestDB(t)
	repo := postgres.NewExperienceRepository(db)
	svc := experience.NewService(repo)
	embedder := &provider.MockEmbedding{Dim: 1536}
	pipeline, err := experience.NewStorePipeline(svc, embedder, experience.StorePipelineConfig{})
	if err != nil {
		t.Fatalf("pipeline: %v", err)
	}
	retriever, err := retrieval.New(svc, embedder, retrieval.RankConfig{
		CandidateTopK: 10,
		DefaultTopK:   5,
	})
	if err != nil {
		t.Fatalf("retriever: %v", err)
	}
	ctx := context.Background()

	_, err = pipeline.StoreCandidates(ctx, "tenant_pg_exp", "ep_pg_1", []experience.Candidate{
		{
			Type: experience.TypeProcedural, Trigger: "create jira issue when project key unknown",
			Content:    "Resolve Jira project key before create_issue.",
			Confidence: 0.92, Scope: experience.ScopeTool, ScopeKey: "jira",
		},
		{
			Type: experience.TypeFailure, Trigger: "using display name as jira project key",
			Content:    "Do not use display name as project key.",
			Confidence: 0.9, Scope: experience.ScopeTool, ScopeKey: "jira",
		},
	})
	if err != nil {
		t.Fatalf("StoreCandidates: %v", err)
	}

	results, err := retriever.Retrieve(ctx, retrieval.Query{
		TenantID: "tenant_pg_exp",
		Task:     "Create another Jira issue when project key is unknown",
		TopK:     5,
	})
	if err != nil {
		t.Fatalf("Retrieve: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected hits")
	}
	for _, r := range results {
		if r.Experience.TenantID != "tenant_pg_exp" {
			t.Fatalf("tenant leak: %s", r.Experience.TenantID)
		}
		if r.Experience.Status != experience.StatusActive && r.Experience.Status != experience.StatusCandidate {
			t.Fatalf("unexpected status %s", r.Experience.Status)
		}
	}

	cross, err := retriever.Retrieve(ctx, retrieval.Query{
		TenantID: "other_tenant",
		Task:     "Create another Jira issue when project key is unknown",
	})
	if err != nil {
		t.Fatalf("cross retrieve: %v", err)
	}
	if len(cross) != 0 {
		t.Fatalf("cross-tenant leakage: %#v", cross)
	}

	got, err := svc.Get(ctx, "tenant_pg_exp", results[0].Experience.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Content == "" {
		t.Fatal("empty content")
	}
}
