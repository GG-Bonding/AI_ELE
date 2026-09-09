package simulator

import (
	"context"

	"github.com/agent-experience-engine/agent-experience-engine/internal/eval/jirasim"
	"github.com/agent-experience-engine/agent-experience-engine/internal/toolprovider"
	"github.com/agent-experience-engine/agent-experience-engine/internal/toolregistry"
)

// JiraProvider exposes the in-process Jira simulator as a ToolProvider (test / default offline).
type JiraProvider struct {
	Sim      *jirasim.Simulator
	Registry *toolregistry.Registry
}

func (p *JiraProvider) Name() string { return "simulator/jira" }

func (p *JiraProvider) ListTools(context.Context) ([]toolregistry.Definition, error) {
	reg := p.Registry
	if reg == nil {
		reg = toolregistry.Default()
	}
	var out []toolregistry.Definition
	for _, d := range reg.List() {
		if len(d.Name) >= 5 && d.Name[:5] == "jira." {
			out = append(out, d)
		}
	}
	if len(out) == 0 {
		return reg.List(), nil
	}
	return out, nil
}

func (p *JiraProvider) Execute(ctx context.Context, call toolprovider.Call) (toolprovider.Result, error) {
	_ = ctx
	return p.call(call.Tool, call.Input)
}

func (p *JiraProvider) Preview(ctx context.Context, call toolprovider.Call) (toolprovider.Result, error) {
	_ = ctx
	reg := p.Registry
	if reg == nil {
		reg = toolregistry.Default()
	}
	if def, ok := reg.Get(call.Tool); ok && def.SideEffect {
		out := map[string]any{"_shadow": true, "_idempotency_key": call.IdempotencyKey}
		for k, v := range call.Input {
			out[k] = v
		}
		return toolprovider.Result{OK: true, Output: out}, nil
	}
	return p.call(call.Tool, call.Input)
}

func (p *JiraProvider) call(tool string, input map[string]any) (toolprovider.Result, error) {
	sim := p.Sim
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
	return toolprovider.Result{OK: res.OK, ErrorCode: res.ErrorCode, Output: out}, nil
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
