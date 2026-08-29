package experienceclient_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	experienceclient "github.com/agent-experience-engine/agent-experience-engine/sdk/go"
)

func TestClientEpisodeContextFeedbackLoop(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/episodes", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "ep1", "tenant_id": "tenant_a", "agent_id": "a", "user_id": "u",
			"goal": "Create Jira issue", "status": "RUNNING",
		})
	})
	mux.HandleFunc("POST /api/v1/episodes/{id}/attempts", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "at1", "episode_id": "ep1", "tenant_id": "tenant_a", "status": "FAILED",
		})
	})
	mux.HandleFunc("POST /api/v1/episodes/{id}/outcome", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"episode": map[string]any{"id": "ep1", "tenant_id": "tenant_a", "status": "SUCCESS"},
			"outcome": map[string]any{"id": "out1", "episode_id": "ep1", "status": "SUCCESS"},
		})
	})
	mux.HandleFunc("POST /api/v1/context", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["task"] == "" {
			t.Fatalf("missing task: %#v", body)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"disclaimer": "untrusted",
			"experiences": []map[string]any{{
				"type": "PROCEDURAL", "content": "Resolve project key first", "source": "x", "confidence": 0.9,
			}},
			"selections": []map[string]any{},
		})
	})
	mux.HandleFunc("POST /api/v1/feedback", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"feedback":       map[string]any{"id": "fb1"},
			"episode_reward": map[string]any{"weighted_reward": 1.0},
		})
	})
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	client, err := experienceclient.New(srv.URL,
		experienceclient.WithTenant("tenant_a"),
		experienceclient.WithAgent("a"),
		experienceclient.WithUser("u"),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx := context.Background()
	if _, err := client.Healthz(ctx); err != nil {
		t.Fatalf("Healthz: %v", err)
	}

	handle, ep, err := client.StartEpisode(ctx, experienceclient.StartEpisodeInput{
		Goal: "Create Jira issue",
	})
	if err != nil {
		t.Fatalf("StartEpisode: %v", err)
	}
	if ep.ID != "ep1" || handle.ID != "ep1" {
		t.Fatalf("episode=%#v handle=%#v", ep, handle)
	}

	if _, err := handle.AddAttempt(ctx, experienceclient.AddAttemptInput{
		Action: "create_issue", ToolName: "jira.create_issue", Status: "FAILED", ErrorCode: "INVALID_PROJECT_KEY",
	}); err != nil {
		t.Fatalf("AddAttempt: %v", err)
	}
	if _, err := handle.Complete(ctx, experienceclient.CompleteInput{Status: "SUCCESS", Verified: true, Verifier: "tool"}); err != nil {
		t.Fatalf("Complete: %v", err)
	}

	payload, err := client.GetContext(ctx, experienceclient.GetContextInput{
		EpisodeID: handle.ID, Task: "Create a Jira issue", Tools: []string{"jira"}, MaxExperiences: 5,
	})
	if err != nil {
		t.Fatalf("GetContext: %v", err)
	}
	if len(payload.Experiences) != 1 {
		t.Fatalf("context=%#v", payload)
	}

	reward := 1.0
	if _, err := client.Feedback(ctx, experienceclient.FeedbackInput{
		EpisodeID: handle.ID, Source: "business", Reward: &reward, Confidence: 1,
	}); err != nil {
		t.Fatalf("Feedback: %v", err)
	}
}

func TestAPIError(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":"boom"}`, http.StatusBadRequest)
	}))
	t.Cleanup(srv.Close)
	client, _ := experienceclient.New(srv.URL)
	_, _, err := client.StartEpisode(context.Background(), experienceclient.StartEpisodeInput{TenantID: "t", Goal: "g"})
	var apiErr *experienceclient.APIError
	if err == nil {
		t.Fatal("expected error")
	}
	if !asAPIError(err, &apiErr) || apiErr.StatusCode != http.StatusBadRequest {
		t.Fatalf("err=%v", err)
	}
}

func asAPIError(err error, target **experienceclient.APIError) bool {
	e, ok := err.(*experienceclient.APIError)
	if !ok {
		return false
	}
	*target = e
	return true
}
