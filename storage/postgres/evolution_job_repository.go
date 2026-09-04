package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/agent-experience-engine/agent-experience-engine/internal/evolutionjob"
	"github.com/agent-experience-engine/agent-experience-engine/internal/experience"
	"github.com/google/uuid"
)

// EvolutionJobRepository persists dirty groups and evolution jobs.
type EvolutionJobRepository struct {
	db  *sql.DB
	now func() time.Time
	id  func() string
}

// NewEvolutionJobRepository constructs a Postgres-backed evolution job store.
func NewEvolutionJobRepository(db *sql.DB) *EvolutionJobRepository {
	return &EvolutionJobRepository{
		db:  db,
		now: time.Now().UTC,
		id:  func() string { return uuid.NewString() },
	}
}

func (r *EvolutionJobRepository) MarkDirty(ctx context.Context, tenantID string, typ experience.Type, scope experience.Scope, scopeKey string) error {
	tenantID = strings.TrimSpace(tenantID)
	if tenantID == "" {
		return fmt.Errorf("tenant_id is required")
	}
	now := r.now()
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO evolution_dirty_groups (tenant_id, type, scope, scope_key, updated_at)
		VALUES ($1,$2,$3,$4,$5)
		ON CONFLICT (tenant_id, type, scope, scope_key)
		DO UPDATE SET updated_at = EXCLUDED.updated_at
	`, tenantID, string(typ), string(scope), scopeKey, now)
	if err != nil {
		return fmt.Errorf("mark evolution dirty: %w", err)
	}
	return nil
}

func (r *EvolutionJobRepository) ListDirty(ctx context.Context, limit int) ([]evolutionjob.DirtyGroup, error) {
	q := `
		SELECT tenant_id, type, scope, scope_key, updated_at
		FROM evolution_dirty_groups
		ORDER BY updated_at ASC`
	args := []any{}
	if limit > 0 {
		args = append(args, limit)
		q += fmt.Sprintf(" LIMIT $%d", len(args))
	}
	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("list evolution dirty: %w", err)
	}
	defer rows.Close()

	var out []evolutionjob.DirtyGroup
	for rows.Next() {
		var g evolutionjob.DirtyGroup
		var typ, scope string
		if err := rows.Scan(&g.TenantID, &typ, &scope, &g.ScopeKey, &g.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan dirty group: %w", err)
		}
		g.Type = experience.Type(typ)
		g.Scope = experience.Scope(scope)
		out = append(out, g)
	}
	return out, rows.Err()
}

func (r *EvolutionJobRepository) ClearDirty(ctx context.Context, g evolutionjob.DirtyGroup) error {
	_, err := r.db.ExecContext(ctx, `
		DELETE FROM evolution_dirty_groups
		WHERE tenant_id = $1 AND type = $2 AND scope = $3 AND scope_key = $4
	`, g.TenantID, string(g.Type), string(g.Scope), g.ScopeKey)
	if err != nil {
		return fmt.Errorf("clear evolution dirty: %w", err)
	}
	return nil
}

func (r *EvolutionJobRepository) UpsertPending(ctx context.Context, g evolutionjob.DirtyGroup) (evolutionjob.Job, error) {
	existing, err := r.GetByFamily(ctx, g.TenantID, g.Type, g.Scope, g.ScopeKey)
	if err == nil {
		if existing.Status == evolutionjob.StatusProcessing {
			return existing, nil
		}
		_, err = r.db.ExecContext(ctx, `
			UPDATE evolution_jobs
			SET status = $1, last_error = '', updated_at = $2
			WHERE tenant_id = $3 AND type = $4 AND scope = $5 AND scope_key = $6
		`, string(evolutionjob.StatusPending), r.now(), g.TenantID, string(g.Type), string(g.Scope), g.ScopeKey)
		if err != nil {
			return evolutionjob.Job{}, fmt.Errorf("reset evolution job pending: %w", err)
		}
		existing.Status = evolutionjob.StatusPending
		existing.LastError = ""
		existing.UpdatedAt = r.now()
		return existing, nil
	}
	if !errors.Is(err, evolutionjob.ErrNotFound) {
		return evolutionjob.Job{}, err
	}

	now := r.now()
	job := evolutionjob.Job{
		ID:        r.id(),
		TenantID:  g.TenantID,
		Type:      g.Type,
		Scope:     g.Scope,
		ScopeKey:  g.ScopeKey,
		Status:    evolutionjob.StatusPending,
		CreatedAt: now,
		UpdatedAt: now,
	}
	_, err = r.db.ExecContext(ctx, `
		INSERT INTO evolution_jobs (
			id, tenant_id, type, scope, scope_key, status, last_error, created_count, created_at, updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,'',0,$7,$8)
	`, job.ID, job.TenantID, string(job.Type), string(job.Scope), job.ScopeKey, string(job.Status), job.CreatedAt, job.UpdatedAt)
	if err != nil {
		return evolutionjob.Job{}, fmt.Errorf("insert evolution job: %w", err)
	}
	return job, nil
}

func (r *EvolutionJobRepository) GetByFamily(ctx context.Context, tenantID string, typ experience.Type, scope experience.Scope, scopeKey string) (evolutionjob.Job, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, tenant_id, type, scope, scope_key, status, last_error, created_count, created_at, updated_at
		FROM evolution_jobs
		WHERE tenant_id = $1 AND type = $2 AND scope = $3 AND scope_key = $4
	`, tenantID, string(typ), string(scope), scopeKey)
	return scanEvolutionJob(row)
}

