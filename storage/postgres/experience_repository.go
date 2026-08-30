package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/agent-experience-engine/agent-experience-engine/internal/experience"
)

// ExperienceRepository implements experience.Repository with PostgreSQL + pgvector.
type ExperienceRepository struct {
	db *sql.DB
}

// NewExperienceRepository constructs a Postgres-backed experience repository.
func NewExperienceRepository(db *sql.DB) *ExperienceRepository {
	return &ExperienceRepository{db: db}
}

func (r *ExperienceRepository) Create(ctx context.Context, exp experience.Experience) (experience.Experience, error) {
	vec, err := formatVector(exp.Embedding)
	if err != nil {
		return experience.Experience{}, err
	}

	_, err = r.db.ExecContext(ctx, `
		INSERT INTO experiences (
			id, tenant_id, type, scope, scope_key, trigger_text, content, source_episode_id,
			confidence, utility, alpha, beta, success_count, failure_count, use_count,
			status, version, supersedes_id, embedding, created_at, updated_at, last_used_at, evidence
		) VALUES (
			$1,$2,$3,$4,$5,$6,$7,$8,
			$9,$10,$11,$12,$13,$14,$15,
			$16,$17,$18,$19::vector,$20,$21,$22,$23
		)
	`,
		exp.ID, exp.TenantID, string(exp.Type), string(exp.Scope), exp.ScopeKey, exp.Trigger, exp.Content, exp.SourceEpisodeID,
		exp.Confidence, exp.Utility, exp.Alpha, exp.Beta, exp.SuccessCount, exp.FailureCount, exp.UseCount,
		string(exp.Status), exp.Version, exp.SupersedesID, vec, exp.CreatedAt, exp.UpdatedAt, exp.LastUsedAt, mustJSON(exp.Evidence),
	)
	if err != nil {
		return experience.Experience{}, fmt.Errorf("insert experience: %w", err)
	}
	return exp, nil
}

