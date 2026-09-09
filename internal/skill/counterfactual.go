package skill

import (
	"math"
)

// EstimateACE is the Average Causal Effect proxy: Reward(T|S) − Reward(T|¬S).
func EstimateACE(withReward, withoutReward float64) float64 {
	return withReward - withoutReward
}

// OfflineComparison summarizes an offline with-skill vs without-skill replay.
type OfflineComparison struct {
	ACE       float64 // step-savings proxy: withoutSteps − withSteps
	StepDelta int     // withSteps − withoutSteps
}

// OfflineCompare contrasts step counts from an offline replay pair.
func OfflineCompare(withSteps, withoutSteps int) OfflineComparison {
	return OfflineComparison{
		ACE:       float64(withoutSteps - withSteps),
		StepDelta: withSteps - withoutSteps,
	}
}

// ReplayArm is one offline policy outcome for counterfactual attribution.
type ReplayArm struct {
	Success bool
	Steps   int
	Reward  float64 // typically +1 success / 0 fail (or custom)
}

// CounterfactualResult is the V3.2 offline ACE estimate over matched arms.
type CounterfactualResult struct {
	ACEReward   float64
	ACESteps    float64
	PreferSkill bool
	WithSkill   ReplayArm
	WithoutSkill ReplayArm
}

// CompareReplayArms estimates ACE from an offline with-skill vs without-skill pair.
// Does not mutate production utility.
func CompareReplayArms(withSkill, withoutSkill ReplayArm) CounterfactualResult {
	if withSkill.Reward == 0 && withSkill.Success {
		withSkill.Reward = 1
	}
	if withoutSkill.Reward == 0 && withoutSkill.Success {
		withoutSkill.Reward = 1
	}
	aceReward := EstimateACE(withSkill.Reward, withoutSkill.Reward)
	aceSteps := OfflineCompare(withSkill.Steps, withoutSkill.Steps).ACE
	prefer := aceReward > 0 || (math.Abs(aceReward) < 1e-9 && aceSteps > 0)
	return CounterfactualResult{
		ACEReward:    aceReward,
		ACESteps:     aceSteps,
		PreferSkill:  prefer,
		WithSkill:    withSkill,
		WithoutSkill: withoutSkill,
	}
}
