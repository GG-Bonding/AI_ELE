package skillruntime

import (
	"fmt"
	"strings"

	"github.com/agent-experience-engine/agent-experience-engine/internal/skill"
	"github.com/agent-experience-engine/agent-experience-engine/internal/toolregistry"
)

// RecoveryDecision is how crash recovery should treat the current step (V3.4 closeout).
type RecoveryDecision string

const (
	RecoveryRetry     RecoveryDecision = "RETRY"
	RecoveryReconcile RecoveryDecision = "RECONCILE"
	RecoveryContinue  RecoveryDecision = "CONTINUE"
	RecoveryAbort     RecoveryDecision = "ABORT"
)

// RecoveryPlan is the outcome of RecoveryPlanner.Plan.
type RecoveryPlan struct {
	Decision    RecoveryDecision
	NextAttempt int    // for RETRY
	Reason      string
	LastStep    skill.StepExecution
	Tool        string
	Capability  toolregistry.IdempotencyCapability
}

// RecoveryPlanner decides retry vs reconcile from last attempt + tool capability.
type RecoveryPlanner struct {
	Tools *toolregistry.Registry
}

// Plan inspects steps for the current step cursor.
func (p RecoveryPlanner) Plan(ex skill.Execution, spec skill.Spec, steps []skill.StepExecution) RecoveryPlan {
	tools := p.Tools
	if tools == nil {
		tools = toolregistry.Default()
	}
	cursor := ex.StepCursor
	if cursor < 0 {
		cursor = 0
	}
	if cursor >= len(spec.Steps) {
		return RecoveryPlan{Decision: RecoveryContinue, Reason: "cursor past last step"}
	}
	st := spec.Steps[cursor]
	last := latestAttemptForStep(steps, st.ID)
	if last.ID == "" {
		// No attempt yet for this cursor — safe to start attempt 1.
		return RecoveryPlan{Decision: RecoveryRetry, NextAttempt: 1, Reason: "no prior attempt", Tool: st.Tool}
	}

	cap := toolregistry.IdempotencyNone
	if def, ok := tools.Get(st.Tool); ok {
		cap = def.IdempotencyCapability
		if cap == "" {
			if def.Idempotent {
				cap = toolregistry.IdempotencyNative
			} else {
				cap = toolregistry.IdempotencyNone
			}
		}
	}

	plan := RecoveryPlan{LastStep: last, Tool: st.Tool, Capability: cap}
	switch last.Status {
	case skill.StepSucceeded, skill.StepShadowed, skill.StepSkipped:
		plan.Decision = RecoveryContinue
		plan.Reason = fmt.Sprintf("last attempt %d already %s", last.Attempt, last.Status)
		return plan
	case skill.StepFailed:
		maxAttempts := 1
		if st.Retry != nil && st.Retry.MaxAttempts > 1 {
			maxAttempts = st.Retry.MaxAttempts
		}
		next := last.Attempt + 1
		if next <= maxAttempts {
			plan.Decision = RecoveryRetry
			plan.NextAttempt = next
			plan.Reason = fmt.Sprintf("last attempt failed; retry %d/%d", next, maxAttempts)
			return plan
		}
		plan.Decision = RecoveryAbort
		plan.Reason = fmt.Sprintf("retries exhausted after attempt %d", last.Attempt)
		return plan
	case skill.StepUnknownOutcome, skill.StepRunning, skill.StepPending:
		switch cap {
		case toolregistry.IdempotencyNative:
			plan.Decision = RecoveryRetry
			plan.NextAttempt = last.Attempt + 1
			if plan.NextAttempt < 2 {
				plan.NextAttempt = 2
			}
			plan.Reason = "UNKNOWN with NATIVE idempotency; safe same operation_key retry"
			return plan
		case toolregistry.IdempotencyQueryReconcile:
			plan.Decision = RecoveryReconcile
			plan.Reason = "UNKNOWN requires provider reconcile"
			return plan
		default:
			plan.Decision = RecoveryAbort
			plan.Reason = "UNKNOWN on non-idempotent tool; needs human/provider reconciliation"
			return plan
		}
	default:
		plan.Decision = RecoveryAbort
		plan.Reason = "unrecognized last status: " + string(last.Status)
		return plan
	}
}

func latestAttemptForStep(steps []skill.StepExecution, stepID string) skill.StepExecution {
	stepID = strings.TrimSpace(stepID)
	var best skill.StepExecution
	for _, st := range steps {
		if st.StepID != stepID {
			continue
		}
		if best.ID == "" || st.Attempt > best.Attempt || (st.Attempt == best.Attempt && st.Sequence >= best.Sequence) {
			best = st
		}
	}
	return best
}
