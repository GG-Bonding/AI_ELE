package learning

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/agent-experience-engine/agent-experience-engine/internal/experience"
)

// MemoryEventApplier applies learning events atomically in-process for memory repositories.
type MemoryEventApplier struct {
	mu           sync.Mutex
	experiences  experience.Repository
	events       EventRepository
	now          func() time.Time
	maxAttempts  int
}

// NewMemoryEventApplier constructs an in-process atomic applier.
func NewMemoryEventApplier(experiences experience.Repository, events EventRepository) *MemoryEventApplier {
	return &MemoryEventApplier{
		experiences: experiences,
		events:      events,
		now:         time.Now().UTC,
		maxAttempts: 8,
	}
}

func (a *MemoryEventApplier) ApplyPendingEvent(ctx context.Context, tenantID string, ev Event) (ApplyResult, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if ev.Status == EventApplied {
		exp, err := a.experiences.Get(ctx, tenantID, ev.ExperienceID)
		if err != nil {
			return ApplyResult{}, fmt.Errorf("get applied experience %s: %w", ev.ExperienceID, err)
		}
		return ApplyResult{Experience: exp, OldUtility: exp.Utility, AlreadyApplied: true}, nil
	}

	expReward := ev.NormalizedReward * ev.Credit
	now := a.now()
	var lastErr error
	for attempt := 0; attempt < a.maxAttempts; attempt++ {
		exp, err := a.experiences.Get(ctx, tenantID, ev.ExperienceID)
		if err != nil {
			return ApplyResult{}, fmt.Errorf("get experience %s: %w", ev.ExperienceID, err)
		}
		oldUtil := exp.Utility
		updated, err := experience.ApplyBetaUpdate(exp, expReward, ev.Confidence, now)
		if err != nil {
			return ApplyResult{}, fmt.Errorf("beta update experience %s: %w", ev.ExperienceID, err)
		}
		saved, err := a.experiences.Update(ctx, updated)
		if err != nil {
			if errors.Is(err, experience.ErrConflict) && attempt+1 < a.maxAttempts {
				lastErr = err
				continue
			}
			return ApplyResult{}, fmt.Errorf("persist utility for experience %s: %w", ev.ExperienceID, err)
		}
		if err := a.events.MarkApplied(ctx, tenantID, ev.ID, now); err != nil {
			// Roll back utility fields on the current version so retry does not double-apply.
			cur, getErr := a.experiences.Get(ctx, tenantID, ev.ExperienceID)
			if getErr != nil {
				return ApplyResult{}, fmt.Errorf("mark applied failed (%v) and get for rollback failed: %w", err, getErr)
			}
			cur.Utility = exp.Utility
			cur.Alpha = exp.Alpha
			cur.Beta = exp.Beta
			cur.SuccessCount = exp.SuccessCount
			cur.FailureCount = exp.FailureCount
			cur.UpdatedAt = exp.UpdatedAt
			cur.LastUsedAt = exp.LastUsedAt
			if _, rollbackErr := a.experiences.Update(ctx, cur); rollbackErr != nil {
				return ApplyResult{}, fmt.Errorf("mark applied failed (%v) and rollback failed: %w", err, rollbackErr)
			}
			return ApplyResult{}, fmt.Errorf("mark learning event applied: %w", err)
		}
		return ApplyResult{Experience: saved, OldUtility: oldUtil}, nil
	}
	if lastErr != nil {
		return ApplyResult{}, fmt.Errorf("persist utility for experience %s: %w", ev.ExperienceID, lastErr)
	}
	return ApplyResult{}, fmt.Errorf("apply learning event %s: exhausted retries", ev.ID)
}
