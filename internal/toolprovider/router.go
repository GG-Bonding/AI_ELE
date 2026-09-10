package toolprovider

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/agent-experience-engine/agent-experience-engine/internal/skillruntime"
	"github.com/agent-experience-engine/agent-experience-engine/internal/toolregistry"
)

// CredentialResolver resolves tenant/principal credentials for a tool (no secrets in SkillSpec).
type CredentialResolver interface {
	Resolve(ctx context.Context, tenantID, principalID, toolName string) (map[string]string, error)
}

// NoopCredentials returns empty headers.
type NoopCredentials struct{}

func (NoopCredentials) Resolve(context.Context, string, string, string) (map[string]string, error) {
	return nil, nil
}

// Router dispatches tool calls across Providers and adapts to skillruntime executors.
type Router struct {
	Providers             []Provider
	Registry              *toolregistry.Registry
	Credentials           CredentialResolver
	RejectDuplicateRoutes bool // V3.4: fail SyncRegistry on name clash
	mu                    sync.RWMutex
	routes                map[string]Provider
}

// NewRouter builds a router. Call SyncRegistry to register tools into Registry.
func NewRouter(providers []Provider, registry *toolregistry.Registry, creds CredentialResolver) *Router {
	if registry == nil {
		registry = toolregistry.New()
	}
	if creds == nil {
		creds = NoopCredentials{}
	}
	return &Router{
		Providers:   providers,
		Registry:    registry,
		Credentials: creds,
		routes:      map[string]Provider{},
	}
}

// SyncRegistry lists tools from all providers and registers them.
func (r *Router) SyncRegistry(ctx context.Context) error {
	if r == nil {
		return fmt.Errorf("toolprovider: nil router")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.routes = map[string]Provider{}
	for _, p := range r.Providers {
		if p == nil {
			return fmt.Errorf("%w: nil provider", ErrUnknownTool)
		}
		defs, err := p.ListTools(ctx)
		if err != nil {
			return fmt.Errorf("list tools from %s: %w", p.Name(), err)
		}
		for _, def := range defs {
			name := strings.TrimSpace(def.Name)
			if name == "" {
				continue
			}
			if prev, ok := r.routes[name]; ok && r.RejectDuplicateRoutes {
				return fmt.Errorf("%w: %s claimed by %s and %s", ErrDuplicateRoute, name, prev.Name(), p.Name())
			}
			if err := r.Registry.Register(def); err != nil {
				return err
			}
			r.routes[name] = p
		}
	}
	return nil
}

// RegisterRoute manually maps a tool to a provider (tests / overrides).
func (r *Router) RegisterRoute(tool string, p Provider) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.routes[strings.TrimSpace(tool)] = p
}

func (r *Router) providerFor(tool string) (Provider, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.routes[strings.TrimSpace(tool)]
	if !ok || p == nil {
		return nil, fmt.Errorf("%w: %s", ErrUnknownTool, tool)
	}
	return p, nil
}

func (r *Router) enrich(ctx context.Context, call Call) (Call, error) {
	headers, err := r.Credentials.Resolve(ctx, call.TenantID, call.PrincipalID, call.Tool)
	if err != nil {
		return call, err
	}
	if len(headers) > 0 {
		if call.Headers == nil {
			call.Headers = map[string]string{}
		}
		for k, v := range headers {
			call.Headers[k] = v
		}
	}
	return call, nil
}

// Execute implements skillruntime.ToolExecutor.
func (r *Router) Execute(ctx context.Context, tool string, input map[string]any) (skillruntime.ToolResult, error) {
	return r.ExecuteCall(ctx, Call{Tool: tool, Input: input})
}

// Preview implements skillruntime.PreviewExecutor.
func (r *Router) Preview(ctx context.Context, tool string, input map[string]any) (skillruntime.ToolResult, error) {
	return r.PreviewCall(ctx, Call{Tool: tool, Input: input})
}

