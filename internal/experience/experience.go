package experience

import (
	"errors"
	"time"
)

var (
	// ErrNotFound is returned when an experience is missing for the tenant.
	ErrNotFound = errors.New("experience not found")
	// ErrInvalidInput is returned for validation failures.
	ErrInvalidInput = errors.New("invalid input")
)

// Status is the lifecycle state of a stored Experience.
type Status string

const (
	StatusCandidate  Status = "CANDIDATE"
	StatusActive     Status = "ACTIVE"
	StatusDeprecated Status = "DEPRECATED"
	StatusBlocked    Status = "BLOCKED"
	StatusArchived   Status = "ARCHIVED"
)

// Valid reports whether s is a known experience status.
func (s Status) Valid() bool {
	switch s {
	case StatusCandidate, StatusActive, StatusDeprecated, StatusBlocked, StatusArchived:
		return true
	default:
		return false
	}
}

// Retrievable reports whether the experience may appear in default retrieval.
func (s Status) Retrievable() bool {
	return s == StatusActive || s == StatusCandidate
}

// Evidence is persisted supporting-trace metadata for an experience (V1-08).
type Evidence struct {
	FailedAttemptCount  int      `json:"failed_attempt_count,omitempty"`
	SuccessAttemptCount int      `json:"success_attempt_count,omitempty"`
	HasFailureContrast  bool     `json:"has_failure_contrast,omitempty"`
	HasToolErrorCode    bool     `json:"has_tool_error_code,omitempty"`
	SourceEpisodeID     string   `json:"source_episode_id,omitempty"`
	AttemptIDs          []string `json:"attempt_ids,omitempty"`
	OutcomeID           string   `json:"outcome_id,omitempty"`
}

// Experience is a long-lived reusable lesson extracted from episodes.
type Experience struct {
	ID       string `json:"id"`
	TenantID string `json:"tenant_id"`

	Type     Type   `json:"type"`
	Scope    Scope  `json:"scope"`
	ScopeKey string `json:"scope_key,omitempty"`

	Trigger string `json:"trigger"`
	Content string `json:"content"`

	SourceEpisodeID string `json:"source_episode_id,omitempty"`

	Evidence Evidence `json:"evidence,omitempty"`

	Confidence float64 `json:"confidence"`
	Utility    float64 `json:"utility"`
	Alpha      float64 `json:"alpha"`
	Beta       float64 `json:"beta"`

	SuccessCount int64 `json:"success_count"`
	FailureCount int64 `json:"failure_count"`
	UseCount     int64 `json:"use_count"`

	Status  Status `json:"status"`
	Version int64  `json:"version"`

	SupersedesID *string `json:"supersedes_id,omitempty"`

	// Embedding is optional in API responses; persistence always stores it when created via embedder.
	Embedding []float32 `json:"-"`

	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
}

// SearchFilter constrains metadata before vector ranking.
type SearchFilter struct {
	TenantID string
	Types    []Type
	Scopes   []Scope
	ScopeKey string
	Statuses []Status // empty => ACTIVE + CANDIDATE only

	// QueryEmbedding is required for similarity search.
	QueryEmbedding []float32
	TopK           int
}

// ScoredExperience is a retrieved experience with similarity score.
type ScoredExperience struct {
	Experience Experience `json:"experience"`
	Similarity float64    `json:"similarity"`
}
