package experience

import (
	"context"
	"sort"
	"strings"
	"sync"
)

// MemoryRelationRepository is an in-memory RelationRepository for tests.
type MemoryRelationRepository struct {
	mu   sync.Mutex
	byID map[string]ExperienceRelation
	// uniqKey tenant|from|to|type → id
	uniq map[string]string
}

// NewMemoryRelationRepository constructs an empty relation store.
func NewMemoryRelationRepository() *MemoryRelationRepository {
	return &MemoryRelationRepository{
		byID: make(map[string]ExperienceRelation),
		uniq: make(map[string]string),
	}
}

func relationUniqKey(tenantID, fromID, toID string, typ RelationType) string {
	return tenantID + "|" + fromID + "|" + toID + "|" + string(typ)
}

func (m *MemoryRelationRepository) Upsert(_ context.Context, rel ExperienceRelation) (ExperienceRelation, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := relationUniqKey(rel.TenantID, rel.FromExperienceID, rel.ToExperienceID, rel.Type)
	if id, ok := m.uniq[key]; ok {
		existing := m.byID[id]
		existing.Confidence = rel.Confidence
		existing.Reason = rel.Reason
		m.byID[id] = existing
		return existing, nil
	}
	m.byID[rel.ID] = rel
	m.uniq[key] = rel.ID
	return rel, nil
}

func (m *MemoryRelationRepository) ListByExperience(_ context.Context, tenantID, experienceID string) ([]ExperienceRelation, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]ExperienceRelation, 0)
	for _, rel := range m.byID {
		if rel.TenantID != tenantID {
			continue
		}
		if rel.FromExperienceID == experienceID || rel.ToExperienceID == experienceID {
			out = append(out, rel)
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

func (m *MemoryRelationRepository) ConflictPeers(_ context.Context, tenantID string, experienceIDs []string) (map[string]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	want := make(map[string]struct{}, len(experienceIDs))
	for _, id := range experienceIDs {
		id = strings.TrimSpace(id)
		if id != "" {
			want[id] = struct{}{}
		}
	}
	out := make(map[string]string)
	for _, rel := range m.byID {
		if rel.TenantID != tenantID || rel.Type != RelationConflicts {
			continue
		}
		_, fromWanted := want[rel.FromExperienceID]
		_, toWanted := want[rel.ToExperienceID]
		if fromWanted {
			out[rel.FromExperienceID] = rel.ToExperienceID
		}
		if toWanted {
			out[rel.ToExperienceID] = rel.FromExperienceID
		}
	}
	return out, nil
}
