package retrieval

import (
	"strings"

	"github.com/agent-experience-engine/agent-experience-engine/internal/experience"
)

// Authorized reports whether an experience may enter the candidate set for q.
// V1-09–11: USER / AGENT / TOOL scopes are hard-filtered before ranking.
func Authorized(exp experience.Experience, q Query) bool {
	switch exp.Scope {
	case experience.ScopeTool:
		return toolAuthorized(exp.ScopeKey, q.Tools, q.ScopeKey)
	case experience.ScopeAgent:
		agentID := strings.TrimSpace(q.AgentID)
		if agentID == "" {
			return true // no agent constraint → soft match only
		}
		key := strings.TrimSpace(exp.ScopeKey)
		return key == "" || key == agentID
	case experience.ScopeUser:
		userID := strings.TrimSpace(q.UserID)
		if userID == "" {
			return true // no user constraint → soft match only
		}
		key := strings.TrimSpace(exp.ScopeKey)
		return key == "" || key == userID
	default:
		return true
	}
}

func toolAuthorized(scopeKey string, tools []string, queryScopeKey string) bool {
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
	// No tool constraint on the query → do not hard-exclude (soft ScopeMatch still applies).
	if len(allowed) == 0 {
		return true
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

// FilterAuthorized drops experiences that fail hard scope authorization.
func FilterAuthorized(candidates []experience.ScoredExperience, q Query) []experience.ScoredExperience {
	if len(candidates) == 0 {
		return candidates
	}
	out := make([]experience.ScoredExperience, 0, len(candidates))
	for _, c := range candidates {
		if Authorized(c.Experience, q) {
			out = append(out, c)
		}
	}
	return out
}
