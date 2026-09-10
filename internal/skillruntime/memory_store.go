package skillruntime

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/agent-experience-engine/agent-experience-engine/internal/skill"
)

// MemoryExecutionStore is an in-memory ExecutionStore + LearningStore for tests.
type MemoryExecutionStore struct {
	mu         sync.Mutex
	executions map[string]skill.Execution       // tenant|id
	byIdem     map[string]string                // tenant|key → id
	steps      map[string][]skill.StepExecution // tenant|executionID
	approvals  map[string]skill.ApprovalRequest // tenant|id
	learning   map[string]skill.LearningEvent   // tenant|id
	byFeedback map[string]string                // tenant|feedback|version → id
	now        func() time.Time
	idSeq      int
}

// NewMemoryExecutionStore constructs an empty memory execution store.
func NewMemoryExecutionStore() *MemoryExecutionStore {
	return &MemoryExecutionStore{
		executions: map[string]skill.Execution{},
		byIdem:     map[string]string{},
		steps:      map[string][]skill.StepExecution{},
		approvals:  map[string]skill.ApprovalRequest{},
		learning:   map[string]skill.LearningEvent{},
		byFeedback: map[string]string{},
		now:        func() time.Time { return time.Now().UTC() },
	}
}

func (m *MemoryExecutionStore) key(tenantID, id string) string {
	return tenantID + "|" + id
}

func (m *MemoryExecutionStore) nextID(prefix string) string {
	m.idSeq++
	return fmt.Sprintf("%s_%d", prefix, m.idSeq)
}

// CreateExecution implements skill.ExecutionStore.
func (m *MemoryExecutionStore) CreateExecution(ctx context.Context, ex skill.Execution) (skill.Execution, error) {
	_ = ctx
	m.mu.Lock()
	defer m.mu.Unlock()
	if ex.TenantID == "" {
		return skill.Execution{}, fmt.Errorf("%w: tenant_id required", skill.ErrInvalidInput)
	}
	if ex.ID == "" {
		ex.ID = m.nextID("ex")
	}
	if ex.StartedAt.IsZero() {
		ex.StartedAt = m.now()
	}
	if ex.Status == "" {
		ex.Status = skill.ExecPending
	}
	if ex.LeaseEpoch == 0 {
		ex.LeaseEpoch = 1
	}
	if ex.IdempotencyKey != "" {
		if existingID, ok := m.byIdem[m.key(ex.TenantID, ex.IdempotencyKey)]; ok {
			if existing, ok := m.executions[m.key(ex.TenantID, existingID)]; ok {
				return existing, nil
			}
		}
		m.byIdem[m.key(ex.TenantID, ex.IdempotencyKey)] = ex.ID
	}
	m.executions[m.key(ex.TenantID, ex.ID)] = ex
	return ex, nil
}

// UpdateExecution implements skill.ExecutionStore.
// While status is RUNNING, callers must use UpdateExecutionFenced (V3.4).
func (m *MemoryExecutionStore) UpdateExecution(ctx context.Context, ex skill.Execution) (skill.Execution, error) {
	_ = ctx
	m.mu.Lock()
	defer m.mu.Unlock()
	k := m.key(ex.TenantID, ex.ID)
	cur, ok := m.executions[k]
	if !ok {
		return skill.Execution{}, skill.ErrNotFound
	}
	if cur.Status == skill.ExecRunning {
		return skill.Execution{}, fmt.Errorf("%w: use UpdateExecutionFenced while RUNNING", skill.ErrInvalidTransition)
	}
	m.executions[k] = ex
	return ex, nil
}

// GetExecution implements skill.ExecutionStore.
func (m *MemoryExecutionStore) GetExecution(ctx context.Context, tenantID, id string) (skill.Execution, error) {
	_ = ctx
	m.mu.Lock()
	defer m.mu.Unlock()
	ex, ok := m.executions[m.key(tenantID, id)]
	if !ok {
		return skill.Execution{}, skill.ErrNotFound
	}
	return ex, nil
}

// GetExecutionByIdempotency implements skill.ExecutionStore.
func (m *MemoryExecutionStore) GetExecutionByIdempotency(ctx context.Context, tenantID, key string) (skill.Execution, error) {
	_ = ctx
	m.mu.Lock()
	defer m.mu.Unlock()
	id, ok := m.byIdem[m.key(tenantID, key)]
	if !ok {
		return skill.Execution{}, skill.ErrNotFound
	}
	ex, ok := m.executions[m.key(tenantID, id)]
	if !ok {
		return skill.Execution{}, skill.ErrNotFound
	}
	return ex, nil
}

