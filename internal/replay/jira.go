package replay

import (
	"context"

	"github.com/agent-experience-engine/agent-experience-engine/internal/eval/jirasim"
	"github.com/agent-experience-engine/agent-experience-engine/internal/skill"
	"github.com/agent-experience-engine/agent-experience-engine/internal/skillruntime"
	"github.com/agent-experience-engine/agent-experience-engine/internal/skillvalidator"
	"github.com/agent-experience-engine/agent-experience-engine/internal/toolprovider"
	"github.com/agent-experience-engine/agent-experience-engine/internal/toolprovider/simulator"
	"github.com/agent-experience-engine/agent-experience-engine/internal/toolregistry"
)

// JiraEnvironment replays against the Jira simulator via ToolProvider (not raw executor).
type JiraEnvironment struct {
	Mode jirasim.Mode
}

func (e JiraEnvironment) Name() string { return "jira" }

func (e JiraEnvironment) ReplayWithSkill(ctx context.Context, task Task, skillYAML string) (Result, error) {
	tools := toolregistry.Default()
	sim := jirasim.New()
	if e.Mode != "" {
		sim = sim.WithMode(e.Mode)
	}
	prov := &simulator.JiraProvider{Sim: sim, Registry: tools}
	router := toolprovider.NewRouter([]toolprovider.Provider{prov}, tools, nil)
	if err := router.SyncRegistry(ctx); err != nil {
		return Result{}, err
	}
	repo := skill.NewMemoryRepository()
	reg := &skill.RegistryService{
		Repo: repo, Validator: skillvalidator.Adapt(skillvalidator.New(tools, skillvalidator.Options{TenantID: "replay"})),
	}
	sk, ver, rep, err := reg.CompileAndCreate(ctx, "replay", "skill", "", "", skillYAML, 0.9, 0.8)
	if err != nil || !rep.OK {
		return Result{}, err
	}
	_, _ = reg.MoveToShadow(ctx, "replay", ver.ID)
	for i := 0; i < 5; i++ {
		_, _ = reg.RecordShadowOutcome(ctx, "replay", ver.ID, true)
	}
	ver, err = reg.Activate(ctx, "replay", ver.ID, skill.DefaultPromoteConfig())
	if err != nil {
		return Result{}, err
	}
	store := skillruntime.NewMemoryExecutionStore()
	rt := &skillruntime.Runtime{Tools: tools, Exec: router, Preview: router, Store: store, Policy: skillruntime.DefaultPolicy{}}
	execSvc := &skill.ExecutionService{Repo: repo, Store: store, Runner: rt, Registry: reg}
	inputs := task.Inputs
	if inputs == nil {
		inputs = map[string]any{"project_name": "Payment", "title": task.Prompt}
	}
	avail := task.Tools
	if len(avail) == 0 {
		avail = []string{"jira.search_projects", "jira.create_issue"}
	}
	ex, steps, err := execSvc.Execute(ctx, skill.ExecuteInput{
		TenantID: "replay", SkillID: sk.ID, VersionID: ver.ID, Mode: skill.ModeLive,
		Inputs: inputs, AvailableTools: avail, RuntimeEnabled: true, IdempotencyKey: "replay-" + task.ID,
	})
	if err != nil {
		return Result{}, err
	}
	ok := ex.Status == skill.ExecSucceeded
	reward := 0.0
	if ok {
		reward = 1
	}
	return Result{Success: ok, Steps: len(steps), Reward: reward}, nil
}

func (e JiraEnvironment) ReplayWithoutSkill(ctx context.Context, task Task, patternTips []string) (Result, error) {
	_ = ctx
	sim := jirasim.New()
	if e.Mode != "" {
		sim = sim.WithMode(e.Mode)
	}
	prompt := task.Prompt
	if prompt == "" {
		prompt = "Create a Jira issue for payment timeout"
	}
	ok, tipSteps := sim.Run(jirasim.AgentPolicy{}.Plan(prompt, patternTips))
	reward := 0.0
	if ok {
		reward = 1
	}
	return Result{Success: ok, Steps: len(tipSteps), Reward: reward}, nil
}
