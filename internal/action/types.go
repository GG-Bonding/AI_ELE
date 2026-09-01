package action

import (
	"encoding/json"
	"time"
)

// Type classifies what kind of agent action was taken.
type Type string

const (
	TypeToolCall     Type = "TOOL_CALL"
	TypePlan         Type = "PLAN"
	TypeDecision     Type = "DECISION"
	TypeAnswer       Type = "ANSWER"
	TypeWorkflowStep Type = "WORKFLOW_STEP"
)

// Valid reports whether t is a known action type.
func (t Type) Valid() bool {
	switch t {
	case TypeToolCall, TypePlan, TypeDecision, TypeAnswer, TypeWorkflowStep:
		return true
	default:
		return false
	}
}

// Status is the result of a single agent action.
type Status string

const (
	StatusRunning Status = "RUNNING"
	StatusSuccess Status = "SUCCESS"
	StatusFailed  Status = "FAILED"
	StatusSkipped Status = "SKIPPED"
)

// Valid reports whether s is a known action status.
func (s Status) Valid() bool {
	switch s {
	case StatusRunning, StatusSuccess, StatusFailed, StatusSkipped:
		return true
	default:
		return false
	}
}

// AgentAction is one concrete step the agent took inside an episode.
// Distinct from Attempt (trial log): Actions are the attribution graph nodes.
type AgentAction struct {
	ID        string `json:"id"`
	TenantID  string `json:"tenant_id"`
	EpisodeID string `json:"episode_id"`
	Sequence  int    `json:"sequence"`

	Type     Type   `json:"type"`
	ToolName string `json:"tool_name,omitempty"`

	Input  json.RawMessage `json:"input,omitempty"`
	Output json.RawMessage `json:"output,omitempty"`

	Status Status `json:"status"`

	// AttemptID optionally links back to a Phase-1 Attempt row.
	AttemptID string `json:"attempt_id,omitempty"`

	StartedAt   time.Time  `json:"started_at"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
}

// ExperienceActionLink asserts that an experience influenced a specific action.
// This is the foundation for V2 attribution (not retrieval-score credit).
type ExperienceActionLink struct {
	ID           string  `json:"id"`
	TenantID     string  `json:"tenant_id"`
	EpisodeID    string  `json:"episode_id"`
	ExperienceID string  `json:"experience_id"`
	ActionID     string  `json:"action_id"`
	Influence    float64 `json:"influence"`
	// AffectedFields lists JSON paths this experience influenced (e.g. "input.priority").
	// Used by ACTION_FIELD attribution (V2.1); empty means action-level influence only.
	AffectedFields []string  `json:"affected_fields,omitempty"`
	Evidence       string    `json:"evidence,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
}
