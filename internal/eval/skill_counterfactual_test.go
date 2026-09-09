package eval_test

import (
	"context"
	"testing"

	"github.com/agent-experience-engine/agent-experience-engine/internal/eval"
)

func TestReplayJiraCounterfactualPrefersSkill(t *testing.T) {
	t.Parallel()
	cf, err := eval.ReplayJiraCounterfactual(context.Background(), `
name: jira_safe_create_issue
description: Resolve project key then create issue
inputs:
  project_name: {type: string, required: true}
  title: {type: string, required: true}
steps:
  - id: resolve_project
    tool: jira.search_projects
    args: {query: "{{ inputs.project_name }}"}
    save_as: project
  - id: create_issue
    tool: jira.create_issue
    args:
      project: "{{ project.key }}"
      title: "{{ inputs.title }}"
risk: {level: LOW}
max_steps: 5
`)
	if err != nil {
		t.Fatal(err)
	}
	if !cf.PreferSkill || cf.ACEReward <= 0 {
		t.Fatalf("%#v", cf)
	}
}
