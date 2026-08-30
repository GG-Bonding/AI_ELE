package feedback

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// EpisodeChecker verifies the episode exists for the tenant before accepting feedback.
type EpisodeChecker interface {
	EpisodeExists(ctx context.Context, tenantID, episodeID string) (bool, error)
}

// UtilityLearner applies ONE feedback's reward to experiences used by that episode.
type UtilityLearner interface {
	ApplyFeedbackReward(ctx context.Context, tenantID, episodeID, feedbackID string, reward, confidence float64) ([]UtilityChange, error)
}

// UtilityChange is a compact learning result exposed on feedback submit.
type UtilityChange struct {
	ExperienceID    string  `json:"experience_id"`
	OldUtility      float64 `json:"old_utility"`
	NewUtility      float64 `json:"new_utility"`
	Credit          float64 `json:"credit"`
	EffectiveReward float64 `json:"effective_reward,omitempty"`
	AlreadyApplied  bool    `json:"already_applied,omitempty"`
}

// Service collects, normalizes, and aggregates feedback.
type Service struct {
	repo     Repository
	episodes EpisodeChecker
	actions  ActionVerifier // optional; validates ACTION / ACTION_FIELD targets
	engine   *RewardEngine
	learner  UtilityLearner // optional
	now      func() time.Time
	id       func() string
}

// NewService constructs a feedback service.
func NewService(repo Repository, episodes EpisodeChecker, engine *RewardEngine) *Service {
	return NewServiceWithLearner(repo, episodes, engine, nil)
}

// NewServiceWithLearner constructs a feedback service that can update experience utilities.
func NewServiceWithLearner(repo Repository, episodes EpisodeChecker, engine *RewardEngine, learner UtilityLearner) *Service {
	if engine == nil {
		engine = NewRewardEngine(nil)
	}
	return &Service{
		repo:     repo,
		episodes: episodes,
		engine:   engine,
		learner:  learner,
		now:      time.Now().UTC,
		id:       func() string { return uuid.NewString() },
	}
}

// WithActionVerifier attaches an optional action ownership checker for targeted feedback.
func (s *Service) WithActionVerifier(v ActionVerifier) *Service {
	s.actions = v
	return s
}


// ActionVerifier optionally confirms an action_id belongs to the feedback episode.
type ActionVerifier interface {
	ActionInEpisode(ctx context.Context, tenantID, episodeID, actionID string) (bool, error)
}

// SubmitInput is one incoming feedback signal.
type SubmitInput struct {
	TenantID       string
	EpisodeID      string
	Source         string
	Signal         string
	Reward         *float64 // optional when Signal maps to a reward
	Confidence     float64
	Evidence       string
	Target         *Target
	IdempotencyKey string
}

// SubmitResult returns the persisted raw feedback and current episode aggregate.
type SubmitResult struct {
	Feedback       Feedback        `json:"feedback"`
	EpisodeReward  EpisodeReward   `json:"episode_reward"`
	UtilityUpdates []UtilityChange `json:"utility_updates,omitempty"`
	IdempotentReplay bool          `json:"idempotent_replay,omitempty"`
}

// Submit validates, normalizes, persists raw feedback, then learns from THIS row only.
// Aggregate reward is for display; it is never replayed into utility updates.
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

	key := strings.TrimSpace(in.IdempotencyKey)
	if key != "" {
		if prior, err := s.repo.GetByIdempotencyKey(ctx, in.TenantID, key); err == nil {
			return s.replaySubmit(ctx, prior, true)
		} else if !errors.Is(err, ErrNotFound) {
			return SubmitResult{}, fmt.Errorf("lookup idempotency key: %w", err)
		}
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

	if err := ValidateTarget(in.Target); err != nil {
		return SubmitResult{}, err
	}
	if err := s.verifyTargetAction(ctx, in.TenantID, in.EpisodeID, in.Target); err != nil {
		return SubmitResult{}, err
	}

	fb := Feedback{
		ID:             s.id(),
		TenantID:       strings.TrimSpace(in.TenantID),
		EpisodeID:      strings.TrimSpace(in.EpisodeID),
		Source:         source,
		Signal:         strings.TrimSpace(in.Signal),
		Reward:         reward,
		Confidence:     confidence,
		Evidence:       in.Evidence,
		Target:         in.Target,
		IdempotencyKey: key,
		CreatedAt:      s.now(),
	}

	created, err := s.repo.Create(ctx, fb)
	if err != nil {
		// concurrent idempotent insert: return original
		if key != "" {
			if prior, lookupErr := s.repo.GetByIdempotencyKey(ctx, in.TenantID, key); lookupErr == nil {
				return s.replaySubmit(ctx, prior, true)
			}
		}
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

	result := SubmitResult{Feedback: created, EpisodeReward: agg}
	if s.learner != nil {
		// Learn from THIS feedback only (reward + confidence paired).
		updates, learnErr := s.learner.ApplyFeedbackReward(
			ctx, in.TenantID, in.EpisodeID, created.ID, created.Reward, created.Confidence,
		)
		if learnErr != nil {
			return SubmitResult{}, fmt.Errorf("apply utility learning for feedback %s: %w", created.ID, learnErr)
		}
		result.UtilityUpdates = updates
	}
	return result, nil
}

func (s *Service) replaySubmit(ctx context.Context, prior Feedback, idempotent bool) (SubmitResult, error) {
	rows, err := s.repo.ListByEpisode(ctx, prior.TenantID, prior.EpisodeID)
	if err != nil {
		return SubmitResult{}, fmt.Errorf("list feedback for episode %s: %w", prior.EpisodeID, err)
	}
	agg, err := s.engine.Aggregate(prior.TenantID, prior.EpisodeID, rows)
	if err != nil {
		return SubmitResult{}, fmt.Errorf("aggregate reward for episode %s: %w", prior.EpisodeID, err)
	}
	result := SubmitResult{Feedback: prior, EpisodeReward: agg, IdempotentReplay: idempotent}
	if s.learner != nil {
		updates, learnErr := s.learner.ApplyFeedbackReward(
			ctx, prior.TenantID, prior.EpisodeID, prior.ID, prior.Reward, prior.Confidence,
		)
		if learnErr != nil {
			return SubmitResult{}, fmt.Errorf("replay learning for feedback %s: %w", prior.ID, learnErr)
		}
		result.UtilityUpdates = updates
	}
	return result, nil
}

// GetEpisodeReward recomputes weighted reward from all raw feedback (display only).
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

func (s *Service) verifyTargetAction(ctx context.Context, tenantID, episodeID string, t *Target) error {
	if t == nil || s.actions == nil {
		return nil
	}
	if t.Type != TargetAction && t.Type != TargetActionField {
		return nil
	}
	ok, err := s.actions.ActionInEpisode(ctx, tenantID, episodeID, t.ActionID)
	if err != nil {
		return fmt.Errorf("verify action target %s: %w", t.ActionID, err)
	}
	if !ok {
		return fmt.Errorf("%w: action %s not found in episode %s", ErrNotFound, t.ActionID, episodeID)
	}
	return nil
}
