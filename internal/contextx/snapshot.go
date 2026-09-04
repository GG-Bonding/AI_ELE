package contextx

import (
	"context"
	"time"
)

// Snapshot is a persisted record of what entered an agent context build (V2.2-2).
// Actions can reference ContextID to auto-bind Experiences/Patterns as provenance.
type Snapshot struct {
	ID       string `json:"id"`
	TenantID string `json:"tenant_id"`

	EpisodeID string `json:"episode_id,omitempty"`
	AgentID   string `json:"agent_id,omitempty"`
	UserID    string `json:"user_id,omitempty"`
	Task      string `json:"task"`

	ExperienceIDs []string `json:"experience_ids"`
	PatternIDs    []string `json:"pattern_ids"`

	CreatedAt time.Time `json:"created_at"`
}

// SnapshotStore persists context snapshots for provenance.
type SnapshotStore interface {
	Create(ctx context.Context, snap Snapshot) (Snapshot, error)
	Get(ctx context.Context, tenantID, id string) (Snapshot, error)
}
