package experience

import (
	"context"
	"time"
)

// SkillStatus is the lifecycle of a Skill Candidate (V2-9).
// V2 only materializes candidates — never auto-executes them.
type SkillStatus string

const (
	SkillStatusCandidate  SkillStatus = "CANDIDATE"
	SkillStatusDeprecated SkillStatus = "DEPRECATED"
	SkillStatusArchived   SkillStatus = "ARCHIVED"
)

// Valid reports whether s is a known skill status.
func (s SkillStatus) Valid() bool {
	switch s {
	case SkillStatusCandidate, SkillStatusDeprecated, SkillStatusArchived:
		return true
	default:
		return false
	}
}

// SkillCandidate is an executable-looking Skill description derived from a Pattern.
// It is advisory for agents; the engine does not run it (V3 territory).
type SkillCandidate struct {
	ID        string `json:"id"`
	TenantID  string `json:"tenant_id"`
	PatternID string `json:"pattern_id"`

	Name        string `json:"name"`
	Description string `json:"description"`
	SpecYAML    string `json:"spec_yaml"`

	Confidence float64 `json:"confidence"`
	Utility    float64 `json:"utility"`

	Status SkillStatus `json:"status"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// SkillRepository persists skill candidates.
type SkillRepository interface {
	Create(ctx context.Context, sk SkillCandidate) (SkillCandidate, error)
	Get(ctx context.Context, tenantID, id string) (SkillCandidate, error)
	FindByPattern(ctx context.Context, tenantID, patternID string) (SkillCandidate, error)
}
