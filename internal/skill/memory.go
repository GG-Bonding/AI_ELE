package skill

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
)

// MemoryRepository is an in-memory Skill store for tests.
type MemoryRepository struct {
	mu       sync.Mutex
	skills   map[string]Skill            // tenant|id
	byName   map[string]string           // tenant|name → id
	versions map[string]Version          // tenant|id
	byNum    map[string]string           // tenant|skill|version → id
	now      func() time.Time
	idSeq    int
}

// NewMemoryRepository constructs an empty memory store.
func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{
		skills:   map[string]Skill{},
		byName:   map[string]string{},
		versions: map[string]Version{},
		byNum:    map[string]string{},
		now:      func() time.Time { return time.Now().UTC() },
	}
}

func (m *MemoryRepository) key(tenantID, id string) string {
	return tenantID + "|" + id
}

func (m *MemoryRepository) nextID(prefix string) string {
	m.idSeq++
	return fmt.Sprintf("%s_%d", prefix, m.idSeq)
}

// CreateSkill implements Repository.
func (m *MemoryRepository) CreateSkill(ctx context.Context, sk Skill) (Skill, error) {
	_ = ctx
	m.mu.Lock()
	defer m.mu.Unlock()
	sk.TenantID = strings.TrimSpace(sk.TenantID)
	sk.Name = strings.TrimSpace(sk.Name)
	if sk.TenantID == "" || sk.Name == "" {
		return Skill{}, fmt.Errorf("%w: tenant_id and name are required", ErrInvalidInput)
	}
	nameKey := m.key(sk.TenantID, sk.Name)
	if _, ok := m.byName[nameKey]; ok {
		return Skill{}, fmt.Errorf("%w: skill name %q already exists", ErrConflict, sk.Name)
	}
	if sk.ID == "" {
		sk.ID = m.nextID("sk")
	}
	if !sk.Status.Valid() {
		sk.Status = StatusCandidate
	}
	now := m.now()
	if sk.CreatedAt.IsZero() {
		sk.CreatedAt = now
	}
	sk.UpdatedAt = now
	m.skills[m.key(sk.TenantID, sk.ID)] = sk
	m.byName[nameKey] = sk.ID
	return sk, nil
}

// GetSkill implements Repository.
func (m *MemoryRepository) GetSkill(ctx context.Context, tenantID, id string) (Skill, error) {
	_ = ctx
	m.mu.Lock()
	defer m.mu.Unlock()
	sk, ok := m.skills[m.key(tenantID, id)]
	if !ok {
		return Skill{}, ErrNotFound
	}
	return sk, nil
}

// GetSkillByName implements Repository.
func (m *MemoryRepository) GetSkillByName(ctx context.Context, tenantID, name string) (Skill, error) {
	_ = ctx
	m.mu.Lock()
	defer m.mu.Unlock()
	id, ok := m.byName[m.key(tenantID, strings.TrimSpace(name))]
	if !ok {
		return Skill{}, ErrNotFound
	}
	return m.skills[m.key(tenantID, id)], nil
}

// UpdateSkill implements Repository.
func (m *MemoryRepository) UpdateSkill(ctx context.Context, sk Skill) (Skill, error) {
	_ = ctx
	m.mu.Lock()
	defer m.mu.Unlock()
	key := m.key(sk.TenantID, sk.ID)
	existing, ok := m.skills[key]
	if !ok {
		return Skill{}, ErrNotFound
	}
	sk.CreatedAt = existing.CreatedAt
	sk.UpdatedAt = m.now()
	m.skills[key] = sk
	return sk, nil
}

