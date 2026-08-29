package feedback_test

import (
	"context"
	"errors"
	"math"
	"testing"

	"github.com/agent-experience-engine/agent-experience-engine/internal/feedback"
)

type stubEpisodes struct {
	exists map[string]bool
}

func (s stubEpisodes) EpisodeExists(_ context.Context, tenantID, episodeID string) (bool, error) {
	return s.exists[tenantID+"/"+episodeID], nil
}

func TestNormalizeRewardClamps(t *testing.T) {
	t.Parallel()
	if feedback.NormalizeReward(2) != 1 || feedback.NormalizeReward(-3) != -1 {
		t.Fatal("clamp failed")
	}
}

func TestSignalToRewardMapping(t *testing.T) {
	t.Parallel()
	cases := map[string]float64{
		"task_success":    1.0,
		"partial_success": 0.4,
		"neutral":         0,
		"partial_failure": -0.5,
		"hard_failure":    -1.0,
	}
	for sig, want := range cases {
		got, ok := feedback.SignalToReward(sig)
		if !ok || got != want {
			t.Fatalf("%s => %v ok=%v", sig, got, ok)
		}
	}
}

func TestRewardEngineWeightedAggregate(t *testing.T) {
	t.Parallel()
	engine := feedback.NewRewardEngine(nil)
	// BUSINESS(1.0)*1.0*(+1) + LLM_JUDGE(0.5)*1.0*(-1) = 1 - 0.5 = 0.5; den=1.5 => 0.5/1.5
	agg, err := engine.Aggregate("t", "ep", []feedback.Feedback{
		{Source: feedback.SourceBusiness, Reward: 1, Confidence: 1},
		{Source: feedback.SourceLLMJudge, Reward: -1, Confidence: 1},
	})
	if err != nil {
		t.Fatalf("Aggregate: %v", err)
	}
	want := 0.5 / 1.5
	if math.Abs(agg.WeightedReward-want) > 1e-9 {
		t.Fatalf("weighted=%v want %v", agg.WeightedReward, want)
	}
	if agg.FeedbackCount != 2 {
		t.Fatalf("count=%d", agg.FeedbackCount)
	}
}

func TestSubmitPersistsRawAndAggregates(t *testing.T) {
	t.Parallel()
	repo := feedback.NewMemoryRepository()
	svc := feedback.NewService(repo, stubEpisodes{exists: map[string]bool{"t/ep1": true}}, nil)

	r1 := 1.0
	res, err := svc.Submit(context.Background(), feedback.SubmitInput{
		TenantID: "t", EpisodeID: "ep1", Source: "business", Reward: &r1, Confidence: 1,
	})
	if err != nil {
		t.Fatalf("Submit1: %v", err)
	}
	if res.Feedback.ID == "" || res.EpisodeReward.WeightedReward != 1 {
		t.Fatalf("%#v", res)
	}

	r2 := -1.0
	res, err = svc.Submit(context.Background(), feedback.SubmitInput{
		TenantID: "t", EpisodeID: "ep1", Source: "llm_judge", Reward: &r2, Confidence: 1,
	})
	if err != nil {
		t.Fatalf("Submit2: %v", err)
	}
	want := 0.5 / 1.5
	if math.Abs(res.EpisodeReward.WeightedReward-want) > 1e-9 {
		t.Fatalf("agg=%v want %v", res.EpisodeReward.WeightedReward, want)
	}

	rows, err := repo.ListByEpisode(context.Background(), "t", "ep1")
	if err != nil || len(rows) != 2 {
		t.Fatalf("raw rows=%d err=%v", len(rows), err)
	}
}

func TestSubmitSignalWithoutReward(t *testing.T) {
	t.Parallel()
	svc := feedback.NewService(feedback.NewMemoryRepository(), stubEpisodes{exists: map[string]bool{"t/ep": true}}, nil)
	res, err := svc.Submit(context.Background(), feedback.SubmitInput{
		TenantID: "t", EpisodeID: "ep", Source: "tool", Signal: "partial_success",
	})
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if res.Feedback.Reward != 0.4 {
		t.Fatalf("reward=%v", res.Feedback.Reward)
	}
}

func TestSubmitMissingEpisode(t *testing.T) {
	t.Parallel()
	svc := feedback.NewService(feedback.NewMemoryRepository(), stubEpisodes{exists: map[string]bool{}}, nil)
	r := 1.0
	_, err := svc.Submit(context.Background(), feedback.SubmitInput{
		TenantID: "t", EpisodeID: "missing", Source: "tool", Reward: &r,
	})
	if !errors.Is(err, feedback.ErrNotFound) {
		t.Fatalf("err=%v", err)
	}
}

func TestTenantIsolationOnList(t *testing.T) {
	t.Parallel()
	repo := feedback.NewMemoryRepository()
	svc := feedback.NewService(repo, stubEpisodes{exists: map[string]bool{"a/ep": true, "b/ep": true}}, nil)
	r := 1.0
	_, _ = svc.Submit(context.Background(), feedback.SubmitInput{TenantID: "a", EpisodeID: "ep", Source: "tool", Reward: &r})
	rows, _ := repo.ListByEpisode(context.Background(), "b", "ep")
	if len(rows) != 0 {
		t.Fatalf("leak: %#v", rows)
	}
}
