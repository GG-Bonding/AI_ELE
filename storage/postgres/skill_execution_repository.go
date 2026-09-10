package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/agent-experience-engine/agent-experience-engine/internal/skill"
)

// SkillExecutionRepository persists skill executions / steps / approvals / learning.
type SkillExecutionRepository struct {
	db *sql.DB
}

// NewSkillExecutionRepository constructs a Postgres execution ledger.
func NewSkillExecutionRepository(db *sql.DB) *SkillExecutionRepository {
	return &SkillExecutionRepository{db: db}
}

func (r *SkillExecutionRepository) CreateExecution(ctx context.Context, ex skill.Execution) (skill.Execution, error) {
	in, _ := json.Marshal(ex.Inputs)
	out, _ := json.Marshal(ex.Outputs)
	var idem any
	if strings.TrimSpace(ex.IdempotencyKey) != "" {
		idem = ex.IdempotencyKey
	}
	if ex.LeaseEpoch == 0 {
		ex.LeaseEpoch = 1
	}
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO skill_executions (
			id, tenant_id, episode_id, skill_id, skill_version_id, mode, status,
			idempotency_key, inputs, outputs, error_code, error_message, started_at, completed_at,
			step_cursor, lease_owner, lease_until, heartbeat_at, requester_id, spec_snapshot, lease_epoch
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21)
	`, ex.ID, ex.TenantID, nullStr(ex.EpisodeID), ex.SkillID, ex.SkillVersionID, string(ex.Mode), string(ex.Status),
		idem, in, out, ex.ErrorCode, ex.ErrorMessage, ex.StartedAt, ex.CompletedAt,
		ex.StepCursor, ex.LeaseOwner, ex.LeaseUntil, ex.HeartbeatAt, ex.RequesterID, ex.SpecSnapshot, ex.LeaseEpoch)
	if err != nil {
		if strings.TrimSpace(ex.IdempotencyKey) != "" &&
			(strings.Contains(err.Error(), "idx_skill_executions_idempotency") ||
				strings.Contains(err.Error(), "duplicate key") ||
				strings.Contains(err.Error(), "unique")) {
			existing, getErr := r.GetExecutionByIdempotency(ctx, ex.TenantID, ex.IdempotencyKey)
			if getErr == nil {
				return existing, nil
			}
		}
		return skill.Execution{}, fmt.Errorf("insert skill execution: %w", err)
	}
	return ex, nil
}

func (r *SkillExecutionRepository) UpdateExecution(ctx context.Context, ex skill.Execution) (skill.Execution, error) {
	out, _ := json.Marshal(ex.Outputs)
	// Refuse unfenced mutation while RUNNING (V3.4) — lease_epoch is not rewritten here either.
	res, err := r.db.ExecContext(ctx, `
		UPDATE skill_executions
		SET status=$3, outputs=$4, error_code=$5, error_message=$6, completed_at=$7,
		    step_cursor=$8, lease_owner=$9, lease_until=$10, heartbeat_at=$11,
		    requester_id=$12, spec_snapshot=$13
		WHERE tenant_id=$1 AND id=$2 AND status <> 'RUNNING'
	`, ex.TenantID, ex.ID, string(ex.Status), out, ex.ErrorCode, ex.ErrorMessage, ex.CompletedAt,
		ex.StepCursor, ex.LeaseOwner, ex.LeaseUntil, ex.HeartbeatAt, ex.RequesterID, ex.SpecSnapshot)
	if err != nil {
		return skill.Execution{}, err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		cur, getErr := r.GetExecution(ctx, ex.TenantID, ex.ID)
		if getErr != nil {
			return skill.Execution{}, getErr
		}
		if cur.Status == skill.ExecRunning {
			return skill.Execution{}, fmt.Errorf("%w: use UpdateExecutionFenced while RUNNING", skill.ErrInvalidTransition)
		}
		return skill.Execution{}, skill.ErrNotFound
	}
	return ex, nil
}

func (r *SkillExecutionRepository) UpdateExecutionFenced(ctx context.Context, ex skill.Execution, expectedEpoch int64) (skill.Execution, bool, error) {
	out, _ := json.Marshal(ex.Outputs)
	res, err := r.db.ExecContext(ctx, `
		UPDATE skill_executions
		SET status=$3, outputs=$4, error_code=$5, error_message=$6, completed_at=$7,
		    step_cursor=$8, lease_owner=$9, lease_until=$10, heartbeat_at=$11,
		    requester_id=$12, spec_snapshot=$13
		WHERE tenant_id=$1 AND id=$2 AND lease_epoch=$14
	`, ex.TenantID, ex.ID, string(ex.Status), out, ex.ErrorCode, ex.ErrorMessage, ex.CompletedAt,
		ex.StepCursor, ex.LeaseOwner, ex.LeaseUntil, ex.HeartbeatAt, ex.RequesterID, ex.SpecSnapshot, expectedEpoch)
	if err != nil {
		return skill.Execution{}, false, err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		cur, getErr := r.GetExecution(ctx, ex.TenantID, ex.ID)
		return cur, false, getErr
	}
	ex.LeaseEpoch = expectedEpoch
	return ex, true, nil
}

func (r *SkillExecutionRepository) GetExecution(ctx context.Context, tenantID, id string) (skill.Execution, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, tenant_id, COALESCE(episode_id,''), skill_id, skill_version_id, mode, status,
		       COALESCE(idempotency_key,''), inputs, outputs, error_code, error_message, started_at, completed_at,
		       step_cursor, COALESCE(lease_owner,''), lease_until, heartbeat_at, COALESCE(requester_id,''), COALESCE(spec_snapshot,''),
		       COALESCE(lease_epoch,0)
		FROM skill_executions WHERE tenant_id=$1 AND id=$2
	`, tenantID, id)
	return scanExecution(row)
}

