package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/agent-experience-engine/agent-experience-engine/internal/experience"
)

// PatternRepository persists patterns in PostgreSQL.
type PatternRepository struct {
	db *sql.DB
}

// NewPatternRepository constructs a Postgres-backed pattern repository.
func NewPatternRepository(db *sql.DB) *PatternRepository {
	return &PatternRepository{db: db}
}

func (r *PatternRepository) Create(ctx context.Context, p experience.Pattern) (experience.Pattern, error) {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO patterns (
			id, tenant_id, type, scope, scope_key, trigger_text, content,
			confidence, utility, alpha, beta, success_count, failure_count,
			support_count, status, created_at, updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17)
	`,
		p.ID, p.TenantID, string(p.Type), string(p.Scope), p.ScopeKey, p.Trigger, p.Content,
		p.Confidence, p.Utility, p.Alpha, p.Beta, p.SuccessCount, p.FailureCount,
		p.SupportCount, string(p.Status), p.CreatedAt, p.UpdatedAt,
	)
	if err != nil {
		return experience.Pattern{}, fmt.Errorf("insert pattern: %w", err)
	}
	return p, nil
}

func (r *PatternRepository) Update(ctx context.Context, p experience.Pattern) (experience.Pattern, error) {
	res, err := r.db.ExecContext(ctx, `
		UPDATE patterns SET
			type = $1, scope = $2, scope_key = $3, trigger_text = $4, content = $5,
			confidence = $6, utility = $7, alpha = $8, beta = $9,
			success_count = $10, failure_count = $11, support_count = $12,
			status = $13, updated_at = $14
		WHERE tenant_id = $15 AND id = $16
	`,
		string(p.Type), string(p.Scope), p.ScopeKey, p.Trigger, p.Content,
		p.Confidence, p.Utility, p.Alpha, p.Beta,
		p.SuccessCount, p.FailureCount, p.SupportCount,
		string(p.Status), p.UpdatedAt,
		p.TenantID, p.ID,
	)
	if err != nil {
		return experience.Pattern{}, fmt.Errorf("update pattern: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return experience.Pattern{}, fmt.Errorf("rows affected: %w", err)
	}
	if n == 0 {
		return experience.Pattern{}, experience.ErrNotFound
	}
	return p, nil
}

func (r *PatternRepository) Get(ctx context.Context, tenantID, id string) (experience.Pattern, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, tenant_id, type, scope, scope_key, trigger_text, content,
		       confidence, utility, alpha, beta, success_count, failure_count,
		       support_count, status, created_at, updated_at
		FROM patterns
		WHERE tenant_id = $1 AND id = $2
	`, tenantID, id)
	p, err := scanPattern(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return experience.Pattern{}, experience.ErrNotFound
		}
		return experience.Pattern{}, fmt.Errorf("get pattern: %w", err)
	}
	return p, nil
}

func (r *PatternRepository) AddEvidence(ctx context.Context, ev experience.PatternEvidence) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO pattern_evidence (pattern_id, experience_id, created_at)
		VALUES ($1, $2, $3)
		ON CONFLICT (pattern_id, experience_id) DO NOTHING
	`, ev.PatternID, ev.ExperienceID, ev.CreatedAt)
	if err != nil {
		return fmt.Errorf("insert pattern evidence: %w", err)
	}
	return nil
}

func (r *PatternRepository) ListEvidence(ctx context.Context, tenantID, patternID string) ([]experience.PatternEvidence, error) {
	if _, err := r.Get(ctx, tenantID, patternID); err != nil {
		return nil, err
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT pattern_id, experience_id, created_at
		FROM pattern_evidence
		WHERE pattern_id = $1
		ORDER BY created_at ASC, experience_id ASC
	`, patternID)
	if err != nil {
		return nil, fmt.Errorf("list pattern evidence: %w", err)
	}
	defer rows.Close()

	var out []experience.PatternEvidence
	for rows.Next() {
		var ev experience.PatternEvidence
		if err := rows.Scan(&ev.PatternID, &ev.ExperienceID, &ev.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan pattern evidence: %w", err)
		}
		out = append(out, ev)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate pattern evidence: %w", err)
	}
	return out, nil
}

func (r *PatternRepository) FindByExperience(ctx context.Context, tenantID string, experienceIDs []string) ([]experience.Pattern, error) {
	ids := make([]string, 0, len(experienceIDs))
	for _, id := range experienceIDs {
		id = strings.TrimSpace(id)
		if id != "" {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return nil, nil
	}

	args := []any{tenantID}
	preds := make([]string, 0, len(ids))
	for _, id := range ids {
		args = append(args, id)
		preds = append(preds, fmt.Sprintf("pe.experience_id = $%d", len(args)))
	}
	q := fmt.Sprintf(`
		SELECT DISTINCT p.id, p.tenant_id, p.type, p.scope, p.scope_key, p.trigger_text, p.content,
		       p.confidence, p.utility, p.alpha, p.beta, p.success_count, p.failure_count,
		       p.support_count, p.status, p.created_at, p.updated_at
		FROM patterns p
		INNER JOIN pattern_evidence pe ON pe.pattern_id = p.id
		WHERE p.tenant_id = $1 AND (%s)
		ORDER BY p.id ASC
	`, strings.Join(preds, " OR "))

	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("find patterns by experience: %w", err)
	}
	defer rows.Close()

	var out []experience.Pattern
	for rows.Next() {
		p, err := scanPattern(rows)
		if err != nil {
			return nil, fmt.Errorf("scan pattern: %w", err)
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate patterns: %w", err)
	}
	return out, nil
}

type patternScanner interface {
	Scan(dest ...any) error
}

func scanPattern(row patternScanner) (experience.Pattern, error) {
	var p experience.Pattern
	var typ, scope, status string
	var createdAt, updatedAt time.Time
	if err := row.Scan(
		&p.ID, &p.TenantID, &typ, &scope, &p.ScopeKey, &p.Trigger, &p.Content,
		&p.Confidence, &p.Utility, &p.Alpha, &p.Beta, &p.SuccessCount, &p.FailureCount,
		&p.SupportCount, &status, &createdAt, &updatedAt,
	); err != nil {
		return experience.Pattern{}, err
	}
	p.Type = experience.Type(typ)
	p.Scope = experience.Scope(scope)
	p.Status = experience.PatternStatus(status)
	p.CreatedAt = createdAt
	p.UpdatedAt = updatedAt
	return p, nil
}
