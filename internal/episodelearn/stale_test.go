package episodelearn_test

import (
	"context"
	"testing"
	"time"

	"github.com/agent-experience-engine/agent-experience-engine/internal/attempt"
	"github.com/agent-experience-engine/agent-experience-engine/internal/episode"
	"github.com/agent-experience-engine/agent-experience-engine/internal/episodelearn"
	"github.com/agent-experience-engine/agent-experience-engine/internal/experience"
	"github.com/agent-experience-engine/agent-experience-engine/internal/extractor"
	"github.com/agent-experience-engine/agent-experience-engine/internal/outcome"
)

type stubExtractor struct{}

func (stubExtractor) Extract(context.Context, extractor.ExtractInput) ([]experience.Candidate, error) {
	return []experience.Candidate{{
		Type: experience.TypeProcedural, Trigger: "t", Content: "c",
		Confidence: 0.9, Scope: experience.ScopeTenant,
	}}, nil
}

type stubStore struct {
	calls int
}

func (s *stubStore) StoreCandidatesWithOptions(
	context.Context, string, string, []experience.Candidate, experience.StoreOptions,
) (experience.StoreCandidatesResult, error) {
	s.calls++
	return experience.StoreCandidatesResult{}, nil
}

func TestRetryRecoversStaleProcessing(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	jobs := episodelearn.NewMemoryRepository()
	store := &stubStore{}
	proc, err := episodelearn.NewProcessor(stubExtractor{}, store, jobs)
	if err != nil {
		t.Fatal(err)
	}
	proc = proc.WithStaleAfter(time.Millisecond)

	ep := episode.Episode{ID: "ep1", TenantID: "t"}
	if _, err := jobs.UpsertPending(ctx, "t", "ep1"); err != nil {
		t.Fatal(err)
	}
	if err := jobs.MarkProcessing(ctx, "t", "ep1"); err != nil {
		t.Fatal(err)
	}

	// Fresh PROCESSING must not be stolen.
	fresh, err := proc.Retry(ctx, "t", ep, nil, outcome.Outcome{})
	if err != nil {
		t.Fatal(err)
	}
	if fresh.LearningStatus != episodelearn.StatusProcessing {
		t.Fatalf("fresh status=%s", fresh.LearningStatus)
	}
	if store.calls != 0 {
		t.Fatal("fresh PROCESSING should not re-run pipeline")
	}

	time.Sleep(3 * time.Millisecond)
	got, err := proc.Retry(ctx, "t", ep, []attempt.Attempt{}, outcome.Outcome{ID: "out1"})
	if err != nil {
		t.Fatal(err)
	}
	if got.LearningStatus != episodelearn.StatusApplied {
		t.Fatalf("stale retry status=%s err=%s", got.LearningStatus, got.LearningLastError)
	}
	if store.calls != 1 {
		t.Fatalf("store calls=%d want 1", store.calls)
	}
}

func TestRecoverStaleJobsSweep(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	jobs := episodelearn.NewMemoryRepository()
	proc, err := episodelearn.NewProcessor(stubExtractor{}, &stubStore{}, jobs)
	if err != nil {
		t.Fatal(err)
	}
	proc = proc.WithStaleAfter(time.Millisecond)

	if _, err := jobs.UpsertPending(ctx, "t", "epA"); err != nil {
		t.Fatal(err)
	}
	if err := jobs.MarkProcessing(ctx, "t", "epA"); err != nil {
		t.Fatal(err)
	}
	if _, err := jobs.UpsertPending(ctx, "t", "epB"); err != nil {
		t.Fatal(err)
	}
	// epB stays PENDING — not recovered.

	time.Sleep(3 * time.Millisecond)
	n, err := proc.RecoverStaleJobs(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("recovered=%d want 1", n)
	}
	a, err := jobs.GetByEpisode(ctx, "t", "epA")
	if err != nil {
		t.Fatal(err)
	}
	if a.Status != episodelearn.StatusFailed {
		t.Fatalf("epA status=%s", a.Status)
	}
	if a.LastError == "" {
		t.Fatal("expected recovery error message")
	}
	b, err := jobs.GetByEpisode(ctx, "t", "epB")
	if err != nil {
		t.Fatal(err)
	}
	if b.Status != episodelearn.StatusPending {
		t.Fatalf("epB status=%s", b.Status)
	}
}
