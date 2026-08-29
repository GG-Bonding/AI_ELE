package eval

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

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

const (
	tenantID = "eval_tenant"
	agentID  = "eval_agent"
	userID   = "eval_user"

	helpfulID = "exp_helpful_jira"
	harmfulID = "exp_harmful_jira"

	helpfulTrigger = "create jira issue when project key unknown"
	helpfulContent = "Resolve project key before create_issue"
	harmfulTrigger = "Create a Jira issue for payment timeout"
	harmfulContent = "Use display name Payment as project key"
)

// Engine wires in-memory Experience Engine components for offline evaluation.
type Engine struct {
	Experiences *experience.Service
	ExpRepo     experience.Repository
	Retriever   *retrieval.Retriever
	Selector    *selector.Selector
	Context     *contextx.Service
	Episodes    *episode.Service
	Feedback    *feedback.Service
	Learning    *learning.Service
	Embedder    *provider.MockEmbedding

	HelpfulID string
	HarmfulID string
}

// NewEngine constructs a deterministic evaluation engine.
func NewEngine() (*Engine, error) {
	expRepo := experience.NewMemoryRepository()
	expSvc := experience.NewService(expRepo)
	usageRepo := experience.NewMemoryUsageRepository()
	learnSvc, err := learning.New(usageRepo, expRepo, attribution.NewDefault())
	if err != nil {
		return nil, err
	}
	embedder := &provider.MockEmbedding{Dim: 32}
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
	epSvc := episode.NewService(episode.NewMemoryRepository())
	fbSvc := feedback.NewServiceWithLearner(
		feedback.NewMemoryRepository(),
		epSvc,
		feedback.NewRewardEngine(nil),
		learning.FeedbackLearner{Inner: learnSvc},
	)
	return &Engine{
		Experiences: expSvc,
		ExpRepo:     expRepo,
		Retriever:   retriever,
		Selector:    selector.New(selector.DefaultConfig()),
		Context:     contextSvc,
		Episodes:    epSvc,
		Feedback:    fbSvc,
		Learning:    learnSvc,
		Embedder:    embedder,
		HelpfulID:   helpfulID,
		HarmfulID:   harmfulID,
	}, nil
}

// SeedJiraStore loads conflicting helpful / harmful experiences.
// Both share the query embedding so Similarity ties; Utility decides utility-aware ranking,
// while raw similarity ranking breaks ties by id (harmful < helpful ⇒ harmful first).
func (e *Engine) SeedJiraStore(ctx context.Context, helpfulUtility, harmfulUtility float64) error {
	task := harmfulTrigger
	vecs, err := e.Embedder.Embed(ctx, []string{task})
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	hAlpha, hBeta := betaParams(helpfulUtility)
	badAlpha, badBeta := betaParams(harmfulUtility)
	helpful := experience.Experience{
		ID: helpfulID, TenantID: tenantID, Type: experience.TypeProcedural,
		Scope: experience.ScopeTool, ScopeKey: "jira",
		Trigger: helpfulTrigger, Content: helpfulContent,
		Confidence: 0.9, Utility: helpfulUtility, Alpha: hAlpha, Beta: hBeta,
		Status: experience.StatusActive, Version: 1, Embedding: append([]float32(nil), vecs[0]...),
		CreatedAt: now, UpdatedAt: now,
	}
	harmful := experience.Experience{
		ID: harmfulID, TenantID: tenantID, Type: experience.TypeProcedural,
		Scope: experience.ScopeTool, ScopeKey: "jira",
		Trigger: harmfulTrigger, Content: harmfulContent,
		Confidence: 0.9, Utility: harmfulUtility, Alpha: badAlpha, Beta: badBeta,
		Status: experience.StatusActive, Version: 1, Embedding: append([]float32(nil), vecs[0]...),
		CreatedAt: now, UpdatedAt: now,
	}
	if _, err := e.ExpRepo.Create(ctx, helpful); err != nil {
		return fmt.Errorf("seed helpful: %w", err)
	}
	if _, err := e.ExpRepo.Create(ctx, harmful); err != nil {
		return fmt.Errorf("seed harmful: %w", err)
	}
	return nil
}

