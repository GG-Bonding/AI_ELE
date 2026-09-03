package experience

import "context"

// Repository persists experiences. No business logic beyond tenant-scoped CRUD/search.
type Repository interface {
	Create(ctx context.Context, exp Experience) (Experience, error)
	Update(ctx context.Context, exp Experience) (Experience, error)
	Get(ctx context.Context, tenantID, id string) (Experience, error)
	GetByEpisodeDedup(ctx context.Context, tenantID, episodeID, dedupKey string) (Experience, error)
	Search(ctx context.Context, filter SearchFilter) ([]ScoredExperience, error)
	// List returns experiences matching metadata filters (includes embeddings for evolution scans).
	List(ctx context.Context, filter ListFilter) ([]Experience, error)
	Supersede(ctx context.Context, tenantID, oldID, newID string) error
	Archive(ctx context.Context, tenantID, id string) error
}
