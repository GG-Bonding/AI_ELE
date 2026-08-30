package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/agent-experience-engine/agent-experience-engine/internal/experience"
)

// SkillRepository persists skill candidates in PostgreSQL.
type SkillRepository struct {
	db *sql.DB
}

// NewSkillRepository constructs a Postgres-backed skill repository.
func NewSkillRepository(db *sql.DB) *SkillRepository {
	return &SkillRepository{db: db}
}

func (r *SkillRepository) Create(ctx context.Context, sk experience.SkillCandidate) (experience.SkillCandidate, error) {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO skill_candidates (
			id, tenant_id, pattern_id, name, description, spec_yaml,
			confidence, utility, status, created_at, updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
	`,
		sk.ID, sk.TenantID, sk.PatternID, sk.Name, sk.Description, sk.SpecYAML,
		sk.Confidence, sk.Utility, string(sk.Status), sk.CreatedAt, sk.UpdatedAt,
	)
	if err != nil {
		return experience.SkillCandidate{}, fmt.Errorf("insert skill candidate: %w", err)
	}
	return sk, nil
}

func (r *SkillRepository) Get(ctx context.Context, tenantID, id string) (experience.SkillCandidate, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, tenant_id, pattern_id, name, description, spec_yaml,
		       confidence, utility, status, created_at, updated_at
		FROM skill_candidates
		WHERE tenant_id = $1 AND id = $2
	`, tenantID, id)
	sk, err := scanSkill(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return experience.SkillCandidate{}, experience.ErrNotFound
		}
		return experience.SkillCandidate{}, fmt.Errorf("get skill candidate: %w", err)
	}
	return sk, nil
}

func (r *SkillRepository) FindByPattern(ctx context.Context, tenantID, patternID string) (experience.SkillCandidate, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, tenant_id, pattern_id, name, description, spec_yaml,
		       confidence, utility, status, created_at, updated_at
		FROM skill_candidates
		WHERE tenant_id = $1 AND pattern_id = $2
	`, tenantID, patternID)
	sk, err := scanSkill(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return experience.SkillCandidate{}, experience.ErrNotFound
		}
		return experience.SkillCandidate{}, fmt.Errorf("find skill by pattern: %w", err)
	}
	return sk, nil
}

type skillScanner interface {
	Scan(dest ...any) error
}

func scanSkill(row skillScanner) (experience.SkillCandidate, error) {
	var sk experience.SkillCandidate
	var status string
	var createdAt, updatedAt time.Time
	if err := row.Scan(
		&sk.ID, &sk.TenantID, &sk.PatternID, &sk.Name, &sk.Description, &sk.SpecYAML,
		&sk.Confidence, &sk.Utility, &status, &createdAt, &updatedAt,
	); err != nil {
		return experience.SkillCandidate{}, err
	}
	sk.Status = experience.SkillStatus(status)
	sk.CreatedAt = createdAt
	sk.UpdatedAt = updatedAt
	return sk, nil
}
