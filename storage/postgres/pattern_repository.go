package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/agent-experience-engine/agent-experience-engine/internal/experience"
	"github.com/jackc/pgx/v5/pgconn"
)

const patternSelectCols = `id, tenant_id, type, scope, scope_key, trigger_text, content,
		       confidence, utility, alpha, beta, success_count, failure_count,
		       support_count, status, cluster_fingerprint, created_at, updated_at,
		       embedding::text`

// PatternRepository persists patterns in PostgreSQL.
type PatternRepository struct {
	db *sql.DB
}

// NewPatternRepository constructs a Postgres-backed pattern repository.
func NewPatternRepository(db *sql.DB) *PatternRepository {
	return &PatternRepository{db: db}
}

func (r *PatternRepository) Create(ctx context.Context, p experience.Pattern) (experience.Pattern, error) {
	var emb any
	if len(p.Embedding) > 0 {
		vec, err := formatVector(p.Embedding)
		if err != nil {
			return experience.Pattern{}, err
		}
		emb = vec
	}
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO patterns (
			id, tenant_id, type, scope, scope_key, trigger_text, content,
			confidence, utility, alpha, beta, success_count, failure_count,
			support_count, status, cluster_fingerprint, created_at, updated_at, embedding
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19::vector)
	`,
		p.ID, p.TenantID, string(p.Type), string(p.Scope), p.ScopeKey, p.Trigger, p.Content,
		p.Confidence, p.Utility, p.Alpha, p.Beta, p.SuccessCount, p.FailureCount,
		p.SupportCount, string(p.Status), p.ClusterFingerprint, p.CreatedAt, p.UpdatedAt, emb,
	)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return experience.Pattern{}, experience.ErrDuplicateCluster
		}
		return experience.Pattern{}, fmt.Errorf("insert pattern: %w", err)
	}
	return p, nil
}

func (r *PatternRepository) Update(ctx context.Context, p experience.Pattern) (experience.Pattern, error) {
	// Embedding is set at create / re-embed paths; utility/status updates leave it unchanged.
	res, err := r.db.ExecContext(ctx, `
		UPDATE patterns SET
			type = $1, scope = $2, scope_key = $3, trigger_text = $4, content = $5,
			confidence = $6, utility = $7, alpha = $8, beta = $9,
			success_count = $10, failure_count = $11, support_count = $12,
			status = $13, cluster_fingerprint = $14, updated_at = $15
		WHERE tenant_id = $16 AND id = $17
	`,
		string(p.Type), string(p.Scope), p.ScopeKey, p.Trigger, p.Content,
		p.Confidence, p.Utility, p.Alpha, p.Beta,
		p.SuccessCount, p.FailureCount, p.SupportCount,
		string(p.Status), p.ClusterFingerprint, p.UpdatedAt,
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
		SELECT `+patternSelectCols+`
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

func (r *PatternRepository) GetByFingerprint(ctx context.Context, tenantID, fingerprint string) (experience.Pattern, error) {
	fp := strings.TrimSpace(fingerprint)
	if fp == "" {
		return experience.Pattern{}, experience.ErrNotFound
	}
	row := r.db.QueryRowContext(ctx, `
		SELECT `+patternSelectCols+`
		FROM patterns
		WHERE tenant_id = $1 AND cluster_fingerprint = $2
	`, tenantID, fp)
	p, err := scanPattern(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return experience.Pattern{}, experience.ErrNotFound
		}
		return experience.Pattern{}, fmt.Errorf("get pattern by fingerprint: %w", err)
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
		       p.support_count, p.status, p.cluster_fingerprint, p.created_at, p.updated_at,
		       p.embedding::text
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

func (r *PatternRepository) List(ctx context.Context, filter experience.PatternListFilter) ([]experience.Pattern, error) {
	tenantID := strings.TrimSpace(filter.TenantID)
	if tenantID == "" {
		return nil, experience.ErrInvalidInput
	}
	args := []any{tenantID}
	where := []string{"tenant_id = $1"}

	if len(filter.Statuses) > 0 {
		preds := make([]string, 0, len(filter.Statuses))
		for _, st := range filter.Statuses {
			args = append(args, string(st))
			preds = append(preds, fmt.Sprintf("$%d", len(args)))
		}
		where = append(where, "status IN ("+strings.Join(preds, ",")+ ")")
	}
	if len(filter.Types) > 0 {
		preds := make([]string, 0, len(filter.Types))
		for _, typ := range filter.Types {
			args = append(args, string(typ))
			preds = append(preds, fmt.Sprintf("$%d", len(args)))
		}
		where = append(where, "type IN ("+strings.Join(preds, ",")+ ")")
	}
	if len(filter.Scopes) > 0 {
		preds := make([]string, 0, len(filter.Scopes))
		for _, sc := range filter.Scopes {
			args = append(args, string(sc))
			preds = append(preds, fmt.Sprintf("$%d", len(args)))
		}
		where = append(where, "scope IN ("+strings.Join(preds, ",")+ ")")
	}
	if sk := strings.TrimSpace(filter.ScopeKey); sk != "" {
		args = append(args, sk)
		where = append(where, fmt.Sprintf("scope_key = $%d", len(args)))
	}

	q := `
		SELECT ` + patternSelectCols + `
		FROM patterns
		WHERE ` + strings.Join(where, " AND ") + `
		ORDER BY utility DESC, id ASC`
	if filter.Limit > 0 {
		args = append(args, filter.Limit)
		q += fmt.Sprintf(" LIMIT $%d", len(args))
	}

	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("list patterns: %w", err)
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

func (r *PatternRepository) Search(ctx context.Context, filter experience.PatternSearchFilter) ([]experience.ScoredPattern, error) {
	tenantID := strings.TrimSpace(filter.TenantID)
	if tenantID == "" {
		return nil, experience.ErrInvalidInput
	}
	if len(filter.QueryEmbedding) == 0 {
		return nil, fmt.Errorf("%w: query embedding is required", experience.ErrInvalidInput)
	}
	vec, err := formatVector(filter.QueryEmbedding)
	if err != nil {
		return nil, err
	}
	topK := filter.TopK
	if topK <= 0 {
		topK = 20
	}

	args := []any{tenantID, vec}
	where := []string{"tenant_id = $1", "embedding IS NOT NULL"}

	statuses := filter.Statuses
	if len(statuses) == 0 {
		statuses = []experience.PatternStatus{experience.PatternStatusActive}
	}
	statusPlaceholders := make([]string, len(statuses))
	for i, s := range statuses {
		args = append(args, string(s))
		statusPlaceholders[i] = fmt.Sprintf("$%d", len(args))
	}
	where = append(where, "status IN ("+strings.Join(statusPlaceholders, ",")+")")

	authSQL, authArgs := scopeAuthSQL(filter.AgentID, filter.UserID, filter.ScopeKey, filter.Tools, args)
	where = append(where, authSQL)
	args = authArgs

	args = append(args, topK)
	limitPlaceholder := fmt.Sprintf("$%d", len(args))

	q := fmt.Sprintf(`
		SELECT id, tenant_id, type, scope, scope_key, trigger_text, content,
		       confidence, utility, alpha, beta, success_count, failure_count,
		       support_count, status, cluster_fingerprint, created_at, updated_at,
		       embedding::text,
		       1 - (embedding <=> $2::vector) AS similarity
		FROM patterns
		WHERE %s
		ORDER BY embedding <=> $2::vector
		LIMIT %s
	`, strings.Join(where, " AND "), limitPlaceholder)

	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("search patterns: %w", err)
	}
	defer rows.Close()

	var out []experience.ScoredPattern
	for rows.Next() {
		p, sim, err := scanPatternScored(rows)
		if err != nil {
			return nil, fmt.Errorf("scan pattern search row: %w", err)
		}
		out = append(out, experience.ScoredPattern{Pattern: p, Similarity: sim})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate pattern search: %w", err)
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
	var embText sql.NullString
	if err := row.Scan(
		&p.ID, &p.TenantID, &typ, &scope, &p.ScopeKey, &p.Trigger, &p.Content,
		&p.Confidence, &p.Utility, &p.Alpha, &p.Beta, &p.SuccessCount, &p.FailureCount,
		&p.SupportCount, &status, &p.ClusterFingerprint, &createdAt, &updatedAt,
		&embText,
	); err != nil {
		return experience.Pattern{}, err
	}
	p.Type = experience.Type(typ)
	p.Scope = experience.Scope(scope)
	p.Status = experience.PatternStatus(status)
	p.CreatedAt = createdAt
	p.UpdatedAt = updatedAt
	if embText.Valid && strings.TrimSpace(embText.String) != "" {
		emb, err := parseVector(embText.String)
		if err != nil {
			return experience.Pattern{}, fmt.Errorf("parse pattern embedding: %w", err)
		}
		p.Embedding = emb
	}
	return p, nil
}

func scanPatternScored(row patternScanner) (experience.Pattern, float64, error) {
	var p experience.Pattern
	var typ, scope, status string
	var createdAt, updatedAt time.Time
	var embText sql.NullString
	var sim float64
	if err := row.Scan(
		&p.ID, &p.TenantID, &typ, &scope, &p.ScopeKey, &p.Trigger, &p.Content,
		&p.Confidence, &p.Utility, &p.Alpha, &p.Beta, &p.SuccessCount, &p.FailureCount,
		&p.SupportCount, &status, &p.ClusterFingerprint, &createdAt, &updatedAt,
		&embText, &sim,
	); err != nil {
		return experience.Pattern{}, 0, err
	}
	p.Type = experience.Type(typ)
	p.Scope = experience.Scope(scope)
	p.Status = experience.PatternStatus(status)
	p.CreatedAt = createdAt
	p.UpdatedAt = updatedAt
	if embText.Valid && strings.TrimSpace(embText.String) != "" {
		emb, err := parseVector(embText.String)
		if err != nil {
			return experience.Pattern{}, 0, fmt.Errorf("parse pattern embedding: %w", err)
		}
		p.Embedding = emb
	}
	return p, sim, nil
}
