package skill

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// PromoteConfig controls SHADOW→ACTIVE and live auto-suspend gates (V3-3).
type PromoteConfig struct {
	ShadowMinExecutions   int     // default 5
	ShadowMinSuccessRate  float64 // default 0.90
	SuspendWindow         int     // default 10
	SuspendMaxFailureRate float64 // default 0.30
}

// DefaultPromoteConfig returns the V3 promotion / suspension defaults.
func DefaultPromoteConfig() PromoteConfig {
	return PromoteConfig{
		ShadowMinExecutions:   5,
		ShadowMinSuccessRate:  0.90,
		SuspendWindow:         10,
		SuspendMaxFailureRate: 0.30,
	}
}

func (c PromoteConfig) withDefaults() PromoteConfig {
	d := DefaultPromoteConfig()
	if c.ShadowMinExecutions <= 0 {
		c.ShadowMinExecutions = d.ShadowMinExecutions
	}
	if c.ShadowMinSuccessRate <= 0 {
		c.ShadowMinSuccessRate = d.ShadowMinSuccessRate
	}
	if c.SuspendWindow <= 0 {
		c.SuspendWindow = d.SuspendWindow
	}
	if c.SuspendMaxFailureRate <= 0 {
		c.SuspendMaxFailureRate = d.SuspendMaxFailureRate
	}
	return c
}

// ValidationReport is the lifecycle-facing validation outcome
// (mirrors skillvalidator.Report without importing it — avoids a package cycle).
type ValidationReport struct {
	OK               bool
	Normalized       Spec
	ComputedRisk     Risk
	RequiresApproval bool
}

// SpecValidator validates a Spec. *skillvalidator.Validator is adapted via
// ValidatorFrom or a thin wrapper in callers (skillvalidator → skill cycle).
type SpecValidator interface {
	Validate(spec Spec) ValidationReport
}

// ApplyValidationReport sets ValidationStatus / VersionStatus from a report
// (same semantics as skillvalidator.ApplyToVersion).
func ApplyValidationReport(ver Version, rep ValidationReport) Version {
	out := ver
	out.Spec = rep.Normalized
	if hash, err := HashSpec(rep.Normalized); err == nil {
		out.SpecHash = hash
	}
	if rep.OK {
		out.ValidationStatus = ValidationPassed
		out.Status = VersionValidated
		out.Spec.Risk.Level = rep.ComputedRisk
		out.Spec.Risk.RequiresApproval = rep.RequiresApproval
	} else {
		out.ValidationStatus = ValidationFailed
		if out.Status == "" {
			out.Status = VersionCandidate
		}
	}
	return out
}

// RegistryService owns Candidate → Validated → Shadow → Active lifecycle.
type RegistryService struct {
	Repo      Repository
	Validator SpecValidator
}

// CompileAndCreate parses YAML, creates Skill+Version, validates, and persists the result.
func (s *RegistryService) CompileAndCreate(
	ctx context.Context,
	tenantID, name, description, patternID, yamlDoc string,
	confidence, utility float64,
) (Skill, Version, ValidationReport, error) {
	if s == nil || s.Repo == nil {
		return Skill{}, Version{}, ValidationReport{}, fmt.Errorf("%w: registry service not configured", ErrInvalidInput)
	}
	tenantID = strings.TrimSpace(tenantID)
	name = strings.TrimSpace(name)
	if tenantID == "" || name == "" {
		return Skill{}, Version{}, ValidationReport{}, fmt.Errorf("%w: tenant_id and name are required", ErrInvalidInput)
	}

	spec, err := ParseYAML(yamlDoc)
	if err != nil {
		return Skill{}, Version{}, ValidationReport{}, err
	}
	if description == "" {
		description = spec.Description
	}

	now := time.Now().UTC()
	sk, err := s.Repo.CreateSkill(ctx, Skill{
		ID:          uuid.NewString(),
		TenantID:    tenantID,
		Name:        name,
		Description: strings.TrimSpace(description),
		Status:      StatusCandidate,
		CreatedAt:   now,
		UpdatedAt:   now,
	})
	if err != nil {
		return Skill{}, Version{}, ValidationReport{}, err
	}

	ver, err := NewVersion(tenantID, sk.ID, patternID, 1, spec, yamlDoc, confidence, utility, now)
	if err != nil {
		return sk, Version{}, ValidationReport{}, err
	}
	ver.ID = uuid.NewString()
	ver = WithSeededBeta(ver)
	ver, err = s.Repo.CreateVersion(ctx, ver)
	if err != nil {
		return sk, Version{}, ValidationReport{}, err
	}

	rep := ValidationReport{OK: true, Normalized: ver.Spec, ComputedRisk: ver.Spec.Risk.Level}
	if s.Validator != nil {
		rep = s.Validator.Validate(ver.Spec)
	}
	ver = ApplyValidationReport(ver, rep)
	ver, err = s.Repo.UpdateVersion(ctx, ver)
	if err != nil {
		return sk, ver, rep, err
	}

	if rep.OK {
		sk.Status = StatusValidated
		sk, err = s.Repo.UpdateSkill(ctx, sk)
		if err != nil {
			return sk, ver, rep, err
		}
	}
	return sk, ver, rep, nil
}

