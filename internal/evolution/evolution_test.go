package evolution_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/agent-experience-engine/agent-experience-engine/internal/evolution"
	"github.com/agent-experience-engine/agent-experience-engine/internal/experience"
	"github.com/agent-experience-engine/agent-experience-engine/internal/provider"
	"github.com/agent-experience-engine/agent-experience-engine/internal/retrieval"
)

func TestSupersedeDeprecatesOldAndExcludesFromRetrieval(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := experience.NewMemoryRepository()
	svc := experience.NewService(repo)
	embedder := &provider.MockEmbedding{Dim: 16}
	evo, err := evolution.New(repo, retrieval.DefaultRankConfig())
	if err != nil {
		t.Fatalf("evolution.New: %v", err)
	}
	retriever, err := retrieval.New(svc, embedder, retrieval.RankConfig{CandidateTopK: 10, DefaultTopK: 10})
	if err != nil {
		t.Fatalf("retriever: %v", err)
	}

	task := "jira project key for payment"
	vec, _ := embedder.Embed(ctx, []string{task})
	oldExp, err := svc.Create(ctx, experience.CreateInput{
		TenantID: "t", Type: experience.TypeSemantic, Scope: experience.ScopeTool, ScopeKey: "jira",
		Trigger: task, Content: "PAY project key = PAYMENT", Confidence: 0.9, Embedding: vec[0],
	})
	if err != nil {
		t.Fatalf("create old: %v", err)
	}
	newExp, err := svc.Create(ctx, experience.CreateInput{
		TenantID: "t", Type: experience.TypeSemantic, Scope: experience.ScopeTool, ScopeKey: "jira",
		Trigger: task, Content: "PAY project key = PAY", Confidence: 0.95, Embedding: vec[0],
	})
	if err != nil {
		t.Fatalf("create new: %v", err)
	}

	before, err := retriever.Retrieve(ctx, retrieval.Query{TenantID: "t", Task: task, Tools: []string{"jira"}, TopK: 10})
	if err != nil {
		t.Fatalf("retrieve before: %v", err)
	}
	if !containsID(before, oldExp.ID) {
		t.Fatalf("old experience missing before supersede: %#v", before)
	}

	if err := evo.Supersede(ctx, "t", oldExp.ID, newExp.ID); err != nil {
		t.Fatalf("Supersede: %v", err)
	}

	gotOld, err := svc.Get(ctx, "t", oldExp.ID)
	if err != nil {
		t.Fatalf("get old: %v", err)
	}
	if gotOld.Status != experience.StatusDeprecated {
		t.Fatalf("old status=%s", gotOld.Status)
	}
	gotNew, err := svc.Get(ctx, "t", newExp.ID)
	if err != nil {
		t.Fatalf("get new: %v", err)
	}
	if gotNew.SupersedesID == nil || *gotNew.SupersedesID != oldExp.ID {
		t.Fatalf("new supersedes_id=%v want %s", gotNew.SupersedesID, oldExp.ID)
	}

	after, err := retriever.Retrieve(ctx, retrieval.Query{TenantID: "t", Task: task, Tools: []string{"jira"}, TopK: 10})
	if err != nil {
		t.Fatalf("retrieve after: %v", err)
	}
	if containsID(after, oldExp.ID) {
		t.Fatalf("deprecated experience still retrieved: %#v", after)
	}
	if !containsID(after, newExp.ID) {
		t.Fatalf("replacement missing after supersede: %#v", after)
	}
}

func TestDecayDeclinesForLongUnused(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 8, 29, 12, 0, 0, 0, time.UTC)
	repo := experience.NewMemoryRepository()
	evo, err := evolution.New(repo, retrieval.DefaultRankConfig())
	if err != nil {
		t.Fatalf("evolution.New: %v", err)
	}

	usedAt := now.Add(-60 * 24 * time.Hour)
	fresh := experience.Experience{
		Type: experience.TypeProcedural, Scope: experience.ScopeTool,
		UpdatedAt: now, LastUsedAt: &now,
	}
	stale := experience.Experience{
		Type: experience.TypeProcedural, Scope: experience.ScopeTool,
		UpdatedAt: usedAt, LastUsedAt: &usedAt,
	}

	fFresh := evo.Decay(fresh, now)
	fStale := evo.Decay(stale, now)
	if !(fFresh > fStale) {
		t.Fatalf("fresh=%v should be > stale=%v", fFresh, fStale)
	}
	if fStale <= 0 || fStale >= 1 {
		t.Fatalf("stale freshness out of range: %v", fStale)
	}
}

func TestMergeAndGeneralizeNotImplemented(t *testing.T) {
	t.Parallel()
	evo, err := evolution.New(experience.NewMemoryRepository(), retrieval.DefaultRankConfig())
	if err != nil {
		t.Fatalf("evolution.New: %v", err)
	}
	if _, err := evo.Merge(context.Background(), "t", []string{"a", "b"}); !errors.Is(err, evolution.ErrNotImplemented) {
		t.Fatalf("Merge: %v", err)
	}
	if _, err := evo.Generalize(context.Background(), "t", "a"); !errors.Is(err, evolution.ErrNotImplemented) {
		t.Fatalf("Generalize: %v", err)
	}
}

func containsID(ranked []retrieval.RankedExperience, id string) bool {
	for _, r := range ranked {
		if r.Experience.ID == id {
			return true
		}
	}
	return false
}
