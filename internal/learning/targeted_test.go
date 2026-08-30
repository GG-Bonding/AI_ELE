package learning_test

import (
	"context"
	"testing"

	"github.com/agent-experience-engine/agent-experience-engine/internal/action"
	"github.com/agent-experience-engine/agent-experience-engine/internal/attribution"
	"github.com/agent-experience-engine/agent-experience-engine/internal/episode"
	"github.com/agent-experience-engine/agent-experience-engine/internal/experience"
	"github.com/agent-experience-engine/agent-experience-engine/internal/feedback"
	"github.com/agent-experience-engine/agent-experience-engine/internal/learning"
)

func TestTargetedFeedbackOnlyUpdatesLinkedExperience(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	epSvc := episode.NewService(episode.NewMemoryRepository())
	ep, err := epSvc.CreateEpisode(ctx, episode.CreateEpisodeInput{
		TenantID: "t", AgentID: "a", UserID: "u", Goal: "create jira issue",
	})
	if err != nil {
		t.Fatalf("CreateEpisode: %v", err)
	}

	expRepo := experience.NewMemoryRepository()
	expSvc := experience.NewService(expRepo)
	e1, err := expSvc.Create(ctx, experience.CreateInput{
		TenantID: "t", Type: experience.TypeProcedural, Scope: experience.ScopeTool, ScopeKey: "jira",
		Trigger: "project key", Content: "lookup project key first", Confidence: 0.9,
		Embedding: []float32{1, 0, 0, 0},
	})
	if err != nil {
		t.Fatalf("create e1: %v", err)
	}
	e2, err := expSvc.Create(ctx, experience.CreateInput{
		TenantID: "t", Type: experience.TypePreference, Scope: experience.ScopeTool, ScopeKey: "jira",
		Trigger: "priority", Content: "default priority blocker", Confidence: 0.9,
		Embedding: []float32{0, 1, 0, 0},
	})
	if err != nil {
		t.Fatalf("create e2: %v", err)
	}

	usageRepo := experience.NewMemoryUsageRepository()
	if _, err := usageRepo.Create(ctx, experience.Usage{
		ID: "u1", TenantID: "t", EpisodeID: ep.ID, ExperienceID: e1.ID, FinalScore: 10,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := usageRepo.Create(ctx, experience.Usage{
		ID: "u2", TenantID: "t", EpisodeID: ep.ID, ExperienceID: e2.ID, FinalScore: 1,
	}); err != nil {
		t.Fatal(err)
	}

	actionSvc := action.NewService(action.NewMemoryRepository(), epSvc)
	a2, err := actionSvc.RecordAction(ctx, action.RecordInput{
		TenantID: "t", EpisodeID: ep.ID, Type: action.TypeToolCall, ToolName: "jira.create_issue",
		Input: []byte(`{"priority":"blocker"}`), Status: action.StatusSuccess,
	})
	if err != nil {
		t.Fatalf("RecordAction: %v", err)
	}
	inf := 0.95
	if _, err := actionSvc.LinkExperience(ctx, action.LinkInput{
		TenantID: "t", EpisodeID: ep.ID, ActionID: a2.ID, ExperienceID: e2.ID, Influence: &inf,
		Evidence: "priority field came from e2",
	}); err != nil {
		t.Fatalf("LinkExperience: %v", err)
	}

	learnSvc, err := learning.New(usageRepo, expRepo, attribution.NewDefault())
	if err != nil {
		t.Fatalf("learning.New: %v", err)
	}
	learnSvc = learnSvc.WithActionGraph(actionSvc, actionSvc)

	before1, err := expSvc.Get(ctx, "t", e1.ID)
	if err != nil {
		t.Fatal(err)
	}
	before2, err := expSvc.Get(ctx, "t", e2.ID)
	if err != nil {
		t.Fatal(err)
	}

	updates, err := learnSvc.ApplyFeedbackReward(
		ctx, "t", ep.ID, "fb-priority", -1.0, 1.0,
		&feedback.Target{Type: feedback.TargetActionField, ActionID: a2.ID, Field: "priority"},
	)
	if err != nil {
		t.Fatalf("ApplyFeedbackReward: %v", err)
	}
	if len(updates) != 1 || updates[0].ExperienceID != e2.ID {
		t.Fatalf("updates=%#v want only %s", updates, e2.ID)
	}

	after1, _ := expSvc.Get(ctx, "t", e1.ID)
	after2, _ := expSvc.Get(ctx, "t", e2.ID)
	if after1.Utility != before1.Utility {
		t.Fatalf("e1 utility changed %v -> %v (high retrieval score should not absorb blame)", before1.Utility, after1.Utility)
	}
	if after2.Utility >= before2.Utility {
		t.Fatalf("e2 utility should drop: %v -> %v", before2.Utility, after2.Utility)
	}
}
