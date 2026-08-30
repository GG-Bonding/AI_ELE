package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/agent-experience-engine/agent-experience-engine/internal/episodelearn"
	"github.com/google/uuid"
)

// EpisodeLearningRepository implements episodelearn.Repository with PostgreSQL.
type EpisodeLearningRepository struct {
	db  *sql.DB
	now func() time.Time
	id  func() string
}

// NewEpisodeLearningRepository constructs a Postgres-backed episode learning job store.
func NewEpisodeLearningRepository(db *sql.DB) *EpisodeLearningRepository {
	return &EpisodeLearningRepository{
		db:  db,
		now: time.Now().UTC,
		id:  func() string { return uuid.NewString() },
	}
}

func (r *EpisodeLearningRepository) UpsertPending(ctx context.Context, tenantID, episodeID string) (episodelearn.Job, error) {
	existing, err := r.GetByEpisode(ctx, tenantID, episodeID)
	if err == nil {
		if existing.Status == episodelearn.StatusApplied {
			return existing, nil
		}
		_, err = r.db.ExecContext(ctx, `
			UPDATE episode_learning_jobs
			SET status = $1, last_error = '', updated_at = $2
			WHERE tenant_id = $3 AND episode_id = $4
		`, string(episodelearn.StatusPending), r.now(), tenantID, episodeID)
		if err != nil {
			return episodelearn.Job{}, fmt.Errorf("reset learning job pending: %w", err)
		}
		existing.Status = episodelearn.StatusPending
		existing.LastError = ""
		existing.UpdatedAt = r.now()
		return existing, nil
	}
	if !errors.Is(err, episodelearn.ErrNotFound) {
		return episodelearn.Job{}, err
	}

	now := r.now()
	job := episodelearn.Job{
		ID:        r.id(),
		TenantID:  tenantID,
		EpisodeID: episodeID,
		Status:    episodelearn.StatusPending,
		CreatedAt: now,
		UpdatedAt: now,
	}
	_, err = r.db.ExecContext(ctx, `
		INSERT INTO episode_learning_jobs (
			id, tenant_id, episode_id, status, last_error, created_at, updated_at
		) VALUES ($1,$2,$3,$4,'',$5,$6)
	`, job.ID, job.TenantID, job.EpisodeID, string(job.Status), job.CreatedAt, job.UpdatedAt)
	if err != nil {
		return episodelearn.Job{}, fmt.Errorf("insert learning job: %w", err)
	}
	return job, nil
}

func (r *EpisodeLearningRepository) GetByEpisode(ctx context.Context, tenantID, episodeID string) (episodelearn.Job, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, tenant_id, episode_id, status, last_error, created_at, updated_at
		FROM episode_learning_jobs
		WHERE tenant_id = $1 AND episode_id = $2
	`, tenantID, episodeID)
	var job episodelearn.Job
	var status string
	err := row.Scan(&job.ID, &job.TenantID, &job.EpisodeID, &status, &job.LastError, &job.CreatedAt, &job.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return episodelearn.Job{}, episodelearn.ErrNotFound
	}
	if err != nil {
		return episodelearn.Job{}, fmt.Errorf("get learning job: %w", err)
	}
	job.Status = episodelearn.Status(status)
	return job, nil
}

func (r *EpisodeLearningRepository) MarkProcessing(ctx context.Context, tenantID, episodeID string) error {
	res, err := r.db.ExecContext(ctx, `
		UPDATE episode_learning_jobs
		SET status = $1, updated_at = $2
		WHERE tenant_id = $3 AND episode_id = $4
	`, string(episodelearn.StatusProcessing), r.now(), tenantID, episodeID)
	if err != nil {
		return fmt.Errorf("mark learning processing: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return episodelearn.ErrNotFound
	}
	return nil
}

func (r *EpisodeLearningRepository) MarkApplied(ctx context.Context, tenantID, episodeID string) error {
	res, err := r.db.ExecContext(ctx, `
		UPDATE episode_learning_jobs
		SET status = $1, last_error = '', updated_at = $2
		WHERE tenant_id = $3 AND episode_id = $4
	`, string(episodelearn.StatusApplied), r.now(), tenantID, episodeID)
	if err != nil {
		return fmt.Errorf("mark learning applied: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return episodelearn.ErrNotFound
	}
	return nil
}

func (r *EpisodeLearningRepository) MarkFailed(ctx context.Context, tenantID, episodeID, lastError string) error {
	res, err := r.db.ExecContext(ctx, `
		UPDATE episode_learning_jobs
		SET status = $1, last_error = $2, updated_at = $3
		WHERE tenant_id = $4 AND episode_id = $5
	`, string(episodelearn.StatusFailed), lastError, r.now(), tenantID, episodeID)
	if err != nil {
		return fmt.Errorf("mark learning failed: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return episodelearn.ErrNotFound
	}
	return nil
}
