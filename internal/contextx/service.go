package contextx

import (
	"context"
	"fmt"
	"strings"

	"github.com/agent-experience-engine/agent-experience-engine/internal/learning"
	"github.com/agent-experience-engine/agent-experience-engine/internal/retrieval"
	"github.com/agent-experience-engine/agent-experience-engine/internal/selector"
)

// Retriever is the retrieval port used by the context service.
type Retriever interface {
	Retrieve(ctx context.Context, q retrieval.Query) ([]retrieval.RankedExperience, error)
}

// UsageRecorder records which experiences entered context for an episode.
type UsageRecorder interface {
	RecordUsages(ctx context.Context, in learning.RecordInput) error
}

// Service orchestrates Retrieve → Select → Build for agent context.
type Service struct {
	retriever Retriever
	selector  *selector.Selector
	builder   *Builder
	usages    UsageRecorder
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
	return &Service{retriever: retriever, selector: sel, builder: builder, usages: usages}, nil
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
	MaxTokens      int
	TopK           int
}

// Response includes the safe context payload plus selection diagnostics.
type Response struct {
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

	ranked, err := s.retriever.Retrieve(ctx, retrieval.Query{
		TenantID: req.TenantID,
		Task:     req.Task,
		AgentID:  req.AgentID,
		UserID:   req.UserID,
		Tools:    req.Tools,
		TopK:     topK,
	})
	if err != nil {
		return Response{}, fmt.Errorf("retrieve for context: %w", err)
	}

	selected := s.selector.Select(req.Task, ranked)

	builder := s.builder
	if req.MaxExperiences > 0 || req.MaxTokens > 0 {
		cfg := s.builder.Config()
		if req.MaxExperiences > 0 {
			cfg.MaxExperiences = req.MaxExperiences
		}
		if req.MaxTokens > 0 {
			cfg.MaxTokens = req.MaxTokens
		}
		builder = New(cfg)
	}

	payload, err := builder.Build(selected)
	if err != nil {
		return Response{}, fmt.Errorf("build context: %w", err)
	}

	if s.usages != nil && strings.TrimSpace(req.EpisodeID) != "" {
		ids := make([]string, 0, len(payload.Experiences))
		for _, item := range payload.Experiences {
			ids = append(ids, item.Source)
		}
		if err := s.usages.RecordUsages(ctx, learning.RecordInput{
			TenantID:   req.TenantID,
			EpisodeID:  req.EpisodeID,
			Selections: selected,
			ContextIDs: ids,
		}); err != nil {
			return Response{}, fmt.Errorf("record experience usages for episode %s: %w", req.EpisodeID, err)
		}
	}

	return Response{Context: payload, Selections: selected}, nil
}
