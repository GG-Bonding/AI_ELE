package eval

import (
	"context"
	"fmt"

	"github.com/agent-experience-engine/agent-experience-engine/internal/eval/jirasim"
	"github.com/agent-experience-engine/agent-experience-engine/internal/skill"
	"github.com/agent-experience-engine/agent-experience-engine/internal/skillruntime"
	"github.com/agent-experience-engine/agent-experience-engine/internal/skillvalidator"
	"github.com/agent-experience-engine/agent-experience-engine/internal/toolregistry"
)

const skillBenchYAML = `
name: jira_safe_create_issue
description: Resolve project key then create issue
inputs:
  project_name:
    type: string
    required: true
  title:
    type: string
    required: true
steps:
  - id: resolve_project
    tool: jira.search_projects
    args:
      query: "{{ inputs.project_name }}"
    save_as: project
  - id: create_issue
    tool: jira.create_issue
    args:
      project: "{{ project.key }}"
      title: "{{ inputs.title }}"
risk:
  level: LOW
max_steps: 5
idempotent: true
`

// SkillBenchmarkMetrics compares Pattern-only vs Skill-enabled agents (V3-10).
type SkillBenchmarkMetrics struct {
	PatternOnlySuccess float64 `json:"pattern_only_success"`
	SkillSuccess       float64 `json:"skill_success"`
	PatternOnlySteps   float64 `json:"pattern_only_avg_steps"`
	SkillSteps         float64 `json:"skill_avg_steps"`
	UnsafeSkillRate    float64 `json:"unsafe_skill_rate"`
	SkillSelectionHit  float64 `json:"skill_selection_hit"`
	ACESteps           float64 `json:"ace_steps"`
	ShadowOK           bool    `json:"shadow_ok"`
	Activated          bool    `json:"activated"`
}

// RunSkillBenchmark proves Skill-enabled execution beats naive display-name baseline.
func RunSkillBenchmark(ctx context.Context) (SkillBenchmarkMetrics, error) {
	tools := toolregistry.Default()
	repo := skill.NewMemoryRepository()
	reg := &skill.RegistryService{
		Repo:      repo,
		Validator: skillvalidator.Adapt(skillvalidator.New(tools, skillvalidator.Options{TenantID: "bench"})),
	}

	sk, ver, rep, err := reg.CompileAndCreate(ctx, "bench", "jira_safe_create_issue",
		"safe jira create", "pat_bench", skillBenchYAML, 0.9, 0.85)
	if err != nil {
		return SkillBenchmarkMetrics{}, err
	}
	if !rep.OK {
		return SkillBenchmarkMetrics{}, fmt.Errorf("validation failed")
	}
	ver, err = reg.MoveToShadow(ctx, "bench", ver.ID)
	if err != nil {
		return SkillBenchmarkMetrics{}, err
	}

	store := skillruntime.NewMemoryExecutionStore()
	rt := &skillruntime.Runtime{
		Tools: tools,
		Exec: &skillruntime.JiraSimExecutor{
			Sim: jirasim.New().WithMode(jirasim.ModeStrict), Registry: tools,
		},
		Policy: skillruntime.DefaultPolicy{},
		Store:  store,
	}

	inputs := map[string]any{"project_name": "Payment", "title": "payment timeout"}
	available := []string{"jira.search_projects", "jira.create_issue"}

	for i := 0; i < 5; i++ {
		ex, _, err := rt.Run(ctx, skillruntime.RunRequest{
			TenantID: "bench", SkillID: sk.ID, SkillVersionID: ver.ID,
			Mode: skill.ModeShadow, Spec: ver.Spec, Inputs: inputs,
			AvailableTools: available, RuntimeEnabled: true,
			IdempotencyKey: fmt.Sprintf("shadow-%d", i),
		})
		if err != nil {
			return SkillBenchmarkMetrics{}, err
		}
		if _, err := reg.RecordShadowOutcome(ctx, "bench", ver.ID, ex.Status == skill.ExecSucceeded); err != nil {
			return SkillBenchmarkMetrics{}, err
		}
	}
	ver, err = reg.Activate(ctx, "bench", ver.ID, skill.DefaultPromoteConfig())
	if err != nil {
		return SkillBenchmarkMetrics{}, err
	}
	sk, err = repo.GetSkill(ctx, "bench", sk.ID)
	if err != nil {
		return SkillBenchmarkMetrics{}, err
	}

	const trials = 10
	var patternOK, skillOK, patternSteps, skillSteps, unsafe, selectHit int

	for i := 0; i < trials; i++ {
		sim := jirasim.New().WithMode(jirasim.ModeStrict)
		ok, steps := sim.Run(jirasim.AgentPolicy{}.Plan("Create a Jira issue for payment timeout", []string{
			"You must always use display name Payment as the project field for create_issue",
		}))
		if ok {
			patternOK++
		}
		patternSteps += len(steps)

		ranked, err := skill.Retrieve(ctx, repo, tools, skill.RetrieveQuery{
			TenantID: "bench", Task: "Create a Jira issue for payment timeout",
			Tools: available, TopK: 3,
		})
		if err != nil {
			return SkillBenchmarkMetrics{}, err
		}
		if len(ranked) > 0 && ranked[0].Skill.ID == sk.ID {
			selectHit++
		}
		ex, stepRuns, err := rt.Run(ctx, skillruntime.RunRequest{
			TenantID: "bench", SkillID: sk.ID, SkillVersionID: ver.ID,
			Mode: skill.ModeLive, Spec: ver.Spec, Inputs: inputs,
			AvailableTools: available, RuntimeEnabled: true,
			IdempotencyKey: fmt.Sprintf("live-%d", i),
		})
		if err != nil {
			return SkillBenchmarkMetrics{}, err
		}
		skillSteps += len(stepRuns)
		if ex.Status == skill.ExecSucceeded {
			skillOK++
		}
	}

	denyEx, _, err := rt.Run(ctx, skillruntime.RunRequest{
		TenantID: "bench", SkillID: sk.ID, SkillVersionID: ver.ID,
		Mode: skill.ModeLive,
		Spec: skill.Spec{
			Name: "delete", Risk: skill.SkillRisk{Level: skill.RiskCritical},
			Steps: []skill.SkillStep{{
				ID: "d", Tool: "jira.delete_issue",
				Args: map[string]any{"issue_key": "PAY-1"},
			}},
			MaxSteps: 3, TimeoutMs: 5000,
		},
		Inputs: map[string]any{}, AvailableTools: []string{"jira.delete_issue"},
		RuntimeEnabled: true, IdempotencyKey: "crit-probe",
	})
	if err != nil {
		return SkillBenchmarkMetrics{}, err
	}
	if denyEx.Status != skill.ExecDenied && denyEx.Status != skill.ExecFailed {
		unsafe++
	}

	avgSkillSteps := float64(skillSteps) / trials
	avgPatternSteps := float64(patternSteps) / trials
	return SkillBenchmarkMetrics{
		PatternOnlySuccess: float64(patternOK) / trials,
		SkillSuccess:       float64(skillOK) / trials,
		PatternOnlySteps:   avgPatternSteps,
		SkillSteps:         avgSkillSteps,
		UnsafeSkillRate:    float64(unsafe),
		SkillSelectionHit:  float64(selectHit) / trials,
		ACESteps:           skill.OfflineCompare(int(avgSkillSteps), int(avgPatternSteps)).ACE,
		ShadowOK:           true,
		Activated:          sk.Status == skill.StatusActive,
	}, nil
}
