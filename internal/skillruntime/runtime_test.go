package skillruntime_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/agent-experience-engine/agent-experience-engine/internal/eval/jirasim"
	"github.com/agent-experience-engine/agent-experience-engine/internal/skill"
	"github.com/agent-experience-engine/agent-experience-engine/internal/skillruntime"
	"github.com/agent-experience-engine/agent-experience-engine/internal/toolregistry"
)

func jiraSafeSpec() skill.Spec {
	return skill.Spec{
		Name: "jira_safe_create_issue",
		Inputs: map[string]skill.FieldSchema{
			"project_name": {Type: skill.FieldString, Required: true},
			"title":        {Type: skill.FieldString, Required: true},
		},
		Steps: []skill.SkillStep{
			{
				ID:     "resolve_project",
				Tool:   "jira.search_projects",
				Args:   map[string]any{"query": "{{ inputs.project_name }}"},
				SaveAs: "project",
			},
			{
				ID:   "create_issue",
				Tool: "jira.create_issue",
				Args: map[string]any{
					"project": "{{ project.key }}",
					"title":   "{{ inputs.title }}",
				},
			},
		},
		Risk:      skill.SkillRisk{Level: skill.RiskLow},
		MaxSteps:  10,
		TimeoutMs: 30_000,
	}
}

func newTestRuntime(t *testing.T) (*skillruntime.Runtime, *skillruntime.MemoryExecutionStore) {
	t.Helper()
	store := skillruntime.NewMemoryExecutionStore()
	reg := toolregistry.Default()
	exec := &skillruntime.JiraSimExecutor{Sim: jirasim.New(), Registry: reg}
	var seq int
	rt := &skillruntime.Runtime{
		Tools:   reg,
		Exec:    exec,
		Preview: exec,
		Policy:  skillruntime.DefaultPolicy{},
		Store:   store,
		IDs: func() string {
			seq++
			return fmt.Sprintf("id_%d", seq)
		},
		Now: func() time.Time { return time.Now().UTC() },
	}
	return rt, store
}

