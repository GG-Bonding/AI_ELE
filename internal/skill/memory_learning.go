package skill

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// MemoryLearningStore is an in-memory LearningStore for tests.
type MemoryLearningStore struct {
	mu     sync.Mutex
	byID   map[string]LearningEvent // tenant|id
	unique map[string]string        // tenant|feedback|version → id
	idSeq  int
}

// NewMemoryLearningStore constructs an empty learning event store.
func NewMemoryLearningStore() *MemoryLearningStore {
	return &MemoryLearningStore{
		byID:   map[string]LearningEvent{},
		unique: map[string]string{},
	}
}

func (m *MemoryLearningStore) key(tenantID, id string) string {
	return tenantID + "|" + id
}

func (m *MemoryLearningStore) uniqueKey(tenantID, feedbackID, versionID string) string {
	return tenantID + "|" + feedbackID + "|" + versionID
}

// CreateLearningEvent implements LearningStore.
func (m *MemoryLearningStore) CreateLearningEvent(_ context.Context, ev LearningEvent) (LearningEvent, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	uk := m.uniqueKey(ev.TenantID, ev.FeedbackID, ev.SkillVersionID)
	if _, ok := m.unique[uk]; ok {
		return LearningEvent{}, ErrDuplicateLearning
	}
	if ev.ID == "" {
		m.idSeq++
		ev.ID = fmt.Sprintf("sle_%d", m.idSeq)
	}
	if ev.CreatedAt.IsZero() {
		ev.CreatedAt = time.Now().UTC()
	}
	if ev.Status == "" {
		ev.Status = "PENDING"
	}
	m.byID[m.key(ev.TenantID, ev.ID)] = ev
	m.unique[uk] = ev.ID
	return ev, nil
}

// GetLearningEvent implements LearningStore.
func (m *MemoryLearningStore) GetLearningEvent(_ context.Context, tenantID, id string) (LearningEvent, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	ev, ok := m.byID[m.key(tenantID, id)]
	if !ok {
		return LearningEvent{}, ErrLearningNotFound
	}
	return ev, nil
}

// GetLearningEventByFeedbackVersion implements LearningStore.
func (m *MemoryLearningStore) GetLearningEventByFeedbackVersion(_ context.Context, tenantID, feedbackID, versionID string) (LearningEvent, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	id, ok := m.unique[m.uniqueKey(tenantID, feedbackID, versionID)]
	if !ok {
		return LearningEvent{}, ErrLearningNotFound
	}
	ev, ok := m.byID[m.key(tenantID, id)]
	if !ok {
		return LearningEvent{}, ErrLearningNotFound
	}
	return ev, nil
}

// MarkLearningApplied implements LearningStore.
func (m *MemoryLearningStore) MarkLearningApplied(_ context.Context, tenantID, id string, at time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := m.key(tenantID, id)
	ev, ok := m.byID[key]
	if !ok {
		return ErrLearningNotFound
	}
	ev.Status = "APPLIED"
	t := at
	ev.AppliedAt = &t
	m.byID[key] = ev
	return nil
}

// MarkLearningFailed implements LearningStore.
func (m *MemoryLearningStore) MarkLearningFailed(_ context.Context, tenantID, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := m.key(tenantID, id)
	ev, ok := m.byID[key]
	if !ok {
		return ErrLearningNotFound
	}
	ev.Status = "FAILED"
	m.byID[key] = ev
	return nil
}
