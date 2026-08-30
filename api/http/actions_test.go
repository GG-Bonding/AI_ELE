package httpserver_test

import (
	"io"
	"log/slog"
	"net/http"
	"testing"

	httpserver "github.com/agent-experience-engine/agent-experience-engine/api/http"
	"github.com/agent-experience-engine/agent-experience-engine/internal/action"
	"github.com/agent-experience-engine/agent-experience-engine/internal/episode"
)

func TestActionTrackingHTTP(t *testing.T) {
	t.Parallel()
	epSvc := episode.NewService(episode.NewMemoryRepository())
	actionSvc := action.NewService(action.NewMemoryRepository(), epSvc)
	h := httpserver.New(
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		stubReady{},
		httpserver.Options{Episodes: epSvc, Actions: actionSvc},
	).Handler()

	ep := postJSON(t, h, "/api/v1/episodes", map[string]any{
		"tenant_id": "tenant_a", "agent_id": "a", "user_id": "u", "goal": "create jira issue",
	}, http.StatusCreated)
	episodeID, _ := ep["id"].(string)

	a1 := postJSON(t, h, "/api/v1/episodes/"+episodeID+"/actions", map[string]any{
		"tenant_id": "tenant_a",
		"type":      "TOOL_CALL",
		"tool_name": "jira.search_projects",
		"input":     map[string]any{"query": "PAY"},
		"status":    "SUCCESS",
	}, http.StatusCreated)
	if a1["sequence"].(float64) != 1 {
		t.Fatalf("sequence=%v want 1", a1["sequence"])
	}

	a2 := postJSON(t, h, "/api/v1/episodes/"+episodeID+"/actions", map[string]any{
		"tenant_id": "tenant_a",
		"type":      "TOOL_CALL",
		"tool_name": "jira.create_issue",
		"input":     map[string]any{"project": "PAY", "priority": "blocker"},
		"status":    "SUCCESS",
	}, http.StatusCreated)
	actionID, _ := a2["id"].(string)

	inf := 0.95
	link := postJSON(t, h, "/api/v1/episodes/"+episodeID+"/actions/"+actionID+"/links", map[string]any{
		"tenant_id":     "tenant_a",
		"experience_id": "exp-priority-blocker",
		"influence":     inf,
		"evidence":      "priority field came from E3 preference",
	}, http.StatusCreated)
	if link["action_id"] != actionID {
		t.Fatalf("link action_id=%v want %s", link["action_id"], actionID)
	}

	listed := getJSON(t, h, "/api/v1/episodes/"+episodeID+"/actions?tenant_id=tenant_a", http.StatusOK)
	actions, _ := listed["actions"].([]any)
	if len(actions) != 2 {
		t.Fatalf("actions=%d want 2 %#v", len(actions), listed)
	}

	links := getJSON(t, h, "/api/v1/episodes/"+episodeID+"/action-links?tenant_id=tenant_a", http.StatusOK)
	rows, _ := links["links"].([]any)
	if len(rows) != 1 {
		t.Fatalf("links=%d want 1 %#v", len(rows), links)
	}

	// duplicate link → 409
	postJSON(t, h, "/api/v1/episodes/"+episodeID+"/actions/"+actionID+"/links", map[string]any{
		"tenant_id": "tenant_a", "experience_id": "exp-priority-blocker",
	}, http.StatusConflict)

	// TOOL_CALL without tool_name → 400
	postJSON(t, h, "/api/v1/episodes/"+episodeID+"/actions", map[string]any{
		"tenant_id": "tenant_a", "type": "TOOL_CALL",
	}, http.StatusBadRequest)

	// missing episode → 404
	postJSON(t, h, "/api/v1/episodes/missing/actions", map[string]any{
		"tenant_id": "tenant_a", "type": "PLAN", "status": "SUCCESS",
	}, http.StatusNotFound)
}
