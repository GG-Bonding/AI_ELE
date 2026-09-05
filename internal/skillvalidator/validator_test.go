package skillvalidator_test

import (
	"testing"
	"time"

	"github.com/agent-experience-engine/agent-experience-engine/internal/skill"
	"github.com/agent-experience-engine/agent-experience-engine/internal/skillvalidator"
	"github.com/agent-experience-engine/agent-experience-engine/internal/toolregistry"
)

const validJiraYAML = `
name: jira_safe_create_issue
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
`

func TestValidateValidJiraSkill(t *testing.T) {
	t.Parallel()
	spec, err := skill.ParseYAML(validJiraYAML)
	if err != nil {
		t.Fatal(err)
	}
	rep := skillvalidator.New(nil, skillvalidator.Options{TenantID: "t"}).Validate(spec)
	if !rep.OK {
		t.Fatalf("issues=%#v", rep.Issues)
	}
	if rep.ComputedRisk != skill.RiskLow {
		t.Fatalf("risk=%s", rep.ComputedRisk)
	}
	if len(rep.Tools) != 2 {
		t.Fatalf("tools=%v", rep.Tools)
	}
}

func TestValidateUnknownTool(t *testing.T) {
	t.Parallel()
	spec, err := skill.ParseYAML(`
name: bad
steps:
  - id: s1
    tool: not.a.tool
    args: {}
`)
	if err != nil {
		t.Fatal(err)
	}
	rep := skillvalidator.New(nil, skillvalidator.Options{}).Validate(spec)
	if rep.OK || !hasCode(rep, "TOOL_NOT_REGISTERED") {
		t.Fatalf("%#v", rep)
	}
}

func TestValidateForbiddenTool(t *testing.T) {
	t.Parallel()
	reg := toolregistry.New()
	_ = reg.Register(toolregistry.Definition{Name: "sys.shell", Risk: toolregistry.RiskCritical})
	spec, err := skill.ParseYAML(`
name: evil
steps:
  - id: s1
    tool: sys.shell
    args: {}
`)
	if err != nil {
		t.Fatal(err)
	}
	rep := skillvalidator.New(reg, skillvalidator.Options{}).Validate(spec)
	if rep.OK || !hasCode(rep, "FORBIDDEN_TOOL") {
		t.Fatalf("%#v", rep)
	}
}

func TestValidateDuplicateStepAndSaveAs(t *testing.T) {
	t.Parallel()
	spec, err := skill.ParseYAML(`
name: dup
steps:
  - id: s1
    tool: jira.search_projects
    args: {query: "x"}
    save_as: project
  - id: s1
    tool: jira.search_projects
    args: {query: "y"}
    save_as: project
`)
	if err != nil {
		t.Fatal(err)
	}
	rep := skillvalidator.New(nil, skillvalidator.Options{}).Validate(spec)
	if rep.OK || !hasCode(rep, "DUP_STEP_ID") || !hasCode(rep, "DUP_SAVE_AS") {
		t.Fatalf("%#v", rep)
	}
}

func TestValidateForwardReference(t *testing.T) {
	t.Parallel()
	spec, err := skill.ParseYAML(`
name: forward
inputs:
  project_name: {type: string}
steps:
  - id: create_issue
    tool: jira.create_issue
    args:
      project: "{{ project.key }}"
  - id: resolve_project
    tool: jira.search_projects
    args:
      query: "{{ inputs.project_name }}"
    save_as: project
`)
	if err != nil {
		t.Fatal(err)
	}
	rep := skillvalidator.New(nil, skillvalidator.Options{}).Validate(spec)
	if rep.OK || !hasCode(rep, "UNKNOWN_REF") {
		t.Fatalf("%#v", rep)
	}
}

func TestValidateMissingRequiredArg(t *testing.T) {
	t.Parallel()
	spec, err := skill.ParseYAML(`
name: missing_arg
steps:
  - id: s1
    tool: jira.search_projects
    args: {}
`)
	if err != nil {
		t.Fatal(err)
	}
	rep := skillvalidator.New(nil, skillvalidator.Options{}).Validate(spec)
	if rep.OK || !hasCode(rep, "TOOL_ARG_REQUIRED") {
		t.Fatalf("%#v", rep)
	}
}

