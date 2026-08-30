package eval_test

import (
	"context"
	"testing"

	"github.com/agent-experience-engine/agent-experience-engine/internal/eval"
	"github.com/agent-experience-engine/agent-experience-engine/internal/experience"
)

func TestSequentialV2BeatsV1OnSuccessAndNegativeTransfer(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	metrics, err := eval.CompareSequentialV1V2(ctx)
	if err != nil {
		t.Fatalf("CompareSequentialV1V2: %v", err)
	}
	v1 := metrics[eval.ArmV1Utility]
	v2 := metrics[eval.ArmV2Intelligence]
	t.Logf("v1=%+v", v1)
	t.Logf("v2=%+v", v2)

	if !(v2.TaskSuccessRate > v1.TaskSuccessRate) {
		t.Fatalf("V2 success should beat V1: v2=%.3f v1=%.3f", v2.TaskSuccessRate, v1.TaskSuccessRate)
	}
	if !(v2.NegativeTransferRate < v1.NegativeTransferRate) {
		t.Fatalf("V2 negative transfer should be lower: v2=%.3f v1=%.3f",
			v2.NegativeTransferRate, v1.NegativeTransferRate)
	}
	if v1.NegativeTransferRate <= 0 {
		t.Fatalf("V1 should exhibit negative transfer from stale harmful tip: %+v", v1)
	}
	if v2.Probe3SuccessRate < 1.0 {
		t.Fatalf("V2 post-conflict probe should fully succeed after supersession: %.3f", v2.Probe3SuccessRate)
	}
}

func TestV2SupersessionDeprecatesHarmfulBeforeSequentialRun(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	eng, err := eval.NewEngineV2()
	if err != nil {
		t.Fatal(err)
	}
	if err := eng.SeedStaleHarmfulDominant(ctx); err != nil {
		t.Fatal(err)
	}
	res, err := eng.ApplyV2Supersession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if res.Kind != experience.ConflictSuperseded {
		t.Fatalf("kind=%s", res.Kind)
	}
	harmful, err := eng.Experiences.Get(ctx, "eval_tenant", eng.HarmfulID)
	if err != nil {
		t.Fatal(err)
	}
	if harmful.Status != experience.StatusDeprecated {
		t.Fatalf("harmful status=%s want DEPRECATED", harmful.Status)
	}
	helpful, err := eng.Experiences.Get(ctx, "eval_tenant", eng.HelpfulID)
	if err != nil {
		t.Fatal(err)
	}
	if !helpful.Status.Retrievable() {
		t.Fatalf("helpful should stay retrievable, status=%s", helpful.Status)
	}
}
