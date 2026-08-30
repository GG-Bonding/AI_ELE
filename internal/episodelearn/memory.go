package episodelearn

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"
)

// MemoryRepository is an in-memory episode learning job store.
type MemoryRepository struct {
	mu   sync.Mutex
	jobs map[string]Job // tenantID/episodeID
	now  func() time.Time
	id   func() string
}

// NewMemoryRepository constructs an empty store.
func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{
		jobs: make(map[string]Job),
		now:  func() time.Time { return time.Now().UTC() },
		id:   func() string { return uuid.NewString() },
	}
}

func jobKey(tenantID, episodeID string) string { return tenantID + "/" + episodeID }

func (m *MemoryRepository) UpsertPending(_ context.Context, tenantID, episodeID string) (Job, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	k := jobKey(tenantID, episodeID)
	if existing, ok := m.jobs[k]; ok {
		if existing.Status == StatusApplied {
			return existing, nil
		}
		existing.Status = StatusPending
		existing.LastError = ""
		existing.UpdatedAt = m.now()
		m.jobs[k] = existing
		return existing, nil
	}
	now := m.now()
	job := Job{
		ID:        m.id(),
		TenantID:  tenantID,
		EpisodeID: episodeID,
		Status:    StatusPending,
		CreatedAt: now,
		UpdatedAt: now,
	}
	m.jobs[k] = job
	return job, nil
}

func (m *MemoryRepository) GetByEpisode(_ context.Context, tenantID, episodeID string) (Job, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	job, ok := m.jobs[jobKey(tenantID, episodeID)]
	if !ok {
		return Job{}, ErrNotFound
	}
	return job, nil
}

func (m *MemoryRepository) MarkProcessing(_ context.Context, tenantID, episodeID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	k := jobKey(tenantID, episodeID)
	job, ok := m.jobs[k]
	if !ok {
		return ErrNotFound
	}
	job.Status = StatusProcessing
	job.UpdatedAt = m.now()
	m.jobs[k] = job
	return nil
}

func (m *MemoryRepository) MarkApplied(_ context.Context, tenantID, episodeID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	k := jobKey(tenantID, episodeID)
	job, ok := m.jobs[k]
	if !ok {
		return ErrNotFound
	}
	job.Status = StatusApplied
	job.LastError = ""
	job.UpdatedAt = m.now()
	m.jobs[k] = job
	return nil
}

func (m *MemoryRepository) MarkFailed(_ context.Context, tenantID, episodeID, lastError string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	k := jobKey(tenantID, episodeID)
	job, ok := m.jobs[k]
	if !ok {
		return ErrNotFound
	}
	job.Status = StatusFailed
	job.LastError = lastError
	job.UpdatedAt = m.now()
	m.jobs[k] = job
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
