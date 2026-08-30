package eval

import (
	"context"
	"fmt"
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

// V2 benchmark arms (V2-10).
const (
	// ArmV1Utility is utility-aware retrieval + learning without conflict intelligence.
	ArmV1Utility Arm = "v1_utility"
	// ArmV2Intelligence enables relations, conflict blocking, and authority supersession.
	ArmV2Intelligence Arm = "v2_intelligence"
)

// SequentialPhase is one segment of a PATH-like probe/train schedule.
type SequentialPhase string

const (
	PhaseProbe1         SequentialPhase = "probe_1"
	PhaseTrainPositive  SequentialPhase = "train_positive"
	PhaseProbe2         SequentialPhase = "probe_2" // forward transfer check
	PhaseTrainNegative  SequentialPhase = "train_negative"
	PhaseProbe3         SequentialPhase = "probe_3" // post-conflict resilience
)

// SequentialSchedule returns the PATH-inspired task schedule.
// Probe → positive train → probe → negative pressure → probe.
func SequentialSchedule() []SequentialPhase {
	var out []SequentialPhase
	appendN := func(p SequentialPhase, n int) {
		for i := 0; i < n; i++ {
			out = append(out, p)
		}
	}
	appendN(PhaseProbe1, 2)
	appendN(PhaseTrainPositive, 4)
	appendN(PhaseProbe2, 2)
	appendN(PhaseTrainNegative, 4)
	appendN(PhaseProbe3, 2)
	return out
}

// SequentialTaskResult extends TaskResult with PATH phase labels.
type SequentialTaskResult struct {
	TaskResult
	Phase SequentialPhase `json:"phase"`
}

// SequentialMetrics aggregates PATH-like sequential learning metrics.
type SequentialMetrics struct {
	Arm                  Arm     `json:"arm"`
	Overall              Metrics `json:"overall"`
	Probe1SuccessRate    float64 `json:"probe_1_success_rate"`
	Probe2SuccessRate    float64 `json:"probe_2_success_rate"`
	Probe3SuccessRate    float64 `json:"probe_3_success_rate"`
	ForwardTransferGain  float64 `json:"forward_transfer_gain"`  // probe2 - probe1
	PostConflictSuccess  float64 `json:"post_conflict_success"`  // probe3
	NegativeTransferRate float64 `json:"negative_transfer_rate"`
	TaskSuccessRate      float64 `json:"task_success_rate"`
}

// NewEngineV1 constructs the V1 utility-learning evaluation stack (no conflict graph).
func NewEngineV1() (*Engine, error) {
	return NewEngine()
}

// NewEngineV2 constructs the V2 intelligence stack: relations + conflict-aware context.
func NewEngineV2() (*Engine, error) {
	expRepo := experience.NewMemoryRepository()
	rels := experience.NewMemoryRelationRepository()
	expSvc := experience.NewService(expRepo).WithRelations(rels)
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
	contextSvc = contextSvc.WithConflicts(expSvc)
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

// SeedStaleHarmfulDominant seeds a high-utility harmful tip and a lower-utility but
// higher-authority helpful tip (more evidence). Both are equally fresh so V1 utility
// ranking prefers harmful; V2 authority supersession prefers helpful.
func (e *Engine) SeedStaleHarmfulDominant(ctx context.Context) error {
	task := harmfulTrigger
	vecs, err := e.Embedder.Embed(ctx, []string{task})
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	hAlpha, hBeta := betaParams(0.55)
	badAlpha, badBeta := betaParams(0.85)

	helpful := experience.Experience{
		ID: helpfulID, TenantID: tenantID, Type: experience.TypeProcedural,
		Scope: experience.ScopeTool, ScopeKey: "jira",
		Trigger: helpfulTrigger, Content: helpfulContent,
		Confidence: 0.95, Utility: 0.55, Alpha: hAlpha, Beta: hBeta,
		Status: experience.StatusActive, Version: 1, Embedding: append([]float32(nil), vecs[0]...),
		Evidence: experience.Evidence{
			SourceEpisodeID:     "ep_h1",
			SupportEpisodeIDs:   []string{"ep_h1", "ep_h2", "ep_h3", "ep_h4"},
			SuccessAttemptCount: 4, HasFailureContrast: true,
		},
		CreatedAt: now, UpdatedAt: now,
	}
	harmful := experience.Experience{
		ID: harmfulID, TenantID: tenantID, Type: experience.TypeProcedural,
		Scope: experience.ScopeTool, ScopeKey: "jira",
		Trigger: harmfulTrigger, Content: harmfulContent,
		Confidence: 0.9, Utility: 0.85, Alpha: badAlpha, Beta: badBeta,
		Status: experience.StatusActive, Version: 1, Embedding: append([]float32(nil), vecs[0]...),
		Evidence: experience.Evidence{
			SourceEpisodeID: "ep_bad", SupportEpisodeIDs: []string{"ep_bad"},
			SuccessAttemptCount: 1,
		},
		// Same freshness as helpful so Utility (not Validity) dominates V1 ranking.
		CreatedAt: now.Add(-200 * 24 * time.Hour),
		UpdatedAt: now,
	}
	if _, err := e.ExpRepo.Create(ctx, helpful); err != nil {
		return fmt.Errorf("seed helpful: %w", err)
	}
	if _, err := e.ExpRepo.Create(ctx, harmful); err != nil {
		return fmt.Errorf("seed harmful: %w", err)
	}
	return nil
}

// ApplyV2Supersession resolves the seeded conflict so the helpful tip SUPERSEDES the harmful one.
func (e *Engine) ApplyV2Supersession(ctx context.Context) (experience.ConflictResolution, error) {
	return e.Experiences.ResolveConflict(ctx, tenantID, e.HelpfulID, e.HarmfulID, 0.99)
}

// RunSequentialArm executes the PATH-like schedule under utility_learning behavior.
func (e *Engine) RunSequentialArm(ctx context.Context, arm Arm) ([]SequentialTaskResult, error) {
	schedule := SequentialSchedule()
	out := make([]SequentialTaskResult, 0, len(schedule))
	task := "Create a Jira issue for payment timeout"
	for i, phase := range schedule {
		learn := phase == PhaseTrainPositive || phase == PhaseTrainNegative
		runArm := ArmUtilityRetrieval
		if learn {
			runArm = ArmUtilityLearning
		}
		tr, err := e.runOne(ctx, runArm, i, task)
		if err != nil {
			return out, err
		}
		tr.Arm = arm
		out = append(out, SequentialTaskResult{TaskResult: tr, Phase: phase})
	}
	return out, nil
}

// AggregateSequential computes PATH-style metrics from labeled task results.
func AggregateSequential(arm Arm, results []SequentialTaskResult) SequentialMetrics {
	plain := make([]TaskResult, len(results))
	for i, r := range results {
		plain[i] = r.TaskResult
	}
	overall := Aggregate(arm, plain)
	m := SequentialMetrics{
		Arm:                  arm,
		Overall:              overall,
		NegativeTransferRate: overall.NegativeTransferRate,
		TaskSuccessRate:      overall.TaskSuccessRate,
		PostConflictSuccess:  phaseSuccess(results, PhaseProbe3),
	}
	m.Probe1SuccessRate = phaseSuccess(results, PhaseProbe1)
	m.Probe2SuccessRate = phaseSuccess(results, PhaseProbe2)
	m.Probe3SuccessRate = phaseSuccess(results, PhaseProbe3)
	m.ForwardTransferGain = m.Probe2SuccessRate - m.Probe1SuccessRate
	return m
}

func phaseSuccess(results []SequentialTaskResult, phase SequentialPhase) float64 {
	var n, ok int
	for _, r := range results {
		if r.Phase != phase {
			continue
		}
		n++
		if r.Success {
			ok++
		}
	}
	if n == 0 {
		return 0
	}
	return float64(ok) / float64(n)
}

// CompareSequentialV1V2 runs the V2-10 benchmark: V1 utility-only vs V2 intelligence.
//
// Expected shape (PATH-like DoD):
//
//	SuccessRate_V2 > SuccessRate_V1
//	NegativeTransfer_V2 < NegativeTransfer_V1
func CompareSequentialV1V2(ctx context.Context) (map[Arm]SequentialMetrics, error) {
	out := make(map[Arm]SequentialMetrics, 2)

	// --- V1: popular harmful tip dominates utility ranking ---
	v1, err := NewEngineV1()
	if err != nil {
		return nil, err
	}
	if err := v1.SeedStaleHarmfulDominant(ctx); err != nil {
		return nil, fmt.Errorf("v1 seed: %w", err)
	}
	v1Results, err := v1.RunSequentialArm(ctx, ArmV1Utility)
	if err != nil {
		return nil, fmt.Errorf("v1 run: %w", err)
	}
	out[ArmV1Utility] = AggregateSequential(ArmV1Utility, v1Results)

	// --- V2: authority supersession removes harmful tip before sequential probes ---
	v2, err := NewEngineV2()
	if err != nil {
		return nil, err
	}
	if err := v2.SeedStaleHarmfulDominant(ctx); err != nil {
		return nil, fmt.Errorf("v2 seed: %w", err)
	}
	res, err := v2.ApplyV2Supersession(ctx)
	if err != nil {
		return nil, fmt.Errorf("v2 supersede: %w", err)
	}
	if res.Kind != experience.ConflictSuperseded {
		return nil, fmt.Errorf("v2 expected SUPERSEDED, got %s (authW=%.3f authL=%.3f)",
			res.Kind, res.WinnerAuthority, res.LoserAuthority)
	}
	v2Results, err := v2.RunSequentialArm(ctx, ArmV2Intelligence)
	if err != nil {
		return nil, fmt.Errorf("v2 run: %w", err)
	}
	out[ArmV2Intelligence] = AggregateSequential(ArmV2Intelligence, v2Results)
	return out, nil
}
