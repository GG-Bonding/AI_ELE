package feedback

import (
	"context"
	"strings"
	"sync"
)

// MemoryRepository is an in-memory feedback store for tests.
type MemoryRepository struct {
	mu   sync.Mutex
	byID map[string]Feedback
	// tenant/idempotency_key -> id (only non-empty keys)
	byIdem map[string]string
	// key: tenant/episode -> ids in insertion order
	byEpisode map[string][]string
}

// NewMemoryRepository constructs an empty store.
func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{
		byID:      make(map[string]Feedback),
		byIdem:    make(map[string]string),
		byEpisode: make(map[string][]string),
	}
}

func epKey(tenantID, episodeID string) string { return tenantID + "/" + episodeID }

func (m *MemoryRepository) Create(_ context.Context, fb Feedback) (Feedback, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if key := strings.TrimSpace(fb.IdempotencyKey); key != "" {
		ik := fb.TenantID + "/" + key
		if _, ok := m.byIdem[ik]; ok {
			return Feedback{}, ErrDuplicateIdempotency
		}
		m.byIdem[ik] = fb.ID
	}
	m.byID[fb.TenantID+"/"+fb.ID] = fb
	k := epKey(fb.TenantID, fb.EpisodeID)
	m.byEpisode[k] = append(m.byEpisode[k], fb.ID)
	return fb, nil
}

func (m *MemoryRepository) GetByIdempotencyKey(_ context.Context, tenantID, key string) (Feedback, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key = strings.TrimSpace(key)
	if key == "" {
		return Feedback{}, ErrNotFound
	}
	id, ok := m.byIdem[tenantID+"/"+key]
	if !ok {
		return Feedback{}, ErrNotFound
	}
	fb, ok := m.byID[tenantID+"/"+id]
	if !ok {
		return Feedback{}, ErrNotFound
	}
	return fb, nil
}

func (m *MemoryRepository) ListByEpisode(_ context.Context, tenantID, episodeID string) ([]Feedback, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	ids := m.byEpisode[epKey(tenantID, episodeID)]
	out := make([]Feedback, 0, len(ids))
	for _, id := range ids {
		if fb, ok := m.byID[tenantID+"/"+id]; ok {
			out = append(out, fb)
		}
	}
	return out, nil
}

func (m *MemoryRepository) Get(_ context.Context, tenantID, id string) (Feedback, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	fb, ok := m.byID[tenantID+"/"+id]
	if !ok {
		return Feedback{}, ErrNotFound
	}
	return fb, nil
}
