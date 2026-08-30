package evaluator

import (
	"context"
	"fmt"
	"math"
	"strings"

	"github.com/agent-experience-engine/agent-experience-engine/internal/outcome"
)

// Status is the store decision status (mirrors experience.Status without importing it).
type Status string

const (
	StatusCandidate Status = "CANDIDATE"
	StatusActive    Status = "ACTIVE"
)

// CandidateInput is the evaluator view of an extracted candidate (cycle-free DTO).
type CandidateInput struct {
	Type       string
	Trigger    string
	Content    string
	Confidence float64
	Scope      string
	ScopeKey   string
}

// Evidence summarizes supporting traces for a candidate (V1 rule inputs).
type Evidence struct {
	FailedAttemptCount  int
	SuccessAttemptCount int
	HasFailureContrast  bool // fail then success pattern
	HasToolErrorCode    bool
	SourceEpisodeID     string
}

// Evaluation is the explainable quality decision for a candidate.
type Evaluation struct {
	Quality     float64    `json:"quality"`
	Status      Status     `json:"status"`
	Store       bool       `json:"store"`
	ReasonCodes []string   `json:"reason_codes"`
	Components  Components `json:"components"`
}

// Components are the rule-based quality factors (sum weights ≈ 1 before risk).
type Components struct {
	ExtractionConfidence float64 `json:"extraction_confidence"` // C
	EvidenceStrength     float64 `json:"evidence_strength"`     // E
	OutcomeVerified      float64 `json:"outcome_verified"`      // O
	Reusability          float64 `json:"reusability"`           // R
	Specificity          float64 `json:"specificity"`           // S
	Risk                 float64 `json:"risk"`
}

// Evaluator scores experience candidates independently of raw extractor confidence.
type Evaluator interface {
	Evaluate(ctx context.Context, candidate CandidateInput, evidence Evidence, out outcome.Outcome) (Evaluation, error)
}

// RuleEvaluator is the V1 deterministic evaluator.
//
//	Q = 0.30C + 0.25E + 0.20O + 0.15R + 0.10S - Risk
type RuleEvaluator struct {
	ActiveMin    float64
	CandidateMin float64
}

// NewRuleEvaluator constructs defaults aligned with config evaluator thresholds.
func NewRuleEvaluator(activeMin, candidateMin float64) *RuleEvaluator {
	if activeMin <= 0 {
		activeMin = 0.65
	}
	if candidateMin <= 0 {
		candidateMin = 0.4
	}
	return &RuleEvaluator{ActiveMin: activeMin, CandidateMin: candidateMin}
}

// Evaluate implements Evaluator.
func (e *RuleEvaluator) Evaluate(_ context.Context, candidate CandidateInput, evidence Evidence, out outcome.Outcome) (Evaluation, error) {
	c := clamp01(candidate.Confidence)
	ev := evidenceStrength(evidence)
	o := outcomeScore(out)
	r := reusabilityScore(candidate)
	s := specificityScore(candidate)
	risk := riskScore(candidate)

	q := 0.30*c + 0.25*ev + 0.20*o + 0.15*r + 0.10*s - risk
	q = clamp01(q)

	reasons := make([]string, 0, 6)
	if out.Verified && strings.EqualFold(out.Status, "SUCCESS") {
		reasons = append(reasons, "verified_success")
	}
	if evidence.HasFailureContrast {
		reasons = append(reasons, "failure_success_contrast")
	}
	if r >= 0.7 {
		reasons = append(reasons, "reusable_rule")
	}
	if risk > 0 {
		reasons = append(reasons, "content_risk_penalty")
	}
	if c >= e.ActiveMin {
		reasons = append(reasons, "high_extraction_confidence")
	}

	eval := Evaluation{
		Quality:     q,
		ReasonCodes: reasons,
		Components: Components{
			ExtractionConfidence: c,
			EvidenceStrength:     ev,
			OutcomeVerified:      o,
			Reusability:          r,
			Specificity:          s,
			Risk:                 risk,
		},
	}

	switch {
	case q >= e.ActiveMin:
		eval.Status = StatusActive
		eval.Store = true
	case q >= e.CandidateMin:
		eval.Status = StatusCandidate
		eval.Store = true
	default:
		eval.Store = false
		reasons = append(reasons, "below_candidate_threshold")
		eval.ReasonCodes = reasons
	}
	return eval, nil
}

func evidenceStrength(e Evidence) float64 {
	score := 0.2
	if e.FailedAttemptCount > 0 {
		score += 0.25
	}
	if e.SuccessAttemptCount > 0 {
		score += 0.25
	}
	if e.HasFailureContrast {
		score += 0.2
	}
	if e.HasToolErrorCode {
		score += 0.1
	}
	return clamp01(score)
}

func outcomeScore(out outcome.Outcome) float64 {
	if out.Verified && strings.EqualFold(out.Status, "SUCCESS") {
		return 1.0
	}
	if strings.EqualFold(out.Status, "SUCCESS") {
		return 0.7
	}
	if strings.EqualFold(out.Status, "PARTIAL") {
		return 0.45
	}
	return 0.2
}

func reusabilityScore(c CandidateInput) float64 {
	content := strings.ToLower(c.Content + " " + c.Trigger)
	score := 0.5
	for _, tip := range []string{"before", "should", "must", "resolve", "check", "confirm", "when", "if "} {
		if strings.Contains(content, tip) {
			score += 0.1
		}
	}
	switch strings.ToUpper(c.Type) {
	case "PROCEDURAL", "FAILURE", "CONSTRAINT":
		score += 0.1
	}
	return clamp01(score)
}

func specificityScore(c CandidateInput) float64 {
	n := len([]rune(strings.TrimSpace(c.Content)))
	switch {
	case n < 20:
		return 0.3
	case n > 400:
		return 0.4
	case n > 180:
		return 0.7
	default:
		return 0.9
	}
}

func riskScore(c CandidateInput) float64 {
	content := strings.ToLower(c.Content)
	risk := 0.0
	for _, bad := range []string{"ignore previous", "system prompt", "api_key", "password", "secret token"} {
		if strings.Contains(content, bad) {
			risk += 0.25
		}
	}
	return math.Min(risk, 0.8)
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

var _ Evaluator = (*RuleEvaluator)(nil)

// ErrNotConfigured is reserved for wiring failures.
var ErrNotConfigured = fmt.Errorf("evaluator not configured")
