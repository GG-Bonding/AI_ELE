package learning_test

import (
	"context"
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/agent-experience-engine/agent-experience-engine/internal/attribution"
	"github.com/agent-experience-engine/agent-experience-engine/internal/contextx"
	"github.com/agent-experience-engine/agent-experience-engine/internal/episode"
	"github.com/agent-experience-engine/agent-experience-engine/internal/experience"
	"github.com/agent-experience-engine/agent-experience-engine/internal/feedback"
	"github.com/agent-experience-engine/agent-experience-engine/internal/learning"
	"github.com/agent-experience-engine/agent-experience-engine/internal/provider"
	"github.com/agent-experience-engine/agent-experience-engine/internal/retrieval"
	"github.com/agent-experience-engine/agent-experience-engine/internal/selector"
)

func TestIncrementalFeedbackDoesNotReplayAggregate(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	expRepo := experience.NewMemoryRepository()
	expSvc := experience.NewService(expRepo)
	usageRepo := experience.NewMemoryUsageRepository()
	eventRepo := learning.NewMemoryEventRepository()
	learnSvc, err := learning.NewWithEvents(usageRepo, expRepo, eventRepo, attribution.NewDefault(), nil)
	if err != nil {
		t.Fatalf("learning: %v", err)
	}
	epSvc := episode.NewService(episode.NewMemoryRepository())
	fbSvc := feedback.NewServiceWithLearner(
		feedback.NewMemoryRepository(), epSvc, nil, learning.FeedbackLearner{Inner: learnSvc},
	)

	vec := []float32{1, 0, 0, 0}
	exp, err := expSvc.Create(ctx, experience.CreateInput{
		TenantID: "t", Type: experience.TypeProcedural, Scope: experience.ScopeTool, ScopeKey: "jira",
		Trigger: "jira tip", Content: "search project first", Confidence: 0.9, Embedding: vec,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	ep, err := epSvc.CreateEpisode(ctx, episode.CreateEpisodeInput{
		TenantID: "t", AgentID: "a", UserID: "u", Goal: "jira",
	})
	if err != nil {
		t.Fatalf("episode: %v", err)
	}
	if _, _, err := epSvc.CompleteEpisode(ctx, episode.CompleteEpisodeInput{
		TenantID: "t", EpisodeID: ep.ID, Status: episode.StatusSuccess,
	}); err != nil {
		t.Fatalf("complete: %v", err)
	}
	if _, err := usageRepo.Create(ctx, experience.Usage{
		ID: "u1", TenantID: "t", EpisodeID: ep.ID, ExperienceID: exp.ID, FinalScore: 1,
	}); err != nil {
		t.Fatalf("usage: %v", err)
	}

	// alpha=1 beta=1 utility=.5
	got, _ := expSvc.Get(ctx, "t", exp.ID)
	if got.Alpha != 1 || got.Beta != 1 || math.Abs(got.Utility-0.5) > 1e-9 {
		t.Fatalf("initial %#v", got)
	}

	r1 := 1.0
	if _, err := fbSvc.Submit(ctx, feedback.SubmitInput{
		TenantID: "t", EpisodeID: ep.ID, Source: "business", Reward: &r1, Confidence: 1,
	}); err != nil {
		t.Fatalf("F1: %v", err)
	}
	after1, _ := expSvc.Get(ctx, "t", exp.ID)
	if math.Abs(after1.Alpha-2) > 1e-9 || math.Abs(after1.Beta-1) > 1e-9 {
		t.Fatalf("after F1 want alpha=2 beta=1 got %#v", after1)
	}

	r2 := -1.0
	if _, err := fbSvc.Submit(ctx, feedback.SubmitInput{
		TenantID: "t", EpisodeID: ep.ID, Source: "business", Reward: &r2, Confidence: 1,
	}); err != nil {
		t.Fatalf("F2: %v", err)
	}
	after2, _ := expSvc.Get(ctx, "t", exp.ID)
	if math.Abs(after2.Alpha-2) > 1e-9 || math.Abs(after2.Beta-2) > 1e-9 {
		t.Fatalf("after F2 want alpha=2 beta=2 got %#v (aggregate replay bug?)", after2)
	}
	if math.Abs(after2.Utility-0.5) > 1e-9 {
		t.Fatalf("utility=%v want 0.5", after2.Utility)
	}
}

func TestFeedbackIdempotencyDoesNotDoubleLearn(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	expRepo := experience.NewMemoryRepository()
	expSvc := experience.NewService(expRepo)
	usageRepo := experience.NewMemoryUsageRepository()
	learnSvc, _ := learning.New(usageRepo, expRepo, attribution.NewDefault())
	epSvc := episode.NewService(episode.NewMemoryRepository())
	fbSvc := feedback.NewServiceWithLearner(
		feedback.NewMemoryRepository(), epSvc, nil, learning.FeedbackLearner{Inner: learnSvc},
	)

	exp, _ := expSvc.Create(ctx, experience.CreateInput{
		TenantID: "t", Type: experience.TypeProcedural, Scope: experience.ScopeTool, ScopeKey: "jira",
		Trigger: "t", Content: "c", Confidence: 0.9, Embedding: []float32{1},
	})
	ep, _ := epSvc.CreateEpisode(ctx, episode.CreateEpisodeInput{TenantID: "t", AgentID: "a", UserID: "u", Goal: "g"})
	_, _, _ = epSvc.CompleteEpisode(ctx, episode.CompleteEpisodeInput{TenantID: "t", EpisodeID: ep.ID, Status: episode.StatusSuccess})
	_, _ = usageRepo.Create(ctx, experience.Usage{ID: "u1", TenantID: "t", EpisodeID: ep.ID, ExperienceID: exp.ID, FinalScore: 1})

	r := 1.0
	res1, err := fbSvc.Submit(ctx, feedback.SubmitInput{
		TenantID: "t", EpisodeID: ep.ID, Source: "business", Reward: &r, Confidence: 1,
		IdempotencyKey: "idem-1",
	})
	if err != nil {
		t.Fatalf("submit1: %v", err)
	}
	after1, _ := expSvc.Get(ctx, "t", exp.ID)

	res2, err := fbSvc.Submit(ctx, feedback.SubmitInput{
		TenantID: "t", EpisodeID: ep.ID, Source: "business", Reward: &r, Confidence: 1,
		IdempotencyKey: "idem-1",
	})
	if err != nil {
		t.Fatalf("submit2: %v", err)
	}
	if !res2.IdempotentReplay || res2.Feedback.ID != res1.Feedback.ID {
		t.Fatalf("expected idempotent replay: %#v %#v", res1, res2)
	}
	after2, _ := expSvc.Get(ctx, "t", exp.ID)
	if after2.Alpha != after1.Alpha || after2.Beta != after1.Beta {
		t.Fatalf("utility changed on replay: %#v -> %#v", after1, after2)
	}
}

func TestSuccessRaisesUtilityAndChangesRanking(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	expRepo := experience.NewMemoryRepository()
	expSvc := experience.NewService(expRepo)
	usageRepo := experience.NewMemoryUsageRepository()
	learnSvc, err := learning.New(usageRepo, expRepo, attribution.NewDefault())
	if err != nil {
		t.Fatalf("learning.New: %v", err)
	}
	embedder := &provider.MockEmbedding{Dim: 32}
	retriever, err := retrieval.New(expSvc, embedder, retrieval.RankConfig{CandidateTopK: 10, DefaultTopK: 10})
	if err != nil {
		t.Fatalf("retriever: %v", err)
	}
	contextSvc, err := contextx.NewServiceWithUsage(
		retriever,
		selector.New(selector.DefaultConfig()),
		contextx.New(contextx.DefaultConfig()),
		learning.Recorder{Inner: learnSvc},
	)
	if err != nil {
		t.Fatalf("context: %v", err)
	}
	epSvc := episode.NewService(episode.NewMemoryRepository())
	fbSvc := feedback.NewServiceWithLearner(
		feedback.NewMemoryRepository(),
		epSvc,
		feedback.NewRewardEngine(nil),
		learning.FeedbackLearner{Inner: learnSvc},
	)

	task := "create jira issue when project key unknown"
	vec, _ := embedder.Embed(ctx, []string{task})
	used, err := expSvc.Create(ctx, experience.CreateInput{
		TenantID: "t", Type: experience.TypeProcedural, Scope: experience.ScopeTool, ScopeKey: "jira",
		Trigger: task, Content: "Resolve project key before create_issue",
		Confidence: 0.9, Embedding: vec[0],
	})
	if err != nil {
		t.Fatalf("create used: %v", err)
	}
	otherTask := "unrelated slack posting tip"
	otherVec, _ := embedder.Embed(ctx, []string{otherTask})
	other, err := expSvc.Create(ctx, experience.CreateInput{
		TenantID: "t", Type: experience.TypeProcedural, Scope: experience.ScopeTool, ScopeKey: "slack",
		Trigger: otherTask, Content: "Always set channel id",
		Confidence: 0.9, Embedding: otherVec[0],
	})
	if err != nil {
		t.Fatalf("create other: %v", err)
	}

	ep, err := epSvc.CreateEpisode(ctx, episode.CreateEpisodeInput{
		TenantID: "t", AgentID: "a", UserID: "u", Goal: "Create Jira issue",
	})
	if err != nil {
		t.Fatalf("episode: %v", err)
	}
	if _, _, err := epSvc.CompleteEpisode(ctx, episode.CompleteEpisodeInput{
		TenantID: "t", EpisodeID: ep.ID, Status: episode.StatusSuccess,
	}); err != nil {
		t.Fatalf("complete: %v", err)
	}

	built, err := contextSvc.BuildContext(ctx, contextx.Request{
		TenantID: "t", EpisodeID: ep.ID, Task: task, Tools: []string{"jira"}, MaxExperiences: 5,
	})
	if err != nil {
		t.Fatalf("BuildContext: %v", err)
	}
	if len(built.Context.Experiences) == 0 {
		t.Fatalf("expected context experiences; selections=%#v", built.Selections)
	}

	before, err := expSvc.Get(ctx, "t", used.ID)
	if err != nil {
		t.Fatalf("get before: %v", err)
	}
	beforeRanked, err := retriever.Retrieve(ctx, retrieval.Query{
		TenantID: "t", Task: task, Tools: []string{"jira"}, TopK: 5,
	})
	if err != nil {
		t.Fatalf("retrieve before: %v", err)
	}
	beforeUsedScore := scoreOf(beforeRanked, used.ID)
	beforeOtherScore := scoreOf(beforeRanked, other.ID)

	reward := 1.0
	res, err := fbSvc.Submit(ctx, feedback.SubmitInput{
		TenantID: "t", EpisodeID: ep.ID, Source: "business", Reward: &reward, Confidence: 1,
	})
	if err != nil {
		t.Fatalf("feedback: %v", err)
	}
	if len(res.UtilityUpdates) == 0 {
		t.Fatalf("expected utility updates: %#v", res)
	}
	after, err := expSvc.Get(ctx, "t", used.ID)
	if err != nil {
		t.Fatalf("get after: %v", err)
	}
	if !(after.Utility > before.Utility) {
		t.Fatalf("utility did not rise: before=%v after=%v updates=%#v", before.Utility, after.Utility, res.UtilityUpdates)
	}

	ranked, err := retriever.Retrieve(ctx, retrieval.Query{
		TenantID: "t", Task: task, Tools: []string{"jira"}, TopK: 5,
	})
	if err != nil {
		t.Fatalf("retrieve: %v", err)
	}
	if len(ranked) == 0 || ranked[0].Experience.ID != used.ID {
		t.Fatalf("expected used experience to rank first after learning: %#v", ranked)
	}
	afterUsedScore := scoreOf(ranked, used.ID)
	afterOtherScore := scoreOf(ranked, other.ID)
	if !(afterUsedScore > beforeUsedScore) {
		t.Fatalf("used final score did not rise: before=%v after=%v", beforeUsedScore, afterUsedScore)
	}
	if !(afterUsedScore-afterOtherScore > beforeUsedScore-beforeOtherScore) {
		t.Fatalf("ranking gap did not improve: before used=%v other=%v; after used=%v other=%v",
			beforeUsedScore, beforeOtherScore, afterUsedScore, afterOtherScore)
	}
}

func scoreOf(ranked []retrieval.RankedExperience, id string) float64 {
	for _, r := range ranked {
		if r.Experience.ID == id {
			return r.Score.FinalScore
		}
	}
	return 0
}

func TestFailureLowersUtility(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	expRepo := experience.NewMemoryRepository()
	expSvc := experience.NewService(expRepo)
	usageRepo := experience.NewMemoryUsageRepository()
	learnSvc, _ := learning.New(usageRepo, expRepo, attribution.NewDefault())
	embedder := &provider.MockEmbedding{Dim: 16}
	vec, _ := embedder.Embed(ctx, []string{"jira tip"})
	exp, _ := expSvc.Create(ctx, experience.CreateInput{
		TenantID: "t", Type: experience.TypeProcedural, Scope: experience.ScopeTool, ScopeKey: "jira",
		Trigger: "jira tip", Content: "search project first", Confidence: 0.9, Embedding: vec[0],
	})
	_, _ = usageRepo.Create(ctx, experience.Usage{
		ID: "u1", TenantID: "t", EpisodeID: "ep1", ExperienceID: exp.ID, FinalScore: 0.5,
	})
	before := exp.Utility
	updates, err := learnSvc.ApplyFeedbackReward(ctx, "t", "ep1", "fb1", -1.0, 1.0)
	if err != nil {
		t.Fatalf("ApplyFeedbackReward: %v", err)
	}
	if len(updates) != 1 || !(updates[0].NewUtility < before) {
		t.Fatalf("%#v", updates)
	}
}

func TestRetryFailedLearningEventUpdatesUtility(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	expRepo := experience.NewMemoryRepository()
	expSvc := experience.NewService(expRepo)
	usageRepo := experience.NewMemoryUsageRepository()
	eventRepo := learning.NewMemoryEventRepository()
	learnSvc, err := learning.NewWithEvents(usageRepo, expRepo, eventRepo, attribution.NewDefault(), nil)
	if err != nil {
		t.Fatalf("learning: %v", err)
	}

	vec := []float32{1, 0}
	exp, _ := expSvc.Create(ctx, experience.CreateInput{
		TenantID: "t", Type: experience.TypeProcedural, Scope: experience.ScopeTool, ScopeKey: "jira",
		Trigger: "t", Content: "c", Confidence: 0.9, Embedding: vec,
	})
	_, _ = usageRepo.Create(ctx, experience.Usage{
		ID: "u1", TenantID: "t", EpisodeID: "ep1", ExperienceID: exp.ID, FinalScore: 1,
	})

	before := exp.Utility
	failedEv, err := eventRepo.Create(ctx, learning.Event{
		ID: "ev-failed", TenantID: "t", FeedbackID: "fb-retry", EpisodeID: "ep1",
		ExperienceID: exp.ID, NormalizedReward: 1, Confidence: 1, Credit: 1,
		EffectiveReward: 1, Status: learning.EventFailed,
	})
	if err != nil {
		t.Fatalf("create failed event: %v", err)
	}

	retryUpdates, err := learnSvc.ApplyFeedbackReward(ctx, "t", "ep1", "fb-retry", 1.0, 1.0)
	if err != nil {
		t.Fatalf("retry apply: %v", err)
	}
	if len(retryUpdates) != 1 {
		t.Fatalf("updates: %#v", retryUpdates)
	}
	if !(retryUpdates[0].NewUtility > before) {
		t.Fatalf("utility did not rise on retry: before=%v after=%v", before, retryUpdates[0].NewUtility)
	}
	ev, err := eventRepo.GetByFeedbackExperience(ctx, "t", "fb-retry", exp.ID)
	if err != nil {
		t.Fatalf("get event: %v", err)
	}
	if ev.Status != learning.EventApplied {
		t.Fatalf("event %s status = %s want APPLIED", failedEv.ID, ev.Status)
	}
}

func TestRetryUsesEventPersistedRewardNotCaller(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	expRepo := experience.NewMemoryRepository()
	expSvc := experience.NewService(expRepo)
	usageRepo := experience.NewMemoryUsageRepository()
	eventRepo := learning.NewMemoryEventRepository()
	learnSvc, err := learning.NewWithEvents(usageRepo, expRepo, eventRepo, attribution.NewDefault(), nil)
	if err != nil {
		t.Fatalf("learning: %v", err)
	}

	vec := []float32{1, 0}
	exp, _ := expSvc.Create(ctx, experience.CreateInput{
		TenantID: "t", Type: experience.TypeProcedural, Scope: experience.ScopeTool, ScopeKey: "jira",
		Trigger: "t", Content: "c", Confidence: 0.9, Embedding: vec,
	})
	_, _ = usageRepo.Create(ctx, experience.Usage{
		ID: "u1", TenantID: "t", EpisodeID: "ep1", ExperienceID: exp.ID, FinalScore: 1,
	})

	before := exp.Utility
	_, err = eventRepo.Create(ctx, learning.Event{
		ID: "ev-reward", TenantID: "t", FeedbackID: "fb-reward", EpisodeID: "ep1",
		ExperienceID: exp.ID, NormalizedReward: -1, Confidence: 1, Credit: 1,
		EffectiveReward: -1, Status: learning.EventFailed,
	})
	if err != nil {
		t.Fatalf("create failed event: %v", err)
	}

	// Caller passes positive reward; retry must still apply event's -1 reward.
	retryUpdates, err := learnSvc.ApplyFeedbackReward(ctx, "t", "ep1", "fb-reward", 1.0, 1.0)
	if err != nil {
		t.Fatalf("retry apply: %v", err)
	}
	if len(retryUpdates) != 1 {
		t.Fatalf("updates: %#v", retryUpdates)
	}
	if !(retryUpdates[0].NewUtility < before) {
		t.Fatalf("utility should drop with event reward -1: before=%v after=%v", before, retryUpdates[0].NewUtility)
	}
}

type failMarkAppliedRepo struct {
	learning.EventRepository
	fail bool
}

func (f *failMarkAppliedRepo) MarkApplied(ctx context.Context, tenantID, id string, appliedAt time.Time) error {
	if f.fail {
		return fmt.Errorf("forced mark applied failure")
	}
	return f.EventRepository.MarkApplied(ctx, tenantID, id, appliedAt)
}

func TestAtomicApplyRollsBackOnMarkAppliedFailure(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	expRepo := experience.NewMemoryRepository()
	expSvc := experience.NewService(expRepo)
	baseEvents := learning.NewMemoryEventRepository()
	eventRepo := &failMarkAppliedRepo{EventRepository: baseEvents, fail: true}
	applier := learning.NewMemoryEventApplier(expRepo, eventRepo)
	learnSvc, err := learning.NewWithEvents(
		experience.NewMemoryUsageRepository(), expRepo, eventRepo, attribution.NewDefault(), applier,
	)
	if err != nil {
		t.Fatalf("learning: %v", err)
	}

	vec := []float32{1}
	exp, _ := expSvc.Create(ctx, experience.CreateInput{
		TenantID: "t", Type: experience.TypeProcedural, Scope: experience.ScopeTool, ScopeKey: "jira",
		Trigger: "t", Content: "c", Confidence: 0.9, Embedding: vec,
	})
	before, _ := expSvc.Get(ctx, "t", exp.ID)

	_, err = eventRepo.Create(ctx, learning.Event{
		ID: "ev-atomic", TenantID: "t", FeedbackID: "fb-atomic", EpisodeID: "ep1",
		ExperienceID: exp.ID, NormalizedReward: 1, Confidence: 1, Credit: 1,
		EffectiveReward: 1, Status: learning.EventFailed,
	})
	if err != nil {
		t.Fatalf("create event: %v", err)
	}

	_, err = learnSvc.ApplyFeedbackReward(ctx, "t", "ep1", "fb-atomic", 1.0, 1.0)
	if err == nil {
		t.Fatal("expected apply to fail when MarkApplied fails")
	}

	after, _ := expSvc.Get(ctx, "t", exp.ID)
	if after.Alpha != before.Alpha || after.Beta != before.Beta || after.Utility != before.Utility {
		t.Fatalf("utility changed despite failed apply: before=%#v after=%#v", before, after)
	}
	ev, _ := baseEvents.GetByFeedbackExperience(ctx, "t", "fb-atomic", exp.ID)
	if ev.Status == learning.EventApplied {
		t.Fatalf("event should not be APPLIED after failure: %#v", ev)
	}
}
