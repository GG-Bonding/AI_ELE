package experience

import (
	"context"
	"time"
)

// PatternRewardClaim marks that a feedback already credited a Pattern via PatternUsage (V2.1-3).
// V2.1-4 replaces this with a full PatternLearningEvent ledger.
type PatternRewardClaim struct {
	TenantID   string
	FeedbackID string
	PatternID  string
	Reward     float64
	Confidence float64
	Credit     float64
	CreatedAt  time.Time
}

// PatternRewardClaimRepository provides insert-if-absent claims for pattern usage rewards.
type PatternRewardClaimRepository interface {
	// Claim inserts the row. already=true when the (tenant, feedback, pattern) key exists.
	Claim(ctx context.Context, c PatternRewardClaim) (already bool, err error)
}
