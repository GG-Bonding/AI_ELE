package retrieval

import (
	"github.com/agent-experience-engine/agent-experience-engine/internal/experience"
)

// Authorized reports whether an experience may enter the candidate set for q.
// V1-09–11: USER / AGENT / TOOL scopes are hard-filtered before ranking.
func Authorized(exp experience.Experience, q Query) bool {
	return experience.AuthorizedForSearch(exp, q.AgentID, q.UserID, q.Tools, q.ScopeKey)
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
