package feedback

import (
	"errors"
	"time"
)

var (
	// ErrNotFound is returned when feedback rows are missing for a tenant/episode.
	ErrNotFound = errors.New("feedback not found")
	// ErrInvalidInput is returned for validation failures.
	ErrInvalidInput = errors.New("invalid input")
	// ErrDuplicateIdempotency is returned when creating a feedback with a reused key.
	ErrDuplicateIdempotency = errors.New("duplicate feedback idempotency key")
)

// Source identifies who produced the feedback signal.
type Source string

const (
	SourceUserExplicit Source = "USER_EXPLICIT"
	SourceUserImplicit Source = "USER_IMPLICIT"
	SourceTool         Source = "TOOL"
	SourceBusiness     Source = "BUSINESS"
	SourceHumanReview  Source = "HUMAN_REVIEW"
	SourceLLMJudge     Source = "LLM_JUDGE"
)

// Valid reports whether s is a known feedback source.
func (s Source) Valid() bool {
	switch s {
	case SourceUserExplicit, SourceUserImplicit, SourceTool, SourceBusiness, SourceHumanReview, SourceLLMJudge:
		return true
	default:
		return false
	}
}

// Feedback is one raw environment/user signal about an episode.
// Raw rows are always persisted; never only the aggregated reward.
type Feedback struct {
	ID             string    `json:"id"`
	TenantID       string    `json:"tenant_id"`
	EpisodeID      string    `json:"episode_id"`
	Source         Source    `json:"source"`
	Signal         string    `json:"signal,omitempty"`
	Reward         float64   `json:"reward"`
	Confidence     float64   `json:"confidence"`
	Evidence       string    `json:"evidence,omitempty"`
	Target         *Target   `json:"target,omitempty"`
	IdempotencyKey string    `json:"idempotency_key,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
}

// EpisodeReward is the weighted aggregation over raw feedback for one episode.
type EpisodeReward struct {
	TenantID       string  `json:"tenant_id"`
	EpisodeID      string  `json:"episode_id"`
	WeightedReward float64 `json:"weighted_reward"`
	TotalWeight    float64 `json:"total_weight"`
	FeedbackCount  int     `json:"feedback_count"`
}
