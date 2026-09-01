package retrieval

import (
	"strings"
	"unicode"
)

// LexicalOverlap is Jaccard similarity of alphanumeric / Han tokens between a and b.
func LexicalOverlap(a, b string) float64 {
	ta := tokenizeLexical(a)
	tb := tokenizeLexical(b)
	if len(ta) == 0 || len(tb) == 0 {
		return 0
	}
	inter := 0
	for t := range ta {
		if _, ok := tb[t]; ok {
			inter++
		}
	}
	union := len(ta) + len(tb) - inter
	if union == 0 {
		return 0
	}
	return float64(inter) / float64(union)
}

func tokenizeLexical(s string) map[string]struct{} {
	out := make(map[string]struct{})
	var b strings.Builder
	flush := func() {
		tok := strings.ToLower(strings.TrimSpace(b.String()))
		b.Reset()
		if tok == "" {
			return
		}
		out[tok] = struct{}{}
	}
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			continue
		}
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
