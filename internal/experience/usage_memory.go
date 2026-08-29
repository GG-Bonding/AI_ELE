package experience

import (
	"context"
	"sync"
)

// MemoryUsageRepository stores usages in memory for tests.
type MemoryUsageRepository struct {
	mu        sync.Mutex
	byID      map[string]Usage
	byEpisode map[string][]string
}

// NewMemoryUsageRepository constructs an empty usage store.
func NewMemoryUsageRepository() *MemoryUsageRepository {
	return &MemoryUsageRepository{
		byID:      make(map[string]Usage),
		byEpisode: make(map[string][]string),
	}
}

func usageKey(tenantID, id string) string { return tenantID + "/" + id }
func usageEpKey(tenantID, episodeID string) string {
	return tenantID + "/" + episodeID
}

func (m *MemoryUsageRepository) Create(_ context.Context, u Usage) (Usage, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.byID[usageKey(u.TenantID, u.ID)] = u
	ek := usageEpKey(u.TenantID, u.EpisodeID)
	m.byEpisode[ek] = append(m.byEpisode[ek], u.ID)
	return u, nil
}

func (m *MemoryUsageRepository) ListByEpisode(_ context.Context, tenantID, episodeID string) ([]Usage, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	ids := m.byEpisode[usageEpKey(tenantID, episodeID)]
	out := make([]Usage, 0, len(ids))
	for _, id := range ids {
		if u, ok := m.byID[usageKey(tenantID, id)]; ok {
			out = append(out, u)
		}
	}
	return out, nil
}