func TestValidateRiskUnderstated(t *testing.T) {
	t.Parallel()
	spec, err := skill.ParseYAML(`
name: understate
steps:
  - id: s1
    tool: jira.delete_issue
    args:
      issue_key: "PAY-1"
risk:
  level: READ_ONLY
`)
	if err != nil {
		t.Fatal(err)
	}
	rep := skillvalidator.New(nil, skillvalidator.Options{}).Validate(spec)
	if rep.OK || !hasCode(rep, "RISK_UNDERSTATED") {
		t.Fatalf("%#v", rep)
	}
}

func TestValidateCriticalDenied(t *testing.T) {
	t.Parallel()
	reg := toolregistry.New()
	_ = reg.Register(toolregistry.Definition{
		Name: "payments.wire_transfer",
		Risk: toolregistry.RiskCritical,
		InputSchema: map[string]toolregistry.ParamSchema{
			"amount": {Type: toolregistry.ParamNumber, Required: true},
		},
		SideEffect: true,
	})
	spec, err := skill.ParseYAML(`
name: wire
steps:
  - id: s1
    tool: payments.wire_transfer
    args:
      amount: 100
risk:
  level: CRITICAL
  requires_approval: true
`)
	if err != nil {
		t.Fatal(err)
	}
	rep := skillvalidator.New(reg, skillvalidator.Options{}).Validate(spec)
	if rep.OK || !hasCode(rep, "CRITICAL_DENIED") {
		t.Fatalf("%#v", rep)
	}
	rep2 := skillvalidator.New(reg, skillvalidator.Options{AllowCritical: true}).Validate(spec)
	if !rep2.OK || !rep2.RequiresApproval {
		t.Fatalf("%#v", rep2)
	}
}

func TestValidateTenantDenied(t *testing.T) {
	t.Parallel()
	reg := toolregistry.New()
	_ = reg.Register(toolregistry.Definition{
		Name: "tenant.only", Risk: toolregistry.RiskReadOnly,
		AllowedTenants: []string{"allowed"},
	})
	spec, err := skill.ParseYAML(`
name: t
steps:
  - id: s1
    tool: tenant.only
`)
	if err != nil {
		t.Fatal(err)
	}
	rep := skillvalidator.New(reg, skillvalidator.Options{TenantID: "other"}).Validate(spec)
	if rep.OK || !hasCode(rep, "TOOL_TENANT_DENIED") {
		t.Fatalf("%#v", rep)
	}
}

func TestApplyToVersion(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 9, 5, 12, 0, 0, 0, time.UTC)
	spec, err := skill.ParseYAML(validJiraYAML)
	if err != nil {
		t.Fatal(err)
	}
	rep := skillvalidator.New(nil, skillvalidator.Options{}).Validate(spec)
	if !rep.OK {
		t.Fatalf("%#v", rep.Issues)
	}
	ver, err := skill.NewVersion("t", "sk1", "pat", 1, spec, validJiraYAML, 0.9, 0.8, now)
	if err != nil {
		t.Fatal(err)
	}
	out := skillvalidator.ApplyToVersion(ver, rep)
	if out.ValidationStatus != skill.ValidationPassed || out.Status != skill.VersionValidated {
		t.Fatalf("%#v", out)
	}

	bad, err := skill.ParseYAML(`
name: bad
steps:
  - id: s1
    tool: missing.tool
`)
	if err != nil {
		t.Fatal(err)
	}
	badRep := skillvalidator.New(nil, skillvalidator.Options{}).Validate(bad)
	badVer, err := skill.NewVersion("t", "sk1", "", 1, bad, "", 0.5, 0.5, now)
	if err != nil {
		t.Fatal(err)
	}
	failed := skillvalidator.ApplyToVersion(badVer, badRep)
	if failed.ValidationStatus != skill.ValidationFailed {
		t.Fatalf("%#v", failed)
	}
}

func hasCode(rep skillvalidator.Report, code string) bool {
	for _, iss := range rep.Issues {
		if iss.Code == code {
			return true
		}
	}
	return false
}
