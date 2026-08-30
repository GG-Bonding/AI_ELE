package ghsim

import "strings"

// Result is one GitHub tool call outcome.
type Result struct {
	OK        bool
	ErrorCode string
	Payload   map[string]any
}

// Simulator is a tiny deterministic GitHub PR environment.
type Simulator struct{}

// New constructs the simulator.
func New() *Simulator { return &Simulator{} }

// Call executes one tool.
func (s *Simulator) Call(tool string, input map[string]any) Result {
	switch tool {
	case "github.get_pr":
		num := strings.TrimSpace(asString(input["number"]))
		if num == "" || num == "Payment" {
			return Result{OK: false, ErrorCode: "INVALID_PR_NUMBER", Payload: map[string]any{"error": "number required"}}
		}
		return Result{OK: true, Payload: map[string]any{"number": num, "state": "open"}}
	case "github.merge_pr":
		num := strings.TrimSpace(asString(input["number"]))
		if num == "" || !isDigits(num) {
			return Result{OK: false, ErrorCode: "INVALID_PR_NUMBER", Payload: map[string]any{"error": "bad number"}}
		}
		return Result{OK: true, Payload: map[string]any{"merged": true, "number": num}}
	default:
		return Result{OK: false, ErrorCode: "UNKNOWN_TOOL", Payload: map[string]any{"tool": tool}}
	}
}

// ToolCall is a planned invocation.
type ToolCall struct {
	Tool  string
	Input map[string]any
}

// AgentPolicy plans from context tips.
type AgentPolicy struct{}

// Plan returns tool calls; first matching tip wins.
func (AgentPolicy) Plan(task string, contextContents []string) []ToolCall {
	_ = task
	for _, c := range contextContents {
		lower := strings.ToLower(c)
		if strings.Contains(lower, "numeric pr") || strings.Contains(lower, "pr number") {
			return []ToolCall{
				{Tool: "github.get_pr", Input: map[string]any{"number": "42"}},
				{Tool: "github.merge_pr", Input: map[string]any{"number": "42"}},
			}
		}
		if strings.Contains(lower, "use title") || strings.Contains(lower, "payment as number") {
			return []ToolCall{{Tool: "github.merge_pr", Input: map[string]any{"number": "Payment"}}}
		}
	}
	return []ToolCall{{Tool: "github.merge_pr", Input: map[string]any{"number": "Payment"}}}
}

// Run executes planned calls; success requires final merge OK.
func (s *Simulator) Run(calls []ToolCall) (bool, []Result) {
	steps := make([]Result, 0, len(calls))
	ok := false
	for _, c := range calls {
		res := s.Call(c.Tool, c.Input)
		steps = append(steps, res)
		if c.Tool == "github.merge_pr" {
			ok = res.OK
		}
	}
	return ok, steps
}

func asString(v any) string {
	if v == nil {
		return ""
	}
	return strings.TrimSpace(stringify(v))
}

func stringify(v any) string {
	switch x := v.(type) {
	case string:
		return x
	default:
		return ""
	}
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
