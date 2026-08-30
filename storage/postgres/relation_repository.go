package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/agent-experience-engine/agent-experience-engine/internal/experience"
)

// RelationRepository persists experience relations in PostgreSQL.
type RelationRepository struct {
	db *sql.DB
}

// NewRelationRepository constructs a Postgres-backed relation repository.
func NewRelationRepository(db *sql.DB) *RelationRepository {
	return &RelationRepository{db: db}
}

func (r *RelationRepository) Upsert(ctx context.Context, rel experience.ExperienceRelation) (experience.ExperienceRelation, error) {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO experience_relations (
			id, tenant_id, from_experience_id, to_experience_id, type, confidence, reason, created_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
		ON CONFLICT (tenant_id, from_experience_id, to_experience_id, type) DO UPDATE SET
			confidence = EXCLUDED.confidence,
			reason = EXCLUDED.reason
	`, rel.ID, rel.TenantID, rel.FromExperienceID, rel.ToExperienceID, string(rel.Type), rel.Confidence, rel.Reason, rel.CreatedAt)
	if err != nil {
		return experience.ExperienceRelation{}, fmt.Errorf("upsert experience relation: %w", err)
	}
	row := r.db.QueryRowContext(ctx, `
		SELECT id, tenant_id, from_experience_id, to_experience_id, type, confidence, reason, created_at
		FROM experience_relations
		WHERE tenant_id = $1 AND from_experience_id = $2 AND to_experience_id = $3 AND type = $4
	`, rel.TenantID, rel.FromExperienceID, rel.ToExperienceID, string(rel.Type))
	out, scanErr := scanRelation(row)
	if scanErr != nil {
		return experience.ExperienceRelation{}, fmt.Errorf("load upserted experience relation: %w", scanErr)
	}
	return out, nil
}

func (r *RelationRepository) ListByExperience(ctx context.Context, tenantID, experienceID string) ([]experience.ExperienceRelation, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, tenant_id, from_experience_id, to_experience_id, type, confidence, reason, created_at
		FROM experience_relations
		WHERE tenant_id = $1 AND (from_experience_id = $2 OR to_experience_id = $2)
		ORDER BY created_at ASC, id ASC
	`, tenantID, experienceID)
	if err != nil {
		return nil, fmt.Errorf("list experience relations: %w", err)
	}
	defer rows.Close()
	return scanRelations(rows)
}

func (r *RelationRepository) ConflictPeers(ctx context.Context, tenantID string, experienceIDs []string) (map[string]string, error) {
	ids := make([]string, 0, len(experienceIDs))
	for _, id := range experienceIDs {
		id = strings.TrimSpace(id)
		if id != "" {
			ids = append(ids, id)
		}
	}
	out := make(map[string]string)
	if len(ids) == 0 {
		return out, nil
	}

	// ANY($1) with pq array — use unnest of text[] for portability via simple OR expansion.
	args := []any{tenantID, string(experience.RelationConflicts)}
	fromPreds := make([]string, 0, len(ids))
	toPreds := make([]string, 0, len(ids))
	for _, id := range ids {
		args = append(args, id)
		n := len(args)
		fromPreds = append(fromPreds, fmt.Sprintf("from_experience_id = $%d", n))
		toPreds = append(toPreds, fmt.Sprintf("to_experience_id = $%d", n))
	}
	q := fmt.Sprintf(`
		SELECT from_experience_id, to_experience_id
		FROM experience_relations
		WHERE tenant_id = $1 AND type = $2
		  AND ((%s) OR (%s))
	`, strings.Join(fromPreds, " OR "), strings.Join(toPreds, " OR "))

	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("query conflict peers: %w", err)
	}
	defer rows.Close()

	want := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		want[id] = struct{}{}
	}
	for rows.Next() {
		var fromID, toID string
		if err := rows.Scan(&fromID, &toID); err != nil {
			return nil, fmt.Errorf("scan conflict peer: %w", err)
		}
		if _, ok := want[fromID]; ok {
			out[fromID] = toID
		}
		if _, ok := want[toID]; ok {
			out[toID] = fromID
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate conflict peers: %w", err)
	}
	return out, nil
}

type relationScanner interface {
	Scan(dest ...any) error
}

func scanRelation(row relationScanner) (experience.ExperienceRelation, error) {
	var rel experience.ExperienceRelation
	var typ string
	if err := row.Scan(
		&rel.ID, &rel.TenantID, &rel.FromExperienceID, &rel.ToExperienceID,
		&typ, &rel.Confidence, &rel.Reason, &rel.CreatedAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return experience.ExperienceRelation{}, sql.ErrNoRows
		}
		return experience.ExperienceRelation{}, err
	}
	rel.Type = experience.RelationType(typ)
	return rel, nil
}

func scanRelations(rows *sql.Rows) ([]experience.ExperienceRelation, error) {
	var out []experience.ExperienceRelation
	for rows.Next() {
		rel, err := scanRelation(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, rel)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}
