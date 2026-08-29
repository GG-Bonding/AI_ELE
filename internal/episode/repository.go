package episode

import (
	"context"
	"errors"

	"github.com/agent-experience-engine/agent-experience-engine/internal/attempt"
	"github.com/agent-experience-engine/agent-experience-engine/internal/outcome"
)

var (
	// ErrNotFound is returned when an episode (or related row) is missing for the tenant.
	ErrNotFound = errors.New("episode not found")
	// ErrAlreadyCompleted is returned when mutating a terminal episode.
	ErrAlreadyCompleted = errors.New("episode already completed")
	// ErrInvalidInput is returned for validation failures.
	ErrInvalidInput = errors.New("invalid input")
	// ErrOutcomeExists is returned when an outcome is already recorded.
	ErrOutcomeExists = errors.New("outcome already exists")
)

// Repository persists episodes, attempts, and outcomes.
// Implementations must enforce tenant isolation on every read/write.
type Repository interface {
	CreateEpisode(ctx context.Context, ep Episode) (Episode, error)
	GetEpisode(ctx context.Context, tenantID, id string) (Episode, error)
	UpdateEpisode(ctx context.Context, ep Episode) (Episode, error)

	CreateAttempt(ctx context.Context, a attempt.Attempt) (attempt.Attempt, error)
	ListAttempts(ctx context.Context, tenantID, episodeID string) ([]attempt.Attempt, error)
	NextAttemptSequence(ctx context.Context, tenantID, episodeID string) (int, error)

	CreateOutcome(ctx context.Context, o outcome.Outcome) (outcome.Outcome, error)
	GetOutcome(ctx context.Context, tenantID, episodeID string) (outcome.Outcome, error)
}
