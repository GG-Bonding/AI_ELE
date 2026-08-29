package attribution

import (
	"fmt"

	"github.com/agent-experience-engine/agent-experience-engine/internal/experience"
)

// Credit is the share of episode reward assigned to one experience.
type Credit struct {
	ExperienceID string
	Weight       float64 // normalized credit in (0,1], sums to 1 across credits
	Score        float64 // raw score used for attribution
}

// Strategy assigns episode-level reward credit across used experiences.
type Strategy interface {
	Attribute(usages []experience.Usage, episodeReward float64) ([]Credit, error)
}

// ScoreProportional splits credit by FinalScore (fallback RetrievalScore).
type ScoreProportional struct{}

// Attribute implements Strategy.
func (ScoreProportional) Attribute(usages []experience.Usage, episodeReward float64) ([]Credit, error) {
	if len(usages) == 0 {
		return nil, nil
	}
	if len(usages) == 1 {
		score := usages[0].FinalScore
		if score <= 0 {
			score = usages[0].RetrievalScore
		}
		if score <= 0 {
			score = 1
		}
		return []Credit{{
			ExperienceID: usages[0].ExperienceID,
			Weight:       1,
			Score:        score,
		}}, nil
	}

	var sum float64
	raw := make([]float64, len(usages))
	for i, u := range usages {
		score := u.FinalScore
		if score <= 0 {
			score = u.RetrievalScore
		}
		if score < 0 {
			score = 0
		}
		raw[i] = score
		sum += score
	}
	if sum == 0 {
		// equal split when scores are unavailable
		out := make([]Credit, len(usages))
		w := 1 / float64(len(usages))
		for i, u := range usages {
			out[i] = Credit{ExperienceID: u.ExperienceID, Weight: w, Score: 0}
		}
		return out, nil
	}

	out := make([]Credit, 0, len(usages))
	for i, u := range usages {
		out = append(out, Credit{
			ExperienceID: u.ExperienceID,
			Weight:       raw[i] / sum,
			Score:        raw[i],
		})
	}
	_ = episodeReward // reward magnitude applied by caller; strategy only allocates weights
	return out, nil
}

// Ensure interface compliance.
var _ Strategy = ScoreProportional{}

// NewDefault returns the V1 attribution strategy.
func NewDefault() Strategy {
	return ScoreProportional{}
}

// ValidateCredits checks weights roughly sum to 1.
func ValidateCredits(credits []Credit) error {
	if len(credits) == 0 {
		return nil
	}
	var sum float64
	for _, c := range credits {
		if c.Weight < 0 {
			return fmt.Errorf("negative attribution weight for %s", c.ExperienceID)
		}
		sum += c.Weight
	}
	if sum < 0.999 || sum > 1.001 {
		return fmt.Errorf("attribution weights sum to %v, want ~1", sum)
	}
	return nil
}
