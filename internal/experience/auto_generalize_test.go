package experience_test

import (
	"context"
	"testing"

	"github.com/agent-experience-engine/agent-experience-engine/internal/experience"
)

func TestAutoGeneralizeCreatesPatternFromNeighborhood(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := experience.NewMemoryRepository()
	patterns := experience.NewMemoryPatternRepository()
	svc := experience.NewService(repo).WithPatterns(patterns)

	base := []float32{1, 0, 0, 0}
	ids := make([]string, 0, 3)
	for i, ep := range []string{"ep1", "ep2", "ep3"} {
		emb := append([]float32(nil), base...)
		if i > 0 {
			emb[0] = 0.98
			emb[1] = 0.1 * float32(i)
		}
		exp, err := svc.Create(ctx, experience.CreateInput{
			TenantID: "t", Type: experience.TypeProcedural, Scope: experience.ScopeTool, ScopeKey: "jira",
			Trigger: "create jira issue", Content: "Resolve project key before creating the issue.",
			SourceEpisodeID: ep, Confidence: 0.9, Embedding: emb,
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

	res, err := svc.AutoGeneralize(ctx, "t", experience.AutoGeneralizeOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if res.Scanned != 3 {
		t.Fatalf("scanned=%d", res.Scanned)
	}
	if len(res.Created) != 1 {
		t.Fatalf("created=%#v skipped=%#v", res.Created, res.Skipped)
	}
	if res.Created[0].ClusterFingerprint == "" {
		t.Fatal("expected cluster fingerprint")
	}

	again, err := svc.AutoGeneralize(ctx, "t", experience.AutoGeneralizeOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(again.Created) != 0 {
		t.Fatalf("second scan should not recreate: %#v", again.Created)
	}
	foundSkip := false
	for _, sk := range again.Skipped {
		if sk.Reason != "" {
			foundSkip = true
			break
		}
	}
	if !foundSkip && again.Clusters == 0 {
		t.Fatalf("expected skip or no new clusters: %#v", again)
	}
	_ = ids
}

func TestClusterFingerprintStable(t *testing.T) {
	t.Parallel()
	a := experience.ClusterFingerprint([]string{"b", "a", "c"})
	b := experience.ClusterFingerprint([]string{"c", "a", "b"})
	if a == "" || a != b {
		t.Fatalf("%q vs %q", a, b)
	}
}
