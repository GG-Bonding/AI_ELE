package experience

import (
	"context"
	"sync"
)

// MemoryPatternUsageRepository stores pattern usages in memory for tests.
type MemoryPatternUsageRepository struct {
	mu        sync.Mutex
	byID      map[string]PatternUsage
	byEpisode map[string][]string
}

// NewMemoryPatternUsageRepository constructs an empty pattern usage store.
func NewMemoryPatternUsageRepository() *MemoryPatternUsageRepository {
	return &MemoryPatternUsageRepository{
		byID:      make(map[string]PatternUsage),
		byEpisode: make(map[string][]string),
	}
}

func patternUsageKey(tenantID, id string) string { return tenantID + "/" + id }
func patternUsageEpKey(tenantID, episodeID string) string {
	return tenantID + "/" + episodeID
}

func (m *MemoryPatternUsageRepository) Create(_ context.Context, u PatternUsage) (PatternUsage, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.byID[patternUsageKey(u.TenantID, u.ID)] = u
	ek := patternUsageEpKey(u.TenantID, u.EpisodeID)
	m.byEpisode[ek] = append(m.byEpisode[ek], u.ID)
	return u, nil
}

func (m *MemoryPatternUsageRepository) ListByEpisode(_ context.Context, tenantID, episodeID string) ([]PatternUsage, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	ids := m.byEpisode[patternUsageEpKey(tenantID, episodeID)]
	out := make([]PatternUsage, 0, len(ids))
	for _, id := range ids {
		if u, ok := m.byID[patternUsageKey(tenantID, id)]; ok {
			out = append(out, u)
		}
	}
	return out, nil
}
