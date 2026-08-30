package httpserver_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"testing"

	httpserver "github.com/agent-experience-engine/agent-experience-engine/api/http"
	"github.com/agent-experience-engine/agent-experience-engine/internal/experience"
)

func TestPatternGeneralizeHTTP(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := experience.NewMemoryRepository()
	rels := experience.NewMemoryRelationRepository()
	patterns := experience.NewMemoryPatternRepository()
	svc := experience.NewService(repo).WithRelations(rels).WithPatterns(patterns)

	ids := make([]string, 0, 3)
	for _, ep := range []string{"ep1", "ep2", "ep3"} {
		exp, err := svc.Create(ctx, experience.CreateInput{
			TenantID: "t", Type: experience.TypeProcedural, Scope: experience.ScopeTool, ScopeKey: "jira",
			Trigger: "create or update jira issue", Content: "Resolve project key first.",
			SourceEpisodeID: ep, Confidence: 0.9, Embedding: []float32{1, 0, 0, 0},
			Evidence: experience.Evidence{SourceEpisodeID: ep, SupportEpisodeIDs: []string{ep}},
			Status:   experience.StatusActive,
		})
		if err != nil {
			t.Fatal(err)
		}
		exp.Utility = 0.85
		updated, err := repo.Update(ctx, exp)
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, updated.ID)
	}

	srv := httpserver.New(
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		stubReady{},
		httpserver.Options{Experiences: svc},
	)
	h := srv.Handler()

	out := postJSON(t, h, "/api/v1/patterns/generalize", map[string]any{
		"tenant_id":      "t",
		"experience_ids": ids,
	}, http.StatusCreated)
	if out["created"] != true {
		t.Fatalf("created=%v skipped=%v", out["created"], out["skipped"])
	}
	pat, ok := out["pattern"].(map[string]any)
	if !ok || pat["id"] == "" {
		t.Fatalf("pattern=%#v", out["pattern"])
	}
	pid := pat["id"].(string)

	got := getJSON(t, h, "/api/v1/patterns/"+pid+"?tenant_id=t", http.StatusOK)
	if got["id"] != pid {
		t.Fatalf("get pattern id=%v", got["id"])
	}
	ev := getJSON(t, h, "/api/v1/patterns/"+pid+"/evidence?tenant_id=t", http.StatusOK)
	list, _ := ev["evidence"].([]any)
	if len(list) != 3 {
		t.Fatalf("evidence=%d", len(list))
	}
}
