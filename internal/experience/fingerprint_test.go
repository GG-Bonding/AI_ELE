package experience_test

import (
	"testing"

	"github.com/agent-experience-engine/agent-experience-engine/internal/experience"
)

func TestFingerprintStableAndSensitive(t *testing.T) {
	t.Parallel()
	c := experience.Candidate{
		Type: experience.TypeProcedural, Scope: experience.ScopeTool, ScopeKey: "jira",
		Trigger: "Create issue", Content: "Resolve project key first",
	}
	fp1 := experience.Fingerprint(c)
	fp2 := experience.Fingerprint(c)
	if fp1 == "" || fp1 != fp2 {
		t.Fatalf("fingerprint not stable: %q vs %q", fp1, fp2)
	}

	c2 := c
	c2.Content = "Different content"
	if experience.Fingerprint(c2) == fp1 {
		t.Fatal("fingerprint should change when content changes")
	}
}

func TestMemoryCreateDedupIdempotent(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	repo := experience.NewMemoryRepository()
	svc := experience.NewService(repo)
	vec := []float32{1, 0}

	in := experience.CreateInput{
		TenantID: "t", Type: experience.TypeProcedural, Scope: experience.ScopeTool, ScopeKey: "jira",
		Trigger: "t", Content: "c", SourceEpisodeID: "ep1", DedupKey: "dedup-1",
		Confidence: 0.9, Embedding: vec,
	}
	first, err := svc.Create(ctx, in)
	if err != nil {
		t.Fatalf("first create: %v", err)
	}
	second, err := svc.Create(ctx, in)
	if err != nil {
		t.Fatalf("second create: %v", err)
	}
	if second.ID != first.ID {
		t.Fatalf("dedup retry should return existing id: first=%s second=%s", first.ID, second.ID)
	}
}
