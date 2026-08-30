package jirasim

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Result is one tool invocation outcome from the simulator.
type Result struct {
	OK        bool
	ErrorCode string
	Payload   map[string]any
}

// Simulator is a tiny deterministic Jira tool environment for evaluation.
type Simulator struct {
	projects map[string]string // display/query → key
}

// New constructs a simulator with Payment → PAY mapping.
func New() *Simulator {
	return &Simulator{projects: map[string]string{
		"payment": "PAY",
		"pay":     "PAY",
	}}
}

// Call executes a tool against the simulator.
func (s *Simulator) Call(tool string, input map[string]any) Result {
	switch tool {
	case "jira.search_projects":
		q := strings.ToLower(strings.TrimSpace(fmt.Sprint(input["query"])))
		if key, ok := s.projects[q]; ok {
			return Result{OK: true, Payload: map[string]any{
				"projects": []map[string]any{{"name": fmt.Sprint(input["query"]), "key": key}},
			}}
		}
		return Result{OK: true, Payload: map[string]any{"projects": []any{}}}
	case "jira.create_issue":
		project := strings.TrimSpace(fmt.Sprint(input["project"]))
		if project == "" {
			return Result{OK: false, ErrorCode: "MISSING_PROJECT", Payload: map[string]any{"error": "project required"}}
		}
		for _, key := range s.projects {
			if project == key {
				return Result{OK: true, Payload: map[string]any{"issue_key": key + "-1001"}}
			}
		}
		return Result{OK: false, ErrorCode: "INVALID_PROJECT_KEY", Payload: map[string]any{
			"error": "project key invalid", "project": project,
		}}
	default:
		return Result{OK: false, ErrorCode: "UNKNOWN_TOOL", Payload: map[string]any{"tool": tool}}
	}
}

// ToolCall is one planned tool invocation.
type ToolCall struct {
	Tool  string
	Input map[string]any
}

// AgentPolicy decides tool calls from task + experience context.
// Success is determined only by simulator outcomes, not by this planner.
type AgentPolicy struct{}

// Plan returns the tool sequence an agent would run.
// Context order matters: the first matching tip wins (top-ranked experience),
// so raw similarity that surfaces harmful advice first can fail under the simulator.
func (AgentPolicy) Plan(task string, contextContents []string) []ToolCall {
	_ = task
	for _, content := range contextContents {
		joined := strings.ToLower(content)
		helpful := strings.Contains(joined, "resolve project key") ||
			strings.Contains(joined, "search project") ||
			strings.Contains(joined, "project key before")
		harmful := strings.Contains(joined, "display name") ||
			strings.Contains(joined, "payment as project")
		switch {
		case helpful:
			return []ToolCall{
				{Tool: "jira.search_projects", Input: map[string]any{"query": "Payment"}},
				{Tool: "jira.create_issue", Input: map[string]any{"project": "PAY", "summary": "payment timeout"}},
			}
		case harmful:
			return []ToolCall{{Tool: "jira.create_issue", Input: map[string]any{"project": "Payment", "summary": "payment timeout"}}}
		}
	}
	return []ToolCall{{Tool: "jira.create_issue", Input: map[string]any{"project": "Payment", "summary": "payment timeout"}}}
}

// Run executes a planned sequence; success requires final create_issue OK.
func (s *Simulator) Run(calls []ToolCall) (success bool, steps []Result) {
	steps = make([]Result, 0, len(calls))
	for _, c := range calls {
		res := s.Call(c.Tool, c.Input)
		steps = append(steps, res)
		if c.Tool == "jira.create_issue" {
			success = res.OK
		}
	}
	return success, steps
}

// MustJSON marshals v or panics (test helper).
func MustJSON(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}
