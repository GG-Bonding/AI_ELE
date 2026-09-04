package jirasim

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Mode controls whether display-name project values are accepted (Env V1) or rejected (Env V2).
type Mode string

const (
	// ModeStrict requires a project key such as PAY (default; Env V2 / production-like).
	ModeStrict Mode = "strict"
	// ModeLenient accepts the Payment display name as a project value (Env V1).
	ModeLenient Mode = "lenient"
)

// Result is one tool invocation outcome from the simulator.
type Result struct {
	OK        bool
	ErrorCode string
	Payload   map[string]any
}

// Simulator is a tiny deterministic Jira tool environment for evaluation.
type Simulator struct {
	mode     Mode
	projects map[string]string // display/query → key
}

// New constructs a simulator with Payment → PAY mapping in strict mode.
func New() *Simulator {
	return &Simulator{
		mode: ModeStrict,
		projects: map[string]string{
			"payment": "PAY",
			"pay":     "PAY",
		},
	}
}

// WithMode switches Env V1 (lenient) vs Env V2 (strict).
func (s *Simulator) WithMode(mode Mode) *Simulator {
	if mode == "" {
		mode = ModeStrict
	}
	s.mode = mode
	return s
}

// Mode returns the current environment mode.
func (s *Simulator) Mode() Mode { return s.mode }

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
		if s.mode == ModeLenient && strings.EqualFold(project, "Payment") {
			return Result{OK: true, Payload: map[string]any{"issue_key": "PAY-1001", "note": "accepted display name"}}
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
		// Prefer explicit display-name mandate (Env V1 tip) before generic "resolve" substrings.
		// E1 text may include "never resolve project key" which still contains "resolve project key".
		// "never use display name …" still contains "use display name …"; exclude negation first.
		neverDisplay := strings.Contains(joined, "never use display") ||
			strings.Contains(joined, "must never") && strings.Contains(joined, "display name")
		useDisplayName := !neverDisplay && (strings.Contains(joined, "must always use display name") ||
			strings.Contains(joined, "must use display name") ||
			strings.Contains(joined, "use display name payment") ||
			strings.Contains(joined, "payment as project"))
		useProjectKey := neverDisplay ||
			strings.Contains(joined, "resolve project key") ||
			strings.Contains(joined, "search project") ||
			strings.Contains(joined, "project key before") ||
			strings.Contains(joined, "use project key pay")
		switch {
		case useDisplayName:
			return []ToolCall{{Tool: "jira.create_issue", Input: map[string]any{"project": "Payment", "summary": "payment timeout"}}}
		case useProjectKey:
			return []ToolCall{
				{Tool: "jira.search_projects", Input: map[string]any{"query": "Payment"}},
				{Tool: "jira.create_issue", Input: map[string]any{"project": "PAY", "summary": "payment timeout"}},
			}
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

// ExecuteWithRecovery plans from context, runs tools, and on INVALID_PROJECT_KEY
// recovers by searching then creating with the project key (Env V2 shock path).
func (s *Simulator) ExecuteWithRecovery(task string, contextContents []string) (success bool, calls []ToolCall, steps []Result) {
	calls = AgentPolicy{}.Plan(task, contextContents)
	success, steps = s.Run(calls)
	if success {
		return success, calls, steps
	}
	for _, step := range steps {
		if step.ErrorCode != "INVALID_PROJECT_KEY" {
			continue
		}
		recovery := []ToolCall{
			{Tool: "jira.search_projects", Input: map[string]any{"query": "Payment"}},
			{Tool: "jira.create_issue", Input: map[string]any{"project": "PAY", "summary": "payment timeout"}},
		}
		ok, more := s.Run(recovery)
		calls = append(calls, recovery...)
		steps = append(steps, more...)
		return ok, calls, steps
	}
	return success, calls, steps
}

// MustJSON marshals v or panics (test helper).
func MustJSON(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}
