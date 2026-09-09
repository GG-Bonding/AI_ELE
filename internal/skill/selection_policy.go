package skill

import (
	"math/rand"
	"strings"
)

// SelectionPolicy picks one skill from ranked candidates (V3.3 default selector).
type SelectionPolicy interface {
	Name() string
	Select(ranked []RankedSkill, rng *rand.Rand) (RankedSkill, bool)
}

// GreedyPolicy always returns the top-ranked eligible skill.
type GreedyPolicy struct{}

func (GreedyPolicy) Name() string { return "greedy" }

func (GreedyPolicy) Select(ranked []RankedSkill, _ *rand.Rand) (RankedSkill, bool) {
	for _, r := range ranked {
		if r.Score > 0 && r.Availability > 0 && r.Validity > 0 {
			return r, true
		}
	}
	if len(ranked) == 0 {
		return RankedSkill{}, false
	}
	return ranked[0], ranked[0].Score > 0
}

// ThompsonPolicy wraps SelectThompson.
type ThompsonPolicy struct{}

func (ThompsonPolicy) Name() string { return "thompson" }

func (ThompsonPolicy) Select(ranked []RankedSkill, rng *rand.Rand) (RankedSkill, bool) {
	return SelectThompson(ranked, rng)
}

// EpsilonGreedyPolicy explores randomly with probability Epsilon.
type EpsilonGreedyPolicy struct {
	Epsilon float64
}

func (p EpsilonGreedyPolicy) Name() string { return "epsilon_greedy" }

func (p EpsilonGreedyPolicy) Select(ranked []RankedSkill, rng *rand.Rand) (RankedSkill, bool) {
	if len(ranked) == 0 {
		return RankedSkill{}, false
	}
	if rng == nil {
		rng = rand.New(rand.NewSource(1))
	}
	eps := p.Epsilon
	if eps < 0 {
		eps = 0
	}
	if eps > 1 {
		eps = 1
	}
	eligible := make([]RankedSkill, 0, len(ranked))
	for _, r := range ranked {
		if r.Score > 0 && r.Availability > 0 && r.Validity > 0 {
			eligible = append(eligible, r)
		}
	}
	if len(eligible) == 0 {
		return ranked[0], ranked[0].Score > 0
	}
	if rng.Float64() < eps {
		return eligible[rng.Intn(len(eligible))], true
	}
	return GreedyPolicy{}.Select(eligible, rng)
}

// ParseSelectionPolicy maps config names to policies (default greedy).
func ParseSelectionPolicy(name string) SelectionPolicy {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "thompson":
		return ThompsonPolicy{}
	case "epsilon_greedy", "epsilon-greedy", "eps":
		return EpsilonGreedyPolicy{Epsilon: 0.1}
	default:
		return GreedyPolicy{}
	}
}
