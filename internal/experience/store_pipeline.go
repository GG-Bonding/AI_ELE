package experience

import (
	"context"
	"fmt"
	"strings"

	"github.com/agent-experience-engine/agent-experience-engine/internal/provider"
)

// StorePipeline embeds candidates and persists qualifying experiences.
type StorePipeline struct {
	svc          *Service
	embedder     provider.EmbeddingProvider
	activeMin    float64
	candidateMin float64
}

// StorePipelineConfig configures quality thresholds.
type StorePipelineConfig struct {
	ActiveMin    float64
	CandidateMin float64
}

// NewStorePipeline constructs a store pipeline.
func NewStorePipeline(svc *Service, embedder provider.EmbeddingProvider, cfg StorePipelineConfig) (*StorePipeline, error) {
	if svc == nil {
		return nil, fmt.Errorf("experience service is required")
	}
	if embedder == nil {
		return nil, fmt.Errorf("embedding provider is required")
	}
	if cfg.ActiveMin == 0 {
		cfg.ActiveMin = 0.65
	}
	if cfg.CandidateMin == 0 {
		cfg.CandidateMin = 0.4
	}
	return &StorePipeline{
		svc:          svc,
		embedder:     embedder,
		activeMin:    cfg.ActiveMin,
		candidateMin: cfg.CandidateMin,
	}, nil
}

// StoreCandidatesResult summarizes persistence after extraction.
type StoreCandidatesResult struct {
	Stored  []Experience
	Skipped int
}

// StoreCandidates embeds and stores candidates that pass quality thresholds.
func (p *StorePipeline) StoreCandidates(
	ctx context.Context,
	tenantID, sourceEpisodeID string,
	candidates []Candidate,
) (StoreCandidatesResult, error) {
	var result StoreCandidatesResult
	if len(candidates) == 0 {
		return result, nil
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
		status, ok := StatusFromConfidence(c.Confidence, p.activeMin, p.candidateMin)
		if !ok {
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
			Confidence:      c.Confidence,
			Status:          status,
			Embedding:       vectors[i],
		})
		if err != nil {
			return result, fmt.Errorf("store candidate %d for episode %s: %w", i, sourceEpisodeID, err)
		}
		result.Stored = append(result.Stored, created)
	}
	return result, nil
}
