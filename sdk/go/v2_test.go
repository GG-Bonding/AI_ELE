package experienceclient_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	experienceclient "github.com/agent-experience-engine/agent-experience-engine/sdk/go"
)

func TestV2TracingTargetPatternFlow(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/episodes", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "ep1", "tenant_id": "tenant_a", "agent_id": "a", "user_id": "u",
			"goal": "create jira", "status": "RUNNING",
		})
	})
	mux.HandleFunc("POST /api/v1/context", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"context_id": "ctx1",
			"disclaimer": "untrusted",
			"patterns": []map[string]any{{
				"id": "pat1", "type": "PROCEDURAL", "content": "resolve key first",
				"utility": 0.8, "confidence": 0.9,
			}},
			"experiences": []map[string]any{},
			"selections":  []map[string]any{},
		})
	})
	mux.HandleFunc("POST /api/v1/episodes/{id}/actions", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["context_id"] != "ctx1" {
			t.Fatalf("context_id=%v", body["context_id"])
		}
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "act1", "episode_id": "ep1", "tenant_id": "tenant_a",
			"type": "TOOL_CALL", "tool_name": body["tool_name"], "status": body["status"],
			"context_id": "ctx1",
		})
	})
	mux.HandleFunc("POST /api/v1/feedback", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		target, _ := body["target"].(map[string]any)
		if target["type"] != "ACTION_FIELD" || target["field"] != "priority" {
			t.Fatalf("target=%#v", body["target"])
		}
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"feedback": map[string]any{"id": "fb1"}, "episode_reward": map[string]any{},
		})
	})
	mux.HandleFunc("POST /api/v1/patterns/evolve", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{"created": []any{}, "skipped": 0})
	})
	mux.HandleFunc("POST /api/v1/patterns/{id}/reward", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "pat1", "tenant_id": "tenant_a", "utility": 0.9, "status": "ACTIVE",
		})
	})
	mux.HandleFunc("POST /api/v1/patterns/{id}/skill", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"created": true, "skill": map[string]any{"id": "sk1", "pattern_id": "pat1", "name": "jira.resolve_key"},
		})
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	client, err := experienceclient.New(srv.URL,
		experienceclient.WithTenant("tenant_a"),
		experienceclient.WithAgent("a"),
		experienceclient.WithUser("u"),
	)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	ep, _, err := client.StartEpisode(ctx, experienceclient.StartEpisodeInput{Goal: "create jira"})
	if err != nil {
		t.Fatal(err)
	}
	payload, err := ep.GetContext(ctx, experienceclient.GetContextInput{Task: "create PAY jira issue", Tools: []string{"jira"}})
	if err != nil {
		t.Fatal(err)
	}
	if payload.ContextID != "ctx1" || len(payload.Patterns) != 1 {
		t.Fatalf("payload=%#v", payload)
	}
	call, err := ep.ToolCall(ctx, payload.ContextID, "jira.create_issue", map[string]any{"project": "PAY"}, "SUCCESS")
	if err != nil {
		t.Fatal(err)
	}
	reward := -1.0
	if _, err := ep.Feedback(ctx, experienceclient.FeedbackInput{
		Source: "human", Reward: &reward, Confidence: 1,
		Target: call.Field("priority"),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := client.EvolvePatterns(ctx, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := client.ApplyPatternReward(ctx, "pat1", experienceclient.PatternRewardInput{
		Reward: 0.1, Confidence: 1, IdempotencyKey: "k1",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := client.ProposeSkill(ctx, "", "pat1"); err != nil {
		t.Fatal(err)
	}
}