func (r *SkillExecutionRepository) GetExecutionByIdempotency(ctx context.Context, tenantID, key string) (skill.Execution, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, tenant_id, COALESCE(episode_id,''), skill_id, skill_version_id, mode, status,
		       COALESCE(idempotency_key,''), inputs, outputs, error_code, error_message, started_at, completed_at,
		       step_cursor, COALESCE(lease_owner,''), lease_until, heartbeat_at, COALESCE(requester_id,''), COALESCE(spec_snapshot,''),
		       COALESCE(lease_epoch,0)
		FROM skill_executions WHERE tenant_id=$1 AND idempotency_key=$2
	`, tenantID, key)
	return scanExecution(row)
}

func (r *SkillExecutionRepository) CreateStep(ctx context.Context, st skill.StepExecution) (skill.StepExecution, error) {
	in, _ := json.Marshal(st.Input)
	out, _ := json.Marshal(st.Output)
	if st.Attempt <= 0 {
		st.Attempt = 1
	}
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO skill_step_executions (
			id, execution_id, tenant_id, step_id, tool, input, output, status, error_code, duration_ms, sequence,
			attempt, operation_key, lease_epoch
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)
	`, st.ID, st.ExecutionID, st.TenantID, st.StepID, st.Tool, in, out, string(st.Status), st.ErrorCode, st.DurationMs, st.Sequence,
		st.Attempt, st.OperationKey, st.LeaseEpoch)
	if err != nil {
		return skill.StepExecution{}, err
	}
	return st, nil
}

func (r *SkillExecutionRepository) UpdateStep(ctx context.Context, st skill.StepExecution) (skill.StepExecution, error) {
	in, _ := json.Marshal(st.Input)
	out, _ := json.Marshal(st.Output)
	res, err := r.db.ExecContext(ctx, `
		UPDATE skill_step_executions
		SET input=$4, output=$5, status=$6, error_code=$7, duration_ms=$8, attempt=$9, operation_key=$10, lease_epoch=$11
		WHERE tenant_id=$1 AND execution_id=$2 AND id=$3
	`, st.TenantID, st.ExecutionID, st.ID, in, out, string(st.Status), st.ErrorCode, st.DurationMs, st.Attempt, st.OperationKey, st.LeaseEpoch)
	if err != nil {
		return skill.StepExecution{}, err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return skill.StepExecution{}, skill.ErrNotFound
	}
	return st, nil
}

