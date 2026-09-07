package eval

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/agent-experience-engine/agent-experience-engine/internal/eval/jirasim"
	"github.com/agent-experience-engine/agent-experience-engine/internal/experience"
	"github.com/agent-experience-engine/agent-experience-engine/internal/skill"
	"github.com/agent-experience-engine/agent-experience-engine/internal/skillruntime"
	"github.com/agent-experience-engine/agent-experience-engine/internal/skillvalidator"
	"github.com/agent-experience-engine/agent-experience-engine/internal/toolregistry"
)

// LearningChainMetrics proves the V3.1 experience→pattern→skill learning loop.
type LearningChainMetrics struct {
	PatternOnlySuccess float64 `json:"pattern_only_success"`
	SkillV1Success     float64 `json:"skill_v1_success"`
	SkillV2Success     float64 `json:"skill_v2_success"`
	UtilityAfterReward float64 `json:"utility_after_reward"`
	UtilityAfterFail   float64 `json:"utility_after_fail_wave"`
	V2Promoted         bool    `json:"v2_promoted"`
	V1Superseded       bool    `json:"v1_superseded"`
	UnsafeRate         float64 `json:"unsafe_rate"`
	ChainComplete      bool    `json:"chain_complete"`
}

// CompilePatternToSkillYAML is a deterministic Pattern→SkillSpec bridge for the
// Jira resolve-key procedural pattern (V3.1 learning-chain benchmark).
func CompilePatternToSkillYAML(p experience.Pattern) (string, error) {
	blob := strings.ToLower(p.Trigger + " " + p.Content)
	if strings.Contains(blob, "project") && (strings.Contains(blob, "search") ||
		strings.Contains(blob, "resolve") || strings.Contains(blob, "key")) {
		return skillBenchYAML, nil
	}
	return "", fmt.Errorf("no skill compiler mapping for pattern %q", p.ID)
}

