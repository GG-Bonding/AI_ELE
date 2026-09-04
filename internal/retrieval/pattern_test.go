package retrieval_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/agent-experience-engine/agent-experience-engine/internal/experience"
	"github.com/agent-experience-engine/agent-experience-engine/internal/provider"
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
	embedder := &provider.MockEmbedding{Dim: 16}
	now := time.Now().UTC()
	task := "modify jira project issue"

	vecs, err := embedder.Embed(ctx, []string{task})
	if err != nil {
		t.Fatal(err)
	}

	active, err := repo.Create(ctx, experience.Pattern{
		ID: "p-active", TenantID: "t", Type: experience.TypeProcedural,
		Scope: experience.ScopeTool, ScopeKey: "jira",
		Trigger: "jira project", Content: "confirm jira project key before acting",
		Confidence: 0.9, Utility: 0.85, Status: experience.PatternStatusActive,
		Embedding: vecs[0], CreatedAt: now, UpdatedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Create(ctx, experience.Pattern{
		ID: "p-cand", TenantID: "t", Type: experience.TypeProcedural,
		Scope: experience.ScopeTool, ScopeKey: "jira",
		Trigger: "jira project", Content: "confirm jira project key before acting",
		Confidence: 0.9, Utility: 0.99, Status: experience.PatternStatusCandidate,
		Embedding: vecs[0], CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	if err := repo.AddEvidence(ctx, experience.PatternEvidence{
		PatternID: active.ID, ExperienceID: "e1", CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}

	pr, err := retrieval.NewPatternRetriever(repo, embedder, retrieval.DefaultRankConfig())
	if err != nil {
		t.Fatal(err)
	}
	got, err := pr.RetrievePatterns(ctx, retrieval.Query{
		TenantID: "t", Task: task, Tools: []string{"jira"},
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
	if got[0].Score.Validity <= 0 || got[0].Score.Similarity <= 0 {
		t.Fatalf("score=%#v", got[0].Score)
	}
}

func TestRetrievePatternsHardFiltersWrongTool(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := experience.NewMemoryPatternRepository()
	embedder := &provider.MockEmbedding{Dim: 8}
	now := time.Now().UTC()
	vecs, err := embedder.Embed(ctx, []string{"jira key\nconfirm jira project key"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Create(ctx, experience.Pattern{
		ID: "p1", TenantID: "t", Type: experience.TypeProcedural,
		Scope: experience.ScopeTool, ScopeKey: "jira",
		Trigger: "jira key", Content: "confirm jira project key",
		Confidence: 0.9, Utility: 0.9, Status: experience.PatternStatusActive,
		Embedding: vecs[0], CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	pr, err := retrieval.NewPatternRetriever(repo, embedder, retrieval.DefaultRankConfig())
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

// synonymEmbedder maps semantically related Chinese/English project tips to one vector.
type synonymEmbedder struct{}

func (synonymEmbedder) Dimensions() int { return 4 }

func (synonymEmbedder) Embed(_ context.Context, texts []string) ([][]float32, error) {
	out := make([][]float32, len(texts))
	for i, text := range texts {
		lower := strings.ToLower(text)
		if strings.Contains(text, "项目") || strings.Contains(lower, "pay") ||
			strings.Contains(lower, "jira") || strings.Contains(lower, "project") {
			out[i] = []float32{1, 0, 0, 0}
			continue
		}
		out[i] = []float32{0, 1, 0, 0}
	}
	return out, nil
}

func TestRetrievePatternsSemanticBeatsLexical(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := experience.NewMemoryPatternRepository()
	embedder := synonymEmbedder{}
	now := time.Now().UTC()

	patternText := "创建工单前，如果项目标识未知，应先解析项目标识。"
	task := "帮我在 PAY 下新建 Jira bug"
	if retrieval.LexicalOverlap(task, patternText) >= 0.15 {
		t.Fatalf("test setup broken: lexical should be low, got %v", retrieval.LexicalOverlap(task, patternText))
	}
	vecs, err := embedder.Embed(ctx, []string{patternText})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Create(ctx, experience.Pattern{
		ID: "p-sem", TenantID: "t", Type: experience.TypeProcedural,
		Scope: experience.ScopeTool, ScopeKey: "jira",
		Trigger: "项目标识", Content: patternText,
		Confidence: 0.9, Utility: 0.9, Status: experience.PatternStatusActive,
		Embedding: vecs[0], CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}

	pr, err := retrieval.NewPatternRetriever(repo, embedder, retrieval.DefaultRankConfig())
	if err != nil {
		t.Fatal(err)
	}
	got, err := pr.RetrievePatterns(ctx, retrieval.Query{
		TenantID: "t", Task: task, Tools: []string{"jira"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Pattern.ID != "p-sem" {
		t.Fatalf("semantic retrieval failed: %#v", got)
	}
	if got[0].Score.Similarity < 0.99 {
		t.Fatalf("want near-1 similarity, got %#v", got[0].Score)
	}
}

func TestRetrievePatternsLegacyLexicalWithoutEmbedding(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := experience.NewMemoryPatternRepository()
	embedder := &provider.MockEmbedding{Dim: 8}
	now := time.Now().UTC()
	if _, err := repo.Create(ctx, experience.Pattern{
		ID: "p-legacy", TenantID: "t", Type: experience.TypeProcedural,
		Scope: experience.ScopeTool, ScopeKey: "jira",
		Trigger: "jira project", Content: "confirm jira project key before acting",
		Confidence: 0.9, Utility: 0.85, Status: experience.PatternStatusActive,
		CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	pr, err := retrieval.NewPatternRetriever(repo, embedder, retrieval.DefaultRankConfig())
	if err != nil {
		t.Fatal(err)
	}
	got, err := pr.RetrievePatterns(ctx, retrieval.Query{
		TenantID: "t", Task: "modify jira project issue", Tools: []string{"jira"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Pattern.ID != "p-legacy" {
		t.Fatalf("legacy lexical fallback failed: %#v", got)
	}
}
