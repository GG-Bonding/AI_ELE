package experience

import (
	"context"
	"sync"
)

// MemoryPatternRewardClaimRepository is an in-memory claim store for tests.
type MemoryPatternRewardClaimRepository struct {
	mu   sync.Mutex
	seen map[string]PatternRewardClaim
}

// NewMemoryPatternRewardClaimRepository constructs an empty claim store.
func NewMemoryPatternRewardClaimRepository() *MemoryPatternRewardClaimRepository {
	return &MemoryPatternRewardClaimRepository{seen: make(map[string]PatternRewardClaim)}
}

func claimKey(tenantID, feedbackID, patternID string) string {
	return tenantID + "/" + feedbackID + "/" + patternID
}

func (m *MemoryPatternRewardClaimRepository) Claim(_ context.Context, c PatternRewardClaim) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	k := claimKey(c.TenantID, c.FeedbackID, c.PatternID)
	if _, ok := m.seen[k]; ok {
		return true, nil
	}
	m.seen[k] = c
	return false, nil
}
