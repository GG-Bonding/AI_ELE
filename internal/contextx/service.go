package contextx

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/agent-experience-engine/agent-experience-engine/internal/learning"
	"github.com/agent-experience-engine/agent-experience-engine/internal/retrieval"
	"github.com/agent-experience-engine/agent-experience-engine/internal/selector"
	"github.com/google/uuid"
)

// Retriever is the retrieval port used by the context service.
type Retriever interface {
	Retrieve(ctx context.Context, q retrieval.Query) ([]retrieval.RankedExperience, error)
}

// PatternSource retrieves generalized patterns for context (V2.1-2).
type PatternSource interface {
	RetrievePatterns(ctx context.Context, q retrieval.Query) ([]retrieval.RankedPattern, error)
}

// UsageRecorder records which experiences entered context for an episode.
type UsageRecorder interface {
	RecordUsages(ctx context.Context, in learning.RecordInput) error
}

// ConflictLookup resolves unresolved experience conflicts for selection (V2-5).
type ConflictLookup interface {
	ConflictPeers(ctx context.Context, tenantID string, experienceIDs []string) (map[string]string, error)
}

// Service orchestrates Retrieve → Select → Build for agent context.
type Service struct {
	retriever Retriever
	patterns  PatternSource
	selector  *selector.Selector
	builder   *Builder
	usages    UsageRecorder
	conflicts ConflictLookup
	snapshots SnapshotStore
	now       func() time.Time
	id        func() string
}

// NewService constructs a context service.
func NewService(retriever Retriever, sel *selector.Selector, builder *Builder) (*Service, error) {
	return NewServiceWithUsage(retriever, sel, builder, nil)
}

// NewServiceWithUsage constructs a context service that can record experience usages.
func NewServiceWithUsage(retriever Retriever, sel *selector.Selector, builder *Builder, usages UsageRecorder) (*Service, error) {
	if retriever == nil {
		return nil, fmt.Errorf("retriever is required")
	}
	if sel == nil {
		return nil, fmt.Errorf("selector is required")
	}
	if builder == nil {
		return nil, fmt.Errorf("context builder is required")
	}
	return &Service{
		retriever: retriever,
		selector:  sel,
		builder:   builder,
		usages:    usages,
		now:       time.Now().UTC,
		id:        func() string { return "ctx_" + uuid.NewString() },
	}, nil
}

// WithConflicts attaches a conflict lookup used to BLOCK unresolved opposing experiences.
func (s *Service) WithConflicts(lookup ConflictLookup) *Service {
	s.conflicts = lookup
	return s
}

// WithPatterns attaches a pattern retriever so ACTIVE patterns enter context (V2.1-2).
func (s *Service) WithPatterns(src PatternSource) *Service {
	s.patterns = src
	return s
}

// WithSnapshots persists context builds so Actions can bind provenance (V2.2-2).
func (s *Service) WithSnapshots(store SnapshotStore) *Service {
	s.snapshots = store
	return s
}

// Request is a context build request.
type Request struct {
	TenantID       string
	AgentID        string
	UserID         string
	EpisodeID      string // optional; when set, KEEP/COMPRESS context entries are tracked
	Task           string
	Tools          []string
	MaxExperiences int
	MaxPatterns    int
	MaxTokens      int
	TopK           int
}

// Response includes the safe context payload plus selection diagnostics.
type Response struct {
	ContextID  string            `json:"context_id,omitempty"`
	Context    Payload           `json:"context"`
	Selections []selector.Result `json:"selections,omitempty"`
}

