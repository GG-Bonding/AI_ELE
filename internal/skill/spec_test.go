package skill_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/agent-experience-engine/agent-experience-engine/internal/skill"
)

const jiraSafeCreateYAML = `
name: jira_safe_create_issue
version: 1
description: Resolve project key then create issue
inputs:
  project_name:
    type: string
    required: true
  title:
    type: string
    required: true
outputs:
  issue_key:
    type: string
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
max_steps: 8
timeout_ms: 20000
idempotent: true
`

func TestParseYAMLJiraSafeCreate(t *testing.T) {
	t.Parallel()
	spec, err := skill.ParseYAML(jiraSafeCreateYAML)
	if err != nil {
		t.Fatalf("ParseYAML: %v", err)
	}
	if spec.Name != "jira_safe_create_issue" {
		t.Fatalf("name=%q", spec.Name)
	}
	if len(spec.Steps) != 2 {
		t.Fatalf("steps=%d", len(spec.Steps))
	}
	if spec.Steps[0].Tool != "jira.search_projects" || spec.Steps[0].SaveAs != "project" {
		t.Fatalf("step0=%#v", spec.Steps[0])
	}
	if spec.Steps[1].Tool != "jira.create_issue" {
		t.Fatalf("step1=%#v", spec.Steps[1])
	}
	if spec.Inputs["project_name"].Type != skill.FieldString || !spec.Inputs["project_name"].Required {
		t.Fatalf("inputs=%#v", spec.Inputs)
	}
	if spec.Risk.Level != skill.RiskLow {
		t.Fatalf("risk=%s", spec.Risk.Level)
	}
	if spec.MaxSteps != 8 || spec.TimeoutMs != 20000 || !spec.Idempotent {
		t.Fatalf("limits=%#v", spec)
	}
}

func TestParseYAMLRequiresStepsAndName(t *testing.T) {
	t.Parallel()
	if _, err := skill.ParseYAML(`name: x`); err == nil || !strings.Contains(err.Error(), "step") {
		t.Fatalf("want steps error, got %v", err)
	}
	if _, err := skill.ParseYAML(`steps: [{id: a, tool: t}]`); err == nil || !strings.Contains(err.Error(), "name") {
		t.Fatalf("want name error, got %v", err)
	}
}

func TestHashSpecStable(t *testing.T) {
	t.Parallel()
	spec, err := skill.ParseYAML(jiraSafeCreateYAML)
	if err != nil {
		t.Fatal(err)
	}
	h1, err := skill.HashSpec(spec)
	if err != nil {
		t.Fatal(err)
	}
	h2, err := skill.HashSpec(spec)
	if err != nil {
		t.Fatal(err)
	}
	if h1 != h2 || len(h1) != 64 {
		t.Fatalf("hash unstable: %s vs %s", h1, h2)
	}
}

func TestNewVersionAndMemoryRepository(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := skill.NewMemoryRepository()
	now := time.Date(2026, 9, 5, 12, 0, 0, 0, time.UTC)

	sk, err := repo.CreateSkill(ctx, skill.Skill{
		TenantID: "t", Name: "jira_safe_create_issue", Description: "safe create",
		Status: skill.StatusCandidate, CreatedAt: now, UpdatedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	spec, err := skill.ParseYAML(jiraSafeCreateYAML)
	if err != nil {
		t.Fatal(err)
	}
	ver, err := skill.NewVersion("t", sk.ID, "pat_1", 1, spec, jiraSafeCreateYAML, 0.9, 0.8, now)
	if err != nil {
		t.Fatal(err)
	}
	if ver.Status != skill.VersionCandidate || ver.ValidationStatus != skill.ValidationPending {
		t.Fatalf("%#v", ver)
	}
	if ver.SpecHash == "" {
		t.Fatal("missing spec hash")
	}
	stored, err := repo.CreateVersion(ctx, ver)
	if err != nil {
		t.Fatal(err)
	}
	got, err := repo.GetVersionByNumber(ctx, "t", sk.ID, 1)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != stored.ID || got.Spec.Name != "jira_safe_create_issue" {
		t.Fatalf("%#v", got)
	}
	listed, err := repo.ListVersions(ctx, "t", sk.ID)
	if err != nil || len(listed) != 1 {
		t.Fatalf("listed=%v err=%v", listed, err)
	}

	// Immutable: second create of same version number conflicts.
	ver.ID = ""
	if _, err := repo.CreateVersion(ctx, ver); err == nil {
		t.Fatal("expected conflict on duplicate version number")
	}

	// v2 is a new immutable revision.
	ver2, err := skill.NewVersion("t", sk.ID, "pat_1", 2, spec, jiraSafeCreateYAML, 0.91, 0.85, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.CreateVersion(ctx, ver2); err != nil {
		t.Fatal(err)
	}
	listed, err = repo.ListVersions(ctx, "t", sk.ID)
	if err != nil || len(listed) != 2 {
		t.Fatalf("listed=%d err=%v", len(listed), err)
	}
}

func TestDefaultsApplied(t *testing.T) {
	t.Parallel()
	spec, err := skill.ParseYAML(`
name: minimal
steps:
  - id: s1
    tool: jira.search_projects
`)
	if err != nil {
		t.Fatal(err)
	}
	if spec.MaxSteps != skill.DefaultMaxSteps || spec.TimeoutMs != skill.DefaultTimeoutMs {
		t.Fatalf("%#v", spec)
	}
	if spec.Risk.Level != skill.RiskLow {
		t.Fatalf("risk=%s", spec.Risk.Level)
	}
}
