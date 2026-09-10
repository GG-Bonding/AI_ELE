package skillruntime

import (
	"fmt"
	"strings"

	"github.com/agent-experience-engine/agent-experience-engine/internal/skill"
	"github.com/agent-experience-engine/agent-experience-engine/internal/toolregistry"
)

// RecoveryDecision is how crash recovery should treat the current step (V3.4.1).
type RecoveryDecision string

const (
	RecoveryRetry     RecoveryDecision = "RETRY"
	RecoveryReconcile RecoveryDecision = "RECONCILE"
	RecoveryContinue  RecoveryDecision = "CONTINUE"
	RecoveryFail      RecoveryDecision = "FAIL"   // known terminal failure → ExecFailed
	RecoveryManual    RecoveryDecision = "MANUAL" // ambiguous remote outcome → NEEDS_RECONCILIATION

	// RecoveryAbort is deprecated alias of RecoveryManual (kept for older call sites/tests).
	RecoveryAbort = RecoveryManual
)

// RecoveryPlan is the outcome of RecoveryPlanner.Plan.
type RecoveryPlan struct {
	Decision    RecoveryDecision
	NextAttempt int // for RETRY
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

	case skill.StepPending:
		// Ledger written but remote call never started — always safe to resume same attempt.
		plan.Decision = RecoveryRetry
		plan.NextAttempt = last.Attempt
		if plan.NextAttempt < 1 {
			plan.NextAttempt = 1
		}
		plan.Reason = fmt.Sprintf("PENDING attempt %d: remote never started; resume same attempt", plan.NextAttempt)
		return plan

	case skill.StepFailed:
		maxAttempts := 1
		if st.Retry != nil && st.Retry.MaxAttempts > 1 {
			maxAttempts = st.Retry.MaxAttempts
		}
		onError := "fail"
		if st.OnError != nil && strings.TrimSpace(st.OnError.Action) != "" {
			onError = strings.ToLower(strings.TrimSpace(st.OnError.Action))
		}
		next := last.Attempt + 1
		if next <= maxAttempts && (onError == "retry" || st.Retry != nil) {
			plan.Decision = RecoveryRetry
			plan.NextAttempt = next
			plan.Reason = fmt.Sprintf("last attempt failed; retry %d/%d", next, maxAttempts)
			return plan
		}
		if onError == "continue" {
			plan.Decision = RecoveryContinue
			plan.Reason = "last attempt failed with on_error=continue"
			return plan
		}
		plan.Decision = RecoveryFail
		plan.Reason = fmt.Sprintf("step failed permanently after attempt %d", last.Attempt)
		return plan

	case skill.StepUnknownOutcome, skill.StepRunning:
		opKey := last.OperationKey
		if opKey == "" {
			opKey = OperationKey(ex.ID, st.ID)
		}
		switch cap {
		case toolregistry.IdempotencyNative:
			plan.Decision = RecoveryRetry
			plan.NextAttempt = last.Attempt + 1
			if plan.NextAttempt < 2 {
				plan.NextAttempt = 2
			}
			plan.Reason = fmt.Sprintf("UNKNOWN with NATIVE idempotency; retry attempt=%d op=%s", plan.NextAttempt, opKey)
			return plan
		case toolregistry.IdempotencyQueryReconcile:
			plan.Decision = RecoveryReconcile
			plan.Reason = fmt.Sprintf("UNKNOWN requires provider reconcile op=%s", opKey)
			return plan
		default:
			plan.Decision = RecoveryManual
			plan.Reason = fmt.Sprintf("UNKNOWN on non-idempotent tool op=%s; needs human/provider reconciliation", opKey)
			return plan
		}

	default:
		plan.Decision = RecoveryManual
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
