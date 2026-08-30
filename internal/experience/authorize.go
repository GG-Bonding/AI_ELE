package experience

import "strings"

// AuthorizedForSearch reports whether exp may appear in search results for the given auth context.
// V1 fail-closed: USER/AGENT scopes require matching IDs; TOOL scope requires tools or scope_key.
func AuthorizedForSearch(exp Experience, agentID, userID string, tools []string, queryScopeKey string) bool {
	switch exp.Scope {
	case ScopeTool:
		return toolAuthorizedForSearch(exp.ScopeKey, tools, queryScopeKey)
	case ScopeAgent:
		agentID = strings.TrimSpace(agentID)
		if agentID == "" {
			return false
		}
		key := strings.TrimSpace(exp.ScopeKey)
		return key == "" || key == agentID
	case ScopeUser:
		userID = strings.TrimSpace(userID)
		if userID == "" {
			return false
		}
		key := strings.TrimSpace(exp.ScopeKey)
		return key == "" || key == userID
	default:
		return true
	}
}

func toolAuthorizedForSearch(scopeKey string, tools []string, queryScopeKey string) bool {
	key := strings.TrimSpace(scopeKey)
	if key == "" {
		return true
	}
	allowed := make(map[string]struct{}, len(tools)+1)
	for _, t := range tools {
		t = strings.TrimSpace(t)
		if t != "" {
			allowed[t] = struct{}{}
		}
	}
	if k := strings.TrimSpace(queryScopeKey); k != "" {
		allowed[k] = struct{}{}
	}
	if len(allowed) == 0 {
		return false
	}
	if _, ok := allowed[key]; ok {
		return true
	}
	for t := range allowed {
		if strings.HasPrefix(t, key+".") || strings.HasPrefix(t, key+"/") {
			return true
		}
	}
	return false
}
