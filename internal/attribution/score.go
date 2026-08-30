package attribution

import (
	"fmt"
	"strings"

	"github.com/agent-experience-engine/agent-experience-engine/internal/experience"
)

// ScoreProportional splits credit by FinalScore (fallback RetrievalScore).
// Used as V1 behavior and as fallback when feedback has no precise target.
type ScoreProportional struct{}

// Attribute implements Strategy.
func (ScoreProportional) Attribute(req Request) ([]Credit, error) {
	return attributeByScores(req.Usages, req.EpisodeReward)
}

func attributeByScores(usages []experience.Usage, episodeReward float64) ([]Credit, error) {
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
	_ = episodeReward
	return out, nil
}

// Ensure interface compliance.
var _ Strategy = ScoreProportional{}

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

func usageSet(usages []experience.Usage) map[string]experience.Usage {
	out := make(map[string]experience.Usage, len(usages))
	for _, u := range usages {
		out[u.ExperienceID] = u
	}
	return out
}

func normalizeInfluenceCredits(weights map[string]float64) []Credit {
	var sum float64
	for _, w := range weights {
		if w > 0 {
			sum += w
		}
	}
	if sum <= 0 {
		return nil
	}
	out := make([]Credit, 0, len(weights))
	for id, w := range weights {
		if w <= 0 {
			continue
		}
		out = append(out, Credit{
			ExperienceID: id,
			Weight:       w / sum,
			Score:        w,
		})
	}
	// stable-ish order by experience id
	for i := 0; i < len(out); i++ {
		for j := i + 1; j < len(out); j++ {
			if out[j].ExperienceID < out[i].ExperienceID {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return out
}

func trim(s string) string { return strings.TrimSpace(s) }
