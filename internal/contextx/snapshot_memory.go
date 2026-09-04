package contextx

import (
	"context"
	"errors"
	"strings"
	"sync"
)

// ErrSnapshotNotFound is returned when a context snapshot is missing.
var ErrSnapshotNotFound = errors.New("context snapshot not found")

// MemorySnapshotStore is an in-memory SnapshotStore for tests.
type MemorySnapshotStore struct {
	mu   sync.Mutex
	byID map[string]Snapshot
}

// NewMemorySnapshotStore constructs an empty snapshot store.
func NewMemorySnapshotStore() *MemorySnapshotStore {
	return &MemorySnapshotStore{byID: make(map[string]Snapshot)}
}

func (m *MemorySnapshotStore) Create(_ context.Context, snap Snapshot) (Snapshot, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	stored := snap
	stored.ExperienceIDs = append([]string(nil), snap.ExperienceIDs...)
	stored.PatternIDs = append([]string(nil), snap.PatternIDs...)
	m.byID[snap.ID] = stored
	return stored, nil
}

func (m *MemorySnapshotStore) Get(_ context.Context, tenantID, id string) (Snapshot, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	snap, ok := m.byID[strings.TrimSpace(id)]
	if !ok || snap.TenantID != strings.TrimSpace(tenantID) {
		return Snapshot{}, ErrSnapshotNotFound
	}
	out := snap
	out.ExperienceIDs = append([]string(nil), snap.ExperienceIDs...)
	out.PatternIDs = append([]string(nil), snap.PatternIDs...)
	return out, nil
}
