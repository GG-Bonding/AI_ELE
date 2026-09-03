package learning_test

import (
	"context"
	"testing"
	"time"

	"github.com/agent-experience-engine/agent-experience-engine/internal/attribution"
	"github.com/agent-experience-engine/agent-experience-engine/internal/experience"
	"github.com/agent-experience-engine/agent-experience-engine/internal/learning"
)

func TestPatternUsageDirectEpisodeReward(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	now := time.Now().UTC()

	expRepo := experience.NewMemoryRepository()
	usageRepo := experience.NewMemoryUsageRepository()
	patternUsageRepo := experience.NewMemoryPatternUsageRepository()
	patterns := experience.NewMemoryPatternRepository()
	patternEvents := learning.NewMemoryPatternEventRepository()

	p, err := patterns.Create(ctx, experience.Pattern{
		ID: "p1", TenantID: "t", Type: experience.TypeProcedural,
		Scope: experience.ScopeTool, ScopeKey: "jira",
		Trigger: "jira key", Content: "confirm project key first",
		Confidence: 0.9, Utility: 0.5, Alpha: 1, Beta: 1,
		Status: experience.PatternStatusActive, CreatedAt: now, UpdatedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	before := p.Utility

	learnSvc, err := learning.New(usageRepo, expRepo, attribution.NewDefault())
	if err != nil {
		t.Fatal(err)
	}
	learnSvc = learnSvc.WithPatterns(patterns).
		WithPatternUsages(patternUsageRepo).
		WithPatternLearning(patternEvents, learning.NewMemoryPatternEventApplier(patterns, patternEvents))

	if _, err := patternUsageRepo.Create(ctx, experience.PatternUsage{
		ID: "pu1", TenantID: "t", EpisodeID: "ep1", PatternID: p.ID,
		RetrievalScore: 0.4, FinalScore: 0.8, UsedAt: now,
	}); err != nil {
		t.Fatal(err)
	}

	updates, err := learnSvc.ApplyFeedbackReward(ctx, "t", "ep1", "fb-pat", 1.0, 1.0, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(updates) != 0 {
		t.Fatalf("expected no experience updates, got %#v", updates)
	}

	got, err := patterns.Get(ctx, "t", p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !(got.Utility > before) {
		t.Fatalf("pattern utility should rise via PatternUsage: %.3f -> %.3f", before, got.Utility)
	}

	rows, err := patternEvents.ListByFeedback(ctx, "t", "fb-pat")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].SourceType != learning.PatternSourceUsage || rows[0].Status != learning.EventApplied {
		t.Fatalf("pattern events=%#v", rows)
	}

	mid := got.Utility
	_, err = learnSvc.ApplyFeedbackReward(ctx, "t", "ep1", "fb-pat", 1.0, 1.0, nil)
	if err != nil {
		t.Fatal(err)
	}
	again, _ := patterns.Get(ctx, "t", p.ID)
	if again.Utility != mid {
		t.Fatalf("replay should not re-apply pattern usage reward: %.3f -> %.3f", mid, again.Utility)
	}
}

func TestMemberPatternLearningEventExactlyOnce(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	expRepo := experience.NewMemoryRepository()
	usageRepo := experience.NewMemoryUsageRepository()
	patterns := experience.NewMemoryPatternRepository()
	patternEvents := learning.NewMemoryPatternEventRepository()
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
		t.Fatalf("generalize: %#v err=%v", gen, err)
	}
	before := gen.Pattern.Utility

	learnSvc, err := learning.New(usageRepo, expRepo, attribution.NewDefault())
	if err != nil {
		t.Fatal(err)
	}
	learnSvc = learnSvc.WithPatterns(patterns).
		WithPatternLearning(patternEvents, learning.NewMemoryPatternEventApplier(patterns, patternEvents))

	if _, err := usageRepo.Create(ctx, experience.Usage{
		ID: "u1", TenantID: "t", EpisodeID: "ep-use", ExperienceID: ids[0], FinalScore: 1,
	}); err != nil {
		t.Fatal(err)
	}

	if _, err := learnSvc.ApplyFeedbackReward(ctx, "t", "ep-use", "fb-member", 1.0, 1.0, nil); err != nil {
		t.Fatal(err)
	}
	pat, err := patterns.Get(ctx, "t", gen.Pattern.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !(pat.Utility > before) {
		t.Fatalf("utility %.3f -> %.3f", before, pat.Utility)
	}
	mid := pat.Utility

	events, err := patternEvents.ListByFeedback(ctx, "t", "fb-member")
	if err != nil {
		t.Fatal(err)
	}
	if len(events) == 0 {
		t.Fatal("expected member pattern learning events")
	}
	for _, ev := range events {
		if ev.SourceType != learning.PatternSourceMember || ev.Status != learning.EventApplied {
			t.Fatalf("%#v", ev)
		}
	}

	if _, err := learnSvc.ApplyFeedbackReward(ctx, "t", "ep-use", "fb-member", 1.0, 1.0, nil); err != nil {
		t.Fatal(err)
	}
	again, _ := patterns.Get(ctx, "t", gen.Pattern.ID)
	if again.Utility != mid {
		t.Fatalf("replay double-applied pattern: %.3f -> %.3f", mid, again.Utility)
	}
}

func TestDirectPatternRewardIdempotent(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	expRepo := experience.NewMemoryRepository()
	usageRepo := experience.NewMemoryUsageRepository()
	patterns := experience.NewMemoryPatternRepository()
	patternEvents := learning.NewMemoryPatternEventRepository()

	p, err := patterns.Create(ctx, experience.Pattern{
		ID: "p-direct", TenantID: "t", Type: experience.TypeProcedural,
		Scope: experience.ScopeTool, ScopeKey: "jira",
		Trigger: "x", Content: "y", Confidence: 0.9, Utility: 0.5, Alpha: 1, Beta: 1,
		Status: experience.PatternStatusActive, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	learnSvc, err := learning.New(usageRepo, expRepo, attribution.NewDefault())
	if err != nil {
		t.Fatal(err)
	}
	learnSvc = learnSvc.WithPatterns(patterns).
		WithPatternLearning(patternEvents, learning.NewMemoryPatternEventApplier(patterns, patternEvents))

	first, err := learnSvc.ApplyDirectPatternReward(ctx, "t", p.ID, "idem-1", 1.0, 1.0)
	if err != nil {
		t.Fatal(err)
	}
	second, err := learnSvc.ApplyDirectPatternReward(ctx, "t", p.ID, "idem-1", 1.0, 1.0)
	if err != nil {
		t.Fatal(err)
	}
	if first.Utility != second.Utility {
		t.Fatalf("idempotent direct reward drifted: %.3f vs %.3f", first.Utility, second.Utility)
	}
}

func TestRecordUsagesWritesPatternLedger(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	expRepo := experience.NewMemoryRepository()
	usageRepo := experience.NewMemoryUsageRepository()
	patternUsageRepo := experience.NewMemoryPatternUsageRepository()

	learnSvc, err := learning.New(usageRepo, expRepo, attribution.NewDefault())
	if err != nil {
		t.Fatal(err)
	}
	learnSvc = learnSvc.WithPatternUsages(patternUsageRepo)

	_, err = learnSvc.RecordUsages(ctx, learning.RecordInput{
		TenantID:  "t",
		EpisodeID: "ep1",
		Patterns: []learning.PatternRecord{
			{PatternID: "p1", RetrievalScore: 0.5, FinalScore: 0.7},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	rows, err := patternUsageRepo.ListByEpisode(ctx, "t", "ep1")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].PatternID != "p1" || rows[0].FinalScore != 0.7 {
		t.Fatalf("%#v", rows)
	}
}