// SeedHelpfulOnly stores the verified procedural tip (learning-loop / cold-start after task 1).
func (e *Engine) SeedHelpfulOnly(ctx context.Context, utility float64) error {
	vecs, err := e.Embedder.Embed(ctx, []string{helpfulTrigger})
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	alpha, beta := betaParams(utility)
	exp := experience.Experience{
		ID: helpfulID, TenantID: tenantID, Type: experience.TypeProcedural,
		Scope: experience.ScopeTool, ScopeKey: "jira",
		Trigger: helpfulTrigger, Content: helpfulContent,
		Confidence: 0.9, Utility: utility, Alpha: alpha, Beta: beta,
		Status: experience.StatusActive, Version: 1, Embedding: vecs[0],
		CreatedAt: now, UpdatedAt: now,
	}
	if _, err := e.ExpRepo.Create(ctx, exp); err != nil {
		return err
	}
	e.HarmfulID = ""
	return nil
}

// JiraTasks is the repeated create-jira workload used across arms.
// Texts are identical so mock embeddings stay on-distribution; TaskIndex still distinguishes runs.
func JiraTasks() []string {
	q := "Create a Jira issue for payment timeout"
	return []string{q, q, q}
}

// RunArm executes the jira task sequence under one experimental arm.
func (e *Engine) RunArm(ctx context.Context, arm Arm) ([]TaskResult, error) {
	tasks := JiraTasks()
	out := make([]TaskResult, 0, len(tasks))
	for i, task := range tasks {
		tr, err := e.runOne(ctx, arm, i, task)
		if err != nil {
			return out, err
		}
		out = append(out, tr)
	}
	return out, nil
}

func (e *Engine) runOne(ctx context.Context, arm Arm, idx int, task string) (TaskResult, error) {
	start := time.Now()
	tr := TaskResult{
		Arm:         arm,
		TaskIndex:   idx,
		Task:        task,
		RelevantIDs: []string{e.HelpfulID},
	}

	ep, err := e.Episodes.CreateEpisode(ctx, episode.CreateEpisodeInput{
		TenantID: tenantID, AgentID: agentID, UserID: userID, Goal: task,
	})
	if err != nil {
		return tr, err
	}

	var kept []string
	var retrieved []string
	tokens := 40 // base prompt budget

	switch arm {
	case ArmBaseline:
		// no retrieval
	case ArmRawRetrieval:
		ranked, err := e.Retriever.RetrieveBySimilarity(ctx, retrieval.Query{
			TenantID: tenantID, Task: task, Tools: []string{"jira"}, TopK: 5,
		})
		if err != nil {
			return tr, err
		}
		selected := e.Selector.Select(task, ranked)
		payload, err := contextx.New(contextx.DefaultConfig()).Build(selected)
		if err != nil {
			return tr, err
		}
		for _, r := range ranked {
			retrieved = append(retrieved, r.Experience.ID)
		}
		for _, item := range payload.Experiences {
			kept = append(kept, item.Content)
			tokens += utf8.RuneCountInString(item.Content)/4 + 8
		}
		if err := e.recordUsages(ctx, ep.ID, selected, payload); err != nil {
			return tr, err
		}
	case ArmUtilityRetrieval, ArmUtilityLearning:
		built, err := e.Context.BuildContext(ctx, contextx.Request{
			TenantID: tenantID, AgentID: agentID, UserID: userID, EpisodeID: ep.ID,
			Task: task, Tools: []string{"jira"}, MaxExperiences: 5,
		})
		if err != nil {
			return tr, err
		}
		for _, sel := range built.Selections {
			retrieved = append(retrieved, sel.Experience.Experience.ID)
		}
		for _, item := range built.Context.Experiences {
			kept = append(kept, item.Content)
			tokens += utf8.RuneCountInString(item.Content)/4 + 8
		}
	default:
		return tr, fmt.Errorf("unknown arm %q", arm)
	}

	tr.RetrievedIDs = retrieved
	tr.KeptContents = kept
	tr.Tokens = tokens
	tr.Latency = time.Since(start)

	decision := decideFromContext(kept)
	tr.Success = decision.success
	tr.UsedExperience = decision.usedHelpful
	tr.NegativeTransfer = decision.negativeTransfer

	status := episode.StatusFailed
	if tr.Success {
		status = episode.StatusSuccess
	}
	if _, _, err := e.Episodes.CompleteEpisode(ctx, episode.CompleteEpisodeInput{
		TenantID: tenantID, EpisodeID: ep.ID, Status: status,
	}); err != nil {
		return tr, err
	}

	if arm == ArmUtilityLearning && len(kept) > 0 {
		reward := 1.0
		if !tr.Success {
			reward = -1.0
		}
		if _, err := e.Feedback.Submit(ctx, feedback.SubmitInput{
			TenantID: tenantID, EpisodeID: ep.ID, Source: "business", Reward: &reward, Confidence: 1,
		}); err != nil {
			return tr, err
		}
	}

	if e.HelpfulID != "" {
		if h, err := e.Experiences.Get(ctx, tenantID, e.HelpfulID); err == nil {
			tr.HelpfulUtility = h.Utility
		}
		if scores, err := e.Retriever.Retrieve(ctx, retrieval.Query{
			TenantID: tenantID, Task: task, Tools: []string{"jira"}, TopK: 5,
		}); err == nil {
			for _, s := range scores {
				if s.Experience.ID == e.HelpfulID {
					tr.HelpfulFinalScore = s.Score.FinalScore
				}
			}
		}
	}
	if e.HarmfulID != "" {
		if h, err := e.Experiences.Get(ctx, tenantID, e.HarmfulID); err == nil {
			tr.HarmfulUtility = h.Utility
		}
	}
	return tr, nil
}

