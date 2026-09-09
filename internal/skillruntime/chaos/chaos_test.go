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
	ex, err := store.CreateExecution(ctx, skill.Execution{
		ID: "e1", TenantID: "t", SkillID: "s", SkillVersionID: "v",
		Mode: skill.ModeLive, Status: skill.ExecRunning, LeaseEpoch: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	until := time.Now().UTC().Add(-time.Second)
	ex.LeaseUntil = &until
	_, _ = store.UpdateExecution(ctx, ex)

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
