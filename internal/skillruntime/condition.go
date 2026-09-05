package skillruntime

import (
	"fmt"
	"strconv"
	"strings"
)

// EvalCondition evaluates a simple Skill condition expression against bindings.
//
// Supported forms (empty expr → true):
//
//	path                  — truthy lookup (e.g. project.key)
//	path == value         — string equality
//	path != value         — string inequality
//	exists path           — key present and non-nil
func EvalCondition(expr string, bindings map[string]any) (bool, error) {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return true, nil
	}
	lower := strings.ToLower(expr)
	if strings.HasPrefix(lower, "exists ") {
		path := strings.TrimSpace(expr[len("exists "):])
		_, ok := conditionLookup(bindings, path)
		return ok, nil
	}
	if i := strings.Index(expr, "!="); i >= 0 {
		left := strings.TrimSpace(expr[:i])
		right := unquote(strings.TrimSpace(expr[i+2:]))
		got, ok := conditionLookup(bindings, left)
		if !ok {
			return right != "", nil
		}
		return stringify(got) != right, nil
	}
	if i := strings.Index(expr, "=="); i >= 0 {
		left := strings.TrimSpace(expr[:i])
		right := unquote(strings.TrimSpace(expr[i+2:]))
		got, ok := conditionLookup(bindings, left)
		if !ok {
			return false, nil
		}
		return stringify(got) == right, nil
	}
	got, ok := conditionLookup(bindings, expr)
	if !ok {
		return false, nil
	}
	return isTruthy(got), nil
}

func unquote(s string) string {
	if len(s) >= 2 {
		if (s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'') {
			return s[1 : len(s)-1]
		}
	}
	return s
}

func stringify(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case bool:
		return strconv.FormatBool(t)
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	case int:
		return strconv.Itoa(t)
	case int64:
		return strconv.FormatInt(t, 10)
	default:
		return fmt.Sprint(v)
	}
}

func isTruthy(v any) bool {
	switch t := v.(type) {
	case nil:
		return false
	case bool:
		return t
	case string:
		return t != ""
	case float64:
		return t != 0
	case int:
		return t != 0
	case int64:
		return t != 0
	default:
		return true
	}
}

func conditionLookup(bindings map[string]any, path string) (any, bool) {
	path = strings.TrimSpace(path)
	path = strings.TrimPrefix(path, "{{")
	path = strings.TrimSuffix(path, "}}")
	path = strings.TrimSpace(path)
	if path == "" || bindings == nil {
		return nil, false
	}
	parts := strings.Split(path, ".")
	var cur any = bindings
	for _, p := range parts {
		m, ok := cur.(map[string]any)
		if !ok {
			return nil, false
		}
		next, ok := m[p]
		if !ok {
			return nil, false
		}
		cur = next
	}
	return cur, true
}
