package evolution

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/agent-experience-engine/agent-experience-engine/internal/experience"
	"github.com/agent-experience-engine/agent-experience-engine/internal/retrieval"
)

var (
	// ErrNotImplemented is returned for V2-only evolution operations.
	ErrNotImplemented = errors.New("evolution operation not implemented in v1")
)

// ExperienceEvolution is the V1 evolution surface.
// Merge/Generalize are reserved for V2; only Supersede and Decay are implemented.
type ExperienceEvolution interface {
	Merge(ctx context.Context, tenantID string, experienceIDs []string) (experience.Experience, error)
	Generalize(ctx context.Context, tenantID, experienceID string) (experience.Experience, error)
	Supersede(ctx context.Context, tenantID, oldID, newID string) error
	Decay(exp experience.Experience, now time.Time) float64
}

// Service implements ExperienceEvolution for V1.
type Service struct {
	repo experience.Repository
	rank retrieval.RankConfig
	now  func() time.Time
}

// New constructs a V1 evolution service.
func New(repo experience.Repository, rank retrieval.RankConfig) (*Service, error) {
	if repo == nil {
		return nil, fmt.Errorf("experience repository is required")
	}
	if rank.DefaultLambda == 0 && len(rank.TypeLambda) == 0 {
		rank = retrieval.DefaultRankConfig()
	}
	return &Service{
		repo: repo,
		rank: rank,
		now:  time.Now().UTC,
	}, nil
}

// Merge is reserved for V2.
func (s *Service) Merge(context.Context, string, []string) (experience.Experience, error) {
	return experience.Experience{}, fmt.Errorf("%w: Merge", ErrNotImplemented)
}

// Generalize is reserved for V2.
func (s *Service) Generalize(context.Context, string, string) (experience.Experience, error) {
	return experience.Experience{}, fmt.Errorf("%w: Generalize", ErrNotImplemented)
}

// Supersede marks oldID DEPRECATED and links newID.supersedes_id = oldID.
func (s *Service) Supersede(ctx context.Context, tenantID, oldID, newID string) error {
	if err := requireIDs(tenantID, oldID, newID); err != nil {
		return err
	}
	if oldID == newID {
		return fmt.Errorf("%w: old and new experience ids must differ", experience.ErrInvalidInput)
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

// Decay returns Freshness ∈ [0,1] for an experience at now (unused age declines over time).
func (s *Service) Decay(exp experience.Experience, now time.Time) float64 {
	if now.IsZero() {
		now = s.now()
	}
	return retrieval.Freshness(exp, s.rank, now)
}

func requireIDs(tenantID, oldID, newID string) error {
	if strings.TrimSpace(tenantID) == "" {
		return fmt.Errorf("%w: tenant_id is required", experience.ErrInvalidInput)
	}
	if strings.TrimSpace(oldID) == "" {
		return fmt.Errorf("%w: old experience id is required", experience.ErrInvalidInput)
	}
	if strings.TrimSpace(newID) == "" {
		return fmt.Errorf("%w: replacement experience id is required", experience.ErrInvalidInput)
	}
	return nil
}
