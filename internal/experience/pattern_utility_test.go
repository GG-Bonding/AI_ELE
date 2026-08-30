package experience_test

import (
	"context"
	"testing"
	"time"

	"github.com/agent-experience-engine/agent-experience-engine/internal/experience"
)

func TestApplyPatternBetaUpdateRaisesAndLowersUtility(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	p := experience.Pattern{
		Utility: 0.5, Alpha: 1, Beta: 1, Status: experience.PatternStatusCandidate,
	}
	up, err := experience.ApplyPatternBetaUpdate(p, 1.0, 1.0, now)
	if err != nil {
		t.Fatal(err)
	}
	if !(up.Utility > 0.5) || up.SuccessCount != 1 {
		t.Fatalf("positive update: %+v", up)
	}
	down, err := experience.ApplyPatternBetaUpdate(up, -1.0, 1.0, now)
	if err != nil {
		t.Fatal(err)
	}
	if !(down.Utility < up.Utility) || down.FailureCount != 1 {
		t.Fatalf("negative update: %+v", down)
	}
}

func TestMaybePromotePattern(t *testing.T) {
	t.Parallel()
	p := experience.Pattern{
		Status: experience.PatternStatusCandidate,
		Utility: experience.PatternPromoteMinUtility,
		SuccessCount: experience.PatternPromoteMinSuccess,
	}
	got := experience.MaybePromotePattern(p)
	if got.Status != experience.PatternStatusActive {
		t.Fatalf("status=%s", got.Status)
	}
	low := experience.MaybePromotePattern(experience.Pattern{
		Status: experience.PatternStatusCandidate, Utility: 0.5, SuccessCount: 10,
	})
	if low.Status != experience.PatternStatusCandidate {
		t.Fatalf("low utility should stay candidate, got %s", low.Status)
	}
}

func TestApplyPatternRewardHTTPPath(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	patterns := experience.NewMemoryPatternRepository()
	svc := experience.NewService(experience.NewMemoryRepository()).WithPatterns(patterns)

	alpha, beta := experience.SeedBetaFromUtility(0.7)
	created, err := patterns.Create(ctx, experience.Pattern{
		ID: "p1", TenantID: "t", Type: experience.TypeProcedural, Scope: experience.ScopeTenant,
		Trigger: "when ops", Content: "shared rule", Confidence: 0.8, Utility: 0.7,
		Alpha: alpha, Beta: beta, SupportCount: 3, Status: experience.PatternStatusCandidate,
		CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}

	updated, err := svc.ApplyPatternReward(ctx, "t", created.ID, 1.0, 1.0)
	if err != nil {
		t.Fatal(err)
	}
	if !(updated.Utility > created.Utility) {
		t.Fatalf("utility did not rise: %.3f → %.3f", created.Utility, updated.Utility)
	}
}
