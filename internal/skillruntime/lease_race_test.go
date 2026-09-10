package skillruntime

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/agent-experience-engine/agent-experience-engine/internal/skill"
)

// TestRenewLeaseDoesNotRaceWithFencedExecutionUpdates exercises concurrent
// heartbeat RenewLease against business fenced writes without sharing *Execution.
func TestRenewLeaseDoesNotRaceWithFencedExecutionUpdates(t *testing.T) {
	store := NewMemoryExecutionStore()
	ctx := context.Background()
	until := time.Now().UTC().Add(time.Minute)
	ex, err := store.CreateExecution(ctx, skill.Execution{
		ID: "race1", TenantID: "t", SkillID: "s", SkillVersionID: "v",
		Mode: skill.ModeLive, Status: skill.ExecRunning, LeaseEpoch: 3,
		LeaseOwner: "worker", LeaseUntil: &until, StepCursor: 0,
	})
	if err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	errCh := make(chan error, 64)
	for i := 0; i < 32; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			ok, err := store.RenewLease(ctx, "t", "race1", "worker", 3, time.Now().UTC().Add(time.Minute))
			if err != nil {
				errCh <- err
				return
			}
			if !ok {
				errCh <- errStale
			}
		}()
		go func() {
			defer wg.Done()
			cur := ex
			cur.StepCursor++
			_, ok, err := store.UpdateExecutionFenced(ctx, cur, 3)
			if err != nil {
				errCh <- err
				return
			}
			if !ok {
				errCh <- errStale
			}
		}()
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil && err != errStale {
			t.Fatal(err)
		}
	}
	got, err := store.GetExecution(ctx, "t", "race1")
	if err != nil {
		t.Fatal(err)
	}
	if got.LeaseEpoch != 3 {
		t.Fatalf("epoch mutated: %d", got.LeaseEpoch)
	}
}

var errStale = errString("stale")

type errString string

func (e errString) Error() string { return string(e) }
