package skill_test

import (
	"context"
	"testing"

	"github.com/agent-experience-engine/agent-experience-engine/internal/eval/jirasim"
	"github.com/agent-experience-engine/agent-experience-engine/internal/skill"
	"github.com/agent-experience-engine/agent-experience-engine/internal/skillruntime"
	"github.com/agent-experience-engine/agent-experience-engine/internal/skillvalidator"
	"github.com/agent-experience-engine/agent-experience-engine/internal/toolregistry"
)

func TestExecuteRejectsCandidateVersion(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	tools := toolregistry.Default()
	repo := skill.NewMemoryRepository()
	store := skillruntime.NewMemoryExecutionStore()
	exec := &skillruntime.JiraSimExecutor{Sim: jirasim.New(), Registry: tools}
	rt := &skillruntime.Runtime{Tools: tools, Exec: exec, Preview: exec, Store: store, Policy: skillruntime.DefaultPolicy{}}
	reg := &skill.RegistryService{
		Repo:      repo,
		Validator: skillvalidator.Adapt(skillvalidator.New(tools, skillvalidator.Options{TenantID: "t"})),
	}
	_, ver, _, err := reg.CompileAndCreate(ctx, "t", "jira_safe_create_issue", "", "", jiraSafeCreateYAML, 0.9, 0.5)
	if err != nil {
		t.Fatal(err)
	}
	svc := &skill.ExecutionService{Repo: repo, Store: store, Runner: rt, Registry: reg}
	_, _, err = svc.Execute(ctx, skill.ExecuteInput{
		TenantID: "t", VersionID: ver.ID, Mode: skill.ModeShadow,
		Inputs:         map[string]any{"project_name": "Payment", "title": "x"},
		AvailableTools: []string{"jira.search_projects", "jira.create_issue"},
		RuntimeEnabled: true,
	})
	if err == nil {
		t.Fatal("expected lifecycle error for VALIDATED-only version without SHADOW")
	}
}

func TestExecuteAllowsShadowThenLive(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	tools := toolregistry.Default()
	repo := skill.NewMemoryRepository()
	store := skillruntime.NewMemoryExecutionStore()
	jira := &skillruntime.JiraSimExecutor{Sim: jirasim.New(), Registry: tools}
	rt := &skillruntime.Runtime{Tools: tools, Exec: jira, Preview: jira, Store: store, Policy: skillruntime.DefaultPolicy{}}
	reg := &skill.RegistryService{
		Repo:      repo,
		Validator: skillvalidator.Adapt(skillvalidator.New(tools, skillvalidator.Options{TenantID: "t"})),
	}
	_, ver, _, err := reg.CompileAndCreate(ctx, "t", "jira_safe_create_issue", "", "", jiraSafeCreateYAML, 0.9, 0.5)
	if err != nil {
		t.Fatal(err)
	}
	ver, err = reg.MoveToShadow(ctx, "t", ver.ID)
	if err != nil {
		t.Fatal(err)
	}
	svc := &skill.ExecutionService{Repo: repo, Store: store, Runner: rt, Registry: reg}
	ex, _, err := svc.Execute(ctx, skill.ExecuteInput{
		TenantID: "t", SkillID: ver.SkillID, VersionID: ver.ID, Mode: skill.ModeShadow,
		Inputs:         map[string]any{"project_name": "Payment", "title": "x"},
		AvailableTools: []string{"jira.search_projects", "jira.create_issue"},
		RuntimeEnabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if ex.Status != skill.ExecSucceeded {
		t.Fatalf("status=%s", ex.Status)
	}
}
