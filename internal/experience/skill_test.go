package experience_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/agent-experience-engine/agent-experience-engine/internal/experience"
)

func TestProposeSkillFromActivePattern(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	patterns := experience.NewMemoryPatternRepository()
	skills := experience.NewMemorySkillRepository()
	svc := experience.NewService(experience.NewMemoryRepository()).
		WithPatterns(patterns).
		WithSkills(skills)

	p, err := patterns.Create(ctx, experience.Pattern{
		ID: "pat1", TenantID: "t", Type: experience.TypeProcedural,
		Scope: experience.ScopeTool, ScopeKey: "jira",
		Trigger: "when project key unknown", Content: "Resolve project key before create_issue.",
		Confidence: 0.9, Utility: 0.82, Alpha: 4, Beta: 1, SupportCount: 3,
		Status: experience.PatternStatusActive,
		CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}

	res, err := svc.ProposeSkill(ctx, "t", experience.ProposeSkillInput{PatternID: p.ID})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Created {
		t.Fatalf("expected create, skipped=%q", res.Skipped)
	}
	if res.Skill.Status != experience.SkillStatusCandidate {
		t.Fatalf("status=%s", res.Skill.Status)
	}
	if res.Skill.PatternID != p.ID {
		t.Fatalf("pattern_id=%s", res.Skill.PatternID)
	}
	if !strings.Contains(res.Skill.SpecYAML, "auto_execute: false") {
		t.Fatalf("spec missing auto_execute=false:\n%s", res.Skill.SpecYAML)
	}
	if !strings.Contains(res.Skill.SpecYAML, "source_pattern_id:") {
		t.Fatalf("spec missing source_pattern_id:\n%s", res.Skill.SpecYAML)
	}
	if res.Skill.Name == "" {
		t.Fatal("empty skill name")
	}

	again, err := svc.ProposeSkill(ctx, "t", experience.ProposeSkillInput{PatternID: p.ID})
	if err != nil {
		t.Fatal(err)
	}
	if again.Created {
		t.Fatal("expected idempotent skip")
	}
	if again.Skill.ID != res.Skill.ID {
		t.Fatalf("skill id changed: %s vs %s", again.Skill.ID, res.Skill.ID)
	}

	got, err := svc.GetSkill(ctx, "t", res.Skill.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != res.Skill.ID {
		t.Fatalf("get id=%s", got.ID)
	}
	byPat, err := svc.GetSkillByPattern(ctx, "t", p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if byPat.ID != res.Skill.ID {
		t.Fatalf("by pattern id=%s", byPat.ID)
	}
}

func TestProposeSkillRejectsWeakCandidatePattern(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	patterns := experience.NewMemoryPatternRepository()
	skills := experience.NewMemorySkillRepository()
	svc := experience.NewService(experience.NewMemoryRepository()).
		WithPatterns(patterns).
		WithSkills(skills)

	p, err := patterns.Create(ctx, experience.Pattern{
		ID: "weak", TenantID: "t", Type: experience.TypeProcedural, Scope: experience.ScopeTenant,
		Trigger: "ops", Content: "rule", Confidence: 0.5, Utility: 0.4, SupportCount: 1,
		Status: experience.PatternStatusCandidate,
		CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	res, err := svc.ProposeSkill(ctx, "t", experience.ProposeSkillInput{PatternID: p.ID})
	if err != nil {
		t.Fatal(err)
	}
	if res.Created {
		t.Fatal("weak candidate should not become skill")
	}
	if res.Skipped == "" {
		t.Fatal("expected skip reason")
	}
}
