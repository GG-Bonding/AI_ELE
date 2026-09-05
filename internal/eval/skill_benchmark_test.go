package eval_test

import (
	"context"
	"testing"

	"github.com/agent-experience-engine/agent-experience-engine/internal/eval"
)

func TestSkillBenchmarkBeatsPatternOnly(t *testing.T) {
	t.Parallel()
	m, err := eval.RunSkillBenchmark(context.Background())
	if err != nil {
		t.Fatalf("RunSkillBenchmark: %v", err)
	}
	t.Logf("skill bench: %+v", m)
	if !m.Activated || !m.ShadowOK {
		t.Fatalf("expected shadow+activate path, got %+v", m)
	}
	if !(m.SkillSuccess > m.PatternOnlySuccess) {
		t.Fatalf("skill success %.2f should beat pattern-only %.2f", m.SkillSuccess, m.PatternOnlySuccess)
	}
	if m.SkillSuccess < 1.0 {
		t.Fatalf("skill live success=%.2f want 1.0", m.SkillSuccess)
	}
	if m.UnsafeSkillRate > 0 {
		t.Fatalf("unsafe rate %.2f want 0", m.UnsafeSkillRate)
	}
	if m.SkillSelectionHit <= 0 {
		t.Fatal("expected skill retrieval hit")
	}
}
