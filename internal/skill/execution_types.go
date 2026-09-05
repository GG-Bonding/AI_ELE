package skill

import (
	"context"
	"time"
)

// ExecutionMode is SHADOW (no side effects) or LIVE.
type ExecutionMode string

const (
	ModeShadow ExecutionMode = "SHADOW"
	ModeLive   ExecutionMode = "LIVE"
)

// ExecutionStatus is the lifecycle of one SkillExecution.
type ExecutionStatus string

const (
	ExecPending          ExecutionStatus = "PENDING"
	ExecRunning          ExecutionStatus = "RUNNING"
	ExecSucceeded        ExecutionStatus = "SUCCEEDED"
	ExecFailed           ExecutionStatus = "FAILED"
	ExecCancelled        ExecutionStatus = "CANCELLED"
	ExecWaitingApproval  ExecutionStatus = "WAITING_APPROVAL"
	ExecDenied           ExecutionStatus = "DENIED"
)

// StepStatus is one step outcome.
type StepStatus string

const (
	StepPending   StepStatus = "PENDING"
	StepRunning   StepStatus = "RUNNING"
	StepSucceeded StepStatus = "SUCCEEDED"
	StepFailed    StepStatus = "FAILED"
	StepSkipped   StepStatus = "SKIPPED"
	StepShadowed  StepStatus = "SHADOWED" // dry-run side-effect step
)

// Execution is one Skill run (shadow or live).
type Execution struct {
	ID             string          `json:"id"`
	TenantID       string          `json:"tenant_id"`
	EpisodeID      string          `json:"episode_id,omitempty"`
	SkillID        string          `json:"skill_id"`
	SkillVersionID string          `json:"skill_version_id"`
	Mode           ExecutionMode   `json:"mode"`
	Status         ExecutionStatus `json:"status"`
	IdempotencyKey string          `json:"idempotency_key,omitempty"`
	Inputs         map[string]any  `json:"inputs,omitempty"`
	Outputs        map[string]any  `json:"outputs,omitempty"`
	ErrorCode      string          `json:"error_code,omitempty"`
	ErrorMessage   string          `json:"error_message,omitempty"`
	StartedAt      time.Time       `json:"started_at"`
	CompletedAt    *time.Time      `json:"completed_at,omitempty"`
}

// StepExecution is one tool step within an Execution.
type StepExecution struct {
	ID          string         `json:"id"`
	ExecutionID string         `json:"execution_id"`
	TenantID    string         `json:"tenant_id"`
	StepID      string         `json:"step_id"`
	Tool        string         `json:"tool"`
	Input       map[string]any `json:"input,omitempty"`
	Output      map[string]any `json:"output,omitempty"`
	Status      StepStatus     `json:"status"`
	ErrorCode   string         `json:"error_code,omitempty"`
	DurationMs  int64          `json:"duration_ms,omitempty"`
	Sequence    int            `json:"sequence"`
}

// ApprovalStatus for high-risk live runs.
type ApprovalStatus string

const (
	ApprovalPending  ApprovalStatus = "PENDING"
	ApprovalApproved ApprovalStatus = "APPROVED"
	ApprovalRejected ApprovalStatus = "REJECTED"
)

// ApprovalRequest is a gate for HIGH/CRITICAL live executions (V3-6; no UI).
type ApprovalRequest struct {
	ID          string         `json:"id"`
	TenantID    string         `json:"tenant_id"`
	ExecutionID string         `json:"execution_id"`
	SkillID     string         `json:"skill_id"`
	Status      ApprovalStatus `json:"status"`
	Reason      string         `json:"reason,omitempty"`
	CreatedAt   time.Time      `json:"created_at"`
	ResolvedAt  *time.Time     `json:"resolved_at,omitempty"`
}

// LearningEvent is an exactly-once Skill utility update (V3-7).
type LearningEvent struct {
	ID             string     `json:"id"`
	TenantID       string     `json:"tenant_id"`
	SkillID        string     `json:"skill_id"`
	SkillVersionID string     `json:"skill_version_id"`
	ExecutionID    string     `json:"execution_id,omitempty"`
	FeedbackID     string     `json:"feedback_id"`
	Reward         float64    `json:"reward"`
	Confidence     float64    `json:"confidence"`
	Credit         float64    `json:"credit"`
	Status         string     `json:"status"` // PENDING | APPLIED | FAILED
	CreatedAt      time.Time  `json:"created_at"`
	AppliedAt      *time.Time `json:"applied_at,omitempty"`
}

// ExecutionStore persists executions / steps / approvals.
type ExecutionStore interface {
	CreateExecution(ctx context.Context, ex Execution) (Execution, error)
	UpdateExecution(ctx context.Context, ex Execution) (Execution, error)
	GetExecution(ctx context.Context, tenantID, id string) (Execution, error)
	GetExecutionByIdempotency(ctx context.Context, tenantID, key string) (Execution, error)
	CreateStep(ctx context.Context, st StepExecution) (StepExecution, error)
	ListSteps(ctx context.Context, tenantID, executionID string) ([]StepExecution, error)
	CreateApproval(ctx context.Context, req ApprovalRequest) (ApprovalRequest, error)
	UpdateApproval(ctx context.Context, req ApprovalRequest) (ApprovalRequest, error)
	GetApproval(ctx context.Context, tenantID, id string) (ApprovalRequest, error)
	GetApprovalByExecution(ctx context.Context, tenantID, executionID string) (ApprovalRequest, error)
}

// LearningStore persists skill learning events.
type LearningStore interface {
	CreateLearningEvent(ctx context.Context, ev LearningEvent) (LearningEvent, error)
	GetLearningEvent(ctx context.Context, tenantID, id string) (LearningEvent, error)
	GetLearningEventByFeedbackVersion(ctx context.Context, tenantID, feedbackID, versionID string) (LearningEvent, error)
	MarkLearningApplied(ctx context.Context, tenantID, id string, at time.Time) error
	MarkLearningFailed(ctx context.Context, tenantID, id string) error
}

// LearningApplier atomically applies a PENDING/FAILED skill learning event to utility.
type LearningApplier interface {
	ApplyPending(ctx context.Context, tenantID, eventID string) (LearningApplyResult, error)
}

// LearningApplyResult is the outcome of an atomic skill learning apply.
type LearningApplyResult struct {
	Version        Version
	AlreadyApplied bool
}
