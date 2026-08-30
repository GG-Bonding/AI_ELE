package learning

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/agent-experience-engine/agent-experience-engine/internal/attribution"
	"github.com/agent-experience-engine/agent-experience-engine/internal/experience"
	"github.com/agent-experience-engine/agent-experience-engine/internal/selector"
	"github.com/google/uuid"
)

// Service records experience usage and applies incremental feedback learning events.
type Service struct {
	usages      experience.UsageRepository
	experiences experience.Repository
	events      EventRepository
	strategy    attribution.Strategy
	now         func() time.Time
	id          func() string
}

// New constructs a learning service.
func New(usages experience.UsageRepository, experiences experience.Repository, strategy attribution.Strategy) (*Service, error) {
	return NewWithEvents(usages, experiences, NewMemoryEventRepository(), strategy)
}

// NewWithEvents constructs a learning service with a persistent event ledger.
func NewWithEvents(
	usages experience.UsageRepository,
	experiences experience.Repository,
	events EventRepository,
	strategy attribution.Strategy,
) (*Service, error) {
	if usages == nil {
		return nil, fmt.Errorf("usage repository is required")
	}
	if experiences == nil {
		return nil, fmt.Errorf("experience repository is required")
	}
	if events == nil {
		return nil, fmt.Errorf("learning event repository is required")
	}
	if strategy == nil {
		strategy = attribution.NewDefault()
	}
	return &Service{
		usages:      usages,
		experiences: experiences,
		events:      events,
		strategy:    strategy,
		now:         time.Now().UTC,
		id:          func() string { return uuid.NewString() },
	}, nil
}

// RecordInput captures KEEP/COMPRESS experiences that entered context for an episode.
type RecordInput struct {
	TenantID   string
	EpisodeID  string
	Selections []selector.Result
	ContextIDs []string
}

// UtilityUpdate is one experience utility change after learning.
type UtilityUpdate struct {
	ExperienceID    string  `json:"experience_id"`
	LearningEventID string  `json:"learning_event_id,omitempty"`
	Credit          float64 `json:"credit"`
	Reward          float64 `json:"experience_reward"`
	EffectiveReward float64 `json:"effective_reward"`
	OldUtility      float64 `json:"old_utility"`
	NewUtility      float64 `json:"new_utility"`
	Alpha           float64 `json:"alpha"`
	Beta            float64 `json:"beta"`
	AlreadyApplied  bool    `json:"already_applied,omitempty"`
}

// RecordUsages persists usage rows for experiences that entered context.
func (s *Service) RecordUsages(ctx context.Context, in RecordInput) ([]experience.Usage, error) {
	if strings.TrimSpace(in.TenantID) == "" || strings.TrimSpace(in.EpisodeID) == "" {
		return nil, fmt.Errorf("tenant_id and episode_id are required to record usage")
	}
	allowed := map[string]struct{}{}
	for _, id := range in.ContextIDs {
		allowed[id] = struct{}{}
	}

	now := s.now()
	var created []experience.Usage
	for _, sel := range in.Selections {
		if sel.Decision != selector.DecisionKeep && sel.Decision != selector.DecisionCompress {
			continue
		}
		expID := sel.Experience.Experience.ID
		if len(allowed) > 0 {
			if _, ok := allowed[expID]; !ok {
				continue
			}
		}
		u := experience.Usage{
			ID:                s.id(),
			TenantID:          in.TenantID,
			EpisodeID:         in.EpisodeID,
			ExperienceID:      expID,
			RetrievalScore:    sel.Experience.Score.Similarity,
			SelectionDecision: string(sel.Decision),
			FinalScore:        sel.Experience.Score.FinalScore,
			UsedAt:            now,
		}
		row, err := s.usages.Create(ctx, u)
		if err != nil {
			return created, fmt.Errorf("create usage for experience %s: %w", expID, err)
		}
		created = append(created, row)

		exp, err := s.experiences.Get(ctx, in.TenantID, expID)
		if err != nil {
			return created, fmt.Errorf("get experience %s for use_count: %w", expID, err)
		}
		exp.UseCount++
		t := now
		exp.LastUsedAt = &t
		exp.UpdatedAt = now
		if _, err := s.experiences.Update(ctx, exp); err != nil {
			return created, fmt.Errorf("update use_count for experience %s: %w", expID, err)
		}
	}
	return created, nil
}

