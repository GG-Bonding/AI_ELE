package episodelearn_test

import (
	"context"
	"errors"
	"testing"

	"github.com/agent-experience-engine/agent-experience-engine/internal/episodelearn"
)

func TestMemoryRepositoryLifecycle(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := episodelearn.NewMemoryRepository()

	job, err := repo.UpsertPending(ctx, "t", "ep1")
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if job.Status != episodelearn.StatusPending {
		t.Fatalf("status=%s", job.Status)
	}
	if err := repo.MarkProcessing(ctx, "t", "ep1"); err != nil {
		t.Fatalf("processing: %v", err)
	}
	if err := repo.MarkFailed(ctx, "t", "ep1", "boom"); err != nil {
		t.Fatalf("failed: %v", err)
	}
	got, err := repo.GetByEpisode(ctx, "t", "ep1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Status != episodelearn.StatusFailed || got.LastError != "boom" {
		t.Fatalf("got=%+v", got)
	}
	if _, err := repo.UpsertPending(ctx, "t", "ep1"); err != nil {
		t.Fatalf("re-upsert: %v", err)
	}
	got, _ = repo.GetByEpisode(ctx, "t", "ep1")
	if got.Status != episodelearn.StatusPending || got.LastError != "" {
		t.Fatalf("retry pending=%+v", got)
	}
	if err := repo.MarkApplied(ctx, "t", "ep1"); err != nil {
		t.Fatalf("applied: %v", err)
	}
	got, _ = repo.GetByEpisode(ctx, "t", "ep1")
	if got.Status != episodelearn.StatusApplied {
		t.Fatalf("applied=%+v", got)
	}
	// Applied stays applied on upsert.
	got2, err := repo.UpsertPending(ctx, "t", "ep1")
	if err != nil {
		t.Fatalf("upsert applied: %v", err)
	}
	if got2.Status != episodelearn.StatusApplied {
		t.Fatalf("applied should stick, got %s", got2.Status)
	}
	if _, err := repo.GetByEpisode(ctx, "t", "missing"); !errors.Is(err, episodelearn.ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}
