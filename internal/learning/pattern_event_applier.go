package learning

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/agent-experience-engine/agent-experience-engine/internal/experience"
)

// MemoryPatternEventApplier applies pattern learning events atomically in-process.
type MemoryPatternEventApplier struct {
	mu       sync.Mutex
	patterns experience.PatternRepository
	events   PatternEventRepository
	now      func() time.Time
}

// NewMemoryPatternEventApplier constructs an in-process pattern event applier.
func NewMemoryPatternEventApplier(patterns experience.PatternRepository, events PatternEventRepository) *MemoryPatternEventApplier {
	return &MemoryPatternEventApplier{
		patterns: patterns,
		events:   events,
		now:      time.Now().UTC,
	}
}

func (a *MemoryPatternEventApplier) ApplyPendingPatternEvent(ctx context.Context, tenantID string, ev PatternEvent) (PatternApplyResult, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.patterns == nil {
		return PatternApplyResult{}, fmt.Errorf("pattern repository is required")
	}
	if ev.Status == EventApplied {
		p, err := a.patterns.Get(ctx, tenantID, ev.PatternID)
		if err != nil {
			return PatternApplyResult{}, fmt.Errorf("get applied pattern %s: %w", ev.PatternID, err)
		}
		return PatternApplyResult{Pattern: p, OldUtility: p.Utility, AlreadyApplied: true}, nil
	}

	patReward := ev.NormalizedReward * ev.Credit
	now := a.now()
	p, err := a.patterns.Get(ctx, tenantID, ev.PatternID)
	if err != nil {
		return PatternApplyResult{}, fmt.Errorf("get pattern %s: %w", ev.PatternID, err)
	}
	oldUtil := p.Utility
	updated, err := experience.ApplyPatternBetaUpdate(p, patReward, ev.Confidence, now)
	if err != nil {
		return PatternApplyResult{}, fmt.Errorf("beta update pattern %s: %w", ev.PatternID, err)
	}
	updated = experience.MaybePromotePattern(updated)
	saved, err := a.patterns.Update(ctx, updated)
	if err != nil {
		return PatternApplyResult{}, fmt.Errorf("persist pattern %s utility: %w", ev.PatternID, err)
	}
	if err := a.events.MarkApplied(ctx, tenantID, ev.ID, now); err != nil {
		cur, getErr := a.patterns.Get(ctx, tenantID, ev.PatternID)
		if getErr == nil {
			cur.Utility = p.Utility
			cur.Alpha = p.Alpha
			cur.Beta = p.Beta
			cur.SuccessCount = p.SuccessCount
			cur.FailureCount = p.FailureCount
			cur.Status = p.Status
			cur.UpdatedAt = p.UpdatedAt
			_, _ = a.patterns.Update(ctx, cur)
		}
		return PatternApplyResult{}, fmt.Errorf("mark pattern learning event applied: %w", err)
	}
	return PatternApplyResult{Pattern: saved, OldUtility: oldUtil}, nil
}

var _ PatternEventApplier = (*MemoryPatternEventApplier)(nil)