// MoveToShadow promotes a PASSED version into SHADOW (skill + version).
func (s *RegistryService) MoveToShadow(ctx context.Context, tenantID, versionID string) (Version, error) {
	if s == nil || s.Repo == nil {
		return Version{}, fmt.Errorf("%w: registry service not configured", ErrInvalidInput)
	}
	ver, err := s.Repo.GetVersion(ctx, tenantID, versionID)
	if err != nil {
		return Version{}, err
	}
	if ver.ValidationStatus != ValidationPassed {
		return Version{}, fmt.Errorf("%w: version must be ValidationPassed", ErrInvalidTransition)
	}
	sk, err := s.Repo.GetSkill(ctx, tenantID, ver.SkillID)
	if err != nil {
		return Version{}, err
	}

	ver.Status = VersionShadow
	ver, err = s.Repo.UpdateVersion(ctx, ver)
	if err != nil {
		return Version{}, err
	}
	sk.Status = StatusShadow
	if _, err := s.Repo.UpdateSkill(ctx, sk); err != nil {
		return ver, err
	}
	return ver, nil
}

// Activate promotes a SHADOW version to ACTIVE when shadow gates pass.
// Previous active version (if any) is DEPRECATED.
func (s *RegistryService) Activate(ctx context.Context, tenantID, versionID string, cfg PromoteConfig) (Version, error) {
	if s == nil || s.Repo == nil {
		return Version{}, fmt.Errorf("%w: registry service not configured", ErrInvalidInput)
	}
	cfg = cfg.withDefaults()
	ver, err := s.Repo.GetVersion(ctx, tenantID, versionID)
	if err != nil {
		return Version{}, err
	}
	if ver.Status != VersionShadow {
		return Version{}, fmt.Errorf("%w: version must be SHADOW", ErrInvalidTransition)
	}
	total := ver.ShadowSuccesses + ver.ShadowFailures
	if total < cfg.ShadowMinExecutions {
		return Version{}, fmt.Errorf("%w: shadow executions %d < %d", ErrPromotionGate, total, cfg.ShadowMinExecutions)
	}
	rate := float64(ver.ShadowSuccesses) / float64(total)
	if rate < cfg.ShadowMinSuccessRate {
		return Version{}, fmt.Errorf("%w: shadow success rate %.3f < %.3f", ErrPromotionGate, rate, cfg.ShadowMinSuccessRate)
	}

	sk, err := s.Repo.GetSkill(ctx, tenantID, ver.SkillID)
	if err != nil {
		return Version{}, err
	}
	if sk.ActiveVersionID != nil && *sk.ActiveVersionID != ver.ID {
		prev, err := s.Repo.GetVersion(ctx, tenantID, *sk.ActiveVersionID)
		if err != nil {
			return Version{}, err
		}
		prev.Status = VersionDeprecated
		if _, err := s.Repo.UpdateVersion(ctx, prev); err != nil {
			return Version{}, err
		}
	}

	ver.Status = VersionActive
	ver, err = s.Repo.UpdateVersion(ctx, ver)
	if err != nil {
		return Version{}, err
	}
	id := ver.ID
	sk.Status = StatusActive
	sk.ActiveVersionID = &id
	if _, err := s.Repo.UpdateSkill(ctx, sk); err != nil {
		return ver, err
	}
	return ver, nil
}

// Suspend marks a skill and its active version SUSPENDED.
func (s *RegistryService) Suspend(ctx context.Context, tenantID, skillID, reason string) (Skill, error) {
	_ = reason
	if s == nil || s.Repo == nil {
		return Skill{}, fmt.Errorf("%w: registry service not configured", ErrInvalidInput)
	}
	sk, err := s.Repo.GetSkill(ctx, tenantID, skillID)
	if err != nil {
		return Skill{}, err
	}
	if sk.ActiveVersionID != nil {
		ver, err := s.Repo.GetVersion(ctx, tenantID, *sk.ActiveVersionID)
		if err != nil {
			return Skill{}, err
		}
		ver.Status = VersionSuspended
		if _, err := s.Repo.UpdateVersion(ctx, ver); err != nil {
			return Skill{}, err
		}
	}
	sk.Status = StatusSuspended
	return s.Repo.UpdateSkill(ctx, sk)
}

// RecordShadowOutcome bumps shadow success/failure counters on a version.
func (s *RegistryService) RecordShadowOutcome(ctx context.Context, tenantID, versionID string, success bool) (Version, error) {
	if s == nil || s.Repo == nil {
		return Version{}, fmt.Errorf("%w: registry service not configured", ErrInvalidInput)
	}
	ver, err := s.Repo.GetVersion(ctx, tenantID, versionID)
	if err != nil {
		return Version{}, err
	}
	if success {
		ver.ShadowSuccesses++
	} else {
		ver.ShadowFailures++
	}
	return s.Repo.UpdateVersion(ctx, ver)
}

// MaybeSuspendFromLiveStats suspends when FailureCount/(Success+Failure) exceeds the gate
// after at least SuspendWindow live outcomes.
func (s *RegistryService) MaybeSuspendFromLiveStats(ctx context.Context, tenantID, versionID string, cfg PromoteConfig) (bool, error) {
	if s == nil || s.Repo == nil {
		return false, fmt.Errorf("%w: registry service not configured", ErrInvalidInput)
	}
	cfg = cfg.withDefaults()
	ver, err := s.Repo.GetVersion(ctx, tenantID, versionID)
	if err != nil {
		return false, err
	}
	total := ver.SuccessCount + ver.FailureCount
	if total < cfg.SuspendWindow {
		return false, nil
	}
	failRate := float64(ver.FailureCount) / float64(total)
	if failRate <= cfg.SuspendMaxFailureRate {
		return false, nil
	}
	if _, err := s.Suspend(ctx, tenantID, ver.SkillID, "live failure rate exceeded"); err != nil {
		return false, err
	}
	return true, nil
}
