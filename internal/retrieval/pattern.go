package retrieval

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/agent-experience-engine/agent-experience-engine/internal/experience"
)

// PatternStore is the persistence port for pattern retrieval (V2.1-2).
type PatternStore interface {
	List(ctx context.Context, filter experience.PatternListFilter) ([]experience.Pattern, error)
	ListEvidence(ctx context.Context, tenantID, patternID string) ([]experience.PatternEvidence, error)
}

// PatternRetriever ranks ACTIVE patterns for agent context (V2.1-2).
// Patterns lack embeddings today, so relevance uses lexical overlap × utility × confidence × scope.
type PatternRetriever struct {
	store          PatternStore
	defaultTopK    int
	candidateLimit int
	minSimilarity  float64
}

// NewPatternRetriever constructs a pattern retriever.
func NewPatternRetriever(store PatternStore) (*PatternRetriever, error) {
	if store == nil {
		return nil, fmt.Errorf("pattern store is required")
	}
	return &PatternRetriever{
		store:          store,
		defaultTopK:    3,
		candidateLimit: 50,
		minSimilarity:  0.05,
	}, nil
}

// PatternScore explains pattern ranking.
type PatternScore struct {
	Similarity float64 `json:"similarity"`
	Utility    float64 `json:"utility"`
	Confidence float64 `json:"confidence"`
	ScopeMatch float64 `json:"scope_match"`
	FinalScore float64 `json:"final_score"`
}

// RankedPattern is a pattern selected for context.
type RankedPattern struct {
	Pattern     experience.Pattern `json:"pattern"`
	Score       PatternScore       `json:"score"`
	EvidenceIDs []string           `json:"evidence_ids,omitempty"`
}

// RetrievePatterns returns ACTIVE patterns relevant to the task.
func (r *PatternRetriever) RetrievePatterns(ctx context.Context, q Query) ([]RankedPattern, error) {
	if strings.TrimSpace(q.TenantID) == "" {
		return nil, fmt.Errorf("%w: tenant_id is required", experience.ErrInvalidInput)
	}
	if strings.TrimSpace(q.Task) == "" {
		return nil, fmt.Errorf("%w: task is required", experience.ErrInvalidInput)
	}
	topK := q.TopK
	if topK <= 0 {
		topK = r.defaultTopK
	}

	candidates, err := r.store.List(ctx, experience.PatternListFilter{
		TenantID: q.TenantID,
		Statuses: []experience.PatternStatus{experience.PatternStatusActive},
		Limit:    r.candidateLimit,
	})
	if err != nil {
		return nil, fmt.Errorf("list active patterns: %w", err)
	}

	scope := ScopeContext{
		AgentID:  q.AgentID,
		UserID:   q.UserID,
		ScopeKey: q.ScopeKey,
		Tools:    q.Tools,
	}

	out := make([]RankedPattern, 0, len(candidates))
	for _, p := range candidates {
		authExp := experience.Experience{Scope: p.Scope, ScopeKey: p.ScopeKey, TenantID: p.TenantID}
		if !experience.AuthorizedForSearch(authExp, q.AgentID, q.UserID, q.Tools, q.ScopeKey) {
			continue
		}
		sim := LexicalOverlap(q.Task, strings.TrimSpace(p.Trigger+" "+p.Content))
		if sim < r.minSimilarity {
			continue
		}
		util := clamp01(p.Utility)
		conf := clamp01(p.Confidence)
		sm := ScopeMatch(authExp, scope)
		final := sim * util * conf * sm
		if final <= 0 {
			continue
		}
		evIDs, err := r.evidenceIDs(ctx, q.TenantID, p.ID)
		if err != nil {
			return nil, err
		}
		out = append(out, RankedPattern{
			Pattern:     p,
			Score:       PatternScore{Similarity: sim, Utility: util, Confidence: conf, ScopeMatch: sm, FinalScore: final},
			EvidenceIDs: evIDs,
		})
	}

	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Score.FinalScore == out[j].Score.FinalScore {
			if out[i].Score.Similarity == out[j].Score.Similarity {
				return out[i].Pattern.ID < out[j].Pattern.ID
			}
			return out[i].Score.Similarity > out[j].Score.Similarity
		}
		return out[i].Score.FinalScore > out[j].Score.FinalScore
	})
	if topK > 0 && len(out) > topK {
		out = out[:topK]
	}
	return out, nil
}

func (r *PatternRetriever) evidenceIDs(ctx context.Context, tenantID, patternID string) ([]string, error) {
	evs, err := r.store.ListEvidence(ctx, tenantID, patternID)
	if err != nil {
		return nil, fmt.Errorf("list pattern evidence %s: %w", patternID, err)
	}
	ids := make([]string, 0, len(evs))
	for _, ev := range evs {
		ids = append(ids, ev.ExperienceID)
	}
	return ids, nil
}
