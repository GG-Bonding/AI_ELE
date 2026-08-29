package experience

import (
	"context"
	"time"
)

// Usage links an episode to experiences that entered agent context.
type Usage struct {
	ID                string    `json:"id"`
	TenantID          string    `json:"tenant_id"`
	EpisodeID         string    `json:"episode_id"`
	ExperienceID      string    `json:"experience_id"`
	RetrievalScore    float64   `json:"retrieval_score"`
	SelectionDecision string    `json:"selection_decision"`
	FinalScore        float64   `json:"final_score"`
	UsedAt            time.Time `json:"used_at"`
}

// UsageRepository persists experience usage rows for attribution.
type UsageRepository interface {
	Create(ctx context.Context, u Usage) (Usage, error)
	ListByEpisode(ctx context.Context, tenantID, episodeID string) ([]Usage, error)
}
