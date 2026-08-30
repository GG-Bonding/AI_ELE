package experience_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/agent-experience-engine/agent-experience-engine/internal/experience"
)

func TestMemoryUpdateOptimisticLock(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := experience.NewMemoryRepository()
	now := time.Now().UTC()
	created, err := repo.Create(ctx, experience.Experience{
		ID: "e1", TenantID: "t", Type: experience.TypeProcedural,
		Scope: experience.ScopeTool, ScopeKey: "jira",
		Trigger: "x", Content: "y", Confidence: 0.9, Utility: 0.5,
		Alpha: 1, Beta: 1, Status: experience.StatusActive, Version: 1,
		CreatedAt: now, UpdatedAt: now,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	a := created
	b := created
	a.Utility = 0.6
	if _, err := repo.Update(ctx, a); err != nil {
		t.Fatalf("first update: %v", err)
	}
	b.Utility = 0.7
	if _, err := repo.Update(ctx, b); !errors.Is(err, experience.ErrConflict) {
		t.Fatalf("want ErrConflict, got %v", err)
	}
}

func TestConcurrentBetaUpdatesSerialize(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := experience.NewMemoryRepository()
	now := time.Now().UTC()
	if _, err := repo.Create(ctx, experience.Experience{
		ID: "e2", TenantID: "t", Type: experience.TypeProcedural,
		Scope: experience.ScopeTool, ScopeKey: "jira",
		Trigger: "x", Content: "y", Confidence: 0.9, Utility: 0.5,
		Alpha: 1, Beta: 1, Status: experience.StatusActive, Version: 1,
		CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("create: %v", err)
	}

	const workers = 20
	var conflicts atomic.Int64
	var wg sync.WaitGroup
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			for attempt := 0; attempt < 32; attempt++ {
				exp, err := repo.Get(ctx, "t", "e2")
				if err != nil {
					t.Errorf("get: %v", err)
					return
				}
				updated, err := experience.ApplyBetaUpdate(exp, 1.0, 1.0, now)
				if err != nil {
					t.Errorf("beta: %v", err)
					return
				}
				if _, err := repo.Update(ctx, updated); err != nil {
					if errors.Is(err, experience.ErrConflict) {
						conflicts.Add(1)
						continue
					}
					t.Errorf("update: %v", err)
					return
				}
				return
			}
			t.Errorf("exhausted retries")
		}()
	}
	wg.Wait()

	got, err := repo.Get(ctx, "t", "e2")
	if err != nil {
		t.Fatalf("final get: %v", err)
	}
	// Each success adds +1 to alpha from base 1 → alpha should be 1+workers.
	wantAlpha := 1.0 + float64(workers)
	if got.Alpha != wantAlpha {
		t.Fatalf("alpha=%v want %v (conflicts=%d version=%d)", got.Alpha, wantAlpha, conflicts.Load(), got.Version)
	}
	if got.Version != int64(1+workers) {
		t.Fatalf("version=%d want %d", got.Version, 1+workers)
	}
}
