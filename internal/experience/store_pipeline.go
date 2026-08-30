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
	dedup    SemanticDedupConfig
}

// StorePipelineConfig configures quality thresholds when using the default rule evaluator.
type StorePipelineConfig struct {
	ActiveMin     float64
	CandidateMin  float64
	Evaluator     evaluator.Evaluator
	SemanticDedup SemanticDedupConfig
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
	return &StorePipeline{
		svc:      svc,
		embedder: embedder,
		eval:     ev,
		dedup:    cfg.SemanticDedup.withDefaults(),
	}, nil
}

// StoreCandidatesResult summarizes persistence after extraction.
type StoreCandidatesResult struct {
	Stored        []Experience
	Reinforced    []Experience
	Conflicts     []ExperienceRelation
	Supersessions []ConflictResolution
	Evaluations   []evaluator.Evaluation
	Skipped       int
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
	// Legacy callers without explicit outcome default to UNKNOWN (low score), not SUCCESS.
	if out.Status == "" {
		out.Status = "UNKNOWN"
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

	storedEvidence := toStoredEvidence(evidence)

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

		merge, err := p.trySemanticMerge(ctx, tenantID, sourceEpisodeID, c, vectors[i], storedEvidence, eval.Quality)
		if err != nil {
			return result, fmt.Errorf("semantic dedup candidate %d for episode %s: %w", i, sourceEpisodeID, err)
		}
		if merge.Reinforced {
			result.Reinforced = append(result.Reinforced, merge.Experience)
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
			DedupKey:        Fingerprint(c),
			Evidence:        storedEvidence,
			Confidence:      eval.Quality,
			Status:          Status(eval.Status),
			Embedding:       vectors[i],
		})
		if err != nil {
			return result, fmt.Errorf("store candidate %d for episode %s: %w", i, sourceEpisodeID, err)
		}
		result.Stored = append(result.Stored, created)

		for _, peer := range merge.ConflictPeers {
			resolution, err := p.svc.ResolveConflict(ctx, tenantID, created.ID, peer.ID, peer.Similarity)
			if err != nil {
				return result, fmt.Errorf("resolve conflict for candidate %d episode %s: %w", i, sourceEpisodeID, err)
			}
			switch resolution.Kind {
			case ConflictSuperseded:
				result.Supersessions = append(result.Supersessions, resolution)
			case ConflictUnresolved:
				if resolution.Relation.ID != "" {
					result.Conflicts = append(result.Conflicts, resolution.Relation)
				}
			}
		}
	}
	return result, nil
}

func (p *StorePipeline) trySemanticMerge(
	ctx context.Context,
	tenantID, sourceEpisodeID string,
	c Candidate,
	embedding []float32,
	evidence Evidence,
	quality float64,
) (semanticMergeResult, error) {
	out := semanticMergeResult{}
	if p.dedup.Disabled {
		return out, nil
	}

	neighbors, err := p.svc.Search(ctx, SearchInput{
		TenantID:       tenantID,
		Types:          []Type{c.Type},
		Scopes:         []Scope{c.Scope},
		ScopeKey:       c.ScopeKey,
		QueryEmbedding: embedding,
		TopK:           p.dedup.NeighborTopK,
	})
	if err != nil {
		return out, err
	}

	for _, n := range neighbors {
		if n.Similarity <= p.dedup.MinSimilarity {
			break
		}
		// Exact within-episode fingerprint path already returns existing row on Create;
		// skip reinforcing the same source episode as a no-op semantic hit.
		if n.Experience.SourceEpisodeID == sourceEpisodeID && Fingerprint(c) == n.Experience.DedupKey {
			continue
		}
		decision, err := p.dedup.Judge.Judge(ctx, DedupPair{
			CandidateTrigger: c.Trigger,
			CandidateContent: c.Content,
			NeighborTrigger:  n.Experience.Trigger,
			NeighborContent:  n.Experience.Content,
			Similarity:       n.Similarity,
		})
		if err != nil {
			return out, err
		}
		switch decision {
		case DedupSame:
			reinforced, err := p.svc.Reinforce(ctx, tenantID, n.Experience.ID, ReinforceInput{
				EpisodeID:  sourceEpisodeID,
				Evidence:   evidence,
				Confidence: quality,
			})
			if err != nil {
				return out, err
			}
			out.Reinforced = true
			out.Experience = reinforced
			return out, nil
		case DedupConflict:
			out.ConflictPeers = append(out.ConflictPeers, conflictPeer{
				ID:         n.Experience.ID,
				Similarity: n.Similarity,
			})
			continue
		default:
			// RELATED / DIFFERENT: keep searching lower-ranked neighbors.
			continue
		}
	}
	return out, nil
}

type conflictPeer struct {
	ID         string
	Similarity float64
}

type semanticMergeResult struct {
	Reinforced    bool
	Experience    Experience
	ConflictPeers []conflictPeer
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
		SupportEpisodeIDs:   appendSupportEpisode(nil, e.SourceEpisodeID),
		AttemptIDs:          append([]string(nil), e.AttemptIDs...),
		OutcomeID:           e.OutcomeID,
	}
}
