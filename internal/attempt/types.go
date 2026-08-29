package attempt

import (
	"encoding/json"
	"time"
)

// Status is the result of a single attempt inside an episode.
type Status string

const (
	StatusRunning Status = "RUNNING"
	StatusSuccess Status = "SUCCESS"
	StatusFailed  Status = "FAILED"
	StatusSkipped Status = "SKIPPED"
)

// Valid reports whether s is a known attempt status.
func (s Status) Valid() bool {
	switch s {
	case StatusRunning, StatusSuccess, StatusFailed, StatusSkipped:
		return true
	default:
		return false
	}
}

// Attempt is one try within an Episode.
type Attempt struct {
	ID        string `json:"id"`
	EpisodeID string `json:"episode_id"`
	TenantID  string `json:"tenant_id"`

	Sequence int `json:"sequence"`

	Hypothesis string `json:"hypothesis"`
	Action     string `json:"action"`
	ToolName   string `json:"tool_name"`

	Input  json.RawMessage `json:"input,omitempty"`
	Output json.RawMessage `json:"output,omitempty"`

	Status Status `json:"status"`

	ErrorCode    string `json:"error_code,omitempty"`
	ErrorMessage string `json:"error_message,omitempty"`

	StartedAt   time.Time  `json:"started_at"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
}
