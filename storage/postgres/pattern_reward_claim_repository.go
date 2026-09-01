package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/agent-experience-engine/agent-experience-engine/internal/experience"
	"github.com/jackc/pgx/v5/pgconn"
)

// PatternRewardClaimRepository implements experience.PatternRewardClaimRepository.
type PatternRewardClaimRepository struct {
	db *sql.DB
}

// NewPatternRewardClaimRepository constructs a Postgres-backed claim store.
func NewPatternRewardClaimRepository(db *sql.DB) *PatternRewardClaimRepository {
	return &PatternRewardClaimRepository{db: db}
}

func (r *PatternRewardClaimRepository) Claim(ctx context.Context, c experience.PatternRewardClaim) (bool, error) {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO pattern_reward_claims (
			tenant_id, feedback_id, pattern_id, reward, confidence, credit, created_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7)
	`, c.TenantID, c.FeedbackID, c.PatternID, c.Reward, c.Confidence, c.Credit, c.CreatedAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return true, nil
		}
		return false, fmt.Errorf("insert pattern reward claim: %w", err)
	}
	return false, nil
}
