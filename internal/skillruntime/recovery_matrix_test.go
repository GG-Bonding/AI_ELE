package skillruntime_test

import (
	"context"
	"testing"
	"time"

	"github.com/agent-experience-engine/agent-experience-engine/internal/skill"
	"github.com/agent-experience-engine/agent-experience-engine/internal/skillruntime"
	"github.com/agent-experience-engine/agent-experience-engine/internal/toolprovider"
	"github.com/agent-experience-engine/agent-experience-engine/internal/toolprovider/simulator"
	"github.com/agent-experience-engine/agent-experience-engine/internal/toolregistry"
)

type reconcileStub struct {
	inner toolprovider.Provider
	state toolprovider.ReconcileState
}

func (s *reconcileStub) Name() string { return "reconcile-stub" }
func (s *reconcileStub) ListTools(ctx context.Context) ([]toolregistry.Definition, error) {
	return s.inner.ListTools(ctx)
}
func (s *reconcileStub) Execute(ctx context.Context, call toolprovider.Call) (toolprovider.Result, error) {
	return s.inner.Execute(ctx, call)
}
func (s *reconcileStub) Preview(ctx context.Context, call toolprovider.Call) (toolprovider.Result, error) {
	return s.inner.Preview(ctx, call)
}
func (s *reconcileStub) Reconcile(context.Context, toolprovider.Call) (toolprovider.ReconcileResult, error) {
	return toolprovider.ReconcileResult{State: s.state, Output: map[string]any{"reconciled": true}}, nil
}

func TestRecoveryMatrix(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	now := time.Now().UTC()
	until := now.Add(-time.Second)

	type tc struct {
		name       string
		stepStatus skill.StepStatus
		tool       string
		cap        toolregistry.IdempotencyCapability
		reconcile  toolprovider.ReconcileState
		wantStatus skill.ExecutionStatus
		wantAtt2   bool
	}
	cases := []tc{
		{
			name: "pending_none_retries_safely", stepStatus: skill.StepPending,
			tool: "jira.create_issue", cap: toolregistry.IdempotencyNone,
			wantStatus: skill.ExecSucceeded,
		},
		{
			name: "running_none_needs_manual", stepStatus: skill.StepRunning,
			tool: "jira.create_issue", cap: toolregistry.IdempotencyNone,
			wantStatus: skill.ExecNeedsReconciliation,
		},
		{
			name: "running_native_retries", stepStatus: skill.StepRunning,
			tool: "jira.search_projects", cap: toolregistry.IdempotencyNative,
			wantStatus: skill.ExecSucceeded, wantAtt2: true,
		},
		{
			name: "failed_exhausted_is_failed", stepStatus: skill.StepFailed,
			tool: "jira.create_issue", cap: toolregistry.IdempotencyNone,
			wantStatus: skill.ExecFailed,
		},
	}

	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			tools := toolregistry.Default()
			if def, ok := tools.Get(c.tool); ok {
				def.IdempotencyCapability = c.cap
				_ = tools.Register(def)
			}
			store := skillruntime.NewMemoryExecutionStore()
			router := toolprovider.NewRouter([]toolprovider.Provider{&simulator.JiraProvider{Registry: tools}}, tools, nil)
			_ = router.SyncRegistry(ctx)
			rt := &skillruntime.Runtime{Tools: tools, Exec: router, Preview: router, Store: store, LeaseOwner: "matrix"}

			var specYAML string
			var inputs map[string]any
			if c.tool == "jira.search_projects" {
				specYAML = `
name: search
inputs: {project_name: {type: string, required: true}}
steps:
  - id: s1
    tool: jira.search_projects
    args: {query: "{{ inputs.project_name }}"}
risk: {level: LOW}
max_steps: 1
`
				inputs = map[string]any{"project_name": "Payment"}
			} else {
				specYAML = `
name: create
inputs:
  project_name: {type: string, required: true}
  title: {type: string, required: true}
steps:
  - id: s1
    tool: jira.create_issue
    args: {project: "{{ inputs.project_name }}", title: "{{ inputs.title }}"}
risk: {level: LOW}
max_steps: 1
`
				inputs = map[string]any{"project_name": "PAY", "title": "x"}
			}
			spec, err := skill.ParseYAML(specYAML)
			if err != nil {
				t.Fatal(err)
			}
			ex, err := store.CreateExecution(ctx, skill.Execution{
				ID: "m-" + c.name, TenantID: "t", SkillID: "s", SkillVersionID: "v",
				Mode: skill.ModeLive, Status: skill.ExecRunning, LeaseEpoch: 1,
				StepCursor: 0, LeaseUntil: &until, LeaseOwner: "dead",
				Inputs: inputs, SpecSnapshot: specYAML,
			})
			if err != nil {
				t.Fatal(err)
			}
			_, err = store.CreateStep(ctx, skill.StepExecution{
				ID: "att1", ExecutionID: ex.ID, TenantID: "t", StepID: "s1",
				Tool: c.tool, Status: c.stepStatus, Attempt: 1,
				OperationKey: skillruntime.OperationKey(ex.ID, "s1"),
				LeaseEpoch: 1, Sequence: 1, Input: map[string]any{"project": "PAY", "title": "x", "query": "Payment"},
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
			if out.Status != c.wantStatus {
				t.Fatalf("status=%s want=%s reason=%s/%s", out.Status, c.wantStatus, out.ErrorCode, out.ErrorMessage)
			}
			if c.wantAtt2 {
				found := false
				for _, st := range steps {
					if st.Attempt == 2 && st.Status == skill.StepSucceeded {
						found = true
					}
				}
				if !found {
					t.Fatalf("expected attempt=2 success, steps=%+v", steps)
				}
			}
		})
	}
}

