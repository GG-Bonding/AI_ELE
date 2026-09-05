package skillruntime

import (
	"context"

	"github.com/agent-experience-engine/agent-experience-engine/internal/eval/jirasim"
	"github.com/agent-experience-engine/agent-experience-engine/internal/toolregistry"
)

// ToolResult is one tool invocation outcome from a ToolExecutor.
type ToolResult struct {
	OK        bool
	ErrorCode string
	Output    map[string]any
}

// ToolExecutor invokes registered tools (real or simulated).
type ToolExecutor interface {
	Call(ctx context.Context, tool string, input map[string]any, shadow bool) (ToolResult, error)
}

// JiraSimExecutor wraps jirasim.Simulator for Skill runs.
// Side-effect tools are short-circuited in shadow mode (no create/delete).
type JiraSimExecutor struct {
	Sim      *jirasim.Simulator
	Registry *toolregistry.Registry
}

// Call implements ToolExecutor.
func (e *JiraSimExecutor) Call(ctx context.Context, tool string, input map[string]any, shadow bool) (ToolResult, error) {
	_ = ctx
	reg := e.Registry
	if reg == nil {
		reg = toolregistry.Default()
	}
	if shadow {
		if def, ok := reg.Get(tool); ok && def.SideEffect {
			out := map[string]any{"_shadow": true}
			for k, v := range input {
				out[k] = v
			}
			return ToolResult{OK: true, Output: out}, nil
		}
	}

	sim := e.Sim
	if sim == nil {
		sim = jirasim.New()
	}
	res := sim.Call(tool, input)
	out := res.Payload
	if out == nil {
		out = map[string]any{}
	} else {
		// Shallow copy so callers can mutate safely.
		cp := make(map[string]any, len(out)+2)
		for k, v := range out {
			cp[k] = v
		}
		out = cp
	}
	// Promote first search hit to top-level key/name for {{ project.key }} templates.
	if tool == "jira.search_projects" && res.OK {
		enrichSearchProject(out)
	}
	return ToolResult{OK: res.OK, ErrorCode: res.ErrorCode, Output: out}, nil
}

func enrichSearchProject(out map[string]any) {
	projects, ok := out["projects"]
	if !ok {
		return
	}
	var first map[string]any
	switch list := projects.(type) {
	case []map[string]any:
		if len(list) > 0 {
			first = list[0]
		}
	case []any:
		if len(list) > 0 {
			if m, ok := list[0].(map[string]any); ok {
				first = m
			}
		}
	}
	if first == nil {
		return
	}
	if _, ok := out["key"]; !ok {
		if k, ok := first["key"]; ok {
			out["key"] = k
		}
	}
	if _, ok := out["name"]; !ok {
		if n, ok := first["name"]; ok {
			out["name"] = n
		}
	}
}
