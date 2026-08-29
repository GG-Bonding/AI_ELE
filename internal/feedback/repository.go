package feedback

import "context"

// Repository persists raw feedback rows (no business aggregation logic).
type Repository interface {
	Create(ctx context.Context, fb Feedback) (Feedback, error)
	ListByEpisode(ctx context.Context, tenantID, episodeID string) ([]Feedback, error)
	Get(ctx context.Context, tenantID, id string) (Feedback, error)
}
