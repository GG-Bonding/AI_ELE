package action_test

import (
	"context"
	"strings"
	"testing"

	"github.com/agent-experience-engine/agent-experience-engine/internal/action"
	"github.com/agent-experience-engine/agent-experience-engine/internal/episode"
)

func TestRecordActionAutoLinksFromContext(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	epSvc := episode.NewService(episode.NewMemoryRepository())
	ep, err := epSvc.CreateEpisode(ctx, episode.CreateEpisodeInput{
		TenantID: "t", AgentID: "a", UserID: "u", Goal: "jira",
	})
	if err != nil {
		t.Fatal(err)
	}

	repo := action.NewMemoryRepository()
	svc := action.NewService(repo, epSvc).WithContexts(action.SnapshotFunc(
		func(_ context.Context, tenantID, contextID string) (action.ContextSnapshot, error) {
			if tenantID != "t" || contextID != "ctx_1" {
				return action.ContextSnapshot{}, action.ErrContextNotFound
			}
			return action.ContextSnapshot{
				ID: "ctx_1", TenantID: "t", EpisodeID: ep.ID,
				ExperienceIDs: []string{"e1", "e2"},
				PatternIDs:    []string{"p1"},
			}, nil
		},
	))

	a, err := svc.RecordAction(ctx, action.RecordInput{
		TenantID: "t", EpisodeID: ep.ID, Type: action.TypeToolCall,
		ToolName: "jira.create_issue", Status: action.StatusSuccess,
		ContextID: "ctx_1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if a.ContextID != "ctx_1" {
		t.Fatalf("context_id=%q", a.ContextID)
	}

	links, err := svc.ListLinks(ctx, "t", ep.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(links) != 2 {
		t.Fatalf("experience links=%d want 2 %#v", len(links), links)
	}
	for _, link := range links {
		if !strings.HasPrefix(link.Evidence, "context:ctx_1") {
			t.Fatalf("evidence=%q", link.Evidence)
		}
		if link.ActionID != a.ID {
			t.Fatalf("action_id=%s want %s", link.ActionID, a.ID)
		}
	}

	patLinks, err := svc.ListPatternLinks(ctx, "t", ep.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(patLinks) != 1 || patLinks[0].PatternID != "p1" {
		t.Fatalf("pattern links=%#v", patLinks)
	}
}

func TestRecordActionRejectsCrossEpisodeContext(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	epSvc := episode.NewService(episode.NewMemoryRepository())
	ep, err := epSvc.CreateEpisode(ctx, episode.CreateEpisodeInput{
		TenantID: "t", AgentID: "a", UserID: "u", Goal: "jira",
	})
	if err != nil {
		t.Fatal(err)
	}
	svc := action.NewService(action.NewMemoryRepository(), epSvc).WithContexts(action.SnapshotFunc(
		func(_ context.Context, _, _ string) (action.ContextSnapshot, error) {
			return action.ContextSnapshot{
				ID: "ctx_x", TenantID: "t", EpisodeID: "other-ep",
				ExperienceIDs: []string{"e1"},
			}, nil
		},
	))
	_, err = svc.RecordAction(ctx, action.RecordInput{
		TenantID: "t", EpisodeID: ep.ID, Type: action.TypeAnswer,
		Status: action.StatusSuccess, ContextID: "ctx_x",
	})
	if err == nil || !strings.Contains(err.Error(), "belongs to episode") {
		t.Fatalf("want episode mismatch error, got %v", err)
	}
}

func TestRecordActionMissingContext(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	epSvc := episode.NewService(episode.NewMemoryRepository())
	ep, err := epSvc.CreateEpisode(ctx, episode.CreateEpisodeInput{
		TenantID: "t", AgentID: "a", UserID: "u", Goal: "g",
	})
	if err != nil {
		t.Fatal(err)
	}
	svc := action.NewService(action.NewMemoryRepository(), epSvc).WithContexts(action.SnapshotFunc(
		func(_ context.Context, _, _ string) (action.ContextSnapshot, error) {
			return action.ContextSnapshot{}, action.ErrContextNotFound
		},
	))
	_, err = svc.RecordAction(ctx, action.RecordInput{
		TenantID: "t", EpisodeID: ep.ID, Type: action.TypeAnswer,
		Status: action.StatusSuccess, ContextID: "missing",
	})
	if err == nil {
		t.Fatal("expected error")
	}
}
