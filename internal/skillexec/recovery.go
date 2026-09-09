package skillexec

import (
	"context"
	"log/slog"
	"time"

	"github.com/agent-experience-engine/agent-experience-engine/internal/skill"
	"github.com/agent-experience-engine/agent-experience-engine/internal/skillruntime"
)

// RecoveryWorker reclaims stale RUNNING skill executions after crash (V3.3).
type RecoveryWorker struct {
	Store    skill.DurableExecutionStore
	Runtime  *skillruntime.Runtime
	Owner    string
	LeaseTTL time.Duration
	Logger   *slog.Logger
}

// RecoverOnce claims and resumes up to limit stale executions.
func (w *RecoveryWorker) RecoverOnce(ctx context.Context, limit int) (int, error) {
	if w == nil || w.Store == nil || w.Runtime == nil {
		return 0, nil
	}
	owner := w.Owner
	if owner == "" {
		owner = "recovery-worker"
	}
	ttl := w.LeaseTTL
	if ttl <= 0 {
		ttl = 30 * time.Second
	}
	now := time.Now().UTC()
	stale, err := w.Store.ListStaleRunning(ctx, now, limit)
	if err != nil {
		return 0, err
	}
	recovered := 0
	for _, ex := range stale {
		until := now.Add(ttl)
		claimed, ok, err := w.Store.ClaimExecutionLease(ctx, ex.TenantID, ex.ID, owner, until)
		if err != nil || !ok {
			continue
		}
		_, _, err = w.Runtime.Recover(ctx, claimed.TenantID, claimed.ID, skill.Spec{})
		if err != nil {
			if w.Logger != nil {
				w.Logger.Warn("skill execution recovery failed", "execution_id", claimed.ID, "err", err)
			}
			continue
		}
		recovered++
	}
	return recovered, nil
}

// RunLoop periodically recovers stale executions until ctx is cancelled.
func (w *RecoveryWorker) RunLoop(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = 15 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			n, err := w.RecoverOnce(ctx, 25)
			if err != nil && w.Logger != nil {
				w.Logger.Warn("skill execution recovery sweep failed", "err", err)
			} else if n > 0 && w.Logger != nil {
				w.Logger.Info("recovered skill executions", "count", n)
			}
		}
	}
}
