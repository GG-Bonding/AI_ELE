package action

import "context"

// SnapshotFunc adapts a function to ContextLookup.
type SnapshotFunc func(ctx context.Context, tenantID, contextID string) (ContextSnapshot, error)

// GetSnapshot implements ContextLookup.
func (f SnapshotFunc) GetSnapshot(ctx context.Context, tenantID, contextID string) (ContextSnapshot, error) {
	return f(ctx, tenantID, contextID)
}
