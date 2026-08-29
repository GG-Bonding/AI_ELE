package episode

import (
	"context"
	"sync"

	"github.com/agent-experience-engine/agent-experience-engine/internal/attempt"
	"github.com/agent-experience-engine/agent-experience-engine/internal/outcome"
)

// MemoryRepository is an in-memory Repository for unit tests.
type MemoryRepository struct {
	mu        sync.Mutex
	episodes  map[string]Episode
	attempts  map[string][]attempt.Attempt // key: tenantID + "/" + episodeID
	outcomes  map[string]outcome.Outcome   // key: tenantID + "/" + episodeID
	sequences map[string]int
}

// NewMemoryRepository constructs an empty in-memory store.
func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{
		episodes:  make(map[string]Episode),
		attempts:  make(map[string][]attempt.Attempt),
		outcomes:  make(map[string]outcome.Outcome),
		sequences: make(map[string]int),
	}
}

func tenantKey(tenantID, id string) string {
	return tenantID + "/" + id
}

func (m *MemoryRepository) CreateEpisode(_ context.Context, ep Episode) (Episode, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.episodes[tenantKey(ep.TenantID, ep.ID)] = ep
	return ep, nil
}

func (m *MemoryRepository) GetEpisode(_ context.Context, tenantID, id string) (Episode, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	ep, ok := m.episodes[tenantKey(tenantID, id)]
	if !ok {
		return Episode{}, ErrNotFound
	}
	return ep, nil
}

func (m *MemoryRepository) UpdateEpisode(_ context.Context, ep Episode) (Episode, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := tenantKey(ep.TenantID, ep.ID)
	if _, ok := m.episodes[key]; !ok {
		return Episode{}, ErrNotFound
	}
	m.episodes[key] = ep
	return ep, nil
}

func (m *MemoryRepository) CreateAttempt(_ context.Context, a attempt.Attempt) (attempt.Attempt, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := tenantKey(a.TenantID, a.EpisodeID)
	if _, ok := m.episodes[key]; !ok {
		return attempt.Attempt{}, ErrNotFound
	}
	m.attempts[key] = append(m.attempts[key], a)
	if a.Sequence > m.sequences[key] {
		m.sequences[key] = a.Sequence
	}
	return a, nil
}

func (m *MemoryRepository) ListAttempts(_ context.Context, tenantID, episodeID string) ([]attempt.Attempt, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := tenantKey(tenantID, episodeID)
	if _, ok := m.episodes[key]; !ok {
		return nil, ErrNotFound
	}
	src := m.attempts[key]
	out := make([]attempt.Attempt, len(src))
	copy(out, src)
	return out, nil
}

func (m *MemoryRepository) NextAttemptSequence(_ context.Context, tenantID, episodeID string) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := tenantKey(tenantID, episodeID)
	if _, ok := m.episodes[key]; !ok {
		return 0, ErrNotFound
	}
	m.sequences[key]++
	return m.sequences[key], nil
}

func (m *MemoryRepository) CreateOutcome(_ context.Context, o outcome.Outcome) (outcome.Outcome, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := tenantKey(o.TenantID, o.EpisodeID)
	if _, ok := m.episodes[key]; !ok {
		return outcome.Outcome{}, ErrNotFound
	}
	if _, exists := m.outcomes[key]; exists {
		return outcome.Outcome{}, ErrOutcomeExists
	}
	m.outcomes[key] = o
	return o, nil
}

func (m *MemoryRepository) GetOutcome(_ context.Context, tenantID, episodeID string) (outcome.Outcome, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	o, ok := m.outcomes[tenantKey(tenantID, episodeID)]
	if !ok {
		return outcome.Outcome{}, ErrNotFound
	}
	return o, nil
}
