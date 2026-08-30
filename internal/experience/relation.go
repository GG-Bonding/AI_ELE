package experience

import (
	"context"
	"time"
)

// RelationType classifies how two experiences relate (V2-5+).
type RelationType string

const (
	RelationDuplicate  RelationType = "DUPLICATE"
	RelationSupports   RelationType = "SUPPORTS"
	RelationConflicts  RelationType = "CONFLICTS"
	RelationSupersedes RelationType = "SUPERSEDES"
	RelationDerivedFrom RelationType = "DERIVED_FROM"
)

// Valid reports whether t is a known relation type.
func (t RelationType) Valid() bool {
	switch t {
	case RelationDuplicate, RelationSupports, RelationConflicts, RelationSupersedes, RelationDerivedFrom:
		return true
	default:
		return false
	}
}

// ExperienceRelation is a directed edge between two experiences in a tenant.
type ExperienceRelation struct {
	ID                 string       `json:"id"`
	TenantID           string       `json:"tenant_id"`
	FromExperienceID   string       `json:"from_experience_id"`
	ToExperienceID     string       `json:"to_experience_id"`
	Type               RelationType `json:"type"`
	Confidence         float64      `json:"confidence"`
	Reason             string       `json:"reason,omitempty"`
	CreatedAt          time.Time    `json:"created_at"`
}

// RelationRepository persists experience relations.
type RelationRepository interface {
	Upsert(ctx context.Context, rel ExperienceRelation) (ExperienceRelation, error)
	ListByExperience(ctx context.Context, tenantID, experienceID string) ([]ExperienceRelation, error)
	// ConflictPeers returns experienceID → peerID for unresolved CONFLICTS edges
	// involving any of the given IDs (either side of the edge).
	ConflictPeers(ctx context.Context, tenantID string, experienceIDs []string) (map[string]string, error)
}
