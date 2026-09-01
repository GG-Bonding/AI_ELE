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
		WithPatternRewardClaims(experience.NewMemoryPatternRewardClaimRepository())

	if _, err := patternUsageRepo.Create(ctx, experience.PatternUsage{
		ID: "pu1", TenantID: "t", EpisodeID: "ep1", PatternID: p.ID,
		RetrievalScore: 0.4, FinalScore: 0.8, UsedAt: now,
	}); err != nil {
		t.Fatal(err)
	}

	// No experience usages — pattern-only context still learns.
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
