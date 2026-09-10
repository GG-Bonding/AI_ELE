package toolprovider

import (
	"context"
	"fmt"

	"github.com/agent-experience-engine/agent-experience-engine/internal/toolregistry"
)

// Call is one tool invocation with durable-execution metadata (V3.4).
type Call struct {
	Tool           string
	Input          map[string]any
	IdempotencyKey string // stable logical operation key (execution:step), NOT attempt
	TenantID       string
	PrincipalID    string
	ExecutionID    string
	StepID         string
	Attempt        int // transport attempt for diagnostics only
	Headers        map[string]string
}

// ReconcileState is whether a prior side-effect applied (V3.4.1).
type ReconcileState string

const (
	ReconcileApplied    ReconcileState = "APPLIED"
	ReconcileNotApplied ReconcileState = "NOT_APPLIED"
	ReconcileUnknown    ReconcileState = "UNKNOWN"
)

// ReconcileResult is distinct from Result: query OK ≠ original operation applied.
type ReconcileResult struct {
	State     ReconcileState
	Output    map[string]any
	ErrorCode string
}

// Result is a provider-neutral tool outcome.
type Result struct {
	OK        bool
	ErrorCode string
	Output    map[string]any
	Unknown   bool // true when transport timeout / ambiguous remote outcome
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

// ErrDuplicateRoute means two providers claimed the same tool name.
var ErrDuplicateRoute = fmt.Errorf("toolprovider: duplicate tool route")

// OperationKey is the stable logical operation id (never includes attempt).
func OperationKey(executionID, stepID string) string {
	return fmt.Sprintf("%s:%s", executionID, stepID)
}

// ToolIdempotencyKey returns the stable Idempotency-Key for remote tools (V3.4).
// Attempt is intentionally excluded so transport retries share one logical key.
func ToolIdempotencyKey(executionID, stepID string, _ int) string {
	return OperationKey(executionID, stepID)
}