func (r *SkillExecutionRepository) UpdateStepFenced(ctx context.Context, st skill.StepExecution, expectedEpoch int64) (skill.StepExecution, bool, error) {
	in, _ := json.Marshal(st.Input)
	out, _ := json.Marshal(st.Output)
	res, err := r.db.ExecContext(ctx, `
		UPDATE skill_step_executions s
		SET input=$4, output=$5, status=$6, error_code=$7, duration_ms=$8, attempt=$9, operation_key=$10, lease_epoch=$11
		FROM skill_executions e
		WHERE s.tenant_id=$1 AND s.execution_id=$2 AND s.id=$3
		  AND e.tenant_id=s.tenant_id AND e.id=s.execution_id
		  AND e.lease_epoch=$11
	`, st.TenantID, st.ExecutionID, st.ID, in, out, string(st.Status), st.ErrorCode, st.DurationMs, st.Attempt, st.OperationKey, expectedEpoch)
	if err != nil {
		return skill.StepExecution{}, false, err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return st, false, nil
	}
	st.LeaseEpoch = expectedEpoch
	return st, true, nil
}

func (r *SkillExecutionRepository) RenewLease(ctx context.Context, tenantID, executionID, owner string, expectedEpoch int64, until time.Time) (bool, error) {
	res, err := r.db.ExecContext(ctx, `
		UPDATE skill_executions
		SET lease_until=$4, heartbeat_at=NOW(), lease_owner=COALESCE(NULLIF($3,''), lease_owner)
		WHERE tenant_id=$1 AND id=$2 AND lease_epoch=$5 AND status='RUNNING'
	`, tenantID, executionID, owner, until, expectedEpoch)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

func (r *SkillExecutionRepository) ListSteps(ctx context.Context, tenantID, executionID string) ([]skill.StepExecution, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, execution_id, tenant_id, step_id, tool, input, output, status, error_code, duration_ms, sequence,
		       COALESCE(attempt,1), COALESCE(operation_key,''), COALESCE(lease_epoch,0)
		FROM skill_step_executions WHERE tenant_id=$1 AND execution_id=$2 ORDER BY sequence ASC, attempt ASC
	`, tenantID, executionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []skill.StepExecution
	for rows.Next() {
		st, err := scanStep(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, st)
	}
	return out, rows.Err()
}

func (r *SkillExecutionRepository) CreateApproval(ctx context.Context, req skill.ApprovalRequest) (skill.ApprovalRequest, error) {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO skill_approval_requests (id, tenant_id, execution_id, skill_id, status, reason, created_at, resolved_at, requester_id, approved_by)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
	`, req.ID, req.TenantID, req.ExecutionID, req.SkillID, string(req.Status), req.Reason, req.CreatedAt, req.ResolvedAt, req.RequesterID, req.ApprovedBy)
	if err != nil {
		return skill.ApprovalRequest{}, err
	}
	return req, nil
}

func (r *SkillExecutionRepository) UpdateApproval(ctx context.Context, req skill.ApprovalRequest) (skill.ApprovalRequest, error) {
	res, err := r.db.ExecContext(ctx, `
		UPDATE skill_approval_requests SET status=$3, reason=$4, resolved_at=$5, approved_by=$6
		WHERE tenant_id=$1 AND id=$2
	`, req.TenantID, req.ID, string(req.Status), req.Reason, req.ResolvedAt, req.ApprovedBy)
	if err != nil {
		return skill.ApprovalRequest{}, err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return skill.ApprovalRequest{}, skill.ErrNotFound
	}
	return req, nil
}

func (r *SkillExecutionRepository) GetApproval(ctx context.Context, tenantID, id string) (skill.ApprovalRequest, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, tenant_id, execution_id, skill_id, status, reason, created_at, resolved_at,
		       COALESCE(requester_id,''), COALESCE(approved_by,'')
		FROM skill_approval_requests WHERE tenant_id=$1 AND id=$2
	`, tenantID, id)
	return scanApproval(row)
}

func (r *SkillExecutionRepository) GetApprovalByExecution(ctx context.Context, tenantID, executionID string) (skill.ApprovalRequest, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, tenant_id, execution_id, skill_id, status, reason, created_at, resolved_at,
		       COALESCE(requester_id,''), COALESCE(approved_by,'')
		FROM skill_approval_requests
		WHERE tenant_id=$1 AND execution_id=$2
		ORDER BY created_at DESC
		LIMIT 1
	`, tenantID, executionID)
	return scanApproval(row)
}

