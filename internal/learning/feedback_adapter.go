package learning

import (
	"context"

	"github.com/agent-experience-engine/agent-experience-engine/internal/feedback"
)

// FeedbackLearner adapts Service to feedback.UtilityLearner.
type FeedbackLearner struct {
	Inner *Service
}

// ApplyFeedbackReward implements feedback.UtilityLearner.
func (f FeedbackLearner) ApplyFeedbackReward(ctx context.Context, in feedback.LearnInput) ([]feedback.UtilityChange, error) {
	if f.Inner == nil {
		return nil, nil
	}
	updates, err := f.Inner.ApplyFeedbackReward(
		ctx, in.TenantID, in.EpisodeID, in.FeedbackID, in.Reward, in.Confidence, in.Target,
	)
	if err != nil {
		return nil, err
	}
	out := make([]feedback.UtilityChange, 0, len(updates))
	for _, u := range updates {
		out = append(out, feedback.UtilityChange{
			ExperienceID:    u.ExperienceID,
			OldUtility:      u.OldUtility,
			NewUtility:      u.NewUtility,
			Credit:          u.Credit,
			EffectiveReward: u.EffectiveReward,
			AlreadyApplied:  u.AlreadyApplied,
		})
	}
	return out, nil
}
