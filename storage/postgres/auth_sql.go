package postgres

import (
	"fmt"
	"strings"
)

// expandToolsForAuth includes tool names and their base prefixes for scope_key matching.
func expandToolsForAuth(tools []string, queryScopeKey string) []string {
	set := make(map[string]struct{})
	add := func(s string) {
		s = strings.TrimSpace(s)
		if s == "" {
			return
		}
		set[s] = struct{}{}
		if i := strings.IndexAny(s, "./"); i > 0 {
			set[s[:i]] = struct{}{}
		}
	}
	for _, t := range tools {
		add(t)
	}
	add(queryScopeKey)
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	return out
}

func pgTextArray(vals []string) string {
	if len(vals) == 0 {
		return "{}"
	}
	parts := make([]string, len(vals))
	for i, v := range vals {
		parts[i] = `"` + strings.ReplaceAll(v, `"`, `\"`) + `"`
	}
	return "{" + strings.Join(parts, ",") + "}"
}

// scopeAuthSQL returns a WHERE fragment and appends bind args for fail-closed scope authorization.
func scopeAuthSQL(agentID, userID, queryScopeKey string, tools []string, args []any) (string, []any) {
	agentID = strings.TrimSpace(agentID)
	userID = strings.TrimSpace(userID)
	tools = expandToolsForAuth(tools, queryScopeKey)

	var parts []string
	parts = append(parts, "scope NOT IN ('USER','AGENT','TOOL')")

	args = append(args, userID)
	userPH := fmt.Sprintf("$%d", len(args))
	parts = append(parts, fmt.Sprintf("(scope = 'USER' AND scope_key <> '' AND scope_key = %s)", userPH))

	args = append(args, agentID)
	agentPH := fmt.Sprintf("$%d", len(args))
	parts = append(parts, fmt.Sprintf("(scope = 'AGENT' AND scope_key <> '' AND scope_key = %s)", agentPH))

	args = append(args, pgTextArray(tools))
	toolsPH := fmt.Sprintf("$%d::text[]", len(args))
	toolClause := fmt.Sprintf(`(scope = 'TOOL' AND scope_key <> '' AND (
		scope_key = ANY(%s)
		OR EXISTS (
			SELECT 1 FROM unnest(%s) AS t(tool)
			WHERE t.tool LIKE scope_key || '.%%' OR t.tool LIKE scope_key || '/%%'
		)
	))`, toolsPH, toolsPH)
	parts = append(parts, toolClause)

	return "(" + strings.Join(parts, " OR ") + ")", args
}
