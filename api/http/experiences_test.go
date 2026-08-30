package httpserver_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"testing"

	httpserver "github.com/agent-experience-engine/agent-experience-engine/api/http"
	"github.com/agent-experience-engine/agent-experience-engine/internal/episode"
	"github.com/agent-experience-engine/agent-experience-engine/internal/experience"
	"github.com/agent-experience-engine/agent-experience-engine/internal/provider"
	"github.com/agent-experience-engine/agent-experience-engine/internal/retrieval"
)

func TestExperienceSearchHTTP(t *testing.T) {
	t.Parallel()

	expRepo := experience.NewMemoryRepository()
	expSvc := experience.NewService(expRepo)
	embedder := &provider.MockEmbedding{Dim: 32}
	pipeline, err := experience.NewStorePipeline(expSvc, embedder, experience.StorePipelineConfig{})
	if err != nil {
		t.Fatalf("pipeline: %v", err)
	}
	retriever, err := retrieval.New(expSvc, embedder, retrieval.RankConfig{
		CandidateTopK: 10,
		DefaultTopK:   5,
	})
	if err != nil {
		t.Fatalf("retriever: %v", err)
	}

	epSvc := episode.NewService(episode.NewMemoryRepository())
	h := httpserver.New(
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		stubReady{},
		httpserver.Options{
			Episodes:      epSvc,
			Experiences:   expSvc,
			Retriever:     retriever,
			StorePipeline: pipeline,
			Extractor: stubExtractor{
				candidates: []experience.Candidate{{
					Type: experience.TypeProcedural, Trigger: "create jira issue when project key unknown",
					Content: "Resolve project key before create_issue", Confidence: 0.9,
					Scope: experience.ScopeTool, ScopeKey: "jira",
				}},
			},
		},
	).Handler()

	ep := postJSON(t, h, "/api/v1/episodes", map[string]any{
		"tenant_id": "tenant_a", "agent_id": "a", "user_id": "u", "goal": "Create Jira issue",
	}, http.StatusCreated)
	id, _ := ep["id"].(string)

	out := postJSON(t, h, "/api/v1/episodes/"+id+"/outcome", map[string]any{
		"tenant_id": "tenant_a", "status": "SUCCESS",
	}, http.StatusCreated)
	stored, ok := out["stored_experiences"].([]any)
	if !ok || len(stored) != 1 {
		t.Fatalf("stored_experiences = %#v", out["stored_experiences"])
	}

	search := postJSON(t, h, "/api/v1/experiences/search", map[string]any{
		"tenant_id": "tenant_a",
		"task":      "Create a Jira issue for payment timeout",
		"tools":     []string{"jira"},
		"top_k":     5,
	}, http.StatusOK)
	exps, ok := search["experiences"].([]any)
	if !ok || len(exps) == 0 {
		t.Fatalf("search experiences = %#v", search["experiences"])
	}

	first := exps[0].(map[string]any)
	getJSON(t, h, "/api/v1/experiences/"+first["id"].(string)+"?tenant_id=tenant_a", http.StatusOK)
	getJSON(t, h, "/api/v1/experiences/"+first["id"].(string)+"?tenant_id=tenant_b", http.StatusNotFound)
}

func TestSupersedeExperienceHTTP(t *testing.T) {
	t.Parallel()

	expRepo := experience.NewMemoryRepository()
	expSvc := experience.NewService(expRepo)
	embedder := &provider.MockEmbedding{Dim: 16}
	ctxVec, _ := embedder.Embed(context.Background(), []string{"jira project key"})
	oldExp, err := expSvc.Create(context.Background(), experience.CreateInput{
		TenantID: "tenant_a", Type: experience.TypeSemantic, Scope: experience.ScopeTool, ScopeKey: "jira",
		Trigger: "jira project key", Content: "PAYMENT", Confidence: 0.9, Embedding: ctxVec[0],
	})
	if err != nil {
		t.Fatalf("create old: %v", err)
	}
	newExp, err := expSvc.Create(context.Background(), experience.CreateInput{
		TenantID: "tenant_a", Type: experience.TypeSemantic, Scope: experience.ScopeTool, ScopeKey: "jira",
		Trigger: "jira project key", Content: "PAY", Confidence: 0.95, Embedding: ctxVec[0],
	})
	if err != nil {
		t.Fatalf("create new: %v", err)
	}

	retriever, err := retrieval.New(expSvc, embedder, retrieval.RankConfig{DefaultTopK: 5})
	if err != nil {
		t.Fatalf("retriever: %v", err)
	}
	h := httpserver.New(
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		stubReady{},
		httpserver.Options{Experiences: expSvc, Retriever: retriever},
	).Handler()

	out := postJSON(t, h, "/api/v1/experiences/"+oldExp.ID+"/supersede", map[string]any{
		"tenant_id": "tenant_a", "replacement_id": newExp.ID,
	}, http.StatusOK)
	dep, _ := out["deprecated"].(map[string]any)
	rep, _ := out["replacement"].(map[string]any)
	if dep["status"] != "DEPRECATED" {
		t.Fatalf("deprecated=%#v", dep)
	}
	if rep["supersedes_id"] != oldExp.ID {
		t.Fatalf("replacement=%#v", rep)
	}

	search := postJSON(t, h, "/api/v1/experiences/search", map[string]any{
		"tenant_id": "tenant_a", "task": "jira project key", "tools": []string{"jira"}, "top_k": 5,
	}, http.StatusOK)
	for _, raw := range search["experiences"].([]any) {
		item := raw.(map[string]any)
		if item["id"] == oldExp.ID {
			t.Fatalf("deprecated experience returned: %#v", search)
		}
	}
}