func TestShadowJiraSkillSucceeds(t *testing.T) {
	t.Parallel()
	rt, _ := newTestRuntime(t)
	ex, steps, err := rt.Run(context.Background(), skill.ExecutionRunRequest{
		TenantID:       "t1",
		SkillID:        "sk1",
		SkillVersionID: "skv1",
		Mode:           skill.ModeShadow,
		Spec:           jiraSafeSpec(),
		Inputs: map[string]any{
			"project_name": "Payment",
			"title":        "payment timeout",
		},
		AvailableTools: []string{"jira.search_projects", "jira.create_issue"},
		RuntimeEnabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if ex.Status != skill.ExecSucceeded {
		t.Fatalf("status=%s code=%s msg=%s", ex.Status, ex.ErrorCode, ex.ErrorMessage)
	}
	if len(steps) != 2 {
		t.Fatalf("steps=%d", len(steps))
	}
	if steps[0].Status != skill.StepSucceeded {
		t.Fatalf("search step=%s", steps[0].Status)
	}
	if steps[1].Status != skill.StepShadowed {
		t.Fatalf("create step want SHADOWED got %s out=%v", steps[1].Status, steps[1].Output)
	}
	if steps[1].Output["_shadow"] != true {
		t.Fatalf("create output=%v", steps[1].Output)
	}
}

func TestLiveDeniedWhenRuntimeDisabled(t *testing.T) {
	t.Parallel()
	rt, _ := newTestRuntime(t)
	ex, steps, err := rt.Run(context.Background(), skill.ExecutionRunRequest{
		TenantID:       "t1",
		SkillID:        "sk1",
		SkillVersionID: "skv1",
		Mode:           skill.ModeLive,
		Spec:           jiraSafeSpec(),
		Inputs:         map[string]any{"project_name": "Payment", "title": "x"},
		AvailableTools: []string{"jira.search_projects", "jira.create_issue"},
		RuntimeEnabled: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	if ex.Status != skill.ExecDenied {
		t.Fatalf("status=%s", ex.Status)
	}
	if len(steps) != 0 {
		t.Fatalf("steps=%d", len(steps))
	}
}

func TestHighRiskRequiresApprovalThenResume(t *testing.T) {
	t.Parallel()
	rt, store := newTestRuntime(t)
	spec := jiraSafeSpec()
	spec.Risk = skill.SkillRisk{Level: skill.RiskHigh}
	ex, steps, err := rt.Run(context.Background(), skill.ExecutionRunRequest{
		TenantID:       "t1",
		SkillID:        "sk1",
		SkillVersionID: "skv1",
		Mode:           skill.ModeLive,
		Spec:           spec,
		Inputs:         map[string]any{"project_name": "Payment", "title": "x"},
		AvailableTools: []string{"jira.search_projects", "jira.create_issue"},
		RuntimeEnabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if ex.Status != skill.ExecWaitingApproval {
		t.Fatalf("status=%s code=%s", ex.Status, ex.ErrorCode)
	}
	if len(steps) != 0 {
		t.Fatalf("steps=%d", len(steps))
	}
	appr, err := store.GetApprovalByExecution(context.Background(), "t1", ex.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rt.Approve(context.Background(), "t1", appr.ID); err != nil {
		t.Fatal(err)
	}
	ex2, steps2, err := rt.Resume(context.Background(), skill.ResumeRequest{
		TenantID:       "t1",
		ExecutionID:    ex.ID,
		AvailableTools: []string{"jira.search_projects", "jira.create_issue"},
		RuntimeEnabled: true,
		Spec:           spec,
	})
	if err != nil {
		t.Fatal(err)
	}
	if ex2.Status != skill.ExecSucceeded {
		t.Fatalf("resume status=%s code=%s msg=%s", ex2.Status, ex2.ErrorCode, ex2.ErrorMessage)
	}
	if len(steps2) != 2 {
		t.Fatalf("steps=%d", len(steps2))
	}
}

func TestMissingToolDenies(t *testing.T) {
	t.Parallel()
	rt, _ := newTestRuntime(t)
	ex, _, err := rt.Run(context.Background(), skill.ExecutionRunRequest{
		TenantID:       "t1",
		SkillID:        "sk1",
		SkillVersionID: "skv1",
		Mode:           skill.ModeShadow,
		Spec:           jiraSafeSpec(),
		Inputs:         map[string]any{"project_name": "Payment", "title": "x"},
		AvailableTools: []string{"jira.search_projects"},
		RuntimeEnabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if ex.Status != skill.ExecDenied {
		t.Fatalf("status=%s msg=%s", ex.Status, ex.ErrorMessage)
	}
}

func TestTemplateResolution(t *testing.T) {
	t.Parallel()
	bindings := map[string]any{
		"inputs": map[string]any{
			"project_name": "Payment",
			"title":        "timeout",
		},
		"project": map[string]any{
			"key":  "PAY",
			"name": "Payment",
		},
	}
	resolved, err := skillruntime.ResolveArgs(map[string]any{
		"query":   "{{ inputs.project_name }}",
		"project": "{{ project.key }}",
		"title":   "{{ inputs.title }}",
		"label":   "issue: {{ project.key }}",
	}, bindings)
	if err != nil {
		t.Fatal(err)
	}
	if resolved["query"] != "Payment" {
		t.Fatalf("query=%v", resolved["query"])
	}
	if resolved["project"] != "PAY" {
		t.Fatalf("project=%v", resolved["project"])
	}
	if resolved["title"] != "timeout" {
		t.Fatalf("title=%v", resolved["title"])
	}
	if resolved["label"] != "issue: PAY" {
		t.Fatalf("label=%v", resolved["label"])
	}
}

func TestIdempotencyReturnsExisting(t *testing.T) {
	t.Parallel()
	rt, _ := newTestRuntime(t)
	req := skill.ExecutionRunRequest{
		TenantID:       "t1",
		SkillID:        "sk1",
		SkillVersionID: "skv1",
		Mode:           skill.ModeShadow,
		Spec:           jiraSafeSpec(),
		Inputs:         map[string]any{"project_name": "Payment", "title": "x"},
		IdempotencyKey: "idem-1",
		AvailableTools: []string{"jira.search_projects", "jira.create_issue"},
		RuntimeEnabled: true,
	}
	ex1, steps1, err := rt.Run(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	ex2, steps2, err := rt.Run(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if ex1.ID != ex2.ID {
		t.Fatalf("idempotency broken: %s vs %s", ex1.ID, ex2.ID)
	}
	if len(steps1) != len(steps2) {
		t.Fatalf("steps %d vs %d", len(steps1), len(steps2))
	}
}

func TestShadowRequiresPreview(t *testing.T) {
	t.Parallel()
	store := skillruntime.NewMemoryExecutionStore()
	reg := toolregistry.Default()
	rt := &skillruntime.Runtime{
		Tools:  reg,
		Exec:   &skillruntime.JiraSimExecutor{Sim: jirasim.New(), Registry: reg},
		Policy: skillruntime.DefaultPolicy{},
		Store:  store,
		// Preview intentionally nil
	}
	ex, _, err := rt.Run(context.Background(), skill.ExecutionRunRequest{
		TenantID: "t1", SkillID: "sk1", SkillVersionID: "skv1",
		Mode: skill.ModeShadow, Spec: jiraSafeSpec(),
		Inputs:         map[string]any{"project_name": "Payment", "title": "x"},
		AvailableTools: []string{"jira.search_projects", "jira.create_issue"},
		RuntimeEnabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if ex.Status != skill.ExecFailed || ex.ErrorCode != "SHADOW_UNSUPPORTED" {
		t.Fatalf("status=%s code=%s", ex.Status, ex.ErrorCode)
	}
}

func TestWhenSkipAndOnErrorContinue(t *testing.T) {
	t.Parallel()
	rt, _ := newTestRuntime(t)
	spec := jiraSafeSpec()
	spec.Steps[0].When = &skill.Condition{Expr: `inputs.project_name == "Other"`}
	spec.Steps[0].OnError = &skill.ErrorPolicy{Action: "continue"}
	// First step skipped → create_issue will fail template (no project) — make create continue too
	spec.Steps[1].OnError = &skill.ErrorPolicy{Action: "continue"}
	ex, steps, err := rt.Run(context.Background(), skill.ExecutionRunRequest{
		TenantID: "t1", SkillID: "sk1", SkillVersionID: "skv1",
		Mode: skill.ModeShadow, Spec: spec,
		Inputs:         map[string]any{"project_name": "Payment", "title": "x"},
		AvailableTools: []string{"jira.search_projects", "jira.create_issue"},
		RuntimeEnabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if ex.Status != skill.ExecSucceeded {
		t.Fatalf("status=%s msg=%s", ex.Status, ex.ErrorMessage)
	}
	if steps[0].Status != skill.StepSkipped {
		t.Fatalf("step0=%s", steps[0].Status)
	}
}