// ExecuteCall runs a contextual tool call via the owning provider.
func (r *Router) ExecuteCall(ctx context.Context, call Call) (skillruntime.ToolResult, error) {
	p, err := r.providerFor(call.Tool)
	if err != nil {
		return skillruntime.ToolResult{}, err
	}
	call, err = r.enrich(ctx, call)
	if err != nil {
		return skillruntime.ToolResult{}, err
	}
	res, err := p.Execute(ctx, call)
	return toRuntime(res), err
}

// ExecuteToolCall implements skillruntime.CallAwareExecutor.
func (r *Router) ExecuteToolCall(ctx context.Context, call skillruntime.ToolCall) (skillruntime.ToolResult, error) {
	return r.ExecuteCall(ctx, Call{
		Tool: call.Tool, Input: call.Input, IdempotencyKey: call.IdempotencyKey,
		TenantID: call.TenantID, PrincipalID: call.PrincipalID,
		ExecutionID: call.ExecutionID, StepID: call.StepID, Attempt: call.Attempt,
	})
}

// PreviewCall dry-runs via the owning provider after local schema validation.
func (r *Router) PreviewCall(ctx context.Context, call Call) (skillruntime.ToolResult, error) {
	if def, ok := r.Registry.Get(call.Tool); ok {
		if err := toolregistry.ValidateInput(def.InputSchema, call.Input); err != nil {
			return skillruntime.ToolResult{
				OK: false, ErrorCode: "SCHEMA_INVALID",
				Output: map[string]any{"error": err.Error()},
			}, nil
		}
		if def.PreviewCapability == toolregistry.PreviewNone {
			return skillruntime.ToolResult{OK: false, ErrorCode: "PREVIEW_UNSUPPORTED"}, nil
		}
	}
	p, err := r.providerFor(call.Tool)
	if err != nil {
		return skillruntime.ToolResult{}, err
	}
	call, err = r.enrich(ctx, call)
	if err != nil {
		return skillruntime.ToolResult{}, err
	}
	res, err := p.Preview(ctx, call)
	return toRuntime(res), err
}

// PreviewToolCall implements skillruntime.CallAwarePreviewer.
func (r *Router) PreviewToolCall(ctx context.Context, call skillruntime.ToolCall) (skillruntime.ToolResult, error) {
	return r.PreviewCall(ctx, Call{
		Tool: call.Tool, Input: call.Input, IdempotencyKey: call.IdempotencyKey,
		TenantID: call.TenantID, PrincipalID: call.PrincipalID,
		ExecutionID: call.ExecutionID, StepID: call.StepID, Attempt: call.Attempt,
	})
}

// Reconcile resolves UNKNOWN outcomes when a provider supports QUERY_RECONCILE.
func (r *Router) Reconcile(ctx context.Context, call skillruntime.ToolCall) (skillruntime.ToolResult, error) {
	p, err := r.providerFor(call.Tool)
	if err != nil {
		return skillruntime.ToolResult{}, err
	}
	tpCall := Call{
		Tool: call.Tool, Input: call.Input, IdempotencyKey: call.IdempotencyKey,
		TenantID: call.TenantID, PrincipalID: call.PrincipalID,
		ExecutionID: call.ExecutionID, StepID: call.StepID, Attempt: call.Attempt,
	}
	tpCall, err = r.enrich(ctx, tpCall)
	if err != nil {
		return skillruntime.ToolResult{}, err
	}
	if rec, ok := p.(interface {
		Reconcile(context.Context, Call) (Result, error)
	}); ok {
		res, rerr := rec.Reconcile(ctx, tpCall)
		return toRuntime(res), rerr
	}
	return skillruntime.ToolResult{OK: false, ErrorCode: "RECONCILE_UNSUPPORTED", Unknown: true}, nil
}

func toRuntime(res Result) skillruntime.ToolResult {
	return skillruntime.ToolResult{
		OK: res.OK, ErrorCode: res.ErrorCode, Output: res.Output, Unknown: res.Unknown,
	}
}
