package experience

import (
	"context"
	"fmt"
	"strings"
)

// WithSkills attaches a skill-candidate store (V2-9).
func (s *Service) WithSkills(repo SkillRepository) *Service {
	s.skills = repo
	return s
}

// WithSkillBuilder overrides the default HeuristicSkillBuilder.
func (s *Service) WithSkillBuilder(b SkillBuilder) *Service {
	s.skillBuilder = b
	return s
}

// ProposeSkillInput requests a Skill Candidate from a Pattern.
type ProposeSkillInput struct {
	PatternID string
}

// ProposeSkillResult is the outcome of proposing a skill from a pattern.
type ProposeSkillResult struct {
	Created bool
	Skill   SkillCandidate
	Skipped string
}

// ProposeSkill builds a Skill Candidate from a Pattern when gates pass.
// Idempotent: an existing skill for the same pattern is returned without recreating.
// The engine never executes the skill (auto_execute remains false).
func (s *Service) ProposeSkill(ctx context.Context, tenantID string, in ProposeSkillInput) (ProposeSkillResult, error) {
	if s.patterns == nil {
		return ProposeSkillResult{Skipped: "pattern repository not configured"}, nil
	}
	if s.skills == nil {
		return ProposeSkillResult{Skipped: "skill repository not configured"}, nil
	}
	if err := requireNonEmpty("tenant_id", tenantID, "pattern_id", in.PatternID); err != nil {
		return ProposeSkillResult{}, err
	}

	p, err := s.patterns.Get(ctx, tenantID, strings.TrimSpace(in.PatternID))
	if err != nil {
		return ProposeSkillResult{}, fmt.Errorf("get pattern %s: %w", in.PatternID, err)
	}
	if reason := skillProposeGate(p); reason != "" {
		return ProposeSkillResult{Skipped: reason}, nil
	}

	existing, err := s.skills.FindByPattern(ctx, tenantID, p.ID)
	if err == nil && existing.ID != "" {
		return ProposeSkillResult{Created: false, Skill: existing, Skipped: "skill already exists for pattern"}, nil
	}
	if err != nil && err != ErrNotFound {
		return ProposeSkillResult{}, fmt.Errorf("find skill by pattern: %w", err)
	}

	builder := s.skillBuilder
	if builder == nil {
		builder = HeuristicSkillBuilder{}
	}
	draft, err := builder.Build(p)
	if err != nil {
		return ProposeSkillResult{}, fmt.Errorf("build skill draft: %w", err)
	}
	if strings.TrimSpace(draft.Name) == "" || strings.TrimSpace(draft.SpecYAML) == "" {
		return ProposeSkillResult{}, fmt.Errorf("%w: skill name/spec required", ErrInvalidInput)
	}

	now := s.now()
	sk := SkillCandidate{
		ID:          s.id(),
		TenantID:    strings.TrimSpace(tenantID),
		PatternID:   p.ID,
		Name:        strings.TrimSpace(draft.Name),
		Description: strings.TrimSpace(draft.Description),
		SpecYAML:    draft.SpecYAML,
		Confidence:  clamp01(draft.Confidence),
		Utility:     clamp01(draft.Utility),
		Status:      SkillStatusCandidate,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	created, err := s.skills.Create(ctx, sk)
	if err != nil {
		return ProposeSkillResult{}, fmt.Errorf("create skill candidate: %w", err)
	}
	return ProposeSkillResult{Created: true, Skill: created}, nil
}

// GetSkill returns one skill candidate for a tenant.
func (s *Service) GetSkill(ctx context.Context, tenantID, skillID string) (SkillCandidate, error) {
	if s.skills == nil {
		return SkillCandidate{}, ErrNotFound
	}
	if err := requireNonEmpty("tenant_id", tenantID, "skill_id", skillID); err != nil {
		return SkillCandidate{}, err
	}
	sk, err := s.skills.Get(ctx, tenantID, skillID)
	if err != nil {
		return SkillCandidate{}, fmt.Errorf("get skill %s: %w", skillID, err)
	}
	return sk, nil
}

// GetSkillByPattern returns the skill candidate linked to a pattern, if any.
func (s *Service) GetSkillByPattern(ctx context.Context, tenantID, patternID string) (SkillCandidate, error) {
	if s.skills == nil {
		return SkillCandidate{}, ErrNotFound
	}
	if err := requireNonEmpty("tenant_id", tenantID, "pattern_id", patternID); err != nil {
		return SkillCandidate{}, err
	}
	sk, err := s.skills.FindByPattern(ctx, tenantID, patternID)
	if err != nil {
		return SkillCandidate{}, fmt.Errorf("find skill for pattern %s: %w", patternID, err)
	}
	return sk, nil
}