func (r *ExperienceRepository) Update(ctx context.Context, exp experience.Experience) (experience.Experience, error) {
	nextVersion := exp.Version + 1
	res, err := r.db.ExecContext(ctx, `
		UPDATE experiences SET
			type = $1, scope = $2, scope_key = $3, trigger_text = $4, content = $5,
			confidence = $6, utility = $7, alpha = $8, beta = $9,
			success_count = $10, failure_count = $11, use_count = $12,
			status = $13, version = $14, supersedes_id = $15,
			updated_at = $16, last_used_at = $17
		WHERE tenant_id = $18 AND id = $19 AND version = $20
	`,
		string(exp.Type), string(exp.Scope), exp.ScopeKey, exp.Trigger, exp.Content,
		exp.Confidence, exp.Utility, exp.Alpha, exp.Beta,
		exp.SuccessCount, exp.FailureCount, exp.UseCount,
		string(exp.Status), nextVersion, exp.SupersedesID,
		exp.UpdatedAt, exp.LastUsedAt, exp.TenantID, exp.ID, exp.Version,
	)
	if err != nil {
		return experience.Experience{}, fmt.Errorf("update experience: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return experience.Experience{}, fmt.Errorf("rows affected: %w", err)
	}
	if n == 0 {
		// Distinguish missing vs stale version.
		_, getErr := r.Get(ctx, exp.TenantID, exp.ID)
		if errors.Is(getErr, experience.ErrNotFound) {
			return experience.Experience{}, experience.ErrNotFound
		}
		if getErr != nil {
			return experience.Experience{}, getErr
		}
		return experience.Experience{}, experience.ErrConflict
	}
	exp.Version = nextVersion
	return exp, nil
}

func (r *ExperienceRepository) Get(ctx context.Context, tenantID, id string) (experience.Experience, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, tenant_id, type, scope, scope_key, trigger_text, content, source_episode_id,
		       confidence, utility, alpha, beta, success_count, failure_count, use_count,
		       status, version, supersedes_id, created_at, updated_at, last_used_at, evidence
		FROM experiences
		WHERE tenant_id = $1 AND id = $2
	`, tenantID, id)
	exp, err := scanExperience(row)
	if errors.Is(err, sql.ErrNoRows) {
		return experience.Experience{}, experience.ErrNotFound
	}
	if err != nil {
		return experience.Experience{}, fmt.Errorf("get experience: %w", err)
	}
	return exp, nil
}

func (r *ExperienceRepository) Search(ctx context.Context, filter experience.SearchFilter) ([]experience.ScoredExperience, error) {
	vec, err := formatVector(filter.QueryEmbedding)
	if err != nil {
		return nil, err
	}
	topK := filter.TopK
	if topK <= 0 {
		topK = 20
	}
	fetchLimit := topK * 4
	if fetchLimit < topK {
		fetchLimit = topK
	}

	args := []any{filter.TenantID, vec}
	var where []string
	where = append(where, "tenant_id = $1")

	statuses := filter.Statuses
	if len(statuses) == 0 {
		statuses = []experience.Status{experience.StatusActive, experience.StatusCandidate}
	}
	statusPlaceholders := make([]string, len(statuses))
	for i, s := range statuses {
		args = append(args, string(s))
		statusPlaceholders[i] = fmt.Sprintf("$%d", len(args))
	}
	where = append(where, "status IN ("+strings.Join(statusPlaceholders, ",")+")")

	if len(filter.Types) > 0 {
		typePlaceholders := make([]string, len(filter.Types))
		for i, t := range filter.Types {
			args = append(args, string(t))
			typePlaceholders[i] = fmt.Sprintf("$%d", len(args))
		}
		where = append(where, "type IN ("+strings.Join(typePlaceholders, ",")+")")
	}
	if len(filter.Scopes) > 0 {
		scopePlaceholders := make([]string, len(filter.Scopes))
		for i, s := range filter.Scopes {
			args = append(args, string(s))
			scopePlaceholders[i] = fmt.Sprintf("$%d", len(args))
		}
		where = append(where, "scope IN ("+strings.Join(scopePlaceholders, ",")+")")
	}
	if filter.ScopeKey != "" {
		args = append(args, filter.ScopeKey)
		where = append(where, fmt.Sprintf("scope_key = $%d", len(args)))
	}

	args = append(args, fetchLimit)
	limitPlaceholder := fmt.Sprintf("$%d", len(args))

	// $2 is query vector; cosine distance <=> ; similarity = 1 - distance
	q := fmt.Sprintf(`
		SELECT id, tenant_id, type, scope, scope_key, trigger_text, content, source_episode_id,
		       confidence, utility, alpha, beta, success_count, failure_count, use_count,
		       status, version, supersedes_id, created_at, updated_at, last_used_at, evidence,
		       1 - (embedding <=> $2::vector) AS similarity
		FROM experiences
		WHERE %s
		ORDER BY embedding <=> $2::vector
		LIMIT %s
	`, strings.Join(where, " AND "), limitPlaceholder)

	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("search experiences: %w", err)
	}
	defer rows.Close()

	var out []experience.ScoredExperience
	for rows.Next() {
		var exp experience.Experience
		var typ, scope, status string
		var sim float64
		var evidenceJSON []byte
		if err := rows.Scan(
			&exp.ID, &exp.TenantID, &typ, &scope, &exp.ScopeKey, &exp.Trigger, &exp.Content, &exp.SourceEpisodeID,
			&exp.Confidence, &exp.Utility, &exp.Alpha, &exp.Beta, &exp.SuccessCount, &exp.FailureCount, &exp.UseCount,
			&status, &exp.Version, &exp.SupersedesID, &exp.CreatedAt, &exp.UpdatedAt, &exp.LastUsedAt, &evidenceJSON,
			&sim,
		); err != nil {
			return nil, fmt.Errorf("scan search row: %w", err)
		}
		exp.Type = experience.Type(typ)
		exp.Scope = experience.Scope(scope)
		exp.Status = experience.Status(status)
		if len(evidenceJSON) > 0 && string(evidenceJSON) != "null" {
			if err := json.Unmarshal(evidenceJSON, &exp.Evidence); err != nil {
				return nil, fmt.Errorf("decode evidence: %w", err)
			}
		}
		out = append(out, experience.ScoredExperience{Experience: exp, Similarity: sim})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate search rows: %w", err)
	}

	filtered := out[:0]
	for _, row := range out {
		if experience.AuthorizedForSearch(row.Experience, filter.AgentID, filter.UserID, filter.Tools, filter.ScopeKey) {
			filtered = append(filtered, row)
		}
	}
	if topK > 0 && len(filtered) > topK {
		filtered = filtered[:topK]
	}
	return filtered, nil
}

func (r *ExperienceRepository) Supersede(ctx context.Context, tenantID, oldID, newID string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin supersede: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	res, err := tx.ExecContext(ctx, `
		UPDATE experiences SET status = $1, updated_at = NOW()
		WHERE tenant_id = $2 AND id = $3
	`, string(experience.StatusDeprecated), tenantID, oldID)
	if err != nil {
		return fmt.Errorf("deprecate old experience: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return experience.ErrNotFound
	}

	res, err = tx.ExecContext(ctx, `
		UPDATE experiences SET supersedes_id = $1, updated_at = NOW()
		WHERE tenant_id = $2 AND id = $3
	`, oldID, tenantID, newID)
	if err != nil {
		return fmt.Errorf("link superseding experience: %w", err)
	}
	n, _ = res.RowsAffected()
	if n == 0 {
		return experience.ErrNotFound
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit supersede: %w", err)
	}
	return nil
}

func (r *ExperienceRepository) Archive(ctx context.Context, tenantID, id string) error {
	res, err := r.db.ExecContext(ctx, `
		UPDATE experiences SET status = $1, updated_at = NOW()
		WHERE tenant_id = $2 AND id = $3
	`, string(experience.StatusArchived), tenantID, id)
	if err != nil {
		return fmt.Errorf("archive experience: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if n == 0 {
		return experience.ErrNotFound
	}
	return nil
}

type scannable interface {
	Scan(dest ...any) error
}

func scanExperience(row scannable) (experience.Experience, error) {
	var exp experience.Experience
	var typ, scope, status string
	var evidenceJSON []byte
	err := row.Scan(
		&exp.ID, &exp.TenantID, &typ, &scope, &exp.ScopeKey, &exp.Trigger, &exp.Content, &exp.SourceEpisodeID,
		&exp.Confidence, &exp.Utility, &exp.Alpha, &exp.Beta, &exp.SuccessCount, &exp.FailureCount, &exp.UseCount,
		&status, &exp.Version, &exp.SupersedesID, &exp.CreatedAt, &exp.UpdatedAt, &exp.LastUsedAt, &evidenceJSON,
	)
	if err != nil {
		return experience.Experience{}, err
	}
	exp.Type = experience.Type(typ)
	exp.Scope = experience.Scope(scope)
	exp.Status = experience.Status(status)
	if len(evidenceJSON) > 0 && string(evidenceJSON) != "null" {
		if err := json.Unmarshal(evidenceJSON, &exp.Evidence); err != nil {
			return experience.Experience{}, fmt.Errorf("decode evidence: %w", err)
		}
	}
	return exp, nil
}

func mustJSON(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		return []byte("{}")
	}
	return b
}

func formatVector(v []float32) (string, error) {
	if len(v) == 0 {
		return "", fmt.Errorf("vector is empty")
	}
	var b strings.Builder
	b.WriteByte('[')
	for i, x := range v {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(fmt.Sprintf("%g", x))
	}
	b.WriteByte(']')
	return b.String(), nil
}
