package experience_test

import (
	"testing"
	"time"

	"github.com/agent-experience-engine/agent-experience-engine/internal/experience"
)

func TestApplyBetaUpdatePositiveRaisesUtility(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	exp := experience.Experience{Alpha: 1, Beta: 1, Utility: 0.5}
	updated, err := experience.ApplyBetaUpdate(exp, 1.0, 1.0, now)
	if err != nil {
		t.Fatalf("ApplyBetaUpdate: %v", err)
	}
	if updated.Utility <= 0.5 || updated.Alpha <= 1 || updated.SuccessCount != 1 {
		t.Fatalf("%#v", updated)
	}
}

func TestApplyBetaUpdateNegativeLowersUtility(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	exp := experience.Experience{Alpha: 1, Beta: 1, Utility: 0.5}
	updated, err := experience.ApplyBetaUpdate(exp, -1.0, 1.0, now)
	if err != nil {
		t.Fatalf("ApplyBetaUpdate: %v", err)
	}
	if updated.Utility >= 0.5 || updated.Beta <= 1 || updated.FailureCount != 1 {
		t.Fatalf("%#v", updated)
	}
}
