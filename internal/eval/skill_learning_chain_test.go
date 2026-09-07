package eval_test

import (
	"context"
	"testing"

	"github.com/agent-experience-engine/agent-experience-engine/internal/eval"
)

func TestLearningChainBeatsPatternAndEvolves(t *testing.T) {
	t.Parallel()
	m, err := eval.RunLearningChainBenchmark(context.Background())
	if err != nil {
		t.Fatalf("RunLearningChainBenchmark: %v", err)
	}
	t.Logf("%+v", m)
	if !m.ChainComplete {
		t.Fatal("chain incomplete")
	}
	if !(m.SkillV1Success > m.PatternOnlySuccess) {
		t.Fatalf("skill v1 should beat pattern-only: %#v", m)
	}
	if !(m.SkillV2Success > m.PatternOnlySuccess) {
		t.Fatalf("skill v2 should beat pattern-only: %#v", m)
	}
	if m.UtilityAfterFail >= m.UtilityAfterReward {
		t.Fatalf("failure wave should lower utility: %#v", m)
	}
	if !m.V2Promoted || !m.V1Superseded {
		t.Fatalf("v2 should promote and deprecate v1: %#v", m)
	}
	if m.UnsafeRate != 0 {
		t.Fatalf("unsafe=%v", m.UnsafeRate)
	}
}
