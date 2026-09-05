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
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO skill_executions (
			id, tenant_id, episode_id, skill_id, skill_version_id, mode, status,
			idempotency_key, inputs, outputs, error_code, error_message, started_at, completed_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)
	`, ex.ID, ex.TenantID, nullStr(ex.EpisodeID), ex.SkillID, ex.SkillVersionID, string(ex.Mode), string(ex.Status),
		idem, in, out, ex.ErrorCode, ex.ErrorMessage, ex.StartedAt, ex.CompletedAt)
	if err != nil {
		return skill.Execution{}, fmt.Errorf("insert skill execution: %w", err)
	}
	return ex, nil
}

func (r *SkillExecutionRepository) UpdateExecution(ctx context.Context, ex skill.Execution) (skill.Execution, error) {
	out, _ := json.Marshal(ex.Outputs)
	res, err := r.db.ExecContext(ctx, `
		UPDATE skill_executions
		SET status=$3, outputs=$4, error_code=$5, error_message=$6, completed_at=$7
		WHERE tenant_id=$1 AND id=$2
	`, ex.TenantID, ex.ID, string(ex.Status), out, ex.ErrorCode, ex.ErrorMessage, ex.CompletedAt)
	if err != nil {
		return skill.Execution{}, err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return skill.Execution{}, skill.ErrNotFound
	}
	return ex, nil
}

func (r *SkillExecutionRepository) GetExecution(ctx context.Context, tenantID, id string) (skill.Execution, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, tenant_id, COALESCE(episode_id,''), skill_id, skill_version_id, mode, status,
		       COALESCE(idempotency_key,''), inputs, outputs, error_code, error_message, started_at, completed_at
		FROM skill_executions WHERE tenant_id=$1 AND id=$2
	`, tenantID, id)
	return scanExecution(row)
}

func (r *SkillExecutionRepository) GetExecutionByIdempotency(ctx context.Context, tenantID, key string) (skill.Execution, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, tenant_id, COALESCE(episode_id,''), skill_id, skill_version_id, mode, status,
		       COALESCE(idempotency_key,''), inputs, outputs, error_code, error_message, started_at, completed_at
		FROM skill_executions WHERE tenant_id=$1 AND idempotency_key=$2
	`, tenantID, key)
	return scanExecution(row)
}

func (r *SkillExecutionRepository) CreateStep(ctx context.Context, st skill.StepExecution) (skill.StepExecution, error) {
	in, _ := json.Marshal(st.Input)
	out, _ := json.Marshal(st.Output)
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO skill_step_executions (
			id, execution_id, tenant_id, step_id, tool, input, output, status, error_code, duration_ms, sequence
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
	`, st.ID, st.ExecutionID, st.TenantID, st.StepID, st.Tool, in, out, string(st.Status), st.ErrorCode, st.DurationMs, st.Sequence)
	if err != nil {
		return skill.StepExecution{}, err
	}
	return st, nil
}

func (r *SkillExecutionRepository) ListSteps(ctx context.Context, tenantID, executionID string) ([]skill.StepExecution, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, execution_id, tenant_id, step_id, tool, input, output, status, error_code, duration_ms, sequence
		FROM skill_step_executions WHERE tenant_id=$1 AND execution_id=$2 ORDER BY sequence ASC
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
		INSERT INTO skill_approval_requests (id, tenant_id, execution_id, skill_id, status, reason, created_at, resolved_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
	`, req.ID, req.TenantID, req.ExecutionID, req.SkillID, string(req.Status), req.Reason, req.CreatedAt, req.ResolvedAt)
	if err != nil {
		return skill.ApprovalRequest{}, err
	}
	return req, nil
}

func (r *SkillExecutionRepository) UpdateApproval(ctx context.Context, req skill.ApprovalRequest) (skill.ApprovalRequest, error) {
	res, err := r.db.ExecContext(ctx, `
		UPDATE skill_approval_requests SET status=$3, reason=$4, resolved_at=$5
		WHERE tenant_id=$1 AND id=$2
	`, req.TenantID, req.ID, string(req.Status), req.Reason, req.ResolvedAt)
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
		SELECT id, tenant_id, execution_id, skill_id, status, reason, created_at, resolved_at
		FROM skill_approval_requests WHERE tenant_id=$1 AND id=$2
	`, tenantID, id)
	var req skill.ApprovalRequest
	var status string
	var resolved sql.NullTime
	if err := row.Scan(&req.ID, &req.TenantID, &req.ExecutionID, &req.SkillID, &status, &req.Reason, &req.CreatedAt, &resolved); err != nil {
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

func (r *SkillExecutionRepository) GetLearningEventByFeedbackVersion(ctx context.Context, tenantID, feedbackID, versionID string) (skill.LearningEvent, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, tenant_id, skill_id, skill_version_id, COALESCE(execution_id,''), feedback_id,
		       reward, confidence, credit, status, created_at, applied_at
		FROM skill_learning_events
		WHERE tenant_id=$1 AND feedback_id=$2 AND skill_version_id=$3
	`, tenantID, feedbackID, versionID)
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
	var completed sql.NullTime
	if err := row.Scan(&ex.ID, &ex.TenantID, &ex.EpisodeID, &ex.SkillID, &ex.SkillVersionID, &mode, &status,
		&ex.IdempotencyKey, &inRaw, &outRaw, &ex.ErrorCode, &ex.ErrorMessage, &ex.StartedAt, &completed); err != nil {
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
	return ex, nil
}

func scanStep(row execScanner) (skill.StepExecution, error) {
	var st skill.StepExecution
	var status string
	var inRaw, outRaw []byte
	if err := row.Scan(&st.ID, &st.ExecutionID, &st.TenantID, &st.StepID, &st.Tool, &inRaw, &outRaw,
		&status, &st.ErrorCode, &st.DurationMs, &st.Sequence); err != nil {
		return skill.StepExecution{}, err
	}
	st.Status = skill.StepStatus(status)
	_ = json.Unmarshal(inRaw, &st.Input)
	_ = json.Unmarshal(outRaw, &st.Output)
	return st, nil
}
