package experience

import (
	"context"
	"fmt"
	"strings"
	"unicode"
)

// Generalization gate defaults (V2-7).
const (
	MinGeneralizeExperiences = 3
	MinGeneralizeEpisodes    = 3
	MinGeneralizeAvgUtility  = 0.70
	MaxGeneralizeConflictRate = 0.10
)

// PatternDraft is a proposed Pattern body before persistence.
type PatternDraft struct {
	Type       Type
	Scope      Scope
	ScopeKey   string
	Trigger    string
	Content    string
	Confidence float64
	Utility    float64
}

// PatternGeneralizer turns a cluster of experiences into a Pattern draft.
// HeuristicGeneralizer is the default; an LLM implementation can replace it later.
type PatternGeneralizer interface {
	Generalize(ctx context.Context, exps []Experience) (PatternDraft, error)
}

// HeuristicPatternGeneralizer builds a deterministic abstract rule without an LLM.
type HeuristicPatternGeneralizer struct{}

// Generalize implements PatternGeneralizer.
func (HeuristicPatternGeneralizer) Generalize(_ context.Context, exps []Experience) (PatternDraft, error) {
	if len(exps) == 0 {
		return PatternDraft{}, fmt.Errorf("%w: no experiences to generalize", ErrInvalidInput)
	}
	typ, scope, scopeKey := exps[0].Type, exps[0].Scope, exps[0].ScopeKey
	var confSum, utilSum float64
	triggers := make([]string, 0, len(exps))
	contents := make([]string, 0, len(exps))
	for _, e := range exps {
		confSum += e.Confidence
		utilSum += e.Utility
		triggers = append(triggers, e.Trigger)
		contents = append(contents, e.Content)
	}
	n := float64(len(exps))
	common := commonTokens(triggers)
	trigger := strings.TrimSpace(strings.Join(common, " "))
	if trigger == "" {
		trigger = "related operations in scope " + string(scope)
		if scopeKey != "" {
			trigger += ":" + scopeKey
		}
	} else {
		trigger = "when " + trigger
	}
	content := "Across " + fmt.Sprintf("%d", len(exps)) + " related experiences: apply a shared rule — " +
		summarizeContents(contents)
	return PatternDraft{
		Type:       typ,
		Scope:      scope,
		ScopeKey:   scopeKey,
		Trigger:    trigger,
		Content:    content,
		Confidence: confSum / n,
		Utility:    utilSum / n,
	}, nil
}

func commonTokens(texts []string) []string {
	if len(texts) == 0 {
		return nil
	}
	sets := make([]map[string]struct{}, 0, len(texts))
	for _, text := range texts {
		sets = append(sets, tokenize(text))
	}
	var out []string
	for tok := range sets[0] {
		ok := true
		for i := 1; i < len(sets); i++ {
			if _, hit := sets[i][tok]; !hit {
				ok = false
				break
			}
		}
		if ok && len(tok) > 2 {
			out = append(out, tok)
		}
	}
	// Stable-ish order by appearance in first text.
	order := tokenizeOrder(texts[0])
	ranked := make([]string, 0, len(out))
	seen := map[string]struct{}{}
	for _, tok := range order {
		for _, c := range out {
			if c == tok {
				if _, ok := seen[c]; ok {
					continue
				}
				seen[c] = struct{}{}
				ranked = append(ranked, c)
			}
		}
	}
	if len(ranked) > 8 {
		ranked = ranked[:8]
	}
	return ranked
}

func tokenize(s string) map[string]struct{} {
	out := make(map[string]struct{})
	for _, tok := range tokenizeOrder(s) {
		out[tok] = struct{}{}
	}
	return out
}

func tokenizeOrder(s string) []string {
	var b strings.Builder
	var out []string
	flush := func() {
		tok := strings.ToLower(strings.TrimSpace(b.String()))
		b.Reset()
		if tok == "" {
			return
		}
		out = append(out, tok)
	}
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			continue
		}
		if unicode.In(r, unicode.Han) {
			flush()
			out = append(out, string(r))
			continue
		}
		flush()
	}
	flush()
	return out
}

func summarizeContents(contents []string) string {
	if len(contents) == 0 {
		return "resolve shared prerequisites before acting"
	}
	// Prefer the shortest non-empty content as a compact prototype rule.
	best := strings.TrimSpace(contents[0])
	for _, c := range contents[1:] {
		c = strings.TrimSpace(c)
		if c == "" {
			continue
		}
		if best == "" || len(c) < len(best) {
			best = c
		}
	}
	if best == "" {
		return "resolve shared prerequisites before acting"
	}
	return best
}