type agentDecision struct {
	success          bool
	usedHelpful      bool
	negativeTransfer bool
}

func decideFromContext(contents []string) agentDecision {
	joined := strings.ToLower(strings.Join(contents, "\n"))
	hasHelpful := strings.Contains(joined, "resolve project key") || strings.Contains(joined, "search project")
	hasHarmful := strings.Contains(joined, "display name") || strings.Contains(joined, "payment as project")
	switch {
	case hasHarmful && !hasHelpful:
		return agentDecision{success: false, negativeTransfer: true}
	case hasHarmful && hasHelpful:
		first := ""
		if len(contents) > 0 {
			first = strings.ToLower(contents[0])
		}
		if strings.Contains(first, "display name") || strings.Contains(first, "payment as project") {
			return agentDecision{success: false, negativeTransfer: true}
		}
		return agentDecision{success: true, usedHelpful: true}
	case hasHelpful:
		return agentDecision{success: true, usedHelpful: true}
	default:
		return agentDecision{success: false}
	}
}

func (e *Engine) recordUsages(ctx context.Context, episodeID string, selected []selector.Result, payload contextx.Payload) error {
	ids := make([]string, 0, len(payload.Experiences))
	for _, item := range payload.Experiences {
		ids = append(ids, item.Source)
	}
	_, err := e.Learning.RecordUsages(ctx, learning.RecordInput{
		TenantID: tenantID, EpisodeID: episodeID, Selections: selected, ContextIDs: ids,
	})
	return err
}

// CompareArms runs all four arms with arm-specific store priors and returns metrics.
func CompareArms(ctx context.Context) (map[Arm]Metrics, error) {
	out := make(map[Arm]Metrics, len(AllArms()))
	for _, arm := range AllArms() {
		engine, err := NewEngine()
		if err != nil {
			return nil, err
		}
		switch arm {
		case ArmBaseline:
			// empty store
		case ArmRawRetrieval:
			// equal utility → similarity-tie prefers harmful id
			if err := engine.SeedJiraStore(ctx, 0.5, 0.5); err != nil {
				return nil, err
			}
		case ArmUtilityRetrieval:
			if err := engine.SeedJiraStore(ctx, 0.9, 0.2); err != nil {
				return nil, err
			}
		case ArmUtilityLearning:
			// helpful slightly ahead; learning widens the gap after successes
			if err := engine.SeedJiraStore(ctx, 0.6, 0.5); err != nil {
				return nil, err
			}
		}
		results, err := engine.RunArm(ctx, arm)
		if err != nil {
			return nil, fmt.Errorf("arm %s: %w", arm, err)
		}
		out[arm] = Aggregate(arm, results)
	}
	return out, nil
}

// betaParams picks α,β such that α/(α+β) ≈ utility (α,β ≥ 1).
func betaParams(utility float64) (alpha, beta float64) {
	if utility <= 0 {
		return 1, 9
	}
	if utility >= 1 {
		return 9, 1
	}
	if utility >= 0.5 {
		return utility / (1 - utility), 1
	}
	return 1, (1 - utility) / utility
}
