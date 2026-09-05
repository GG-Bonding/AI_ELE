package skillruntime

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/agent-experience-engine/agent-experience-engine/internal/skill"
	"github.com/agent-experience-engine/agent-experience-engine/internal/toolregistry"
	"github.com/google/uuid"
)

// Runtime executes Skill Specs in SHADOW or LIVE mode behind a policy gate.
type Runtime struct {
	Tools   *toolregistry.Registry
	Exec    ToolExecutor
	Preview PreviewExecutor // required for SHADOW side-effect tools
	Policy  ExecutionPolicy
	Store   skill.ExecutionStore
	IDs     func() string
	Now     func() time.Time
	Sleep   func(ctx context.Context, d time.Duration) error // injectable for tests
}

// Run executes a Skill under policy, budget, and template resolution.
// Client-supplied approval flags are not accepted — use Approve + Resume.
func (r *Runtime) Run(ctx context.Context, req skill.ExecutionRunRequest) (skill.Execution, []skill.StepExecution, error) {
	if r == nil || r.Store == nil || r.Exec == nil {
		return skill.Execution{}, nil, fmt.Errorf("skillruntime: Runtime requires Store and Exec")
	}
	policy := r.policy()
	ids := r.ids()
	nowFn := r.now()
	tools := r.tools()

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

	specTools := uniqueTools(req.Spec)
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
	ex, err = r.createExecutionIdempotent(ctx, ex)
	if err != nil {
		return skill.Execution{}, nil, err
	}
	// Concurrent create may have returned an existing execution.
	if ex.Status != skill.ExecPending && ex.IdempotencyKey != "" {
		steps, listErr := r.Store.ListSteps(ctx, req.TenantID, ex.ID)
		return ex, steps, listErr
	}

	switch decision {
	case DecisionDeny:
		return r.finishDenied(ctx, ex, "POLICY_DENIED", reason, nowFn)
	case DecisionRequireApproval:
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
	case DecisionShadowOnly:
		if req.Mode == skill.ModeLive {
			return r.finishDenied(ctx, ex, "SHADOW_ONLY", orDefault(reason, "live execution not permitted; shadow only"), nowFn)
		}
	case DecisionAllow:
		// continue
	default:
		return r.finishDenied(ctx, ex, "UNKNOWN_DECISION", string(decision), nowFn)
	}

	return r.runSteps(ctx, ex, req.Spec, req.Inputs, req.Mode == skill.ModeShadow, nowFn, ids, tools)
}

// Resume continues a WAITING_APPROVAL execution after a persisted APPROVED approval.
func (r *Runtime) Resume(ctx context.Context, req skill.ResumeRequest) (skill.Execution, []skill.StepExecution, error) {
	if r == nil || r.Store == nil || r.Exec == nil {
		return skill.Execution{}, nil, fmt.Errorf("skillruntime: Runtime requires Store and Exec")
	}
	nowFn := r.now()
	tools := r.tools()
	ids := r.ids()

	ex, err := r.Store.GetExecution(ctx, req.TenantID, req.ExecutionID)
	if err != nil {
		return skill.Execution{}, nil, err
	}
	if ex.Status != skill.ExecWaitingApproval {
		steps, listErr := r.Store.ListSteps(ctx, req.TenantID, ex.ID)
		return ex, steps, listErr
	}

	appr, err := r.Store.GetApprovalByExecution(ctx, req.TenantID, ex.ID)
	if err != nil {
		return skill.Execution{}, nil, fmt.Errorf("%w: no approval for execution", skill.ErrInvalidInput)
	}
	if appr.Status != skill.ApprovalApproved {
		return skill.Execution{}, nil, fmt.Errorf("%w: approval status is %s (must be APPROVED)", skill.ErrInvalidTransition, appr.Status)
	}
	if appr.ExecutionID != ex.ID || appr.TenantID != ex.TenantID || appr.SkillID != ex.SkillID {
		return skill.Execution{}, nil, fmt.Errorf("%w: approval does not match execution", skill.ErrInvalidInput)
	}
	if !req.RuntimeEnabled {
		return r.finishDenied(ctx, ex, "RUNTIME_DISABLED", "skill runtime disabled", nowFn)
	}

	policy := r.policy()
	decision, reason, err := policy.Decide(ctx, PolicyRequest{
		Mode:             ex.Mode,
		Risk:             req.Spec.Risk.Level,
		RequiresApproval: req.Spec.Risk.RequiresApproval,
		RuntimeEnabled:   true,
		AvailableTools:   req.AvailableTools,
		SpecTools:        uniqueTools(req.Spec),
		ApprovalApproved: true,
	})
	if err != nil {
		return skill.Execution{}, nil, err
	}
	if decision == DecisionDeny {
		return r.finishDenied(ctx, ex, "POLICY_DENIED", reason, nowFn)
	}

	ex.Status = skill.ExecPending
	ex.ErrorCode = ""
	ex.ErrorMessage = ""
	ex, err = r.Store.UpdateExecution(ctx, ex)
	if err != nil {
		return ex, nil, err
	}
	_ = ids
	return r.runSteps(ctx, ex, req.Spec, ex.Inputs, ex.Mode == skill.ModeShadow, nowFn, ids, tools)
}

