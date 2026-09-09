package skillexec_test

import (
	"context"
	"testing"
	"time"

	"github.com/agent-experience-engine/agent-experience-engine/internal/skill"
	"github.com/agent-experience-engine/agent-experience-engine/internal/skillexec"
	"github.com/agent-experience-engine/agent-experience-engine/internal/skillruntime"
	"github.com/agent-experience-engine/agent-experience-engine/internal/toolprovider"
	"github.com/agent-experience-engine/agent-experience-engine/internal/toolprovider/simulator"
	"github.com/agent-experience-engine/agent-experience-engine/internal/toolregistry"
)

func TestRecoveryContinuesFromStepCursor(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	tools := toolregistry.Default()
	router := toolprovider.NewRouter([]toolprovider.Provider{&simulator.JiraProvider{Registry: tools}}, tools, nil)
	_ = router.SyncRegistry(ctx)
	store := skillruntime.NewMemoryExecutionStore()
	rt := &skillruntime.Runtime{Tools: tools, Exec: router, Preview: router, Store: store, LeaseOwner: "test", LeaseTTL: time.Second}

	spec, err := skill.ParseYAML(`
name: jira_safe
inputs:
  project_name: {type: string, required: true}
  title: {type: string, required: true}
steps:
  - id: resolve_project
    tool: jira.search_projects
    args: {query: "{{ inputs.project_name }}"}
    save_as: project
  - id: create_issue
    tool: jira.create_issue
    args:
      project: "{{ project.key }}"
      title: "{{ inputs.title }}"
risk: {level: LOW}
max_steps: 5
`)
	if err != nil {
		t.Fatal(err)
	}

	ex, steps, err := rt.Run(ctx, skill.ExecutionRunRequest{
		TenantID: "t", SkillID: "s", SkillVersionID: "v", Mode: skill.ModeLive,
		Spec: spec, Inputs: map[string]any{"project_name": "Payment", "title": "x"},
		AvailableTools: []string{"jira.search_projects", "jira.create_issue"},
		RuntimeEnabled: true, IdempotencyKey: "rec-1",
	})
	if err != nil || ex.Status != skill.ExecSucceeded {
		t.Fatalf("status=%s steps=%d err=%v", ex.Status, len(steps), err)
	}

	// Simulate crash after first step: clone execution as RUNNING with cursor=1.
	partial := ex
	partial.ID = "crash-1"
	partial.Status = skill.ExecRunning
	partial.StepCursor = 1
	partial.CompletedAt = nil
	partial.IdempotencyKey = "crash-1"
	past := time.Now().UTC().Add(-time.Minute)
	partial.LeaseUntil = &past
	partial.SpecSnapshot = `
name: jira_safe
inputs:
  project_name: {type: string, required: true}
  title: {type: string, required: true}
steps:
  - id: resolve_project
    tool: jira.search_projects
    args: {query: "{{ inputs.project_name }}"}
    save_as: project
  - id: create_issue
    tool: jira.create_issue
    args:
      project: "{{ project.key }}"
      title: "{{ inputs.title }}"
risk: {level: LOW}
max_steps: 5
`
	partial.Inputs = map[string]any{"project_name": "Payment", "title": "x"}
	if _, err := store.CreateExecution(ctx, partial); err != nil {
		t.Fatal(err)
	}
	// Persist first step success so recovery rebuilds bindings.
	_, _ = store.CreateStep(ctx, skill.StepExecution{
		ID: "st1", ExecutionID: partial.ID, TenantID: "t", StepID: "resolve_project",
		Tool: "jira.search_projects", Status: skill.StepSucceeded, Sequence: 1,
		Output: map[string]any{"key": "PAY", "name": "Payment"},
	})

	worker := &skillexec.RecoveryWorker{Store: store, Runtime: rt, Owner: "worker"}
	n, err := worker.RecoverOnce(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("recovered=%d", n)
	}
	got, err := store.GetExecution(ctx, "t", partial.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != skill.ExecSucceeded {
		t.Fatalf("status=%s cursor=%d", got.Status, got.StepCursor)
	}
}
