package skillruntime

import (
	"context"
	"fmt"

	"github.com/agent-experience-engine/agent-experience-engine/internal/eval/jirasim"
	"github.com/agent-experience-engine/agent-experience-engine/internal/toolregistry"
)

// ToolResult is one tool invocation outcome.
type ToolResult struct {
	OK        bool
	ErrorCode string
	Output    map[string]any
	Unknown   bool // ambiguous remote outcome (timeout after send)
}

// ToolCall carries durable-execution / credential metadata for one tool invoke (V3.4).
type ToolCall struct {
	Tool           string
	Input          map[string]any
	IdempotencyKey string // stable operation key (execution:step)
	TenantID       string
	PrincipalID    string
	ExecutionID    string
	StepID         string
	Attempt        int
}

// ToolExecutor invokes registered tools for LIVE (and READ_ONLY shadow) steps.
type ToolExecutor interface {
	Execute(ctx context.Context, tool string, input map[string]any) (ToolResult, error)
}

// CallAwareExecutor optionally receives idempotency / execution context (V3.3).
type CallAwareExecutor interface {
	ExecuteToolCall(ctx context.Context, call ToolCall) (ToolResult, error)
}

// PreviewExecutor dry-runs side-effect tools for SHADOW mode.
// Runtime never calls ToolExecutor.Execute for side-effect tools while shadowing.
type PreviewExecutor interface {
	Preview(ctx context.Context, tool string, input map[string]any) (ToolResult, error)
}

// CallAwarePreviewer optionally receives idempotency context for shadow (V3.3).
type CallAwarePreviewer interface {
	PreviewToolCall(ctx context.Context, call ToolCall) (ToolResult, error)
}

// ErrShadowUnsupported means a side-effect tool cannot be safely shadowed.
var ErrShadowUnsupported = fmt.Errorf("shadow unsupported: tool lacks PreviewExecutor")

// JiraSimExecutor wraps jirasim.Simulator for Skill runs.
type JiraSimExecutor struct {
	Sim      *jirasim.Simulator
	Registry *toolregistry.Registry
}

// Execute implements ToolExecutor (always real sim call — no shadow flag).
func (e *JiraSimExecutor) Execute(ctx context.Context, tool string, input map[string]any) (ToolResult, error) {
	_ = ctx
	return e.call(tool, input)
}

// ExecuteToolCall implements CallAwareExecutor.
func (e *JiraSimExecutor) ExecuteToolCall(ctx context.Context, call ToolCall) (ToolResult, error) {
	return e.Execute(ctx, call.Tool, call.Input)
}

// Preview implements PreviewExecutor for side-effect tools (no real write).
func (e *JiraSimExecutor) Preview(ctx context.Context, tool string, input map[string]any) (ToolResult, error) {
	_ = ctx
	reg := e.Registry
	if reg == nil {
		reg = toolregistry.Default()
	}
	if def, ok := reg.Get(tool); ok && def.SideEffect {
		out := map[string]any{"_shadow": true}
		for k, v := range input {
			out[k] = v
		}
		return ToolResult{OK: true, Output: out}, nil
	}
	// READ_ONLY tools may preview via Execute semantics.
	return e.call(tool, input)
}

// PreviewToolCall implements CallAwarePreviewer.
func (e *JiraSimExecutor) PreviewToolCall(ctx context.Context, call ToolCall) (ToolResult, error) {
	return e.Preview(ctx, call.Tool, call.Input)
}

func (e *JiraSimExecutor) call(tool string, input map[string]any) (ToolResult, error) {
	sim := e.Sim
	if sim == nil {
		sim = jirasim.New()
	}
	res := sim.Call(tool, input)
	out := res.Payload
	if out == nil {
		out = map[string]any{}
	} else {
		cp := make(map[string]any, len(out)+2)
		for k, v := range out {
			cp[k] = v
		}
		out = cp
	}
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
