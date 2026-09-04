package evolutionjob

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/agent-experience-engine/agent-experience-engine/internal/experience"
	"github.com/google/uuid"
)

// MemoryRepository is an in-memory Repository for tests.
type MemoryRepository struct {
	mu    sync.Mutex
	dirty map[string]DirtyGroup
	jobs  map[string]Job
	now   func() time.Time
	id    func() string
}

// NewMemoryRepository constructs an empty store.
func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{
		dirty: make(map[string]DirtyGroup),
		jobs:  make(map[string]Job),
		now:   time.Now().UTC,
		id:    func() string { return uuid.NewString() },
	}
}

func familyKey(tenantID string, typ experience.Type, scope experience.Scope, scopeKey string) string {
	return tenantID + "|" + string(typ) + "|" + string(scope) + "|" + scopeKey
}

func (m *MemoryRepository) MarkDirty(_ context.Context, tenantID string, typ experience.Type, scope experience.Scope, scopeKey string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	tenantID = strings.TrimSpace(tenantID)
	if tenantID == "" {
		return fmt.Errorf("tenant_id is required")
	}
	key := familyKey(tenantID, typ, scope, scopeKey)
	m.dirty[key] = DirtyGroup{
		TenantID:  tenantID,
		Type:      typ,
		Scope:     scope,
		ScopeKey:  scopeKey,
		UpdatedAt: m.now(),
	}
	return nil
}

func (m *MemoryRepository) ListDirty(_ context.Context, limit int) ([]DirtyGroup, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]DirtyGroup, 0, len(m.dirty))
	for _, g := range m.dirty {
		out = append(out, g)
	}
	// oldest first
	for i := 0; i < len(out); i++ {
		for j := i + 1; j < len(out); j++ {
			if out[j].UpdatedAt.Before(out[i].UpdatedAt) {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (m *MemoryRepository) ClearDirty(_ context.Context, g DirtyGroup) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.dirty, familyKey(g.TenantID, g.Type, g.Scope, g.ScopeKey))
	return nil
}

func (m *MemoryRepository) UpsertPending(_ context.Context, g DirtyGroup) (Job, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := familyKey(g.TenantID, g.Type, g.Scope, g.ScopeKey)
	if existing, ok := m.jobs[key]; ok {
		if existing.Status == StatusApplied || existing.Status == StatusFailed || existing.Status == StatusPending {
			existing.Status = StatusPending
			existing.LastError = ""
			existing.UpdatedAt = m.now()
			m.jobs[key] = existing
			return existing, nil
		}
		// PROCESSING: leave as-is but return current
		return existing, nil
	}
	now := m.now()
	job := Job{
		ID:        m.id(),
		TenantID:  g.TenantID,
		Type:      g.Type,
		Scope:     g.Scope,
		ScopeKey:  g.ScopeKey,
		Status:    StatusPending,
		CreatedAt: now,
		UpdatedAt: now,
	}
	m.jobs[key] = job
	return job, nil
}

func (m *MemoryRepository) GetByFamily(_ context.Context, tenantID string, typ experience.Type, scope experience.Scope, scopeKey string) (Job, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	job, ok := m.jobs[familyKey(tenantID, typ, scope, scopeKey)]
	if !ok {
		return Job{}, ErrNotFound
	}
	return job, nil
}

func (m *MemoryRepository) MarkProcessing(_ context.Context, tenantID string, typ experience.Type, scope experience.Scope, scopeKey string) error {
	return m.setStatus(tenantID, typ, scope, scopeKey, StatusProcessing, "", -1)
}

func (m *MemoryRepository) MarkApplied(_ context.Context, tenantID string, typ experience.Type, scope experience.Scope, scopeKey string, createdCount int) error {
	return m.setStatus(tenantID, typ, scope, scopeKey, StatusApplied, "", createdCount)
}

func (m *MemoryRepository) MarkFailed(_ context.Context, tenantID string, typ experience.Type, scope experience.Scope, scopeKey, lastError string) error {
	return m.setStatus(tenantID, typ, scope, scopeKey, StatusFailed, lastError, -1)
}

func (m *MemoryRepository) setStatus(tenantID string, typ experience.Type, scope experience.Scope, scopeKey string, status Status, lastError string, createdCount int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := familyKey(tenantID, typ, scope, scopeKey)
	job, ok := m.jobs[key]
	if !ok {
		return ErrNotFound
	}
	job.Status = status
	job.LastError = lastError
	job.UpdatedAt = m.now()
	if createdCount >= 0 {
		job.CreatedCount = createdCount
	}
	m.jobs[key] = job
	return nil
}

func (m *MemoryRepository) ListStaleProcessing(_ context.Context, cutoff time.Time) ([]Job, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []Job
	for _, job := range m.jobs {
		if job.Status == StatusProcessing && job.UpdatedAt.Before(cutoff) {
			out = append(out, job)
		}
	}
	return out, nil
}

// BackdateUpdatedAt is a test helper to simulate a stuck PROCESSING job.
func (m *MemoryRepository) BackdateUpdatedAt(_ context.Context, tenantID string, typ experience.Type, scope experience.Scope, scopeKey string, at time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := familyKey(tenantID, typ, scope, scopeKey)
	job, ok := m.jobs[key]
	if !ok {
		return ErrNotFound
	}
	job.UpdatedAt = at
	m.jobs[key] = job
	return nil
}