func TestRecoveryMatrixReconcileApplied(t *testing.T) {
	t.Parallel()
	runReconcileMatrix(t, toolprovider.ReconcileApplied, skill.ExecSucceeded, false)
}

func TestRecoveryMatrixReconcileNotApplied(t *testing.T) {
	t.Parallel()
	runReconcileMatrix(t, toolprovider.ReconcileNotApplied, skill.ExecSucceeded, true)
}

func TestRecoveryMatrixReconcileUnknown(t *testing.T) {
	t.Parallel()
	runReconcileMatrix(t, toolprovider.ReconcileUnknown, skill.ExecNeedsReconciliation, false)
}

func runReconcileMatrix(t *testing.T, state toolprovider.ReconcileState, want skill.ExecutionStatus, wantAttempt2 bool) {
	t.Helper()
	ctx := context.Background()
	tools := toolregistry.Default()
	def, _ := tools.Get("jira.create_issue")
	def.IdempotencyCapability = toolregistry.IdempotencyQueryReconcile
	_ = tools.Register(def)

	store := skillruntime.NewMemoryExecutionStore()
	stub := &reconcileStub{inner: &simulator.JiraProvider{Registry: tools}, state: state}
	router := toolprovider.NewRouter([]toolprovider.Provider{stub}, tools, nil)
	_ = router.SyncRegistry(ctx)
	rt := &skillruntime.Runtime{Tools: tools, Exec: router, Preview: router, Store: store, LeaseOwner: "matrix"}

	specYAML := `
name: create
inputs:
  project_name: {type: string, required: true}
  title: {type: string, required: true}
steps:
  - id: s1
    tool: jira.create_issue
    args: {project: "{{ inputs.project_name }}", title: "{{ inputs.title }}"}
risk: {level: LOW}
max_steps: 1
`
	spec, _ := skill.ParseYAML(specYAML)
	now := time.Now().UTC()
	until := now.Add(-time.Second)
	exID := "m-rec-" + string(state)
	ex, _ := store.CreateExecution(ctx, skill.Execution{
		ID: exID, TenantID: "t", SkillID: "s", SkillVersionID: "v",
		Mode: skill.ModeLive, Status: skill.ExecRunning, LeaseEpoch: 1,
		StepCursor: 0, LeaseUntil: &until, Inputs: map[string]any{"project_name": "PAY", "title": "x"},
		SpecSnapshot: specYAML,
	})
	_, _ = store.CreateStep(ctx, skill.StepExecution{
		ID: "att1", ExecutionID: ex.ID, TenantID: "t", StepID: "s1",
		Tool: "jira.create_issue", Status: skill.StepRunning, Attempt: 1,
		OperationKey: skillruntime.OperationKey(ex.ID, "s1"), LeaseEpoch: 1, Sequence: 1,
		Input: map[string]any{"project": "PAY", "title": "x"},
	})
	claimed, ok, err := store.ClaimExecutionLease(ctx, "t", ex.ID, "recovery", now.Add(time.Minute))
	if err != nil || !ok {
		t.Fatalf("claim ok=%v err=%v", ok, err)
	}
	out, steps, err := rt.Recover(ctx, claimed.TenantID, claimed.ID, spec)
	if err != nil {
		t.Fatal(err)
	}
	if out.Status != want {
		t.Fatalf("status=%s want=%s", out.Status, want)
	}
	if wantAttempt2 {
		found := false
		for _, st := range steps {
			if st.Attempt == 2 && st.Status == skill.StepSucceeded &&
				st.OperationKey == skillruntime.OperationKey(ex.ID, "s1") {
				found = true
			}
		}
		if !found {
			t.Fatalf("NOT_APPLIED should execute attempt=2 same op key, steps=%+v", steps)
		}
	}
	if state == toolprovider.ReconcileUnknown {
		for _, st := range steps {
			if st.Attempt > 1 && st.Status == skill.StepSucceeded {
				t.Fatal("UNKNOWN must not auto-execute a new attempt")
			}
		}
	}
}

func TestTerminalExecutionRejectsStepFencing(t *testing.T) {
	t.Parallel()
	store := skillruntime.NewMemoryExecutionStore()
	ctx := context.Background()
	ex, err := store.CreateExecution(ctx, skill.Execution{
		ID: "term1", TenantID: "t", SkillID: "s", SkillVersionID: "v",
		Mode: skill.ModeLive, Status: skill.ExecFailed, LeaseEpoch: 5,
	})
	if err != nil {
		t.Fatal(err)
	}
	st, err := store.CreateStep(ctx, skill.StepExecution{
		ID: "st1", ExecutionID: ex.ID, TenantID: "t", StepID: "s1",
		Status: skill.StepFailed, Attempt: 1, Sequence: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	st.Status = skill.StepSucceeded
	_, ok, err := store.UpdateStepFenced(ctx, st, 5)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("terminal execution must reject step updates")
	}
}