// CreateStep implements skill.ExecutionStore.
func (m *MemoryExecutionStore) CreateStep(ctx context.Context, st skill.StepExecution) (skill.StepExecution, error) {
	_ = ctx
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.createStepLocked(st)
}

func (m *MemoryExecutionStore) createStepLocked(st skill.StepExecution) (skill.StepExecution, error) {
	if st.TenantID == "" || st.ExecutionID == "" {
		return skill.StepExecution{}, fmt.Errorf("%w: tenant_id and execution_id required", skill.ErrInvalidInput)
	}
	if st.ID == "" {
		st.ID = m.nextID("st")
	}
	if st.Attempt <= 0 {
		st.Attempt = 1
	}
	k := m.key(st.TenantID, st.ExecutionID)
	m.steps[k] = append(m.steps[k], st)
	return st, nil
}

// CreateStepFenced implements skill.DurableExecutionStore.
func (m *MemoryExecutionStore) CreateStepFenced(ctx context.Context, st skill.StepExecution, expectedEpoch int64) (skill.StepExecution, bool, error) {
	_ = ctx
	m.mu.Lock()
	defer m.mu.Unlock()
	ex, ok := m.executions[m.key(st.TenantID, st.ExecutionID)]
	if !ok {
		return skill.StepExecution{}, false, skill.ErrNotFound
	}
	if ex.LeaseEpoch != expectedEpoch {
		return st, false, nil
	}
	if ex.Status != skill.ExecRunning {
		return st, false, nil
	}
	st.LeaseEpoch = expectedEpoch
	created, err := m.createStepLocked(st)
	if err != nil {
		return skill.StepExecution{}, false, err
	}
	return created, true, nil
}

// UpdateStep implements skill.ExecutionStore (V3.4).
func (m *MemoryExecutionStore) UpdateStep(ctx context.Context, st skill.StepExecution) (skill.StepExecution, error) {
	_ = ctx
	m.mu.Lock()
	defer m.mu.Unlock()
	k := m.key(st.TenantID, st.ExecutionID)
	list := m.steps[k]
	for i := range list {
		if list[i].ID == st.ID {
			list[i] = st
			m.steps[k] = list
			return st, nil
		}
	}
	return skill.StepExecution{}, skill.ErrNotFound
}

// UpdateStepFenced implements skill.DurableExecutionStore.
func (m *MemoryExecutionStore) UpdateStepFenced(ctx context.Context, st skill.StepExecution, expectedEpoch int64) (skill.StepExecution, bool, error) {
	_ = ctx
	m.mu.Lock()
	defer m.mu.Unlock()
	ex, ok := m.executions[m.key(st.TenantID, st.ExecutionID)]
	if !ok {
		return skill.StepExecution{}, false, skill.ErrNotFound
	}
	if ex.LeaseEpoch != expectedEpoch {
		return st, false, nil
	}
	if ex.Status != skill.ExecRunning {
		return st, false, nil
	}
	k := m.key(st.TenantID, st.ExecutionID)
	list := m.steps[k]
	for i := range list {
		if list[i].ID == st.ID {
			st.LeaseEpoch = expectedEpoch
			list[i] = st
			m.steps[k] = list
			return st, true, nil
		}
	}
	return skill.StepExecution{}, false, skill.ErrNotFound
}

// RenewLease implements skill.DurableExecutionStore.
func (m *MemoryExecutionStore) RenewLease(ctx context.Context, tenantID, executionID, owner string, expectedEpoch int64, until time.Time) (bool, error) {
	_ = ctx
	m.mu.Lock()
	defer m.mu.Unlock()
	k := m.key(tenantID, executionID)
	ex, ok := m.executions[k]
	if !ok {
		return false, skill.ErrNotFound
	}
	if ex.LeaseEpoch != expectedEpoch {
		return false, nil
	}
	if owner != "" && ex.LeaseOwner != "" && ex.LeaseOwner != owner {
		return false, nil
	}
	now := m.now()
	ex.LeaseUntil = &until
	ex.HeartbeatAt = &now
	if owner != "" {
		ex.LeaseOwner = owner
	}
	m.executions[k] = ex
	return true, nil
}

// ListSteps implements skill.ExecutionStore.
func (m *MemoryExecutionStore) ListSteps(ctx context.Context, tenantID, executionID string) ([]skill.StepExecution, error) {
	_ = ctx
	m.mu.Lock()
	defer m.mu.Unlock()
	src := m.steps[m.key(tenantID, executionID)]
	out := make([]skill.StepExecution, len(src))
	copy(out, src)
	return out, nil
}

