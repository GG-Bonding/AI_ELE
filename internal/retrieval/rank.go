package retrieval

import (
	"math"
	"sort"
	"strings"
	"time"

	"github.com/agent-experience-engine/agent-experience-engine/internal/experience"
)

// RankConfig configures utility-aware ranking (all weights/decay are explainable).
type RankConfig struct {
	// CandidateTopK is Phase-1 semantic candidate set size.
	CandidateTopK int
	// DefaultTopK is the final returned list size when query TopK is unset.
	DefaultTopK int

	// DefaultLambda is freshness decay when type-specific lambda is absent.
	DefaultLambda float64
	// TypeLambda maps experience type -> freshness λ (ageDays).
	TypeLambda map[experience.Type]float64
	// ToolScopeLambda overrides λ when scope is TOOL (typically faster decay).
	ToolScopeLambda float64

	Now func() time.Time
}

// DefaultRankConfig returns V1 defaults.
func DefaultRankConfig() RankConfig {
	return RankConfig{
		CandidateTopK:   20,
		DefaultTopK:     20,
		DefaultLambda:   0.02,
		ToolScopeLambda: 0.05,
		TypeLambda: map[experience.Type]float64{
			experience.TypeSemantic:   0.005, // slower decay
			experience.TypeEpisodic:   0.03,
			experience.TypeProcedural: 0.05, // faster decay
			experience.TypeFailure:    0.04,
			experience.TypeConstraint: 0.01,
			experience.TypePreference: 0.02,
		},
		Now: time.Now().UTC,
	}
}

// ScoreBreakdown makes FinalScore explainable and testable.
type ScoreBreakdown struct {
	Similarity float64 `json:"similarity"`
	Utility    float64 `json:"utility"`
	Confidence float64 `json:"confidence"`
	Freshness  float64 `json:"freshness"`
	ScopeMatch float64 `json:"scope_match"`
	FinalScore float64 `json:"final_score"`
}

// RankedExperience is a Phase-2 ranked candidate.
type RankedExperience struct {
	Experience experience.Experience `json:"experience"`
	Score      ScoreBreakdown        `json:"score"`
}

// ScopeContext soft-matches experience scope to the current task.
type ScopeContext struct {
	AgentID  string
	UserID   string
	ScopeKey string
	Tools    []string
}

// Rank reorders Phase-1 candidates by:
// FinalScore = Similarity × Utility × Confidence × Freshness × ScopeMatch
func Rank(candidates []experience.ScoredExperience, scope ScopeContext, cfg RankConfig, now time.Time) []RankedExperience {
	if cfg.Now != nil && now.IsZero() {
		now = cfg.Now()
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}

	out := make([]RankedExperience, 0, len(candidates))
	for _, c := range candidates {
		bd := scoreOne(c, scope, cfg, now)
		out = append(out, RankedExperience{
			Experience: c.Experience,
			Score:      bd,
		})
	}

	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Score.FinalScore == out[j].Score.FinalScore {
			if out[i].Score.Similarity == out[j].Score.Similarity {
				return out[i].Experience.ID < out[j].Experience.ID
			}
			return out[i].Score.Similarity > out[j].Score.Similarity
		}
		return out[i].Score.FinalScore > out[j].Score.FinalScore
	})
	return out
}

// RankBySimilarity orders candidates by Similarity only (raw retrieval arm).
func RankBySimilarity(candidates []experience.ScoredExperience) []RankedExperience {
	out := make([]RankedExperience, 0, len(candidates))
	for _, c := range candidates {
		sim := clamp01(c.Similarity)
		out = append(out, RankedExperience{
			Experience: c.Experience,
			Score: ScoreBreakdown{
				Similarity: sim,
				Utility:    1,
				Confidence: 1,
				Freshness:  1,
				ScopeMatch: 1,
				FinalScore: sim,
			},
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Score.Similarity == out[j].Score.Similarity {
			return out[i].Experience.ID < out[j].Experience.ID
		}
		return out[i].Score.Similarity > out[j].Score.Similarity
	})
	return out
}

func scoreOne(c experience.ScoredExperience, scope ScopeContext, cfg RankConfig, now time.Time) ScoreBreakdown {
	sim := clamp01(c.Similarity)
	util := clamp01(c.Experience.Utility)
	conf := clamp01(c.Experience.Confidence)
	fresh := Freshness(c.Experience, cfg, now)
	scopeMatch := ScopeMatch(c.Experience, scope)
	final := sim * util * conf * fresh * scopeMatch
	return ScoreBreakdown{
		Similarity: sim,
		Utility:    util,
		Confidence: conf,
		Freshness:  fresh,
		ScopeMatch: scopeMatch,
		FinalScore: final,
	}
}

// Freshness ∈ [0,1] = exp(-λ × ageDays).
// Age is measured from LastUsedAt when set (unused experiences decay), else UpdatedAt/CreatedAt.
func Freshness(exp experience.Experience, cfg RankConfig, now time.Time) float64 {
	ref := exp.UpdatedAt
	if exp.LastUsedAt != nil && !exp.LastUsedAt.IsZero() {
		ref = *exp.LastUsedAt
	} else if ref.IsZero() {
		ref = exp.CreatedAt
	}
	if ref.IsZero() {
		return 1
	}
	ageDays := now.Sub(ref).Hours() / 24
	if ageDays < 0 {
		ageDays = 0
	}
	lambda := cfg.DefaultLambda
	if cfg.TypeLambda != nil {
		if v, ok := cfg.TypeLambda[exp.Type]; ok {
			lambda = v
		}
	}
	if exp.Scope == experience.ScopeTool && cfg.ToolScopeLambda > 0 {
		if cfg.ToolScopeLambda > lambda {
			lambda = cfg.ToolScopeLambda
		}
	}
	if lambda < 0 {
		lambda = 0
	}
	return math.Exp(-lambda * ageDays)
}

// ScopeMatch ∈ [0,1] scores how well experience scope fits the query context.
func ScopeMatch(exp experience.Experience, scope ScopeContext) float64 {
	tools := map[string]struct{}{}
	for _, t := range scope.Tools {
		if t != "" {
			tools[t] = struct{}{}
		}
	}
	if scope.ScopeKey != "" {
		tools[scope.ScopeKey] = struct{}{}
	}

	switch exp.Scope {
	case experience.ScopeTool:
		if len(tools) == 0 {
			return 0.7
		}
		if exp.ScopeKey == "" {
			return 0.5
		}
		if _, ok := tools[exp.ScopeKey]; ok {
			return 1.0
		}
		for t := range tools {
			if strings.HasPrefix(t, exp.ScopeKey+".") || strings.HasPrefix(t, exp.ScopeKey+"/") {
				return 1.0
			}
		}
		return 0.25
	case experience.ScopeAgent:
		if scope.AgentID == "" {
			return 0.7
		}
		if exp.ScopeKey == scope.AgentID {
			return 1.0
		}
		return 0.2
	case experience.ScopeUser:
		if scope.UserID == "" {
			return 0.7
		}
		if exp.ScopeKey == scope.UserID {
			return 1.0
		}
		return 0.2
	case experience.ScopeTenant, experience.ScopeTeam, experience.ScopeTaskType:
		return 0.85
	case experience.ScopeGlobal:
		return 0.7
	default:
		return 0.5
	}
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}
