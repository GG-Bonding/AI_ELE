package experience_test

import (
	"context"
	"strings"
	"testing"

	"github.com/agent-experience-engine/agent-experience-engine/internal/experience"
	"github.com/agent-experience-engine/agent-experience-engine/internal/provider"
)

func TestStructuredJudgmentToDedupDecision(t *testing.T) {
	t.Parallel()
	cases := []struct {
		rel  experience.StructuredRelation
		want experience.DedupDecision
	}{
		{experience.StructuredSame, experience.DedupSame},
		{experience.StructuredSupports, experience.DedupRelated},
		{experience.StructuredSpecializes, experience.DedupRelated},
		{experience.StructuredGeneralizes, experience.DedupRelated},
		{experience.StructuredConflicts, experience.DedupConflict},
		{experience.StructuredSupersedes, experience.DedupConflict},
		{experience.StructuredUnrelated, experience.DedupDifferent},
	}
	for _, tc := range cases {
		got := experience.StructuredJudgment{Relation: tc.rel}.ToDedupDecision()
		if got != tc.want {
			t.Fatalf("%s → %s want %s", tc.rel, got, tc.want)
		}
	}
}

func TestHybridDedupJudgePolarityFastPath(t *testing.T) {
	t.Parallel()
	mock := &provider.MockLLM{Responses: []string{`should not be called`}}
	j := experience.NewHybridDedupJudge(mock)
	ctx := context.Background()

	got, err := j.JudgeStructured(ctx, experience.DedupPair{
		CandidateTrigger: "feature flag",
		CandidateContent: "必须打开开关 before deploy",
		NeighborTrigger:  "feature flag",
		NeighborContent:  "禁止打开开关 before deploy",
		Similarity:       0.96,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Relation != experience.StructuredConflicts || got.ConflictType != "POLARITY" {
		t.Fatalf("got %#v", got)
	}
	if len(mock.Calls) != 0 {
		t.Fatal("LLM should not be called for polarity fast-path")
	}
}

func TestHybridDedupJudgeSpecializationViaLLM(t *testing.T) {
	t.Parallel()
	mock := &provider.MockLLM{Responses: []string{`{
		"relation":"SPECIALIZES",
		"subject":"refund_window",
		"old_value":"30 days",
		"new_value":"45 days",
		"qualifiers":["VIP"],
		"conflict_type":"",
		"confidence":0.93,
		"reason":"VIP exception widens the normal window"
	}`}}
	j := experience.NewHybridDedupJudge(mock)
	ctx := context.Background()

	got, err := j.JudgeStructured(ctx, experience.DedupPair{
		CandidateTrigger: "refund policy for VIP",
		CandidateContent: "VIP customers get a 45 day refund window",
		NeighborTrigger:  "refund policy",
		NeighborContent:  "Customers get a 30 day refund window",
		Similarity:       0.94,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Relation != experience.StructuredSpecializes {
		t.Fatalf("got %s want SPECIALIZES", got.Relation)
	}
	if got.ToDedupDecision() != experience.DedupRelated {
		t.Fatalf("SPECIALIZES should map to RELATED, got %s", got.ToDedupDecision())
	}
	if len(mock.Calls) != 1 {
		t.Fatalf("want 1 LLM call, got %d", len(mock.Calls))
	}
	if !strings.Contains(mock.Calls[0].User, "45 day") {
		t.Fatalf("prompt missing candidate: %s", mock.Calls[0].User)
	}
}

func TestHybridDedupJudgeTemporalSupersedesViaLLM(t *testing.T) {
	t.Parallel()
	mock := &provider.MockLLM{Responses: []string{`{
		"relation":"SUPERSEDES",
		"subject":"refund_window",
		"old_value":"30 days",
		"new_value":"14 days",
		"qualifiers":[],
		"conflict_type":"TEMPORAL_UPDATE",
		"confidence":0.94,
		"reason":"policy shortened refund window"
	}`}}
	j := experience.NewHybridDedupJudge(mock)
	dec, err := j.Judge(context.Background(), experience.DedupPair{
		CandidateTrigger: "refund policy",
		CandidateContent: "Refund window is 14 days",
		NeighborTrigger:  "refund policy",
		NeighborContent:  "Refund window is 30 days",
		Similarity:       0.95,
	})
	if err != nil {
		t.Fatal(err)
	}
	if dec != experience.DedupConflict {
		t.Fatalf("SUPERSEDES should map to CONFLICT for store path, got %s", dec)
	}
}

func TestHybridDedupJudgeFallsBackWhenLLMFails(t *testing.T) {
	t.Parallel()
	mock := &provider.MockLLM{
		Responses: []string{`not-json`, `still-bad`},
	}
	j := experience.NewHybridDedupJudge(mock)
	// Near-duplicate → heuristic SAME after LLM failure.
	dec, err := j.Judge(context.Background(), experience.DedupPair{
		CandidateTrigger: "create jira when project key unknown",
		CandidateContent: "Resolve the Jira project key before calling create_issue.",
		NeighborTrigger:  "create jira when project key unknown",
		NeighborContent:  "Resolve the Jira project key before calling create_issue.",
		Similarity:       0.95,
	})
	if err != nil {
		t.Fatal(err)
	}
	if dec != experience.DedupSame {
		t.Fatalf("got %s want SAME via heuristic fallback", dec)
	}
}

func TestHybridDedupJudgeSkipBelowThreshold(t *testing.T) {
	t.Parallel()
	mock := &provider.MockLLM{Responses: []string{`{"relation":"SAME","confidence":1}`}}
	j := experience.NewHybridDedupJudge(mock)
	got, err := j.JudgeStructured(context.Background(), experience.DedupPair{
		CandidateContent: "alpha", NeighborContent: "beta", Similarity: 0.5,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Relation != experience.StructuredUnrelated {
		t.Fatalf("got %s", got.Relation)
	}
	if len(mock.Calls) != 0 {
		t.Fatal("LLM should not run below skip threshold")
	}
}
