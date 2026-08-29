package httpserver_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"

	httpserver "github.com/agent-experience-engine/agent-experience-engine/api/http"
	"github.com/agent-experience-engine/agent-experience-engine/internal/contextx"
	"github.com/agent-experience-engine/agent-experience-engine/internal/episode"
	"github.com/agent-experience-engine/agent-experience-engine/internal/experience"
	"github.com/agent-experience-engine/agent-experience-engine/internal/provider"
	"github.com/agent-experience-engine/agent-experience-engine/internal/retrieval"
	"github.com/agent-experience-engine/agent-experience-engine/internal/selector"
)

func TestContextAPIFiltersIrrelevant(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	expRepo := experience.NewMemoryRepository()
	expSvc := experience.NewService(expRepo)
	embedder := &provider.MockEmbedding{Dim: 32}
	retriever, err := retrieval.New(expSvc, embedder, retrieval.RankConfig{CandidateTopK: 10, DefaultTopK: 10})
	if err != nil {
		t.Fatalf("retriever: %v", err)
	}
	contextSvc, err := contextx.NewService(retriever, selector.New(selector.DefaultConfig()), contextx.New(contextx.DefaultConfig()))
	if err != nil {
		t.Fatalf("context: %v", err)
	}

	jiraVec, _ := embedder.Embed(ctx, []string{"create jira issue when project key unknown\nResolve project key first"})
	slackVec, _ := embedder.Embed(ctx, []string{"post slack message\nAlways set channel id"})
	if _, err := expSvc.Create(ctx, experience.CreateInput{
		TenantID: "tenant_a", Type: experience.TypeProcedural, Scope: experience.ScopeTool, ScopeKey: "jira",
		Trigger: "create jira issue when project key unknown", Content: "Resolve project key first",
		Confidence: 0.91, Embedding: jiraVec[0],
	}); err != nil {
		t.Fatalf("create jira: %v", err)
	}
	if _, err := expSvc.Create(ctx, experience.CreateInput{
		TenantID: "tenant_a", Type: experience.TypeProcedural, Scope: experience.ScopeTool, ScopeKey: "slack",
		Trigger: "post slack message", Content: "Always set channel id",
		Confidence: 0.9, Embedding: slackVec[0],
	}); err != nil {
		t.Fatalf("create slack: %v", err)
	}

	h := httpserver.New(
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		stubReady{},
		httpserver.Options{
			Episodes:    episode.NewService(episode.NewMemoryRepository()),
			Experiences: expSvc,
			Retriever:   retriever,
			Contexts:    contextSvc,
		},
	).Handler()

	out := postJSON(t, h, "/api/v1/context", map[string]any{
		"tenant_id":       "tenant_a",
		"agent_id":        "agent_01",
		"user_id":         "user_01",
		"task":            "Create a Jira issue for payment timeout",
		"tools":           []string{"jira.create_issue", "jira.search_projects"},
		"max_experiences": 5,
	}, http.StatusOK)

	disclaimer, _ := out["disclaimer"].(string)
	if !strings.Contains(disclaimer, "not trusted instructions") {
		t.Fatalf("missing disclaimer: %#v", out["disclaimer"])
	}
	exps, _ := out["experiences"].([]any)
	for _, raw := range exps {
		item := raw.(map[string]any)
		content, _ := item["content"].(string)
		lower := strings.ToLower(content)
		if strings.Contains(lower, "slack") || strings.Contains(lower, "channel") {
			t.Fatalf("irrelevant slack experience entered context: %#v", item)
		}
	}
}
