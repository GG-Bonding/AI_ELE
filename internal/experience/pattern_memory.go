package experience

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
)

// MemoryPatternRepository is an in-memory PatternRepository for tests.
type MemoryPatternRepository struct {
	mu       sync.Mutex
	byID     map[string]Pattern
	evidence map[string][]PatternEvidence // patternID → evidence
	byExp    map[string][]string          // experienceID → patternIDs
}

// NewMemoryPatternRepository constructs an empty pattern store.
func NewMemoryPatternRepository() *MemoryPatternRepository {
	return &MemoryPatternRepository{
		byID:     make(map[string]Pattern),
		evidence: make(map[string][]PatternEvidence),
		byExp:    make(map[string][]string),
	}
}

func (m *MemoryPatternRepository) Create(_ context.Context, p Pattern) (Pattern, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	stored := p
	if p.Embedding != nil {
		stored.Embedding = append([]float32(nil), p.Embedding...)
	}
	m.byID[p.ID] = stored
	return stored, nil
}

func (m *MemoryPatternRepository) Update(_ context.Context, p Pattern) (Pattern, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	existing, ok := m.byID[p.ID]
	if !ok {
		return Pattern{}, ErrNotFound
	}
	stored := p
	// Preserve embedding unless caller supplies a new vector.
	if len(p.Embedding) == 0 && len(existing.Embedding) > 0 {
		stored.Embedding = append([]float32(nil), existing.Embedding...)
	} else if p.Embedding != nil {
		stored.Embedding = append([]float32(nil), p.Embedding...)
	}
	m.byID[p.ID] = stored
	return stored, nil
}

func (m *MemoryPatternRepository) Get(_ context.Context, tenantID, id string) (Pattern, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	p, ok := m.byID[id]
	if !ok || p.TenantID != tenantID {
		return Pattern{}, ErrNotFound
	}
	return clonePattern(p), nil
}

func (m *MemoryPatternRepository) AddEvidence(_ context.Context, ev PatternEvidence) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.byID[ev.PatternID]; !ok {
		return ErrNotFound
	}
	for _, existing := range m.evidence[ev.PatternID] {
		if existing.ExperienceID == ev.ExperienceID {
			return nil
		}
	}
	m.evidence[ev.PatternID] = append(m.evidence[ev.PatternID], ev)
	m.byExp[ev.ExperienceID] = append(m.byExp[ev.ExperienceID], ev.PatternID)
	return nil
}

func (m *MemoryPatternRepository) ListEvidence(_ context.Context, tenantID, patternID string) ([]PatternEvidence, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	p, ok := m.byID[patternID]
	if !ok || p.TenantID != tenantID {
		return nil, ErrNotFound
	}
	out := append([]PatternEvidence(nil), m.evidence[patternID]...)
	sort.Slice(out, func(i, j int) bool {
		if out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].ExperienceID < out[j].ExperienceID
		}
		return out[i].CreatedAt.Before(out[j].CreatedAt)
	})
	return out, nil
}

