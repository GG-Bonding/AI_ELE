package episode

import (
	"time"

	"github.com/agent-experience-engine/agent-experience-engine/internal/attempt"
	"github.com/agent-experience-engine/agent-experience-engine/internal/outcome"
)

// Status is the lifecycle state of an Episode.
type Status string

const (
	StatusRunning   Status = "RUNNING"
	StatusSuccess   Status = "SUCCESS"
	StatusPartial   Status = "PARTIAL"
	StatusFailed    Status = "FAILED"
	StatusCancelled Status = "CANCELLED"
)

// Valid reports whether s is a known episode status.
func (s Status) Valid() bool {
	switch s {
	case StatusRunning, StatusSuccess, StatusPartial, StatusFailed, StatusCancelled:
		return true
	default:
		return false
	}
}

// Terminal reports whether the episode can no longer accept attempts/outcomes.
func (s Status) Terminal() bool {
	return s != StatusRunning
}

// Episode is one complete agent task execution.
type Episode struct {
	ID       string `json:"id"`
	TenantID string `json:"tenant_id"`
	AgentID  string `json:"agent_id"`
	UserID   string `json:"user_id"`

	TaskType string `json:"task_type"`
	Goal     string `json:"goal"`
	Input    string `json:"input"`

	Status Status `json:"status"`

	StartedAt   time.Time  `json:"started_at"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Nested domain aliases for lifecycle callers.
type (
	Attempt       = attempt.Attempt
	Outcome       = outcome.Outcome
	AttemptStatus = attempt.Status
)

// Attempt status aliases keep service call sites concise.
const (
	AttemptStatusRunning = attempt.StatusRunning
	AttemptStatusSuccess = attempt.StatusSuccess
	AttemptStatusFailed  = attempt.StatusFailed
	AttemptStatusSkipped = attempt.StatusSkipped
)