// CreateVersion implements Repository.
func (m *MemoryRepository) CreateVersion(ctx context.Context, ver Version) (Version, error) {
	_ = ctx
	m.mu.Lock()
	defer m.mu.Unlock()
	ver.TenantID = strings.TrimSpace(ver.TenantID)
	ver.SkillID = strings.TrimSpace(ver.SkillID)
	if ver.TenantID == "" || ver.SkillID == "" {
		return Version{}, fmt.Errorf("%w: tenant_id and skill_id are required", ErrInvalidInput)
	}
	if _, ok := m.skills[m.key(ver.TenantID, ver.SkillID)]; !ok {
		return Version{}, fmt.Errorf("%w: skill %s", ErrNotFound, ver.SkillID)
	}
	numKey := fmt.Sprintf("%s|%s|%d", ver.TenantID, ver.SkillID, ver.Version)
	if _, ok := m.byNum[numKey]; ok {
		return Version{}, fmt.Errorf("%w: version %d already exists", ErrConflict, ver.Version)
	}
	if ver.ID == "" {
		ver.ID = m.nextID("skv")
	}
	if ver.CreatedAt.IsZero() {
		ver.CreatedAt = m.now()
	}
	if !ver.Status.Valid() {
		ver.Status = VersionCandidate
	}
	if !ver.ValidationStatus.Valid() {
		ver.ValidationStatus = ValidationPending
	}
	m.versions[m.key(ver.TenantID, ver.ID)] = ver
	m.byNum[numKey] = ver.ID
	return ver, nil
}

// GetVersion implements Repository.
func (m *MemoryRepository) GetVersion(ctx context.Context, tenantID, id string) (Version, error) {
	_ = ctx
	m.mu.Lock()
	defer m.mu.Unlock()
	ver, ok := m.versions[m.key(tenantID, id)]
	if !ok {
		return Version{}, ErrNotFound
	}
	return ver, nil
}

// ListVersions implements Repository.
func (m *MemoryRepository) ListVersions(ctx context.Context, tenantID, skillID string) ([]Version, error) {
	_ = ctx
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []Version
	for _, ver := range m.versions {
		if ver.TenantID == tenantID && ver.SkillID == skillID {
			out = append(out, ver)
		}
	}
	// Stable-ish by version number.
	for i := 0; i < len(out); i++ {
		for j := i + 1; j < len(out); j++ {
			if out[j].Version < out[i].Version {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return out, nil
}

// GetVersionByNumber implements Repository.
func (m *MemoryRepository) GetVersionByNumber(ctx context.Context, tenantID, skillID string, version int64) (Version, error) {
	_ = ctx
	m.mu.Lock()
	defer m.mu.Unlock()
	id, ok := m.byNum[fmt.Sprintf("%s|%s|%d", tenantID, skillID, version)]
	if !ok {
		return Version{}, ErrNotFound
	}
	return m.versions[m.key(tenantID, id)], nil
}

// ListSkills implements Repository.
func (m *MemoryRepository) ListSkills(ctx context.Context, tenantID string, statuses []Status) ([]Skill, error) {
	_ = ctx
	m.mu.Lock()
	defer m.mu.Unlock()
	allow := map[Status]struct{}{}
	for _, s := range statuses {
		allow[s] = struct{}{}
	}
	var out []Skill
	for _, sk := range m.skills {
		if sk.TenantID != tenantID {
			continue
		}
		if len(allow) > 0 {
			if _, ok := allow[sk.Status]; !ok {
				continue
			}
		}
		out = append(out, sk)
	}
	return out, nil
}

// UpdateVersion implements Repository.
func (m *MemoryRepository) UpdateVersion(ctx context.Context, ver Version) (Version, error) {
	_ = ctx
	m.mu.Lock()
	defer m.mu.Unlock()
	key := m.key(ver.TenantID, ver.ID)
	if _, ok := m.versions[key]; !ok {
		return Version{}, ErrNotFound
	}
	m.versions[key] = ver
	return ver, nil
}

// ListActiveVersions implements Repository.
func (m *MemoryRepository) ListActiveVersions(ctx context.Context, tenantID string) ([]Version, error) {
	_ = ctx
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []Version
	for _, sk := range m.skills {
		if sk.TenantID != tenantID || sk.Status != StatusActive || sk.ActiveVersionID == nil {
			continue
		}
		ver, ok := m.versions[m.key(tenantID, *sk.ActiveVersionID)]
		if ok {
			out = append(out, ver)
		}
	}
	return out, nil
}