// BuildContext retrieves, selects, and builds untrusted experience context.
func (s *Service) BuildContext(ctx context.Context, req Request) (Response, error) {
	if strings.TrimSpace(req.TenantID) == "" {
		return Response{}, fmt.Errorf("tenant_id is required")
	}
	if strings.TrimSpace(req.Task) == "" {
		return Response{}, fmt.Errorf("task is required")
	}

	topK := req.TopK
	if topK <= 0 {
		if req.MaxExperiences > 0 {
			topK = req.MaxExperiences * 4
		} else {
			topK = 20
		}
	}

	q := retrieval.Query{
		TenantID: req.TenantID,
		Task:     req.Task,
		AgentID:  req.AgentID,
		UserID:   req.UserID,
		Tools:    req.Tools,
		TopK:     topK,
	}

	ranked, err := s.retriever.Retrieve(ctx, q)
	if err != nil {
		return Response{}, fmt.Errorf("retrieve for context: %w", err)
	}

	var rankedPatterns []retrieval.RankedPattern
	if s.patterns != nil {
		patTopK := req.MaxPatterns
		if patTopK <= 0 {
			patTopK = s.builder.Config().MaxPatterns
		}
		pq := q
		pq.TopK = patTopK
		rankedPatterns, err = s.patterns.RetrievePatterns(ctx, pq)
		if err != nil {
			return Response{}, fmt.Errorf("retrieve patterns for context: %w", err)
		}
	}

	conflictPeers := map[string]string(nil)
	if s.conflicts != nil && len(ranked) > 0 {
		ids := make([]string, 0, len(ranked))
		for _, item := range ranked {
			ids = append(ids, item.Experience.ID)
		}
		peers, err := s.conflicts.ConflictPeers(ctx, req.TenantID, ids)
		if err != nil {
			return Response{}, fmt.Errorf("lookup experience conflicts: %w", err)
		}
		conflictPeers = peers
	}
	selected := s.selector.SelectWithOptions(req.Task, ranked, selector.SelectOptions{ConflictPeers: conflictPeers})

	builder := s.builder
	if req.MaxExperiences > 0 || req.MaxTokens > 0 || req.MaxPatterns > 0 {
		cfg := s.builder.Config()
		if req.MaxExperiences > 0 {
			cfg.MaxExperiences = req.MaxExperiences
		}
		if req.MaxPatterns > 0 {
			cfg.MaxPatterns = req.MaxPatterns
		}
		if req.MaxTokens > 0 {
			cfg.MaxTokens = req.MaxTokens
		}
		builder = New(cfg)
	}

	payload, err := builder.BuildWithPatterns(selected, rankedPatterns)
	if err != nil {
		return Response{}, fmt.Errorf("build context: %w", err)
	}

	expIDs := make([]string, 0, len(payload.Experiences))
	for _, item := range payload.Experiences {
		expIDs = append(expIDs, item.Source)
	}
	patIDs := make([]string, 0, len(payload.Patterns))
	for _, item := range payload.Patterns {
		patIDs = append(patIDs, item.ID)
	}

	if s.usages != nil && strings.TrimSpace(req.EpisodeID) != "" {
		patternRecords := make([]learning.PatternRecord, 0, len(payload.Patterns))
		byID := map[string]retrieval.RankedPattern{}
		for _, rp := range rankedPatterns {
			byID[rp.Pattern.ID] = rp
		}
		for _, item := range payload.Patterns {
			rec := learning.PatternRecord{PatternID: item.ID}
			if rp, ok := byID[item.ID]; ok {
				rec.RetrievalScore = rp.Score.Similarity
				rec.FinalScore = rp.Score.FinalScore
			} else {
				rec.FinalScore = item.FinalScore
			}
			patternRecords = append(patternRecords, rec)
		}
		if err := s.usages.RecordUsages(ctx, learning.RecordInput{
			TenantID:   req.TenantID,
			EpisodeID:  req.EpisodeID,
			Selections: selected,
			ContextIDs: expIDs,
			Patterns:   patternRecords,
		}); err != nil {
			return Response{}, fmt.Errorf("record experience usages for episode %s: %w", req.EpisodeID, err)
		}
	}

	out := Response{Context: payload, Selections: selected}
	if s.snapshots != nil {
		snap := Snapshot{
			ID:            s.id(),
			TenantID:      strings.TrimSpace(req.TenantID),
			EpisodeID:     strings.TrimSpace(req.EpisodeID),
			AgentID:       strings.TrimSpace(req.AgentID),
			UserID:        strings.TrimSpace(req.UserID),
			Task:          strings.TrimSpace(req.Task),
			ExperienceIDs: expIDs,
			PatternIDs:    patIDs,
			CreatedAt:     s.now(),
		}
		created, err := s.snapshots.Create(ctx, snap)
		if err != nil {
			return Response{}, fmt.Errorf("persist context snapshot: %w", err)
		}
		out.ContextID = created.ID
	}
	return out, nil
}

// GetSnapshot returns a persisted context snapshot (V2.2-2 provenance).
func (s *Service) GetSnapshot(ctx context.Context, tenantID, contextID string) (Snapshot, error) {
	if s.snapshots == nil {
		return Snapshot{}, ErrSnapshotNotFound
	}
	return s.snapshots.Get(ctx, tenantID, contextID)
}
