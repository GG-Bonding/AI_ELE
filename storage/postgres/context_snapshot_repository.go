package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/agent-experience-engine/agent-experience-engine/internal/contextx"
)

// ContextSnapshotRepository persists context snapshots in PostgreSQL.
type ContextSnapshotRepository struct {
	db *sql.DB
}

// NewContextSnapshotRepository constructs a Postgres-backed snapshot store.
func NewContextSnapshotRepository(db *sql.DB) *ContextSnapshotRepository {
	return &ContextSnapshotRepository{db: db}
}

func (r *ContextSnapshotRepository) Create(ctx context.Context, snap contextx.Snapshot) (contextx.Snapshot, error) {
	expJSON, err := json.Marshal(nonNilStrings(snap.ExperienceIDs))
	if err != nil {
		return contextx.Snapshot{}, fmt.Errorf("marshal experience_ids: %w", err)
	}
	patJSON, err := json.Marshal(nonNilStrings(snap.PatternIDs))
	if err != nil {
		return contextx.Snapshot{}, fmt.Errorf("marshal pattern_ids: %w", err)
	}
	_, err = r.db.ExecContext(ctx, `
		INSERT INTO context_snapshots (
			id, tenant_id, episode_id, agent_id, user_id, task,
			experience_ids, pattern_ids, created_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
	`,
		snap.ID, snap.TenantID, snap.EpisodeID, snap.AgentID, snap.UserID, snap.Task,
		expJSON, patJSON, snap.CreatedAt,
	)
	if err != nil {
		return contextx.Snapshot{}, fmt.Errorf("insert context snapshot: %w", err)
	}
	return snap, nil
}

func (r *ContextSnapshotRepository) Get(ctx context.Context, tenantID, id string) (contextx.Snapshot, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, tenant_id, episode_id, agent_id, user_id, task,
		       experience_ids, pattern_ids, created_at
		FROM context_snapshots
		WHERE tenant_id = $1 AND id = $2
	`, strings.TrimSpace(tenantID), strings.TrimSpace(id))

	var snap contextx.Snapshot
	var expJSON, patJSON []byte
	if err := row.Scan(
		&snap.ID, &snap.TenantID, &snap.EpisodeID, &snap.AgentID, &snap.UserID, &snap.Task,
		&expJSON, &patJSON, &snap.CreatedAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return contextx.Snapshot{}, contextx.ErrSnapshotNotFound
		}
		return contextx.Snapshot{}, fmt.Errorf("get context snapshot: %w", err)
	}
	if err := json.Unmarshal(expJSON, &snap.ExperienceIDs); err != nil {
		return contextx.Snapshot{}, fmt.Errorf("unmarshal experience_ids: %w", err)
	}
	if err := json.Unmarshal(patJSON, &snap.PatternIDs); err != nil {
		return contextx.Snapshot{}, fmt.Errorf("unmarshal pattern_ids: %w", err)
	}
	return snap, nil
}

func nonNilStrings(in []string) []string {
	if in == nil {
		return []string{}
	}
	return in
}
