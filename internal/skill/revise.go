package skill

import (
	"context"
	"fmt"
	"strings"
)

// RevisionHint carries failure / pattern signals for automatic revision (V3.2).
type RevisionHint struct {
	FailureCodes    []string // e.g. INVALID_PROJECT_KEY, TOOL_FAILED
	FailureMessages []string
	PatternContent  string // related pattern / experience text
}

// SuggestRevisionYAML proposes an improved Skill Spec YAML from failure signals.
// Returns ok=false when no safe automatic revision is known.
func SuggestRevisionYAML(current Spec, hint RevisionHint) (yamlDoc string, ok bool) {
	blob := strings.ToLower(strings.Join(append(append([]string{}, hint.FailureCodes...), hint.FailureMessages...), " "))
	blob += " " + strings.ToLower(hint.PatternContent)

	needsProjectResolve := strings.Contains(blob, "project") &&
		(strings.Contains(blob, "invalid") || strings.Contains(blob, "unknown") ||
			strings.Contains(blob, "key") || strings.Contains(blob, "resolve") || strings.Contains(blob, "search"))

	hasSearch := false
	hasCreate := false
	for _, st := range current.Steps {
		switch st.Tool {
		case "jira.search_projects":
			hasSearch = true
		case "jira.create_issue":
			hasCreate = true
		}
	}
	if needsProjectResolve && hasCreate && !hasSearch {
		name := current.Name
		if name == "" {
			name = "jira_safe_create_issue"
		}
		return fmt.Sprintf(`
name: %s
description: Resolve project key then create issue (auto-revision)
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
`, name), true
	}
	return "", false
}

// AutoRevise creates an immutable CANDIDATE revision when SuggestRevisionYAML finds a fix.
func AutoRevise(ctx context.Context, repo Repository, tenantID, skillID, patternID string, current Spec, hint RevisionHint) (Version, bool, error) {
	yamlDoc, ok := SuggestRevisionYAML(current, hint)
	if !ok {
		return Version{}, false, nil
	}
	ver, err := ProposeRevision(ctx, repo, tenantID, skillID, yamlDoc, patternID)
	if err != nil {
		return Version{}, false, err
	}
	return ver, true, nil
}
