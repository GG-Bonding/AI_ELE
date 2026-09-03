package learning

import (
	"context"
	"errors"
	"time"

	"github.com/agent-experience-engine/agent-experience-engine/internal/experience"
)

var (
	// ErrDuplicatePatternEvent is returned when a pattern learning event key already exists.
	ErrDuplicatePatternEvent = errors.New("pattern learning event already exists")
	// ErrPatternEventNotFound is returned when a pattern learning event is missing.
	ErrPatternEventNotFound = errors.New("pattern learning event not found")
)

// PatternEventSource identifies how a PatternLearningEvent was derived.
type PatternEventSource string

const (
	// PatternSourceMember is derived from an applied experience LearningEvent.
	PatternSourceMember PatternEventSource = "MEMBER_EXPERIENCE"
	// PatternSourceUsage is derived from PatternUsage for episode-level feedback.
	PatternSourceUsage PatternEventSource = "PATTERN_USAGE"
	// PatternSourceDirect is an explicit pattern reward (HTTP / API).
	PatternSourceDirect PatternEventSource = "DIRECT"
)

// PatternEvent is one incremental Pattern utility update (V2.1-4).
type PatternEvent struct {
	ID                    string
	TenantID              string
	FeedbackID            string
	EpisodeID             string
	PatternID             string
	SourceType            PatternEventSource
	SourceLearningEventID string // set for MEMBER_EXPERIENCE
	NormalizedReward      float64
	Confidence            float64
	Credit                float64
	EffectiveReward       float64
	Status                EventStatus
	CreatedAt             time.Time
	AppliedAt             *time.Time
}

// PatternEventRepository persists pattern learning events for exactly-once apply.
type PatternEventRepository interface {
	Create(ctx context.Context, ev PatternEvent) (PatternEvent, error)
	GetByMemberSource(ctx context.Context, tenantID, sourceLearningEventID, patternID string) (PatternEvent, error)
	GetByFeedbackPatternSource(ctx context.Context, tenantID, feedbackID, patternID string, source PatternEventSource) (PatternEvent, error)
	ListByFeedback(ctx context.Context, tenantID, feedbackID string) ([]PatternEvent, error)
	MarkApplied(ctx context.Context, tenantID, id string, appliedAt time.Time) error
	MarkFailed(ctx context.Context, tenantID, id string) error
}

// PatternApplyResult is the outcome of applying one pattern learning event.
type PatternApplyResult struct {
	Pattern        experience.Pattern
	OldUtility     float64
	AlreadyApplied bool
}

// PatternEventApplier atomically applies pattern utility updates and marks events APPLIED.
type PatternEventApplier interface {
	ApplyPendingPatternEvent(ctx context.Context, tenantID string, ev PatternEvent) (PatternApplyResult, error)
}
