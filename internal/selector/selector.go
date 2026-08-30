package selector

import (
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/agent-experience-engine/agent-experience-engine/internal/experience"
	"github.com/agent-experience-engine/agent-experience-engine/internal/retrieval"
)

// Decision is the selector outcome for one ranked experience.
type Decision string

const (
	DecisionKeep     Decision = "KEEP"
	DecisionCompress Decision = "COMPRESS" // V1: strip ids + truncate (not LLM abstraction)
	DecisionIgnore   Decision = "IGNORE"
	DecisionBlock    Decision = "BLOCK"

	// DecisionAbstract is a deprecated alias kept for reading old usage rows.
	DecisionAbstract Decision = DecisionCompress
)

// Config holds explainable thresholds for V1 rule-based selection.
type Config struct {
	BlockUtilityMax     float64
	BlockConfidenceMax  float64
	IgnoreFinalScoreMax float64
	IgnoreScopeMatchMax float64
	KeepFinalScoreMin   float64
	CompressMaxChars    int
}

// DefaultConfig returns V1 defaults.
func DefaultConfig() Config {
	return Config{
		BlockUtilityMax:     0.15,
		BlockConfidenceMax:  0.25,
		IgnoreFinalScoreMax: 0.05,
		IgnoreScopeMatchMax: 0.3,
		KeepFinalScoreMin:   0.08,
		CompressMaxChars:    180,
	}
}

// Result is one selection decision with the content that may enter context.
type Result struct {
	Experience retrieval.RankedExperience
	Decision   Decision
	Reason     string
	Content    string // KEEP/COMPRESS content ready for context; empty for IGNORE/BLOCK
}

// Selector chooses KEEP / COMPRESS / IGNORE / BLOCK for ranked candidates.
type Selector struct {
	cfg Config
}

// New constructs a Selector.
func New(cfg Config) *Selector {
	if cfg.CompressMaxChars <= 0 {
		cfg.CompressMaxChars = DefaultConfig().CompressMaxChars
	}
	// accept legacy field name via zero CompressMaxChars already handled
	return &Selector{cfg: cfg}
}

var (
	episodeIDPattern = regexp.MustCompile(`(?i)\b(ep|episode)[_-]?[0-9a-f]{6,}\b`)
	uuidPattern      = regexp.MustCompile(`(?i)\b[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}\b`)
)

// Select applies decisions in order: BLOCK → IGNORE → COMPRESS → KEEP.
// Lexical overlap is intentionally NOT used (V1-12): embedding similarity already gates relevance.
// SelectOptions carries optional V2 signals into selection (e.g. unresolved conflicts).
type SelectOptions struct {
	// ConflictPeers maps experienceID → conflicting peer ID. Either side is BLOCKED.
	ConflictPeers map[string]string
}

// Select applies decisions in order: BLOCK → IGNORE → COMPRESS → KEEP.
func (s *Selector) Select(task string, ranked []retrieval.RankedExperience) []Result {
	return s.SelectWithOptions(task, ranked, SelectOptions{})
}

// SelectWithOptions is Select plus optional conflict-awareness (V2-5).
func (s *Selector) SelectWithOptions(task string, ranked []retrieval.RankedExperience, opts SelectOptions) []Result {
	_ = task // reserved for future Unicode/BM25 gates
	out := make([]Result, 0, len(ranked))
	for _, item := range ranked {
		out = append(out, s.selectOne(item, opts))
	}
	return out
}

func (s *Selector) selectOne(item retrieval.RankedExperience, opts SelectOptions) Result {
	exp := item.Experience
	score := item.Score

	if peer, ok := opts.ConflictPeers[exp.ID]; ok && peer != "" {
		return Result{
			Experience: item,
			Decision:   DecisionBlock,
			Reason:     "unresolved conflict with experience " + peer,
		}
	}

	if !exp.Status.Retrievable() {
		return Result{Experience: item, Decision: DecisionBlock, Reason: fmt.Sprintf("status %s is not retrievable", exp.Status)}
	}
	if exp.Utility <= s.cfg.BlockUtilityMax {
		return Result{Experience: item, Decision: DecisionBlock, Reason: fmt.Sprintf("utility %.3f below block threshold", exp.Utility)}
	}
	if exp.Confidence <= s.cfg.BlockConfidenceMax {
		return Result{Experience: item, Decision: DecisionBlock, Reason: fmt.Sprintf("confidence %.3f below block threshold", exp.Confidence)}
	}

	if score.FinalScore < s.cfg.IgnoreFinalScoreMax {
		return Result{Experience: item, Decision: DecisionIgnore, Reason: fmt.Sprintf("final_score %.4f too low", score.FinalScore)}
	}
	if score.ScopeMatch < s.cfg.IgnoreScopeMatchMax {
		return Result{Experience: item, Decision: DecisionIgnore, Reason: fmt.Sprintf("scope_match %.3f too low", score.ScopeMatch)}
	}

	needsCompress := utf8.RuneCountInString(exp.Content) > s.cfg.CompressMaxChars ||
		episodeIDPattern.MatchString(exp.Content) ||
		uuidPattern.MatchString(exp.Content)

	if needsCompress {
		content := compressContent(exp)
		return Result{
			Experience: item,
			Decision:   DecisionCompress,
			Reason:     "valuable but verbose or episode-specific; compressed (ids stripped, truncated)",
			Content:    content,
		}
	}

	if score.FinalScore < s.cfg.KeepFinalScoreMin {
		return Result{Experience: item, Decision: DecisionIgnore, Reason: fmt.Sprintf("final_score %.4f below keep threshold", score.FinalScore)}
	}

	return Result{
		Experience: item,
		Decision:   DecisionKeep,
		Reason:     "directly relevant, concise, and high enough score",
		Content:    strings.TrimSpace(exp.Content),
	}
}

func compressContent(exp experience.Experience) string {
	trigger := strings.TrimSpace(exp.Trigger)
	content := strings.TrimSpace(exp.Content)
	content = episodeIDPattern.ReplaceAllString(content, "")
	content = uuidPattern.ReplaceAllString(content, "")
	content = strings.Join(strings.Fields(content), " ")

	if trigger != "" {
		general := trigger
		if content != "" {
			trimmed := trimRunes(content, 120)
			if !strings.EqualFold(general, trimmed) {
				return general + ". " + trimmed
			}
		}
		return general
	}
	return trimRunes(content, 160)
}

func trimRunes(s string, max int) string {
	if max <= 0 || utf8.RuneCountInString(s) <= max {
		return s
	}
	runes := []rune(s)
	return string(runes[:max]) + "…"
}

// IsContextDecision reports whether the decision may enter agent context.
func IsContextDecision(d Decision) bool {
	return d == DecisionKeep || d == DecisionCompress || d == DecisionAbstract
}
