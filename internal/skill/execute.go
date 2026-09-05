package skill

import (
	"context"
	"fmt"
	"strings"
)

// ExecutionRunner runs a validated SkillVersion (implemented by skillruntime.Runtime).
type ExecutionRunner interface {
	Run(ctx context.Context, req ExecutionRunRequest) (Execution, []StepExecution, error)
	Resume(ctx context.Context, req ResumeRequest) (Execution, []StepExecution, error)
	Approve(ctx context.Context, tenantID, approvalID string) (ApprovalRequest, error)
	Reject(ctx context.Context, tenantID, approvalID, reason string) (ApprovalRequest, error)
}

// ExecutionRunRequest is the gated request passed to the runtime.
type ExecutionRunRequest struct {
	TenantID       string
	EpisodeID      string
	SkillID        string
	SkillVersionID string
	Mode           ExecutionMode
	Spec           Spec
	Inputs         map[string]any
	IdempotencyKey string
	AvailableTools []string
	RuntimeEnabled bool
}

// ResumeRequest continues a WAITING_APPROVAL execution after server-side approval.
type ResumeRequest struct {
	TenantID       string
	ExecutionID    string
	AvailableTools []string
	RuntimeEnabled bool
	Spec           Spec // filled by ExecutionService from version
}

// ExecuteInput is the HTTP/service-facing execute request (IDs only — no Spec).
type ExecuteInput struct {
	TenantID       string
	EpisodeID      string
	SkillID        string
	VersionID      string
	Mode           ExecutionMode
	Inputs         map[string]any
	AvailableTools []string
	IdempotencyKey string
	RuntimeEnabled bool
}

// ExecutionService enforces lifecycle ownership before any Runtime call.
type ExecutionService struct {
	Repo     Repository
	Store    ExecutionStore // for resume Spec lookup
	Runner   ExecutionRunner
	Registry *RegistryService
	Promote  PromoteConfig
}

// Execute loads the version, enforces lifecycle, then runs the skill.
func (s *ExecutionService) Execute(ctx context.Context, in ExecuteInput) (Execution, []StepExecution, error) {
	if s == nil || s.Repo == nil || s.Runner == nil {
		return Execution{}, nil, fmt.Errorf("%w: execution service not configured", ErrInvalidInput)
	}
	tenantID := strings.TrimSpace(in.TenantID)
	versionID := strings.TrimSpace(in.VersionID)
	skillID := strings.TrimSpace(in.SkillID)
	if tenantID == "" || versionID == "" {
		return Execution{}, nil, fmt.Errorf("%w: tenant_id and version_id are required", ErrInvalidInput)
	}

	ver, err := s.Repo.GetVersion(ctx, tenantID, versionID)
	if err != nil {
		return Execution{}, nil, err
	}
	if skillID != "" && skillID != ver.SkillID {
		return Execution{}, nil, fmt.Errorf("%w: skill_id does not own version", ErrInvalidInput)
	}
	if ver.ValidationStatus != ValidationPassed {
		return Execution{}, nil, fmt.Errorf("%w: version validation is %s (must be PASSED)", ErrInvalidTransition, ver.ValidationStatus)
	}

	mode := in.Mode
	if mode == "" {
		mode = ModeShadow
	}
	switch mode {
	case ModeShadow:
		switch ver.Status {
		case VersionShadow, VersionActive:
			// ok
		default:
			return Execution{}, nil, fmt.Errorf("%w: SHADOW execution requires SHADOW or ACTIVE version (got %s)", ErrInvalidTransition, ver.Status)
		}
	case ModeLive:
		if ver.Status != VersionActive {
			return Execution{}, nil, fmt.Errorf("%w: LIVE execution requires ACTIVE version (got %s)", ErrInvalidTransition, ver.Status)
		}
	default:
		return Execution{}, nil, fmt.Errorf("%w: unknown mode %q", ErrInvalidInput, mode)
	}

	ex, steps, err := s.Runner.Run(ctx, ExecutionRunRequest{
		TenantID:       tenantID,
		EpisodeID:      strings.TrimSpace(in.EpisodeID),
		SkillID:        ver.SkillID,
		SkillVersionID: ver.ID,
		Mode:           mode,
		Spec:           ver.Spec,
		Inputs:         in.Inputs,
		IdempotencyKey: strings.TrimSpace(in.IdempotencyKey),
		AvailableTools: in.AvailableTools,
		RuntimeEnabled: in.RuntimeEnabled,
	})
	if err != nil {
		return ex, steps, err
	}

	if mode == ModeShadow && s.Registry != nil {
		_, _ = s.Registry.RecordShadowOutcome(ctx, tenantID, ver.ID, ex.Status == ExecSucceeded)
	}
	return ex, steps, nil
}

// Resume continues a previously approved execution (Spec loaded from the version ledger).
func (s *ExecutionService) Resume(ctx context.Context, tenantID, executionID string, availableTools []string, runtimeEnabled bool) (Execution, []StepExecution, error) {
	if s == nil || s.Repo == nil || s.Store == nil || s.Runner == nil {
		return Execution{}, nil, fmt.Errorf("%w: execution service not configured", ErrInvalidInput)
	}
	tenantID = strings.TrimSpace(tenantID)
	executionID = strings.TrimSpace(executionID)
	if tenantID == "" || executionID == "" {
		return Execution{}, nil, fmt.Errorf("%w: tenant_id and execution_id are required", ErrInvalidInput)
	}
	ex, err := s.Store.GetExecution(ctx, tenantID, executionID)
	if err != nil {
		return Execution{}, nil, err
	}
	ver, err := s.Repo.GetVersion(ctx, tenantID, ex.SkillVersionID)
	if err != nil {
		return Execution{}, nil, err
	}
	if ver.ValidationStatus != ValidationPassed || ver.Status != VersionActive {
		return Execution{}, nil, fmt.Errorf("%w: resume requires ACTIVE+PASSED version", ErrInvalidTransition)
	}
	return s.Runner.Resume(ctx, ResumeRequest{
		TenantID:       tenantID,
		ExecutionID:    executionID,
		AvailableTools: availableTools,
		RuntimeEnabled: runtimeEnabled,
		Spec:           ver.Spec,
	})
}

// ApproveApproval marks a persisted approval APPROVED (server-side only).
func (s *ExecutionService) ApproveApproval(ctx context.Context, tenantID, approvalID string) (ApprovalRequest, error) {
	if s == nil || s.Runner == nil {
		return ApprovalRequest{}, fmt.Errorf("%w: execution service not configured", ErrInvalidInput)
	}
	return s.Runner.Approve(ctx, tenantID, approvalID)
}

// RejectApproval marks a persisted approval REJECTED.
func (s *ExecutionService) RejectApproval(ctx context.Context, tenantID, approvalID, reason string) (ApprovalRequest, error) {
	if s == nil || s.Runner == nil {
		return ApprovalRequest{}, fmt.Errorf("%w: execution service not configured", ErrInvalidInput)
	}
	return s.Runner.Reject(ctx, tenantID, approvalID, reason)
}
