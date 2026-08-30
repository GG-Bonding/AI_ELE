package experience

import (
	"context"
	"strings"
	"unicode"
)

// DedupDecision is the semantic-dedup judge outcome (V2-4).
type DedupDecision string

const (
	DedupSame      DedupDecision = "SAME"
	DedupRelated   DedupDecision = "RELATED"
	DedupDifferent DedupDecision = "DIFFERENT"
	DedupConflict  DedupDecision = "CONFLICT"
)

// DedupPair is a candidate compared against an existing neighbor.
type DedupPair struct {
	CandidateTrigger string
	CandidateContent string
	NeighborTrigger  string
	NeighborContent  string
	Similarity       float64
}

// DedupJudge classifies near-neighbor experiences. High similarity alone is not SAME
// (e.g. "must enable" vs "must not enable" → CONFLICT).
type DedupJudge interface {
	Judge(ctx context.Context, pair DedupPair) (DedupDecision, error)
}

// SemanticDedupConfig controls cross-episode semantic merge in the store pipeline.
type SemanticDedupConfig struct {
	// Disabled opts out of semantic dedup (exact fingerprint dedup still applies).
	Disabled bool
	// MinSimilarity is the cosine threshold to invoke the judge (default 0.92).
	MinSimilarity float64
	// AutoSameSimilarity treats near-identical embeddings as SAME when no conflict (default 0.97).
	AutoSameSimilarity float64
	// NeighborTopK is how many nearest same-type/scope neighbors to inspect (default 5).
	NeighborTopK int
	// Judge defaults to HeuristicDedupJudge when nil.
	Judge DedupJudge
}

func (c SemanticDedupConfig) withDefaults() SemanticDedupConfig {
	out := c
	if out.MinSimilarity <= 0 {
		out.MinSimilarity = 0.92
	}
	if out.AutoSameSimilarity <= 0 {
		out.AutoSameSimilarity = 0.97
	}
	if out.NeighborTopK <= 0 {
		out.NeighborTopK = 5
	}
	if out.Judge == nil {
		out.Judge = HeuristicDedupJudge{AutoSameSimilarity: out.AutoSameSimilarity}
	}
	return out
}

// HeuristicDedupJudge is a deterministic judge for V2-4 without an LLM.
// CONFLICT when opposing polarity is detected; SAME on high lexical overlap or auto-same similarity;
// otherwise RELATED when similarity already passed the pipeline threshold.
type HeuristicDedupJudge struct {
	AutoSameSimilarity float64
}

// Judge implements DedupJudge.
func (j HeuristicDedupJudge) Judge(_ context.Context, pair DedupPair) (DedupDecision, error) {
	autoSame := j.AutoSameSimilarity
	if autoSame <= 0 {
		autoSame = 0.97
	}
	if opposingPolarity(pair.CandidateTrigger+" "+pair.CandidateContent, pair.NeighborTrigger+" "+pair.NeighborContent) {
		return DedupConflict, nil
	}
	overlap := tokenJaccard(
		pair.CandidateTrigger+" "+pair.CandidateContent,
		pair.NeighborTrigger+" "+pair.NeighborContent,
	)
	if overlap >= 0.85 || pair.Similarity >= autoSame {
		return DedupSame, nil
	}
	if pair.Similarity >= 0.92 {
		return DedupRelated, nil
	}
	return DedupDifferent, nil
}

func opposingPolarity(a, b string) bool {
	na, pa := polaritySignals(a)
	nb, pb := polaritySignals(b)
	// One side forbid-ish, other require-ish, sharing enough lexical mass.
	if (na && pb) || (nb && pa) {
		return tokenJaccard(a, b) >= 0.4
	}
	return false
}

func polaritySignals(s string) (negative, positive bool) {
	lower := strings.ToLower(s)
	negMarkers := []string{
		"禁止", "不要", "不可", "不能", "不得", "勿", "严禁",
		"must not", "mustn’t", "mustn't", "do not", "don't", "never", "forbid", "forbidden", "prohibit", "禁止打开",
	}
	posMarkers := []string{
		"必须", "务必", "应当", "应该", "一定要", "需要打开",
		"must ", "must\t", "always ", "require", "required", "应当打开", "必须打开",
	}
	for _, m := range negMarkers {
		if strings.Contains(lower, m) {
			negative = true
			break
		}
	}
	for _, m := range posMarkers {
		if strings.Contains(lower, m) {
			positive = true
			break
		}
	}
	// "must not" also matches "must " — clear positive when negative present.
	if negative {
		positive = false
	}
	return negative, positive
}

func tokenJaccard(a, b string) float64 {
	sa := tokenSet(a)
	sb := tokenSet(b)
	if len(sa) == 0 || len(sb) == 0 {
		return 0
	}
	inter := 0
	for t := range sa {
		if _, ok := sb[t]; ok {
			inter++
		}
	}
	union := len(sa) + len(sb) - inter
	if union == 0 {
		return 0
	}
	return float64(inter) / float64(union)
}

func tokenSet(s string) map[string]struct{} {
	out := make(map[string]struct{})
	var b strings.Builder
	flush := func() {
		tok := strings.ToLower(strings.TrimSpace(b.String()))
		b.Reset()
		if tok == "" || len(tok) == 1 {
			return
		}
		out[tok] = struct{}{}
	}
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			continue
		}
		// CJK ideographs as single-char tokens keep Chinese polarity markers useful.
		if unicode.In(r, unicode.Han) {
			flush()
			out[string(r)] = struct{}{}
			continue
		}
		flush()
	}
	flush()
	return out
}
