package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/agent-experience-engine/agent-experience-engine/internal/skill"
	"github.com/agent-experience-engine/agent-experience-engine/internal/skillruntime"
	"github.com/agent-experience-engine/agent-experience-engine/internal/toolprovider"
	"github.com/agent-experience-engine/agent-experience-engine/internal/toolprovider/simulator"
	"github.com/agent-experience-engine/agent-experience-engine/internal/toolregistry"
	"github.com/agent-experience-engine/agent-experience-engine/storage/postgres"
	"github.com/google/uuid"
)

func TestPostgresCrashRecoveryAndFencing(t *testing.T) {
	db := openTestDB(t)
	repo := postgres.NewSkillExecutionRepository(db)
	ctx := context.Background()
	now := time.Now().UTC()
	until := now.Add(-time.Second)
	tenant := "pg_rec_" + uuid.NewString()[:8]
	execID := "ex_" + uuid.NewString()

	tools := toolregistry.Default()
	def, _ := tools.Get("jira.search_projects")
	def.IdempotencyCapability = toolregistry.IdempotencyNative
	_ = tools.Register(def)

	router := toolprovider.NewRouter([]toolprovider.Provider{&simulator.JiraProvider{Registry: tools}}, tools, nil)
	_ = router.SyncRegistry(ctx)
	rt := &skillruntime.Runtime{
		Tools: tools, Exec: router, Preview: router, Store: repo,
		LeaseOwner: "worker-b", LeaseTTL: time.Minute,
	}

	specYAML := `
name: search
inputs: {project_name: {type: string, required: true}}
steps:
  - id: resolve
    tool: jira.search_projects
    args: {query: "{{ inputs.project_name }}"}
    save_as: project
risk: {level: LOW}
max_steps: 1
`
	spec, err := skill.ParseYAML(specYAML)
	if err != nil {
		t.Fatal(err)
	}

	ex, err := repo.CreateExecution(ctx, skill.Execution{
		ID: execID, TenantID: tenant, SkillID: "s", SkillVersionID: "v",
		Mode: skill.ModeLive, Status: skill.ExecRunning, LeaseEpoch: 1,
		StepCursor: 0, LeaseUntil: &until, LeaseOwner: "worker-a",
		Inputs: map[string]any{"project_name": "Payment"}, SpecSnapshot: specYAML,
		StartedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	st, err := repo.CreateStep(ctx, skill.StepExecution{
		ID: "st_" + uuid.NewString(), ExecutionID: ex.ID, TenantID: tenant,
		StepID: "resolve", Tool: "jira.search_projects", Status: skill.StepRunning,
		Attempt: 1, OperationKey: skillruntime.OperationKey(ex.ID, "resolve"),
		LeaseEpoch: 1, Sequence: 1, Input: map[string]any{"query": "Payment"},
	})
	if err != nil {
		t.Fatal(err)
	}

	claimed, ok, err := repo.ClaimExecutionLease(ctx, tenant, execID, "worker-b", now.Add(time.Minute))
	if err != nil || !ok {
		t.Fatalf("claim ok=%v err=%v", ok, err)
	}
	if claimed.LeaseEpoch != 2 {
		t.Fatalf("epoch=%d want 2", claimed.LeaseEpoch)
	}

	// Stale worker A (epoch=1) must be rejected on step + execution writes.
	st.Status = skill.StepSucceeded
	_, ok, err = repo.UpdateStepFenced(ctx, st, 1)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("stale UpdateStepFenced must reject")
	}
	staleEx := claimed
	staleEx.Status = skill.ExecSucceeded
	_, ok, err = repo.UpdateExecutionFenced(ctx, staleEx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("stale UpdateExecutionFenced must reject")
	}

	out, steps, err := rt.Recover(ctx, claimed.TenantID, claimed.ID, spec)
	if err != nil {
		t.Fatal(err)
	}
	if out.Status != skill.ExecSucceeded {
		t.Fatalf("status=%s code=%s msg=%s", out.Status, out.ErrorCode, out.ErrorMessage)
	}
	found := false
	for _, s := range steps {
		if s.StepID == "resolve" && s.Attempt == 2 && s.Status == skill.StepSucceeded {
			found = true
			if s.OperationKey != skillruntime.OperationKey(execID, "resolve") {
				t.Fatalf("opkey=%q", s.OperationKey)
			}
		}
	}
	if !found {
		t.Fatalf("expected attempt=2 success after postgres recovery, steps=%+v", steps)
	}

	got, err := repo.GetExecution(ctx, tenant, execID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != skill.ExecSucceeded {
		t.Fatalf("status=%s", got.Status)
	}
	got.Status = skill.ExecFailed
	got.ErrorCode = "FORCE_TERMINAL"
	if _, err := repo.UpdateExecution(ctx, got); err != nil {
		t.Fatal(err)
	}
	st.Status = skill.StepSucceeded
	_, ok, err = repo.UpdateStepFenced(ctx, st, got.LeaseEpoch)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("terminal execution must reject step fencing")
	}
}

func TestPostgresPendingResumeNonIdempotent(t *testing.T) {
	db := openTestDB(t)
	repo := postgres.NewSkillExecutionRepository(db)
	ctx := context.Background()
	now := time.Now().UTC()
	until := now.Add(-time.Second)
	tenant := "pg_pend_" + uuid.NewString()[:8]
	execID := "ex_" + uuid.NewString()

	tools := toolregistry.Default()
	router := toolprovider.NewRouter([]toolprovider.Provider{&simulator.JiraProvider{Registry: tools}}, tools, nil)
	_ = router.SyncRegistry(ctx)
	rt := &skillruntime.Runtime{Tools: tools, Exec: router, Preview: router, Store: repo, LeaseOwner: "recovery"}

	specYAML := `
name: create
inputs:
  project_name: {type: string, required: true}
  title: {type: string, required: true}
steps:
  - id: create
    tool: jira.create_issue
    args: {project: "{{ inputs.project_name }}", title: "{{ inputs.title }}"}
risk: {level: LOW}
max_steps: 1
`
	spec, err := skill.ParseYAML(specYAML)
	if err != nil {
		t.Fatal(err)
	}
	_, err = repo.CreateExecution(ctx, skill.Execution{
		ID: execID, TenantID: tenant, SkillID: "s", SkillVersionID: "v",
		Mode: skill.ModeLive, Status: skill.ExecRunning, LeaseEpoch: 1,
		StepCursor: 0, LeaseUntil: &until, LeaseOwner: "dead",
		Inputs: map[string]any{"project_name": "PAY", "title": "hello"}, SpecSnapshot: specYAML,
		StartedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = repo.CreateStep(ctx, skill.StepExecution{
		ID: "st_" + uuid.NewString(), ExecutionID: execID, TenantID: tenant,
		StepID: "create", Tool: "jira.create_issue", Status: skill.StepPending, Attempt: 1,
		OperationKey: skillruntime.OperationKey(execID, "create"), LeaseEpoch: 1, Sequence: 1,
		Input: map[string]any{"project": "PAY", "title": "hello"},
	})
	if err != nil {
		t.Fatal(err)
	}
	claimed, ok, err := repo.ClaimExecutionLease(ctx, tenant, execID, "recovery", now.Add(time.Minute))
	if err != nil || !ok {
		t.Fatalf("claim ok=%v err=%v", ok, err)
	}
	out, _, err := rt.Recover(ctx, claimed.TenantID, claimed.ID, spec)
	if err != nil {
		t.Fatal(err)
	}
	if out.Status != skill.ExecSucceeded {
		t.Fatalf("PENDING+NONE should resume safely, got %s (%s)", out.Status, out.ErrorMessage)
	}
}
