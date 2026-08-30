package experience

import (
	"context"
	"fmt"
	"strings"

	"github.com/agent-experience-engine/agent-experience-engine/internal/evaluator"
	"github.com/agent-experience-engine/agent-experience-engine/internal/outcome"
	"github.com/agent-experience-engine/agent-experience-engine/internal/provider"
)

// StorePipeline embeds candidates and persists qualifying experiences via Evaluator.
type StorePipeline struct {
	svc      *Service
	embedder provider.EmbeddingProvider
	eval     evaluator.Evaluator
}

// StorePipelineConfig configures quality thresholds when using the default rule evaluator.
type StorePipelineConfig struct {
	ActiveMin    float64
	CandidateMin float64
	Evaluator    evaluator.Evaluator
}

// NewStorePipeline constructs a store pipeline.
func NewStorePipeline(svc *Service, embedder provider.EmbeddingProvider, cfg StorePipelineConfig) (*StorePipeline, error) {
	if svc == nil {
		return nil, fmt.Errorf("experience service is required")
	}
	if embedder == nil {
		return nil, fmt.Errorf("embedding provider is required")
	}
	ev := cfg.Evaluator
	if ev == nil {
		ev = evaluator.NewRuleEvaluator(cfg.ActiveMin, cfg.CandidateMin)
	}
	return &StorePipeline{svc: svc, embedder: embedder, eval: ev}, nil
}

// StoreCandidatesResult summarizes persistence after extraction.
type StoreCandidatesResult struct {
	Stored      []Experience
	Evaluations []evaluator.Evaluation
	Skipped     int
}

// StoreOptions carries episode outcome/evidence into evaluation.
type StoreOptions struct {
	Outcome  outcome.Outcome
	Evidence evaluator.Evidence
}

// StoreCandidates embeds and stores candidates that pass the evaluator.
func (p *StorePipeline) StoreCandidates(
	ctx context.Context,
	tenantID, sourceEpisodeID string,
	candidates []Candidate,
) (StoreCandidatesResult, error) {
	return p.StoreCandidatesWithOptions(ctx, tenantID, sourceEpisodeID, candidates, StoreOptions{})
}

// StoreCandidatesWithOptions evaluates with outcome/evidence context.
func (p *StorePipeline) StoreCandidatesWithOptions(
	ctx context.Context,
	tenantID, sourceEpisodeID string,
	candidates []Candidate,
	opts StoreOptions,
) (StoreCandidatesResult, error) {
	var result StoreCandidatesResult
	if len(candidates) == 0 {
		return result, nil
	}

	evidence := opts.Evidence
	if evidence.SourceEpisodeID == "" {
		evidence.SourceEpisodeID = sourceEpisodeID
	}
	out := opts.Outcome
	if out.Status == "" {
		out.Status = "SUCCESS"
		out.Verified = false
	}

	texts := make([]string, len(candidates))
	for i, c := range candidates {
		texts[i] = strings.TrimSpace(c.Trigger) + "\n" + strings.TrimSpace(c.Content)
	}
	vectors, err := p.embedder.Embed(ctx, texts)
	if err != nil {
		return result, fmt.Errorf("embed experience candidates for episode %s: %w", sourceEpisodeID, err)
	}
	if len(vectors) != len(candidates) {
		return result, fmt.Errorf("embed experience candidates: got %d vectors for %d candidates", len(vectors), len(candidates))
	}

	for i, c := range candidates {
		eval, err := p.eval.Evaluate(ctx, toEvalInput(c), evidence, out)
		if err != nil {
			return result, fmt.Errorf("evaluate candidate %d: %w", i, err)
		}
		result.Evaluations = append(result.Evaluations, eval)
		if !eval.Store {
			result.Skipped++
			continue
		}
		created, err := p.svc.Create(ctx, CreateInput{
			TenantID:        tenantID,
			Type:            c.Type,
			Scope:           c.Scope,
			ScopeKey:        c.ScopeKey,
			Trigger:         c.Trigger,
			Content:         c.Content,
			SourceEpisodeID: sourceEpisodeID,
			Evidence:        toStoredEvidence(evidence),
			Confidence:      eval.Quality,
			Status:          Status(eval.Status),
			Embedding:       vectors[i],
		})
		if err != nil {
			return result, fmt.Errorf("store candidate %d for episode %s: %w", i, sourceEpisodeID, err)
		}
		result.Stored = append(result.Stored, created)
	}
	return result, nil
}

func toEvalInput(c Candidate) evaluator.CandidateInput {
	return evaluator.CandidateInput{
		Type:       string(c.Type),
		Trigger:    c.Trigger,
		Content:    c.Content,
		Confidence: c.Confidence,
		Scope:      string(c.Scope),
		ScopeKey:   c.ScopeKey,
	}
}

func toStoredEvidence(e evaluator.Evidence) Evidence {
	return Evidence{
		FailedAttemptCount:  e.FailedAttemptCount,
		SuccessAttemptCount: e.SuccessAttemptCount,
		HasFailureContrast:  e.HasFailureContrast,
		HasToolErrorCode:    e.HasToolErrorCode,
		SourceEpisodeID:     e.SourceEpisodeID,
	}
}
