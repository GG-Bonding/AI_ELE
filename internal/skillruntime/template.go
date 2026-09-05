package skillruntime

import (
	"fmt"
	"regexp"
	"strings"
)

var templateRef = regexp.MustCompile(`\{\{\s*([^}]+?)\s*\}\}`)

// ResolveArgs walks args and replaces `{{ path }}` templates via dotted lookup in bindings.
// When a string value is exactly one template expression, the looked-up value is substituted
// with its native type (not stringified). Nested maps/slices are resolved recursively.
func ResolveArgs(args map[string]any, bindings map[string]any) (map[string]any, error) {
	if args == nil {
		return map[string]any{}, nil
	}
	out := make(map[string]any, len(args))
	for k, v := range args {
		resolved, err := resolveValue(v, bindings)
		if err != nil {
			return nil, err
		}
		out[k] = resolved
	}
	return out, nil
}

func resolveValue(v any, bindings map[string]any) (any, error) {
	switch t := v.(type) {
	case string:
		return resolveString(t, bindings)
	case map[string]any:
		return ResolveArgs(t, bindings)
	case []any:
		out := make([]any, len(t))
		for i, child := range t {
			r, err := resolveValue(child, bindings)
			if err != nil {
				return nil, err
			}
			out[i] = r
		}
		return out, nil
	default:
		return v, nil
	}
}

func resolveString(s string, bindings map[string]any) (any, error) {
	trimmed := strings.TrimSpace(s)
	if m := templateRef.FindStringSubmatch(trimmed); m != nil && m[0] == trimmed {
		val, ok := lookupPath(bindings, strings.TrimSpace(m[1]))
		if !ok {
			return nil, fmt.Errorf("unresolved template %q", trimmed)
		}
		return val, nil
	}
	var firstErr error
	out := templateRef.ReplaceAllStringFunc(s, func(match string) string {
		sub := templateRef.FindStringSubmatch(match)
		if sub == nil {
			return match
		}
		val, ok := lookupPath(bindings, strings.TrimSpace(sub[1]))
		if !ok {
			if firstErr == nil {
				firstErr = fmt.Errorf("unresolved template %q", match)
			}
			return match
		}
		return fmt.Sprint(val)
	})
	if firstErr != nil {
		return nil, firstErr
	}
	return out, nil
}

// lookupPath resolves dotted paths against nested maps (e.g. "inputs.title", "project.key").
func lookupPath(root map[string]any, path string) (any, bool) {
	path = strings.TrimSpace(path)
	if path == "" || root == nil {
		return nil, false
	}
	parts := strings.Split(path, ".")
	var cur any = root
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			return nil, false
		}
		m, ok := asMap(cur)
		if !ok {
			return nil, false
		}
		next, ok := m[part]
		if !ok {
			return nil, false
		}
		cur = next
	}
	return cur, true
}

func asMap(v any) (map[string]any, bool) {
	switch m := v.(type) {
	case map[string]any:
		return m, true
	case map[string]string:
		out := make(map[string]any, len(m))
		for k, val := range m {
			out[k] = val
		}
		return out, true
	default:
		return nil, false
	}
}
