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

// PatternLearningEventRepository implements learning.PatternEventRepository.
type PatternLearningEventRepository struct {
	db *sql.DB
}

// NewPatternLearningEventRepository constructs a Postgres-backed pattern event store.
func NewPatternLearningEventRepository(db *sql.DB) *PatternLearningEventRepository {
	return &PatternLearningEventRepository{db: db}
}

func (r *PatternLearningEventRepository) Create(ctx context.Context, ev learning.PatternEvent) (learning.PatternEvent, error) {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO pattern_learning_events (
			id, tenant_id, feedback_id, episode_id, pattern_id, source_type, source_learning_event_id,
			normalized_reward, confidence, credit, effective_reward, status, created_at, applied_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)
	`,
		ev.ID, ev.TenantID, ev.FeedbackID, ev.EpisodeID, ev.PatternID, string(ev.SourceType), ev.SourceLearningEventID,
		ev.NormalizedReward, ev.Confidence, ev.Credit, ev.EffectiveReward, string(ev.Status), ev.CreatedAt, ev.AppliedAt,
	)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return learning.PatternEvent{}, learning.ErrDuplicatePatternEvent
		}
		return learning.PatternEvent{}, fmt.Errorf("insert pattern learning event: %w", err)
	}
	return ev, nil
}

func (r *PatternLearningEventRepository) GetByMemberSource(ctx context.Context, tenantID, sourceLearningEventID, patternID string) (learning.PatternEvent, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, tenant_id, feedback_id, episode_id, pattern_id, source_type, source_learning_event_id,
		       normalized_reward, confidence, credit, effective_reward, status, created_at, applied_at
		FROM pattern_learning_events
		WHERE tenant_id = $1 AND source_learning_event_id = $2 AND pattern_id = $3
		  AND source_type = $4
	`, tenantID, sourceLearningEventID, patternID, string(learning.PatternSourceMember))
	ev, err := scanPatternLearningEvent(row)
	if errors.Is(err, sql.ErrNoRows) {
		return learning.PatternEvent{}, learning.ErrPatternEventNotFound
	}
	if err != nil {
		return learning.PatternEvent{}, fmt.Errorf("get pattern learning event by member source: %w", err)
	}
	return ev, nil
}

func (r *PatternLearningEventRepository) GetByFeedbackPatternSource(ctx context.Context, tenantID, feedbackID, patternID string, source learning.PatternEventSource) (learning.PatternEvent, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, tenant_id, feedback_id, episode_id, pattern_id, source_type, source_learning_event_id,
		       normalized_reward, confidence, credit, effective_reward, status, created_at, applied_at
		FROM pattern_learning_events
		WHERE tenant_id = $1 AND feedback_id = $2 AND pattern_id = $3 AND source_type = $4
	`, tenantID, feedbackID, patternID, string(source))
	ev, err := scanPatternLearningEvent(row)
	if errors.Is(err, sql.ErrNoRows) {
		return learning.PatternEvent{}, learning.ErrPatternEventNotFound
	}
	if err != nil {
		return learning.PatternEvent{}, fmt.Errorf("get pattern learning event by feedback source: %w", err)
	}
	return ev, nil
}

func (r *PatternLearningEventRepository) ListByFeedback(ctx context.Context, tenantID, feedbackID string) ([]learning.PatternEvent, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, tenant_id, feedback_id, episode_id, pattern_id, source_type, source_learning_event_id,
		       normalized_reward, confidence, credit, effective_reward, status, created_at, applied_at
		FROM pattern_learning_events
		WHERE tenant_id = $1 AND feedback_id = $2
		ORDER BY created_at ASC, id ASC
	`, tenantID, feedbackID)
	if err != nil {
		return nil, fmt.Errorf("list pattern learning events: %w", err)
	}
	defer rows.Close()
	var out []learning.PatternEvent
	for rows.Next() {
		ev, err := scanPatternLearningEvent(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, ev)
	}
	return out, rows.Err()
}

func (r *PatternLearningEventRepository) MarkApplied(ctx context.Context, tenantID, id string, appliedAt time.Time) error {
	res, err := r.db.ExecContext(ctx, `
		UPDATE pattern_learning_events
		SET status = $1, applied_at = $2
		WHERE tenant_id = $3 AND id = $4
	`, string(learning.EventApplied), appliedAt, tenantID, id)
	if err != nil {
		return fmt.Errorf("mark pattern learning event applied: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return learning.ErrPatternEventNotFound
	}
	return nil
}

func (r *PatternLearningEventRepository) MarkFailed(ctx context.Context, tenantID, id string) error {
	res, err := r.db.ExecContext(ctx, `
		UPDATE pattern_learning_events SET status = $1
		WHERE tenant_id = $2 AND id = $3
	`, string(learning.EventFailed), tenantID, id)
	if err != nil {
		return fmt.Errorf("mark pattern learning event failed: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return learning.ErrPatternEventNotFound
	}
	return nil
}

type patternLearningEventScanner interface {
	Scan(dest ...any) error
}

func scanPatternLearningEvent(row patternLearningEventScanner) (learning.PatternEvent, error) {
	var ev learning.PatternEvent
	var sourceType, status string
	err := row.Scan(
		&ev.ID, &ev.TenantID, &ev.FeedbackID, &ev.EpisodeID, &ev.PatternID, &sourceType, &ev.SourceLearningEventID,
		&ev.NormalizedReward, &ev.Confidence, &ev.Credit, &ev.EffectiveReward, &status, &ev.CreatedAt, &ev.AppliedAt,
	)
	if err != nil {
		return learning.PatternEvent{}, err
	}
	ev.SourceType = learning.PatternEventSource(sourceType)
	ev.Status = learning.EventStatus(status)
	return ev, nil
}
