package evolutionjob

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/agent-experience-engine/agent-experience-engine/internal/experience"
)

// DefaultStaleProcessingAfter is how long a job may stay PROCESSING before recovery.
const DefaultStaleProcessingAfter = 15 * time.Minute

// DefaultSweepInterval is the background dirty-group poll interval.
const DefaultSweepInterval = 30 * time.Second

// DefaultSweepBatch is how many dirty groups to process per tick.
const DefaultSweepBatch = 10

const staleProcessingError = "stale PROCESSING recovered after crash or timeout"

// Generalizer runs scoped auto-generalization for one experience family.
type Generalizer interface {
	AutoGeneralize(ctx context.Context, tenantID string, opts experience.AutoGeneralizeOptions) (experience.AutoGeneralizeResult, error)
}

// Processor consumes dirty groups and runs Pattern evolution (V2.2-3).
type Processor struct {
	generalizer Generalizer
	repo        Repository
	staleAfter  time.Duration
	batch       int
	now         func() time.Time
	logger      *slog.Logger
}

// NewProcessor constructs an evolution job processor.
func NewProcessor(generalizer Generalizer, repo Repository) (*Processor, error) {
	if generalizer == nil {
		return nil, fmt.Errorf("generalizer is required")
	}
	if repo == nil {
		return nil, fmt.Errorf("evolution job repository is required")
	}
	return &Processor{
		generalizer: generalizer,
		repo:        repo,
		staleAfter:  DefaultStaleProcessingAfter,
		batch:       DefaultSweepBatch,
		now:         time.Now().UTC,
		logger:      slog.Default(),
	}, nil
}

// WithStaleAfter overrides the stale PROCESSING threshold.
func (p *Processor) WithStaleAfter(d time.Duration) *Processor {
	if d > 0 {
		p.staleAfter = d
	}
	return p
}

// WithBatch overrides dirty groups claimed per sweep.
func (p *Processor) WithBatch(n int) *Processor {
	if n > 0 {
		p.batch = n
	}
	return p
}

// WithLogger attaches a structured logger for the background loop.
func (p *Processor) WithLogger(logger *slog.Logger) *Processor {
	if logger != nil {
		p.logger = logger
	}
	return p
}

// ProcessResult summarizes one dirty-group evolution pass.
type ProcessResult struct {
	Job     Job
	Created int
	Skipped int
	Status  Status
	Error   string
}

// ProcessGroup evolves one dirty family: PENDING → PROCESSING → AutoGeneralize → APPLIED/FAILED.
func (p *Processor) ProcessGroup(ctx context.Context, g DirtyGroup) (ProcessResult, error) {
	job, err := p.repo.UpsertPending(ctx, g)
	if err != nil {
		return ProcessResult{}, fmt.Errorf("upsert evolution job: %w", err)
	}
	if job.Status == StatusProcessing && !p.isStale(job) {
		return ProcessResult{Job: job, Status: StatusProcessing}, nil
	}
	if err := p.repo.MarkProcessing(ctx, g.TenantID, g.Type, g.Scope, g.ScopeKey); err != nil {
		return ProcessResult{}, fmt.Errorf("mark evolution processing: %w", err)
	}
	_ = p.repo.ClearDirty(ctx, g)

	res, err := p.generalizer.AutoGeneralize(ctx, g.TenantID, experience.AutoGeneralizeOptions{
		Type:     g.Type,
		Scope:    g.Scope,
		ScopeKey: g.ScopeKey,
	})
	if err != nil {
		_ = p.repo.MarkFailed(ctx, g.TenantID, g.Type, g.Scope, g.ScopeKey, err.Error())
		return ProcessResult{
			Job:    job,
			Status: StatusFailed,
			Error:  err.Error(),
		}, nil
	}
	created := len(res.Created)
	if err := p.repo.MarkApplied(ctx, g.TenantID, g.Type, g.Scope, g.ScopeKey, created); err != nil {
		return ProcessResult{}, fmt.Errorf("mark evolution applied: %w", err)
	}
	job.Status = StatusApplied
	job.CreatedCount = created
	return ProcessResult{
		Job:     job,
		Created: created,
		Skipped: len(res.Skipped),
		Status:  StatusApplied,
	}, nil
}

// Sweep claims dirty groups and processes them. Returns how many groups were attempted.
func (p *Processor) Sweep(ctx context.Context) (int, error) {
	groups, err := p.repo.ListDirty(ctx, p.batch)
	if err != nil {
		return 0, fmt.Errorf("list dirty evolution groups: %w", err)
	}
	n := 0
	for _, g := range groups {
		if _, err := p.ProcessGroup(ctx, g); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

// RecoverStaleJobs marks stuck PROCESSING jobs as FAILED so a later dirty mark can reclaim them.
func (p *Processor) RecoverStaleJobs(ctx context.Context) (int, error) {
	cutoff := p.now().Add(-p.staleAfter)
	stale, err := p.repo.ListStaleProcessing(ctx, cutoff)
	if err != nil {
		return 0, fmt.Errorf("list stale evolution jobs: %w", err)
	}
	n := 0
	for _, job := range stale {
		if err := p.repo.MarkFailed(ctx, job.TenantID, job.Type, job.Scope, job.ScopeKey, staleProcessingError); err != nil {
			return n, fmt.Errorf("mark stale evolution job failed: %w", err)
		}
		// Re-queue so the next sweep retries without waiting for new feedback.
		if err := p.repo.MarkDirty(ctx, job.TenantID, job.Type, job.Scope, job.ScopeKey); err != nil {
			return n, fmt.Errorf("re-dirty stale evolution family: %w", err)
		}
		n++
	}
	return n, nil
}

// RunLoop periodically sweeps dirty groups until ctx is cancelled.
func (p *Processor) RunLoop(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = DefaultSweepInterval
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if n, err := p.RecoverStaleJobs(ctx); err != nil {
				p.logger.Error("evolution recover stale failed", "error", err.Error())
			} else if n > 0 {
				p.logger.Info("recovered stale evolution jobs", "count", n)
			}
			if n, err := p.Sweep(ctx); err != nil {
				p.logger.Error("evolution sweep failed", "error", err.Error())
			} else if n > 0 {
				p.logger.Info("evolution sweep processed dirty groups", "count", n)
			}
		}
	}
}

func (p *Processor) isStale(job Job) bool {
	if job.Status != StatusProcessing {
		return false
	}
	return p.now().Sub(job.UpdatedAt) >= p.staleAfter
}
