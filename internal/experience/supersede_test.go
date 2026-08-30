package experience_test

import (
	"context"
	"testing"
	"time"

	"github.com/agent-experience-engine/agent-experience-engine/internal/experience"
)

func TestAuthorityScorePrefersEvidenceAndFreshness(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 8, 30, 12, 0, 0, 0, time.UTC)
	stale := experience.Experience{
		Confidence: 0.7, Utility: 0.7,
		Evidence:  experience.Evidence{SupportEpisodeIDs: []string{"e1"}, SuccessAttemptCount: 1},
		UpdatedAt: now.Add(-180 * 24 * time.Hour),
	}
	fresh := experience.Experience{
		Confidence: 0.85, Utility: 0.6,
		Evidence: experience.Evidence{
			SupportEpisodeIDs:   []string{"e2", "e3", "e4"},
			SuccessAttemptCount: 3,
			HasFailureContrast:  true,
		},
		UpdatedAt: now.Add(-2 * 24 * time.Hour),
	}
	if experience.AuthorityScore(fresh, now) <= experience.AuthorityScore(stale, now) {
		t.Fatalf("fresh evidence should score higher: fresh=%v stale=%v",
			experience.AuthorityScore(fresh, now), experience.AuthorityScore(stale, now))
	}
}

func TestResolveConflictSupersedesWhenAuthorityClear(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := experience.NewMemoryRepository()
	rels := experience.NewMemoryRelationRepository()
	svc := experience.NewService(repo).WithRelations(rels)

	old := mustCreate(t, svc, experience.CreateInput{
		TenantID: "t", Type: experience.TypeConstraint, Scope: experience.ScopeTenant,
		Trigger: "feature flag", Content: "必须打开开关 before deploy",
		SourceEpisodeID: "ep_old", Confidence: 0.55, Embedding: unitVec(4),
		Evidence: experience.Evidence{SourceEpisodeID: "ep_old", SupportEpisodeIDs: []string{"ep_old"}},
	})
	// Make old look weak/stale.
	old.Utility = 0.4
	old.UpdatedAt = time.Now().UTC().Add(-200 * 24 * time.Hour)
	old, err := repo.Update(ctx, old)
	if err != nil {
		t.Fatal(err)
	}

	newer := mustCreate(t, svc, experience.CreateInput{
		TenantID: "t", Type: experience.TypeConstraint, Scope: experience.ScopeTenant,
		Trigger: "feature flag", Content: "禁止打开开关 before deploy",
		SourceEpisodeID: "ep_new", Confidence: 0.95, Embedding: unitVec(4),
		Evidence: experience.Evidence{
			SourceEpisodeID: "ep_new", SupportEpisodeIDs: []string{"ep_new", "ep_n2", "ep_n3", "ep_n4"},
			SuccessAttemptCount: 4, HasFailureContrast: true,
		},
	})
	newer.Utility = 0.8
	newer, err = repo.Update(ctx, newer)
	if err != nil {
		t.Fatal(err)
	}

	res, err := svc.ResolveConflict(ctx, "t", newer.ID, old.ID, 0.99)
	if err != nil {
		t.Fatal(err)
	}
	if res.Kind != experience.ConflictSuperseded {
		t.Fatalf("kind=%s want SUPERSEDED authW=%v authL=%v", res.Kind, res.WinnerAuthority, res.LoserAuthority)
	}
	if res.WinnerID != newer.ID || res.LoserID != old.ID {
		t.Fatalf("winner=%s loser=%s", res.WinnerID, res.LoserID)
	}
	if res.Relation.Type != experience.RelationSupersedes {
		t.Fatalf("relation type=%s", res.Relation.Type)
	}

	gotOld, err := svc.Get(ctx, "t", old.ID)
	if err != nil {
		t.Fatal(err)
	}
	if gotOld.Status != experience.StatusDeprecated {
		t.Fatalf("old status=%s want DEPRECATED", gotOld.Status)
	}
	gotNew, err := svc.Get(ctx, "t", newer.ID)
	if err != nil {
		t.Fatal(err)
	}
	if gotNew.Status != experience.StatusActive && gotNew.Status != experience.StatusCandidate {
		t.Fatalf("winner status=%s", gotNew.Status)
	}
	if gotNew.SupersedesID == nil || *gotNew.SupersedesID != old.ID {
		t.Fatalf("supersedes_id=%v", gotNew.SupersedesID)
	}

	peers, err := svc.ConflictPeers(ctx, "t", []string{newer.ID, old.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(peers) != 0 {
		t.Fatalf("deprecated loser should not block winner via CONFLICTS: %v", peers)
	}
}

func TestResolveConflictKeepsBothWhenAuthorityClose(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := experience.NewMemoryRepository()
	svc := experience.NewService(repo).WithRelations(experience.NewMemoryRelationRepository())

	a := mustCreate(t, svc, experience.CreateInput{
		TenantID: "t", Type: experience.TypeConstraint, Scope: experience.ScopeTenant,
		Trigger: "flag", Content: "必须打开开关", SourceEpisodeID: "ep_a",
		Confidence: 0.8, Embedding: unitVec(4),
		Evidence: experience.Evidence{SourceEpisodeID: "ep_a", SupportEpisodeIDs: []string{"ep_a"}},
	})
	b := mustCreate(t, svc, experience.CreateInput{
		TenantID: "t", Type: experience.TypeConstraint, Scope: experience.ScopeTenant,
		Trigger: "flag", Content: "禁止打开开关", SourceEpisodeID: "ep_b",
		Confidence: 0.8, Embedding: unitVec(4),
		Evidence: experience.Evidence{SourceEpisodeID: "ep_b", SupportEpisodeIDs: []string{"ep_b"}},
	})

	res, err := svc.ResolveConflict(ctx, "t", a.ID, b.ID, 0.98)
	if err != nil {
		t.Fatal(err)
	}
	if res.Kind != experience.ConflictUnresolved {
		t.Fatalf("kind=%s want UNRESOLVED (gap=%v)", res.Kind, res.WinnerAuthority-res.LoserAuthority)
	}
	if res.Relation.Type != experience.RelationConflicts {
		t.Fatalf("relation=%s", res.Relation.Type)
	}
	for _, id := range []string{a.ID, b.ID} {
		got, err := svc.Get(ctx, "t", id)
		if err != nil {
			t.Fatal(err)
		}
		if got.Status != experience.StatusActive && got.Status != experience.StatusCandidate {
			t.Fatalf("%s status=%s", id, got.Status)
		}
	}
	peers, err := svc.ConflictPeers(ctx, "t", []string{a.ID, b.ID})
	if err != nil {
		t.Fatal(err)
	}
	if peers[a.ID] != b.ID || peers[b.ID] != a.ID {
		t.Fatalf("peers=%v", peers)
	}
}

func mustCreate(t *testing.T, svc *experience.Service, in experience.CreateInput) experience.Experience {
	t.Helper()
	if in.Status == "" {
		in.Status = experience.StatusActive
	}
	got, err := svc.Create(context.Background(), in)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	return got
}

func unitVec(dim int) []float32 {
	v := make([]float32, dim)
	v[0] = 1
	return v
}
