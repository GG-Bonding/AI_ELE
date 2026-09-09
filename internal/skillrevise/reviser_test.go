package skillrevise_test

import (
	"context"
	"testing"

	"github.com/agent-experience-engine/agent-experience-engine/internal/provider"
	"github.com/agent-experience-engine/agent-experience-engine/internal/skill"
	"github.com/agent-experience-engine/agent-experience-engine/internal/skillrevise"
)

func TestProposeOrFallbackRuleBased(t *testing.T) {
	t.Parallel()
	spec, err := skill.ParseYAML(`
name: jira_direct
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
`)
	if err != nil {
		t.Fatal(err)
	}
	p, ok, err := skillrevise.ProposeOrFallback(context.Background(), nil, skillrevise.Input{
		Current: spec, FailureCodes: []string{"INVALID_PROJECT_KEY"},
		FailureMessages: []string{"unknown project key"},
	})
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	if p.NewSpecYAML == "" || p.Confidence <= 0 {
		t.Fatalf("%#v", p)
	}
}

func TestLLMReviserParsesJSON(t *testing.T) {
	t.Parallel()
	mock := &provider.MockLLM{Responses: []string{`{"reason":"add search","changes":["search"],"new_spec_yaml":"name: fixed\ninputs:\n  project_name: {type: string, required: true}\n  title: {type: string, required: true}\nsteps:\n  - id: s\n    tool: jira.search_projects\n    args: {query: \"{{ inputs.project_name }}\"}\n    save_as: project\n  - id: c\n    tool: jira.create_issue\n    args:\n      project: \"{{ project.key }}\"\n      title: \"{{ inputs.title }}\"\nrisk: {level: LOW}\nmax_steps: 5\n","confidence":0.9,"evidence_ids":["e1"]}`}}
	rev, err := skillrevise.NewLLMReviser(mock)
	if err != nil {
		t.Fatal(err)
	}
	spec, _ := skill.ParseYAML(`
name: bad
inputs:
  project_name: {type: string, required: true}
  title: {type: string, required: true}
steps:
  - id: c
    tool: jira.create_issue
    args: {project: "{{ inputs.project_name }}", title: "{{ inputs.title }}"}
risk: {level: LOW}
`)
	p, err := rev.Propose(context.Background(), skillrevise.Input{Current: spec})
	if err != nil {
		t.Fatal(err)
	}
	if p.Confidence < 0.8 || p.Reason == "" {
		t.Fatalf("%#v", p)
	}
}
