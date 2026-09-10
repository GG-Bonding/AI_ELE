package httpserver_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	httpserver "github.com/agent-experience-engine/agent-experience-engine/api/http"
	"github.com/agent-experience-engine/agent-experience-engine/internal/skill"
	"github.com/agent-experience-engine/agent-experience-engine/internal/skillruntime"
	"github.com/agent-experience-engine/agent-experience-engine/internal/toolregistry"
)

func TestRequireAuthPrincipalRejectsSkillRuntimeWithoutHeaders(t *testing.T) {
	t.Parallel()
	repo := skill.NewMemoryRepository()
	store := skillruntime.NewMemoryExecutionStore()
	tools := toolregistry.Default()
	rt := &skillruntime.Runtime{Tools: tools, Store: store, Exec: &stubExec{}, Preview: &stubExec{}}
	srv := httpserver.New(
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		stubReady{},
		httpserver.Options{
			SkillRepo:            repo,
			SkillRegistry:        &skill.RegistryService{Repo: repo},
			SkillRuntime:         rt,
			SkillExec:            &skill.ExecutionService{Repo: repo, Store: store, Runner: rt},
			ToolRegistry:         tools,
			RequireAuthPrincipal: true,
		},
	)
	h := srv.Handler()

	paths := []struct {
		method string
		path   string
		body   map[string]any
	}{
		{http.MethodPost, "/api/v1/skill-runtime/compile", map[string]any{"tenant_id": "spoof", "name": "x", "spec_yaml": "name: x\nsteps: []\n"}},
		{http.MethodPost, "/api/v1/skill-runtime/execute", map[string]any{"tenant_id": "spoof", "requester_id": "attacker"}},
		{http.MethodPost, "/api/v1/skill-runtime/retrieve", map[string]any{"tenant_id": "spoof", "task": "t"}},
		{http.MethodPost, "/api/v1/skill-runtime/revise", map[string]any{"tenant_id": "spoof", "version_id": "v"}},
		{http.MethodPost, "/api/v1/skill-runtime/ab-compare", map[string]any{"tenant_id": "spoof"}},
		{http.MethodPost, "/api/v1/skill-versions/v1/shadow?tenant_id=spoof", nil},
		{http.MethodPost, "/api/v1/skill-versions/v1/activate?tenant_id=spoof", nil},
		{http.MethodPost, "/api/v1/skill-executions/e1/resume", map[string]any{"tenant_id": "spoof"}},
		{http.MethodPost, "/api/v1/skill-approvals/a1/approve", map[string]any{"tenant_id": "spoof", "approved_by": "attacker"}},
		{http.MethodPost, "/api/v1/skill-approvals/a1/reject", map[string]any{"tenant_id": "spoof"}},
	}
	for _, tc := range paths {
		var body io.Reader
		if tc.body != nil {
			raw, _ := json.Marshal(tc.body)
			body = bytes.NewReader(raw)
		}
		req := httptest.NewRequest(tc.method, tc.path, body)
		if tc.body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("%s %s status=%d body=%s", tc.method, tc.path, rec.Code, rec.Body.String())
		}
	}
}

func TestRequireAuthPrincipalAcceptsTrustedHeaders(t *testing.T) {
	t.Parallel()
	repo := skill.NewMemoryRepository()
	srv := httpserver.New(
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		stubReady{},
		httpserver.Options{
			SkillRepo:            repo,
			SkillRetriever:       &skill.Retriever{Repo: repo, Tools: toolregistry.Default()},
			RequireAuthPrincipal: true,
		},
	)
	raw, _ := json.Marshal(map[string]any{"tenant_id": "spoof", "task": "fix jira"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/skill-runtime/retrieve", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-AEE-Tenant-ID", "trusted-tenant")
	req.Header.Set("X-AEE-Actor-ID", "trusted-actor")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

type stubExec struct{}

func (stubExec) Execute(context.Context, string, map[string]any) (skillruntime.ToolResult, error) {
	return skillruntime.ToolResult{OK: true, Output: map[string]any{}}, nil
}

func (stubExec) Preview(context.Context, string, map[string]any) (skillruntime.ToolResult, error) {
	return skillruntime.ToolResult{OK: true, Output: map[string]any{"_shadow": true}}, nil
}
