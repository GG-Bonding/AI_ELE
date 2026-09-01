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
	e3, err := expSvc.Create(ctx, experience.CreateInput{
		TenantID: "t", Type: experience.TypeProcedural, Scope: experience.ScopeTool, ScopeKey: "jira",
		Trigger: "description", Content: "copy ticket description", Confidence: 0.9,
		Embedding: []float32{0, 0, 1, 0},
	})
	if err != nil {
		t.Fatalf("create e3: %v", err)
	}

	usageRepo := experience.NewMemoryUsageRepository()
	usages := []struct {
		id    string
		expID string
		score float64
	}{
		{"u1", e1.ID, 10},
		{"u2", e2.ID, 1},
		{"u3", e3.ID, 1},
	}
	for _, u := range usages {
		if _, err := usageRepo.Create(ctx, experience.Usage{
			ID: u.id, TenantID: "t", EpisodeID: ep.ID, ExperienceID: u.expID, FinalScore: u.score,
		}); err != nil {
			t.Fatal(err)
		}
	}

	actionSvc := action.NewService(action.NewMemoryRepository(), epSvc)
	a2, err := actionSvc.RecordAction(ctx, action.RecordInput{
		TenantID: "t", EpisodeID: ep.ID, Type: action.TypeToolCall, ToolName: "jira.create_issue",
		Input: []byte(`{"project":"X","priority":"blocker","description":"d"}`), Status: action.StatusSuccess,
	})
	if err != nil {
		t.Fatalf("RecordAction: %v", err)
	}
	inf := 1.0
	links := []struct {
		expID  string
		fields []string
	}{
		{e1.ID, []string{"input.project"}},
		{e2.ID, []string{"input.priority"}},
		{e3.ID, []string{"input.description"}},
	}
	for _, l := range links {
		if _, err := actionSvc.LinkExperience(ctx, action.LinkInput{
			TenantID: "t", EpisodeID: ep.ID, ActionID: a2.ID, ExperienceID: l.expID, Influence: &inf,
			AffectedFields: l.fields,
		}); err != nil {
			t.Fatalf("LinkExperience %s: %v", l.expID, err)
		}
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
	before3, err := expSvc.Get(ctx, "t", e3.ID)
	if err != nil {
		t.Fatal(err)
	}

	updates, err := learnSvc.ApplyFeedbackReward(
		ctx, "t", ep.ID, "fb-priority", -1.0, 1.0,
		&feedback.Target{Type: feedback.TargetActionField, ActionID: a2.ID, Field: "input.priority"},
	)
	if err != nil {
		t.Fatalf("ApplyFeedbackReward: %v", err)
	}
	if len(updates) != 1 || updates[0].ExperienceID != e2.ID {
		t.Fatalf("updates=%#v want only %s", updates, e2.ID)
	}

	after1, _ := expSvc.Get(ctx, "t", e1.ID)
	after2, _ := expSvc.Get(ctx, "t", e2.ID)
	after3, _ := expSvc.Get(ctx, "t", e3.ID)
	if after1.Utility != before1.Utility {
		t.Fatalf("e1 utility changed %v -> %v", before1.Utility, after1.Utility)
	}
	if after3.Utility != before3.Utility {
		t.Fatalf("e3 utility changed %v -> %v", before3.Utility, after3.Utility)
	}
	if after2.Utility >= before2.Utility {
		t.Fatalf("e2 utility should drop: %v -> %v", before2.Utility, after2.Utility)
	}
}
