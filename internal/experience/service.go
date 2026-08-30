package experience

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Service owns experience create/get/search rules.
type Service struct {
	repo Repository
	now  func() time.Time
	id   func() string
}

// NewService constructs an experience service.
func NewService(repo Repository) *Service {
	return &Service{
		repo: repo,
		now:  time.Now().UTC,
		id:   func() string { return uuid.NewString() },
	}
}

// CreateInput creates a stored experience (embedding provided by caller).
type CreateInput struct {
	TenantID        string
	Type            Type
	Scope           Scope
	ScopeKey        string
	Trigger         string
	Content         string
	SourceEpisodeID string
	Evidence        Evidence
	Confidence      float64
	Status          Status
	Embedding       []float32
	SupersedesID    *string
}

// Create validates and persists an experience.
func (s *Service) Create(ctx context.Context, in CreateInput) (Experience, error) {
	if err := requireNonEmpty("tenant_id", in.TenantID, "trigger", in.Trigger, "content", in.Content); err != nil {
		return Experience{}, err
	}
	if !in.Type.Valid() {
		return Experience{}, fmt.Errorf("%w: invalid type %q", ErrInvalidInput, in.Type)
	}
	if !in.Scope.Valid() {
		return Experience{}, fmt.Errorf("%w: invalid scope %q", ErrInvalidInput, in.Scope)
	}
	status := in.Status
	if status == "" {
		status = StatusActive
	}
	if !status.Valid() {
		return Experience{}, fmt.Errorf("%w: invalid status %q", ErrInvalidInput, status)
	}
	if in.Confidence < 0 || in.Confidence > 1 {
		return Experience{}, fmt.Errorf("%w: confidence out of range", ErrInvalidInput)
	}
	if len(in.Embedding) == 0 {
		return Experience{}, fmt.Errorf("%w: embedding is required", ErrInvalidInput)
	}

	now := s.now()
	exp := Experience{
		ID:              s.id(),
		TenantID:        strings.TrimSpace(in.TenantID),
		Type:            in.Type,
		Scope:           in.Scope,
		ScopeKey:        strings.TrimSpace(in.ScopeKey),
		Trigger:         strings.TrimSpace(in.Trigger),
		Content:         strings.TrimSpace(in.Content),
		SourceEpisodeID: strings.TrimSpace(in.SourceEpisodeID),
		Evidence:        in.Evidence,
		Confidence:      in.Confidence,
		Utility:         0.5,
		Alpha:           1,
		Beta:            1,
		Status:          status,
		Version:         1,
		SupersedesID:    in.SupersedesID,
		Embedding:       append([]float32(nil), in.Embedding...),
		CreatedAt:       now,
		UpdatedAt:       now,
	}

	created, err := s.repo.Create(ctx, exp)
	if err != nil {
		return Experience{}, fmt.Errorf("create experience: %w", err)
	}
	return created, nil
}

// Get returns one experience for a tenant.
func (s *Service) Get(ctx context.Context, tenantID, id string) (Experience, error) {
	if err := requireNonEmpty("tenant_id", tenantID, "id", id); err != nil {
		return Experience{}, err
	}
	exp, err := s.repo.Get(ctx, tenantID, id)
	if err != nil {
		return Experience{}, fmt.Errorf("get experience %s: %w", id, err)
	}
	return exp, nil
}

// SearchInput is the application-level search request (embedding already computed).
type SearchInput struct {
	TenantID       string
	Types          []Type
	Scopes         []Scope
	ScopeKey       string
	Statuses       []Status
	AgentID        string
	UserID         string
	Tools          []string
	QueryEmbedding []float32
	TopK           int
}

// Search runs metadata-filtered vector similarity retrieval.
func (s *Service) Search(ctx context.Context, in SearchInput) ([]ScoredExperience, error) {
	if err := requireNonEmpty("tenant_id", in.TenantID); err != nil {
		return nil, err
	}
	if len(in.QueryEmbedding) == 0 {
		return nil, fmt.Errorf("%w: query embedding is required", ErrInvalidInput)
	}
	topK := in.TopK
	if topK <= 0 {
		topK = 20
	}

	results, err := s.repo.Search(ctx, SearchFilter{
		TenantID:       in.TenantID,
		Types:          in.Types,
		Scopes:         in.Scopes,
		ScopeKey:       in.ScopeKey,
		Statuses:       in.Statuses,
		AgentID:        in.AgentID,
		UserID:         in.UserID,
		Tools:          in.Tools,
		QueryEmbedding: in.QueryEmbedding,
		TopK:           topK,
	})
	if err != nil {
		return nil, fmt.Errorf("search experiences for tenant %s: %w", in.TenantID, err)
	}
	return results, nil
}

// Supersede marks oldID as DEPRECATED and links newID.supersedes_id = oldID.
func (s *Service) Supersede(ctx context.Context, tenantID, oldID, newID string) error {
	if err := requireNonEmpty("tenant_id", tenantID, "old_id", oldID, "replacement_id", newID); err != nil {
		return err
	}
	if strings.TrimSpace(oldID) == strings.TrimSpace(newID) {
		return fmt.Errorf("%w: old and replacement ids must differ", ErrInvalidInput)
	}
	if _, err := s.repo.Get(ctx, tenantID, oldID); err != nil {
		return fmt.Errorf("get superseded experience %s: %w", oldID, err)
	}
	if _, err := s.repo.Get(ctx, tenantID, newID); err != nil {
		return fmt.Errorf("get replacement experience %s: %w", newID, err)
	}
	if err := s.repo.Supersede(ctx, tenantID, oldID, newID); err != nil {
		return fmt.Errorf("supersede experience %s with %s: %w", oldID, newID, err)
	}
	return nil
}

// StatusFromConfidence applies V1 evaluator thresholds to candidates before store.
func StatusFromConfidence(confidence, activeMin, candidateMin float64) (Status, bool) {
	if confidence >= activeMin {
		return StatusActive, true
	}
	if confidence >= candidateMin {
		return StatusCandidate, true
	}
	return "", false
}

func requireNonEmpty(pairs ...string) error {
	if len(pairs)%2 != 0 {
		return fmt.Errorf("requireNonEmpty: odd argument count")
	}
	for i := 0; i < len(pairs); i += 2 {
		if strings.TrimSpace(pairs[i+1]) == "" {
			return fmt.Errorf("%w: %s is required", ErrInvalidInput, pairs[i])
		}
	}
	return nil
}
