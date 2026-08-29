package postgres_test

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/agent-experience-engine/agent-experience-engine/internal/attempt"
	"github.com/agent-experience-engine/agent-experience-engine/internal/config"
	"github.com/agent-experience-engine/agent-experience-engine/internal/episode"
	"github.com/agent-experience-engine/agent-experience-engine/storage/postgres"
)

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	url := os.Getenv("AEE_DATABASE_URL")
	if url == "" {
		url = "postgres://aee:aee@localhost:5432/aee?sslmode=disable"
	}
	db, err := postgres.Open(config.DatabaseConfig{
		URL:             url,
		MaxOpenConns:    5,
		MaxIdleConns:    2,
		ConnMaxLifetime: time.Minute,
	})
	if err != nil {
		t.Skipf("postgres unavailable: %v", err)
	}
	if err := postgres.Migrate(db); err != nil {
		db.Close()
		t.Fatalf("migrate: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestEpisodeRepositoryTenantIsolation(t *testing.T) {
	db := openTestDB(t)
	repo := postgres.NewEpisodeRepository(db)
	svc := episode.NewService(repo)
	ctx := context.Background()

	ep, err := svc.CreateEpisode(ctx, episode.CreateEpisodeInput{
		TenantID: "tenant_pg_a",
		AgentID:  "agent",
		UserID:   "user",
		Goal:     "pg isolation",
	})
	if err != nil {
		t.Fatalf("CreateEpisode: %v", err)
	}

	_, err = svc.GetEpisode(ctx, "tenant_pg_b", ep.ID)
	if !errors.Is(err, episode.ErrNotFound) {
		t.Fatalf("cross-tenant get err = %v, want ErrNotFound", err)
	}

	_, err = svc.AddAttempt(ctx, episode.AddAttemptInput{
		TenantID:  ep.TenantID,
		EpisodeID: ep.ID,
		Action:    "step",
		Status:    attempt.StatusSuccess,
	})
	if err != nil {
		t.Fatalf("AddAttempt: %v", err)
	}

	updated, out, err := svc.CompleteEpisode(ctx, episode.CompleteEpisodeInput{
		TenantID:  ep.TenantID,
		EpisodeID: ep.ID,
		Status:    episode.StatusSuccess,
		Verified:  true,
		Verifier:  "tool",
	})
	if err != nil {
		t.Fatalf("CompleteEpisode: %v", err)
	}
	if updated.Status != episode.StatusSuccess {
		t.Fatalf("status = %s", updated.Status)
	}
	if out.EpisodeID != ep.ID {
		t.Fatalf("outcome episode_id = %s", out.EpisodeID)
	}
}
