package experience

import (
	"context"
	"math"
	"sort"
	"sync"
)

// MemoryRepository is an in-memory Experience store for unit tests.
type MemoryRepository struct {
	mu   sync.Mutex
	data map[string]Experience // tenantID/id
}

// NewMemoryRepository constructs an empty store.
func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{data: make(map[string]Experience)}
}

func key(tenantID, id string) string { return tenantID + "/" + id }

func (m *MemoryRepository) Create(_ context.Context, exp Experience) (Experience, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data[key(exp.TenantID, exp.ID)] = cloneExperience(exp)
	return cloneExperience(exp), nil
}

func (m *MemoryRepository) Update(_ context.Context, exp Experience) (Experience, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	k := key(exp.TenantID, exp.ID)
	cur, ok := m.data[k]
	if !ok {
		return Experience{}, ErrNotFound
	}
	// Optimistic lock: caller passes the version it read; we bump on success.
	if cur.Version != exp.Version {
		return Experience{}, ErrConflict
	}
	exp.Version = cur.Version + 1
	m.data[k] = cloneExperience(exp)
	return cloneExperience(exp), nil
}

func (m *MemoryRepository) Get(_ context.Context, tenantID, id string) (Experience, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	exp, ok := m.data[key(tenantID, id)]
	if !ok {
		return Experience{}, ErrNotFound
	}
	return cloneExperience(exp), nil
}

func (m *MemoryRepository) Search(_ context.Context, filter SearchFilter) ([]ScoredExperience, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	statuses := filter.Statuses
	if len(statuses) == 0 {
		statuses = []Status{StatusActive, StatusCandidate}
	}
	statusSet := map[Status]struct{}{}
	for _, s := range statuses {
		statusSet[s] = struct{}{}
	}

	var scored []ScoredExperience
	for _, exp := range m.data {
		if exp.TenantID != filter.TenantID {
			continue
		}
		if _, ok := statusSet[exp.Status]; !ok {
			continue
		}
		if len(filter.Types) > 0 && !containsType(filter.Types, exp.Type) {
			continue
		}
		if len(filter.Scopes) > 0 && !containsScope(filter.Scopes, exp.Scope) {
			continue
		}
		if filter.ScopeKey != "" && exp.ScopeKey != filter.ScopeKey {
			continue
		}
		sim := cosineSimilarity(filter.QueryEmbedding, exp.Embedding)
		scored = append(scored, ScoredExperience{Experience: cloneExperience(exp), Similarity: sim})
	}

	sort.Slice(scored, func(i, j int) bool {
		return scored[i].Similarity > scored[j].Similarity
	})
	if filter.TopK > 0 && len(scored) > filter.TopK {
		scored = scored[:filter.TopK]
	}
	return scored, nil
}

func (m *MemoryRepository) Supersede(_ context.Context, tenantID, oldID, newID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	oldKey := key(tenantID, oldID)
	newKey := key(tenantID, newID)
	oldExp, ok := m.data[oldKey]
	if !ok {
		return ErrNotFound
	}
	newExp, ok := m.data[newKey]
	if !ok {
		return ErrNotFound
	}
	oldExp.Status = StatusDeprecated
	sid := oldID
	newExp.SupersedesID = &sid
	m.data[oldKey] = oldExp
	m.data[newKey] = newExp
	return nil
}

func (m *MemoryRepository) Archive(_ context.Context, tenantID, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	k := key(tenantID, id)
	exp, ok := m.data[k]
	if !ok {
		return ErrNotFound
	}
	exp.Status = StatusArchived
	m.data[k] = exp
	return nil
}

func containsType(list []Type, v Type) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

func containsScope(list []Scope, v Scope) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

func cosineSimilarity(a, b []float32) float64 {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	if n == 0 {
		return 0
	}
	var dot, na, nb float64
	for i := 0; i < n; i++ {
		dot += float64(a[i]) * float64(b[i])
		na += float64(a[i]) * float64(a[i])
		nb += float64(b[i]) * float64(b[i])
	}
	if na == 0 || nb == 0 {
		return 0
	}
	return dot / (math.Sqrt(na) * math.Sqrt(nb))
}

func cloneExperience(exp Experience) Experience {
	out := exp
	if exp.Embedding != nil {
		out.Embedding = append([]float32(nil), exp.Embedding...)
	}
	if exp.SupersedesID != nil {
		id := *exp.SupersedesID
		out.SupersedesID = &id
	}
	if exp.LastUsedAt != nil {
		t := *exp.LastUsedAt
		out.LastUsedAt = &t
	}
	if exp.Evidence.AttemptIDs != nil {
		out.Evidence.AttemptIDs = append([]string(nil), exp.Evidence.AttemptIDs...)
	}
	return out
}
