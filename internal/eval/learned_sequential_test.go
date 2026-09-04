package eval_test

import (
	"context"
	"testing"

	"github.com/agent-experience-engine/agent-experience-engine/internal/eval"
	"github.com/agent-experience-engine/agent-experience-engine/internal/eval/jirasim"
)

func TestLearnedPATHRecoveryAfterEnvShift(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	m, err := eval.RunLearnedPATH(ctx)
	if err != nil {
		t.Fatalf("RunLearnedPATH: %v", err)
	}
	t.Logf("learned PATH metrics: FGT=%.2f FWT=%.2f NT=%.2f recovery=%d supersede=%d E1dep=%v E2=%v probeV2=%.2f",
		m.FGT, m.FWT, m.NegativeTransfer, m.RecoveryTime, m.SupersedeCount, m.E1Deprecated, m.E2Active, m.ProbeV2Success)
	for i, ep := range m.Episodes {
		t.Logf("  [%d] phase=%s env=%s ok=%v recovered=%v nt=%v supersede=%d stored=%v tips=%d",
			i, ep.Phase, ep.Env, ep.Success, ep.Recovered, ep.NegativeTransfer, ep.Supersessions, ep.StoredIDs, len(ep.ContextTips))
	}

	if m.FGT <= 0 {
		t.Fatalf("expected positive FGT after Env V1 learning, got %.3f", m.FGT)
	}
	if m.NegativeTransfer <= 0 {
		t.Fatalf("expected Env V2 negative transfer under stale E1, got %.3f", m.NegativeTransfer)
	}
	if !m.E2Active {
		t.Fatal("expected ACTIVE E2 (must never use display name / resolve project key)")
	}
	if !m.E1Deprecated && m.SupersedeCount == 0 {
		t.Fatal("expected E1 SUPERSEDED/DEPRECATED or at least one supersession event")
	}
	if m.RecoveryTime <= 0 || m.RecoveryTime > 6 {
		t.Fatalf("recovery_time=%d want in (0,6]", m.RecoveryTime)
	}
	if m.ProbeV2Success < 1.0 {
		t.Fatalf("post-recovery probe_v2 success=%.2f want 1.0", m.ProbeV2Success)
	}
}

func TestJiraSimLenientAcceptsDisplayName(t *testing.T) {
	t.Parallel()
	sim := jirasim.New().WithMode(jirasim.ModeLenient)
	res := sim.Call("jira.create_issue", map[string]any{"project": "Payment"})
	if !res.OK {
		t.Fatalf("lenient should accept Payment: %#v", res)
	}
	sim.WithMode(jirasim.ModeStrict)
	res = sim.Call("jira.create_issue", map[string]any{"project": "Payment"})
	if res.OK || res.ErrorCode != "INVALID_PROJECT_KEY" {
		t.Fatalf("strict should reject Payment: %#v", res)
	}
}

func TestExecuteWithRecoveryAfterInvalidKey(t *testing.T) {
	t.Parallel()
	sim := jirasim.New().WithMode(jirasim.ModeStrict)
	ok, calls, _ := sim.ExecuteWithRecovery("create", []string{
		"You must use display name Payment as project; never resolve project key",
	})
	if !ok {
		t.Fatal("recovery should succeed with PAY")
	}
	if len(calls) < 3 {
		t.Fatalf("want fail+search+create, got %d calls", len(calls))
	}
}
