package episode

import (
	"context"
	"errors"
	"testing"

	"github.com/agent-experience-engine/agent-experience-engine/internal/attempt"
)

func TestCreateEpisodeRequiresTenant(t *testing.T) {
	t.Parallel()
	svc := NewService(NewMemoryRepository())
	_, err := svc.CreateEpisode(context.Background(), CreateEpisodeInput{
		AgentID: "agent",
		UserID:  "user",
		Goal:    "do thing",
	})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("err = %v, want ErrInvalidInput", err)
	}
}

func TestLifecycleTaskAttemptAttemptOutcome(t *testing.T) {
	t.Parallel()
	svc := NewService(NewMemoryRepository())
	ctx := context.Background()

	ep, err := svc.CreateEpisode(ctx, CreateEpisodeInput{
		TenantID: "tenant_a",
		AgentID:  "agent_01",
		UserID:   "user_01",
		TaskType: "jira.create_issue",
		Goal:     "Create Jira issue",
		Input:    `project="Payment"`,
	})
	if err != nil {
		t.Fatalf("CreateEpisode: %v", err)
	}
	if ep.Status != StatusRunning {
		t.Fatalf("status = %s, want RUNNING", ep.Status)
	}

	a1, err := svc.AddAttempt(ctx, AddAttemptInput{
		TenantID:     ep.TenantID,
		EpisodeID:    ep.ID,
		Hypothesis:   "Payment is the project key",
		Action:       "create_issue",
		ToolName:     "jira.create_issue",
		Status:       attempt.StatusFailed,
		ErrorCode:    "INVALID_PROJECT_KEY",
		ErrorMessage: "project not found",
	})
	if err != nil {
		t.Fatalf("AddAttempt 1: %v", err)
	}
	if a1.Sequence != 1 {
		t.Fatalf("sequence = %d, want 1", a1.Sequence)
	}

	a2, err := svc.AddAttempt(ctx, AddAttemptInput{
		TenantID:  ep.TenantID,
		EpisodeID: ep.ID,
		Action:    "search_projects",
		ToolName:  "jira.search_projects",
		Status:    attempt.StatusSuccess,
	})
	if err != nil {
		t.Fatalf("AddAttempt 2: %v", err)
	}
	if a2.Sequence != 2 {
		t.Fatalf("sequence = %d, want 2", a2.Sequence)
	}

	a3, err := svc.AddAttempt(ctx, AddAttemptInput{
		TenantID:  ep.TenantID,
		EpisodeID: ep.ID,
		Action:    "create_issue with PAY",
		ToolName:  "jira.create_issue",
		Status:    attempt.StatusSuccess,
	})
	if err != nil {
		t.Fatalf("AddAttempt 3: %v", err)
	}
	if a3.Sequence != 3 {
		t.Fatalf("sequence = %d, want 3", a3.Sequence)
	}

	updated, out, err := svc.CompleteEpisode(ctx, CompleteEpisodeInput{
		TenantID:  ep.TenantID,
		EpisodeID: ep.ID,
		Status:    StatusSuccess,
		Verified:  true,
		Verifier:  "tool",
		Metrics:   map[string]float64{"attempts": 3},
	})
	if err != nil {
		t.Fatalf("CompleteEpisode: %v", err)
	}
	if updated.Status != StatusSuccess {
		t.Fatalf("episode status = %s", updated.Status)
	}
	if updated.CompletedAt == nil {
		t.Fatal("expected completed_at")
	}
	if out.Status != string(StatusSuccess) {
		t.Fatalf("outcome status = %s", out.Status)
	}

	attempts, err := svc.ListAttempts(ctx, ep.TenantID, ep.ID)
	if err != nil {
		t.Fatalf("ListAttempts: %v", err)
	}
	if len(attempts) != 3 {
		t.Fatalf("attempts = %d, want 3", len(attempts))
	}
}

func TestTenantIsolationPreventsCrossTenantGet(t *testing.T) {
	t.Parallel()
	svc := NewService(NewMemoryRepository())
	ctx := context.Background()

	ep, err := svc.CreateEpisode(ctx, CreateEpisodeInput{
		TenantID: "tenant_a",
		AgentID:  "agent",
		UserID:   "user",
		Goal:     "secret task",
	})
	if err != nil {
		t.Fatalf("CreateEpisode: %v", err)
	}

	_, err = svc.GetEpisode(ctx, "tenant_b", ep.ID)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}

	_, err = svc.AddAttempt(ctx, AddAttemptInput{
		TenantID:  "tenant_b",
		EpisodeID: ep.ID,
		Action:    "hack",
		Status:    attempt.StatusSuccess,
	})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("AddAttempt err = %v, want ErrNotFound", err)
	}

	_, _, err = svc.CompleteEpisode(ctx, CompleteEpisodeInput{
		TenantID:  "tenant_b",
		EpisodeID: ep.ID,
		Status:    StatusSuccess,
	})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("CompleteEpisode err = %v, want ErrNotFound", err)
	}
}

func TestCannotAddAttemptAfterComplete(t *testing.T) {
	t.Parallel()
	svc := NewService(NewMemoryRepository())
	ctx := context.Background()

	ep, err := svc.CreateEpisode(ctx, CreateEpisodeInput{
		TenantID: "t",
		AgentID:  "a",
		UserID:   "u",
		Goal:     "g",
	})
	if err != nil {
		t.Fatalf("CreateEpisode: %v", err)
	}
	if _, _, err := svc.CompleteEpisode(ctx, CompleteEpisodeInput{
		TenantID:  ep.TenantID,
		EpisodeID: ep.ID,
		Status:    StatusFailed,
	}); err != nil {
		t.Fatalf("CompleteEpisode: %v", err)
	}

	_, err = svc.AddAttempt(ctx, AddAttemptInput{
		TenantID:  ep.TenantID,
		EpisodeID: ep.ID,
		Action:    "late",
		Status:    attempt.StatusSuccess,
	})
	if !errors.Is(err, ErrAlreadyCompleted) {
		t.Fatalf("err = %v, want ErrAlreadyCompleted", err)
	}
}

func TestCompleteRequiresTerminalStatus(t *testing.T) {
	t.Parallel()
	svc := NewService(NewMemoryRepository())
	ctx := context.Background()

	ep, err := svc.CreateEpisode(ctx, CreateEpisodeInput{
		TenantID: "t",
		AgentID:  "a",
		UserID:   "u",
		Goal:     "g",
	})
	if err != nil {
		t.Fatalf("CreateEpisode: %v", err)
	}

	_, _, err = svc.CompleteEpisode(ctx, CompleteEpisodeInput{
		TenantID:  ep.TenantID,
		EpisodeID: ep.ID,
		Status:    StatusRunning,
	})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("err = %v, want ErrInvalidInput", err)
	}
}
