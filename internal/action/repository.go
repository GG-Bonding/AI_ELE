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
	// ErrDuplicatePatternLink is returned when the same pattern→action link already exists.
	ErrDuplicatePatternLink = errors.New("duplicate pattern-action link")
	// ErrEpisodeNotFound is returned when the parent episode is missing.
	ErrEpisodeNotFound = errors.New("episode not found")
	// ErrContextNotFound is returned when a referenced context snapshot is missing.
	ErrContextNotFound = errors.New("context snapshot not found")
)

// EpisodeChecker verifies the episode exists for the tenant.
type EpisodeChecker interface {
	EpisodeExists(ctx context.Context, tenantID, episodeID string) (bool, error)
}

// ContextLookup loads a context snapshot for automatic provenance linking (V2.2-2).
type ContextLookup interface {
	GetSnapshot(ctx context.Context, tenantID, contextID string) (ContextSnapshot, error)
}

// ContextSnapshot is the subset of a context build needed for Action provenance.
type ContextSnapshot struct {
	ID            string
	TenantID      string
	EpisodeID     string
	ExperienceIDs []string
	PatternIDs    []string
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

	CreatePatternLink(ctx context.Context, link PatternActionLink) (PatternActionLink, error)
	ListPatternLinksByEpisode(ctx context.Context, tenantID, episodeID string) ([]PatternActionLink, error)
	ListPatternLinksByAction(ctx context.Context, tenantID, actionID string) ([]PatternActionLink, error)
}
