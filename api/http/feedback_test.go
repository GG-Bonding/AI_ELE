package httpserver_test

import (
	"io"
	"log/slog"
	"net/http"
	"testing"

	httpserver "github.com/agent-experience-engine/agent-experience-engine/api/http"
	"github.com/agent-experience-engine/agent-experience-engine/internal/episode"
	"github.com/agent-experience-engine/agent-experience-engine/internal/feedback"
)

func TestFeedbackHTTP(t *testing.T) {
	t.Parallel()
	epSvc := episode.NewService(episode.NewMemoryRepository())
	fbSvc := feedback.NewService(feedback.NewMemoryRepository(), epSvc, feedback.NewRewardEngine(nil))
	h := httpserver.New(
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		stubReady{},
		httpserver.Options{Episodes: epSvc, Feedbacks: fbSvc},
	).Handler()

	ep := postJSON(t, h, "/api/v1/episodes", map[string]any{
		"tenant_id": "tenant_a", "agent_id": "a", "user_id": "u", "goal": "Create Jira issue",
	}, http.StatusCreated)
	id, _ := ep["id"].(string)

	postJSON(t, h, "/api/v1/episodes/"+id+"/outcome", map[string]any{
		"tenant_id": "tenant_a", "status": "SUCCESS",
	}, http.StatusCreated)

	out := postJSON(t, h, "/api/v1/feedback", map[string]any{
		"tenant_id":  "tenant_a",
		"episode_id": id,
		"source":     "business",
		"reward":     1.0,
		"confidence": 1.0,
	}, http.StatusCreated)
	if out["feedback"] == nil || out["episode_reward"] == nil {
		t.Fatalf("%#v", out)
	}

	postJSON(t, h, "/api/v1/feedback", map[string]any{
		"tenant_id":  "tenant_a",
		"episode_id": id,
		"source":     "llm_judge",
		"signal":     "hard_failure",
	}, http.StatusCreated)

	reward := getJSON(t, h, "/api/v1/episodes/"+id+"/reward?tenant_id=tenant_a", http.StatusOK)
	rows, _ := reward["feedbacks"].([]any)
	if len(rows) != 2 {
		t.Fatalf("raw feedbacks=%d %#v", len(rows), reward)
	}

	// cross-tenant should 404
	postJSON(t, h, "/api/v1/feedback", map[string]any{
		"tenant_id": "tenant_b", "episode_id": id, "source": "tool", "reward": 1.0,
	}, http.StatusNotFound)
}
