package httpserver_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	httpserver "github.com/agent-experience-engine/agent-experience-engine/api/http"
	"github.com/agent-experience-engine/agent-experience-engine/internal/episode"
	"github.com/agent-experience-engine/agent-experience-engine/internal/experience"
	"github.com/agent-experience-engine/agent-experience-engine/internal/extractor"
)

func newTestServer(t *testing.T) http.Handler {
	t.Helper()
	svc := episode.NewService(episode.NewMemoryRepository())
	return httpserver.New(
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		stubReady{},
		httpserver.Options{Episodes: svc},
	).Handler()
}

func newTestServerWithExtractor(t *testing.T, ext httpserver.ExperienceExtractor) http.Handler {
	t.Helper()
	svc := episode.NewService(episode.NewMemoryRepository())
	return httpserver.New(
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		stubReady{},
		httpserver.Options{Episodes: svc, Extractor: ext},
	).Handler()
}

type stubExtractor struct {
	candidates []experience.Candidate
	err        error
}

func (s stubExtractor) Extract(context.Context, extractor.ExtractInput) ([]experience.Candidate, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.candidates, nil
}

func newTestServerWithExtractorAndStore(t *testing.T, ext httpserver.ExperienceExtractor, store *recordingStorePipeline) http.Handler {
	t.Helper()
	svc := episode.NewService(episode.NewMemoryRepository())
	return httpserver.New(
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		stubReady{},
		httpserver.Options{Episodes: svc, Extractor: ext, StorePipeline: store},
	).Handler()
}

type recordingStorePipeline struct {
	lastOpts experience.StoreOptions
}

func (s *recordingStorePipeline) StoreCandidates(ctx context.Context, tenantID, sourceEpisodeID string, candidates []experience.Candidate) (experience.StoreCandidatesResult, error) {
	return s.StoreCandidatesWithOptions(ctx, tenantID, sourceEpisodeID, candidates, experience.StoreOptions{})
}

func (s *recordingStorePipeline) StoreCandidatesWithOptions(_ context.Context, _, _ string, candidates []experience.Candidate, opts experience.StoreOptions) (experience.StoreCandidatesResult, error) {
	s.lastOpts = opts
	stored := make([]experience.Experience, 0, len(candidates))
	for _, c := range candidates {
		stored = append(stored, experience.Experience{
			Type: c.Type, Trigger: c.Trigger, Content: c.Content, Scope: c.Scope, ScopeKey: c.ScopeKey,
		})
	}
	return experience.StoreCandidatesResult{Stored: stored}, nil
}

func TestFailedOutcomePassesRealStatusToStore(t *testing.T) {
	t.Parallel()
	store := &recordingStorePipeline{}
	h := newTestServerWithExtractorAndStore(t, stubExtractor{
		candidates: []experience.Candidate{{
			Type: experience.TypeFailure, Trigger: "jira create failed", Content: "bad key",
			Confidence: 0.8, Scope: experience.ScopeTool, ScopeKey: "jira",
		}},
	}, store)

	ep := postJSON(t, h, "/api/v1/episodes", map[string]any{
		"tenant_id": "tenant_a", "agent_id": "agent", "user_id": "user", "goal": "Create Jira issue",
	}, http.StatusCreated)
	id, _ := ep["id"].(string)

	postJSON(t, h, "/api/v1/episodes/"+id+"/attempts", map[string]any{
		"tenant_id": "tenant_a", "action": "create_issue", "status": "FAILED", "error_code": "INVALID_PROJECT_KEY",
	}, http.StatusCreated)

	postJSON(t, h, "/api/v1/episodes/"+id+"/outcome", map[string]any{
		"tenant_id": "tenant_a", "status": "FAILED",
	}, http.StatusCreated)

	if store.lastOpts.Outcome.Status != "FAILED" {
		t.Fatalf("outcome status = %q want FAILED", store.lastOpts.Outcome.Status)
	}
	if store.lastOpts.Evidence.FailedAttemptCount != 1 {
		t.Fatalf("evidence = %#v", store.lastOpts.Evidence)
	}
	if store.lastOpts.Outcome.Status == "SUCCESS" {
		t.Fatal("FAILED episode must not default to SUCCESS evaluation path")
	}
}

