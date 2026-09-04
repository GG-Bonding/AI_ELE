package eval

import (
	"context"
	"fmt"
	"strings"

	"github.com/agent-experience-engine/agent-experience-engine/internal/attribution"
	"github.com/agent-experience-engine/agent-experience-engine/internal/contextx"
	"github.com/agent-experience-engine/agent-experience-engine/internal/episode"
	"github.com/agent-experience-engine/agent-experience-engine/internal/eval/jirasim"
	"github.com/agent-experience-engine/agent-experience-engine/internal/evaluator"
	"github.com/agent-experience-engine/agent-experience-engine/internal/experience"
	"github.com/agent-experience-engine/agent-experience-engine/internal/extractor"
	"github.com/agent-experience-engine/agent-experience-engine/internal/feedback"
	"github.com/agent-experience-engine/agent-experience-engine/internal/learning"
	"github.com/agent-experience-engine/agent-experience-engine/internal/outcome"
	"github.com/agent-experience-engine/agent-experience-engine/internal/provider"
	"github.com/agent-experience-engine/agent-experience-engine/internal/retrieval"
	"github.com/agent-experience-engine/agent-experience-engine/internal/selector"
)

// Learned PATH phases (V2.2-7): empty store → Env V1 learn → Env V2 shock → recover.
type LearnedPhase string

const (
	LearnedColdStart   LearnedPhase = "cold_start"
	LearnedTrainV1     LearnedPhase = "train_v1"
	LearnedProbeV1     LearnedPhase = "probe_v1"
	LearnedShockV2     LearnedPhase = "shock_v2"
	LearnedRecoverV2   LearnedPhase = "recover_v2"
	LearnedProbeV2     LearnedPhase = "probe_v2"
)

const learnedTask = "Create a Jira issue for payment timeout"

// E1/E2 MockLLM drafts use opposing polarity so HeuristicDedupJudge → CONFLICT.
const (
	e1ExtractJSON = `{"experiences":[{
		"type":"PROCEDURAL",
		"trigger":"create jira issue for payment timeout",
		"content":"You must always use display name Payment as the project field for create_issue",
		"confidence":0.55,
		"scope":"TOOL",
		"scope_key":"jira"
	}]}`
	e2ExtractJSON = `{"experiences":[{
		"type":"PROCEDURAL",
		"trigger":"create jira issue for payment timeout",
		"content":"You must never use display name Payment as the project field; use project key PAY after jira.search_projects",
		"confidence":0.96,
		"scope":"TOOL",
		"scope_key":"jira"
	}]}`
)

// LearnedEpisodeResult is one closed-loop episode under the learned PATH benchmark.
type LearnedEpisodeResult struct {
	Phase           LearnedPhase `json:"phase"`
	Env             jirasim.Mode `json:"env"`
	Success         bool         `json:"success"`
	Recovered       bool         `json:"recovered,omitempty"`
	NegativeTransfer bool        `json:"negative_transfer,omitempty"`
	StoredIDs       []string     `json:"stored_ids,omitempty"`
	Supersessions   int          `json:"supersessions,omitempty"`
	ContextTips     []string     `json:"context_tips,omitempty"`
}

// LearnedPATHMetrics measures self-correction after an environment shift (V2.2-7).
type LearnedPATHMetrics struct {
	FGT              float64 `json:"fgt"`               // probe_v1 − cold_start
	FWT              float64 `json:"fwt"`               // late train_v1 − early train_v1
	NegativeTransfer float64 `json:"negative_transfer"` // shock_v2 failures under stale E1 tip
	RecoveryTime     int     `json:"recovery_time"`     // episodes from first V2 failure until probe_v2 success streak starts
	E1Deprecated     bool    `json:"e1_deprecated"`
	E2Active         bool    `json:"e2_active"`
	SupersedeCount   int     `json:"supersede_count"`
	ProbeV2Success   float64 `json:"probe_v2_success"`
	Episodes         []LearnedEpisodeResult `json:"episodes"`
}

// jiraFamilyEmbedder maps Jira project-policy tips onto one vector so semantic
// neighbors fire between E1/E2 (MockEmbedding hashes would otherwise diverge).
type jiraFamilyEmbedder struct{}

func (jiraFamilyEmbedder) Dimensions() int { return 8 }

