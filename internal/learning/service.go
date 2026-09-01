package learning

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/agent-experience-engine/agent-experience-engine/internal/action"
	"github.com/agent-experience-engine/agent-experience-engine/internal/attribution"
	"github.com/agent-experience-engine/agent-experience-engine/internal/feedback"
	"github.com/agent-experience-engine/agent-experience-engine/internal/experience"
	"github.com/agent-experience-engine/agent-experience-engine/internal/selector"
	"github.com/google/uuid"
)

// Service records experience usage and applies incremental feedback learning events.
type Service struct {
	usages      experience.UsageRepository
	experiences experience.Repository
	events      EventRepository
	applier     EventApplier
	strategy    attribution.Strategy
	links       LinkLister               // optional; experience→action edges for V2 attribution
	actions     ActionLister             // optional; tool_name enrichment for TOOL targets
	patterns    experience.PatternRepository // optional; V2-8 pattern utility propagation
	now         func() time.Time
	id          func() string
}

// LinkLister lists experience→action influence edges for an episode.
type LinkLister interface {
	ListLinks(ctx context.Context, tenantID, episodeID string) ([]action.ExperienceActionLink, error)
}

// ActionLister lists agent actions for an episode (tool_name enrichment).
type ActionLister interface {
	ListActions(ctx context.Context, tenantID, episodeID string) ([]action.AgentAction, error)
}


// New constructs a learning service.
func New(usages experience.UsageRepository, experiences experience.Repository, strategy attribution.Strategy) (*Service, error) {
	return NewWithEvents(usages, experiences, NewMemoryEventRepository(), strategy, nil)
}

// NewWithEvents constructs a learning service with a persistent event ledger.
// When applier is nil, a MemoryEventApplier is used for in-process atomic apply.
func NewWithEvents(
	usages experience.UsageRepository,
	experiences experience.Repository,
	events EventRepository,
	strategy attribution.Strategy,
	applier EventApplier,
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
	if applier == nil {
		applier = NewMemoryEventApplier(experiences, events)
	}
	return &Service{
		usages:      usages,
		experiences: experiences,
		events:      events,
		applier:     applier,
		strategy:    strategy,
		now:         time.Now().UTC,
		id:          func() string { return uuid.NewString() },
	}, nil
}

// WithActionGraph attaches action/link sources used by targeted attribution.
func (s *Service) WithActionGraph(links LinkLister, actions ActionLister) *Service {
	s.links = links
	s.actions = actions
	return s
}

