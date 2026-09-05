package skill_test

import (
	"context"
	"testing"

	"github.com/agent-experience-engine/agent-experience-engine/internal/skill"
	"github.com/agent-experience-engine/agent-experience-engine/internal/skillvalidator"
	"github.com/agent-experience-engine/agent-experience-engine/internal/toolregistry"
)

func TestApplyFeedbackExactlyOnce(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := skill.NewMemoryRepository()
	learn := skill.NewMemoryLearningStore()
	v := skillvalidator.New(toolregistry.Default(), skillvalidator.Options{TenantID: "t"})
	svc := &skill.RegistryService{Repo: repo, Validator: validatorBridge{inner: v}}

	_, ver, _, err := svc.CompileAndCreate(ctx, "t", "jira_safe_create_issue", "", "", jiraSafeCreateYAML, 0.9, 0.5)
	if err != nil {
		t.Fatal(err)
	}
	beforeUtil := ver.Utility

	if err := skill.ApplyFeedback(ctx, repo, learn, "t", "fb1", ver.ID, "ex1", 1.0, 1.0, 1.0); err != nil {
		t.Fatalf("ApplyFeedback: %v", err)
	}
	after, err := repo.GetVersion(ctx, "t", ver.ID)
	if err != nil {
		t.Fatal(err)
	}
	if after.Utility <= beforeUtil || after.SuccessCount != 1 {
		t.Fatalf("utility not updated: before=%v after=%#v", beforeUtil, after)
	}

	// Replay must not double-apply.
	if err := skill.ApplyFeedback(ctx, repo, learn, "t", "fb1", ver.ID, "ex1", 1.0, 1.0, 1.0); err != nil {
		t.Fatalf("duplicate: %v", err)
	}
	again, err := repo.GetVersion(ctx, "t", ver.ID)
	if err != nil {
		t.Fatal(err)
	}
	if again.Utility != after.Utility || again.SuccessCount != 1 || again.Alpha != after.Alpha {
		t.Fatalf("double apply: %#v vs %#v", again, after)
	}

	ev, err := learn.GetLearningEventByFeedbackVersion(ctx, "t", "fb1", ver.ID)
	if err != nil || ev.Status != "APPLIED" {
		t.Fatalf("event=%#v err=%v", ev, err)
	}
}

func TestApplyFeedbackNegativeLowersUtility(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := skill.NewMemoryRepository()
	learn := skill.NewMemoryLearningStore()
	svc := &skill.RegistryService{
		Repo: repo,
		Validator: validatorBridge{inner: skillvalidator.New(toolregistry.Default(), skillvalidator.Options{TenantID: "t"})},
	}
	_, ver, _, err := svc.CompileAndCreate(ctx, "t", "jira_safe_create_issue", "", "", jiraSafeCreateYAML, 0.9, 0.8)
	if err != nil {
		t.Fatal(err)
	}
	before := ver.Utility
	if err := skill.ApplyFeedback(ctx, repo, learn, "t", "fb-neg", ver.ID, "", -1.0, 1.0, 1.0); err != nil {
		t.Fatal(err)
	}
	got, _ := repo.GetVersion(ctx, "t", ver.ID)
	if got.Utility >= before || got.FailureCount != 1 {
		t.Fatalf("%#v", got)
	}
}

func TestApplyBetaUpdateAndSeed(t *testing.T) {
	t.Parallel()
	a, b := skill.SeedBetaFromUtility(0.75)
	if a <= 0 || b <= 0 {
		t.Fatalf("%v %v", a, b)
	}
	ver := skill.Version{Utility: 0.5, Alpha: a, Beta: b}
	up, err := skill.ApplyBetaUpdate(ver, 1.0, 1.0)
	if err != nil || up.Utility <= 0.5 || up.SuccessCount != 1 {
		t.Fatalf("%#v err=%v", up, err)
	}
}
