package experience

import (
	"context"
	"sync"
)

// MemorySkillRepository is an in-memory SkillRepository for tests.
type MemorySkillRepository struct {
	mu        sync.Mutex
	byID      map[string]SkillCandidate
	byPattern map[string]string // tenantID|patternID → skillID
}

// NewMemorySkillRepository constructs an empty skill store.
func NewMemorySkillRepository() *MemorySkillRepository {
	return &MemorySkillRepository{
		byID:      make(map[string]SkillCandidate),
		byPattern: make(map[string]string),
	}
}

func (m *MemorySkillRepository) Create(_ context.Context, sk SkillCandidate) (SkillCandidate, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.byID[sk.ID] = sk
	m.byPattern[sk.TenantID+"|"+sk.PatternID] = sk.ID
	return sk, nil
}

func (m *MemorySkillRepository) Get(_ context.Context, tenantID, id string) (SkillCandidate, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	sk, ok := m.byID[id]
	if !ok || sk.TenantID != tenantID {
		return SkillCandidate{}, ErrNotFound
	}
	return sk, nil
}

func (m *MemorySkillRepository) FindByPattern(_ context.Context, tenantID, patternID string) (SkillCandidate, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	id, ok := m.byPattern[tenantID+"|"+patternID]
	if !ok {
		return SkillCandidate{}, ErrNotFound
	}
	sk, ok := m.byID[id]
	if !ok || sk.TenantID != tenantID {
		return SkillCandidate{}, ErrNotFound
	}
	return sk, nil
}
