package ghsim_test

import (
	"testing"

	"github.com/agent-experience-engine/agent-experience-engine/internal/eval/ghsim"
)

func TestGitHubPolicyPlusSimulator(t *testing.T) {
	t.Parallel()
	sim := ghsim.New()
	policy := ghsim.AgentPolicy{}

	ok, _ := sim.Run(policy.Plan("merge pr", nil))
	if ok {
		t.Fatal("baseline should fail")
	}
	ok, _ = sim.Run(policy.Plan("merge pr", []string{"Use numeric PR number before merge"}))
	if !ok {
		t.Fatal("helpful tip should succeed")
	}
	ok, _ = sim.Run(policy.Plan("merge pr", []string{"Use title Payment as number"}))
	if ok {
		t.Fatal("harmful tip should fail")
	}
}