func scanApproval(row execScanner) (skill.ApprovalRequest, error) {
	var req skill.ApprovalRequest
	var status string
	var resolved sql.NullTime
	if err := row.Scan(&req.ID, &req.TenantID, &req.ExecutionID, &req.SkillID, &status, &req.Reason, &req.CreatedAt, &resolved,
		&req.RequesterID, &req.ApprovedBy); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return skill.ApprovalRequest{}, skill.ErrNotFound
		}
		return skill.ApprovalRequest{}, err
	}
	req.Status = skill.ApprovalStatus(status)
	if resolved.Valid {
		t := resolved.Time
		req.ResolvedAt = &t
	}
	return req, nil
}

func (r *SkillExecutionRepository) CreateLearningEvent(ctx context.Context, ev skill.LearningEvent) (skill.LearningEvent, error) {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO skill_learning_events (
			id, tenant_id, skill_id, skill_version_id, execution_id, feedback_id,
			reward, confidence, credit, status, created_at, applied_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
	`, ev.ID, ev.TenantID, ev.SkillID, ev.SkillVersionID, nullStr(ev.ExecutionID), ev.FeedbackID,
		ev.Reward, ev.Confidence, ev.Credit, ev.Status, ev.CreatedAt, ev.AppliedAt)
	if err != nil {
		if strings.Contains(err.Error(), "skill_learning_events_feedback_version_unique") {
			return skill.LearningEvent{}, skill.ErrDuplicateLearning
		}
		return skill.LearningEvent{}, err
	}
	return ev, nil
}

func (r *SkillExecutionRepository) GetLearningEvent(ctx context.Context, tenantID, id string) (skill.LearningEvent, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, tenant_id, skill_id, skill_version_id, COALESCE(execution_id,''), feedback_id,
		       reward, confidence, credit, status, created_at, applied_at
		FROM skill_learning_events WHERE tenant_id=$1 AND id=$2
	`, tenantID, id)
	return scanSkillLearningEvent(row)
}

func (r *SkillExecutionRepository) GetLearningEventByFeedbackVersion(ctx context.Context, tenantID, feedbackID, versionID string) (skill.LearningEvent, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, tenant_id, skill_id, skill_version_id, COALESCE(execution_id,''), feedback_id,
		       reward, confidence, credit, status, created_at, applied_at
		FROM skill_learning_events
		WHERE tenant_id=$1 AND feedback_id=$2 AND skill_version_id=$3
	`, tenantID, feedbackID, versionID)
	return scanSkillLearningEvent(row)
}

func scanSkillLearningEvent(row execScanner) (skill.LearningEvent, error) {
	var ev skill.LearningEvent
	var applied sql.NullTime
	if err := row.Scan(&ev.ID, &ev.TenantID, &ev.SkillID, &ev.SkillVersionID, &ev.ExecutionID, &ev.FeedbackID,
		&ev.Reward, &ev.Confidence, &ev.Credit, &ev.Status, &ev.CreatedAt, &applied); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return skill.LearningEvent{}, skill.ErrLearningNotFound
		}
		return skill.LearningEvent{}, err
	}
	if applied.Valid {
		t := applied.Time
		ev.AppliedAt = &t
	}
	return ev, nil
}

func (r *SkillExecutionRepository) MarkLearningApplied(ctx context.Context, tenantID, id string, at time.Time) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE skill_learning_events SET status='APPLIED', applied_at=$3 WHERE tenant_id=$1 AND id=$2
	`, tenantID, id, at)
	return err
}

