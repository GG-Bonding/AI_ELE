package episodelearn

import (
	"context"
	"fmt"
	"time"

	"github.com/agent-experience-engine/agent-experience-engine/internal/attempt"
	"github.com/agent-experience-engine/agent-experience-engine/internal/episode"
	"github.com/agent-experience-engine/agent-experience-engine/internal/evaluator"
	"github.com/agent-experience-engine/agent-experience-engine/internal/experience"
	"github.com/agent-experience-engine/agent-experience-engine/internal/extractor"
	"github.com/agent-experience-engine/agent-experience-engine/internal/outcome"
)

// DefaultStaleProcessingAfter is how long a job may stay PROCESSING before recovery (V2-0).
const DefaultStaleProcessingAfter = 15 * time.Minute

const staleProcessingError = "stale PROCESSING recovered after crash or timeout"

// Extractor extracts experience candidates from a completed episode trace.
type Extractor interface {
	Extract(ctx context.Context, in extractor.ExtractInput) ([]experience.Candidate, error)
}

// StorePipeline persists extracted candidates with outcome/evidence context.
type StorePipeline interface {
	StoreCandidatesWithOptions(
		ctx context.Context,
		tenantID, sourceEpisodeID string,
		candidates []experience.Candidate,
		opts experience.StoreOptions,
	) (experience.StoreCandidatesResult, error)
}

// Processor runs Extract → BuildEvidence → Store for a completed episode.
type Processor struct {
	extractor  Extractor
	store      StorePipeline
	jobs       Repository
	staleAfter time.Duration
	now        func() time.Time
}

// NewProcessor constructs an episode learning processor.
func NewProcessor(extractor Extractor, store StorePipeline, jobs Repository) (*Processor, error) {
	if extractor == nil {
		return nil, fmt.Errorf("extractor is required")
	}
	if store == nil {
		return nil, fmt.Errorf("store pipeline is required")
	}
	if jobs == nil {
		return nil, fmt.Errorf("learning job repository is required")
	}
	return &Processor{
		extractor:  extractor,
		store:      store,
		jobs:       jobs,
		staleAfter: DefaultStaleProcessingAfter,
		now:        func() time.Time { return time.Now().UTC() },
	}, nil
}

// WithStaleAfter overrides the stale PROCESSING threshold (tests / ops).
func (p *Processor) WithStaleAfter(d time.Duration) *Processor {
	if d > 0 {
		p.staleAfter = d
	}
	return p
}

// ProcessInput is everything needed to learn from a completed episode.
type ProcessInput struct {
	TenantID string
	Episode  episode.Episode
	Attempts []attempt.Attempt
	Outcome  outcome.Outcome
}

// ProcessResult summarizes extraction, storage, and job status.
type ProcessResult struct {
	Candidates        []experience.Candidate
	Stored            []experience.Experience
	Reinforced        []experience.Experience
	LearningStatus    Status
	LearningLastError string
}

// Process upserts a PENDING job, extracts, stores with real outcome/evidence, and marks APPLIED or FAILED.
func (p *Processor) Process(ctx context.Context, in ProcessInput) (ProcessResult, error) {
	if _, err := p.jobs.UpsertPending(ctx, in.TenantID, in.Episode.ID); err != nil {
		return ProcessResult{}, fmt.Errorf("upsert learning job: %w", err)
	}
	if err := p.jobs.MarkProcessing(ctx, in.TenantID, in.Episode.ID); err != nil {
		return ProcessResult{}, fmt.Errorf("mark learning processing: %w", err)
	}

	result, err := p.runPipeline(ctx, in)
	if err != nil {
		_ = p.jobs.MarkFailed(ctx, in.TenantID, in.Episode.ID, err.Error())
		result.LearningStatus = StatusFailed
		result.LearningLastError = err.Error()
		return result, nil
	}
	if err := p.jobs.MarkApplied(ctx, in.TenantID, in.Episode.ID); err != nil {
		return result, fmt.Errorf("mark learning applied: %w", err)
	}
	result.LearningStatus = StatusApplied
	return result, nil
}

// Retry re-runs the pipeline when the job is FAILED, PENDING, or stale PROCESSING (V2-0).
func (p *Processor) Retry(ctx context.Context, tenantID string, ep episode.Episode, attempts []attempt.Attempt, out outcome.Outcome) (ProcessResult, error) {
	job, err := p.jobs.GetByEpisode(ctx, tenantID, ep.ID)
	if err != nil {
		return ProcessResult{}, err
	}
	switch job.Status {
	case StatusApplied:
		return ProcessResult{LearningStatus: StatusApplied}, nil
	case StatusFailed, StatusPending:
		// fall through to re-process
	case StatusProcessing:
		if !p.isStale(job) {
			return ProcessResult{LearningStatus: StatusProcessing}, nil
		}
		if err := p.jobs.MarkFailed(ctx, tenantID, ep.ID, staleProcessingError); err != nil {
			return ProcessResult{}, fmt.Errorf("requeue stale processing job: %w", err)
		}
		// fall through to re-process
	}
	return p.Process(ctx, ProcessInput{
		TenantID: tenantID,
		Episode:  ep,
		Attempts: attempts,
		Outcome:  out,
	})
}

// RecoverStaleJobs marks stuck PROCESSING jobs as FAILED so retry can reclaim them (V2-0).
// Intended for process startup or a periodic ops sweep.
func (p *Processor) RecoverStaleJobs(ctx context.Context) (int, error) {
	cutoff := p.now().Add(-p.staleAfter)
	stale, err := p.jobs.ListStaleProcessing(ctx, cutoff)
	if err != nil {
		return 0, fmt.Errorf("list stale processing jobs: %w", err)
	}
	n := 0
	for _, job := range stale {
		if err := p.jobs.MarkFailed(ctx, job.TenantID, job.EpisodeID, staleProcessingError); err != nil {
			return n, fmt.Errorf("mark stale job failed for %s/%s: %w", job.TenantID, job.EpisodeID, err)
		}
		n++
	}
	return n, nil
}

func (p *Processor) isStale(job Job) bool {
	if job.Status != StatusProcessing {
		return false
	}
	return p.now().Sub(job.UpdatedAt) >= p.staleAfter
}

func (p *Processor) runPipeline(ctx context.Context, in ProcessInput) (ProcessResult, error) {
	var result ProcessResult
	candidates, err := p.extractor.Extract(ctx, extractor.ExtractInput{
		Episode:  in.Episode,
		Attempts: in.Attempts,
		Outcome:  in.Outcome,
	})
	if err != nil {
		return result, fmt.Errorf("extract experiences: %w", err)
	}
	result.Candidates = candidates

	ev := evaluator.FromAttempts(in.Episode.ID, in.Outcome.ID, in.Attempts)
	stored, err := p.store.StoreCandidatesWithOptions(ctx, in.TenantID, in.Episode.ID, candidates, experience.StoreOptions{
		Outcome:  in.Outcome,
		Evidence: ev,
	})
	if err != nil {
		return result, fmt.Errorf("store experiences: %w", err)
	}
	result.Stored = stored.Stored
	result.Reinforced = stored.Reinforced
	return result, nil
}
