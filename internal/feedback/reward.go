package feedback

import "fmt"

// RewardEngine aggregates raw feedback into a single weighted reward.
type RewardEngine struct {
	weights map[Source]float64
}

// NewRewardEngine constructs a reward engine with configurable source weights.
func NewRewardEngine(weights map[Source]float64) *RewardEngine {
	if len(weights) == 0 {
		weights = DefaultSourceWeights()
	}
	cp := make(map[Source]float64, len(weights))
	for k, v := range weights {
		cp[k] = v
	}
	return &RewardEngine{weights: cp}
}

// Aggregate computes:
//
//	weightedReward = Σ(sourceWeight × confidence × reward) / Σ(sourceWeight × confidence)
func (e *RewardEngine) Aggregate(tenantID, episodeID string, rows []Feedback) (EpisodeReward, error) {
	out := EpisodeReward{
		TenantID:      tenantID,
		EpisodeID:     episodeID,
		FeedbackCount: len(rows),
	}
	if len(rows) == 0 {
		return out, fmt.Errorf("%w: no feedback for episode %s", ErrNotFound, episodeID)
	}

	var num, den float64
	for _, row := range rows {
		w, ok := e.weights[row.Source]
		if !ok {
			return EpisodeReward{}, fmt.Errorf("%w: missing weight for source %s", ErrInvalidInput, row.Source)
		}
		weight := w * NormalizeConfidence(row.Confidence)
		if weight <= 0 {
			continue
		}
		num += weight * NormalizeReward(row.Reward)
		den += weight
	}
	if den == 0 {
		return EpisodeReward{}, fmt.Errorf("%w: total weight is zero for episode %s", ErrInvalidInput, episodeID)
	}
	out.WeightedReward = NormalizeReward(num / den)
	out.TotalWeight = den
	return out, nil
}
