package postgres_test

import (
	"context"
	"math"
	"testing"

	"github.com/agent-experience-engine/agent-experience-engine/internal/episode"
	"github.com/agent-experience-engine/agent-experience-engine/internal/feedback"
	"github.com/agent-experience-engine/agent-experience-engine/storage/postgres"
)

func TestFeedbackRepositoryPersistsRawSignals(t *testing.T) {
	db := openTestDB(t)
	epSvc := episode.NewService(postgres.NewEpisodeRepository(db))
	fbSvc := feedback.NewService(postgres.NewFeedbackRepository(db), epSvc, feedback.NewRewardEngine(nil))
	ctx := context.Background()

	ep, err := epSvc.CreateEpisode(ctx, episode.CreateEpisodeInput{
		TenantID: "tenant_fb", AgentID: "a", UserID: "u", Goal: "g",
	})
	if err != nil {
		t.Fatalf("CreateEpisode: %v", err)
	}
	if _, _, err := epSvc.CompleteEpisode(ctx, episode.CompleteEpisodeInput{
		TenantID: ep.TenantID, EpisodeID: ep.ID, Status: episode.StatusSuccess,
	}); err != nil {
		t.Fatalf("Complete: %v", err)
	}

	r := 1.0
	res, err := fbSvc.Submit(ctx, feedback.SubmitInput{
		TenantID: ep.TenantID, EpisodeID: ep.ID, Source: "BUSINESS", Reward: &r, Confidence: 1,
	})
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if res.EpisodeReward.WeightedReward != 1 {
		t.Fatalf("reward=%v", res.EpisodeReward.WeightedReward)
	}

	neg := -1.0
	res, err = fbSvc.Submit(ctx, feedback.SubmitInput{
		TenantID: ep.TenantID, EpisodeID: ep.ID, Source: "LLM_JUDGE", Reward: &neg, Confidence: 1,
	})
	if err != nil {
		t.Fatalf("Submit2: %v", err)
	}
	want := 0.5 / 1.5
	if math.Abs(res.EpisodeReward.WeightedReward-want) > 1e-9 {
		t.Fatalf("weighted=%v want %v", res.EpisodeReward.WeightedReward, want)
	}

	agg, rows, err := fbSvc.GetEpisodeReward(ctx, ep.TenantID, ep.ID)
	if err != nil {
		t.Fatalf("GetEpisodeReward: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("rows=%d", len(rows))
	}
	if math.Abs(agg.WeightedReward-want) > 1e-9 {
		t.Fatalf("agg=%v", agg.WeightedReward)
	}
}
