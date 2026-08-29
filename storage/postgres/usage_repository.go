package postgres

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/agent-experience-engine/agent-experience-engine/internal/experience"
)

// UsageRepository implements experience.UsageRepository with PostgreSQL.
type UsageRepository struct {
	db *sql.DB
}

// NewUsageRepository constructs a Postgres-backed usage repository.
func NewUsageRepository(db *sql.DB) *UsageRepository {
	return &UsageRepository{db: db}
}

func (r *UsageRepository) Create(ctx context.Context, u experience.Usage) (experience.Usage, error) {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO experience_usages (
			id, tenant_id, episode_id, experience_id,
			retrieval_score, selection_decision, final_score, used_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
	`,
		u.ID, u.TenantID, u.EpisodeID, u.ExperienceID,
		u.RetrievalScore, u.SelectionDecision, u.FinalScore, u.UsedAt,
	)
	if err != nil {
		return experience.Usage{}, fmt.Errorf("insert experience usage: %w", err)
	}
	return u, nil
}

func (r *UsageRepository) ListByEpisode(ctx context.Context, tenantID, episodeID string) ([]experience.Usage, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, tenant_id, episode_id, experience_id,
		       retrieval_score, selection_decision, final_score, used_at
		FROM experience_usages
		WHERE tenant_id = $1 AND episode_id = $2
		ORDER BY used_at ASC, id ASC
	`, tenantID, episodeID)
	if err != nil {
		return nil, fmt.Errorf("query experience usages: %w", err)
	}
	defer rows.Close()

	var out []experience.Usage
	for rows.Next() {
		var u experience.Usage
		if err := rows.Scan(
			&u.ID, &u.TenantID, &u.EpisodeID, &u.ExperienceID,
			&u.RetrievalScore, &u.SelectionDecision, &u.FinalScore, &u.UsedAt,
		); err != nil {
			return nil, fmt.Errorf("scan experience usage: %w", err)
		}
		out = append(out, u)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate experience usages: %w", err)
	}
	return out, nil
}
