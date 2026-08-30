package experience

import (
	"context"
	"time"
)

// PatternStatus is the lifecycle state of a generalized Pattern (V2-7).
type PatternStatus string

const (
	PatternStatusCandidate PatternStatus = "CANDIDATE"
	PatternStatusActive    PatternStatus = "ACTIVE"
	PatternStatusDeprecated PatternStatus = "DEPRECATED"
	PatternStatusArchived  PatternStatus = "ARCHIVED"
)

// Valid reports whether s is a known pattern status.
func (s PatternStatus) Valid() bool {
	switch s {
	case PatternStatusCandidate, PatternStatusActive, PatternStatusDeprecated, PatternStatusArchived:
		return true
	default:
		return false
	}
}

// Retrievable reports whether the pattern may appear in default retrieval later (V2-8+).
func (s PatternStatus) Retrievable() bool {
	return s == PatternStatusActive || s == PatternStatusCandidate
}

// Pattern is a generalized rule derived from multiple concrete experiences (V2-7).
type Pattern struct {
	ID       string `json:"id"`
	TenantID string `json:"tenant_id"`

	Type     Type   `json:"type"`
	Scope    Scope  `json:"scope"`
	ScopeKey string `json:"scope_key,omitempty"`

	Trigger string `json:"trigger"`
	Content string `json:"content"`

	Confidence   float64 `json:"confidence"`
	Utility      float64 `json:"utility"`
	Alpha        float64 `json:"alpha"`
	Beta         float64 `json:"beta"`
	SuccessCount int64   `json:"success_count"`
	FailureCount int64   `json:"failure_count"`
	SupportCount int     `json:"support_count"`

	Status PatternStatus `json:"status"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// PatternEvidence links a Pattern to a supporting Experience.
type PatternEvidence struct {
	PatternID    string    `json:"pattern_id"`
	ExperienceID string    `json:"experience_id"`
	CreatedAt    time.Time `json:"created_at"`
}

// PatternRepository persists patterns and their supporting experiences.
type PatternRepository interface {
	Create(ctx context.Context, p Pattern) (Pattern, error)
	Update(ctx context.Context, p Pattern) (Pattern, error)
	Get(ctx context.Context, tenantID, id string) (Pattern, error)
	AddEvidence(ctx context.Context, ev PatternEvidence) error
	ListEvidence(ctx context.Context, tenantID, patternID string) ([]PatternEvidence, error)
	// FindByExperience returns patterns that already include any of the experience IDs.
	FindByExperience(ctx context.Context, tenantID string, experienceIDs []string) ([]Pattern, error)
}