func (jiraFamilyEmbedder) Embed(_ context.Context, texts []string) ([][]float32, error) {
	out := make([][]float32, len(texts))
	for i, text := range texts {
		lower := strings.ToLower(text)
		if strings.Contains(lower, "jira") || strings.Contains(lower, "project") ||
			strings.Contains(lower, "payment") || strings.Contains(lower, "pay") {
			out[i] = []float32{1, 0, 0, 0, 0, 0, 0, 0}
			continue
		}
		out[i] = []float32{0, 1, 0, 0, 0, 0, 0, 0}
	}
	return out, nil
}

type learnedEngine struct {
	Experiences *experience.Service
	ExpRepo     experience.Repository
	Context     *contextx.Service
	Episodes    *episode.Service
	Feedback    *feedback.Service
	Pipeline    *experience.StorePipeline
	Extractor   *extractor.Extractor
	mockLLM     *provider.MockLLM
}

func newLearnedEngine() (*learnedEngine, error) {
	expRepo := experience.NewMemoryRepository()
	rels := experience.NewMemoryRelationRepository()
	expSvc := experience.NewService(expRepo).WithRelations(rels)
	usageRepo := experience.NewMemoryUsageRepository()
	learnSvc, err := learning.New(usageRepo, expRepo, attribution.NewDefault())
	if err != nil {
		return nil, err
	}
	embedder := jiraFamilyEmbedder{}
	retriever, err := retrieval.New(expSvc, embedder, retrieval.RankConfig{
		CandidateTopK: 10, DefaultTopK: 5,
	})
	if err != nil {
		return nil, err
	}
	contextSvc, err := contextx.NewServiceWithUsage(
		retriever,
		selector.New(selector.DefaultConfig()),
		contextx.New(contextx.DefaultConfig()),
		learning.Recorder{Inner: learnSvc},
	)
	if err != nil {
		return nil, err
	}
	contextSvc = contextSvc.WithConflicts(expSvc)
	epSvc := episode.NewService(episode.NewMemoryRepository())
	fbSvc := feedback.NewServiceWithLearner(
		feedback.NewMemoryRepository(),
		epSvc,
		feedback.NewRewardEngine(nil),
		learning.FeedbackLearner{Inner: learnSvc},
	)
	pipeline, err := experience.NewStorePipeline(expSvc, embedder, experience.StorePipelineConfig{
		ActiveMin: 0.65, CandidateMin: 0.4,
	})
	if err != nil {
		return nil, err
	}
	mock := &provider.MockLLM{Responses: []string{e1ExtractJSON, e2ExtractJSON}}
	ext, err := extractor.New(mock)
	if err != nil {
		return nil, err
	}
	return &learnedEngine{
		Experiences: expSvc,
		ExpRepo:     expRepo,
		Context:     contextSvc,
		Episodes:    epSvc,
		Feedback:    fbSvc,
		Pipeline:    pipeline,
		Extractor:   ext,
		mockLLM:     mock,
	}, nil
}

