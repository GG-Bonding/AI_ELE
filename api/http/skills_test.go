package httpserver_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"time"

	httpserver "github.com/agent-experience-engine/agent-experience-engine/api/http"
	"github.com/agent-experience-engine/agent-experience-engine/internal/experience"
)

func TestProposeSkillHTTP(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	patterns := experience.NewMemoryPatternRepository()
	skills := experience.NewMemorySkillRepository()
	svc := experience.NewService(experience.NewMemoryRepository()).
		WithPatterns(patterns).
		WithSkills(skills)

	p, err := patterns.Create(ctx, experience.Pattern{
		ID: "pat-http", TenantID: "t", Type: experience.TypeProcedural,
		Scope: experience.ScopeTool, ScopeKey: "jira",
		Trigger: "when project key unknown", Content: "Resolve project key first.",
		Confidence: 0.9, Utility: 0.8, SupportCount: 3,
		Status: experience.PatternStatusActive,
		CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}

	srv := httpserver.New(
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		stubReady{},
		httpserver.Options{Experiences: svc},
	)
	h := srv.Handler()

	out := postJSON(t, h, "/api/v1/patterns/"+p.ID+"/skill", map[string]any{
		"tenant_id": "t",
	}, http.StatusCreated)
	if out["created"] != true {
		t.Fatalf("created=%v skipped=%v", out["created"], out["skipped"])
	}
	sk, ok := out["skill"].(map[string]any)
	if !ok || sk["id"] == "" {
		t.Fatalf("skill=%#v", out["skill"])
	}
	spec, _ := sk["spec_yaml"].(string)
	if !strings.Contains(spec, "auto_execute: false") {
		t.Fatalf("spec=%s", spec)
	}
	sid := sk["id"].(string)

	got := getJSON(t, h, "/api/v1/skills/"+sid+"?tenant_id=t", http.StatusOK)
	if got["id"] != sid {
		t.Fatalf("get skill id=%v", got["id"])
	}
	byPat := getJSON(t, h, "/api/v1/patterns/"+p.ID+"/skill?tenant_id=t", http.StatusOK)
	if byPat["id"] != sid {
		t.Fatalf("pattern skill id=%v", byPat["id"])
	}
}
