package learning

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/agent-experience-engine/agent-experience-engine/internal/attribution"
	"github.com/agent-experience-engine/agent-experience-engine/internal/experience"
	"github.com/agent-experience-engine/agent-experience-engine/internal/selector"
	"github.com/google/uuid"
)

// Service records experience usage and applies attributed utility updates.
type Service struct {
	usages      experience.UsageRepository
	experiences experience.Repository
	strategy    attribution.Strategy
	now         func() time.Time
	id          func() string
}

// New constructs a learning service.
func New(usages experience.UsageRepository, experiences experience.Repository, strategy attribution.Strategy) (*Service, error) {
	if usages == nil {
		return nil, fmt.Errorf("usage repository is required")
	}
	if experiences == nil {
		return nil, fmt.Errorf("experience repository is required")
	}
	if strategy == nil {
		strategy = attribution.NewDefault()
	}
	return &Service{
		usages:      usages,
		experiences: experiences,
		strategy:    strategy,
		now:         time.Now().UTC,
		id:          func() string { return uuid.NewString() },
	}, nil
}

// RecordInput captures KEEP/ABSTRACT experiences that entered context for an episode.
type RecordInput struct {
	TenantID   string
	EpisodeID  string
	Selections []selector.Result
	ContextIDs []string // experience IDs that made it into the final context payload
}

// UtilityUpdate is one experience utility change after learning.
type UtilityUpdate struct {
	ExperienceID string  `json:"experience_id"`
	Credit       float64 `json:"credit"`
	Reward       float64 `json:"experience_reward"`
	OldUtility   float64 `json:"old_utility"`
	NewUtility   float64 `json:"new_utility"`
	Alpha        float64 `json:"alpha"`
	Beta         float64 `json:"beta"`
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
		if sel.Decision != selector.DecisionKeep && sel.Decision != selector.DecisionAbstract {
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

		// bump use_count / last_used_at
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

// ApplyEpisodeReward attributes episode reward to used experiences and updates utilities.
func (s *Service) ApplyEpisodeReward(
	ctx context.Context,
	tenantID, episodeID string,
	episodeReward, confidence float64,
) ([]UtilityUpdate, error) {
	usages, err := s.usages.ListByEpisode(ctx, tenantID, episodeID)
	if err != nil {
		return nil, fmt.Errorf("list usages for episode %s: %w", episodeID, err)
	}
	if len(usages) == 0 {
		return nil, nil
	}

	credits, err := s.strategy.Attribute(usages, episodeReward)
	if err != nil {
		return nil, fmt.Errorf("attribute reward for episode %s: %w", episodeID, err)
	}
	if err := attribution.ValidateCredits(credits); err != nil {
		return nil, fmt.Errorf("invalid attribution for episode %s: %w", episodeID, err)
	}

	now := s.now()
	updates := make([]UtilityUpdate, 0, len(credits))
	for _, c := range credits {
		exp, err := s.experiences.Get(ctx, tenantID, c.ExperienceID)
		if err != nil {
			return updates, fmt.Errorf("get experience %s: %w", c.ExperienceID, err)
		}
		old := exp.Utility
		expReward := episodeReward * c.Weight
		updated, err := experience.ApplyBetaUpdate(exp, expReward, confidence, now)
		if err != nil {
			return updates, fmt.Errorf("beta update experience %s: %w", c.ExperienceID, err)
		}
		if _, err := s.experiences.Update(ctx, updated); err != nil {
			return updates, fmt.Errorf("persist utility for experience %s: %w", c.ExperienceID, err)
		}
		updates = append(updates, UtilityUpdate{
			ExperienceID: c.ExperienceID,
			Credit:       c.Weight,
			Reward:       expReward,
			OldUtility:   old,
			NewUtility:   updated.Utility,
			Alpha:        updated.Alpha,
			Beta:         updated.Beta,
		})
	}
	return updates, nil
}