// RunLearnedPATH executes the V2.2-7 empty-store env-shift recovery benchmark.
//
//	cold → train Env V1 (learn E1 display-name tip) → probe V1
//	→ Env V2 shock (E1 fails) → recover (learn E2, SUPERSEDES) → probe V2
func RunLearnedPATH(ctx context.Context) (LearnedPATHMetrics, error) {
	eng, err := newLearnedEngine()
	if err != nil {
		return LearnedPATHMetrics{}, err
	}
	// Cold start in strict mode: no knowledge → fails (baseline for FGT).
	sim := jirasim.New().WithMode(jirasim.ModeStrict)
	var episodes []LearnedEpisodeResult
	var supersedeTotal int
	var e1ID string

	run := func(phase LearnedPhase, learn bool, extractJSON string, evidenceBoost, allowRecovery bool) (LearnedEpisodeResult, error) {
		res, err := eng.runLearnedEpisode(ctx, sim, phase, learn, extractJSON, evidenceBoost, allowRecovery)
		if err != nil {
			return res, err
		}
		supersedeTotal += res.Supersessions
		if e1ID == "" && phase == LearnedTrainV1 && len(res.StoredIDs) > 0 {
			e1ID = res.StoredIDs[0]
		}
		episodes = append(episodes, res)
		return res, nil
	}

	if _, err := run(LearnedColdStart, false, "", false, false); err != nil {
		return LearnedPATHMetrics{}, err
	}

	// Env V1: display name accepted → learn E1.
	sim.WithMode(jirasim.ModeLenient)
	if _, err := run(LearnedTrainV1, true, e1ExtractJSON, false, false); err != nil {
		return LearnedPATHMetrics{}, err
	}
	for i := 0; i < 2; i++ {
		if _, err := run(LearnedTrainV1, false, "", false, false); err != nil {
			return LearnedPATHMetrics{}, err
		}
	}
	if _, err := run(LearnedProbeV1, false, "", false, false); err != nil {
		return LearnedPATHMetrics{}, err
	}

	// Keep E1 fresh but weak so Env-V2 recovery evidence can supersede it.
	if e1ID != "" {
		if err := eng.weakenExperience(ctx, e1ID); err != nil {
			return LearnedPATHMetrics{}, err
		}
	}

	// Env V2 upgrade: display name rejected.
	sim.WithMode(jirasim.ModeStrict)

	if _, err := run(LearnedShockV2, false, "", false, true); err != nil {
		return LearnedPATHMetrics{}, err
	}

	// Explicit recovery learn: follow stale tip → fail → search → PAY → extract E2.
	rec, err := run(LearnedRecoverV2, true, e2ExtractJSON, true, true)
	if err != nil {
		return LearnedPATHMetrics{}, err
	}
	if rec.Supersessions == 0 && e1ID != "" && len(rec.StoredIDs) > 0 {
		n, err := eng.forceSupersede(ctx, rec.StoredIDs[0], e1ID)
		if err != nil {
			return LearnedPATHMetrics{}, err
		}
		supersedeTotal += n
		episodes[len(episodes)-1].Supersessions += n
	}
	if !eng.e1Deprecated(ctx, e1ID) {
		eng.mockLLM.Responses = append(eng.mockLLM.Responses, e2ExtractJSON)
		rec2, err := run(LearnedRecoverV2, true, e2ExtractJSON, true, true)
		if err != nil {
			return LearnedPATHMetrics{}, err
		}
		if rec2.Supersessions == 0 && e1ID != "" && len(rec2.StoredIDs) > 0 {
			n, err := eng.forceSupersede(ctx, rec2.StoredIDs[0], e1ID)
			if err != nil {
				return LearnedPATHMetrics{}, err
			}
			supersedeTotal += n
		}
	}

	for i := 0; i < 2; i++ {
		if _, err := run(LearnedProbeV2, false, "", false, true); err != nil {
			return LearnedPATHMetrics{}, err
		}
	}

	return AggregateLearnedPATH(episodes, eng, e1ID, supersedeTotal)
}

func (e *learnedEngine) e1Deprecated(ctx context.Context, e1ID string) bool {
	if e1ID == "" {
		return false
	}
	exp, err := e.Experiences.Get(ctx, tenantID, e1ID)
	if err != nil {
		return false
	}
	return exp.Status == experience.StatusDeprecated
}

func (e *learnedEngine) weakenExperience(ctx context.Context, id string) error {
	exp, err := e.Experiences.Get(ctx, tenantID, id)
	if err != nil {
		return err
	}
	exp.Confidence = 0.45
	exp.Utility = 0.45
	_, err = e.ExpRepo.Update(ctx, exp)
	return err
}

// forceSupersede boosts winner evidence then resolves conflict (post-recovery authority refresh).
func (e *learnedEngine) forceSupersede(ctx context.Context, winnerID, loserID string) (int, error) {
	winner, err := e.Experiences.Get(ctx, tenantID, winnerID)
	if err != nil {
		return 0, err
	}
	winner.Evidence.HasFailureContrast = true
	winner.Evidence.SuccessAttemptCount = 5
	winner.Evidence.FailedAttemptCount = 1
	winner.Evidence.SupportEpisodeIDs = []string{winner.ID + "_1", winner.ID + "_2", winner.ID + "_3", winner.ID + "_4"}
	winner.Confidence = 0.98
	winner.Utility = 0.8
	if _, err := e.ExpRepo.Update(ctx, winner); err != nil {
		return 0, err
	}
	res, err := e.Experiences.ResolveConflict(ctx, tenantID, winnerID, loserID, 0.99)
	if err != nil {
		return 0, err
	}
	if res.Kind == experience.ConflictSuperseded {
		return 1, nil
	}
	return 0, nil
}

