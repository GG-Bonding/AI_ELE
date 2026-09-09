package replay_test

import (
	"context"
	"testing"

	"github.com/agent-experience-engine/agent-experience-engine/internal/eval/jirasim"
	"github.com/agent-experience-engine/agent-experience-engine/internal/replay"
)

func TestJiraReplayEnvironmentPrefersSkill(t *testing.T) {
	t.Parallel()
	env := replay.JiraEnvironment{Mode: jirasim.ModeStrict}
	cf, err := replay.Compare(context.Background(), env, replay.Task{
		ID: "1", Prompt: "Create a Jira issue for payment timeout",
		Inputs: map[string]any{"project_name": "Payment", "title": "timeout"},
	}, `
name: jira_safe_create_issue
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
`, []string{"Always use display name Payment as the project field"})
	if err != nil {
		t.Fatal(err)
	}
	if !cf.PreferSkill || cf.ACEReward <= 0 {
		t.Fatalf("%#v", cf)
	}
}
