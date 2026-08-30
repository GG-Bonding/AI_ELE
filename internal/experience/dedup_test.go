package experience_test

import (
	"context"
	"testing"

	"github.com/agent-experience-engine/agent-experience-engine/internal/experience"
	"github.com/agent-experience-engine/agent-experience-engine/internal/outcome"
	"github.com/agent-experience-engine/agent-experience-engine/internal/provider"
)

// fixedEmbedder returns the same unit vector for every text (sim ≈ 1.0).
type fixedEmbedder struct {
	dim int
	vec []float32
}

func (f *fixedEmbedder) Dimensions() int { return f.dim }

func (f *fixedEmbedder) Embed(_ context.Context, texts []string) ([][]float32, error) {
	out := make([][]float32, len(texts))
	for i := range texts {
		out[i] = append([]float32(nil), f.vec...)
	}
	return out, nil
}

func newFixedEmbedder(dim int) *fixedEmbedder {
	vec := make([]float32, dim)
	vec[0] = 1
	return &fixedEmbedder{dim: dim, vec: vec}
}

func TestHeuristicDedupJudgeConflictVsSame(t *testing.T) {
	t.Parallel()
	j := experience.HeuristicDedupJudge{AutoSameSimilarity: 0.97}
	ctx := context.Background()

	conflict, err := j.Judge(ctx, experience.DedupPair{
		CandidateTrigger: "feature flag",
		CandidateContent: "必须打开开关 before deploy",
		NeighborTrigger:  "feature flag",
		NeighborContent:  "禁止打开开关 before deploy",
		Similarity:       0.99,
	})
	if err != nil {
		t.Fatal(err)
	}
	if conflict != experience.DedupConflict {
		t.Fatalf("got %s want CONFLICT", conflict)
	}

	same, err := j.Judge(ctx, experience.DedupPair{
		CandidateTrigger: "create jira issue when project key unknown",
		CandidateContent: "Resolve the Jira project key before calling create_issue.",
		NeighborTrigger:  "create jira issue when project key unknown",
		NeighborContent:  "Resolve the Jira project key before calling create_issue.",
		Similarity:       0.99,
	})
	if err != nil {
		t.Fatal(err)
	}
	if same != experience.DedupSame {
		t.Fatalf("got %s want SAME", same)
	}
}

