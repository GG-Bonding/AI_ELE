package skill

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// MemoryLearningApplier applies skill learning events atomically in-process.
type MemoryLearningApplier struct {
	mu       sync.Mutex
	repo     Repository
	learning LearningStore
}

// NewMemoryLearningApplier constructs an in-process atomic applier.
func NewMemoryLearningApplier(repo Repository, learning LearningStore) *MemoryLearningApplier {
	return &MemoryLearningApplier{repo: repo, learning: learning}
}

// ApplyPending implements LearningApplier.
func (a *MemoryLearningApplier) ApplyPending(ctx context.Context, tenantID, eventID string) (LearningApplyResult, error) {
	if a == nil || a.repo == nil || a.learning == nil {
		return LearningApplyResult{}, fmt.Errorf("%w: applier not configured", ErrInvalidInput)
	}
	a.mu.Lock()
	defer a.mu.Unlock()

	ev, err := a.learning.GetLearningEvent(ctx, tenantID, eventID)
	if err != nil {
		return LearningApplyResult{}, err
	}
	ver, err := a.repo.GetVersion(ctx, tenantID, ev.SkillVersionID)
	if err != nil {
		return LearningApplyResult{}, err
	}
	if ev.Status == "APPLIED" {
		return LearningApplyResult{Version: ver, AlreadyApplied: true}, nil
	}

	expReward := ev.Reward * ev.Credit
	if ev.Credit == 0 {
		expReward = ev.Reward
	}
	updated, err := ApplyBetaUpdate(ver, expReward, ev.Confidence)
	if err != nil {
		_ = a.learning.MarkLearningFailed(ctx, tenantID, eventID)
		return LearningApplyResult{}, err
	}
	saved, err := a.repo.UpdateVersion(ctx, updated)
	if err != nil {
		_ = a.learning.MarkLearningFailed(ctx, tenantID, eventID)
		return LearningApplyResult{}, err
	}
	now := time.Now().UTC()
	if err := a.learning.MarkLearningApplied(ctx, tenantID, eventID, now); err != nil {
		return LearningApplyResult{}, err
	}
	return LearningApplyResult{Version: saved}, nil
}

var _ LearningApplier = (*MemoryLearningApplier)(nil)
