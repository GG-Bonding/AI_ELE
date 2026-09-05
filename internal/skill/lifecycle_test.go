package skill_test

import (
	"context"
	"errors"
	"testing"

	"github.com/agent-experience-engine/agent-experience-engine/internal/skill"
	"github.com/agent-experience-engine/agent-experience-engine/internal/skillvalidator"
	"github.com/agent-experience-engine/agent-experience-engine/internal/toolregistry"
)

// validatorBridge adapts *skillvalidator.Validator to skill.SpecValidator
// (skill cannot import skillvalidator without a cycle).
type validatorBridge struct {
	inner *skillvalidator.Validator
}

func (b validatorBridge) Validate(spec skill.Spec) skill.ValidationReport {
	r := b.inner.Validate(spec)
	return skill.ValidationReport{
		OK:               r.OK,
		Normalized:       r.Normalized,
		ComputedRisk:     r.ComputedRisk,
		RequiresApproval: r.RequiresApproval,
	}
}

func newRegistryService(t *testing.T) (*skill.RegistryService, *skill.MemoryRepository) {
	t.Helper()
	repo := skill.NewMemoryRepository()
	v := skillvalidator.New(toolregistry.Default(), skillvalidator.Options{TenantID: "t"})
	return &skill.RegistryService{Repo: repo, Validator: validatorBridge{inner: v}}, repo
}

func TestCompileAndCreateValidatesAndPromotes(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	svc, _ := newRegistryService(t)

	sk, ver, rep, err := svc.CompileAndCreate(ctx, "t", "jira_safe_create_issue", "safe create", "pat_1", jiraSafeCreateYAML, 0.9, 0.8)
	if err != nil {
		t.Fatalf("CompileAndCreate: %v", err)
	}
	if !rep.OK {
		t.Fatalf("want validation OK, got %#v", rep)
	}
	if sk.Status != skill.StatusValidated {
		t.Fatalf("skill status=%s", sk.Status)
	}
	if ver.Status != skill.VersionValidated || ver.ValidationStatus != skill.ValidationPassed {
		t.Fatalf("version=%#v", ver)
	}
	if ver.Alpha <= 0 || ver.Beta <= 0 {
		t.Fatalf("beta not seeded: %#v", ver)
	}
}

