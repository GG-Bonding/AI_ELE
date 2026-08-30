package action

import (
	"context"
	"errors"
)

var (
	// ErrNotFound is returned when an action or link is missing for the tenant.
	ErrNotFound = errors.New("action not found")
	// ErrInvalidInput is returned for validation failures.
	ErrInvalidInput = errors.New("invalid input")
	// ErrDuplicateLink is returned when the same experience→action link already exists.
	ErrDuplicateLink = errors.New("duplicate experience-action link")
	// ErrEpisodeNotFound is returned when the parent episode is missing.
	ErrEpisodeNotFound = errors.New("episode not found")
)

// EpisodeChecker verifies the episode exists for the tenant.
type EpisodeChecker interface {
	EpisodeExists(ctx context.Context, tenantID, episodeID string) (bool, error)
}

// Repository persists agent actions and experience→action links.
// Implementations must enforce tenant isolation on every read/write.
type Repository interface {
	CreateAction(ctx context.Context, a AgentAction) (AgentAction, error)
	GetAction(ctx context.Context, tenantID, actionID string) (AgentAction, error)
	ListActionsByEpisode(ctx context.Context, tenantID, episodeID string) ([]AgentAction, error)
	NextActionSequence(ctx context.Context, tenantID, episodeID string) (int, error)

	CreateLink(ctx context.Context, link ExperienceActionLink) (ExperienceActionLink, error)
	ListLinksByEpisode(ctx context.Context, tenantID, episodeID string) ([]ExperienceActionLink, error)
	ListLinksByAction(ctx context.Context, tenantID, actionID string) ([]ExperienceActionLink, error)
}
