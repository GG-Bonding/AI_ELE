package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/agent-experience-engine/agent-experience-engine/internal/feedback"
)

// FeedbackRepository implements feedback.Repository with PostgreSQL.
type FeedbackRepository struct {
	db *sql.DB
}

// NewFeedbackRepository constructs a Postgres-backed feedback repository.
func NewFeedbackRepository(db *sql.DB) *FeedbackRepository {
	return &FeedbackRepository{db: db}
}

func (r *FeedbackRepository) Create(ctx context.Context, fb feedback.Feedback) (feedback.Feedback, error) {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO feedbacks (
			id, tenant_id, episode_id, source, signal, reward, confidence, evidence, created_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
	`,
		fb.ID, fb.TenantID, fb.EpisodeID, string(fb.Source), fb.Signal,
		fb.Reward, fb.Confidence, fb.Evidence, fb.CreatedAt,
	)
	if err != nil {
		return feedback.Feedback{}, fmt.Errorf("insert feedback: %w", err)
	}
	return fb, nil
}

func (r *FeedbackRepository) ListByEpisode(ctx context.Context, tenantID, episodeID string) ([]feedback.Feedback, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, tenant_id, episode_id, source, signal, reward, confidence, evidence, created_at
		FROM feedbacks
		WHERE tenant_id = $1 AND episode_id = $2
		ORDER BY created_at ASC, id ASC
	`, tenantID, episodeID)
	if err != nil {
		return nil, fmt.Errorf("query feedbacks: %w", err)
	}
	defer rows.Close()

	var out []feedback.Feedback
	for rows.Next() {
		fb, err := scanFeedback(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, fb)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate feedbacks: %w", err)
	}
	return out, nil
}

func (r *FeedbackRepository) Get(ctx context.Context, tenantID, id string) (feedback.Feedback, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, tenant_id, episode_id, source, signal, reward, confidence, evidence, created_at
		FROM feedbacks
		WHERE tenant_id = $1 AND id = $2
	`, tenantID, id)
	fb, err := scanFeedback(row)
	if errors.Is(err, sql.ErrNoRows) {
		return feedback.Feedback{}, feedback.ErrNotFound
	}
	if err != nil {
		return feedback.Feedback{}, fmt.Errorf("get feedback: %w", err)
	}
	return fb, nil
}

type feedbackScanner interface {
	Scan(dest ...any) error
}

func scanFeedback(row feedbackScanner) (feedback.Feedback, error) {
	var fb feedback.Feedback
	var source string
	err := row.Scan(
		&fb.ID, &fb.TenantID, &fb.EpisodeID, &source, &fb.Signal,
		&fb.Reward, &fb.Confidence, &fb.Evidence, &fb.CreatedAt,
	)
	if err != nil {
		return feedback.Feedback{}, err
	}
	fb.Source = feedback.Source(source)
	return fb, nil
}
