package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/agent-experience-engine/agent-experience-engine/internal/learning"
	"github.com/jackc/pgx/v5/pgconn"
)

// LearningEventRepository implements learning.EventRepository with PostgreSQL.
type LearningEventRepository struct {
	db *sql.DB
}

// NewLearningEventRepository constructs a Postgres-backed learning event store.
func NewLearningEventRepository(db *sql.DB) *LearningEventRepository {
	return &LearningEventRepository{db: db}
}

func (r *LearningEventRepository) Create(ctx context.Context, ev learning.Event) (learning.Event, error) {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO learning_events (
			id, tenant_id, feedback_id, episode_id, experience_id,
			normalized_reward, confidence, credit, effective_reward,
			status, created_at, applied_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
	`,
		ev.ID, ev.TenantID, ev.FeedbackID, ev.EpisodeID, ev.ExperienceID,
		ev.NormalizedReward, ev.Confidence, ev.Credit, ev.EffectiveReward,
		string(ev.Status), ev.CreatedAt, ev.AppliedAt,
	)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return learning.Event{}, learning.ErrDuplicateEvent
		}
		return learning.Event{}, fmt.Errorf("insert learning event: %w", err)
	}
	return ev, nil
}

func (r *LearningEventRepository) GetByFeedbackExperience(ctx context.Context, tenantID, feedbackID, experienceID string) (learning.Event, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, tenant_id, feedback_id, episode_id, experience_id,
		       normalized_reward, confidence, credit, effective_reward,
		       status, created_at, applied_at
		FROM learning_events
		WHERE tenant_id = $1 AND feedback_id = $2 AND experience_id = $3
	`, tenantID, feedbackID, experienceID)
	ev, err := scanLearningEvent(row)
	if errors.Is(err, sql.ErrNoRows) {
		return learning.Event{}, learning.ErrEventNotFound
	}
	if err != nil {
		return learning.Event{}, fmt.Errorf("get learning event: %w", err)
	}
	return ev, nil
}

func (r *LearningEventRepository) ListByFeedback(ctx context.Context, tenantID, feedbackID string) ([]learning.Event, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, tenant_id, feedback_id, episode_id, experience_id,
		       normalized_reward, confidence, credit, effective_reward,
		       status, created_at, applied_at
		FROM learning_events
		WHERE tenant_id = $1 AND feedback_id = $2
		ORDER BY created_at ASC, id ASC
	`, tenantID, feedbackID)
	if err != nil {
		return nil, fmt.Errorf("list learning events: %w", err)
	}
	defer rows.Close()
	var out []learning.Event
	for rows.Next() {
		ev, err := scanLearningEvent(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, ev)
	}
	return out, rows.Err()
}

func (r *LearningEventRepository) MarkApplied(ctx context.Context, tenantID, id string, appliedAt time.Time) error {
	res, err := r.db.ExecContext(ctx, `
		UPDATE learning_events
		SET status = $1, applied_at = $2
		WHERE tenant_id = $3 AND id = $4
	`, string(learning.EventApplied), appliedAt, tenantID, id)
	if err != nil {
		return fmt.Errorf("mark learning event applied: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return learning.ErrEventNotFound
	}
	return nil
}

func (r *LearningEventRepository) MarkFailed(ctx context.Context, tenantID, id string) error {
	res, err := r.db.ExecContext(ctx, `
		UPDATE learning_events SET status = $1
		WHERE tenant_id = $2 AND id = $3
	`, string(learning.EventFailed), tenantID, id)
	if err != nil {
		return fmt.Errorf("mark learning event failed: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return learning.ErrEventNotFound
	}
	return nil
}

type learningEventScanner interface {
	Scan(dest ...any) error
}

func scanLearningEvent(row learningEventScanner) (learning.Event, error) {
	var ev learning.Event
	var status string
	err := row.Scan(
		&ev.ID, &ev.TenantID, &ev.FeedbackID, &ev.EpisodeID, &ev.ExperienceID,
		&ev.NormalizedReward, &ev.Confidence, &ev.Credit, &ev.EffectiveReward,
		&status, &ev.CreatedAt, &ev.AppliedAt,
	)
	if err != nil {
		return learning.Event{}, err
	}
	ev.Status = learning.EventStatus(status)
	return ev, nil
}
