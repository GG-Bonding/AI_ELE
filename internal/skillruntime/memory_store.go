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
func (m *MemoryExecutionStore) UpdateExecution(ctx context.Context, ex skill.Execution) (skill.Execution, error) {
	_ = ctx
	m.mu.Lock()
	defer m.mu.Unlock()
	k := m.key(ex.TenantID, ex.ID)
	if _, ok := m.executions[k]; !ok {
		return skill.Execution{}, skill.ErrNotFound
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
	if st.TenantID == "" || st.ExecutionID == "" {
		return skill.StepExecution{}, fmt.Errorf("%w: tenant_id and execution_id required", skill.ErrInvalidInput)
	}
	if st.ID == "" {
		st.ID = m.nextID("st")
	}
	k := m.key(st.TenantID, st.ExecutionID)
	m.steps[k] = append(m.steps[k], st)
	return st, nil
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
