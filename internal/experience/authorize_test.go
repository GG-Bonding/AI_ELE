package experience_test

import (
	"context"
	"errors"
	"testing"

	"github.com/agent-experience-engine/agent-experience-engine/internal/experience"
)

func TestCreateRequiresScopeKeyForScopedExperiences(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	svc := experience.NewService(experience.NewMemoryRepository())
	vec := []float32{1, 0, 0}

	scopes := []experience.Scope{experience.ScopeUser, experience.ScopeAgent, experience.ScopeTool}
	for _, scope := range scopes {
		_, err := svc.Create(ctx, experience.CreateInput{
			TenantID: "t", Type: experience.TypeSemantic, Scope: scope,
			Trigger: "t", Content: "c", Confidence: 0.9, Embedding: vec,
		})
		if !errors.Is(err, experience.ErrInvalidInput) {
			t.Fatalf("scope %s with empty scope_key: got err=%v want ErrInvalidInput", scope, err)
		}
	}

	_, err := svc.Create(ctx, experience.CreateInput{
		TenantID: "t", Type: experience.TypeSemantic, Scope: experience.ScopeTenant,
		Trigger: "t", Content: "c", Confidence: 0.9, Embedding: vec,
	})
	if err != nil {
		t.Fatalf("tenant scope without scope_key should succeed: %v", err)
	}
}

func TestMemorySearchAuthBeforeTopK(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := experience.NewMemoryRepository()
	svc := experience.NewService(repo)
	vec := []float32{1, 0, 0}

	// High-similarity unauthorized experience should not consume TopK slot.
	_, err := svc.Create(ctx, experience.CreateInput{
		TenantID: "t", Type: experience.TypeSemantic, Scope: experience.ScopeTool, ScopeKey: "slack",
		Trigger: "slack tip", Content: "use channels", Confidence: 0.9, Embedding: vec,
	})
	if err != nil {
		t.Fatalf("create slack: %v", err)
	}
	jiraVec := []float32{0.99, 0.01, 0}
	_, err = svc.Create(ctx, experience.CreateInput{
		TenantID: "t", Type: experience.TypeSemantic, Scope: experience.ScopeTool, ScopeKey: "jira",
		Trigger: "jira tip", Content: "resolve project key", Confidence: 0.9, Embedding: jiraVec,
	})
	if err != nil {
		t.Fatalf("create jira: %v", err)
	}

	results, err := svc.Search(ctx, experience.SearchInput{
		TenantID: "t", QueryEmbedding: vec, Tools: []string{"jira"}, TopK: 1,
	})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(results) != 1 || results[0].Experience.ScopeKey != "jira" {
		t.Fatalf("expected authorized jira hit only, got %#v", results)
	}
}
