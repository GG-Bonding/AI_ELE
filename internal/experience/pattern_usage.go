package experience

import (
	"context"
	"time"
)

// PatternUsage records that a Pattern entered agent context for an episode (V2.1-3).
// Enables direct Pattern attribution — not only via supporting Experience feedback.
type PatternUsage struct {
	ID             string    `json:"id"`
	TenantID       string    `json:"tenant_id"`
	EpisodeID      string    `json:"episode_id"`
	PatternID      string    `json:"pattern_id"`
	RetrievalScore float64   `json:"retrieval_score"`
	FinalScore     float64   `json:"final_score"`
	UsedAt         time.Time `json:"used_at"`
}

// PatternUsageRepository persists pattern usage rows for attribution.
type PatternUsageRepository interface {
	Create(ctx context.Context, u PatternUsage) (PatternUsage, error)
	ListByEpisode(ctx context.Context, tenantID, episodeID string) ([]PatternUsage, error)
}
