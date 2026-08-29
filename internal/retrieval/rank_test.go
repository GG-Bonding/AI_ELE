package retrieval_test

import (
	"testing"
	"time"

	"github.com/agent-experience-engine/agent-experience-engine/internal/experience"
	"github.com/agent-experience-engine/agent-experience-engine/internal/retrieval"
)

func TestRankPrefersHighUtilityOverSlightlyHigherSimilarity(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 8, 29, 12, 0, 0, 0, time.UTC)
	cfg := retrieval.DefaultRankConfig()
	cfg.Now = func() time.Time { return now }

	highSimLowUtil := experience.ScoredExperience{
		Similarity: 0.95,
		Experience: experience.Experience{
			ID: "high_sim", Type: experience.TypeProcedural, Scope: experience.ScopeTool, ScopeKey: "jira",
			Confidence: 0.9, Utility: 0.20, UpdatedAt: now,
		},
	}
	lowerSimHighUtil := experience.ScoredExperience{
		Similarity: 0.80,
		Experience: experience.Experience{
			ID: "high_util", Type: experience.TypeProcedural, Scope: experience.ScopeTool, ScopeKey: "jira",
			Confidence: 0.9, Utility: 0.90, UpdatedAt: now,
		},
	}

	ranked := retrieval.Rank(
		[]experience.ScoredExperience{highSimLowUtil, lowerSimHighUtil},
		retrieval.ScopeContext{Tools: []string{"jira"}},
		cfg,
		now,
	)
	if len(ranked) != 2 {
		t.Fatalf("len=%d", len(ranked))
	}
	if ranked[0].Experience.ID != "high_util" {
		t.Fatalf("want high_util first, got %s (scores %#v %#v)",
			ranked[0].Experience.ID, ranked[0].Score, ranked[1].Score)
	}
	if ranked[0].Score.FinalScore <= ranked[1].Score.FinalScore {
		t.Fatalf("final scores not ordered: %#v", ranked)
	}

	// Explainable components
	wantHighUtil := 0.80 * 0.90 * 0.90 * 1.0 * 1.0
	if diff := abs(ranked[0].Score.FinalScore - wantHighUtil); diff > 1e-9 {
		t.Fatalf("final=%v want %v", ranked[0].Score.FinalScore, wantHighUtil)
	}
}

func TestFreshnessSemanticDecaysSlowerThanProcedural(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 8, 29, 12, 0, 0, 0, time.UTC)
	old := now.Add(-30 * 24 * time.Hour)
	cfg := retrieval.DefaultRankConfig()

	sem := experience.Experience{Type: experience.TypeSemantic, UpdatedAt: old}
	proc := experience.Experience{Type: experience.TypeProcedural, Scope: experience.ScopeTool, UpdatedAt: old}

	fs := retrieval.Freshness(sem, cfg, now)
	fp := retrieval.Freshness(proc, cfg, now)
	if !(fs > fp) {
		t.Fatalf("semantic freshness %v should be > procedural %v", fs, fp)
	}
	if fs <= 0 || fs > 1 || fp <= 0 || fp > 1 {
		t.Fatalf("freshness out of range: %v %v", fs, fp)
	}
}

func TestScopeMatchToolBonus(t *testing.T) {
	t.Parallel()
	exp := experience.Experience{Scope: experience.ScopeTool, ScopeKey: "jira"}
	match := retrieval.ScopeMatch(exp, retrieval.ScopeContext{Tools: []string{"jira.create_issue", "jira"}})
	miss := retrieval.ScopeMatch(exp, retrieval.ScopeContext{Tools: []string{"slack"}})
	if match <= miss {
		t.Fatalf("match=%v miss=%v", match, miss)
	}
}

func TestFinalScoreIsProductOfComponents(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	cfg := retrieval.DefaultRankConfig()
	c := experience.ScoredExperience{
		Similarity: 0.5,
		Experience: experience.Experience{
			ID: "x", Type: experience.TypeSemantic, Scope: experience.ScopeTenant,
			Confidence: 0.8, Utility: 0.5, UpdatedAt: now,
		},
	}
	ranked := retrieval.Rank([]experience.ScoredExperience{c}, retrieval.ScopeContext{}, cfg, now)
	s := ranked[0].Score
	product := s.Similarity * s.Utility * s.Confidence * s.Freshness * s.ScopeMatch
	if abs(s.FinalScore-product) > 1e-12 {
		t.Fatalf("final %v != product %v (%#v)", s.FinalScore, product, s)
	}
}

func abs(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}