// WithPatterns attaches a pattern store so member-experience feedback updates Pattern utility (V2-8).
func (s *Service) WithPatterns(repo experience.PatternRepository) *Service {
	s.patterns = repo
	return s
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
// Replays of the same feedback_id return snapshots when all events are APPLIED;
// PENDING/FAILED events are retried without creating duplicates.
func (s *Service) ApplyFeedbackReward(
	ctx context.Context,
	tenantID, episodeID, feedbackID string,
	reward, confidence float64,
	target *feedback.Target,
) ([]UtilityUpdate, error) {
	if strings.TrimSpace(tenantID) == "" || strings.TrimSpace(episodeID) == "" || strings.TrimSpace(feedbackID) == "" {
		return nil, fmt.Errorf("tenant_id, episode_id, and feedback_id are required")
	}

	existing, err := s.events.ListByFeedback(ctx, tenantID, feedbackID)
	if err != nil {
		return nil, fmt.Errorf("list learning events for feedback %s: %w", feedbackID, err)
	}
	if len(existing) > 0 {
		allApplied := true
		for _, ev := range existing {
			if ev.Status != EventApplied {
				allApplied = false
				break
			}
		}
		if allApplied {
			return s.replayApplied(ctx, tenantID, existing)
		}
		return s.retryExistingEvents(ctx, tenantID, existing)
	}

	usages, err := s.usages.ListByEpisode(ctx, tenantID, episodeID)
	if err != nil {
		return nil, fmt.Errorf("list usages for episode %s: %w", episodeID, err)
	}
	if len(usages) == 0 {
		return nil, nil
	}

	links, err := s.loadAttributionLinks(ctx, tenantID, episodeID)
	if err != nil {
		return nil, err
	}
	credits, err := s.strategy.Attribute(attribution.Request{
		Usages:        usages,
		EpisodeReward: reward,
		Target:        mapTargetHint(target),
		Links:         links,
	})
	if err != nil {
		return nil, fmt.Errorf("attribute reward for feedback %s: %w", feedbackID, err)
	}
	if err := attribution.ValidateCredits(credits); err != nil {
		return nil, fmt.Errorf("invalid attribution for feedback %s: %w", feedbackID, err)
	}
	if len(credits) == 0 {
		return nil, nil
	}

	now := s.now()
	updates := make([]UtilityUpdate, 0, len(credits))
	for _, c := range credits {
		u, err := s.createAndApplyEvent(ctx, tenantID, episodeID, feedbackID, reward, confidence, c, now)
		if err != nil {
			return updates, err
		}
		updates = append(updates, u)
	}
	return updates, nil
}

func (s *Service) retryExistingEvents(
	ctx context.Context,
	tenantID string,
	events []Event,
) ([]UtilityUpdate, error) {
	out := make([]UtilityUpdate, 0, len(events))
	for _, ev := range events {
		switch ev.Status {
		case EventApplied:
			u, err := s.snapshotUpdate(ctx, tenantID, ev)
			if err != nil {
				return nil, err
			}
			u.AlreadyApplied = true
			out = append(out, u)
		case EventPending, EventFailed:
			u, err := s.applyExistingEvent(ctx, tenantID, ev)
			if err != nil {
				return out, err
			}
			out = append(out, u)
		}
	}
	return out, nil
}

func (s *Service) applyExistingEvent(
	ctx context.Context,
	tenantID string,
	ev Event,
) (UtilityUpdate, error) {
	const maxAttempts = 8
	var lastErr error
	for attempt := 0; attempt < maxAttempts; attempt++ {
		result, err := s.applier.ApplyPendingEvent(ctx, tenantID, ev)
		if err != nil {
			if errors.Is(err, experience.ErrConflict) && attempt+1 < maxAttempts {
				lastErr = err
				continue
			}
			_ = s.events.MarkFailed(ctx, tenantID, ev.ID)
			return UtilityUpdate{}, fmt.Errorf("apply learning event %s: %w", ev.ID, err)
		}
		expReward := ev.NormalizedReward * ev.Credit
		u := UtilityUpdate{
			ExperienceID:    ev.ExperienceID,
			LearningEventID: ev.ID,
			Credit:          ev.Credit,
			Reward:          expReward,
			EffectiveReward: ev.EffectiveReward,
			OldUtility:      result.OldUtility,
			NewUtility:      result.Experience.Utility,
			Alpha:           result.Experience.Alpha,
			Beta:            result.Experience.Beta,
			AlreadyApplied:  result.AlreadyApplied,
		}
		if !result.AlreadyApplied {
			if err := s.propagatePatternLearning(ctx, tenantID, ev.ExperienceID, expReward, ev.Confidence); err != nil {
				return UtilityUpdate{}, err
			}
		}
		return u, nil
	}
	_ = s.events.MarkFailed(ctx, tenantID, ev.ID)
	return UtilityUpdate{}, fmt.Errorf("apply learning event %s: %w", ev.ID, lastErr)
}

// propagatePatternLearning moves Pattern utility when a supporting experience learns (V2-8).
func (s *Service) propagatePatternLearning(
	ctx context.Context,
	tenantID, experienceID string,
	reward, confidence float64,
) error {
	if s.patterns == nil {
		return nil
	}
	pats, err := s.patterns.FindByExperience(ctx, tenantID, []string{experienceID})
	if err != nil {
		return fmt.Errorf("find patterns for experience %s: %w", experienceID, err)
	}
	now := s.now()
	for _, p := range pats {
		if !p.Status.Retrievable() {
			continue
		}
		updated, err := experience.ApplyPatternBetaUpdate(p, reward, confidence, now)
		if err != nil {
			return fmt.Errorf("beta update pattern %s: %w", p.ID, err)
		}
		updated = experience.MaybePromotePattern(updated)
		if _, err := s.patterns.Update(ctx, updated); err != nil {
			return fmt.Errorf("persist pattern %s utility: %w", p.ID, err)
		}
	}
	return nil
}

func (s *Service) createAndApplyEvent(
	ctx context.Context,
	tenantID, episodeID, feedbackID string,
	reward, confidence float64,
	c attribution.Credit,
	now time.Time,
) (UtilityUpdate, error) {
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
			dup, getErr := s.events.GetByFeedbackExperience(ctx, tenantID, feedbackID, c.ExperienceID)
			if getErr != nil {
				return UtilityUpdate{}, getErr
			}
			switch dup.Status {
			case EventApplied:
				u, replayErr := s.snapshotUpdate(ctx, tenantID, dup)
				if replayErr != nil {
					return UtilityUpdate{}, replayErr
				}
				u.AlreadyApplied = true
				return u, nil
			default:
				return s.applyExistingEvent(ctx, tenantID, dup)
			}
		}
		return UtilityUpdate{}, fmt.Errorf("create learning event for experience %s: %w", c.ExperienceID, err)
	}

	return s.applyExistingEvent(ctx, tenantID, created)
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

func mapTargetHint(t *feedback.Target) *attribution.TargetHint {
	if t == nil {
		return nil
	}
	return &attribution.TargetHint{
		Type:         string(t.Type),
		ActionID:     t.ActionID,
		ToolName:     t.ToolName,
		Field:        t.Field,
		ExperienceID: t.ExperienceID,
	}
}

func (s *Service) loadAttributionLinks(ctx context.Context, tenantID, episodeID string) ([]attribution.LinkHint, error) {
	if s.links == nil {
		return nil, nil
	}
	raw, err := s.links.ListLinks(ctx, tenantID, episodeID)
	if err != nil {
		return nil, fmt.Errorf("list experience-action links for episode %s: %w", episodeID, err)
	}
	toolByAction := map[string]string{}
	if s.actions != nil {
		actions, aerr := s.actions.ListActions(ctx, tenantID, episodeID)
		if aerr != nil {
			return nil, fmt.Errorf("list actions for episode %s: %w", episodeID, aerr)
		}
		for _, a := range actions {
			toolByAction[a.ID] = a.ToolName
		}
	}
	out := make([]attribution.LinkHint, 0, len(raw))
	for _, link := range raw {
		out = append(out, attribution.LinkHint{
			ExperienceID:   link.ExperienceID,
			ActionID:       link.ActionID,
			Influence:      link.Influence,
			ToolName:       toolByAction[link.ActionID],
			AffectedFields: link.AffectedFields,
		})
	}
	return out, nil
}
