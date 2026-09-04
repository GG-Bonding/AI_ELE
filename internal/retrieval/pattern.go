package retrieval

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/agent-experience-engine/agent-experience-engine/internal/experience"
	"github.com/agent-experience-engine/agent-experience-engine/internal/provider"
)

// PatternStore is the persistence port for pattern retrieval (V2.1-2 / V2.2-1).
type PatternStore interface {
	Search(ctx context.Context, filter experience.PatternSearchFilter) ([]experience.ScoredPattern, error)
	List(ctx context.Context, filter experience.PatternListFilter) ([]experience.Pattern, error)
	ListEvidence(ctx context.Context, tenantID, patternID string) ([]experience.PatternEvidence, error)
}

// PatternRetriever ranks ACTIVE patterns for agent context (V2.2-1).
// Primary path: task embedding → pgvector/cosine TopK →
// SemanticSimilarity × Utility × Confidence × Validity.
// Patterns without embeddings fall back to lexical overlap (legacy rows).
type PatternRetriever struct {
	store          PatternStore
	embedder       provider.EmbeddingProvider
	rank           RankConfig
	defaultTopK    int
	candidateLimit int
	minSimilarity  float64
	now            func() time.Time
}

// NewPatternRetriever constructs a semantic pattern retriever.
func NewPatternRetriever(store PatternStore, embedder provider.EmbeddingProvider, rank RankConfig) (*PatternRetriever, error) {
	if store == nil {
		return nil, fmt.Errorf("pattern store is required")
	}
	if embedder == nil {
		return nil, fmt.Errorf("embedding provider is required")
	}
	defaults := DefaultRankConfig()
	if rank.CandidateTopK <= 0 {
		rank.CandidateTopK = defaults.CandidateTopK
	}
	if rank.DefaultTopK <= 0 {
		rank.DefaultTopK = defaults.DefaultTopK
	}
	if rank.DefaultLambda == 0 {
		rank.DefaultLambda = defaults.DefaultLambda
	}
	if rank.ToolScopeLambda == 0 {
		rank.ToolScopeLambda = defaults.ToolScopeLambda
	}
	if rank.TypeLambda == nil {
		rank.TypeLambda = defaults.TypeLambda
	}
	if rank.Now == nil {
		rank.Now = defaults.Now
	}
	cand := rank.CandidateTopK
	if cand <= 0 {
		cand = 50
	}
	return &PatternRetriever{
		store:          store,
		embedder:       embedder,
		rank:           rank,
		defaultTopK:    rank.DefaultTopK,
		candidateLimit: cand,
		minSimilarity:  0.15,
		now:            rank.Now,
	}, nil
}

// PatternScore explains pattern ranking.
type PatternScore struct {
	Similarity float64 `json:"similarity"`
	Utility    float64 `json:"utility"`
	Confidence float64 `json:"confidence"`
	Validity   float64 `json:"validity"`
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

	vectors, err := r.embedder.Embed(ctx, []string{q.Task})
	if err != nil {
		return nil, fmt.Errorf("embed pattern retrieval task: %w", err)
	}
	if len(vectors) != 1 || len(vectors[0]) == 0 {
		return nil, fmt.Errorf("embed pattern retrieval task: expected 1 non-empty vector, got %d", len(vectors))
	}

	hits, err := r.store.Search(ctx, experience.PatternSearchFilter{
		TenantID:       q.TenantID,
		Statuses:       []experience.PatternStatus{experience.PatternStatusActive},
		QueryEmbedding: vectors[0],
		TopK:           r.candidateLimit,
		AgentID:        q.AgentID,
		UserID:         q.UserID,
		ScopeKey:       q.ScopeKey,
		Tools:          q.Tools,
	})
	if err != nil {
		return nil, fmt.Errorf("search active patterns: %w", err)
	}

	now := r.now()
	seen := make(map[string]struct{}, len(hits))
	out := make([]RankedPattern, 0, len(hits))
	for _, hit := range hits {
		p := hit.Pattern
		seen[p.ID] = struct{}{}
		sim := hit.Similarity
		if sim < r.minSimilarity {
			continue
		}
		ranked, err := r.rankHit(ctx, q.TenantID, p, sim, now)
		if err != nil {
			return nil, err
		}
		if ranked.Score.FinalScore <= 0 {
			continue
		}
		out = append(out, ranked)
	}

	// Legacy fallback: ACTIVE patterns without embeddings use lexical overlap.
	legacy, err := r.store.List(ctx, experience.PatternListFilter{
		TenantID: q.TenantID,
		Statuses: []experience.PatternStatus{experience.PatternStatusActive},
		Limit:    r.candidateLimit,
	})
	if err != nil {
		return nil, fmt.Errorf("list legacy patterns: %w", err)
	}
	for _, p := range legacy {
		if _, ok := seen[p.ID]; ok {
			continue
		}
		if len(p.Embedding) > 0 {
			continue
		}
		authExp := experience.Experience{Scope: p.Scope, ScopeKey: p.ScopeKey, TenantID: p.TenantID}
		if !experience.AuthorizedForSearch(authExp, q.AgentID, q.UserID, q.Tools, q.ScopeKey) {
			continue
		}
		sim := LexicalOverlap(q.Task, strings.TrimSpace(p.Trigger+" "+p.Content))
		if sim < r.minSimilarity {
			continue
		}
		ranked, err := r.rankHit(ctx, q.TenantID, p, sim, now)
		if err != nil {
			return nil, err
		}
		if ranked.Score.FinalScore <= 0 {
			continue
		}
		out = append(out, ranked)
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

func (r *PatternRetriever) rankHit(ctx context.Context, tenantID string, p experience.Pattern, sim float64, now time.Time) (RankedPattern, error) {
	util := clamp01(p.Utility)
	conf := clamp01(p.Confidence)
	valid := Validity(experience.Experience{
		Type:      p.Type,
		Scope:     p.Scope,
		UpdatedAt: p.UpdatedAt,
		CreatedAt: p.CreatedAt,
	}, r.rank, now)
	final := sim * util * conf * valid
	evIDs, err := r.evidenceIDs(ctx, tenantID, p.ID)
	if err != nil {
		return RankedPattern{}, err
	}
	return RankedPattern{
		Pattern: p,
		Score: PatternScore{
			Similarity: sim,
			Utility:    util,
			Confidence: conf,
			Validity:   valid,
			FinalScore: final,
		},
		EvidenceIDs: evIDs,
	}, nil
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
