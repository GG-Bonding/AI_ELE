package eval_test

import (
	"testing"

	"github.com/agent-experience-engine/agent-experience-engine/internal/eval/ghsim"
	"github.com/agent-experience-engine/agent-experience-engine/internal/eval/jirasim"
)

// V1-20: success labels come from tool simulators across >1 domain (not keyword checks).
func TestMultiDomainSimulatorsDecideSuccess(t *testing.T) {
	t.Parallel()

	jiraOK, _ := jirasim.New().Run(jirasim.AgentPolicy{}.Plan("create jira", []string{
		"Resolve project key before create_issue",
	}))
	if !jiraOK {
		t.Fatal("jira helpful path should succeed via simulator")
	}
	jiraBad, _ := jirasim.New().Run(jirasim.AgentPolicy{}.Plan("create jira", nil))
	if jiraBad {
		t.Fatal("jira baseline should fail via simulator")
	}

	ghOK, _ := ghsim.New().Run(ghsim.AgentPolicy{}.Plan("merge pr", []string{
		"Use numeric PR number before merge",
	}))
	if !ghOK {
		t.Fatal("github helpful path should succeed via simulator")
	}
	ghBad, _ := ghsim.New().Run(ghsim.AgentPolicy{}.Plan("merge pr", nil))
	if ghBad {
		t.Fatal("github baseline should fail via simulator")
	}
}