func (r *SkillExecutionRepository) MarkLearningFailed(ctx context.Context, tenantID, id string) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE skill_learning_events SET status='FAILED' WHERE tenant_id=$1 AND id=$2
	`, tenantID, id)
	return err
}

func nullStr(s string) any {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return s
}

type execScanner interface {
	Scan(dest ...any) error
}

func scanExecution(row execScanner) (skill.Execution, error) {
	var ex skill.Execution
	var mode, status string
	var inRaw, outRaw []byte
	var completed, leaseUntil, heartbeat sql.NullTime
	if err := row.Scan(&ex.ID, &ex.TenantID, &ex.EpisodeID, &ex.SkillID, &ex.SkillVersionID, &mode, &status,
		&ex.IdempotencyKey, &inRaw, &outRaw, &ex.ErrorCode, &ex.ErrorMessage, &ex.StartedAt, &completed,
		&ex.StepCursor, &ex.LeaseOwner, &leaseUntil, &heartbeat, &ex.RequesterID, &ex.SpecSnapshot, &ex.LeaseEpoch); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return skill.Execution{}, skill.ErrNotFound
		}
		return skill.Execution{}, err
	}
	ex.Mode = skill.ExecutionMode(mode)
	ex.Status = skill.ExecutionStatus(status)
	_ = json.Unmarshal(inRaw, &ex.Inputs)
	_ = json.Unmarshal(outRaw, &ex.Outputs)
	if completed.Valid {
		t := completed.Time
		ex.CompletedAt = &t
	}
	if leaseUntil.Valid {
		t := leaseUntil.Time
		ex.LeaseUntil = &t
	}
	if heartbeat.Valid {
		t := heartbeat.Time
		ex.HeartbeatAt = &t
	}
	return ex, nil
}

// ListStaleRunning returns RUNNING executions whose lease has expired (V3.3 recovery).
func (r *SkillExecutionRepository) ListStaleRunning(ctx context.Context, olderThan time.Time, limit int) ([]skill.Execution, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, tenant_id, COALESCE(episode_id,''), skill_id, skill_version_id, mode, status,
		       COALESCE(idempotency_key,''), inputs, outputs, error_code, error_message, started_at, completed_at,
		       step_cursor, COALESCE(lease_owner,''), lease_until, heartbeat_at, COALESCE(requester_id,''), COALESCE(spec_snapshot,''),
		       COALESCE(lease_epoch,0)
		FROM skill_executions
		WHERE status='RUNNING' AND (lease_until IS NULL OR lease_until < $1)
		ORDER BY started_at ASC
		LIMIT $2
	`, olderThan, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []skill.Execution
	for rows.Next() {
		ex, err := scanExecution(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, ex)
	}
	return out, rows.Err()
}

// ClaimExecutionLease atomically claims a stale RUNNING execution and bumps lease_epoch (fencing).
func (r *SkillExecutionRepository) ClaimExecutionLease(ctx context.Context, tenantID, executionID, owner string, until time.Time) (skill.Execution, bool, error) {
	res, err := r.db.ExecContext(ctx, `
		UPDATE skill_executions
		SET lease_owner=$3, lease_until=$4, heartbeat_at=NOW(), lease_epoch=lease_epoch+1
		WHERE tenant_id=$1 AND id=$2 AND status='RUNNING'
		  AND (lease_until IS NULL OR lease_until < NOW() OR lease_owner=$3)
	`, tenantID, executionID, owner, until)
	if err != nil {
		return skill.Execution{}, false, err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return skill.Execution{}, false, nil
	}
	ex, err := r.GetExecution(ctx, tenantID, executionID)
	return ex, true, err
}

func scanStep(row execScanner) (skill.StepExecution, error) {
	var st skill.StepExecution
	var status string
	var inRaw, outRaw []byte
	if err := row.Scan(&st.ID, &st.ExecutionID, &st.TenantID, &st.StepID, &st.Tool, &inRaw, &outRaw,
		&status, &st.ErrorCode, &st.DurationMs, &st.Sequence, &st.Attempt, &st.OperationKey, &st.LeaseEpoch); err != nil {
		return skill.StepExecution{}, err
	}
	st.Status = skill.StepStatus(status)
	_ = json.Unmarshal(inRaw, &st.Input)
	_ = json.Unmarshal(outRaw, &st.Output)
	return st, nil
}
