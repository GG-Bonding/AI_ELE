package learning

import (
	"context"
	"sort"
	"sync"
	"time"
)

// MemoryPatternEventRepository is an in-memory PatternEvent store for tests.
type MemoryPatternEventRepository struct {
	mu         sync.Mutex
	byID       map[string]PatternEvent
	byFeedback map[string][]string // tenant/feedback → ids
	memberKey  map[string]string   // tenant/sourceEvent/pattern → id
	usageKey   map[string]string   // tenant/feedback/pattern/source → id
}

// NewMemoryPatternEventRepository constructs an empty store.
func NewMemoryPatternEventRepository() *MemoryPatternEventRepository {
	return &MemoryPatternEventRepository{
		byID:       make(map[string]PatternEvent),
		byFeedback: make(map[string][]string),
		memberKey:  make(map[string]string),
		usageKey:   make(map[string]string),
	}
}

func patternFeedbackKey(tenantID, feedbackID string) string {
	return tenantID + "/" + feedbackID
}

func memberSourceKey(tenantID, sourceEventID, patternID string) string {
	return tenantID + "/" + sourceEventID + "/" + patternID
}

func feedbackPatternSourceKey(tenantID, feedbackID, patternID string, source PatternEventSource) string {
	return tenantID + "/" + feedbackID + "/" + patternID + "/" + string(source)
}

func (m *MemoryPatternEventRepository) Create(_ context.Context, ev PatternEvent) (PatternEvent, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	switch ev.SourceType {
	case PatternSourceMember:
		k := memberSourceKey(ev.TenantID, ev.SourceLearningEventID, ev.PatternID)
		if _, ok := m.memberKey[k]; ok {
			return PatternEvent{}, ErrDuplicatePatternEvent
		}
		m.memberKey[k] = ev.ID
	default:
		k := feedbackPatternSourceKey(ev.TenantID, ev.FeedbackID, ev.PatternID, ev.SourceType)
		if _, ok := m.usageKey[k]; ok {
			return PatternEvent{}, ErrDuplicatePatternEvent
		}
		m.usageKey[k] = ev.ID
	}
	m.byID[ev.ID] = ev
	fk := patternFeedbackKey(ev.TenantID, ev.FeedbackID)
	m.byFeedback[fk] = append(m.byFeedback[fk], ev.ID)
	return ev, nil
}

func (m *MemoryPatternEventRepository) GetByMemberSource(_ context.Context, tenantID, sourceLearningEventID, patternID string) (PatternEvent, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	id, ok := m.memberKey[memberSourceKey(tenantID, sourceLearningEventID, patternID)]
	if !ok {
		return PatternEvent{}, ErrPatternEventNotFound
	}
	return m.byID[id], nil
}

func (m *MemoryPatternEventRepository) GetByFeedbackPatternSource(_ context.Context, tenantID, feedbackID, patternID string, source PatternEventSource) (PatternEvent, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	id, ok := m.usageKey[feedbackPatternSourceKey(tenantID, feedbackID, patternID, source)]
	if !ok {
		return PatternEvent{}, ErrPatternEventNotFound
	}
	return m.byID[id], nil
}

func (m *MemoryPatternEventRepository) ListByFeedback(_ context.Context, tenantID, feedbackID string) ([]PatternEvent, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	ids := append([]string(nil), m.byFeedback[patternFeedbackKey(tenantID, feedbackID)]...)
	out := make([]PatternEvent, 0, len(ids))
	for _, id := range ids {
		if ev, ok := m.byID[id]; ok {
			out = append(out, ev)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].ID < out[j].ID
		}
		return out[i].CreatedAt.Before(out[j].CreatedAt)
	})
	return out, nil
}

func (m *MemoryPatternEventRepository) MarkApplied(_ context.Context, tenantID, id string, appliedAt time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	ev, ok := m.byID[id]
	if !ok || ev.TenantID != tenantID {
		return ErrPatternEventNotFound
	}
	ev.Status = EventApplied
	t := appliedAt
	ev.AppliedAt = &t
	m.byID[id] = ev
	return nil
}

func (m *MemoryPatternEventRepository) MarkFailed(_ context.Context, tenantID, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	ev, ok := m.byID[id]
	if !ok || ev.TenantID != tenantID {
		return ErrPatternEventNotFound
	}
	ev.Status = EventFailed
	m.byID[id] = ev
	return nil
}
