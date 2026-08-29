package learning

import "context"

// Recorder adapts Service to a narrow RecordUsages(error-only) port.
type Recorder struct {
	Inner *Service
}

// RecordUsages implements a fire-and-forget recorder for context building.
func (r Recorder) RecordUsages(ctx context.Context, in RecordInput) error {
	if r.Inner == nil {
		return nil
	}
	_, err := r.Inner.RecordUsages(ctx, in)
	return err
}