func TestEpisodeLifecycleHTTP(t *testing.T) {
	t.Parallel()
	h := newTestServer(t)

	epBody := map[string]any{
		"tenant_id": "tenant_a",
		"agent_id":  "agent_01",
		"user_id":   "user_01",
		"task_type": "jira.create_issue",
		"goal":      "Create Jira issue",
		"input":     `project="Payment"`,
	}
	ep := postJSON(t, h, "/api/v1/episodes", epBody, http.StatusCreated)
	episodeID, _ := ep["id"].(string)
	if episodeID == "" {
		t.Fatalf("missing episode id: %#v", ep)
	}
	if ep["status"] != "RUNNING" {
		t.Fatalf("status = %v", ep["status"])
	}

	postJSON(t, h, "/api/v1/episodes/"+episodeID+"/attempts", map[string]any{
		"tenant_id":     "tenant_a",
		"action":        "create_issue",
		"tool_name":     "jira.create_issue",
		"status":        "FAILED",
		"error_code":    "INVALID_PROJECT_KEY",
		"error_message": "bad key",
	}, http.StatusCreated)

	postJSON(t, h, "/api/v1/episodes/"+episodeID+"/attempts", map[string]any{
		"tenant_id": "tenant_a",
		"action":    "search_projects",
		"tool_name": "jira.search_projects",
		"status":    "SUCCESS",
	}, http.StatusCreated)

	out := postJSON(t, h, "/api/v1/episodes/"+episodeID+"/outcome", map[string]any{
		"tenant_id": "tenant_a",
		"status":    "SUCCESS",
		"verified":  true,
		"verifier":  "tool",
		"metrics":   map[string]float64{"attempts": 2},
	}, http.StatusCreated)

	episodeObj, _ := out["episode"].(map[string]any)
	if episodeObj["status"] != "SUCCESS" {
		t.Fatalf("episode status = %v", episodeObj["status"])
	}

	got := getJSON(t, h, "/api/v1/episodes/"+episodeID+"?tenant_id=tenant_a", http.StatusOK)
	if got["status"] != "SUCCESS" {
		t.Fatalf("get status = %v", got["status"])
	}
}

func TestOutcomeExtractionReturnsCandidates(t *testing.T) {
	t.Parallel()
	h := newTestServerWithExtractor(t, stubExtractor{
		candidates: []experience.Candidate{{
			Type:       experience.TypeProcedural,
			Trigger:    "create jira issue when project key unknown",
			Content:    "Resolve project key first",
			Confidence: 0.9,
			Scope:      experience.ScopeTool,
			ScopeKey:   "jira",
		}},
	})

	ep := postJSON(t, h, "/api/v1/episodes", map[string]any{
		"tenant_id": "tenant_a",
		"agent_id":  "agent",
		"user_id":   "user",
		"goal":      "Create Jira issue",
	}, http.StatusCreated)
	id, _ := ep["id"].(string)

	postJSON(t, h, "/api/v1/episodes/"+id+"/attempts", map[string]any{
		"tenant_id": "tenant_a",
		"action":    "search",
		"status":    "SUCCESS",
	}, http.StatusCreated)

	out := postJSON(t, h, "/api/v1/episodes/"+id+"/outcome", map[string]any{
		"tenant_id": "tenant_a",
		"status":    "SUCCESS",
	}, http.StatusCreated)

	cands, ok := out["experience_candidates"].([]any)
	if !ok || len(cands) != 1 {
		t.Fatalf("experience_candidates = %#v", out["experience_candidates"])
	}
}

func TestOutcomeExtractionFailureIsNotSilent(t *testing.T) {
	t.Parallel()
	h := newTestServerWithExtractor(t, stubExtractor{err: errors.New("schema boom")})

	ep := postJSON(t, h, "/api/v1/episodes", map[string]any{
		"tenant_id": "tenant_a",
		"agent_id":  "agent",
		"user_id":   "user",
		"goal":      "g",
	}, http.StatusCreated)
	id, _ := ep["id"].(string)

	out := postJSON(t, h, "/api/v1/episodes/"+id+"/outcome", map[string]any{
		"tenant_id": "tenant_a",
		"status":    "SUCCESS",
	}, http.StatusInternalServerError)
	if out["error"] == nil || out["error"] == "" {
		t.Fatalf("expected extraction error, got %#v", out)
	}
	if out["episode"] == nil || out["outcome"] == nil {
		t.Fatalf("expected episode/outcome in error response: %#v", out)
	}
}

func TestEpisodeTenantIsolationHTTP(t *testing.T) {
	t.Parallel()
	h := newTestServer(t)

	ep := postJSON(t, h, "/api/v1/episodes", map[string]any{
		"tenant_id": "tenant_a",
		"agent_id":  "agent",
		"user_id":   "user",
		"goal":      "secret",
	}, http.StatusCreated)
	id, _ := ep["id"].(string)

	getJSON(t, h, "/api/v1/episodes/"+id+"?tenant_id=tenant_b", http.StatusNotFound)

	postJSON(t, h, "/api/v1/episodes/"+id+"/attempts", map[string]any{
		"tenant_id": "tenant_b",
		"action":    "x",
		"status":    "SUCCESS",
	}, http.StatusNotFound)

	postJSON(t, h, "/api/v1/episodes/"+id+"/outcome", map[string]any{
		"tenant_id": "tenant_b",
		"status":    "SUCCESS",
	}, http.StatusNotFound)
}

func postJSON(t *testing.T, h http.Handler, path string, body any, wantStatus int) map[string]any {
	t.Helper()
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != wantStatus {
		t.Fatalf("POST %s status = %d, want %d; body=%s", path, rec.Code, wantStatus, rec.Body.String())
	}
	return decodeMap(t, rec.Body.Bytes())
}

func getJSON(t *testing.T, h http.Handler, path string, wantStatus int) map[string]any {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != wantStatus {
		t.Fatalf("GET %s status = %d, want %d; body=%s", path, rec.Code, wantStatus, rec.Body.String())
	}
	if rec.Body.Len() == 0 {
		return nil
	}
	return decodeMap(t, rec.Body.Bytes())
}

func decodeMap(t *testing.T, body []byte) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("unmarshal %s: %v", body, err)
	}
	return out
}
