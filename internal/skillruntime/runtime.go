package skillruntime

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/agent-experience-engine/agent-experience-engine/internal/skill"
	"github.com/agent-experience-engine/agent-experience-engine/internal/toolregistry"
	"github.com/google/uuid"
)

// Runtime executes Skill Specs in SHADOW or LIVE mode behind a policy gate.
type Runtime struct {
	Tools  *toolregistry.Registry
	Exec   ToolExecutor
	Policy ExecutionPolicy
	Store  skill.ExecutionStore // can be memory
	IDs    func() string
	Now    func() time.Time
}

// RunRequest is one Skill execution request.
type RunRequest struct {
	TenantID        string
	EpisodeID       string
	SkillID         string
	SkillVersionID  string
	Mode            skill.ExecutionMode
	Spec            skill.Spec
	Inputs          map[string]any
	IdempotencyKey  string
	AvailableTools  []string
	RuntimeEnabled  bool
	ApprovalGranted bool // if resuming after approval
}

// Run executes a Skill under policy, budget, and template resolution.
func (r *Runtime) Run(ctx context.Context, req RunRequest) (skill.Execution, []skill.StepExecution, error) {
	if r == nil || r.Store == nil || r.Exec == nil {
		return skill.Execution{}, nil, fmt.Errorf("skillruntime: Runtime requires Store and Exec")
	}
	policy := r.Policy
	if policy == nil {
		policy = DefaultPolicy{}
	}
	ids := r.IDs
	if ids == nil {
		ids = func() string { return uuid.NewString() }
	}
	nowFn := r.Now
	if nowFn == nil {
		nowFn = func() time.Time { return time.Now().UTC() }
	}
	tools := r.Tools
	if tools == nil {
		tools = toolregistry.Default()
	}

	if req.IdempotencyKey != "" {
		existing, err := r.Store.GetExecutionByIdempotency(ctx, req.TenantID, req.IdempotencyKey)
		if err == nil {
			steps, listErr := r.Store.ListSteps(ctx, req.TenantID, existing.ID)
			if listErr != nil {
				return existing, nil, listErr
			}
			return existing, steps, nil
		}
		if !errors.Is(err, skill.ErrNotFound) {
			return skill.Execution{}, nil, err
		}
	}

	specTools := make([]string, 0, len(req.Spec.Steps))
	seen := map[string]struct{}{}
	for _, st := range req.Spec.Steps {
		if _, ok := seen[st.Tool]; ok {
			continue
		}
		seen[st.Tool] = struct{}{}
		specTools = append(specTools, st.Tool)
	}

	decision, reason, err := policy.Decide(ctx, PolicyRequest{
		Mode:             req.Mode,
		Risk:             req.Spec.Risk.Level,
		RequiresApproval: req.Spec.Risk.RequiresApproval,
		RuntimeEnabled:   req.RuntimeEnabled,
		AvailableTools:   req.AvailableTools,
		SpecTools:        specTools,
	})
	if err != nil {
		return skill.Execution{}, nil, err
	}

	started := nowFn()
	ex := skill.Execution{
		ID:             ids(),
		TenantID:       req.TenantID,
		EpisodeID:      req.EpisodeID,
		SkillID:        req.SkillID,
		SkillVersionID: req.SkillVersionID,
		Mode:           req.Mode,
		Status:         skill.ExecPending,
		IdempotencyKey: req.IdempotencyKey,
		Inputs:         req.Inputs,
		StartedAt:      started,
	}
	ex, err = r.Store.CreateExecution(ctx, ex)
	if err != nil {
		return skill.Execution{}, nil, err
	}

	switch decision {
	case DecisionDeny:
		ex.Status = skill.ExecDenied
		ex.ErrorCode = "POLICY_DENIED"
		ex.ErrorMessage = reason
		completed := nowFn()
		ex.CompletedAt = &completed
		ex, err = r.Store.UpdateExecution(ctx, ex)
		return ex, nil, err
	case DecisionRequireApproval:
		if !req.ApprovalGranted {
			ex.Status = skill.ExecWaitingApproval
			ex.ErrorCode = "REQUIRES_APPROVAL"
			ex.ErrorMessage = reason
			ex, err = r.Store.UpdateExecution(ctx, ex)
			if err != nil {
				return ex, nil, err
			}
			_, _ = r.Store.CreateApproval(ctx, skill.ApprovalRequest{
				ID:          ids(),
				TenantID:    req.TenantID,
				ExecutionID: ex.ID,
				SkillID:     req.SkillID,
				Status:      skill.ApprovalPending,
				Reason:      reason,
				CreatedAt:   nowFn(),
			})
			return ex, nil, nil
		}
	case DecisionShadowOnly:
		if req.Mode == skill.ModeLive {
			ex.Status = skill.ExecDenied
			ex.ErrorCode = "SHADOW_ONLY"
			ex.ErrorMessage = reason
			if reason == "" {
				ex.ErrorMessage = "live execution not permitted; shadow only"
			}
			completed := nowFn()
			ex.CompletedAt = &completed
			ex, err = r.Store.UpdateExecution(ctx, ex)
			return ex, nil, err
		}
	case DecisionAllow:
		// continue
	default:
		ex.Status = skill.ExecDenied
		ex.ErrorCode = "UNKNOWN_DECISION"
		ex.ErrorMessage = string(decision)
		completed := nowFn()
		ex.CompletedAt = &completed
		ex, err = r.Store.UpdateExecution(ctx, ex)
		return ex, nil, err
	}

	ex.Status = skill.ExecRunning
	ex, err = r.Store.UpdateExecution(ctx, ex)
	if err != nil {
		return ex, nil, err
	}

	deadline := time.Time{}
	if req.Spec.TimeoutMs > 0 {
		deadline = started.Add(time.Duration(req.Spec.TimeoutMs) * time.Millisecond)
	}
	maxSteps := req.Spec.MaxSteps
	if maxSteps <= 0 {
		maxSteps = len(req.Spec.Steps)
	}
	if maxSteps > len(req.Spec.Steps) {
		maxSteps = len(req.Spec.Steps)
	}

	bindings := map[string]any{
		"inputs": cloneMap(req.Inputs),
	}
	for k, v := range req.Inputs {
		bindings[k] = v
	}

	shadow := req.Mode == skill.ModeShadow
	steps := make([]skill.StepExecution, 0, maxSteps)

	for i := 0; i < maxSteps; i++ {
		if err := ctx.Err(); err != nil {
			return r.failExecution(ctx, ex, steps, "CANCELLED", err.Error(), nowFn)
		}
		if !deadline.IsZero() && nowFn().After(deadline) {
			return r.failExecution(ctx, ex, steps, "TIMEOUT", "skill wall-clock timeout exceeded", nowFn)
		}

		st := req.Spec.Steps[i]
		stepStart := nowFn()
		resolved, resolveErr := ResolveArgs(st.Args, bindings)
		step := skill.StepExecution{
			ID:          ids(),
			ExecutionID: ex.ID,
			TenantID:    req.TenantID,
			StepID:      st.ID,
			Tool:        st.Tool,
			Input:       resolved,
			Status:      skill.StepRunning,
			Sequence:    i + 1,
		}
		if resolveErr != nil {
			step.Status = skill.StepFailed
			step.ErrorCode = "TEMPLATE_ERROR"
			step.DurationMs = nowFn().Sub(stepStart).Milliseconds()
			step, _ = r.Store.CreateStep(ctx, step)
			steps = append(steps, step)
			return r.failExecution(ctx, ex, steps, "TEMPLATE_ERROR", resolveErr.Error(), nowFn)
		}

		result, callErr := r.Exec.Call(ctx, st.Tool, resolved, shadow)
		step.DurationMs = nowFn().Sub(stepStart).Milliseconds()
		if callErr != nil {
			step.Status = skill.StepFailed
			step.ErrorCode = "EXECUTOR_ERROR"
			step.Output = map[string]any{"error": callErr.Error()}
			step, _ = r.Store.CreateStep(ctx, step)
			steps = append(steps, step)
			return r.failExecution(ctx, ex, steps, "EXECUTOR_ERROR", callErr.Error(), nowFn)
		}
		step.Output = result.Output
		if !result.OK {
			step.Status = skill.StepFailed
			step.ErrorCode = result.ErrorCode
			if step.ErrorCode == "" {
				step.ErrorCode = "TOOL_FAILED"
			}
			step, _ = r.Store.CreateStep(ctx, step)
			steps = append(steps, step)
			return r.failExecution(ctx, ex, steps, step.ErrorCode, "tool call failed", nowFn)
		}

		sideEffect := false
		if def, ok := tools.Get(st.Tool); ok {
			sideEffect = def.SideEffect
		}
		if shadow && sideEffect {
			step.Status = skill.StepShadowed
		} else {
			step.Status = skill.StepSucceeded
		}
		step, err = r.Store.CreateStep(ctx, step)
		if err != nil {
			return ex, steps, err
		}
		steps = append(steps, step)

		if st.SaveAs != "" {
			bindings[st.SaveAs] = result.Output
		}
	}

	completed := nowFn()
	ex.Status = skill.ExecSucceeded
	ex.CompletedAt = &completed
	ex.Outputs = extractOutputs(req.Spec, bindings)
	ex, err = r.Store.UpdateExecution(ctx, ex)
	return ex, steps, err
}

func (r *Runtime) failExecution(
	ctx context.Context,
	ex skill.Execution,
	steps []skill.StepExecution,
	code, msg string,
	nowFn func() time.Time,
) (skill.Execution, []skill.StepExecution, error) {
	completed := nowFn()
	ex.Status = skill.ExecFailed
	ex.ErrorCode = code
	ex.ErrorMessage = msg
	ex.CompletedAt = &completed
	updated, err := r.Store.UpdateExecution(ctx, ex)
	if err != nil {
		return ex, steps, err
	}
	return updated, steps, nil
}

func extractOutputs(spec skill.Spec, bindings map[string]any) map[string]any {
	if len(spec.Outputs) == 0 {
		return nil
	}
	out := map[string]any{}
	for name := range spec.Outputs {
		if v, ok := bindings[name]; ok {
			out[name] = v
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func cloneMap(in map[string]any) map[string]any {
	if in == nil {
		return map[string]any{}
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
