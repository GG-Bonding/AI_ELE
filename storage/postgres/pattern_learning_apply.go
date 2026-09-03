package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/agent-experience-engine/agent-experience-engine/internal/experience"
	"github.com/agent-experience-engine/agent-experience-engine/internal/learning"
)

// PatternLearningEventApplier applies pattern learning events in one Postgres transaction.
type PatternLearningEventApplier struct {
	db *sql.DB
}

// NewPatternLearningEventApplier constructs a transactional pattern event applier.
func NewPatternLearningEventApplier(db *sql.DB) *PatternLearningEventApplier {
	return &PatternLearningEventApplier{db: db}
}

func (a *PatternLearningEventApplier) ApplyPendingPatternEvent(ctx context.Context, tenantID string, ev learning.PatternEvent) (learning.PatternApplyResult, error) {
	tx, err := a.db.BeginTx(ctx, nil)
	if err != nil {
		return learning.PatternApplyResult{}, fmt.Errorf("begin pattern apply tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var (
		status           string
		normalizedReward float64
		confidence       float64
		credit           float64
		patternID        string
	)
	err = tx.QueryRowContext(ctx, `
		SELECT status, normalized_reward, confidence, credit, pattern_id
		FROM pattern_learning_events
		WHERE tenant_id = $1 AND id = $2
		FOR UPDATE
	`, tenantID, ev.ID).Scan(&status, &normalizedReward, &confidence, &credit, &patternID)
	if errors.Is(err, sql.ErrNoRows) {
		return learning.PatternApplyResult{}, learning.ErrPatternEventNotFound
	}
	if err != nil {
		return learning.PatternApplyResult{}, fmt.Errorf("lock pattern learning event: %w", err)
	}

	if learning.EventStatus(status) == learning.EventApplied {
		p, err := getPatternForUpdate(ctx, tx, tenantID, patternID)
		if err != nil {
			return learning.PatternApplyResult{}, err
		}
		return learning.PatternApplyResult{Pattern: p, OldUtility: p.Utility, AlreadyApplied: true}, nil
	}

	p, err := getPatternForUpdate(ctx, tx, tenantID, patternID)
	if err != nil {
		return learning.PatternApplyResult{}, err
	}
	oldUtil := p.Utility
	patReward := normalizedReward * credit
	now := time.Now().UTC()
	updated, err := experience.ApplyPatternBetaUpdate(p, patReward, confidence, now)
	if err != nil {
		return learning.PatternApplyResult{}, fmt.Errorf("beta update pattern %s: %w", patternID, err)
	}
	updated = experience.MaybePromotePattern(updated)

	res, err := tx.ExecContext(ctx, `
		UPDATE patterns SET
			confidence = $1, utility = $2, alpha = $3, beta = $4,
			success_count = $5, failure_count = $6, support_count = $7,
			status = $8, updated_at = $9
		WHERE tenant_id = $10 AND id = $11
	`,
		updated.Confidence, updated.Utility, updated.Alpha, updated.Beta,
		updated.SuccessCount, updated.FailureCount, updated.SupportCount,
		string(updated.Status), updated.UpdatedAt,
		tenantID, patternID,
	)
	if err != nil {
		return learning.PatternApplyResult{}, fmt.Errorf("update pattern: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return learning.PatternApplyResult{}, fmt.Errorf("rows affected: %w", err)
	}
	if n == 0 {
		return learning.PatternApplyResult{}, experience.ErrNotFound
	}

	res, err = tx.ExecContext(ctx, `
		UPDATE pattern_learning_events SET status = $1, applied_at = $2
		WHERE tenant_id = $3 AND id = $4
	`, string(learning.EventApplied), now, tenantID, ev.ID)
	if err != nil {
		return learning.PatternApplyResult{}, fmt.Errorf("mark pattern learning event applied: %w", err)
	}
	n, err = res.RowsAffected()
	if err != nil {
		return learning.PatternApplyResult{}, fmt.Errorf("rows affected: %w", err)
	}
	if n == 0 {
		return learning.PatternApplyResult{}, learning.ErrPatternEventNotFound
	}

	if err := tx.Commit(); err != nil {
		return learning.PatternApplyResult{}, fmt.Errorf("commit pattern apply tx: %w", err)
	}
	return learning.PatternApplyResult{Pattern: updated, OldUtility: oldUtil}, nil
}

func getPatternForUpdate(ctx context.Context, tx *sql.Tx, tenantID, id string) (experience.Pattern, error) {
	row := tx.QueryRowContext(ctx, `
		SELECT id, tenant_id, type, scope, scope_key, trigger_text, content,
		       confidence, utility, alpha, beta, success_count, failure_count,
		       support_count, status, created_at, updated_at
		FROM patterns
		WHERE tenant_id = $1 AND id = $2
		FOR UPDATE
	`, tenantID, id)
	var p experience.Pattern
	var typ, scope, status string
	err := row.Scan(
		&p.ID, &p.TenantID, &typ, &scope, &p.ScopeKey, &p.Trigger, &p.Content,
		&p.Confidence, &p.Utility, &p.Alpha, &p.Beta, &p.SuccessCount, &p.FailureCount,
		&p.SupportCount, &status, &p.CreatedAt, &p.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return experience.Pattern{}, experience.ErrNotFound
	}
	if err != nil {
		return experience.Pattern{}, fmt.Errorf("lock pattern: %w", err)
	}
	p.Type = experience.Type(typ)
	p.Scope = experience.Scope(scope)
	p.Status = experience.PatternStatus(status)
	return p, nil
}
