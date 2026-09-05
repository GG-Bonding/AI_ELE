package skill

import (
	"fmt"
	"math"
)

// ApplyBetaUpdate updates Version alpha/beta/utility from a signed reward
// (same Beta posterior rule as experiences).
//
//	positive: alpha += reward * confidence
//	negative: beta  += abs(reward) * confidence
//	utility  = alpha / (alpha + beta)
func ApplyBetaUpdate(ver Version, reward, confidence float64) (Version, error) {
	if confidence < 0 || confidence > 1 || math.IsNaN(confidence) {
		return Version{}, fmt.Errorf("%w: confidence out of range", ErrInvalidInput)
	}
	if math.IsNaN(reward) || math.IsInf(reward, 0) {
		return Version{}, fmt.Errorf("%w: invalid reward", ErrInvalidInput)
	}

	alpha, beta := ver.Alpha, ver.Beta
	if alpha <= 0 || beta <= 0 {
		alpha, beta = SeedBetaFromUtility(ver.Utility)
	}

	if reward >= 0 {
		alpha += reward * confidence
		ver.SuccessCount++
	} else {
		beta += math.Abs(reward) * confidence
		ver.FailureCount++
	}

	ver.Alpha = alpha
	ver.Beta = beta
	ver.Utility = alpha / (alpha + beta)
	return ver, nil
}

// SeedBetaFromUtility picks α,β ≥ 1 such that α/(α+β) ≈ utility.
func SeedBetaFromUtility(utility float64) (alpha, beta float64) {
	u := clamp01(utility)
	if u <= 0 {
		return 1, 9
	}
	if u >= 1 {
		return 9, 1
	}
	if u >= 0.5 {
		return u / (1 - u), 1
	}
	return 1, (1 - u) / u
}

// WithSeededBeta ensures Alpha/Beta are populated from Utility when unset.
func WithSeededBeta(ver Version) Version {
	if ver.Alpha > 0 && ver.Beta > 0 {
		return ver
	}
	a, b := SeedBetaFromUtility(ver.Utility)
	ver.Alpha = a
	ver.Beta = b
	return ver
}
