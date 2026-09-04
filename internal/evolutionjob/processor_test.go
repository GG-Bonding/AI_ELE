package evolutionjob_test

import (
	"context"
	"testing"
	"time"

	"github.com/agent-experience-engine/agent-experience-engine/internal/evolutionjob"
	"github.com/agent-experience-engine/agent-experience-engine/internal/experience"
)

func TestProcessGroupCreatesPatternFromDirtyFamily(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := experience.NewMemoryRepository()
	patterns := experience.NewMemoryPatternRepository()
	expSvc := experience.NewService(repo).WithPatterns(patterns)
	jobs := evolutionjob.NewMemoryRepository()
	proc, err := evolutionjob.NewProcessor(expSvc, jobs)
	if err != nil {
		t.Fatal(err)
	}

	base := []float32{1, 0, 0, 0}
	for i, ep := range []string{"ep1", "ep2", "ep3"} {
		emb := append([]float32(nil), base...)
		if i > 0 {
			emb[0] = 0.98
			emb[1] = 0.1 * float32(i)
		}
		exp, err := expSvc.Create(ctx, experience.CreateInput{
			TenantID: "t", Type: experience.TypeProcedural, Scope: experience.ScopeTool, ScopeKey: "jira",
			Trigger: "create jira issue", Content: "Resolve project key before creating the issue.",
			SourceEpisodeID: ep, Confidence: 0.9, Embedding: emb,
			Evidence: experience.Evidence{SourceEpisodeID: ep, SupportEpisodeIDs: []string{ep}},
			Status:   experience.StatusActive,
		})
		if err != nil {
			t.Fatal(err)
		}
		exp.Utility = 0.85
		if _, err := repo.Update(ctx, exp); err != nil {
			t.Fatal(err)
		}
	}

	if err := jobs.MarkDirty(ctx, "t", experience.TypeProcedural, experience.ScopeTool, "jira"); err != nil {
		t.Fatal(err)
	}
	n, err := proc.Sweep(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("sweep n=%d", n)
	}
	job, err := jobs.GetByFamily(ctx, "t", experience.TypeProcedural, experience.ScopeTool, "jira")
	if err != nil {
		t.Fatal(err)
	}
	if job.Status != evolutionjob.StatusApplied {
		t.Fatalf("status=%s err=%s", job.Status, job.LastError)
	}
	if job.CreatedCount != 1 {
		t.Fatalf("created_count=%d", job.CreatedCount)
	}
	dirty, err := jobs.ListDirty(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(dirty) != 0 {
		t.Fatalf("dirty should be cleared: %#v", dirty)
	}
}

func TestRecoverStaleJobsRequeuesDirty(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	jobs := evolutionjob.NewMemoryRepository()
	expSvc := experience.NewService(experience.NewMemoryRepository()).WithPatterns(experience.NewMemoryPatternRepository())
	proc, err := evolutionjob.NewProcessor(expSvc, jobs)
	if err != nil {
		t.Fatal(err)
	}

	g := evolutionjob.DirtyGroup{
		TenantID: "t", Type: experience.TypeProcedural, Scope: experience.ScopeTool, ScopeKey: "jira",
	}
	if _, err := jobs.UpsertPending(ctx, g); err != nil {
		t.Fatal(err)
	}
	if err := jobs.MarkProcessing(ctx, g.TenantID, g.Type, g.Scope, g.ScopeKey); err != nil {
		t.Fatal(err)
	}
	// Backdate PROCESSING so it is stale relative to DefaultStaleProcessingAfter.
	if err := jobs.BackdateUpdatedAt(ctx, g.TenantID, g.Type, g.Scope, g.ScopeKey, time.Now().UTC().Add(-2*time.Hour)); err != nil {
		t.Fatal(err)
	}

	n, err := proc.RecoverStaleJobs(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("recovered=%d", n)
	}
	job, err := jobs.GetByFamily(ctx, g.TenantID, g.Type, g.Scope, g.ScopeKey)
	if err != nil {
		t.Fatal(err)
	}
	if job.Status != evolutionjob.StatusFailed {
		t.Fatalf("status=%s", job.Status)
	}
	dirty, err := jobs.ListDirty(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(dirty) != 1 {
		t.Fatalf("want re-queued dirty, got %#v", dirty)
	}
}
