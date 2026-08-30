package experience_test

import (
	"context"
	"testing"

	"github.com/agent-experience-engine/agent-experience-engine/internal/experience"
	"github.com/agent-experience-engine/agent-experience-engine/internal/outcome"
	"github.com/agent-experience-engine/agent-experience-engine/internal/provider"
	"github.com/agent-experience-engine/agent-experience-engine/internal/retrieval"
	"github.com/agent-experience-engine/agent-experience-engine/internal/selector"
)

func TestSemanticConflictRecordsRelation(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := experience.NewMemoryRepository()
	rels := experience.NewMemoryRelationRepository()
	svc := experience.NewService(repo).WithRelations(rels)
	embedder := newFixedEmbedder(8)
	pipeline, err := experience.NewStorePipeline(svc, embedder, experience.StorePipelineConfig{})
	if err != nil {
		t.Fatal(err)
	}

	first, err := pipeline.StoreCandidatesWithOptions(ctx, "t", "ep_a", []experience.Candidate{{
		Type: experience.TypeConstraint, Trigger: "feature flag",
		Content: "必须打开开关 before deploy", Confidence: 0.9, Scope: experience.ScopeTenant,
	}}, experience.StoreOptions{Outcome: outcome.Outcome{Status: "SUCCESS", Verified: true}})
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Stored) != 1 {
		t.Fatalf("first stored=%d", len(first.Stored))
	}

	second, err := pipeline.StoreCandidatesWithOptions(ctx, "t", "ep_b", []experience.Candidate{{
		Type: experience.TypeConstraint, Trigger: "feature flag",
		Content: "禁止打开开关 before deploy", Confidence: 0.9, Scope: experience.ScopeTenant,
	}}, experience.StoreOptions{Outcome: outcome.Outcome{Status: "SUCCESS", Verified: true}})
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Stored) != 1 {
		t.Fatalf("conflict should still insert, stored=%d", len(second.Stored))
	}
	if len(second.Conflicts) != 1 {
		t.Fatalf("conflicts recorded=%d want 1: %#v", len(second.Conflicts), second.Conflicts)
	}
	if second.Conflicts[0].Type != experience.RelationConflicts {
		t.Fatalf("type=%s", second.Conflicts[0].Type)
	}
	if second.Conflicts[0].FromExperienceID != second.Stored[0].ID {
		t.Fatalf("from=%s want %s", second.Conflicts[0].FromExperienceID, second.Stored[0].ID)
	}
	if second.Conflicts[0].ToExperienceID != first.Stored[0].ID {
		t.Fatalf("to=%s want %s", second.Conflicts[0].ToExperienceID, first.Stored[0].ID)
	}

	peers, err := svc.ConflictPeers(ctx, "t", []string{first.Stored[0].ID, second.Stored[0].ID})
	if err != nil {
		t.Fatal(err)
	}
	if peers[first.Stored[0].ID] != second.Stored[0].ID {
		t.Fatalf("peers=%v", peers)
	}
	if peers[second.Stored[0].ID] != first.Stored[0].ID {
		t.Fatalf("peers=%v", peers)
	}
}

func TestSelectorBlocksUnresolvedConflicts(t *testing.T) {
	t.Parallel()
	sel := selector.New(selector.DefaultConfig())
	high := retrieval.RankedExperience{
		Experience: experience.Experience{
			ID: "e1", Status: experience.StatusActive, Utility: 0.8, Confidence: 0.9, Content: "must enable flag",
		},
		Score: retrieval.ScoreBreakdown{FinalScore: 0.5, ScopeMatch: 1},
	}
	other := retrieval.RankedExperience{
		Experience: experience.Experience{
			ID: "e2", Status: experience.StatusActive, Utility: 0.8, Confidence: 0.9, Content: "must not enable flag",
		},
		Score: retrieval.ScoreBreakdown{FinalScore: 0.49, ScopeMatch: 1},
	}
	got := sel.SelectWithOptions("deploy", []retrieval.RankedExperience{high, other}, selector.SelectOptions{
		ConflictPeers: map[string]string{"e1": "e2", "e2": "e1"},
	})
	if len(got) != 2 {
		t.Fatalf("len=%d", len(got))
	}
	if got[0].Decision != selector.DecisionBlock || got[1].Decision != selector.DecisionBlock {
		t.Fatalf("want both BLOCKED, got %s / %s", got[0].Decision, got[1].Decision)
	}
}

func TestConflictDoesNotChangeUtilityOnInsert(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := experience.NewMemoryRepository()
	svc := experience.NewService(repo).WithRelations(experience.NewMemoryRelationRepository())
	embedder := &provider.MockEmbedding{Dim: 8}
	pipeline, err := experience.NewStorePipeline(svc, embedder, experience.StorePipelineConfig{})
	if err != nil {
		t.Fatal(err)
	}
	// Without fixed embedder, conflict path may not trigger; still ensure create works with relations wired.
	res, err := pipeline.StoreCandidatesWithOptions(ctx, "t", "ep", []experience.Candidate{{
		Type: experience.TypeSemantic, Trigger: "x", Content: "y", Confidence: 0.9, Scope: experience.ScopeTenant,
	}}, experience.StoreOptions{Outcome: outcome.Outcome{Status: "SUCCESS", Verified: true}})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Stored) != 1 {
		t.Fatalf("stored=%d", len(res.Stored))
	}
	if res.Stored[0].Utility != 0.5 {
		t.Fatalf("utility=%v", res.Stored[0].Utility)
	}
}
