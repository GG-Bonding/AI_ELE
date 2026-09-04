package action

import (
	"context"
	"sort"
	"sync"
)

// MemoryRepository is an in-memory Repository for unit tests.
type MemoryRepository struct {
	mu          sync.Mutex
	actions     map[string]AgentAction          // id → action
	byEp        map[string][]string             // tenant/episode → action ids
	links       map[string]ExperienceActionLink // id → link
	linkKeys    map[string]string               // tenant/exp/action → link id
	patLinks    map[string]PatternActionLink    // id → pattern link
	patLinkKeys map[string]string               // tenant/pat/action → link id
	seq         map[string]int                  // tenant/episode → last sequence
}

// NewMemoryRepository constructs an empty in-memory store.
func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{
		actions:     make(map[string]AgentAction),
		byEp:        make(map[string][]string),
		links:       make(map[string]ExperienceActionLink),
		linkKeys:    make(map[string]string),
		patLinks:    make(map[string]PatternActionLink),
		patLinkKeys: make(map[string]string),
		seq:         make(map[string]int),
	}
}

func epKey(tenantID, episodeID string) string {
	return tenantID + "/" + episodeID
}

func linkKey(tenantID, experienceID, actionID string) string {
	return tenantID + "/" + experienceID + "/" + actionID
}

func patLinkKey(tenantID, patternID, actionID string) string {
	return tenantID + "/" + patternID + "/" + actionID
}

func (m *MemoryRepository) CreateAction(_ context.Context, a AgentAction) (AgentAction, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.actions[a.ID] = a
	key := epKey(a.TenantID, a.EpisodeID)
	m.byEp[key] = append(m.byEp[key], a.ID)
	if a.Sequence > m.seq[key] {
		m.seq[key] = a.Sequence
	}
	return a, nil
}

func (m *MemoryRepository) GetAction(_ context.Context, tenantID, actionID string) (AgentAction, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	a, ok := m.actions[actionID]
	if !ok || a.TenantID != tenantID {
		return AgentAction{}, ErrNotFound
	}
	return a, nil
}

func (m *MemoryRepository) ListActionsByEpisode(_ context.Context, tenantID, episodeID string) ([]AgentAction, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	ids := m.byEp[epKey(tenantID, episodeID)]
	out := make([]AgentAction, 0, len(ids))
	for _, id := range ids {
		if a, ok := m.actions[id]; ok {
			out = append(out, a)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Sequence == out[j].Sequence {
			return out[i].ID < out[j].ID
		}
		return out[i].Sequence < out[j].Sequence
	})
	return out, nil
}

func (m *MemoryRepository) NextActionSequence(_ context.Context, tenantID, episodeID string) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := epKey(tenantID, episodeID)
	m.seq[key]++
	return m.seq[key], nil
}

func (m *MemoryRepository) CreateLink(_ context.Context, link ExperienceActionLink) (ExperienceActionLink, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := linkKey(link.TenantID, link.ExperienceID, link.ActionID)
	if _, exists := m.linkKeys[key]; exists {
		return ExperienceActionLink{}, ErrDuplicateLink
	}
	m.links[link.ID] = link
	m.linkKeys[key] = link.ID
	return link, nil
}

func (m *MemoryRepository) ListLinksByEpisode(_ context.Context, tenantID, episodeID string) ([]ExperienceActionLink, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]ExperienceActionLink, 0)
	for _, link := range m.links {
		if link.TenantID == tenantID && link.EpisodeID == episodeID {
			out = append(out, link)
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

func (m *MemoryRepository) ListLinksByAction(_ context.Context, tenantID, actionID string) ([]ExperienceActionLink, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]ExperienceActionLink, 0)
	for _, link := range m.links {
		if link.TenantID == tenantID && link.ActionID == actionID {
			out = append(out, link)
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

func (m *MemoryRepository) CreatePatternLink(_ context.Context, link PatternActionLink) (PatternActionLink, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := patLinkKey(link.TenantID, link.PatternID, link.ActionID)
	if _, exists := m.patLinkKeys[key]; exists {
		return PatternActionLink{}, ErrDuplicatePatternLink
	}
	m.patLinks[link.ID] = link
	m.patLinkKeys[key] = link.ID
	return link, nil
}

func (m *MemoryRepository) ListPatternLinksByEpisode(_ context.Context, tenantID, episodeID string) ([]PatternActionLink, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]PatternActionLink, 0)
	for _, link := range m.patLinks {
		if link.TenantID == tenantID && link.EpisodeID == episodeID {
			out = append(out, link)
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

func (m *MemoryRepository) ListPatternLinksByAction(_ context.Context, tenantID, actionID string) ([]PatternActionLink, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]PatternActionLink, 0)
	for _, link := range m.patLinks {
		if link.TenantID == tenantID && link.ActionID == actionID {
			out = append(out, link)
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
