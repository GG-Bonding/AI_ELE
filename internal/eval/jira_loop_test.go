package eval_test

import (
	"context"
	"strings"
	"testing"

	"github.com/agent-experience-engine/agent-experience-engine/internal/contextx"
	"github.com/agent-experience-engine/agent-experience-engine/internal/episode"
	"github.com/agent-experience-engine/agent-experience-engine/internal/eval"
	"github.com/agent-experience-engine/agent-experience-engine/internal/eval/jirasim"
	"github.com/agent-experience-engine/agent-experience-engine/internal/evaluator"
	"github.com/agent-experience-engine/agent-experience-engine/internal/experience"
	"github.com/agent-experience-engine/agent-experience-engine/internal/extractor"
	"github.com/agent-experience-engine/agent-experience-engine/internal/feedback"
	"github.com/agent-experience-engine/agent-experience-engine/internal/outcome"
	"github.com/agent-experience-engine/agent-experience-engine/internal/provider"
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
	t.Logf("baseline=%+v raw=%+v util=%+v learn=%+v", base, raw, util, learn)

	if base.TaskSuccessRate != 0 {
		t.Fatalf("baseline should fail: %+v", base)
	}
	if raw.NegativeTransferRate <= 0 {
		t.Fatalf("raw should show negative transfer: %+v", raw)
	}
	if !(util.TaskSuccessRate > raw.TaskSuccessRate) {
		t.Fatalf("utility should beat raw: util=%v raw=%v", util.TaskSuccessRate, raw.TaskSuccessRate)
	}
	if !(learn.TaskSuccessRate >= util.TaskSuccessRate) {
		t.Fatalf("learning should >= utility: learn=%v util=%v", learn.TaskSuccessRate, util.TaskSuccessRate)
	}
	if !(learn.HelpfulUtilityFinal > 0.6) {
		t.Fatalf("learning should raise helpful utility: %+v", learn)
	}
}

