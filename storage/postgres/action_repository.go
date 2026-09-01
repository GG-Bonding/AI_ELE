package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/agent-experience-engine/agent-experience-engine/internal/action"
	"github.com/jackc/pgx/v5/pgconn"
)

// ActionRepository implements action.Repository with PostgreSQL.
type ActionRepository struct {
	db *sql.DB
}

// NewActionRepository constructs a Postgres-backed action repository.
func NewActionRepository(db *sql.DB) *ActionRepository {
	return &ActionRepository{db: db}
}

func (r *ActionRepository) CreateAction(ctx context.Context, a action.AgentAction) (action.AgentAction, error) {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO agent_actions (
			id, tenant_id, episode_id, sequence, type, tool_name,
			input, output, status, attempt_id, started_at, completed_at, created_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
	`,
		a.ID, a.TenantID, a.EpisodeID, a.Sequence, string(a.Type), a.ToolName,
		nullableJSON(a.Input), nullableJSON(a.Output), string(a.Status), a.AttemptID,
		a.StartedAt, a.CompletedAt, a.CreatedAt,
	)
	if err != nil {
		return action.AgentAction{}, fmt.Errorf("insert agent action: %w", err)
	}
	return a, nil
}

func (r *ActionRepository) GetAction(ctx context.Context, tenantID, actionID string) (action.AgentAction, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, tenant_id, episode_id, sequence, type, tool_name,
		       input, output, status, attempt_id, started_at, completed_at, created_at
		FROM agent_actions
		WHERE tenant_id = $1 AND id = $2
	`, tenantID, actionID)
	a, err := scanAction(row)
	if errors.Is(err, sql.ErrNoRows) {
		return action.AgentAction{}, action.ErrNotFound
	}
	if err != nil {
		return action.AgentAction{}, fmt.Errorf("get agent action: %w", err)
	}
	return a, nil
}

func (r *ActionRepository) ListActionsByEpisode(ctx context.Context, tenantID, episodeID string) ([]action.AgentAction, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, tenant_id, episode_id, sequence, type, tool_name,
		       input, output, status, attempt_id, started_at, completed_at, created_at
		FROM agent_actions
		WHERE tenant_id = $1 AND episode_id = $2
		ORDER BY sequence ASC, id ASC
	`, tenantID, episodeID)
	if err != nil {
		return nil, fmt.Errorf("query agent actions: %w", err)
	}
	defer rows.Close()

	var out []action.AgentAction
	for rows.Next() {
		a, err := scanAction(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate agent actions: %w", err)
	}
	return out, nil
}

func (r *ActionRepository) NextActionSequence(ctx context.Context, tenantID, episodeID string) (int, error) {
	var next int
	err := r.db.QueryRowContext(ctx, `
		SELECT COALESCE(MAX(sequence), 0) + 1
		FROM agent_actions
		WHERE tenant_id = $1 AND episode_id = $2
	`, tenantID, episodeID).Scan(&next)
	if err != nil {
		return 0, fmt.Errorf("next action sequence: %w", err)
	}
	return next, nil
}

func (r *ActionRepository) CreateLink(ctx context.Context, link action.ExperienceActionLink) (action.ExperienceActionLink, error) {
	fieldsJSON, err := marshalStringSlice(link.AffectedFields)
	if err != nil {
		return action.ExperienceActionLink{}, fmt.Errorf("marshal affected_fields: %w", err)
	}
	_, err = r.db.ExecContext(ctx, `
		INSERT INTO experience_action_links (
			id, tenant_id, episode_id, experience_id, action_id, influence, affected_fields, evidence, created_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
	`,
		link.ID, link.TenantID, link.EpisodeID, link.ExperienceID, link.ActionID,
		link.Influence, fieldsJSON, link.Evidence, link.CreatedAt,
	)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return action.ExperienceActionLink{}, action.ErrDuplicateLink
		}
		return action.ExperienceActionLink{}, fmt.Errorf("insert experience action link: %w", err)
	}
	return link, nil
}

func (r *ActionRepository) ListLinksByEpisode(ctx context.Context, tenantID, episodeID string) ([]action.ExperienceActionLink, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, tenant_id, episode_id, experience_id, action_id, influence, affected_fields, evidence, created_at
		FROM experience_action_links
		WHERE tenant_id = $1 AND episode_id = $2
		ORDER BY created_at ASC, id ASC
	`, tenantID, episodeID)
	if err != nil {
		return nil, fmt.Errorf("query experience action links: %w", err)
	}
	defer rows.Close()
	return scanLinks(rows)
}

func (r *ActionRepository) ListLinksByAction(ctx context.Context, tenantID, actionID string) ([]action.ExperienceActionLink, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, tenant_id, episode_id, experience_id, action_id, influence, affected_fields, evidence, created_at
		FROM experience_action_links
		WHERE tenant_id = $1 AND action_id = $2
		ORDER BY created_at ASC, id ASC
	`, tenantID, actionID)
	if err != nil {
		return nil, fmt.Errorf("query experience action links by action: %w", err)
	}
	defer rows.Close()
	return scanLinks(rows)
}

func scanAction(row interface{ Scan(dest ...any) error }) (action.AgentAction, error) {
	var a action.AgentAction
	var typ, status string
	var input, output []byte
	if err := row.Scan(
		&a.ID, &a.TenantID, &a.EpisodeID, &a.Sequence, &typ, &a.ToolName,
		&input, &output, &status, &a.AttemptID, &a.StartedAt, &a.CompletedAt, &a.CreatedAt,
	); err != nil {
		return action.AgentAction{}, err
	}
	a.Type = action.Type(typ)
	a.Status = action.Status(status)
	a.Input = input
	a.Output = output
	return a, nil
}

func scanLinks(rows *sql.Rows) ([]action.ExperienceActionLink, error) {
	var out []action.ExperienceActionLink
	for rows.Next() {
		var link action.ExperienceActionLink
		var fieldsJSON []byte
		if err := rows.Scan(
			&link.ID, &link.TenantID, &link.EpisodeID, &link.ExperienceID, &link.ActionID,
			&link.Influence, &fieldsJSON, &link.Evidence, &link.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan experience action link: %w", err)
		}
		fields, err := unmarshalStringSlice(fieldsJSON)
		if err != nil {
			return nil, fmt.Errorf("unmarshal affected_fields: %w", err)
		}
		link.AffectedFields = fields
		out = append(out, link)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate experience action links: %w", err)
	}
	return out, nil
}

func marshalStringSlice(vals []string) ([]byte, error) {
	if vals == nil {
		vals = []string{}
	}
	return json.Marshal(vals)
}

func unmarshalStringSlice(raw []byte) ([]string, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var out []string
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return nil, nil
	}
	return out, nil
}
