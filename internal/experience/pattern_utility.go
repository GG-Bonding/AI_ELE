package experience

import (
	"fmt"
	"math"
	"time"
)

// Pattern learning thresholds (V2-8).
const (
	PatternPromoteMinUtility = 0.75
	PatternPromoteMinSuccess = 2
)

// ApplyPatternBetaUpdate updates a Pattern's alpha/beta/utility from a signed reward (V2-8).
// Same Beta posterior rule as experiences: Patterns are learnable artifacts, not frozen rules.
func ApplyPatternBetaUpdate(p Pattern, reward, confidence float64, now time.Time) (Pattern, error) {
	if confidence < 0 || confidence > 1 || math.IsNaN(confidence) {
		return Pattern{}, fmt.Errorf("%w: confidence out of range", ErrInvalidInput)
	}
	if math.IsNaN(reward) || math.IsInf(reward, 0) {
		return Pattern{}, fmt.Errorf("%w: invalid pattern reward", ErrInvalidInput)
	}

	alpha, beta := p.Alpha, p.Beta
	if alpha <= 0 || beta <= 0 {
		alpha, beta = SeedBetaFromUtility(p.Utility)
	}

	if reward >= 0 {
		alpha += reward * confidence
		p.SuccessCount++
	} else {
		beta += math.Abs(reward) * confidence
		p.FailureCount++
	}

	p.Alpha = alpha
	p.Beta = beta
	p.Utility = alpha / (alpha + beta)
	p.UpdatedAt = now
	return p, nil
}

// MaybePromotePattern upgrades CANDIDATE → ACTIVE when practice quality is proven.
func MaybePromotePattern(p Pattern) Pattern {
	if p.Status != PatternStatusCandidate {
		return p
	}
	if p.Utility >= PatternPromoteMinUtility && p.SuccessCount >= PatternPromoteMinSuccess {
		p.Status = PatternStatusActive
	}
	return p
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
