package action

import "strings"

// NormalizeFieldPath trims and lowercases a JSON-path-like field locator.
func NormalizeFieldPath(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

// NormalizeAffectedFields dedupes and normalizes field paths, dropping empties.
func NormalizeAffectedFields(fields []string) []string {
	if len(fields) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(fields))
	for _, f := range fields {
		n := NormalizeFieldPath(f)
		if n == "" {
			continue
		}
		if _, ok := seen[n]; ok {
			continue
		}
		seen[n] = struct{}{}
		out = append(out, n)
	}
	return out
}

// FieldPathMatches reports whether a feedback target field matches a declared affected path.
// Exact match, or shared last path segment (priority ↔ input.priority).
func FieldPathMatches(targetField string, affected []string) bool {
	t := NormalizeFieldPath(targetField)
	if t == "" || len(affected) == 0 {
		return false
	}
	tLast := lastPathSegment(t)
	for _, a := range affected {
		a = NormalizeFieldPath(a)
		if a == "" {
			continue
		}
		if a == t || lastPathSegment(a) == tLast {
			return true
		}
		if strings.HasSuffix(a, "."+t) || strings.HasSuffix(t, "."+a) {
			return true
		}
	}
	return false
}

func lastPathSegment(path string) string {
	if i := strings.LastIndex(path, "."); i >= 0 && i+1 < len(path) {
		return path[i+1:]
	}
	return path
}