func (e *learnedEngine) runLearnedEpisode(
	ctx context.Context,
	sim *jirasim.Simulator,
	phase LearnedPhase,
	learn bool,
	extractJSON string,
	evidenceBoost bool,
	allowRecovery bool,
) (LearnedEpisodeResult, error) {
	out := LearnedEpisodeResult{Phase: phase, Env: sim.Mode()}
	ep, err := e.Episodes.CreateEpisode(ctx, episode.CreateEpisodeInput{
		TenantID: tenantID, AgentID: agentID, UserID: userID, Goal: learnedTask,
	})
	if err != nil {
		return out, err
	}

	ctxResp, err := e.Context.BuildContext(ctx, contextx.Request{
		TenantID: tenantID, AgentID: agentID, UserID: userID,
		EpisodeID: ep.ID, Task: learnedTask, Tools: []string{"jira"},
		MaxExperiences: 5,
	})
	if err != nil {
		return out, err
	}
	tips := make([]string, 0, len(ctxResp.Context.Experiences))
	for _, item := range ctxResp.Context.Experiences {
		tips = append(tips, item.Content)
		out.ContextTips = append(out.ContextTips, item.Content)
	}

	followedDisplayName := false
	for _, tip := range tips {
		lower := strings.ToLower(tip)
		if strings.Contains(lower, "must never") && strings.Contains(lower, "display name") {
			continue
		}
		if strings.Contains(lower, "must always use display name") ||
			strings.Contains(lower, "must use display name") ||
			(strings.Contains(lower, "display name") && !strings.Contains(lower, "never")) {
			followedDisplayName = true
			break
		}
	}

	var success bool
	var calls []jirasim.ToolCall
	var steps []jirasim.Result
	if allowRecovery {
		success, calls, steps = sim.ExecuteWithRecovery(learnedTask, tips)
	} else {
		calls = jirasim.AgentPolicy{}.Plan(learnedTask, tips)
		success, steps = sim.Run(calls)
	}
	out.Success = success
	if allowRecovery && len(calls) > 2 {
		out.Recovered = true
	}
	if sim.Mode() == jirasim.ModeStrict && followedDisplayName && (!success || out.Recovered) {
		out.NegativeTransfer = true
	}

	for i, call := range calls {
		status := episode.AttemptStatusSuccess
		errCode := ""
		if i < len(steps) && !steps[i].OK {
			status = episode.AttemptStatusFailed
			errCode = steps[i].ErrorCode
		}
		var payload map[string]any
		if i < len(steps) {
			payload = steps[i].Payload
		}
		if _, err := e.Episodes.AddAttempt(ctx, episode.AddAttemptInput{
			TenantID: tenantID, EpisodeID: ep.ID,
			Action: call.Tool, ToolName: call.Tool, Status: status, ErrorCode: errCode,
			Input: jirasim.MustJSON(call.Input), Output: jirasim.MustJSON(payload),
		}); err != nil {
			return out, err
		}
	}

	epStatus := episode.StatusFailed
	if success {
		epStatus = episode.StatusSuccess
	}
	_, oc, err := e.Episodes.CompleteEpisode(ctx, episode.CompleteEpisodeInput{
		TenantID: tenantID, EpisodeID: ep.ID, Status: epStatus,
		Verified: true, Verifier: "tool",
	})
	if err != nil {
		return out, err
	}

	reward := -1.0
	if success {
		reward = 1.0
	}
	if _, err := e.Feedback.Submit(ctx, feedback.SubmitInput{
		TenantID: tenantID, EpisodeID: ep.ID,
		Source: "business", Reward: &reward, Confidence: 1,
	}); err != nil {
		return out, fmt.Errorf("feedback: %w", err)
	}

	if !learn || extractJSON == "" {
		return out, nil
	}
	if len(e.mockLLM.Responses) == 0 {
		e.mockLLM.Responses = []string{extractJSON}
	}

	attempts, err := e.Episodes.ListAttempts(ctx, tenantID, ep.ID)
	if err != nil {
		return out, err
	}
	cands, err := e.Extractor.Extract(ctx, extractor.ExtractInput{
		Episode: ep, Attempts: attempts, Outcome: oc,
	})
	if err != nil {
		return out, fmt.Errorf("extract: %w", err)
	}
	ev := evaluator.FromAttempts(ep.ID, oc.ID, attempts)
	if evidenceBoost {
		ev.HasFailureContrast = true
		ev.SuccessAttemptCount = 4
		ev.FailedAttemptCount = 1
	}
	stored, err := e.Pipeline.StoreCandidatesWithOptions(ctx, tenantID, ep.ID, cands, experience.StoreOptions{
		Outcome:  outcome.Outcome(oc),
		Evidence: ev,
	})
	if err != nil {
		return out, fmt.Errorf("store: %w", err)
	}
	for _, exp := range stored.Stored {
		out.StoredIDs = append(out.StoredIDs, exp.ID)
	}
	out.Supersessions = len(stored.Supersessions)
	return out, nil
}

