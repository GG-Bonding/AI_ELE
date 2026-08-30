package retrieval_test

import (
	"testing"

	"github.com/agent-experience-engine/agent-experience-engine/internal/experience"
	"github.com/agent-experience-engine/agent-experience-engine/internal/retrieval"
)

func TestAuthorizedToolHardFilter(t *testing.T) {
	t.Parallel()
	exp := experience.Experience{Scope: experience.ScopeTool, ScopeKey: "slack"}
	if retrieval.Authorized(exp, retrieval.Query{Tools: []string{"jira"}}) {
		t.Fatal("slack tool tip must not authorize for jira tools")
	}
	if !retrieval.Authorized(exp, retrieval.Query{Tools: []string{"slack.post"}}) {
		t.Fatal("slack tip should authorize for slack.post prefix")
	}
	if retrieval.Authorized(exp, retrieval.Query{}) {
		t.Fatal("TOOL scope without tools/scope_key must fail closed")
	}
	if retrieval.Authorized(exp, retrieval.Query{Tools: []string{"jira"}}) {
		t.Fatal("slack tip must not authorize for jira tools")
	}
}

func TestAuthorizedUserAndAgentHardFilter(t *testing.T) {
	t.Parallel()
	userExp := experience.Experience{Scope: experience.ScopeUser, ScopeKey: "u1"}
	if retrieval.Authorized(userExp, retrieval.Query{UserID: "u2"}) {
		t.Fatal("user scope must isolate")
	}
	if !retrieval.Authorized(userExp, retrieval.Query{UserID: "u1"}) {
		t.Fatal("matching user should authorize")
	}
	if retrieval.Authorized(userExp, retrieval.Query{}) {
		t.Fatal("USER scope without user_id must fail closed")
	}
	emptyKeyUser := experience.Experience{Scope: experience.ScopeUser, ScopeKey: ""}
	if retrieval.Authorized(emptyKeyUser, retrieval.Query{UserID: "u1"}) {
		t.Fatal("USER scope with empty scope_key must fail closed")
	}

	agentExp := experience.Experience{Scope: experience.ScopeAgent, ScopeKey: "a1"}
	if retrieval.Authorized(agentExp, retrieval.Query{AgentID: "a2"}) {
		t.Fatal("agent scope must isolate")
	}
	if !retrieval.Authorized(agentExp, retrieval.Query{AgentID: "a1"}) {
		t.Fatal("matching agent should authorize")
	}
	if retrieval.Authorized(agentExp, retrieval.Query{}) {
		t.Fatal("AGENT scope without agent_id must fail closed")
	}
	emptyKeyAgent := experience.Experience{Scope: experience.ScopeAgent, ScopeKey: ""}
	if retrieval.Authorized(emptyKeyAgent, retrieval.Query{AgentID: "a1"}) {
		t.Fatal("AGENT scope with empty scope_key must fail closed")
	}
}

func TestAuthorizedToolEmptyScopeKeyDenied(t *testing.T) {
	t.Parallel()
	exp := experience.Experience{Scope: experience.ScopeTool, ScopeKey: ""}
	if retrieval.Authorized(exp, retrieval.Query{Tools: []string{"jira"}}) {
		t.Fatal("TOOL scope with empty scope_key must fail closed")
	}
}
