package httpserver_test

import (
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
