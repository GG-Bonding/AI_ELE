package feedback

import (
	"fmt"
	"math"
	"strings"
)

// SourceWeights is the default trust configuration for feedback sources.
// LLM Judge is intentionally the weakest signal.
func DefaultSourceWeights() map[Source]float64 {
	return map[Source]float64{
		SourceBusiness:     1.0,
		SourceUserExplicit: 1.0,
		SourceHumanReview:  0.95,
		SourceTool:         0.85,
		SourceUserImplicit: 0.60,
		SourceLLMJudge:     0.50,
	}
}

// NormalizeReward clamps reward into [-1, 1].
func NormalizeReward(reward float64) float64 {
	if reward > 1 {
		return 1
	}
	if reward < -1 {
		return -1
	}
	if math.IsNaN(reward) || math.IsInf(reward, 0) {
		return 0
	}
	return reward
}

// NormalizeConfidence clamps confidence into [0, 1].
func NormalizeConfidence(confidence float64) float64 {
	if confidence > 1 {
		return 1
	}
	if confidence < 0 {
		return 0
	}
	if math.IsNaN(confidence) || math.IsInf(confidence, 0) {
		return 0
	}
	return confidence
}

// SignalToReward maps common outcome signals onto the canonical reward scale.
// Unknown signals return false so callers must provide an explicit reward.
func SignalToReward(signal string) (float64, bool) {
	switch strings.ToLower(strings.TrimSpace(signal)) {
	case "task_success", "success", "ok":
		return 1.0, true
	case "partial_success":
		return 0.4, true
	case "neutral":
		return 0.0, true
	case "partial_failure":
		return -0.5, true
	case "hard_failure", "failure", "failed":
		return -1.0, true
	case "user_undo", "undo":
		return -0.8, true
	default:
		return 0, false
	}
}

// ParseSource accepts API-friendly aliases (business, user_explicit, …).
func ParseSource(raw string) (Source, error) {
	s := Source(strings.ToUpper(strings.TrimSpace(raw)))
	s = Source(strings.ReplaceAll(string(s), "-", "_"))
	if !s.Valid() {
		return "", fmt.Errorf("%w: invalid feedback source %q", ErrInvalidInput, raw)
	}
	return s, nil
}
