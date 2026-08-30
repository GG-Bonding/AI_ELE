package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/agent-experience-engine/agent-experience-engine/internal/experience"
	"github.com/agent-experience-engine/agent-experience-engine/internal/learning"
)

// LearningEventApplier applies learning events in a single Postgres transaction.
type LearningEventApplier struct {
	db *sql.DB
}

// NewLearningEventApplier constructs a transactional learning event applier.
func NewLearningEventApplier(db *sql.DB) *LearningEventApplier {
	return &LearningEventApplier{db: db}
}

func (a *LearningEventApplier) ApplyPendingEvent(ctx context.Context, tenantID string, ev learning.Event) (learning.ApplyResult, error) {
	tx, err := a.db.BeginTx(ctx, nil)
	if err != nil {
		return learning.ApplyResult{}, fmt.Errorf("begin apply tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var (
		status          string
		normalizedReward float64
		confidence      float64
		credit          float64
		experienceID    string
	)
	err = tx.QueryRowContext(ctx, `
		SELECT status, normalized_reward, confidence, credit, experience_id
		FROM learning_events
		WHERE tenant_id = $1 AND id = $2
		FOR UPDATE
	`, tenantID, ev.ID).Scan(&status, &normalizedReward, &confidence, &credit, &experienceID)
	if errors.Is(err, sql.ErrNoRows) {
		return learning.ApplyResult{}, learning.ErrEventNotFound
	}
	if err != nil {
		return learning.ApplyResult{}, fmt.Errorf("lock learning event: %w", err)
	}

	if learning.EventStatus(status) == learning.EventApplied {
		exp, err := getExperienceForUpdate(ctx, tx, tenantID, experienceID)
		if err != nil {
			return learning.ApplyResult{}, err
		}
		return learning.ApplyResult{Experience: exp, OldUtility: exp.Utility, AlreadyApplied: true}, nil
	}

	exp, err := getExperienceForUpdate(ctx, tx, tenantID, experienceID)
	if err != nil {
		return learning.ApplyResult{}, err
	}
	oldUtil := exp.Utility
	expReward := normalizedReward * credit
	now := time.Now().UTC()
	updated, err := experience.ApplyBetaUpdate(exp, expReward, confidence, now)
	if err != nil {
		return learning.ApplyResult{}, fmt.Errorf("beta update experience %s: %w", experienceID, err)
	}
	nextVersion := updated.Version + 1
	res, err := tx.ExecContext(ctx, `
		UPDATE experiences SET
			confidence = $1, utility = $2, alpha = $3, beta = $4,
			success_count = $5, failure_count = $6, use_count = $7,
			status = $8, version = $9, updated_at = $10, last_used_at = $11
		WHERE tenant_id = $12 AND id = $13 AND version = $14
	`,
		updated.Confidence, updated.Utility, updated.Alpha, updated.Beta,
		updated.SuccessCount, updated.FailureCount, updated.UseCount,
		string(updated.Status), nextVersion, updated.UpdatedAt, updated.LastUsedAt,
		tenantID, experienceID, exp.Version,
	)
	if err != nil {
		return learning.ApplyResult{}, fmt.Errorf("update experience: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return learning.ApplyResult{}, fmt.Errorf("rows affected: %w", err)
	}
	if n == 0 {
		return learning.ApplyResult{}, experience.ErrConflict
	}
	updated.Version = nextVersion

	res, err = tx.ExecContext(ctx, `
		UPDATE learning_events SET status = $1, applied_at = $2
		WHERE tenant_id = $3 AND id = $4
	`, string(learning.EventApplied), now, tenantID, ev.ID)
	if err != nil {
		return learning.ApplyResult{}, fmt.Errorf("mark learning event applied: %w", err)
	}
	n, err = res.RowsAffected()
	if err != nil {
		return learning.ApplyResult{}, fmt.Errorf("rows affected: %w", err)
	}
	if n == 0 {
		return learning.ApplyResult{}, learning.ErrEventNotFound
	}

	if err := tx.Commit(); err != nil {
		return learning.ApplyResult{}, fmt.Errorf("commit apply tx: %w", err)
	}
	return learning.ApplyResult{Experience: updated, OldUtility: oldUtil}, nil
}

func getExperienceForUpdate(ctx context.Context, tx *sql.Tx, tenantID, id string) (experience.Experience, error) {
	row := tx.QueryRowContext(ctx, `
		SELECT id, tenant_id, type, scope, scope_key, trigger_text, content, source_episode_id,
		       confidence, utility, alpha, beta, success_count, failure_count, use_count,
		       status, version, supersedes_id, created_at, updated_at, last_used_at, evidence
		FROM experiences
		WHERE tenant_id = $1 AND id = $2
		FOR UPDATE
	`, tenantID, id)
	var exp experience.Experience
	var typ, scope, status string
	var evidenceJSON []byte
	err := row.Scan(
		&exp.ID, &exp.TenantID, &typ, &scope, &exp.ScopeKey, &exp.Trigger, &exp.Content, &exp.SourceEpisodeID,
		&exp.Confidence, &exp.Utility, &exp.Alpha, &exp.Beta, &exp.SuccessCount, &exp.FailureCount, &exp.UseCount,
		&status, &exp.Version, &exp.SupersedesID, &exp.CreatedAt, &exp.UpdatedAt, &exp.LastUsedAt, &evidenceJSON,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return experience.Experience{}, experience.ErrNotFound
	}
	if err != nil {
		return experience.Experience{}, fmt.Errorf("lock experience: %w", err)
	}
	exp.Type = experience.Type(typ)
	exp.Scope = experience.Scope(scope)
	exp.Status = experience.Status(status)
	if len(evidenceJSON) > 0 && string(evidenceJSON) != "null" {
		if err := json.Unmarshal(evidenceJSON, &exp.Evidence); err != nil {
			return experience.Experience{}, fmt.Errorf("decode evidence: %w", err)
		}
	}
	return exp, nil
}
