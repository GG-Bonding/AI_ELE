package retrieval

import (
	"context"
	"fmt"
	"strings"

	"github.com/agent-experience-engine/agent-experience-engine/internal/experience"
	"github.com/agent-experience-engine/agent-experience-engine/internal/provider"
)

// Retriever performs two-phase retrieval:
// 1) semantic candidate set (metadata + vector TopK)
// 2) utility-aware ranking (FinalScore)
type Retriever struct {
	experiences *experience.Service
	embedder    provider.EmbeddingProvider
	rank        RankConfig
}

// New constructs a Retriever.
func New(experiences *experience.Service, embedder provider.EmbeddingProvider, rank RankConfig) (*Retriever, error) {
	if experiences == nil {
		return nil, fmt.Errorf("experience service is required")
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
	return &Retriever{
		experiences: experiences,
		embedder:    embedder,
		rank:        rank,
	}, nil
}

// Query is a retrieval request.
type Query struct {
	TenantID string
	Task     string
	AgentID  string
	UserID   string
	Types    []experience.Type
	Scopes   []experience.Scope
	ScopeKey string
	Tools    []string
	TopK     int
}

// Retrieve runs two-phase retrieval and returns utility-ranked experiences.
func (r *Retriever) Retrieve(ctx context.Context, q Query) ([]RankedExperience, error) {
	if strings.TrimSpace(q.TenantID) == "" {
		return nil, fmt.Errorf("%w: tenant_id is required", experience.ErrInvalidInput)
	}
	if strings.TrimSpace(q.Task) == "" {
		return nil, fmt.Errorf("%w: task is required", experience.ErrInvalidInput)
	}

	finalTopK := q.TopK
	if finalTopK <= 0 {
		finalTopK = r.rank.DefaultTopK
	}

	vectors, err := r.embedder.Embed(ctx, []string{q.Task})
	if err != nil {
		return nil, fmt.Errorf("embed retrieval task: %w", err)
	}
	if len(vectors) != 1 {
		return nil, fmt.Errorf("embed retrieval task: expected 1 vector, got %d", len(vectors))
	}

	// Phase 1: semantic candidates (auth filter applied inside Search before TopK).
	candidates, err := r.experiences.Search(ctx, experience.SearchInput{
		TenantID:       q.TenantID,
		Types:          q.Types,
		Scopes:         q.Scopes,
		ScopeKey:       q.ScopeKey,
		AgentID:        q.AgentID,
		UserID:         q.UserID,
		Tools:          q.Tools,
		QueryEmbedding: vectors[0],
		TopK:           r.rank.CandidateTopK,
	})
	if err != nil {
		return nil, fmt.Errorf("phase1 semantic retrieval: %w", err)
	}
	// Defense in depth: FilterAuthorized still applied after Search.
	candidates = FilterAuthorized(candidates, q)

	// Phase 2: utility-aware ranking (soft scope match remains a score factor).
	ranked := Rank(candidates, ScopeContext{
		AgentID:  q.AgentID,
		UserID:   q.UserID,
		ScopeKey: q.ScopeKey,
		Tools:    q.Tools,
	}, r.rank, r.rank.Now())

	if finalTopK > 0 && len(ranked) > finalTopK {
		ranked = ranked[:finalTopK]
	}
	return ranked, nil
}

// RetrieveBySimilarity runs Phase-1 semantic retrieval and ranks by Similarity only
// (no utility / freshness / scope product). Used by evaluation "raw retrieval" arm.
func (r *Retriever) RetrieveBySimilarity(ctx context.Context, q Query) ([]RankedExperience, error) {
	if strings.TrimSpace(q.TenantID) == "" {
		return nil, fmt.Errorf("%w: tenant_id is required", experience.ErrInvalidInput)
	}
	if strings.TrimSpace(q.Task) == "" {
		return nil, fmt.Errorf("%w: task is required", experience.ErrInvalidInput)
	}

	finalTopK := q.TopK
	if finalTopK <= 0 {
		finalTopK = r.rank.DefaultTopK
	}

	vectors, err := r.embedder.Embed(ctx, []string{q.Task})
	if err != nil {
		return nil, fmt.Errorf("embed retrieval task: %w", err)
	}
	if len(vectors) != 1 {
		return nil, fmt.Errorf("embed retrieval task: expected 1 vector, got %d", len(vectors))
	}

	candidates, err := r.experiences.Search(ctx, experience.SearchInput{
		TenantID:       q.TenantID,
		Types:          q.Types,
		Scopes:         q.Scopes,
		ScopeKey:       q.ScopeKey,
		AgentID:        q.AgentID,
		UserID:         q.UserID,
		Tools:          q.Tools,
		QueryEmbedding: vectors[0],
		TopK:           r.rank.CandidateTopK,
	})
	if err != nil {
		return nil, fmt.Errorf("raw semantic retrieval: %w", err)
	}
	candidates = FilterAuthorized(candidates, q)

	ranked := RankBySimilarity(candidates)
	if finalTopK > 0 && len(ranked) > finalTopK {
		ranked = ranked[:finalTopK]
	}
	return ranked, nil
}