func TestJiraTraceToExperienceLearningLoop(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	engine, err := eval.NewEngine()
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	sim := jirasim.New()
	task := "Create a Jira issue for payment timeout"

	ep1, err := engine.Episodes.CreateEpisode(ctx, episode.CreateEpisodeInput{
		TenantID: "eval_tenant", AgentID: "eval_agent", UserID: "eval_user", Goal: task,
	})
	if err != nil {
		t.Fatalf("ep1: %v", err)
	}
	fail := sim.Call("jira.create_issue", map[string]any{"project": "Payment"})
	if fail.OK {
		t.Fatal("expected display-name create to fail")
	}
	if _, err := engine.Episodes.AddAttempt(ctx, episode.AddAttemptInput{
		TenantID: "eval_tenant", EpisodeID: ep1.ID, Action: "create_issue", ToolName: "jira.create_issue",
		Status: episode.AttemptStatusFailed, ErrorCode: fail.ErrorCode,
		Input: jirasim.MustJSON(map[string]any{"project": "Payment"}), Output: jirasim.MustJSON(fail.Payload),
	}); err != nil {
		t.Fatalf("fail attempt: %v", err)
	}
	search := sim.Call("jira.search_projects", map[string]any{"query": "Payment"})
	if _, err := engine.Episodes.AddAttempt(ctx, episode.AddAttemptInput{
		TenantID: "eval_tenant", EpisodeID: ep1.ID, Action: "search_projects", ToolName: "jira.search_projects",
		Status: episode.AttemptStatusSuccess,
		Input:  jirasim.MustJSON(map[string]any{"query": "Payment"}), Output: jirasim.MustJSON(search.Payload),
	}); err != nil {
		t.Fatalf("search attempt: %v", err)
	}
	okCreate := sim.Call("jira.create_issue", map[string]any{"project": "PAY", "summary": "timeout"})
	if !okCreate.OK {
		t.Fatalf("PAY create should succeed: %#v", okCreate)
	}
	if _, err := engine.Episodes.AddAttempt(ctx, episode.AddAttemptInput{
		TenantID: "eval_tenant", EpisodeID: ep1.ID, Action: "create_issue", ToolName: "jira.create_issue",
		Status: episode.AttemptStatusSuccess,
		Input:  jirasim.MustJSON(map[string]any{"project": "PAY"}), Output: jirasim.MustJSON(okCreate.Payload),
	}); err != nil {
		t.Fatalf("ok attempt: %v", err)
	}
	_, out, err := engine.Episodes.CompleteEpisode(ctx, episode.CompleteEpisodeInput{
		TenantID: "eval_tenant", EpisodeID: ep1.ID, Status: episode.StatusSuccess,
		Verified: true, Verifier: "tool", Result: jirasim.MustJSON(okCreate.Payload),
	})
	if err != nil {
		t.Fatalf("complete ep1: %v", err)
	}

	attempts, err := engine.Episodes.ListAttempts(ctx, "eval_tenant", ep1.ID)
	if err != nil {
		t.Fatalf("list attempts: %v", err)
	}

	mock := &provider.MockLLM{Responses: []string{`{"experiences":[{
		"type":"PROCEDURAL",
		"trigger":"create jira issue when project key unknown",
		"content":"Resolve project key via jira.search_projects before create_issue; INVALID_PROJECT_KEY means use PAY not Payment display name",
		"confidence":0.85,
		"scope":"TOOL",
		"scope_key":"jira"
	}]}`}}
	ext, err := extractor.New(mock)
	if err != nil {
		t.Fatalf("extractor: %v", err)
	}
	cands, err := ext.Extract(ctx, extractor.ExtractInput{Episode: ep1, Attempts: attempts, Outcome: out})
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	if len(mock.Calls) == 0 {
		t.Fatal("expected LLM call")
	}
	prompt := mock.Calls[0].User
	for _, signal := range []string{"INVALID_PROJECT_KEY", "Payment", "PAY"} {
		if !strings.Contains(prompt, signal) {
			t.Fatalf("extract prompt missing %q:\n%s", signal, prompt)
		}
	}

	pipeline, err := experience.NewStorePipeline(engine.Experiences, engine.Embedder, experience.StorePipelineConfig{
		ActiveMin: 0.65, CandidateMin: 0.4,
	})
	if err != nil {
		t.Fatalf("pipeline: %v", err)
	}
	ev := evaluator.FromAttempts(ep1.ID, out.ID, attempts)
	stored, err := pipeline.StoreCandidatesWithOptions(ctx, "eval_tenant", ep1.ID, cands, experience.StoreOptions{
		Outcome:  outcome.Outcome(out),
		Evidence: ev,
	})
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	if len(stored.Stored) != 1 {
		t.Fatalf("want 1 stored, skipped=%d evals=%#v", stored.Skipped, stored.Evaluations)
	}
	helpful := stored.Stored[0]
	if helpful.SourceEpisodeID != ep1.ID {
		t.Fatalf("source=%s want %s", helpful.SourceEpisodeID, ep1.ID)
	}
	engine.HelpfulID = helpful.ID
	before := helpful.Utility
	vecs, _ := engine.Embedder.Embed(ctx, []string{task})
	helpful.Embedding = vecs[0]
	if _, err := engine.ExpRepo.Update(ctx, helpful); err != nil {
		t.Fatalf("embed update: %v", err)
	}

	runLearnedTask := func(name string) (utility float64, score float64) {
		t.Helper()
		ep, err := engine.Episodes.CreateEpisode(ctx, episode.CreateEpisodeInput{
			TenantID: "eval_tenant", AgentID: "eval_agent", UserID: "eval_user", Goal: task,
		})
		if err != nil {
			t.Fatalf("%s create: %v", name, err)
		}
		built, err := engine.Context.BuildContext(ctx, contextx.Request{
			TenantID: "eval_tenant", AgentID: "eval_agent", UserID: "eval_user",
			EpisodeID: ep.ID, Task: task, Tools: []string{"jira"}, MaxExperiences: 5,
		})
		if err != nil {
			t.Fatalf("%s context: %v", name, err)
		}
		if len(built.Context.Experiences) == 0 {
			t.Fatalf("%s expected context experiences; selections=%+v", name, built.Selections)
		}
		var contents []string
		for _, item := range built.Context.Experiences {
			contents = append(contents, item.Content)
		}
		ok, _ := sim.Run(jirasim.AgentPolicy{}.Plan(task, contents))
		if !ok {
			t.Fatalf("%s simulator should succeed", name)
		}
		if _, _, err := engine.Episodes.CompleteEpisode(ctx, episode.CompleteEpisodeInput{
			TenantID: "eval_tenant", EpisodeID: ep.ID, Status: episode.StatusSuccess, Verified: true, Verifier: "tool",
		}); err != nil {
			t.Fatalf("%s complete: %v", name, err)
		}
		reward := 1.0
		if _, err := engine.Feedback.Submit(ctx, feedback.SubmitInput{
			TenantID: "eval_tenant", EpisodeID: ep.ID, Source: "business", Reward: &reward, Confidence: 1,
		}); err != nil {
			t.Fatalf("%s feedback: %v", name, err)
		}
		got, err := engine.Experiences.Get(ctx, "eval_tenant", helpful.ID)
		if err != nil {
			t.Fatalf("%s get: %v", name, err)
		}
		ranked, err := engine.Retriever.Retrieve(ctx, retrieval.Query{
			TenantID: "eval_tenant", Task: task, Tools: []string{"jira"}, TopK: 5,
		})
		if err != nil {
			t.Fatalf("%s retrieve: %v", name, err)
		}
		return got.Utility, finalScoreOf(ranked, helpful.ID)
	}

	u2, s2 := runLearnedTask("task2")
	if !(u2 > before) {
		t.Fatalf("utility did not rise: before=%v after=%v", before, u2)
	}
	u3, s3 := runLearnedTask("task3")
	if !(u3 > u2) {
		t.Fatalf("utility did not rise further: %v -> %v", u2, u3)
	}
	if !(s3 > s2) {
		t.Fatalf("ranking score did not rise: %v -> %v", s2, s3)
	}
}

func TestAggregateMetrics(t *testing.T) {
	t.Parallel()
	results := []eval.TaskResult{
		{Success: true, RetrievedIDs: []string{"a", "b"}, RelevantIDs: []string{"a"}, UsedExperience: true, Tokens: 10},
		{Success: false, RetrievedIDs: []string{"b"}, RelevantIDs: []string{"a"}, NegativeTransfer: true, Tokens: 20},
	}
	m := eval.Aggregate(eval.ArmRawRetrieval, results)
	if m.TaskSuccessRate != 0.5 || m.RetrievalPrecision != 1.0/3.0 || m.TokenCost != 30 {
		t.Fatalf("%+v", m)
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
