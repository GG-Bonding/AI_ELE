package eval_test

import (
	"context"
	"testing"

	"github.com/agent-experience-engine/agent-experience-engine/internal/contextx"
	"github.com/agent-experience-engine/agent-experience-engine/internal/episode"
	"github.com/agent-experience-engine/agent-experience-engine/internal/eval"
	"github.com/agent-experience-engine/agent-experience-engine/internal/feedback"
	"github.com/agent-experience-engine/agent-experience-engine/internal/retrieval"
)

func TestCompareArmsLearningBeatsStaticAndRaw(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	metrics, err := eval.CompareArms(ctx)
	if err != nil {
		t.Fatalf("CompareArms: %v", err)
	}

	base := metrics[eval.ArmBaseline]
	raw := metrics[eval.ArmRawRetrieval]
	util := metrics[eval.ArmUtilityRetrieval]
	learn := metrics[eval.ArmUtilityLearning]

	t.Logf("baseline:  %+v", base)
	t.Logf("raw:       %+v", raw)
	t.Logf("utility:   %+v", util)
	t.Logf("learning:  %+v", learn)

	if base.TaskSuccessRate != 0 {
		t.Fatalf("baseline should fail without experiences: %+v", base)
	}
	if raw.NegativeTransferRate <= 0 {
		t.Fatalf("raw retrieval should exhibit negative transfer: %+v", raw)
	}
	if !(util.TaskSuccessRate > raw.TaskSuccessRate) {
		t.Fatalf("utility retrieval should beat raw success: util=%v raw=%v", util.TaskSuccessRate, raw.TaskSuccessRate)
	}
	if !(learn.TaskSuccessRate >= util.TaskSuccessRate) {
		t.Fatalf("learning success should be at least utility: learn=%v util=%v", learn.TaskSuccessRate, util.TaskSuccessRate)
	}
	if !(learn.HelpfulUtilityFinal > 0.6) {
		t.Fatalf("learning should raise helpful utility above prior: %+v", learn)
	}
	if !(learn.TaskSuccessRate > base.TaskSuccessRate && learn.TaskSuccessRate > raw.TaskSuccessRate) {
		t.Fatalf("learning should beat baseline and raw")
	}
}

