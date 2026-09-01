package retrieval_test

import (
	"context"
	"testing"
	"time"

	"github.com/agent-experience-engine/agent-experience-engine/internal/experience"
	"github.com/agent-experience-engine/agent-experience-engine/internal/retrieval"
)

func TestLexicalOverlap(t *testing.T) {
	t.Parallel()
	if got := retrieval.LexicalOverlap("confirm jira project key", "jira project key before create"); got <= 0 {
		t.Fatalf("overlap=%v", got)
	}
	if got := retrieval.LexicalOverlap("alpha", "beta"); got != 0 {
		t.Fatalf("want 0, got %v", got)
	}
}

func TestRetrievePatternsRanksActiveAndSkipsCandidate(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := experience.NewMemoryPatternRepository()
	now := time.Now().UTC()

	active, err := repo.Create(ctx, experience.Pattern{
		ID: "p-active", TenantID: "t", Type: experience.TypeProcedural,
		Scope: experience.ScopeTool, ScopeKey: "jira",
		Trigger: "jira project", Content: "confirm jira project key before acting",
		Confidence: 0.9, Utility: 0.85, Status: experience.PatternStatusActive,
		CreatedAt: now, UpdatedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Create(ctx, experience.Pattern{
		ID: "p-cand", TenantID: "t", Type: experience.TypeProcedural,
		Scope: experience.ScopeTool, ScopeKey: "jira",
		Trigger: "jira project", Content: "confirm jira project key before acting",
		Confidence: 0.9, Utility: 0.99, Status: experience.PatternStatusCandidate,
		CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	if err := repo.AddEvidence(ctx, experience.PatternEvidence{
		PatternID: active.ID, ExperienceID: "e1", CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}

	pr, err := retrieval.NewPatternRetriever(repo)
	if err != nil {
		t.Fatal(err)
	}
	got, err := pr.RetrievePatterns(ctx, retrieval.Query{
		TenantID: "t", Task: "modify jira project issue", Tools: []string{"jira"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Pattern.ID != "p-active" {
		t.Fatalf("got %#v", got)
	}
	if len(got[0].EvidenceIDs) != 1 || got[0].EvidenceIDs[0] != "e1" {
		t.Fatalf("evidence=%v", got[0].EvidenceIDs)
	}
}

func TestRetrievePatternsHardFiltersWrongTool(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := experience.NewMemoryPatternRepository()
	now := time.Now().UTC()
	if _, err := repo.Create(ctx, experience.Pattern{
		ID: "p1", TenantID: "t", Type: experience.TypeProcedural,
		Scope: experience.ScopeTool, ScopeKey: "jira",
		Trigger: "jira key", Content: "confirm jira project key",
		Confidence: 0.9, Utility: 0.9, Status: experience.PatternStatusActive,
		CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	pr, err := retrieval.NewPatternRetriever(repo)
	if err != nil {
		t.Fatal(err)
	}
	got, err := pr.RetrievePatterns(ctx, retrieval.Query{
		TenantID: "t", Task: "confirm jira project key", Tools: []string{"slack"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("want empty, got %#v", got)
	}
}
