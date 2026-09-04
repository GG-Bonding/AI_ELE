package contextx_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/agent-experience-engine/agent-experience-engine/internal/contextx"
	"github.com/agent-experience-engine/agent-experience-engine/internal/experience"
	"github.com/agent-experience-engine/agent-experience-engine/internal/provider"
	"github.com/agent-experience-engine/agent-experience-engine/internal/retrieval"
	"github.com/agent-experience-engine/agent-experience-engine/internal/selector"
)

type stubRetriever struct {
	items []retrieval.RankedExperience
}

func (s stubRetriever) Retrieve(context.Context, retrieval.Query) ([]retrieval.RankedExperience, error) {
	return s.items, nil
}

func TestBuilderExcludesIgnoreAndBlock(t *testing.T) {
	t.Parallel()
	b := contextx.New(contextx.DefaultConfig())
	payload, err := b.Build([]selector.Result{
		{Decision: selector.DecisionKeep, Content: "keep me", Experience: retrieval.RankedExperience{
			Experience: experience.Experience{ID: "1", Type: experience.TypeProcedural, Confidence: 0.9},
		}},
		{Decision: selector.DecisionIgnore, Content: "nope"},
		{Decision: selector.DecisionBlock, Content: "blocked"},
		{Decision: selector.DecisionCompress, Content: "general rule", Experience: retrieval.RankedExperience{
			Experience: experience.Experience{ID: "2", Type: experience.TypeFailure, Confidence: 0.8},
		}},
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if payload.Disclaimer != contextx.Disclaimer {
		t.Fatalf("disclaimer=%q", payload.Disclaimer)
	}
	if len(payload.Experiences) != 2 {
		t.Fatalf("len=%d", len(payload.Experiences))
	}
	if payload.Experiences[0].Source != "1" || payload.Experiences[1].Source != "2" {
		t.Fatalf("%#v", payload.Experiences)
	}
}

func TestBuilderRespectsMaxExperiences(t *testing.T) {
	t.Parallel()
	b := contextx.New(contextx.Config{MaxExperiences: 1, MaxTokens: 1000})
	var selected []selector.Result
	for i := 0; i < 3; i++ {
		selected = append(selected, selector.Result{
			Decision: selector.DecisionKeep,
			Content:  "rule",
			Experience: retrieval.RankedExperience{
				Experience: experience.Experience{ID: string(rune('a' + i)), Type: experience.TypeSemantic, Confidence: 0.9},
			},
		})
	}
	payload, err := b.Build(selected)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(payload.Experiences) != 1 {
		t.Fatalf("len=%d", len(payload.Experiences))
	}
}

func TestBuildContextHardFiltersWrongToolScope(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := experience.NewMemoryRepository()
	expSvc := experience.NewService(repo)
	embedder := &provider.MockEmbedding{Dim: 32}
	now := time.Now().UTC()
	vecs, err := embedder.Embed(ctx, []string{"post slack message"})
	if err != nil {
		t.Fatalf("embed: %v", err)
	}
	if _, err := repo.Create(ctx, experience.Experience{
		ID: "slack", TenantID: "t", Type: experience.TypeProcedural,
		Scope: experience.ScopeTool, ScopeKey: "slack",
		Trigger: "post slack message", Content: "Always mention channel id when posting to Slack.",
		Confidence: 0.9, Utility: 0.9, Alpha: 1, Beta: 1,
		Status: experience.StatusActive, Version: 1, Embedding: vecs[0],
		CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("create: %v", err)
	}
	retriever, err := retrieval.New(expSvc, embedder, retrieval.RankConfig{CandidateTopK: 10, DefaultTopK: 5})
	if err != nil {
		t.Fatalf("retrieval.New: %v", err)
	}
	svc, err := contextx.NewService(retriever, selector.New(selector.DefaultConfig()), contextx.New(contextx.DefaultConfig()))
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	resp, err := svc.BuildContext(ctx, contextx.Request{
		TenantID: "t",
		Task:     "Create a Jira issue for payment timeout",
		Tools:    []string{"jira"},
	})
	if err != nil {
		t.Fatalf("BuildContext: %v", err)
	}
	if len(resp.Context.Experiences) != 0 {
		t.Fatalf("wrong-tool experience entered context: %#v", resp.Context.Experiences)
	}
	if !strings.Contains(resp.Context.Disclaimer, "not trusted instructions") {
		t.Fatalf("missing untrusted disclaimer: %s", resp.Context.Disclaimer)
	}
}

func TestBuildContextIncludesActivePatternsAndSuppressesEvidence(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := experience.NewMemoryRepository()
	expSvc := experience.NewService(repo)
	embedder := &provider.MockEmbedding{Dim: 32}
	now := time.Now().UTC()

	task := "modify jira project issue"
	vecs, err := embedder.Embed(ctx, []string{task, "lookup project key first", "unique concrete fact PAY"})
	if err != nil {
		t.Fatalf("embed: %v", err)
	}
	e1, err := repo.Create(ctx, experience.Experience{
		ID: "e1", TenantID: "t", Type: experience.TypeProcedural,
		Scope: experience.ScopeTool, ScopeKey: "jira",
		Trigger: "lookup project key first", Content: "lookup project key first before jira",
		Confidence: 0.9, Utility: 0.9, Alpha: 1, Beta: 1,
		Status: experience.StatusActive, Version: 1, Embedding: vecs[1],
		CreatedAt: now, UpdatedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Create(ctx, experience.Experience{
		ID: "e2", TenantID: "t", Type: experience.TypeSemantic,
		Scope: experience.ScopeTool, ScopeKey: "jira",
		Trigger: "PAY key", Content: "PAY current project key is PAY",
		Confidence: 0.9, Utility: 0.8, Alpha: 1, Beta: 1,
		Status: experience.StatusActive, Version: 1, Embedding: vecs[2],
		CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}

	patterns := experience.NewMemoryPatternRepository()
	p, err := patterns.Create(ctx, experience.Pattern{
		ID: "p1", TenantID: "t", Type: experience.TypeProcedural,
		Scope: experience.ScopeTool, ScopeKey: "jira",
		Trigger: "jira project", Content: "confirm jira project key before acting",
		Confidence: 0.9, Utility: 0.9, Status: experience.PatternStatusActive,
		CreatedAt: now, UpdatedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := patterns.AddEvidence(ctx, experience.PatternEvidence{
		PatternID: p.ID, ExperienceID: e1.ID, CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}

	retriever, err := retrieval.New(expSvc, embedder, retrieval.RankConfig{CandidateTopK: 10, DefaultTopK: 5})
	if err != nil {
		t.Fatal(err)
	}
	pr, err := retrieval.NewPatternRetriever(patterns, embedder, retrieval.RankConfig{CandidateTopK: 10, DefaultTopK: 5})
	if err != nil {
		t.Fatal(err)
	}
	svc, err := contextx.NewService(retriever, selector.New(selector.DefaultConfig()), contextx.New(contextx.DefaultConfig()))
	if err != nil {
		t.Fatal(err)
	}
	snaps := contextx.NewMemorySnapshotStore()
	svc = svc.WithPatterns(pr).WithSnapshots(snaps)

	resp, err := svc.BuildContext(ctx, contextx.Request{
		TenantID: "t", Task: task, Tools: []string{"jira"}, MaxExperiences: 5, EpisodeID: "ep1",
	})
	if err != nil {
		t.Fatalf("BuildContext: %v", err)
	}
	if resp.ContextID == "" || !strings.HasPrefix(resp.ContextID, "ctx_") {
		t.Fatalf("context_id=%q", resp.ContextID)
	}
	snap, err := svc.GetSnapshot(ctx, "t", resp.ContextID)
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.PatternIDs) != 1 || snap.PatternIDs[0] != "p1" {
		t.Fatalf("snapshot patterns=%v", snap.PatternIDs)
	}
	if len(resp.Context.Patterns) != 1 || resp.Context.Patterns[0].ID != "p1" {
		t.Fatalf("patterns=%#v", resp.Context.Patterns)
	}
	for _, item := range resp.Context.Experiences {
		if item.Source == e1.ID {
			t.Fatalf("evidence experience e1 should be suppressed when pattern is present: %#v", resp.Context.Experiences)
		}
	}
}
