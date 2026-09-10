package skillruntime

import (
	"testing"

	"github.com/agent-experience-engine/agent-experience-engine/internal/skill"
	"github.com/agent-experience-engine/agent-experience-engine/internal/toolregistry"
)

func TestRecoveryPlannerNativeUnknownRetriesNextAttempt(t *testing.T) {
	t.Parallel()
	tools := toolregistry.Default()
	def, _ := tools.Get("jira.search_projects")
	def.IdempotencyCapability = toolregistry.IdempotencyNative
	_ = tools.Register(def)

	spec := skill.Spec{Steps: []skill.SkillStep{{ID: "s1", Tool: "jira.search_projects"}}}
	ex := skill.Execution{StepCursor: 0}
	steps := []skill.StepExecution{{
		ID: "a1", StepID: "s1", Tool: "jira.search_projects",
		Status: skill.StepUnknownOutcome, Attempt: 1, OperationKey: "ex:s1",
	}}
	plan := RecoveryPlanner{Tools: tools}.Plan(ex, spec, steps)
	if plan.Decision != RecoveryRetry || plan.NextAttempt != 2 {
		t.Fatalf("plan=%+v", plan)
	}
}

func TestRecoveryPlannerNoneUnknownAborts(t *testing.T) {
	t.Parallel()
	tools := toolregistry.Default()
	spec := skill.Spec{Steps: []skill.SkillStep{{ID: "refund", Tool: "jira.create_issue"}}}
	ex := skill.Execution{StepCursor: 0}
	steps := []skill.StepExecution{{
		ID: "a1", StepID: "refund", Tool: "jira.create_issue",
		Status: skill.StepUnknownOutcome, Attempt: 1, OperationKey: "ex:refund",
	}}
	plan := RecoveryPlanner{Tools: tools}.Plan(ex, spec, steps)
	if plan.Decision != RecoveryAbort {
		t.Fatalf("want ABORT got %+v", plan)
	}
}

func TestRecoveryPlannerSucceededContinues(t *testing.T) {
	t.Parallel()
	spec := skill.Spec{Steps: []skill.SkillStep{{ID: "s1", Tool: "jira.search_projects"}}}
	ex := skill.Execution{StepCursor: 0}
	steps := []skill.StepExecution{{
		ID: "a1", StepID: "s1", Status: skill.StepSucceeded, Attempt: 1,
	}}
	plan := RecoveryPlanner{Tools: toolregistry.Default()}.Plan(ex, spec, steps)
	if plan.Decision != RecoveryContinue {
		t.Fatalf("want CONTINUE got %+v", plan)
	}
}

func TestRecoveryPlannerFailedRespectsRetryPolicy(t *testing.T) {
	t.Parallel()
	spec := skill.Spec{Steps: []skill.SkillStep{{
		ID: "s1", Tool: "jira.search_projects",
		Retry: &skill.RetryPolicy{MaxAttempts: 2},
	}}}
	ex := skill.Execution{StepCursor: 0}
	steps := []skill.StepExecution{{
		ID: "a1", StepID: "s1", Status: skill.StepFailed, Attempt: 2,
	}}
	plan := RecoveryPlanner{Tools: toolregistry.Default()}.Plan(ex, spec, steps)
	if plan.Decision != RecoveryAbort {
		t.Fatalf("want ABORT after exhausted retries, got %+v", plan)
	}
}