func TestSemanticDedupReinforcesInsteadOfInsert(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := experience.NewMemoryRepository()
	svc := experience.NewService(repo)
	embedder := newFixedEmbedder(8)
	pipeline, err := experience.NewStorePipeline(svc, embedder, experience.StorePipelineConfig{})
	if err != nil {
		t.Fatalf("NewStorePipeline: %v", err)
	}

	cand := experience.Candidate{
		Type: experience.TypeProcedural, Trigger: "create jira when project key unknown",
		Content:    "Resolve Jira project key before create_issue.",
		Confidence: 0.9, Scope: experience.ScopeTool, ScopeKey: "jira",
	}
	first, err := pipeline.StoreCandidatesWithOptions(ctx, "tenant_a", "ep_1", []experience.Candidate{cand}, experience.StoreOptions{
		Outcome: outcome.Outcome{Status: "SUCCESS", Verified: true},
	})
	if err != nil {
		t.Fatalf("first store: %v", err)
	}
	if len(first.Stored) != 1 || len(first.Reinforced) != 0 {
		t.Fatalf("first: stored=%d reinforced=%d", len(first.Stored), len(first.Reinforced))
	}
	id := first.Stored[0].ID
	utilBefore := first.Stored[0].Utility
	confBefore := first.Stored[0].Confidence

	// Synonymous wording; fixed embedder → sim=1 → SAME.
	synonym := experience.Candidate{
		Type: experience.TypeProcedural, Trigger: "create jira issue if project key is unknown",
		Content:    "Resolve Jira project key before create_issue.",
		Confidence: 0.88, Scope: experience.ScopeTool, ScopeKey: "jira",
	}
	second, err := pipeline.StoreCandidatesWithOptions(ctx, "tenant_a", "ep_2", []experience.Candidate{synonym}, experience.StoreOptions{
		Outcome: outcome.Outcome{Status: "SUCCESS", Verified: true},
	})
	if err != nil {
		t.Fatalf("second store: %v", err)
	}
	if len(second.Stored) != 0 {
		t.Fatalf("expected no new ACTIVE insert, got %#v", second.Stored)
	}
	if len(second.Reinforced) != 1 {
		t.Fatalf("reinforced=%d want 1", len(second.Reinforced))
	}
	got := second.Reinforced[0]
	if got.ID != id {
		t.Fatalf("reinforced id=%s want %s", got.ID, id)
	}
	if got.Utility != utilBefore {
		t.Fatalf("utility changed on reinforce: %v → %v", utilBefore, got.Utility)
	}
	if got.Confidence <= confBefore {
		t.Fatalf("confidence should rise: before=%v after=%v", confBefore, got.Confidence)
	}
	if got.Evidence.SupportCount() < 2 {
		t.Fatalf("support count=%d want >=2 ids=%v", got.Evidence.SupportCount(), got.Evidence.SupportEpisodeIDs)
	}

	// Only one ACTIVE row for this tenant/type/scope.
	hits, err := svc.Search(ctx, experience.SearchInput{
		TenantID: "tenant_a", Types: []experience.Type{experience.TypeProcedural},
		Scopes: []experience.Scope{experience.ScopeTool}, ScopeKey: "jira",
		QueryEmbedding: embedder.vec, TopK: 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 1 {
		t.Fatalf("active rows=%d want 1", len(hits))
	}
}

func TestSemanticDedupConflictDoesNotMerge(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := experience.NewMemoryRepository()
	svc := experience.NewService(repo)
	embedder := newFixedEmbedder(8)
	pipeline, err := experience.NewStorePipeline(svc, embedder, experience.StorePipelineConfig{})
	if err != nil {
		t.Fatal(err)
	}

	_, err = pipeline.StoreCandidatesWithOptions(ctx, "t", "ep_a", []experience.Candidate{{
		Type: experience.TypeConstraint, Trigger: "feature flag",
		Content:    "必须打开开关 before deploy",
		Confidence: 0.9, Scope: experience.ScopeTenant,
	}}, experience.StoreOptions{Outcome: outcome.Outcome{Status: "SUCCESS", Verified: true}})
	if err != nil {
		t.Fatal(err)
	}

	second, err := pipeline.StoreCandidatesWithOptions(ctx, "t", "ep_b", []experience.Candidate{{
		Type: experience.TypeConstraint, Trigger: "feature flag",
		Content:    "禁止打开开关 before deploy",
		Confidence: 0.9, Scope: experience.ScopeTenant,
	}}, experience.StoreOptions{Outcome: outcome.Outcome{Status: "SUCCESS", Verified: true}})
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Stored) != 1 || len(second.Reinforced) != 0 {
		t.Fatalf("conflict should insert, stored=%d reinforced=%d", len(second.Stored), len(second.Reinforced))
	}
}

func TestExactFingerprintStillIdempotentWithinEpisode(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := experience.NewMemoryRepository()
	svc := experience.NewService(repo)
	embedder := &provider.MockEmbedding{Dim: 8}
	pipeline, err := experience.NewStorePipeline(svc, embedder, experience.StorePipelineConfig{
		SemanticDedup: experience.SemanticDedupConfig{Disabled: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	cand := experience.Candidate{
		Type: experience.TypeSemantic, Trigger: "same", Content: "body",
		Confidence: 0.9, Scope: experience.ScopeTenant,
	}
	a, err := pipeline.StoreCandidates(ctx, "t", "ep", []experience.Candidate{cand})
	if err != nil {
		t.Fatal(err)
	}
	b, err := pipeline.StoreCandidates(ctx, "t", "ep", []experience.Candidate{cand})
	if err != nil {
		t.Fatal(err)
	}
	if len(a.Stored) != 1 || len(b.Stored) != 1 {
		t.Fatalf("stored a=%d b=%d", len(a.Stored), len(b.Stored))
	}
	if a.Stored[0].ID != b.Stored[0].ID {
		t.Fatalf("exact dedup should return same id")
	}
}

func TestReinforceDoesNotChangeUtility(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := experience.NewMemoryRepository()
	svc := experience.NewService(repo)
	embedder := &provider.MockEmbedding{Dim: 4}
	vec, _ := embedder.Embed(ctx, []string{"x"})
	created, err := svc.Create(ctx, experience.CreateInput{
		TenantID: "t", Type: experience.TypeSemantic, Scope: experience.ScopeTenant,
		Trigger: "t", Content: "c", SourceEpisodeID: "ep1",
		Confidence: 0.7, Embedding: vec[0],
		Evidence: experience.Evidence{SourceEpisodeID: "ep1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	created.Utility = 0.42
	created.Alpha, created.Beta = 3, 4
	updated, err := repo.Update(ctx, created)
	if err != nil {
		t.Fatal(err)
	}

	got, err := svc.Reinforce(ctx, "t", updated.ID, experience.ReinforceInput{
		EpisodeID: "ep2", Confidence: 0.8,
		Evidence: experience.Evidence{SourceEpisodeID: "ep2", SuccessAttemptCount: 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Utility != 0.42 || got.Alpha != 3 || got.Beta != 4 {
		t.Fatalf("utility fields changed: util=%v α=%v β=%v", got.Utility, got.Alpha, got.Beta)
	}
	if got.Confidence <= 0.7 {
		t.Fatalf("confidence=%v", got.Confidence)
	}
	if got.Evidence.SupportCount() != 2 {
		t.Fatalf("support=%d ids=%v", got.Evidence.SupportCount(), got.Evidence.SupportEpisodeIDs)
	}
}
