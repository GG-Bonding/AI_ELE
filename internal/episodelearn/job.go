package episodelearn

import (
	"context"
	"time"
)

// Status tracks episode learning job progress.
type Status string

const (
	StatusPending    Status = "PENDING"
	StatusProcessing Status = "PROCESSING"
	StatusApplied    Status = "APPLIED"
	StatusFailed     Status = "FAILED"
)

// Job is the learning pipeline state for a completed episode.
type Job struct {
	ID        string    `json:"id"`
	TenantID  string    `json:"tenant_id"`
	EpisodeID string    `json:"episode_id"`
	Status    Status    `json:"status"`
	LastError string    `json:"last_error,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Repository persists episode learning jobs.
type Repository interface {
	UpsertPending(ctx context.Context, tenantID, episodeID string) (Job, error)
	GetByEpisode(ctx context.Context, tenantID, episodeID string) (Job, error)
	MarkProcessing(ctx context.Context, tenantID, episodeID string) error
	MarkApplied(ctx context.Context, tenantID, episodeID string) error
	MarkFailed(ctx context.Context, tenantID, episodeID, lastError string) error
}