// Approve marks a PENDING approval APPROVED (server-side workflow).
func (r *Runtime) Approve(ctx context.Context, tenantID, approvalID string) (skill.ApprovalRequest, error) {
	if r == nil || r.Store == nil {
		return skill.ApprovalRequest{}, fmt.Errorf("skillruntime: Store required")
	}
	appr, err := r.Store.GetApproval(ctx, tenantID, approvalID)
	if err != nil {
		return skill.ApprovalRequest{}, err
	}
	if appr.Status != skill.ApprovalPending {
		return skill.ApprovalRequest{}, fmt.Errorf("%w: approval is %s", skill.ErrInvalidTransition, appr.Status)
	}
	now := r.now()()
	appr.Status = skill.ApprovalApproved
	appr.ResolvedAt = &now
	return r.Store.UpdateApproval(ctx, appr)
}

// Reject marks a PENDING approval REJECTED and cancels the execution.
func (r *Runtime) Reject(ctx context.Context, tenantID, approvalID, reason string) (skill.ApprovalRequest, error) {
	if r == nil || r.Store == nil {
		return skill.ApprovalRequest{}, fmt.Errorf("skillruntime: Store required")
	}
	appr, err := r.Store.GetApproval(ctx, tenantID, approvalID)
	if err != nil {
		return skill.ApprovalRequest{}, err
	}
	if appr.Status != skill.ApprovalPending {
		return skill.ApprovalRequest{}, fmt.Errorf("%w: approval is %s", skill.ErrInvalidTransition, appr.Status)
	}
	now := r.now()()
	appr.Status = skill.ApprovalRejected
	if strings.TrimSpace(reason) != "" {
		appr.Reason = reason
	}
	appr.ResolvedAt = &now
	appr, err = r.Store.UpdateApproval(ctx, appr)
	if err != nil {
		return appr, err
	}
	ex, getErr := r.Store.GetExecution(ctx, tenantID, appr.ExecutionID)
	if getErr == nil && ex.Status == skill.ExecWaitingApproval {
		ex.Status = skill.ExecCancelled
		ex.ErrorCode = "APPROVAL_REJECTED"
		ex.ErrorMessage = appr.Reason
		ex.CompletedAt = &now
		_, _ = r.Store.UpdateExecution(ctx, ex)
	}
	return appr, nil
}