// CreateApproval implements skill.ExecutionStore.
func (m *MemoryExecutionStore) CreateApproval(ctx context.Context, req skill.ApprovalRequest) (skill.ApprovalRequest, error) {
	_ = ctx
	m.mu.Lock()
	defer m.mu.Unlock()
	if req.TenantID == "" {
		return skill.ApprovalRequest{}, fmt.Errorf("%w: tenant_id required", skill.ErrInvalidInput)
	}
	if req.ID == "" {
		req.ID = m.nextID("ap")
	}
	if req.CreatedAt.IsZero() {
		req.CreatedAt = m.now()
	}
	if req.Status == "" {
		req.Status = skill.ApprovalPending
	}
	m.approvals[m.key(req.TenantID, req.ID)] = req
	return req, nil
}

// UpdateApproval implements skill.ExecutionStore.
func (m *MemoryExecutionStore) UpdateApproval(ctx context.Context, req skill.ApprovalRequest) (skill.ApprovalRequest, error) {
	_ = ctx
	m.mu.Lock()
	defer m.mu.Unlock()
	k := m.key(req.TenantID, req.ID)
	if _, ok := m.approvals[k]; !ok {
		return skill.ApprovalRequest{}, skill.ErrNotFound
	}
	m.approvals[k] = req
	return req, nil
}

// GetApproval implements skill.ExecutionStore.
func (m *MemoryExecutionStore) GetApproval(ctx context.Context, tenantID, id string) (skill.ApprovalRequest, error) {
	_ = ctx
	m.mu.Lock()
	defer m.mu.Unlock()
	req, ok := m.approvals[m.key(tenantID, id)]
	if !ok {
		return skill.ApprovalRequest{}, skill.ErrNotFound
	}
	return req, nil
}

// GetApprovalByExecution implements skill.ExecutionStore.
func (m *MemoryExecutionStore) GetApprovalByExecution(ctx context.Context, tenantID, executionID string) (skill.ApprovalRequest, error) {
	_ = ctx
	m.mu.Lock()
	defer m.mu.Unlock()
	var latest skill.ApprovalRequest
	found := false
	for _, req := range m.approvals {
		if req.TenantID == tenantID && req.ExecutionID == executionID {
			if !found || req.CreatedAt.After(latest.CreatedAt) {
				latest = req
				found = true
			}
		}
	}
	if !found {
		return skill.ApprovalRequest{}, skill.ErrNotFound
	}
	return latest, nil
}

