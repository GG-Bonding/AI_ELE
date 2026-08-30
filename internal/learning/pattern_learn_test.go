package learning_test

import (
	"context"
	"testing"
	"time"

	"github.com/agent-experience-engine/agent-experience-engine/internal/attribution"
	"github.com/agent-experience-engine/agent-experience-engine/internal/experience"
	"github.com/agent-experience-engine/agent-experience-engine/internal/learning"
)

func TestFeedbackPropagatesToPatternUtility(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	expRepo := experience.NewMemoryRepository()
	usageRepo := experience.NewMemoryUsageRepository()
	patterns := experience.NewMemoryPatternRepository()
	expSvc := experience.NewService(expRepo).WithPatterns(patterns)

	ids := make([]string, 0, 3)
	for _, ep := range []string{"ep1", "ep2", "ep3"} {
		exp, err := expSvc.Create(ctx, experience.CreateInput{
			TenantID: "t", Type: experience.TypeProcedural, Scope: experience.ScopeTool, ScopeKey: "jira",
			Trigger: "create jira issue", Content: "Resolve project key first.",
			SourceEpisodeID: ep, Confidence: 0.9, Embedding: []float32{1, 0, 0, 0},
			Evidence: experience.Evidence{SourceEpisodeID: ep, SupportEpisodeIDs: []string{ep}},
			Status:   experience.StatusActive,
		})
		if err != nil {
			t.Fatal(err)
		}
		exp.Utility = 0.8
		updated, err := expRepo.Update(ctx, exp)
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, updated.ID)
	}

	gen, err := expSvc.Generalize(ctx, "t", experience.GeneralizeInput{ExperienceIDs: ids})
	if err != nil || !gen.Created {
		t.Fatalf("generalize: created=%v skipped=%q err=%v", gen.Created, gen.Skipped, err)
	}
	before := gen.Pattern.Utility

	learnSvc, err := learning.New(usageRepo, expRepo, attribution.NewDefault())
	if err != nil {
		t.Fatal(err)
	}
	learnSvc = learnSvc.WithPatterns(patterns)

	if _, err := usageRepo.Create(ctx, experience.Usage{
		ID: "u1", TenantID: "t", EpisodeID: "ep-use", ExperienceID: ids[0], FinalScore: 1,
	}); err != nil {
		t.Fatal(err)
	}

	updates, err := learnSvc.ApplyFeedbackReward(ctx, "t", "ep-use", "fb1", 1.0, 1.0, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(updates) == 0 {
		t.Fatal("expected experience utility update")
	}

	pat, err := patterns.Get(ctx, "t", gen.Pattern.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !(pat.Utility > before) {
		t.Fatalf("pattern utility should rise: before=%.3f after=%.3f", before, pat.Utility)
	}
	if pat.SuccessCount < 1 {
		t.Fatalf("success_count=%d", pat.SuccessCount)
	}

	mid := pat.Utility
	_, err = learnSvc.ApplyFeedbackReward(ctx, "t", "ep-use", "fb1", 1.0, 1.0, nil)
	if err != nil {
		t.Fatal(err)
	}
	again, err := patterns.Get(ctx, "t", gen.Pattern.ID)
	if err != nil {
		t.Fatal(err)
	}
	if again.Utility != mid {
		t.Fatalf("replay changed pattern utility: %.3f → %.3f", mid, again.Utility)
	}
}

func TestPatternPromotesAfterEnoughSuccess(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	expRepo := experience.NewMemoryRepository()
	usageRepo := experience.NewMemoryUsageRepository()
	patterns := experience.NewMemoryPatternRepository()

	alpha, beta := experience.SeedBetaFromUtility(0.74)
	p, err := patterns.Create(ctx, experience.Pattern{
		ID: "pat", TenantID: "t", Type: experience.TypeProcedural, Scope: experience.ScopeTenant,
		Trigger: "ops", Content: "rule", Confidence: 0.9, Utility: 0.74,
		Alpha: alpha, Beta: beta, SuccessCount: 1, SupportCount: 3,
		Status:    experience.PatternStatusCandidate,
		CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	exp, err := experience.NewService(expRepo).Create(ctx, experience.CreateInput{
		TenantID: "t", Type: experience.TypeProcedural, Scope: experience.ScopeTenant,
		Trigger: "ops", Content: "rule", SourceEpisodeID: "e1", Confidence: 0.9,
		Embedding: []float32{1, 0, 0, 0},
		Evidence:  experience.Evidence{SourceEpisodeID: "e1", SupportEpisodeIDs: []string{"e1"}},
		Status:    experience.StatusActive,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := patterns.AddEvidence(ctx, experience.PatternEvidence{
		PatternID: p.ID, ExperienceID: exp.ID, CreatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}

	learnSvc, err := learning.New(usageRepo, expRepo, attribution.NewDefault())
	if err != nil {
		t.Fatal(err)
	}
	learnSvc = learnSvc.WithPatterns(patterns)
	if _, err := usageRepo.Create(ctx, experience.Usage{
		ID: "u1", TenantID: "t", EpisodeID: "ep", ExperienceID: exp.ID, FinalScore: 1,
	}); err != nil {
		t.Fatal(err)
	}
	_, err = learnSvc.ApplyFeedbackReward(ctx, "t", "ep", "fb", 1.0, 1.0, nil)
	if err != nil {
		t.Fatal(err)
	}
	got, err := patterns.Get(ctx, "t", p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != experience.PatternStatusActive {
		t.Fatalf("expected ACTIVE after proven successes, got %s util=%.3f success=%d",
			got.Status, got.Utility, got.SuccessCount)
	}
}
