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
	DecisionAbstract Decision = "ABSTRACT"
	DecisionIgnore   Decision = "IGNORE"
	DecisionBlock    Decision = "BLOCK"
)

// Config holds explainable thresholds for V1 rule-based selection.
type Config struct {
	BlockUtilityMax     float64
	BlockConfidenceMax  float64
	IgnoreFinalScoreMax float64
	IgnoreScopeMatchMax float64
	KeepFinalScoreMin   float64
	AbstractMaxChars    int
}

// DefaultConfig returns V1 defaults.
func DefaultConfig() Config {
	return Config{
		BlockUtilityMax:     0.15,
		BlockConfidenceMax:  0.25,
		IgnoreFinalScoreMax: 0.05,
		IgnoreScopeMatchMax: 0.3,
		KeepFinalScoreMin:   0.08,
		AbstractMaxChars:    180,
	}
}

// Result is one selection decision with the content that may enter context.
type Result struct {
	Experience retrieval.RankedExperience
	Decision   Decision
	Reason     string
	Content    string // KEEP/ABSTRACT content ready for context; empty for IGNORE/BLOCK
}

// Selector chooses KEEP / ABSTRACT / IGNORE / BLOCK for ranked candidates.
type Selector struct {
	cfg Config
}

// New constructs a Selector.
func New(cfg Config) *Selector {
	if cfg.AbstractMaxChars <= 0 {
		cfg.AbstractMaxChars = DefaultConfig().AbstractMaxChars
	}
	return &Selector{cfg: cfg}
}

var (
	episodeIDPattern = regexp.MustCompile(`(?i)\b(ep|episode)[_-]?[0-9a-f]{6,}\b`)
	uuidPattern      = regexp.MustCompile(`(?i)\b[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}\b`)
)

// Select applies decisions in order: BLOCK → IGNORE → ABSTRACT → KEEP.
func (s *Selector) Select(task string, ranked []retrieval.RankedExperience) []Result {
	out := make([]Result, 0, len(ranked))
	for _, item := range ranked {
		out = append(out, s.selectOne(task, item))
	}
	return out
}

func (s *Selector) selectOne(task string, item retrieval.RankedExperience) Result {
	exp := item.Experience
	score := item.Score

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
	if !taskOverlap(task, exp) {
		return Result{Experience: item, Decision: DecisionIgnore, Reason: "no lexical overlap with current task"}
	}

	needsAbstract := utf8.RuneCountInString(exp.Content) > s.cfg.AbstractMaxChars ||
		episodeIDPattern.MatchString(exp.Content) ||
		uuidPattern.MatchString(exp.Content)

	if needsAbstract {
		content := abstractContent(exp)
		return Result{
			Experience: item,
			Decision:   DecisionAbstract,
			Reason:     "valuable but contains episode-specific detail; abstracted to general rule",
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

func taskOverlap(task string, exp experience.Experience) bool {
	taskTokens := tokenize(task)
	if len(taskTokens) == 0 {
		return true
	}
	hay := strings.ToLower(exp.Trigger + " " + exp.Content + " " + exp.ScopeKey + " " + string(exp.Type))
	for tok := range taskTokens {
		if strings.Contains(hay, tok) {
			return true
		}
	}
	return false
}

func tokenize(s string) map[string]struct{} {
	fields := strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		return !((r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'))
	})
	out := make(map[string]struct{})
	for _, f := range fields {
		if len(f) < 3 {
			continue
		}
		// skip ultra-common tokens
		switch f {
		case "the", "and", "for", "with", "from", "this", "that", "when", "into":
			continue
		}
		out[f] = struct{}{}
	}
	return out
}

func abstractContent(exp experience.Experience) string {
	trigger := strings.TrimSpace(exp.Trigger)
	content := strings.TrimSpace(exp.Content)
	content = episodeIDPattern.ReplaceAllString(content, "")
	content = uuidPattern.ReplaceAllString(content, "")
	content = strings.Join(strings.Fields(content), " ")

	if trigger != "" {
		general := trigger
		if content != "" {
			// Prefer a short general rule: trigger + truncated lesson.
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