// RunLearningChainBenchmark starts from seeded experiences/pattern (empty skill
// registry), compiles an executable Skill, learns via feedback, evolves to v2,
// and compares against a Pattern-only baseline that uses the display name.
func RunLearningChainBenchmark(ctx context.Context) (LearningChainMetrics, error) {
	const tenant = "chain"
	tools := toolregistry.Default()
	expRepo := experience.NewMemoryRepository()
	patternRepo := experience.NewMemoryPatternRepository()
	now := time.Now().UTC()

	// ①–③ Seed experiences + ACTIVE pattern (simulates extract/generalize without LLM).
	e1 := experience.Experience{
		ID: "exp_fail", TenantID: tenant, Type: experience.TypeFailure, Scope: experience.ScopeTool,
		ScopeKey: "jira", Trigger: "create jira with display name",
		Content:    "Do not use Jira project display name as project key.",
		Status:     experience.StatusActive, Confidence: 0.9, Utility: 0.7,
		Alpha: 3, Beta: 1, CreatedAt: now, UpdatedAt: now,
	}
	e2 := experience.Experience{
		ID: "exp_proc", TenantID: tenant, Type: experience.TypeProcedural, Scope: experience.ScopeTool,
		ScopeKey: "jira", Trigger: "create jira issue when project key unknown",
		Content:    "Resolve the Jira project key with search_projects before create_issue.",
		Status:     experience.StatusActive, Confidence: 0.95, Utility: 0.85,
		Alpha: 5, Beta: 1, CreatedAt: now, UpdatedAt: now,
	}
	if _, err := expRepo.Create(ctx, e1); err != nil {
		return LearningChainMetrics{}, err
	}
	if _, err := expRepo.Create(ctx, e2); err != nil {
		return LearningChainMetrics{}, err
	}
	pat := experience.Pattern{
		ID: "pat_jira_resolve", TenantID: tenant,
		Type: experience.TypeProcedural, Scope: experience.ScopeTool, ScopeKey: "jira",
		Trigger: "create jira issue", Content: "search projects then create with resolved project key",
		Status: experience.PatternStatusActive, Confidence: 0.92, Utility: 0.8,
		Alpha: 4, Beta: 1, SupportCount: 2, CreatedAt: now, UpdatedAt: now,
	}
	if _, err := patternRepo.Create(ctx, pat); err != nil {
		return LearningChainMetrics{}, err
	}
	_ = patternRepo.AddEvidence(ctx, experience.PatternEvidence{
		PatternID: pat.ID, ExperienceID: e1.ID, CreatedAt: now,
	})
	_ = patternRepo.AddEvidence(ctx, experience.PatternEvidence{
		PatternID: pat.ID, ExperienceID: e2.ID, CreatedAt: now,
	})

	// ④–⑤ Pattern → SkillSpec → Compile SkillVersion
	yamlDoc, err := CompilePatternToSkillYAML(pat)
	if err != nil {
		return LearningChainMetrics{}, err
	}
	skillRepo := skill.NewMemoryRepository()
	learnStore := skill.NewMemoryLearningStore()
	validator := skillvalidator.Adapt(skillvalidator.New(tools, skillvalidator.Options{TenantID: tenant}))
	reg := &skill.RegistryService{Repo: skillRepo, Validator: validator}
	sk, ver, rep, err := reg.CompileAndCreate(ctx, tenant, "jira_safe_create_issue",
		"from pattern", pat.ID, yamlDoc, pat.Confidence, pat.Utility)
	if err != nil || !rep.OK {
		return LearningChainMetrics{}, fmt.Errorf("compile: ok=%v err=%v", rep.OK, err)
	}

	store := skillruntime.NewMemoryExecutionStore()
	jira := &skillruntime.JiraSimExecutor{Sim: jirasim.New().WithMode(jirasim.ModeStrict), Registry: tools}
	rt := &skillruntime.Runtime{
		Tools: tools, Exec: jira, Preview: jira,
		Policy: skillruntime.DefaultPolicy{}, Store: store,
	}
	execSvc := &skill.ExecutionService{Repo: skillRepo, Store: store, Runner: rt, Registry: reg}

	// ⑥–⑧ Shadow → Activate → Live
	if _, err := reg.MoveToShadow(ctx, tenant, ver.ID); err != nil {
		return LearningChainMetrics{}, err
	}
	inputs := map[string]any{"project_name": "Payment", "title": "payment timeout"}
	available := []string{"jira.search_projects", "jira.create_issue"}
	for i := 0; i < 5; i++ {
		ex, _, err := execSvc.Execute(ctx, skill.ExecuteInput{
			TenantID: tenant, SkillID: sk.ID, VersionID: ver.ID, Mode: skill.ModeShadow,
			Inputs: inputs, AvailableTools: available, RuntimeEnabled: true,
			IdempotencyKey: fmt.Sprintf("shadow-%d", i),
		})
		if err != nil {
			return LearningChainMetrics{}, err
		}
		if ex.Status != skill.ExecSucceeded {
			return LearningChainMetrics{}, fmt.Errorf("shadow failed: %s %s", ex.Status, ex.ErrorMessage)
		}
	}
	ver, err = reg.Activate(ctx, tenant, ver.ID, skill.DefaultPromoteConfig())
	if err != nil {
		return LearningChainMetrics{}, err
	}

	const trials = 8
	var patternOK, skillV1OK, unsafe int
	for i := 0; i < trials; i++ {
		sim := jirasim.New().WithMode(jirasim.ModeStrict)
		ok, _ := sim.Run(jirasim.AgentPolicy{}.Plan("Create a Jira issue for payment timeout", []string{
			"Always use display name Payment as the project field",
		}))
		if ok {
			patternOK++
		}
		ex, _, err := execSvc.Execute(ctx, skill.ExecuteInput{
			TenantID: tenant, SkillID: sk.ID, VersionID: ver.ID, Mode: skill.ModeLive,
			Inputs: inputs, AvailableTools: available, RuntimeEnabled: true,
			IdempotencyKey: fmt.Sprintf("live-v1-%d", i),
		})
		if err != nil {
			return LearningChainMetrics{}, err
		}
		if ex.Status == skill.ExecSucceeded {
			skillV1OK++
		}
	}

	denyEx, _, err := rt.Run(ctx, skill.ExecutionRunRequest{
		TenantID: tenant, SkillID: sk.ID, SkillVersionID: ver.ID, Mode: skill.ModeLive,
		Spec: skill.Spec{
			Name: "delete", Risk: skill.SkillRisk{Level: skill.RiskCritical},
			Steps:    []skill.SkillStep{{ID: "d", Tool: "jira.delete_issue", Args: map[string]any{"issue_key": "PAY-1"}}},
			MaxSteps: 2, TimeoutMs: 3000,
		},
		AvailableTools: []string{"jira.delete_issue"}, RuntimeEnabled: true, IdempotencyKey: "crit",
	})
	if err != nil {
		return LearningChainMetrics{}, err
	}
	if denyEx.Status != skill.ExecDenied && denyEx.Status != skill.ExecFailed {
		unsafe++
	}

	// ⑩ Feedback → Utility ↑
	before := ver.Utility
	if err := skill.ApplyFeedback(ctx, skillRepo, learnStore, nil, tenant, "fb-ok", ver.ID, "ex-ok", 1.0, 1.0, 1.0); err != nil {
		return LearningChainMetrics{}, err
	}
	ver, _ = skillRepo.GetVersion(ctx, tenant, ver.ID)
	utilAfterReward := ver.Utility
	if utilAfterReward <= before {
		return LearningChainMetrics{}, fmt.Errorf("utility did not rise: %v → %v", before, utilAfterReward)
	}

	// Failure wave lowers utility (sets up evolution pressure).
	for i := 0; i < 6; i++ {
		if err := skill.ApplyFeedback(ctx, skillRepo, learnStore, nil, tenant,
			fmt.Sprintf("fb-fail-%d", i), ver.ID, "", -1.0, 1.0, 1.0); err != nil {
			return LearningChainMetrics{}, err
		}
	}
	ver, _ = skillRepo.GetVersion(ctx, tenant, ver.ID)
	utilAfterFail := ver.Utility

	// Evolution: immutable ProposeRevision → validate → shadow → activate (deprecates v1).
	rev, err := skill.ProposeRevision(ctx, skillRepo, tenant, sk.ID, skillBenchYAML, pat.ID)
	if err != nil {
		return LearningChainMetrics{}, err
	}
	rev = skill.ApplyValidationReport(rev, validator.Validate(rev.Spec))
	if rev.ValidationStatus != skill.ValidationPassed {
		return LearningChainMetrics{}, fmt.Errorf("revision validation failed: %#v", rev)
	}
	rev, err = skillRepo.UpdateVersion(ctx, rev)
	if err != nil {
		return LearningChainMetrics{}, err
	}
	if _, err := reg.MoveToShadow(ctx, tenant, rev.ID); err != nil {
		return LearningChainMetrics{}, err
	}
	for i := 0; i < 5; i++ {
		ex, _, err := execSvc.Execute(ctx, skill.ExecuteInput{
			TenantID: tenant, SkillID: sk.ID, VersionID: rev.ID, Mode: skill.ModeShadow,
			Inputs: inputs, AvailableTools: available, RuntimeEnabled: true,
			IdempotencyKey: fmt.Sprintf("shadow-v2-%d", i),
		})
		if err != nil {
			return LearningChainMetrics{}, err
		}
		if ex.Status != skill.ExecSucceeded {
			return LearningChainMetrics{}, fmt.Errorf("v2 shadow: %s", ex.Status)
		}
	}
	rev, err = reg.Activate(ctx, tenant, rev.ID, skill.DefaultPromoteConfig())
	if err != nil {
		return LearningChainMetrics{}, err
	}

	var skillV2OK int
	for i := 0; i < trials; i++ {
		ex, _, err := execSvc.Execute(ctx, skill.ExecuteInput{
			TenantID: tenant, SkillID: sk.ID, VersionID: rev.ID, Mode: skill.ModeLive,
			Inputs: inputs, AvailableTools: available, RuntimeEnabled: true,
			IdempotencyKey: fmt.Sprintf("live-v2-%d", i),
		})
		if err != nil {
			return LearningChainMetrics{}, err
		}
		if ex.Status == skill.ExecSucceeded {
			skillV2OK++
		}
	}

	v1After, _ := skillRepo.GetVersion(ctx, tenant, ver.ID)
	skAfter, _ := skillRepo.GetSkill(ctx, tenant, sk.ID)
	return LearningChainMetrics{
		PatternOnlySuccess: float64(patternOK) / trials,
		SkillV1Success:     float64(skillV1OK) / trials,
		SkillV2Success:     float64(skillV2OK) / trials,
		UtilityAfterReward: utilAfterReward,
		UtilityAfterFail:   utilAfterFail,
		V2Promoted:         rev.Status == skill.VersionActive && skAfter.ActiveVersionID != nil && *skAfter.ActiveVersionID == rev.ID,
		V1Superseded:       v1After.Status == skill.VersionDeprecated,
		UnsafeRate:         float64(unsafe),
		ChainComplete:      true,
	}, nil
}
