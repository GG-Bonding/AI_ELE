package postgres

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/agent-experience-engine/agent-experience-engine/internal/experience"
)

// PatternUsageRepository implements experience.PatternUsageRepository with PostgreSQL.
type PatternUsageRepository struct {
	db *sql.DB
}

// NewPatternUsageRepository constructs a Postgres-backed pattern usage repository.
func NewPatternUsageRepository(db *sql.DB) *PatternUsageRepository {
	return &PatternUsageRepository{db: db}
}

func (r *PatternUsageRepository) Create(ctx context.Context, u experience.PatternUsage) (experience.PatternUsage, error) {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO pattern_usages (
			id, tenant_id, episode_id, pattern_id,
			retrieval_score, final_score, used_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7)
	`,
		u.ID, u.TenantID, u.EpisodeID, u.PatternID,
		u.RetrievalScore, u.FinalScore, u.UsedAt,
	)
	if err != nil {
		return experience.PatternUsage{}, fmt.Errorf("insert pattern usage: %w", err)
	}
	return u, nil
}

func (r *PatternUsageRepository) ListByEpisode(ctx context.Context, tenantID, episodeID string) ([]experience.PatternUsage, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, tenant_id, episode_id, pattern_id,
		       retrieval_score, final_score, used_at
		FROM pattern_usages
		WHERE tenant_id = $1 AND episode_id = $2
		ORDER BY used_at ASC, id ASC
	`, tenantID, episodeID)
	if err != nil {
		return nil, fmt.Errorf("query pattern usages: %w", err)
	}
	defer rows.Close()

	var out []experience.PatternUsage
	for rows.Next() {
		var u experience.PatternUsage
		if err := rows.Scan(
			&u.ID, &u.TenantID, &u.EpisodeID, &u.PatternID,
			&u.RetrievalScore, &u.FinalScore, &u.UsedAt,
		); err != nil {
			return nil, fmt.Errorf("scan pattern usage: %w", err)
		}
		out = append(out, u)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate pattern usages: %w", err)
	}
	return out, nil
}
