package experience_test

import (
	"context"
	"testing"

	"github.com/agent-experience-engine/agent-experience-engine/internal/experience"
)

func TestGeneralizeCreatesPatternWithDerivedFrom(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := experience.NewMemoryRepository()
	rels := experience.NewMemoryRelationRepository()
	patterns := experience.NewMemoryPatternRepository()
	svc := experience.NewService(repo).WithRelations(rels).WithPatterns(patterns)

	ids := make([]string, 0, 3)
	for i, ep := range []string{"ep1", "ep2", "ep3"} {
		exp := mustCreateExp(t, svc, experience.CreateInput{
			TenantID: "t", Type: experience.TypeProcedural, Scope: experience.ScopeTool, ScopeKey: "jira",
			Trigger: "create or update jira issue when project key unknown",
			Content: "Resolve Jira project key before mutating issues.",
			SourceEpisodeID: ep, Confidence: 0.9, Embedding: unitVec(4),
			Evidence: experience.Evidence{
				SourceEpisodeID: ep, SupportEpisodeIDs: []string{ep},
				SuccessAttemptCount: 2,
			},
			Status: experience.StatusActive,
		})
		exp.Utility = 0.8
		updated, err := repo.Update(ctx, exp)
		if err != nil {
			t.Fatalf("update utility %d: %v", i, err)
		}
		ids = append(ids, updated.ID)
	}

	res, err := svc.Generalize(ctx, "t", experience.GeneralizeInput{ExperienceIDs: ids})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Created {
		t.Fatalf("expected pattern creation, skipped=%q", res.Skipped)
	}
	if res.Pattern.SupportCount != 3 {
		t.Fatalf("support=%d", res.Pattern.SupportCount)
	}
	if res.Pattern.Status != experience.PatternStatusCandidate && res.Pattern.Status != experience.PatternStatusActive {
		t.Fatalf("status=%s", res.Pattern.Status)
	}
	if res.Pattern.Trigger == "" || res.Pattern.Content == "" {
		t.Fatalf("empty pattern body: %+v", res.Pattern)
	}

	ev, err := svc.ListPatternEvidence(ctx, "t", res.Pattern.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(ev) != 3 {
		t.Fatalf("evidence=%d want 3", len(ev))
	}

	// DERIVED_FROM edges: pattern → each experience
	for _, id := range ids {
		relsFor, err := svc.ListRelations(ctx, "t", id)
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, r := range relsFor {
			if r.Type == experience.RelationDerivedFrom && r.FromExperienceID == res.Pattern.ID && r.ToExperienceID == id {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("missing DERIVED_FROM for experience %s (rels=%v)", id, relsFor)
		}
	}

	// Idempotent: second call should skip creating another pattern.
	again, err := svc.Generalize(ctx, "t", experience.GeneralizeInput{ExperienceIDs: ids})
	if err != nil {
		t.Fatal(err)
	}
	if again.Created {
		t.Fatal("expected skip when pattern already covers cluster")
	}
	if again.Pattern.ID != res.Pattern.ID {
		t.Fatalf("pattern id changed: %s vs %s", again.Pattern.ID, res.Pattern.ID)
	}
}

func TestGeneralizeRejectsFewerThanThreeEpisodes(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := experience.NewMemoryRepository()
	svc := experience.NewService(repo).WithPatterns(experience.NewMemoryPatternRepository())

	ids := make([]string, 0, 3)
	for i := 0; i < 3; i++ {
		exp := mustCreateExp(t, svc, experience.CreateInput{
			TenantID: "t", Type: experience.TypeProcedural, Scope: experience.ScopeTenant,
			Trigger: "same episode cluster", Content: "rule body",
			SourceEpisodeID: "ep_only", Confidence: 0.9, Embedding: unitVec(4),
			Evidence: experience.Evidence{SourceEpisodeID: "ep_only", SupportEpisodeIDs: []string{"ep_only"}},
			Status:   experience.StatusActive,
		})
		exp.Utility = 0.9
		updated, err := repo.Update(ctx, exp)
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, updated.ID)
	}

	res, err := svc.Generalize(ctx, "t", experience.GeneralizeInput{ExperienceIDs: ids})
	if err != nil {
		t.Fatal(err)
	}
	if res.Created {
		t.Fatal("should not generalize from a single episode")
	}
	if res.Skipped == "" {
		t.Fatal("expected skip reason")
	}
}

func TestGeneralizeRejectsLowUtility(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := experience.NewMemoryRepository()
	svc := experience.NewService(repo).WithPatterns(experience.NewMemoryPatternRepository())

	ids := make([]string, 0, 3)
	for i, ep := range []string{"a", "b", "c"} {
		exp := mustCreateExp(t, svc, experience.CreateInput{
			TenantID: "t", Type: experience.TypeProcedural, Scope: experience.ScopeTenant,
			Trigger: "ops", Content: "body", SourceEpisodeID: ep, Confidence: 0.9, Embedding: unitVec(4),
			Evidence: experience.Evidence{SourceEpisodeID: ep, SupportEpisodeIDs: []string{ep}},
			Status:   experience.StatusActive,
		})
		exp.Utility = 0.2
		updated, err := repo.Update(ctx, exp)
		if err != nil {
			t.Fatalf("%d: %v", i, err)
		}
		ids = append(ids, updated.ID)
	}
	res, err := svc.Generalize(ctx, "t", experience.GeneralizeInput{ExperienceIDs: ids})
	if err != nil {
		t.Fatal(err)
	}
	if res.Created {
		t.Fatal("low utility should not generalize")
	}
}

func mustCreateExp(t *testing.T, svc *experience.Service, in experience.CreateInput) experience.Experience {
	t.Helper()
	got, err := svc.Create(context.Background(), in)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	return got
}