func TestJiraExperienceLearningLoop(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	engine, err := eval.NewEngine()
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}

	task := "Create a Jira issue for payment timeout"

	// Task 1: fail → search → success (discovery). Persist extracted procedural tip.
	ep1, err := engine.Episodes.CreateEpisode(ctx, episode.CreateEpisodeInput{
		TenantID: "eval_tenant", AgentID: "eval_agent", UserID: "eval_user", Goal: task,
	})
	if err != nil {
		t.Fatalf("create ep1: %v", err)
	}
	if _, err := engine.Episodes.AddAttempt(ctx, episode.AddAttemptInput{
		TenantID: "eval_tenant", EpisodeID: ep1.ID, Action: "create_issue",
		ToolName: "jira.create_issue", Status: episode.AttemptStatusFailed, ErrorCode: "INVALID_PROJECT_KEY",
	}); err != nil {
		t.Fatalf("attempt fail: %v", err)
	}
	if _, err := engine.Episodes.AddAttempt(ctx, episode.AddAttemptInput{
		TenantID: "eval_tenant", EpisodeID: ep1.ID, Action: "search_projects",
		ToolName: "jira.search_projects", Status: episode.AttemptStatusSuccess,
	}); err != nil {
		t.Fatalf("attempt search: %v", err)
	}
	if _, _, err := engine.Episodes.CompleteEpisode(ctx, episode.CompleteEpisodeInput{
		TenantID: "eval_tenant", EpisodeID: ep1.ID, Status: episode.StatusSuccess, Verified: true, Verifier: "tool",
	}); err != nil {
		t.Fatalf("complete ep1: %v", err)
	}
	if err := engine.SeedHelpfulOnly(ctx, 0.5); err != nil {
		t.Fatalf("seed extracted experience: %v", err)
	}
	vecs, err := engine.Embedder.Embed(ctx, []string{task})
	if err != nil {
		t.Fatalf("embed: %v", err)
	}
	stored, err := engine.Experiences.Get(ctx, "eval_tenant", engine.HelpfulID)
	if err != nil {
		t.Fatalf("get seeded: %v", err)
	}
	stored.Embedding = vecs[0]
	if _, err := engine.ExpRepo.Update(ctx, stored); err != nil {
		t.Fatalf("update embedding: %v", err)
	}
	before := stored.Utility

	// Task 2: retrieve → KEEP → success → business feedback +1 → utility ↑
	ep2, err := engine.Episodes.CreateEpisode(ctx, episode.CreateEpisodeInput{
		TenantID: "eval_tenant", AgentID: "eval_agent", UserID: "eval_user",
		Goal: "Create another Jira issue",
	})
	if err != nil {
		t.Fatalf("create ep2: %v", err)
	}
	built2, err := engine.Context.BuildContext(ctx, contextx.Request{
		TenantID: "eval_tenant", AgentID: "eval_agent", UserID: "eval_user",
		EpisodeID: ep2.ID, Task: task, Tools: []string{"jira"}, MaxExperiences: 5,
	})
	if err != nil {
		t.Fatalf("context2: %v", err)
	}
	if len(built2.Context.Experiences) == 0 {
		t.Fatalf("expected experience in context; selections=%+v", built2.Selections)
	}
	if _, _, err := engine.Episodes.CompleteEpisode(ctx, episode.CompleteEpisodeInput{
		TenantID: "eval_tenant", EpisodeID: ep2.ID, Status: episode.StatusSuccess, Verified: true, Verifier: "tool",
	}); err != nil {
		t.Fatalf("complete ep2: %v", err)
	}
	reward := 1.0
	if _, err := engine.Feedback.Submit(ctx, feedback.SubmitInput{
		TenantID: "eval_tenant", EpisodeID: ep2.ID, Source: "business", Reward: &reward, Confidence: 1,
	}); err != nil {
		t.Fatalf("feedback2: %v", err)
	}
	after2, err := engine.Experiences.Get(ctx, "eval_tenant", engine.HelpfulID)
	if err != nil {
		t.Fatalf("get after2: %v", err)
	}
	if !(after2.Utility > before) {
		t.Fatalf("utility did not rise: before=%v after=%v", before, after2.Utility)
	}
	rank2, err := engine.Retriever.Retrieve(ctx, retrieval.Query{
		TenantID: "eval_tenant", Task: task, Tools: []string{"jira"}, TopK: 5,
	})
	if err != nil {
		t.Fatalf("rank2: %v", err)
	}
	score2 := finalScoreOf(rank2, engine.HelpfulID)

	// Task 3: another success further raises utility / ranking score
	ep3, err := engine.Episodes.CreateEpisode(ctx, episode.CreateEpisodeInput{
		TenantID: "eval_tenant", AgentID: "eval_agent", UserID: "eval_user",
		Goal: "Create a Jira issue again",
	})
	if err != nil {
		t.Fatalf("create ep3: %v", err)
	}
	if _, err := engine.Context.BuildContext(ctx, contextx.Request{
		TenantID: "eval_tenant", EpisodeID: ep3.ID, Task: task, Tools: []string{"jira"}, MaxExperiences: 5,
	}); err != nil {
		t.Fatalf("context3: %v", err)
	}
	if _, _, err := engine.Episodes.CompleteEpisode(ctx, episode.CompleteEpisodeInput{
		TenantID: "eval_tenant", EpisodeID: ep3.ID, Status: episode.StatusSuccess,
	}); err != nil {
		t.Fatalf("complete ep3: %v", err)
	}
	if _, err := engine.Feedback.Submit(ctx, feedback.SubmitInput{
		TenantID: "eval_tenant", EpisodeID: ep3.ID, Source: "business", Reward: &reward, Confidence: 1,
	}); err != nil {
		t.Fatalf("feedback3: %v", err)
	}
	after3, err := engine.Experiences.Get(ctx, "eval_tenant", engine.HelpfulID)
	if err != nil {
		t.Fatalf("get after3: %v", err)
	}
	if !(after3.Utility > after2.Utility) {
		t.Fatalf("utility did not rise further: after2=%v after3=%v", after2.Utility, after3.Utility)
	}
	rank3, err := engine.Retriever.Retrieve(ctx, retrieval.Query{
		TenantID: "eval_tenant", Task: task, Tools: []string{"jira"}, TopK: 5,
	})
	if err != nil {
		t.Fatalf("rank3: %v", err)
	}
	score3 := finalScoreOf(rank3, engine.HelpfulID)
	if !(score3 > score2) {
		t.Fatalf("ranking score did not rise: score2=%v score3=%v ranks=%+v", score2, score3, rank3)
	}
}

func TestAggregateMetrics(t *testing.T) {
	t.Parallel()
	results := []eval.TaskResult{
		{Success: true, RetrievedIDs: []string{"a", "b"}, RelevantIDs: []string{"a"}, UsedExperience: true, Tokens: 10},
		{Success: false, RetrievedIDs: []string{"b"}, RelevantIDs: []string{"a"}, NegativeTransfer: true, Tokens: 20},
	}
	m := eval.Aggregate(eval.ArmRawRetrieval, results)
	if m.TaskSuccessRate != 0.5 {
		t.Fatalf("success=%v", m.TaskSuccessRate)
	}
	if m.RetrievalPrecision != 1.0/3.0 {
		t.Fatalf("precision=%v", m.RetrievalPrecision)
	}
	if m.ExperienceUtilization != 0.5 {
		t.Fatalf("utilization=%v", m.ExperienceUtilization)
	}
	if m.NegativeTransferRate != 0.5 {
		t.Fatalf("neg=%v", m.NegativeTransferRate)
	}
	if m.TokenCost != 30 {
		t.Fatalf("tokens=%v", m.TokenCost)
	}
}

func finalScoreOf(ranked []retrieval.RankedExperience, id string) float64 {
	for _, r := range ranked {
		if r.Experience.ID == id {
			return r.Score.FinalScore
		}
	}
	return 0
}