// ListFailedByVersion implements skill.ExecutionStore.
func (m *MemoryExecutionStore) ListFailedByVersion(ctx context.Context, tenantID, skillVersionID string, limit int) ([]skill.Execution, error) {
	_ = ctx
	m.mu.Lock()
	defer m.mu.Unlock()
	if limit <= 0 {
		limit = 10
	}
	out := make([]skill.Execution, 0, limit)
	for _, ex := range m.executions {
		if ex.TenantID != tenantID || ex.SkillVersionID != skillVersionID {
			continue
		}
		if ex.Status != skill.ExecFailed && ex.Status != skill.ExecNeedsReconciliation {
			continue
		}
		out = append(out, ex)
	}
	// Newest first by StartedAt.
	for i := 0; i < len(out); i++ {
		for j := i + 1; j < len(out); j++ {
			if out[j].StartedAt.After(out[i].StartedAt) {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// CreateLearningEvent implements skill.LearningStore.
func (m *MemoryExecutionStore) CreateLearningEvent(ctx context.Context, ev skill.LearningEvent) (skill.LearningEvent, error) {
	_ = ctx
	m.mu.Lock()
	defer m.mu.Unlock()
	if ev.TenantID == "" || ev.FeedbackID == "" || ev.SkillVersionID == "" {
		return skill.LearningEvent{}, fmt.Errorf("%w: tenant_id, feedback_id, skill_version_id required", skill.ErrInvalidInput)
	}
	fbKey := fmt.Sprintf("%s|%s|%s", ev.TenantID, ev.FeedbackID, ev.SkillVersionID)
	if _, ok := m.byFeedback[fbKey]; ok {
		return skill.LearningEvent{}, skill.ErrDuplicateLearning
	}
	if ev.ID == "" {
		ev.ID = m.nextID("le")
	}
	if ev.CreatedAt.IsZero() {
		ev.CreatedAt = m.now()
	}
	if ev.Status == "" {
		ev.Status = "PENDING"
	}
	m.learning[m.key(ev.TenantID, ev.ID)] = ev
	m.byFeedback[fbKey] = ev.ID
	return ev, nil
}

// GetLearningEvent implements skill.LearningStore.
func (m *MemoryExecutionStore) GetLearningEvent(ctx context.Context, tenantID, id string) (skill.LearningEvent, error) {
	_ = ctx
	m.mu.Lock()
	defer m.mu.Unlock()
	ev, ok := m.learning[m.key(tenantID, id)]
	if !ok {
		return skill.LearningEvent{}, skill.ErrLearningNotFound
	}
	return ev, nil
}

// GetLearningEventByFeedbackVersion implements skill.LearningStore.
func (m *MemoryExecutionStore) GetLearningEventByFeedbackVersion(ctx context.Context, tenantID, feedbackID, versionID string) (skill.LearningEvent, error) {
	_ = ctx
	m.mu.Lock()
	defer m.mu.Unlock()
	id, ok := m.byFeedback[fmt.Sprintf("%s|%s|%s", tenantID, feedbackID, versionID)]
	if !ok {
		return skill.LearningEvent{}, skill.ErrLearningNotFound
	}
	ev, ok := m.learning[m.key(tenantID, id)]
	if !ok {
		return skill.LearningEvent{}, skill.ErrLearningNotFound
	}
	return ev, nil
}

// MarkLearningApplied implements skill.LearningStore.
func (m *MemoryExecutionStore) MarkLearningApplied(ctx context.Context, tenantID, id string, at time.Time) error {
	_ = ctx
	m.mu.Lock()
	defer m.mu.Unlock()
	k := m.key(tenantID, id)
	ev, ok := m.learning[k]
	if !ok {
		return skill.ErrNotFound
	}
	ev.Status = "APPLIED"
	ev.AppliedAt = &at
	m.learning[k] = ev
	return nil
}

// MarkLearningFailed implements skill.LearningStore.
func (m *MemoryExecutionStore) MarkLearningFailed(ctx context.Context, tenantID, id string) error {
	_ = ctx
	m.mu.Lock()
	defer m.mu.Unlock()
	k := m.key(tenantID, id)
	ev, ok := m.learning[k]
	if !ok {
		return skill.ErrNotFound
	}
	ev.Status = "FAILED"
	m.learning[k] = ev
	return nil
}

// ListStaleRunning implements skill.DurableExecutionStore.
func (m *MemoryExecutionStore) ListStaleRunning(ctx context.Context, olderThan time.Time, limit int) ([]skill.Execution, error) {
	_ = ctx
	m.mu.Lock()
	defer m.mu.Unlock()
	if limit <= 0 {
		limit = 50
	}
	var out []skill.Execution
	for _, ex := range m.executions {
		if ex.Status != skill.ExecRunning {
			continue
		}
		if ex.LeaseUntil != nil && !ex.LeaseUntil.Before(olderThan) {
			continue
		}
		out = append(out, ex)
		if len(out) >= limit {
			break
		}
	}
	return out, nil
}

// ClaimExecutionLease implements skill.DurableExecutionStore.
func (m *MemoryExecutionStore) ClaimExecutionLease(ctx context.Context, tenantID, executionID, owner string, until time.Time) (skill.Execution, bool, error) {
	_ = ctx
	m.mu.Lock()
	defer m.mu.Unlock()
	k := m.key(tenantID, executionID)
	ex, ok := m.executions[k]
	if !ok || ex.Status != skill.ExecRunning {
		return skill.Execution{}, false, nil
	}
	now := m.now()
	if ex.LeaseUntil != nil && ex.LeaseUntil.After(now) && ex.LeaseOwner != owner {
		return skill.Execution{}, false, nil
	}
	ex.LeaseOwner = owner
	ex.LeaseUntil = &until
	ex.HeartbeatAt = &now
	ex.LeaseEpoch++
	m.executions[k] = ex
	return ex, true, nil
}

// UpdateExecutionFenced implements skill.DurableExecutionStore.
func (m *MemoryExecutionStore) UpdateExecutionFenced(ctx context.Context, ex skill.Execution, expectedEpoch int64) (skill.Execution, bool, error) {
	_ = ctx
	m.mu.Lock()
	defer m.mu.Unlock()
	k := m.key(ex.TenantID, ex.ID)
	cur, ok := m.executions[k]
	if !ok {
		return skill.Execution{}, false, skill.ErrNotFound
	}
	if cur.LeaseEpoch != expectedEpoch {
		return cur, false, nil
	}
	if cur.Status != skill.ExecRunning && cur.Status != skill.ExecPending {
		return cur, false, nil
	}
	ex.LeaseEpoch = expectedEpoch
	m.executions[k] = ex
	return ex, true, nil
}

var _ skill.DurableExecutionStore = (*MemoryExecutionStore)(nil)
var _ skill.ExecutionStore = (*MemoryExecutionStore)(nil)
