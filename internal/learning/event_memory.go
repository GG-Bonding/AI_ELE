package learning

import (
	"context"
	"sync"
	"time"
)

// MemoryEventRepository is an in-memory LearningEvent store for tests.
type MemoryEventRepository struct {
	mu   sync.Mutex
	byID map[string]Event
	// tenant/feedback/experience -> id
	unique map[string]string
	// tenant/feedback -> ids
	byFeedback map[string][]string
}

// NewMemoryEventRepository constructs an empty store.
func NewMemoryEventRepository() *MemoryEventRepository {
	return &MemoryEventRepository{
		byID:       make(map[string]Event),
		unique:     make(map[string]string),
		byFeedback: make(map[string][]string),
	}
}

func eventUniqueKey(tenantID, feedbackID, experienceID string) string {
	return tenantID + "/" + feedbackID + "/" + experienceID
}

func (m *MemoryEventRepository) Create(_ context.Context, ev Event) (Event, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	uk := eventUniqueKey(ev.TenantID, ev.FeedbackID, ev.ExperienceID)
	if _, ok := m.unique[uk]; ok {
		return Event{}, ErrDuplicateEvent
	}
	m.byID[ev.TenantID+"/"+ev.ID] = ev
	m.unique[uk] = ev.ID
	fk := ev.TenantID + "/" + ev.FeedbackID
	m.byFeedback[fk] = append(m.byFeedback[fk], ev.ID)
	return ev, nil
}

func (m *MemoryEventRepository) GetByFeedbackExperience(_ context.Context, tenantID, feedbackID, experienceID string) (Event, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	id, ok := m.unique[eventUniqueKey(tenantID, feedbackID, experienceID)]
	if !ok {
		return Event{}, ErrEventNotFound
	}
	return m.byID[tenantID+"/"+id], nil
}

func (m *MemoryEventRepository) ListByFeedback(_ context.Context, tenantID, feedbackID string) ([]Event, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	ids := m.byFeedback[tenantID+"/"+feedbackID]
	out := make([]Event, 0, len(ids))
	for _, id := range ids {
		if ev, ok := m.byID[tenantID+"/"+id]; ok {
			out = append(out, ev)
		}
	}
	return out, nil
}

func (m *MemoryEventRepository) MarkApplied(_ context.Context, tenantID, id string, appliedAt time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	ev, ok := m.byID[tenantID+"/"+id]
	if !ok {
		return ErrEventNotFound
	}
	ev.Status = EventApplied
	t := appliedAt
	ev.AppliedAt = &t
	m.byID[tenantID+"/"+id] = ev
	return nil
}

func (m *MemoryEventRepository) MarkFailed(_ context.Context, tenantID, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	ev, ok := m.byID[tenantID+"/"+id]
	if !ok {
		return ErrEventNotFound
	}
	ev.Status = EventFailed
	m.byID[tenantID+"/"+id] = ev
	return nil
}