func (m *MemoryPatternRepository) FindByExperience(_ context.Context, tenantID string, experienceIDs []string) ([]Pattern, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	seen := make(map[string]struct{})
	var out []Pattern
	for _, expID := range experienceIDs {
		expID = strings.TrimSpace(expID)
		for _, pid := range m.byExp[expID] {
			if _, ok := seen[pid]; ok {
				continue
			}
			p, ok := m.byID[pid]
			if !ok || p.TenantID != tenantID {
				continue
			}
			seen[pid] = struct{}{}
			out = append(out, clonePattern(p))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (m *MemoryPatternRepository) List(_ context.Context, filter PatternListFilter) ([]Pattern, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	tenantID := strings.TrimSpace(filter.TenantID)
	if tenantID == "" {
		return nil, ErrInvalidInput
	}
	statusOK := statusSet(filter.Statuses)
	typeOK := typeSet(filter.Types)
	scopeOK := scopeSet(filter.Scopes)
	scopeKey := strings.TrimSpace(filter.ScopeKey)

	out := make([]Pattern, 0)
	for _, p := range m.byID {
		if p.TenantID != tenantID {
			continue
		}
		if len(statusOK) > 0 {
			if _, ok := statusOK[p.Status]; !ok {
				continue
			}
		}
		if len(typeOK) > 0 {
			if _, ok := typeOK[p.Type]; !ok {
				continue
			}
		}
		if len(scopeOK) > 0 {
			if _, ok := scopeOK[p.Scope]; !ok {
				continue
			}
		}
		if scopeKey != "" && p.ScopeKey != scopeKey {
			continue
		}
		out = append(out, clonePattern(p))
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Utility == out[j].Utility {
			return out[i].ID < out[j].ID
		}
		return out[i].Utility > out[j].Utility
	})
	if filter.Limit > 0 && len(out) > filter.Limit {
		out = out[:filter.Limit]
	}
	return out, nil
}

func (m *MemoryPatternRepository) Search(_ context.Context, filter PatternSearchFilter) ([]ScoredPattern, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	tenantID := strings.TrimSpace(filter.TenantID)
	if tenantID == "" {
		return nil, ErrInvalidInput
	}
	if len(filter.QueryEmbedding) == 0 {
		return nil, fmt.Errorf("%w: query embedding is required", ErrInvalidInput)
	}
	statuses := filter.Statuses
	if len(statuses) == 0 {
		statuses = []PatternStatus{PatternStatusActive}
	}
	statusOK := statusSet(statuses)
	topK := filter.TopK
	if topK <= 0 {
		topK = 20
	}

	type scored struct {
		p   Pattern
		sim float64
	}
	var hits []scored
	for _, p := range m.byID {
		if p.TenantID != tenantID {
			continue
		}
		if _, ok := statusOK[p.Status]; !ok {
			continue
		}
		if len(p.Embedding) == 0 {
			continue
		}
		authExp := Experience{Scope: p.Scope, ScopeKey: p.ScopeKey, TenantID: p.TenantID}
		if !AuthorizedForSearch(authExp, filter.AgentID, filter.UserID, filter.Tools, filter.ScopeKey) {
			continue
		}
		sim := CosineSimilarity(filter.QueryEmbedding, p.Embedding)
		hits = append(hits, scored{p: clonePattern(p), sim: sim})
	}
	sort.Slice(hits, func(i, j int) bool {
		if hits[i].sim == hits[j].sim {
			return hits[i].p.ID < hits[j].p.ID
		}
		return hits[i].sim > hits[j].sim
	})
	if len(hits) > topK {
		hits = hits[:topK]
	}
	out := make([]ScoredPattern, 0, len(hits))
	for _, h := range hits {
		out = append(out, ScoredPattern{Pattern: h.p, Similarity: h.sim})
	}
	return out, nil
}

func clonePattern(p Pattern) Pattern {
	out := p
	if p.Embedding != nil {
		out.Embedding = append([]float32(nil), p.Embedding...)
	}
	return out
}

func (m *MemoryPatternRepository) GetByFingerprint(_ context.Context, tenantID, fingerprint string) (Pattern, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	fp := strings.TrimSpace(fingerprint)
	if fp == "" {
		return Pattern{}, ErrNotFound
	}
	for _, p := range m.byID {
		if p.TenantID == tenantID && p.ClusterFingerprint == fp {
			return clonePattern(p), nil
		}
	}
	return Pattern{}, ErrNotFound
}

func statusSet(ss []PatternStatus) map[PatternStatus]struct{} {
	if len(ss) == 0 {
		return nil
	}
	out := make(map[PatternStatus]struct{}, len(ss))
	for _, s := range ss {
		out[s] = struct{}{}
	}
	return out
}

func typeSet(ts []Type) map[Type]struct{} {
	if len(ts) == 0 {
		return nil
	}
	out := make(map[Type]struct{}, len(ts))
	for _, t := range ts {
		out[t] = struct{}{}
	}
	return out
}

func scopeSet(ss []Scope) map[Scope]struct{} {
	if len(ss) == 0 {
		return nil
	}
	out := make(map[Scope]struct{}, len(ss))
	for _, s := range ss {
		out[s] = struct{}{}
	}
	return out
}
