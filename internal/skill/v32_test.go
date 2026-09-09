package skill_test

import (
	"context"
	"math/rand"
	"testing"

	"github.com/agent-experience-engine/agent-experience-engine/internal/provider"
	"github.com/agent-experience-engine/agent-experience-engine/internal/skill"
	"github.com/agent-experience-engine/agent-experience-engine/internal/skillvalidator"
	"github.com/agent-experience-engine/agent-experience-engine/internal/toolregistry"
)

func TestSemanticRetrieveBeatsLexicalMismatch(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := skill.NewMemoryRepository()
	tools := toolregistry.Default()
	embedder := &provider.MockEmbedding{Dim: 8}
	svc := &skill.RegistryService{
		Repo:      repo,
		Validator: skillvalidator.Adapt(skillvalidator.New(tools, skillvalidator.Options{TenantID: "t"})),
		Embedder:  embedder,
	}
	_, ver, _, err := svc.CompileAndCreate(ctx, "t", "jira_safe_create_issue",
		"Resolve project key before creating Jira ticket", "p", jiraSafeCreateYAML, 0.95, 0.9)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = svc.MoveToShadow(ctx, "t", ver.ID)
	for i := 0; i < 5; i++ {
		_, _ = svc.RecordShadowOutcome(ctx, "t", ver.ID, true)
	}
	_, _ = svc.Activate(ctx, "t", ver.ID, skill.DefaultPromoteConfig())

	rt := &skill.Retriever{Repo: repo, Tools: tools, Embedder: embedder}
	// Task uses synonyms that lexical overlap may miss but hash-embed still correlates on shared tokens.
	ranked, err := rt.Retrieve(ctx, skill.RetrieveQuery{
		TenantID: "t",
		Task:     "safely open a jira ticket after resolving project key",
		Tools:    []string{"jira.search_projects", "jira.create_issue"},
		TopK:     3,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(ranked) != 1 || ranked[0].Score <= 0 {
		t.Fatalf("%#v", ranked)
	}
	if !ranked[0].Semantic && ranked[0].Sim <= 0 {
		t.Fatalf("expected positive sim: %#v", ranked[0])
	}
}

func TestAutoReviseInjectsSearchStep(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := skill.NewMemoryRepository()
	tools := toolregistry.Default()
	svc := &skill.RegistryService{
		Repo:      repo,
		Validator: skillvalidator.Adapt(skillvalidator.New(tools, skillvalidator.Options{TenantID: "t"})),
	}
	badYAML := `
name: jira_direct_create
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
max_steps: 3
`
	sk, ver, rep, err := svc.CompileAndCreate(ctx, "t", "jira_direct_create", "", "", badYAML, 0.8, 0.5)
	if err != nil || !rep.OK {
		t.Fatalf("compile: %#v err=%v", rep, err)
	}
	rev, ok, err := skill.AutoRevise(ctx, repo, "t", sk.ID, "pat", ver.Spec, skill.RevisionHint{
		FailureCodes:   []string{"INVALID_PROJECT_KEY"},
		FailureMessages: []string{"unknown project key Payment"},
		PatternContent: "resolve project key with search before create",
	})
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	if rev.Version <= ver.Version {
		t.Fatalf("revision version=%d", rev.Version)
	}
	hasSearch := false
	for _, st := range rev.Spec.Steps {
		if st.Tool == "jira.search_projects" {
			hasSearch = true
		}
	}
	if !hasSearch {
		t.Fatalf("expected search step in %#v", rev.Spec.Steps)
	}
}

func TestCompareShadowABPrefersHigherSuccess(t *testing.T) {
	t.Parallel()
	a := []skill.ShadowTrial{{Success: true, Steps: 3}, {Success: false, Steps: 3}, {Success: true, Steps: 3}}
	b := []skill.ShadowTrial{{Success: true, Steps: 2}, {Success: true, Steps: 2}, {Success: true, Steps: 2}}
	got := skill.CompareShadowAB("va", "vb", a, b, 3)
	if got.WinnerID != "vb" {
		t.Fatalf("%#v", got)
	}
}

func TestSelectThompsonPrefersHighAlpha(t *testing.T) {
	t.Parallel()
	ranked := []skill.RankedSkill{
		{Skill: skill.Skill{Name: "low"}, Version: skill.Version{Alpha: 1, Beta: 20}, Score: 0.5, Availability: 1, Validity: 1},
		{Skill: skill.Skill{Name: "high"}, Version: skill.Version{Alpha: 40, Beta: 1}, Score: 0.5, Availability: 1, Validity: 1},
	}
	rng := rand.New(rand.NewSource(42))
	wins := map[string]int{}
	for i := 0; i < 50; i++ {
		got, ok := skill.SelectThompson(ranked, rng)
		if !ok {
			t.Fatal("expected selection")
		}
		wins[got.Skill.Name]++
	}
	if wins["high"] <= wins["low"] {
		t.Fatalf("thompson should prefer high alpha: %#v", wins)
	}
}

func TestCompareReplayArmsPreferSkill(t *testing.T) {
	t.Parallel()
	cf := skill.CompareReplayArms(
		skill.ReplayArm{Success: true, Steps: 2, Reward: 1},
		skill.ReplayArm{Success: false, Steps: 5, Reward: 0},
	)
	if !cf.PreferSkill || cf.ACEReward != 1 || cf.ACESteps != 3 {
		t.Fatalf("%#v", cf)
	}
}
