package learning

import (
	"context"

	"github.com/agent-experience-engine/agent-experience-engine/internal/experience"
)

// ApplyResult is the outcome of atomically applying one learning event.
type ApplyResult struct {
	Experience experience.Experience
	OldUtility float64
	AlreadyApplied bool
}

// EventApplier atomically applies utility updates and marks learning events APPLIED.
type EventApplier interface {
	ApplyPendingEvent(ctx context.Context, tenantID string, ev Event) (ApplyResult, error)
}
