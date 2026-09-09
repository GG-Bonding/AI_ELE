package eval

import (
	"context"

	"github.com/agent-experience-engine/agent-experience-engine/internal/eval/jirasim"
	"github.com/agent-experience-engine/agent-experience-engine/internal/skill"
	"github.com/agent-experience-engine/agent-experience-engine/internal/skillruntime"
	"github.com/agent-experience-engine/agent-experience-engine/internal/skillvalidator"
	"github.com/agent-experience-engine/agent-experience-engine/internal/toolregistry"
)

// ReplayJiraCounterfactual runs offline with-skill vs pattern-tip arms on strict Jira sim.
func ReplayJiraCounterfactual(ctx context.Context, skillYAML string) (skill.CounterfactualResult, error) {
	tools := toolregistry.Default()
	repo := skill.NewMemoryRepository()
	reg := &skill.RegistryService{
		Repo:      repo,
		Validator: skillvalidator.Adapt(skillvalidator.New(tools, skillvalidator.Options{TenantID: "cf"})),
	}
	sk, ver, rep, err := reg.CompileAndCreate(ctx, "cf", "jira_safe", "", "", skillYAML, 0.9, 0.8)
	if err != nil || !rep.OK {
		return skill.CounterfactualResult{}, err
	}
	_, _ = reg.MoveToShadow(ctx, "cf", ver.ID)
	for i := 0; i < 5; i++ {
		_, _ = reg.RecordShadowOutcome(ctx, "cf", ver.ID, true)
	}
	ver, err = reg.Activate(ctx, "cf", ver.ID, skill.DefaultPromoteConfig())
	if err != nil {
		return skill.CounterfactualResult{}, err
	}

	store := skillruntime.NewMemoryExecutionStore()
	jira := &skillruntime.JiraSimExecutor{Sim: jirasim.New().WithMode(jirasim.ModeStrict), Registry: tools}
	rt := &skillruntime.Runtime{Tools: tools, Exec: jira, Preview: jira, Store: store, Policy: skillruntime.DefaultPolicy{}}
	execSvc := &skill.ExecutionService{Repo: repo, Store: store, Runner: rt, Registry: reg}

	ex, steps, err := execSvc.Execute(ctx, skill.ExecuteInput{
		TenantID: "cf", SkillID: sk.ID, VersionID: ver.ID, Mode: skill.ModeLive,
		Inputs:         map[string]any{"project_name": "Payment", "title": "timeout"},
		AvailableTools: []string{"jira.search_projects", "jira.create_issue"},
		RuntimeEnabled: true,
		IdempotencyKey: "cf-live",
	})
	if err != nil {
		return skill.CounterfactualResult{}, err
	}
	withArm := skill.ReplayArm{Success: ex.Status == skill.ExecSucceeded, Steps: len(steps)}
	if withArm.Success {
		withArm.Reward = 1
	}

	sim := jirasim.New().WithMode(jirasim.ModeStrict)
	ok, tipSteps := sim.Run(jirasim.AgentPolicy{}.Plan("Create a Jira issue for payment timeout", []string{
		"Always use display name Payment as the project field",
	}))
	without := skill.ReplayArm{Success: ok, Steps: len(tipSteps)}
	if ok {
		without.Reward = 1
	}
	return skill.CompareReplayArms(withArm, without), nil
}
