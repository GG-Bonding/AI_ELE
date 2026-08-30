package learning

import (
	"context"
	"errors"
	"time"
)

var (
	// ErrDuplicateEvent is returned when a learning event for feedback+experience already exists.
	ErrDuplicateEvent = errors.New("learning event already exists")
	// ErrEventNotFound is returned when a learning event is missing.
	ErrEventNotFound = errors.New("learning event not found")
)

// EventStatus tracks apply progress for recoverable learning.
type EventStatus string

const (
	EventPending EventStatus = "PENDING"
	EventApplied EventStatus = "APPLIED"
	EventFailed  EventStatus = "FAILED"
)

// Event is one incremental utility update derived from a single feedback row.
type Event struct {
	ID               string
	TenantID         string
	FeedbackID       string
	EpisodeID        string
	ExperienceID     string
	NormalizedReward float64
	Confidence       float64
	Credit           float64
	EffectiveReward  float64
	Status           EventStatus
	CreatedAt        time.Time
	AppliedAt        *time.Time
}

// EventRepository persists learning events for idempotent, recoverable updates.
type EventRepository interface {
	Create(ctx context.Context, ev Event) (Event, error)
	GetByFeedbackExperience(ctx context.Context, tenantID, feedbackID, experienceID string) (Event, error)
	ListByFeedback(ctx context.Context, tenantID, feedbackID string) ([]Event, error)
	MarkApplied(ctx context.Context, tenantID, id string, appliedAt time.Time) error
	MarkFailed(ctx context.Context, tenantID, id string) error
}
