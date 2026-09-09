package skill

import (
	"context"
	"fmt"
)

// ShadowEvalConfig controls automated shadow evaluation before activate (V3.3).
type ShadowEvalConfig struct {
	MinTrials      int
	MinSuccessRate float64
}

// ShadowEvalResult summarizes offline shadow gate checks.
type ShadowEvalResult struct {
	OK          bool
	Trials      int
	Successes   int
	SuccessRate float64
	Reason      string
}

// EvaluateShadowStats checks whether shadow counters meet promotion gates.
func EvaluateShadowStats(shadowRuns, shadowSuccesses int, cfg ShadowEvalConfig) ShadowEvalResult {
	if cfg.MinTrials <= 0 {
		cfg.MinTrials = 5
	}
	if cfg.MinSuccessRate <= 0 {
		cfg.MinSuccessRate = 0.90
	}
	out := ShadowEvalResult{Trials: shadowRuns, Successes: shadowSuccesses}
	if shadowRuns < cfg.MinTrials {
		out.Reason = fmt.Sprintf("need %d trials, have %d", cfg.MinTrials, shadowRuns)
		return out
	}
	out.SuccessRate = float64(shadowSuccesses) / float64(shadowRuns)
	if out.SuccessRate+1e-9 < cfg.MinSuccessRate {
		out.Reason = fmt.Sprintf("success rate %.2f < %.2f", out.SuccessRate, cfg.MinSuccessRate)
		return out
	}
	out.OK = true
	out.Reason = "shadow gates passed"
	return out
}

// AutoShadowEvaluate loads a version and evaluates its shadow counters.
func AutoShadowEvaluate(ctx context.Context, repo Repository, tenantID, versionID string, cfg ShadowEvalConfig) (ShadowEvalResult, Version, error) {
	ver, err := repo.GetVersion(ctx, tenantID, versionID)
	if err != nil {
		return ShadowEvalResult{}, Version{}, err
	}
	trials := ver.ShadowSuccesses + ver.ShadowFailures
	return EvaluateShadowStats(trials, ver.ShadowSuccesses, cfg), ver, nil
}
