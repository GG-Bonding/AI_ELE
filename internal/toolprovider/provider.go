package toolprovider

import (
	"context"
	"fmt"

	"github.com/agent-experience-engine/agent-experience-engine/internal/toolregistry"
)

// Call is one tool invocation with durable-execution metadata (V3.3).
type Call struct {
	Tool           string
	Input          map[string]any
	IdempotencyKey string
	TenantID       string
	PrincipalID    string
	ExecutionID    string
	StepID         string
	Attempt        int
	Headers        map[string]string // filled by credential resolver / router
}

// Result is a provider-neutral tool outcome.
type Result struct {
	OK        bool
	ErrorCode string
	Output    map[string]any
}

// Provider is a pluggable tool backend (simulator, MCP, HTTP, native).
type Provider interface {
	Name() string
	ListTools(ctx context.Context) ([]toolregistry.Definition, error)
	Execute(ctx context.Context, call Call) (Result, error)
	Preview(ctx context.Context, call Call) (Result, error)
}

// ErrUnknownTool means no provider owns the tool name.
var ErrUnknownTool = fmt.Errorf("toolprovider: unknown tool")

// ToolIdempotencyKey builds a stable tool-level key from execution+step+attempt.
func ToolIdempotencyKey(executionID, stepID string, attempt int) string {
	if attempt <= 0 {
		attempt = 1
	}
	return fmt.Sprintf("%s:%s:%d", executionID, stepID, attempt)
}
