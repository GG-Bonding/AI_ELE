package chaos_test

import (
	"context"
	"testing"
	"time"

	"github.com/agent-experience-engine/agent-experience-engine/internal/skill"
	"github.com/agent-experience-engine/agent-experience-engine/internal/skillruntime"
	"github.com/agent-experience-engine/agent-experience-engine/internal/toolprovider"
	"github.com/agent-experience-engine/agent-experience-engine/internal/toolprovider/credential"
	"github.com/agent-experience-engine/agent-experience-engine/internal/toolprovider/simulator"
	"github.com/agent-experience-engine/agent-experience-engine/internal/toolregistry"
)

func jiraSafeYAML() string {
	return `
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
}

func TestChaosStableOperationKeyIgnoresAttempt(t *testing.T) {
	t.Parallel()
	k1 := toolprovider.ToolIdempotencyKey("ex", "refund", 1)
	k2 := toolprovider.ToolIdempotencyKey("ex", "refund", 2)
	if k1 != k2 || k1 != "ex:refund" {
		t.Fatalf("%s vs %s", k1, k2)
	}
}

func TestChaosCredentialIsolation(t *testing.T) {
	t.Parallel()
	r := credential.NewMapResolver(credential.Options{AllowTenantFallback: false})
	r.PutPrincipalCredential("t1", "userA", "jira.create_issue", map[string]string{"Authorization": "A"})
	r.PutPrincipalCredential("t1", "userB", "jira.create_issue", map[string]string{"Authorization": "B"})
	r.PutTenantCredential("t1", "jira.create_issue", map[string]string{"Authorization": "TENANT"})

	a, _ := r.Resolve(context.Background(), "t1", "userA", "jira.create_issue")
	b, _ := r.Resolve(context.Background(), "t1", "userB", "jira.create_issue")
	if a["Authorization"] != "A" || b["Authorization"] != "B" {
		t.Fatalf("a=%v b=%v", a, b)
	}
	miss, _ := r.Resolve(context.Background(), "t1", "userC", "jira.create_issue")
	if len(miss) != 0 {
		t.Fatalf("expected no tenant fallback, got %v", miss)
	}
}

func TestChaosLeaseFencingBlocksStaleWriter(t *testing.T) {
	t.Parallel()
	store := skillruntime.NewMemoryExecutionStore()
	ctx := context.Background()
	until := time.Now().UTC().Add(-time.Second)
	_, err := store.CreateExecution(ctx, skill.Execution{
		ID: "e1", TenantID: "t", SkillID: "s", SkillVersionID: "v",
		Mode: skill.ModeLive, Status: skill.ExecRunning, LeaseEpoch: 1,
		LeaseUntil: &until, LeaseOwner: "worker-a",
	})
	if err != nil {
		t.Fatal(err)
	}

	claimed, ok, err := store.ClaimExecutionLease(ctx, "t", "e1", "worker-b", time.Now().UTC().Add(time.Minute))
	if err != nil || !ok {
		t.Fatalf("claim ok=%v err=%v", ok, err)
	}
	if claimed.LeaseEpoch != 2 {
		t.Fatalf("epoch=%d", claimed.LeaseEpoch)
	}
	stale := claimed
	stale.Status = skill.ExecSucceeded
	stale.LeaseEpoch = 1 // stale worker A
	_, ok, err = store.UpdateExecutionFenced(ctx, stale, 1)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("stale epoch must be rejected")
	}
}

func TestChaosStepAttemptPersistedWithStableKey(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	tools := toolregistry.Default()
	store := skillruntime.NewMemoryExecutionStore()
	router := toolprovider.NewRouter([]toolprovider.Provider{&simulator.JiraProvider{Registry: tools}}, tools, nil)
	_ = router.SyncRegistry(ctx)
	rt := &skillruntime.Runtime{Tools: tools, Exec: router, Preview: router, Store: store, LeaseOwner: "chaos"}
	spec, err := skill.ParseYAML(jiraSafeYAML())
	if err != nil {
		t.Fatal(err)
	}
	ex, steps, err := rt.Run(ctx, skill.ExecutionRunRequest{
		TenantID: "t", SkillID: "s", SkillVersionID: "v", Mode: skill.ModeLive, Spec: spec,
		Inputs:         map[string]any{"project_name": "Payment", "title": "x"},
		AvailableTools: []string{"jira.search_projects", "jira.create_issue"},
		RuntimeEnabled: true, IdempotencyKey: "chaos-attempt", RequesterID: "userA",
	})
	if err != nil {
		t.Fatal(err)
	}
	if ex.Status != skill.ExecSucceeded || len(steps) == 0 {
		t.Fatalf("status=%s steps=%d", ex.Status, len(steps))
	}
	for _, st := range steps {
		if st.Status == skill.StepSkipped {
			continue
		}
		want := skillruntime.OperationKey(ex.ID, st.StepID)
		if st.OperationKey != want {
			t.Fatalf("opkey=%q want=%q", st.OperationKey, want)
		}
		if st.Attempt < 1 {
			t.Fatalf("attempt=%d", st.Attempt)
		}
	}
}

func TestChaosStepFencingBlocksStaleWriter(t *testing.T) {
	t.Parallel()
	store := skillruntime.NewMemoryExecutionStore()
	ctx := context.Background()
	until := time.Now().UTC().Add(-time.Second)
	ex, err := store.CreateExecution(ctx, skill.Execution{
		ID: "e2", TenantID: "t", SkillID: "s", SkillVersionID: "v",
		Mode: skill.ModeLive, Status: skill.ExecRunning, LeaseEpoch: 1,
		LeaseUntil: &until, LeaseOwner: "worker-a",
	})
	if err != nil {
		t.Fatal(err)
	}
	st, err := store.CreateStep(ctx, skill.StepExecution{
		ID: "st1", ExecutionID: ex.ID, TenantID: "t", StepID: "refund",
		Tool: "pay.refund", Status: skill.StepRunning, Attempt: 1,
		OperationKey: "e2:refund", LeaseEpoch: 1, Sequence: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	claimed, ok, err := store.ClaimExecutionLease(ctx, "t", "e2", "worker-b", time.Now().UTC().Add(time.Minute))
	if err != nil || !ok {
		t.Fatalf("claim ok=%v err=%v", ok, err)
	}
	if claimed.LeaseEpoch != 2 {
		t.Fatalf("epoch=%d", claimed.LeaseEpoch)
	}
	st.Status = skill.StepSucceeded
	_, ok, err = store.UpdateStepFenced(ctx, st, 1) // stale worker A
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("stale step update must be rejected")
	}
	staleCreate := skill.StepExecution{
		ID: "stale-create", ExecutionID: ex.ID, TenantID: "t", StepID: "refund",
		Tool: "pay.refund", Status: skill.StepSucceeded, Attempt: 2,
		OperationKey: "e2:refund", LeaseEpoch: 1, Sequence: 2,
	}
	_, ok, err = store.CreateStepFenced(ctx, staleCreate, 1)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("stale step create must be rejected")
	}
}

func TestChaosRecoverNativeUnknownUsesNextAttempt(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	tools := toolregistry.Default()
	def, _ := tools.Get("jira.search_projects")
	def.IdempotencyCapability = toolregistry.IdempotencyNative
	_ = tools.Register(def)

	store := skillruntime.NewMemoryExecutionStore()
	router := toolprovider.NewRouter([]toolprovider.Provider{&simulator.JiraProvider{Registry: tools}}, tools, nil)
	_ = router.SyncRegistry(ctx)
	rt := &skillruntime.Runtime{Tools: tools, Exec: router, Preview: router, Store: store, LeaseOwner: "chaos"}

	specYAML := `
name: search_only
inputs:
  project_name: {type: string, required: true}
steps:
  - id: resolve_project
    tool: jira.search_projects
    args: {query: "{{ inputs.project_name }}"}
    save_as: project
risk: {level: LOW}
max_steps: 2
`
	spec, err := skill.ParseYAML(specYAML)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	until := now.Add(-time.Second)
	ex, err := store.CreateExecution(ctx, skill.Execution{
		ID: "crash1", TenantID: "t", SkillID: "s", SkillVersionID: "v",
		Mode: skill.ModeLive, Status: skill.ExecRunning, LeaseEpoch: 1,
		StepCursor: 0, LeaseUntil: &until, LeaseOwner: "dead-worker",
		Inputs: map[string]any{"project_name": "Payment"}, SpecSnapshot: specYAML,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.CreateStep(ctx, skill.StepExecution{
		ID: "att1", ExecutionID: ex.ID, TenantID: "t", StepID: "resolve_project",
		Tool: "jira.search_projects", Status: skill.StepRunning, Attempt: 1,
		OperationKey: skillruntime.OperationKey(ex.ID, "resolve_project"),
		LeaseEpoch: 1, Sequence: 1, Input: map[string]any{"query": "Payment"},
	})
	if err != nil {
		t.Fatal(err)
	}

	claimed, ok, err := store.ClaimExecutionLease(ctx, "t", ex.ID, "recovery", now.Add(time.Minute))
	if err != nil || !ok {
		t.Fatalf("claim ok=%v err=%v", ok, err)
	}
	out, steps, err := rt.Recover(ctx, claimed.TenantID, claimed.ID, spec)
	if err != nil {
		t.Fatal(err)
	}
	if out.Status != skill.ExecSucceeded {
		t.Fatalf("status=%s err=%s/%s", out.Status, out.ErrorCode, out.ErrorMessage)
	}
	var sawAttempt2 bool
	for _, st := range steps {
		if st.StepID == "resolve_project" && st.Attempt == 2 && st.Status == skill.StepSucceeded {
			sawAttempt2 = true
			if st.OperationKey != skillruntime.OperationKey(ex.ID, "resolve_project") {
				t.Fatalf("opkey=%q", st.OperationKey)
			}
		}
	}
	if !sawAttempt2 {
		t.Fatalf("expected attempt=2 success after recovery, steps=%+v", steps)
	}
}

func TestChaosRecoverNoneUnknownNeedsReconciliation(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	tools := toolregistry.Default()
	store := skillruntime.NewMemoryExecutionStore()
	router := toolprovider.NewRouter([]toolprovider.Provider{&simulator.JiraProvider{Registry: tools}}, tools, nil)
	_ = router.SyncRegistry(ctx)
	rt := &skillruntime.Runtime{Tools: tools, Exec: router, Preview: router, Store: store, LeaseOwner: "chaos"}

	specYAML := `
name: create_issue
inputs:
  project_name: {type: string, required: true}
  title: {type: string, required: true}
steps:
  - id: create_issue
    tool: jira.create_issue
    args:
      project: "{{ inputs.project_name }}"
      title: "{{ inputs.title }}"
risk: {level: LOW}
max_steps: 1
`
	spec, err := skill.ParseYAML(specYAML)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	until := now.Add(-time.Second)
	ex, err := store.CreateExecution(ctx, skill.Execution{
		ID: "crash2", TenantID: "t", SkillID: "s", SkillVersionID: "v",
		Mode: skill.ModeLive, Status: skill.ExecRunning, LeaseEpoch: 1,
		StepCursor: 0, LeaseUntil: &until, LeaseOwner: "dead-worker",
		Inputs: map[string]any{"project_name": "PAY", "title": "x"}, SpecSnapshot: specYAML,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.CreateStep(ctx, skill.StepExecution{
		ID: "att1", ExecutionID: ex.ID, TenantID: "t", StepID: "create_issue",
		Tool: "jira.create_issue", Status: skill.StepRunning, Attempt: 1,
		OperationKey: skillruntime.OperationKey(ex.ID, "create_issue"),
		LeaseEpoch: 1, Sequence: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	claimed, ok, err := store.ClaimExecutionLease(ctx, "t", ex.ID, "recovery", now.Add(time.Minute))
	if err != nil || !ok {
		t.Fatalf("claim ok=%v err=%v", ok, err)
	}
	out, _, err := rt.Recover(ctx, claimed.TenantID, claimed.ID, spec)
	if err != nil {
		t.Fatal(err)
	}
	if out.Status != skill.ExecNeedsReconciliation {
		t.Fatalf("status=%s want NEEDS_RECONCILIATION", out.Status)
	}
}
