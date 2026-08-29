package experience

import (
	"fmt"
	"math"
	"time"
)

// ApplyBetaUpdate updates alpha/beta/utility from a signed experience reward.
//
//	positive: alpha += reward * confidence
//	negative: beta  += abs(reward) * confidence
//	utility  = alpha / (alpha + beta)
func ApplyBetaUpdate(exp Experience, experienceReward, confidence float64, now time.Time) (Experience, error) {
	if confidence < 0 || confidence > 1 || math.IsNaN(confidence) {
		return Experience{}, fmt.Errorf("%w: confidence out of range", ErrInvalidInput)
	}
	if math.IsNaN(experienceReward) || math.IsInf(experienceReward, 0) {
		return Experience{}, fmt.Errorf("%w: invalid experience reward", ErrInvalidInput)
	}

	alpha := exp.Alpha
	beta := exp.Beta
	if alpha <= 0 {
		alpha = 1
	}
	if beta <= 0 {
		beta = 1
	}

	if experienceReward >= 0 {
		alpha += experienceReward * confidence
		exp.SuccessCount++
	} else {
		beta += math.Abs(experienceReward) * confidence
		exp.FailureCount++
	}

	exp.Alpha = alpha
	exp.Beta = beta
	exp.Utility = alpha / (alpha + beta)
	exp.UpdatedAt = now
	exp.Version++
	return exp, nil
}