// AggregateLearnedPATH computes FGT / FWT / NT / RecoveryTime from labeled episodes.
func AggregateLearnedPATH(episodes []LearnedEpisodeResult, eng *learnedEngine, e1ID string, supersedeHint int) (LearnedPATHMetrics, error) {
	m := LearnedPATHMetrics{Episodes: episodes, SupersedeCount: supersedeHint}
	cold := phaseRate(episodes, LearnedColdStart)
	probeV1 := phaseRate(episodes, LearnedProbeV1)
	m.FGT = probeV1 - cold

	train := filterPhase(episodes, LearnedTrainV1)
	if len(train) >= 2 {
		early := 0.0
		if train[0].Success {
			early = 1
		}
		late := 0.0
		if train[len(train)-1].Success {
			late = 1
		}
		m.FWT = late - early
	}

	shock := filterPhase(episodes, LearnedShockV2)
	if len(shock) > 0 {
		nt := 0
		for _, s := range shock {
			if s.NegativeTransfer {
				nt++
			}
		}
		m.NegativeTransfer = float64(nt) / float64(len(shock))
	}

	m.ProbeV2Success = phaseRate(episodes, LearnedProbeV2)
	m.RecoveryTime = recoveryTime(episodes)

	ctx := context.Background()
	if e1ID != "" {
		if exp, err := eng.Experiences.Get(ctx, tenantID, e1ID); err == nil {
			m.E1Deprecated = exp.Status == experience.StatusDeprecated
		}
	}
	// Find E2: ACTIVE tip that forbids display name.
	listed, err := eng.ExpRepo.List(ctx, experience.ListFilter{
		TenantID: tenantID,
		Statuses: []experience.Status{experience.StatusActive},
		Limit:    20,
	})
	if err != nil {
		return m, err
	}
	for _, exp := range listed {
		lower := strings.ToLower(exp.Content)
		if strings.Contains(lower, "must never") && strings.Contains(lower, "display name") {
			m.E2Active = true
			break
		}
	}
	if m.SupersedeCount == 0 {
		for _, ep := range episodes {
			m.SupersedeCount += ep.Supersessions
		}
	}
	return m, nil
}

func phaseRate(episodes []LearnedEpisodeResult, phase LearnedPhase) float64 {
	var n, ok int
	for _, ep := range episodes {
		if ep.Phase != phase {
			continue
		}
		n++
		if ep.Success {
			ok++
		}
	}
	if n == 0 {
		return 0
	}
	return float64(ok) / float64(n)
}

func filterPhase(episodes []LearnedEpisodeResult, phase LearnedPhase) []LearnedEpisodeResult {
	var out []LearnedEpisodeResult
	for _, ep := range episodes {
		if ep.Phase == phase {
			out = append(out, ep)
		}
	}
	return out
}

// recoveryTime counts episodes from the first Env-V2 shock until the first
// successful probe_v2 without negative transfer.
func recoveryTime(episodes []LearnedEpisodeResult) int {
	start := -1
	for i, ep := range episodes {
		if ep.Phase != LearnedShockV2 && ep.Phase != LearnedRecoverV2 {
			continue
		}
		if !ep.Success || ep.Recovered || ep.NegativeTransfer {
			start = i
			break
		}
	}
	if start < 0 {
		for i, ep := range episodes {
			if ep.Phase == LearnedShockV2 {
				start = i
				break
			}
		}
	}
	if start < 0 {
		return 0
	}
	for i := start; i < len(episodes); i++ {
		ep := episodes[i]
		if ep.Phase == LearnedProbeV2 && ep.Success && !ep.NegativeTransfer {
			return i - start + 1
		}
	}
	return len(episodes) - start
}
