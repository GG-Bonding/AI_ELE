package action_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/agent-experience-engine/agent-experience-engine/internal/action"
	"github.com/agent-experience-engine/agent-experience-engine/internal/episode"
)

func TestRecordActionAndLink(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	epSvc := episode.NewService(episode.NewMemoryRepository())
	ep, err := epSvc.CreateEpisode(ctx, episode.CreateEpisodeInput{
		TenantID: "t1",
		AgentID:  "a1",
		UserID:   "u1",
		Goal:     "create jira issue",
	})
	if err != nil {
		t.Fatalf("CreateEpisode: %v", err)
	}

	svc := action.NewService(action.NewMemoryRepository(), epSvc)

	a1, err := svc.RecordAction(ctx, action.RecordInput{
		TenantID:  ep.TenantID,
		EpisodeID: ep.ID,
		Type:      action.TypeToolCall,
		ToolName:  "jira.search_projects",
		Input:     json.RawMessage(`{"query":"PAY"}`),
		Status:    action.StatusSuccess,
	})
	if err != nil {
		t.Fatalf("RecordAction 1: %v", err)
	}
	if a1.Sequence != 1 {
		t.Fatalf("sequence = %d, want 1", a1.Sequence)
	}

	a2, err := svc.RecordAction(ctx, action.RecordInput{
		TenantID:  ep.TenantID,
		EpisodeID: ep.ID,
		Type:      action.TypeToolCall,
		ToolName:  "jira.create_issue",
		Input:     json.RawMessage(`{"project":"PAY","priority":"blocker"}`),
		Status:    action.StatusSuccess,
	})
	if err != nil {
		t.Fatalf("RecordAction 2: %v", err)
	}
	if a2.Sequence != 2 {
		t.Fatalf("sequence = %d, want 2", a2.Sequence)
	}

	inf := 0.95
	link, err := svc.LinkExperience(ctx, action.LinkInput{
		TenantID:     ep.TenantID,
		EpisodeID:    ep.ID,
		ActionID:     a2.ID,
		ExperienceID: "exp-priority-blocker",
		Influence:    &inf,
		Evidence:     "priority field came from E3 preference",
	})
	if err != nil {
		t.Fatalf("LinkExperience: %v", err)
	}
	if link.Influence != 0.95 {
		t.Fatalf("influence = %v, want 0.95", link.Influence)
	}

	actions, err := svc.ListActions(ctx, ep.TenantID, ep.ID)
	if err != nil {
		t.Fatalf("ListActions: %v", err)
	}
	if len(actions) != 2 {
		t.Fatalf("actions len = %d, want 2", len(actions))
	}

	links, err := svc.ListLinks(ctx, ep.TenantID, ep.ID)
	if err != nil {
		t.Fatalf("ListLinks: %v", err)
	}
	if len(links) != 1 {
		t.Fatalf("links len = %d, want 1", len(links))
	}
	if links[0].ActionID != a2.ID {
		t.Fatalf("link action = %s, want %s", links[0].ActionID, a2.ID)
	}

	_, err = svc.LinkExperience(ctx, action.LinkInput{
		TenantID:     ep.TenantID,
		EpisodeID:    ep.ID,
		ActionID:     a2.ID,
		ExperienceID: "exp-priority-blocker",
	})
	if !errors.Is(err, action.ErrDuplicateLink) {
		t.Fatalf("duplicate link err = %v, want ErrDuplicateLink", err)
	}
}

func TestRecordActionRequiresEpisode(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	epSvc := episode.NewService(episode.NewMemoryRepository())
	svc := action.NewService(action.NewMemoryRepository(), epSvc)

	_, err := svc.RecordAction(ctx, action.RecordInput{
		TenantID:  "t1",
		EpisodeID: "missing",
		Type:      action.TypePlan,
		Status:    action.StatusSuccess,
	})
	if !errors.Is(err, action.ErrEpisodeNotFound) {
		t.Fatalf("err = %v, want ErrEpisodeNotFound", err)
	}
}

func TestToolCallRequiresToolName(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	epSvc := episode.NewService(episode.NewMemoryRepository())
	ep, err := epSvc.CreateEpisode(ctx, episode.CreateEpisodeInput{
		TenantID: "t1", AgentID: "a1", UserID: "u1", Goal: "g",
	})
	if err != nil {
		t.Fatalf("CreateEpisode: %v", err)
	}
	svc := action.NewService(action.NewMemoryRepository(), epSvc)

	_, err = svc.RecordAction(ctx, action.RecordInput{
		TenantID:  ep.TenantID,
		EpisodeID: ep.ID,
		Type:      action.TypeToolCall,
	})
	if !errors.Is(err, action.ErrInvalidInput) {
		t.Fatalf("err = %v, want ErrInvalidInput", err)
	}
}