func (r *Runtime) runSteps(
	ctx context.Context,
	ex skill.Execution,
	spec skill.Spec,
	inputs map[string]any,
	shadow bool,
	nowFn func() time.Time,
	ids func() string,
	tools *toolregistry.Registry,
) (skill.Execution, []skill.StepExecution, error) {
	runCtx := ctx
	var cancel context.CancelFunc
	if spec.TimeoutMs > 0 {
		runCtx, cancel = context.WithTimeout(ctx, time.Duration(spec.TimeoutMs)*time.Millisecond)
		defer cancel()
	}

	ex.Status = skill.ExecRunning
	var err error
	ex, err = r.Store.UpdateExecution(runCtx, ex)
	if err != nil {
		return ex, nil, err
	}

	bindings := map[string]any{"inputs": cloneMap(inputs)}
	for k, v := range inputs {
		bindings[k] = v
	}

	for _, pre := range spec.Preconditions {
		ok, evalErr := EvalCondition(pre.Expr, bindings)
		if evalErr != nil {
			return r.failExecution(runCtx, ex, nil, "PRECONDITION_ERROR", evalErr.Error(), nowFn)
		}
		if !ok {
			return r.failExecution(runCtx, ex, nil, "PRECONDITION_FAILED", "precondition not met: "+pre.Expr, nowFn)
		}
	}

	maxSteps := spec.MaxSteps
	if maxSteps <= 0 {
		maxSteps = len(spec.Steps)
	}
	if maxSteps > len(spec.Steps) {
		maxSteps = len(spec.Steps)
	}

	steps := make([]skill.StepExecution, 0, maxSteps)
	seq := 0
	for i := 0; i < maxSteps; i++ {
		if err := runCtx.Err(); err != nil {
			code := "CANCELLED"
			if errors.Is(err, context.DeadlineExceeded) {
				code = "TIMEOUT"
			}
			return r.failExecution(runCtx, ex, steps, code, err.Error(), nowFn)
		}

		st := spec.Steps[i]
		if st.When != nil {
			ok, evalErr := EvalCondition(st.When.Expr, bindings)
			if evalErr != nil {
				return r.failExecution(runCtx, ex, steps, "WHEN_ERROR", evalErr.Error(), nowFn)
			}
			if !ok {
				seq++
				step := skill.StepExecution{
					ID: ids(), ExecutionID: ex.ID, TenantID: ex.TenantID,
					StepID: st.ID, Tool: st.Tool, Status: skill.StepSkipped, Sequence: seq,
				}
				step, _ = r.Store.CreateStep(runCtx, step)
				steps = append(steps, step)
				continue
			}
		}

		resolved, resolveErr := ResolveArgs(st.Args, bindings)
		seq++
		stepStart := nowFn()
		step := skill.StepExecution{
			ID: ids(), ExecutionID: ex.ID, TenantID: ex.TenantID,
			StepID: st.ID, Tool: st.Tool, Input: resolved, Status: skill.StepRunning, Sequence: seq,
		}
		if resolveErr != nil {
			step.Status = skill.StepFailed
			step.ErrorCode = "TEMPLATE_ERROR"
			step.DurationMs = nowFn().Sub(stepStart).Milliseconds()
			step.Output = map[string]any{"error": resolveErr.Error()}
			step, _ = r.Store.CreateStep(runCtx, step)
			steps = append(steps, step)
			onError := "fail"
			if st.OnError != nil && strings.TrimSpace(st.OnError.Action) != "" {
				onError = strings.ToLower(strings.TrimSpace(st.OnError.Action))
			}
			if onError == "continue" {
				continue
			}
			return r.failExecution(runCtx, ex, steps, "TEMPLATE_ERROR", resolveErr.Error(), nowFn)
		}

		maxAttempts := 1
		backoff := time.Duration(0)
		if st.Retry != nil {
			if st.Retry.MaxAttempts > 1 {
				maxAttempts = st.Retry.MaxAttempts
			}
			if st.Retry.BackoffMs > 0 {
				backoff = time.Duration(st.Retry.BackoffMs) * time.Millisecond
			}
		}
		onError := "fail"
		if st.OnError != nil && strings.TrimSpace(st.OnError.Action) != "" {
			onError = strings.ToLower(strings.TrimSpace(st.OnError.Action))
		}

		var result ToolResult
		var callErr error
		for attempt := 1; attempt <= maxAttempts; attempt++ {
			result, callErr = r.invokeTool(runCtx, tools, st.Tool, resolved, shadow)
			if callErr == nil && result.OK {
				break
			}
			if attempt < maxAttempts && (onError == "retry" || st.Retry != nil) {
				if backoff > 0 {
					if sleepErr := r.sleep(runCtx, backoff); sleepErr != nil {
						callErr = sleepErr
						break
					}
				}
				continue
			}
			break
		}

		step.DurationMs = nowFn().Sub(stepStart).Milliseconds()
		step.Output = result.Output
		if callErr != nil {
			step.Status = skill.StepFailed
			step.ErrorCode = "EXECUTOR_ERROR"
			if errors.Is(callErr, ErrShadowUnsupported) {
				step.ErrorCode = "SHADOW_UNSUPPORTED"
			}
			step.Output = map[string]any{"error": callErr.Error()}
			step, _ = r.Store.CreateStep(runCtx, step)
			steps = append(steps, step)
			if onError == "continue" {
				continue
			}
			return r.failExecution(runCtx, ex, steps, step.ErrorCode, callErr.Error(), nowFn)
		}
		if !result.OK {
			step.Status = skill.StepFailed
			step.ErrorCode = result.ErrorCode
			if step.ErrorCode == "" {
				step.ErrorCode = "TOOL_FAILED"
			}
			step, _ = r.Store.CreateStep(runCtx, step)
			steps = append(steps, step)
			if onError == "continue" {
				continue
			}
			return r.failExecution(runCtx, ex, steps, step.ErrorCode, "tool call failed", nowFn)
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
		step, err = r.Store.CreateStep(runCtx, step)
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
	ex.ErrorCode = ""
	ex.ErrorMessage = ""
	ex.Outputs = extractOutputs(spec, bindings)
	ex, err = r.Store.UpdateExecution(runCtx, ex)
	return ex, steps, err
}

func (r *Runtime) invokeTool(ctx context.Context, tools *toolregistry.Registry, tool string, input map[string]any, shadow bool) (ToolResult, error) {
	sideEffect := false
	if def, ok := tools.Get(tool); ok {
		sideEffect = def.SideEffect
	}
	if shadow && sideEffect {
		if r.Preview == nil {
			return ToolResult{}, ErrShadowUnsupported
		}
		return r.Preview.Preview(ctx, tool, input)
	}
	return r.Exec.Execute(ctx, tool, input)
}

func (r *Runtime) createExecutionIdempotent(ctx context.Context, ex skill.Execution) (skill.Execution, error) {
	created, err := r.Store.CreateExecution(ctx, ex)
	if err == nil {
		return created, nil
	}
	if ex.IdempotencyKey == "" {
		return skill.Execution{}, err
	}
	// Unique conflict → return existing.
	existing, getErr := r.Store.GetExecutionByIdempotency(ctx, ex.TenantID, ex.IdempotencyKey)
	if getErr == nil {
		return existing, nil
	}
	return skill.Execution{}, err
}

func (r *Runtime) finishDenied(ctx context.Context, ex skill.Execution, code, msg string, nowFn func() time.Time) (skill.Execution, []skill.StepExecution, error) {
	completed := nowFn()
	ex.Status = skill.ExecDenied
	ex.ErrorCode = code
	ex.ErrorMessage = msg
	ex.CompletedAt = &completed
	updated, err := r.Store.UpdateExecution(ctx, ex)
	return updated, nil, err
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

func (r *Runtime) policy() ExecutionPolicy {
	if r.Policy == nil {
		return DefaultPolicy{}
	}
	return r.Policy
}

func (r *Runtime) ids() func() string {
	if r.IDs == nil {
		return func() string { return uuid.NewString() }
	}
	return r.IDs
}

func (r *Runtime) now() func() time.Time {
	if r.Now == nil {
		return func() time.Time { return time.Now().UTC() }
	}
	return r.Now
}

func (r *Runtime) tools() *toolregistry.Registry {
	if r.Tools == nil {
		return toolregistry.Default()
	}
	return r.Tools
}

func (r *Runtime) sleep(ctx context.Context, d time.Duration) error {
	if r.Sleep != nil {
		return r.Sleep(ctx, d)
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func uniqueTools(spec skill.Spec) []string {
	out := make([]string, 0, len(spec.Steps))
	seen := map[string]struct{}{}
	for _, st := range spec.Steps {
		if _, ok := seen[st.Tool]; ok {
			continue
		}
		seen[st.Tool] = struct{}{}
		out = append(out, st.Tool)
	}
	return out
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

func orDefault(s, def string) string {
	if strings.TrimSpace(s) == "" {
		return def
	}
	return s
}

// Ensure Runtime satisfies skill.ExecutionRunner.
var _ skill.ExecutionRunner = (*Runtime)(nil)
