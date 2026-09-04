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
	// E1-style "must use display name ... never resolve" must not match the resolve tip.
	ok, _ = sim.Run(policy.Plan("create jira", []string{
		"You must use display name Payment as project; never resolve project key before create_issue",
	}))
	if ok {
		t.Fatal("E1 display-name mandate should fail under strict simulator")
	}
}

func TestAgentPolicyPlanContents(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		content string
		wantOK  bool
		mode    jirasim.Mode
	}{
		{
			name:    "jira_loop_helpful",
			content: "Resolve project key via jira.search_projects before create_issue; INVALID_PROJECT_KEY means use PAY not Payment display name",
			wantOK:  true,
			mode:    jirasim.ModeStrict,
		},
		{
			name:    "e1_lenient",
			content: "You must always use display name Payment as the project field for create_issue",
			wantOK:  true,
			mode:    jirasim.ModeLenient,
		},
		{
			name:    "e1_strict_fails",
			content: "You must always use display name Payment as the project field for create_issue",
			wantOK:  false,
			mode:    jirasim.ModeStrict,
		},
		{
			name:    "e2",
			content: "You must never use display name Payment as the project field; use project key PAY after jira.search_projects",
			wantOK:  true,
			mode:    jirasim.ModeStrict,
		},
		{
			name:    "harmful_display",
			content: "Use display name Payment as project key",
			wantOK:  false,
			mode:    jirasim.ModeStrict,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			calls := jirasim.AgentPolicy{}.Plan("Create a Jira issue for payment timeout", []string{tc.content})
			ok, steps := jirasim.New().WithMode(tc.mode).Run(calls)
			t.Logf("calls=%#v ok=%v steps=%#v", calls, ok, steps)
			if ok != tc.wantOK {
				t.Fatalf("ok=%v want %v", ok, tc.wantOK)
			}
		})
	}
}
