package skill

import (
	"context"
	"fmt"
)

// ShadowTrial is one shadow execution outcome used for A/B comparison.
type ShadowTrial struct {
	Success bool
	Steps   int
}

// ABResult compares two SkillVersions under shadow trials (V3.2).
type ABResult struct {
	VersionAID   string
	VersionBID   string
	ASuccessRate float64
	BSuccessRate float64
	AAvgSteps    float64
	BAvgSteps    float64
	WinnerID     string // empty if tie / inconclusive
	ACESteps     float64
}

// CompareShadowAB picks a winner: higher success rate, then fewer steps.
// Requires at least minTrials on each arm.
func CompareShadowAB(versionAID, versionBID string, a, b []ShadowTrial, minTrials int) ABResult {
	if minTrials <= 0 {
		minTrials = 3
	}
	out := ABResult{VersionAID: versionAID, VersionBID: versionBID}
	if len(a) < minTrials || len(b) < minTrials {
		return out
	}
	out.ASuccessRate, out.AAvgSteps = armStats(a)
	out.BSuccessRate, out.BAvgSteps = armStats(b)
	out.ACESteps = out.AAvgSteps - out.BAvgSteps // positive => B fewer steps

	switch {
	case out.BSuccessRate > out.ASuccessRate+1e-9:
		out.WinnerID = versionBID
	case out.ASuccessRate > out.BSuccessRate+1e-9:
		out.WinnerID = versionAID
	case out.BAvgSteps+1e-9 < out.AAvgSteps:
		out.WinnerID = versionBID
	case out.AAvgSteps+1e-9 < out.BAvgSteps:
		out.WinnerID = versionAID
	}
	return out
}

func armStats(trials []ShadowTrial) (successRate, avgSteps float64) {
	if len(trials) == 0 {
		return 0, 0
	}
	var ok, steps int
	for _, t := range trials {
		if t.Success {
			ok++
		}
		steps += t.Steps
	}
	return float64(ok) / float64(len(trials)), float64(steps) / float64(len(trials))
}

// PromoteABWinner shadows-activates the winner when gates pass (caller supplies shadow counts).
func PromoteABWinner(ctx context.Context, reg *RegistryService, tenantID, winnerVersionID string, cfg PromoteConfig) (Version, error) {
	if reg == nil {
		return Version{}, fmt.Errorf("%w: registry required", ErrInvalidInput)
	}
	ver, err := reg.Repo.GetVersion(ctx, tenantID, winnerVersionID)
	if err != nil {
		return Version{}, err
	}
	if ver.Status == VersionActive {
		return ver, nil
	}
	if ver.Status != VersionShadow {
		if ver.ValidationStatus != ValidationPassed {
			return Version{}, fmt.Errorf("%w: winner must be validated", ErrInvalidTransition)
		}
		if _, err := reg.MoveToShadow(ctx, tenantID, winnerVersionID); err != nil {
			return Version{}, err
		}
	}
	return reg.Activate(ctx, tenantID, winnerVersionID, cfg)
}
