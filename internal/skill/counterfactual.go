package skill

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