func (r *EvolutionJobRepository) MarkProcessing(ctx context.Context, tenantID string, typ experience.Type, scope experience.Scope, scopeKey string) error {
	return r.setStatus(ctx, tenantID, typ, scope, scopeKey, evolutionjob.StatusProcessing, "", -1)
}

func (r *EvolutionJobRepository) MarkApplied(ctx context.Context, tenantID string, typ experience.Type, scope experience.Scope, scopeKey string, createdCount int) error {
	return r.setStatus(ctx, tenantID, typ, scope, scopeKey, evolutionjob.StatusApplied, "", createdCount)
}

func (r *EvolutionJobRepository) MarkFailed(ctx context.Context, tenantID string, typ experience.Type, scope experience.Scope, scopeKey, lastError string) error {
	return r.setStatus(ctx, tenantID, typ, scope, scopeKey, evolutionjob.StatusFailed, lastError, -1)
}

func (r *EvolutionJobRepository) setStatus(ctx context.Context, tenantID string, typ experience.Type, scope experience.Scope, scopeKey string, status evolutionjob.Status, lastError string, createdCount int) error {
	var res sql.Result
	var err error
	if createdCount >= 0 {
		res, err = r.db.ExecContext(ctx, `
			UPDATE evolution_jobs
			SET status = $1, last_error = $2, created_count = $3, updated_at = $4
			WHERE tenant_id = $5 AND type = $6 AND scope = $7 AND scope_key = $8
		`, string(status), lastError, createdCount, r.now(), tenantID, string(typ), string(scope), scopeKey)
	} else {
		res, err = r.db.ExecContext(ctx, `
			UPDATE evolution_jobs
			SET status = $1, last_error = $2, updated_at = $3
			WHERE tenant_id = $4 AND type = $5 AND scope = $6 AND scope_key = $7
		`, string(status), lastError, r.now(), tenantID, string(typ), string(scope), scopeKey)
	}
	if err != nil {
		return fmt.Errorf("update evolution job status: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return evolutionjob.ErrNotFound
	}
	return nil
}

func (r *EvolutionJobRepository) ListStaleProcessing(ctx context.Context, cutoff time.Time) ([]evolutionjob.Job, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, tenant_id, type, scope, scope_key, status, last_error, created_count, created_at, updated_at
		FROM evolution_jobs
		WHERE status = $1 AND updated_at < $2
		ORDER BY updated_at ASC
	`, string(evolutionjob.StatusProcessing), cutoff)
	if err != nil {
		return nil, fmt.Errorf("list stale evolution jobs: %w", err)
	}
	defer rows.Close()

	var out []evolutionjob.Job
	for rows.Next() {
		job, err := scanEvolutionJob(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, job)
	}
	return out, rows.Err()
}

func scanEvolutionJob(row interface{ Scan(dest ...any) error }) (evolutionjob.Job, error) {
	var job evolutionjob.Job
	var typ, scope, status string
	err := row.Scan(
		&job.ID, &job.TenantID, &typ, &scope, &job.ScopeKey, &status,
		&job.LastError, &job.CreatedCount, &job.CreatedAt, &job.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return evolutionjob.Job{}, evolutionjob.ErrNotFound
	}
	if err != nil {
		return evolutionjob.Job{}, fmt.Errorf("scan evolution job: %w", err)
	}
	job.Type = experience.Type(typ)
	job.Scope = experience.Scope(scope)
	job.Status = evolutionjob.Status(status)
	return job, nil
}
