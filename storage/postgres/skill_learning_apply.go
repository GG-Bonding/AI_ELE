package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/agent-experience-engine/agent-experience-engine/internal/skill"
)

// SkillLearningEventApplier applies skill learning events in one Postgres transaction.
type SkillLearningEventApplier struct {
	db *sql.DB
}

// NewSkillLearningEventApplier constructs a transactional skill learning applier.
func NewSkillLearningEventApplier(db *sql.DB) *SkillLearningEventApplier {
	return &SkillLearningEventApplier{db: db}
}

// ApplyPending implements skill.LearningApplier.
func (a *SkillLearningEventApplier) ApplyPending(ctx context.Context, tenantID, eventID string) (skill.LearningApplyResult, error) {
	tx, err := a.db.BeginTx(ctx, nil)
	if err != nil {
		return skill.LearningApplyResult{}, fmt.Errorf("begin skill learning tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var (
		status, versionID string
		reward, confidence, credit float64
	)
	err = tx.QueryRowContext(ctx, `
		SELECT status, skill_version_id, reward, confidence, credit
		FROM skill_learning_events
		WHERE tenant_id = $1 AND id = $2
		FOR UPDATE
	`, tenantID, eventID).Scan(&status, &versionID, &reward, &confidence, &credit)
	if errors.Is(err, sql.ErrNoRows) {
		return skill.LearningApplyResult{}, skill.ErrLearningNotFound
	}
	if err != nil {
		return skill.LearningApplyResult{}, fmt.Errorf("lock skill learning event: %w", err)
	}

	ver, err := getSkillVersionForUpdate(ctx, tx, tenantID, versionID)
	if err != nil {
		return skill.LearningApplyResult{}, err
	}
	if status == "APPLIED" {
		return skill.LearningApplyResult{Version: ver, AlreadyApplied: true}, nil
	}

	expReward := reward * credit
	if credit == 0 {
		expReward = reward
	}
	updated, err := skill.ApplyBetaUpdate(ver, expReward, confidence)
	if err != nil {
		_, _ = tx.ExecContext(ctx, `
			UPDATE skill_learning_events SET status='FAILED' WHERE tenant_id=$1 AND id=$2
		`, tenantID, eventID)
		_ = tx.Commit()
		return skill.LearningApplyResult{}, err
	}

	_, err = tx.ExecContext(ctx, `
		UPDATE skill_versions SET
			utility=$3, alpha=$4, beta=$5, success_count=$6, failure_count=$7
		WHERE tenant_id=$1 AND id=$2
	`, tenantID, versionID, updated.Utility, updated.Alpha, updated.Beta, updated.SuccessCount, updated.FailureCount)
	if err != nil {
		return skill.LearningApplyResult{}, fmt.Errorf("update skill version: %w", err)
	}

	now := time.Now().UTC()
	_, err = tx.ExecContext(ctx, `
		UPDATE skill_learning_events SET status='APPLIED', applied_at=$3
		WHERE tenant_id=$1 AND id=$2
	`, tenantID, eventID, now)
	if err != nil {
		return skill.LearningApplyResult{}, fmt.Errorf("mark skill learning applied: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return skill.LearningApplyResult{}, fmt.Errorf("commit skill learning tx: %w", err)
	}
	return skill.LearningApplyResult{Version: updated}, nil
}

func getSkillVersionForUpdate(ctx context.Context, tx *sql.Tx, tenantID, id string) (skill.Version, error) {
	row := tx.QueryRowContext(ctx, `
		SELECT id, skill_id, tenant_id, version, COALESCE(pattern_id, ''),
		       spec_json, spec_yaml, spec_hash, confidence, utility,
		       COALESCE(alpha,1), COALESCE(beta,1), COALESCE(success_count,0), COALESCE(failure_count,0),
		       COALESCE(shadow_successes,0), COALESCE(shadow_failures,0),
		       status, validation_status, created_at
		FROM skill_versions WHERE tenant_id = $1 AND id = $2
		FOR UPDATE
	`, tenantID, id)
	ver, err := scanSkillVersion(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return skill.Version{}, skill.ErrNotFound
		}
		return skill.Version{}, fmt.Errorf("lock skill version: %w", err)
	}
	return ver, nil
}

var _ skill.LearningApplier = (*SkillLearningEventApplier)(nil)
