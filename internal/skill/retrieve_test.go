package skill_test

import (
	"context"
	"testing"

	"github.com/agent-experience-engine/agent-experience-engine/internal/skill"
	"github.com/agent-experience-engine/agent-experience-engine/internal/skillvalidator"
	"github.com/agent-experience-engine/agent-experience-engine/internal/toolregistry"
)

func TestRetrieveRanksActiveValidatedAvailable(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := skill.NewMemoryRepository()
	tools := toolregistry.Default()
	svc := &skill.RegistryService{
		Repo:      repo,
		Validator: validatorBridge{inner: skillvalidator.New(tools, skillvalidator.Options{TenantID: "t"})},
	}

	_, ver, _, err := svc.CompileAndCreate(ctx, "t", "jira_safe_create_issue", "Resolve project then create issue", "p", jiraSafeCreateYAML, 0.95, 0.9)
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

	ranked, err := skill.Retrieve(ctx, repo, tools, skill.RetrieveQuery{
		TenantID: "t",
		Task:     "create jira issue safely with project resolve",
		Tools:    []string{"jira.search_projects", "jira.create_issue"},
		TopK:     5,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(ranked) != 1 {
		t.Fatalf("ranked=%d", len(ranked))
	}
	if ranked[0].Score <= 0 || ranked[0].Validity != 1 || ranked[0].Validation != 1 || ranked[0].Availability != 1 {
		t.Fatalf("%#v", ranked[0])
	}
	if ranked[0].Sim <= 0 {
		t.Fatalf("sim=%v", ranked[0].Sim)
	}
}

func TestRetrieveAvailabilityZeroWhenToolMissing(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := skill.NewMemoryRepository()
	tools := toolregistry.Default()
	svc := &skill.RegistryService{
		Repo:      repo,
		Validator: validatorBridge{inner: skillvalidator.New(tools, skillvalidator.Options{TenantID: "t"})},
	}
	_, ver, _, err := svc.CompileAndCreate(ctx, "t", "jira_safe_create_issue", "create issue", "", jiraSafeCreateYAML, 0.9, 0.8)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = svc.MoveToShadow(ctx, "t", ver.ID)
	for i := 0; i < 5; i++ {
		_, _ = svc.RecordShadowOutcome(ctx, "t", ver.ID, true)
	}
	_, _ = svc.Activate(ctx, "t", ver.ID, skill.DefaultPromoteConfig())

	ranked, err := skill.Retrieve(ctx, repo, tools, skill.RetrieveQuery{
		TenantID: "t",
		Task:     "jira create issue",
		Tools:    []string{"jira.search_projects"}, // missing create_issue
		TopK:     5,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(ranked) != 1 || ranked[0].Availability != 0 || ranked[0].Score != 0 {
		t.Fatalf("%#v", ranked)
	}
}

func TestRetrieveIgnoresNonActive(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := skill.NewMemoryRepository()
	tools := toolregistry.Default()
	svc := &skill.RegistryService{
		Repo:      repo,
		Validator: validatorBridge{inner: skillvalidator.New(tools, skillvalidator.Options{TenantID: "t"})},
	}
	_, _, _, err := svc.CompileAndCreate(ctx, "t", "jira_safe_create_issue", "create", "", jiraSafeCreateYAML, 0.9, 0.8)
	if err != nil {
		t.Fatal(err)
	}
	// Validated but no ActiveVersionID → Retrieve skips.
	ranked, err := skill.Retrieve(ctx, repo, tools, skill.RetrieveQuery{
		TenantID: "t", Task: "jira", TopK: 5,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(ranked) != 0 {
		t.Fatalf("want empty, got %#v", ranked)
	}
}