// ApplyFeedbackReward attributes THIS feedback's reward only (incremental LearningEvent).
// Replays of the same feedback_id are no-ops that return already-applied updates.
func (s *Service) ApplyFeedbackReward(
	ctx context.Context,
	tenantID, episodeID, feedbackID string,
	reward, confidence float64,
) ([]UtilityUpdate, error) {
	if strings.TrimSpace(tenantID) == "" || strings.TrimSpace(episodeID) == "" || strings.TrimSpace(feedbackID) == "" {
		return nil, fmt.Errorf("tenant_id, episode_id, and feedback_id are required")
	}

	existing, err := s.events.ListByFeedback(ctx, tenantID, feedbackID)
	if err != nil {
		return nil, fmt.Errorf("list learning events for feedback %s: %w", feedbackID, err)
	}
	if len(existing) > 0 {
		return s.replayApplied(ctx, tenantID, existing)
	}

	usages, err := s.usages.ListByEpisode(ctx, tenantID, episodeID)
	if err != nil {
		return nil, fmt.Errorf("list usages for episode %s: %w", episodeID, err)
	}
	if len(usages) == 0 {
		return nil, nil
	}

	credits, err := s.strategy.Attribute(usages, reward)
	if err != nil {
		return nil, fmt.Errorf("attribute reward for feedback %s: %w", feedbackID, err)
	}
	if err := attribution.ValidateCredits(credits); err != nil {
		return nil, fmt.Errorf("invalid attribution for feedback %s: %w", feedbackID, err)
	}

	now := s.now()
	updates := make([]UtilityUpdate, 0, len(credits))
	for _, c := range credits {
		effective := experience.EffectiveReward(reward, confidence, c.Weight)
		ev := Event{
			ID:               s.id(),
			TenantID:         tenantID,
			FeedbackID:       feedbackID,
			EpisodeID:        episodeID,
			ExperienceID:     c.ExperienceID,
			NormalizedReward: reward,
			Confidence:       confidence,
			Credit:           c.Weight,
			EffectiveReward:  effective,
			Status:           EventPending,
			CreatedAt:        now,
		}
		created, err := s.events.Create(ctx, ev)
		if err != nil {
			if errors.Is(err, ErrDuplicateEvent) {
				// concurrent create: treat as already handled
				dup, getErr := s.events.GetByFeedbackExperience(ctx, tenantID, feedbackID, c.ExperienceID)
				if getErr != nil {
					return updates, getErr
				}
				u, replayErr := s.snapshotUpdate(ctx, tenantID, dup)
				if replayErr != nil {
					return updates, replayErr
				}
				updates = append(updates, u)
				continue
			}
			return updates, fmt.Errorf("create learning event for experience %s: %w", c.ExperienceID, err)
		}

		expReward := reward * c.Weight
		var (
			updated experience.Experience
			oldUtil float64
		)
		const maxAttempts = 8
		for attempt := 0; attempt < maxAttempts; attempt++ {
			exp, err := s.experiences.Get(ctx, tenantID, c.ExperienceID)
			if err != nil {
				_ = s.events.MarkFailed(ctx, tenantID, created.ID)
				return updates, fmt.Errorf("get experience %s: %w", c.ExperienceID, err)
			}
			oldUtil = exp.Utility
			// experience reward for beta update is reward*credit; confidence applied inside ApplyBetaUpdate
			updated, err = experience.ApplyBetaUpdate(exp, expReward, confidence, now)
			if err != nil {
				_ = s.events.MarkFailed(ctx, tenantID, created.ID)
				return updates, fmt.Errorf("beta update experience %s: %w", c.ExperienceID, err)
			}
			if _, err := s.experiences.Update(ctx, updated); err != nil {
				if errors.Is(err, experience.ErrConflict) && attempt+1 < maxAttempts {
					continue
				}
				_ = s.events.MarkFailed(ctx, tenantID, created.ID)
				return updates, fmt.Errorf("persist utility for experience %s: %w", c.ExperienceID, err)
			}
			break
		}
		if err := s.events.MarkApplied(ctx, tenantID, created.ID, now); err != nil {
			return updates, fmt.Errorf("mark learning event applied: %w", err)
		}
		updates = append(updates, UtilityUpdate{
			ExperienceID:    c.ExperienceID,
			LearningEventID: created.ID,
			Credit:          c.Weight,
			Reward:          expReward,
			EffectiveReward: effective,
			OldUtility:      oldUtil,
			NewUtility:      updated.Utility,
			Alpha:           updated.Alpha,
			Beta:            updated.Beta,
		})
	}
	return updates, nil
}

func (s *Service) replayApplied(ctx context.Context, tenantID string, events []Event) ([]UtilityUpdate, error) {
	out := make([]UtilityUpdate, 0, len(events))
	for _, ev := range events {
		u, err := s.snapshotUpdate(ctx, tenantID, ev)
		if err != nil {
			return nil, err
		}
		u.AlreadyApplied = true
		out = append(out, u)
	}
	return out, nil
}

func (s *Service) snapshotUpdate(ctx context.Context, tenantID string, ev Event) (UtilityUpdate, error) {
	exp, err := s.experiences.Get(ctx, tenantID, ev.ExperienceID)
	if err != nil {
		return UtilityUpdate{}, err
	}
	return UtilityUpdate{
		ExperienceID:    ev.ExperienceID,
		LearningEventID: ev.ID,
		Credit:          ev.Credit,
		Reward:          ev.NormalizedReward * ev.Credit,
		EffectiveReward: ev.EffectiveReward,
		OldUtility:      exp.Utility,
		NewUtility:      exp.Utility,
		Alpha:           exp.Alpha,
		Beta:            exp.Beta,
		AlreadyApplied:  ev.Status == EventApplied,
	}, nil
}
