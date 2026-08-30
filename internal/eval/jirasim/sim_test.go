package jirasim_test

import (
	"testing"

	"github.com/agent-experience-engine/agent-experience-engine/internal/eval/jirasim"
)

func TestSimulatorRejectsDisplayName(t *testing.T) {
	t.Parallel()
	sim := jirasim.New()
	res := sim.Call("jira.create_issue", map[string]any{"project": "Payment"})
	if res.OK || res.ErrorCode != "INVALID_PROJECT_KEY" {
		t.Fatalf("%#v", res)
	}
}

func TestPolicyPlusSimulatorDecidesOutcome(t *testing.T) {
	t.Parallel()
	sim := jirasim.New()
	policy := jirasim.AgentPolicy{}

	ok, _ := sim.Run(policy.Plan("create jira", nil))
	if ok {
		t.Fatal("baseline should fail")
	}
	ok, _ = sim.Run(policy.Plan("create jira", []string{"Resolve project key before create_issue"}))
	if !ok {
		t.Fatal("helpful context should succeed via simulator")
	}
	ok, _ = sim.Run(policy.Plan("create jira", []string{"Use display name Payment as project key"}))
	if ok {
		t.Fatal("harmful context should fail via simulator")
	}
	// First tip wins: harmful before helpful → fail (raw retrieval case).
	ok, _ = sim.Run(policy.Plan("create jira", []string{
		"Use display name Payment as project key",
		"Resolve project key before create_issue",
	}))
	if ok {
		t.Fatal("top harmful tip should fail even if helpful follows")
	}
	ok, _ = sim.Run(policy.Plan("create jira", []string{
		"Resolve project key before create_issue",
		"Use display name Payment as project key",
	}))
	if !ok {
		t.Fatal("top helpful tip should succeed even if harmful follows")
	}
}
