package feedback

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// EpisodeChecker verifies the episode exists for the tenant before accepting feedback.
type EpisodeChecker interface {
	EpisodeExists(ctx context.Context, tenantID, episodeID string) (bool, error)
}

// Service collects, normalizes, and aggregates feedback.
type Service struct {
	repo     Repository
	episodes EpisodeChecker
	engine   *RewardEngine
	now      func() time.Time
	id       func() string
}

// NewService constructs a feedback service.
func NewService(repo Repository, episodes EpisodeChecker, engine *RewardEngine) *Service {
	if engine == nil {
		engine = NewRewardEngine(nil)
	}
	return &Service{
		repo:     repo,
		episodes: episodes,
		engine:   engine,
		now:      time.Now().UTC,
		id:       func() string { return uuid.NewString() },
	}
}

// SubmitInput is one incoming feedback signal.
type SubmitInput struct {
	TenantID   string
	EpisodeID  string
	Source     string
	Signal     string
	Reward     *float64 // optional when Signal maps to a reward
	Confidence float64
	Evidence   string
}

// SubmitResult returns the persisted raw feedback and current episode aggregate.
type SubmitResult struct {
	Feedback      Feedback      `json:"feedback"`
	EpisodeReward EpisodeReward `json:"episode_reward"`
}

// Submit validates, normalizes, persists raw feedback, then recomputes weighted reward.
func (s *Service) Submit(ctx context.Context, in SubmitInput) (SubmitResult, error) {
	if err := requireNonEmpty("tenant_id", in.TenantID, "episode_id", in.EpisodeID); err != nil {
		return SubmitResult{}, err
	}
	source, err := ParseSource(in.Source)
	if err != nil {
		return SubmitResult{}, err
	}

	exists, err := s.episodes.EpisodeExists(ctx, in.TenantID, in.EpisodeID)
	if err != nil {
		return SubmitResult{}, fmt.Errorf("check episode %s: %w", in.EpisodeID, err)
	}
	if !exists {
		return SubmitResult{}, fmt.Errorf("%w: episode %s", ErrNotFound, in.EpisodeID)
	}

	reward, err := resolveReward(in)
	if err != nil {
		return SubmitResult{}, err
	}
	confidence := in.Confidence
	if confidence == 0 {
		confidence = 1.0
	}
	confidence = NormalizeConfidence(confidence)

	fb := Feedback{
		ID:         s.id(),
		TenantID:   strings.TrimSpace(in.TenantID),
		EpisodeID:  strings.TrimSpace(in.EpisodeID),
		Source:     source,
		Signal:     strings.TrimSpace(in.Signal),
		Reward:     reward,
		Confidence: confidence,
		Evidence:   in.Evidence,
		CreatedAt:  s.now(),
	}

	created, err := s.repo.Create(ctx, fb)
	if err != nil {
		return SubmitResult{}, fmt.Errorf("create feedback for episode %s: %w", in.EpisodeID, err)
	}

	rows, err := s.repo.ListByEpisode(ctx, in.TenantID, in.EpisodeID)
	if err != nil {
		return SubmitResult{}, fmt.Errorf("list feedback for episode %s: %w", in.EpisodeID, err)
	}
	agg, err := s.engine.Aggregate(in.TenantID, in.EpisodeID, rows)
	if err != nil {
		return SubmitResult{}, fmt.Errorf("aggregate reward for episode %s: %w", in.EpisodeID, err)
	}
	return SubmitResult{Feedback: created, EpisodeReward: agg}, nil
}

// GetEpisodeReward recomputes weighted reward from all raw feedback.
func (s *Service) GetEpisodeReward(ctx context.Context, tenantID, episodeID string) (EpisodeReward, []Feedback, error) {
	if err := requireNonEmpty("tenant_id", tenantID, "episode_id", episodeID); err != nil {
		return EpisodeReward{}, nil, err
	}
	rows, err := s.repo.ListByEpisode(ctx, tenantID, episodeID)
	if err != nil {
		return EpisodeReward{}, nil, fmt.Errorf("list feedback for episode %s: %w", episodeID, err)
	}
	agg, err := s.engine.Aggregate(tenantID, episodeID, rows)
	if err != nil {
		return EpisodeReward{}, rows, err
	}
	return agg, rows, nil
}

func resolveReward(in SubmitInput) (float64, error) {
	if in.Reward != nil {
		return NormalizeReward(*in.Reward), nil
	}
	if mapped, ok := SignalToReward(in.Signal); ok {
		return mapped, nil
	}
	return 0, fmt.Errorf("%w: reward or recognizable signal is required", ErrInvalidInput)
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
