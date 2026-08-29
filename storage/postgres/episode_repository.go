package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/agent-experience-engine/agent-experience-engine/internal/attempt"
	"github.com/agent-experience-engine/agent-experience-engine/internal/episode"
	"github.com/agent-experience-engine/agent-experience-engine/internal/outcome"
)

// EpisodeRepository implements episode.Repository with PostgreSQL.
type EpisodeRepository struct {
	db *sql.DB
}

// NewEpisodeRepository constructs a Postgres-backed episode repository.
func NewEpisodeRepository(db *sql.DB) *EpisodeRepository {
	return &EpisodeRepository{db: db}
}

func (r *EpisodeRepository) CreateEpisode(ctx context.Context, ep episode.Episode) (episode.Episode, error) {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO episodes (
			id, tenant_id, agent_id, user_id, task_type, goal, input, status,
			started_at, completed_at, created_at, updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
	`,
		ep.ID, ep.TenantID, ep.AgentID, ep.UserID, ep.TaskType, ep.Goal, ep.Input, string(ep.Status),
		ep.StartedAt, ep.CompletedAt, ep.CreatedAt, ep.UpdatedAt,
	)
	if err != nil {
		return episode.Episode{}, fmt.Errorf("insert episode: %w", err)
	}
	return ep, nil
}

func (r *EpisodeRepository) GetEpisode(ctx context.Context, tenantID, id string) (episode.Episode, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, tenant_id, agent_id, user_id, task_type, goal, input, status,
		       started_at, completed_at, created_at, updated_at
		FROM episodes
		WHERE tenant_id = $1 AND id = $2
	`, tenantID, id)

	var ep episode.Episode
	var status string
	err := row.Scan(
		&ep.ID, &ep.TenantID, &ep.AgentID, &ep.UserID, &ep.TaskType, &ep.Goal, &ep.Input, &status,
		&ep.StartedAt, &ep.CompletedAt, &ep.CreatedAt, &ep.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return episode.Episode{}, episode.ErrNotFound
	}
	if err != nil {
		return episode.Episode{}, fmt.Errorf("scan episode: %w", err)
	}
	ep.Status = episode.Status(status)
	return ep, nil
}

func (r *EpisodeRepository) UpdateEpisode(ctx context.Context, ep episode.Episode) (episode.Episode, error) {
	res, err := r.db.ExecContext(ctx, `
		UPDATE episodes
		SET status = $1, completed_at = $2, updated_at = $3,
		    task_type = $4, goal = $5, input = $6
		WHERE tenant_id = $7 AND id = $8
	`, string(ep.Status), ep.CompletedAt, ep.UpdatedAt, ep.TaskType, ep.Goal, ep.Input, ep.TenantID, ep.ID)
	if err != nil {
		return episode.Episode{}, fmt.Errorf("update episode: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return episode.Episode{}, fmt.Errorf("rows affected: %w", err)
	}
	if n == 0 {
		return episode.Episode{}, episode.ErrNotFound
	}
	return ep, nil
}

func (r *EpisodeRepository) CreateAttempt(ctx context.Context, a attempt.Attempt) (attempt.Attempt, error) {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO attempts (
			id, episode_id, tenant_id, sequence, hypothesis, action, tool_name,
			input, output, status, error_code, error_message, started_at, completed_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)
	`,
		a.ID, a.EpisodeID, a.TenantID, a.Sequence, a.Hypothesis, a.Action, a.ToolName,
		nullableJSON(a.Input), nullableJSON(a.Output), string(a.Status), a.ErrorCode, a.ErrorMessage,
		a.StartedAt, a.CompletedAt,
	)
	if err != nil {
		return attempt.Attempt{}, fmt.Errorf("insert attempt: %w", err)
	}
	return a, nil
}

func (r *EpisodeRepository) ListAttempts(ctx context.Context, tenantID, episodeID string) ([]attempt.Attempt, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, episode_id, tenant_id, sequence, hypothesis, action, tool_name,
		       input, output, status, error_code, error_message, started_at, completed_at
		FROM attempts
		WHERE tenant_id = $1 AND episode_id = $2
		ORDER BY sequence ASC
	`, tenantID, episodeID)
	if err != nil {
		return nil, fmt.Errorf("query attempts: %w", err)
	}
	defer rows.Close()

	var out []attempt.Attempt
	for rows.Next() {
		var a attempt.Attempt
		var status string
		var input, output []byte
		if err := rows.Scan(
			&a.ID, &a.EpisodeID, &a.TenantID, &a.Sequence, &a.Hypothesis, &a.Action, &a.ToolName,
			&input, &output, &status, &a.ErrorCode, &a.ErrorMessage, &a.StartedAt, &a.CompletedAt,
		); err != nil {
			return nil, fmt.Errorf("scan attempt: %w", err)
		}
		a.Status = attempt.Status(status)
		a.Input = json.RawMessage(input)
		a.Output = json.RawMessage(output)
		out = append(out, a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate attempts: %w", err)
	}
	return out, nil
}

func (r *EpisodeRepository) NextAttemptSequence(ctx context.Context, tenantID, episodeID string) (int, error) {
	var next int
	err := r.db.QueryRowContext(ctx, `
		SELECT COALESCE(MAX(sequence), 0) + 1
		FROM attempts
		WHERE tenant_id = $1 AND episode_id = $2
	`, tenantID, episodeID).Scan(&next)
	if err != nil {
		return 0, fmt.Errorf("next attempt sequence: %w", err)
	}
	return next, nil
}

func (r *EpisodeRepository) CreateOutcome(ctx context.Context, o outcome.Outcome) (outcome.Outcome, error) {
	metrics, err := json.Marshal(o.Metrics)
	if err != nil {
		return outcome.Outcome{}, fmt.Errorf("marshal metrics: %w", err)
	}
	if o.Metrics == nil {
		metrics = nil
	}

	_, err = r.db.ExecContext(ctx, `
		INSERT INTO outcomes (
			id, episode_id, tenant_id, status, result, verified, verifier, metrics, created_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
	`,
		o.ID, o.EpisodeID, o.TenantID, o.Status, nullableJSON(o.Result), o.Verified, o.Verifier,
		nullableJSON(metrics), o.CreatedAt,
	)
	if err != nil {
		return outcome.Outcome{}, fmt.Errorf("insert outcome: %w", err)
	}
	return o, nil
}

func (r *EpisodeRepository) GetOutcome(ctx context.Context, tenantID, episodeID string) (outcome.Outcome, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, episode_id, tenant_id, status, result, verified, verifier, metrics, created_at
		FROM outcomes
		WHERE tenant_id = $1 AND episode_id = $2
	`, tenantID, episodeID)

	var o outcome.Outcome
	var result, metrics []byte
	err := row.Scan(
		&o.ID, &o.EpisodeID, &o.TenantID, &o.Status, &result, &o.Verified, &o.Verifier, &metrics, &o.CreatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return outcome.Outcome{}, episode.ErrNotFound
	}
	if err != nil {
		return outcome.Outcome{}, fmt.Errorf("scan outcome: %w", err)
	}
	o.Result = json.RawMessage(result)
	if len(metrics) > 0 {
		if err := json.Unmarshal(metrics, &o.Metrics); err != nil {
			return outcome.Outcome{}, fmt.Errorf("unmarshal metrics: %w", err)
		}
	}
	return o, nil
}

func nullableJSON(raw json.RawMessage) any {
	if len(raw) == 0 {
		return nil
	}
	return []byte(raw)
}