func TestMoveToShadowRequiresPassed(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	svc, repo := newRegistryService(t)

	sk, err := repo.CreateSkill(ctx, skill.Skill{TenantID: "t", Name: "x", Status: skill.StatusCandidate})
	if err != nil {
		t.Fatal(err)
	}
	ver, err := repo.CreateVersion(ctx, skill.Version{
		TenantID: "t", SkillID: sk.ID, Version: 1,
		Status: skill.VersionCandidate, ValidationStatus: skill.ValidationPending,
		Spec: skill.Spec{Name: "x", Steps: []skill.SkillStep{{ID: "a", Tool: "jira.search_projects"}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.MoveToShadow(ctx, "t", ver.ID); !errors.Is(err, skill.ErrInvalidTransition) {
		t.Fatalf("err=%v", err)
	}

	_, ver2, _, err := svc.CompileAndCreate(ctx, "t", "jira_safe_create_issue", "", "", jiraSafeCreateYAML, 0.9, 0.8)
	if err != nil {
		t.Fatal(err)
	}
	out, err := svc.MoveToShadow(ctx, "t", ver2.ID)
	if err != nil {
		t.Fatal(err)
	}
	if out.Status != skill.VersionShadow {
		t.Fatalf("%#v", out)
	}
	sk2, _ := repo.GetSkill(ctx, "t", ver2.SkillID)
	if sk2.Status != skill.StatusShadow {
		t.Fatalf("skill=%s", sk2.Status)
	}
}

func TestActivateShadowGatesAndDeprecatePrevious(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	svc, repo := newRegistryService(t)

	_, ver, _, err := svc.CompileAndCreate(ctx, "t", "jira_safe_create_issue", "", "p", jiraSafeCreateYAML, 0.9, 0.85)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.MoveToShadow(ctx, "t", ver.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Activate(ctx, "t", ver.ID, skill.DefaultPromoteConfig()); !errors.Is(err, skill.ErrPromotionGate) {
		t.Fatalf("want promotion gate, got %v", err)
	}

	for i := 0; i < 5; i++ {
		if _, err := svc.RecordShadowOutcome(ctx, "t", ver.ID, true); err != nil {
			t.Fatal(err)
		}
	}
	active, err := svc.Activate(ctx, "t", ver.ID, skill.DefaultPromoteConfig())
	if err != nil {
		t.Fatal(err)
	}
	if active.Status != skill.VersionActive {
		t.Fatalf("%#v", active)
	}
	sk, _ := repo.GetSkill(ctx, "t", ver.SkillID)
	if sk.Status != skill.StatusActive || sk.ActiveVersionID == nil || *sk.ActiveVersionID != ver.ID {
		t.Fatalf("%#v", sk)
	}

	// Propose revision → shadow → activate demotes prior.
	revYAML := jiraSafeCreateYAML
	rev, err := skill.ProposeRevision(ctx, repo, "t", ver.SkillID, revYAML, "p2")
	if err != nil {
		t.Fatal(err)
	}
	v := skillvalidator.New(toolregistry.Default(), skillvalidator.Options{TenantID: "t"})
	rep := validatorBridge{inner: v}.Validate(rev.Spec)
	rev = skill.ApplyValidationReport(rev, rep)
	rev, err = repo.UpdateVersion(ctx, rev)
	if err != nil || !rep.OK {
		t.Fatalf("validate rev: ok=%v err=%v", rep.OK, err)
	}
	if _, err := svc.MoveToShadow(ctx, "t", rev.ID); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5; i++ {
		if _, err := svc.RecordShadowOutcome(ctx, "t", rev.ID, true); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := svc.Activate(ctx, "t", rev.ID, skill.DefaultPromoteConfig()); err != nil {
		t.Fatal(err)
	}
	old, err := repo.GetVersion(ctx, "t", ver.ID)
	if err != nil || old.Status != skill.VersionDeprecated {
		t.Fatalf("old=%#v err=%v", old, err)
	}
}

func TestSuspendAndMaybeSuspendFromLiveStats(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	svc, repo := newRegistryService(t)

	_, ver, _, err := svc.CompileAndCreate(ctx, "t", "jira_safe_create_issue", "", "", jiraSafeCreateYAML, 0.9, 0.8)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.MoveToShadow(ctx, "t", ver.ID); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5; i++ {
		_, _ = svc.RecordShadowOutcome(ctx, "t", ver.ID, true)
	}
	if _, err := svc.Activate(ctx, "t", ver.ID, skill.DefaultPromoteConfig()); err != nil {
		t.Fatal(err)
	}

	ver, err = repo.GetVersion(ctx, "t", ver.ID)
	if err != nil {
		t.Fatal(err)
	}
	ver.SuccessCount = 3
	ver.FailureCount = 7 // 70% failure > 30%
	if _, err := repo.UpdateVersion(ctx, ver); err != nil {
		t.Fatal(err)
	}

	suspended, err := svc.MaybeSuspendFromLiveStats(ctx, "t", ver.ID, skill.DefaultPromoteConfig())
	if err != nil || !suspended {
		t.Fatalf("suspended=%v err=%v", suspended, err)
	}
	sk, _ := repo.GetSkill(ctx, "t", ver.SkillID)
	if sk.Status != skill.StatusSuspended {
		t.Fatalf("skill=%s", sk.Status)
	}
	got, _ := repo.GetVersion(ctx, "t", ver.ID)
	if got.Status != skill.VersionSuspended {
		t.Fatalf("version=%s", got.Status)
	}
}

func TestProposeRevisionIncrementsImmutable(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	svc, repo := newRegistryService(t)
	_, ver, _, err := svc.CompileAndCreate(ctx, "t", "jira_safe_create_issue", "", "p", jiraSafeCreateYAML, 0.9, 0.8)
	if err != nil {
		t.Fatal(err)
	}
	rev, err := skill.ProposeRevision(ctx, repo, "t", ver.SkillID, jiraSafeCreateYAML, "p2")
	if err != nil {
		t.Fatal(err)
	}
	if rev.Version != 2 || rev.Status != skill.VersionCandidate {
		t.Fatalf("%#v", rev)
	}
	orig, _ := repo.GetVersion(ctx, "t", ver.ID)
	if orig.Version != 1 || orig.SpecHash == "" {
		t.Fatalf("original mutated? %#v", orig)
	}
}

func TestOfflineCompareAndACE(t *testing.T) {
	t.Parallel()
	if got := skill.EstimateACE(1.0, 0.2); got != 0.8 {
		t.Fatalf("ACE=%v", got)
	}
	cmp := skill.OfflineCompare(4, 7)
	if cmp.StepDelta != -3 || cmp.ACE != 3 {
		t.Fatalf("%#v", cmp)
	}
}
