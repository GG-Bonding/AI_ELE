package experience_test

import (
	"context"
	"strings"
	"testing"

	"github.com/agent-experience-engine/agent-experience-engine/internal/experience"
	"github.com/agent-experience-engine/agent-experience-engine/internal/provider"
)

func TestLLMPatternGeneralizerEvidenceGrounded(t *testing.T) {
	t.Parallel()
	mock := &provider.MockLLM{Responses: []string{`{
		"type":"PROCEDURAL",
		"scope":"TOOL",
		"scope_key":"jira",
		"trigger":"when creating a Jira issue and the project identifier is unknown",
		"content":"Resolve the project key via search before create_issue; never pass a display name as the project field.",
		"confidence":0.88,
		"utility":0.82
	}`}}
	gen, err := experience.NewLLMPatternGeneralizer(mock)
	if err != nil {
		t.Fatal(err)
	}
	exps := []experience.Experience{
		{ID: "e1", Type: experience.TypeProcedural, Scope: experience.ScopeTool, ScopeKey: "jira",
			Trigger: "create jira", Content: "Search projects first then use PAY", Confidence: 0.8, Utility: 0.75},
		{ID: "e2", Type: experience.TypeProcedural, Scope: experience.ScopeTool, ScopeKey: "jira",
			Trigger: "payment jira", Content: "INVALID_PROJECT_KEY means use key not Payment", Confidence: 0.85, Utility: 0.8},
		{ID: "e3", Type: experience.TypeProcedural, Scope: experience.ScopeTool, ScopeKey: "jira",
			Trigger: "jira issue", Content: "Always resolve project key before create_issue", Confidence: 0.9, Utility: 0.85},
	}
	draft, err := gen.Generalize(context.Background(), exps)
	if err != nil {
		t.Fatal(err)
	}
	if draft.Type != experience.TypeProcedural || draft.Scope != experience.ScopeTool {
		t.Fatalf("draft=%#v", draft)
	}
	if !strings.Contains(strings.ToLower(draft.Content), "project") {
		t.Fatalf("content not grounded: %s", draft.Content)
	}
	// Must not be the heuristic shortest-content prototype.
	if draft.Content == "Search projects first then use PAY" {
		t.Fatal("LLM draft should not be a verbatim shortest experience")
	}
	if len(mock.Calls) != 1 {
		t.Fatalf("calls=%d", len(mock.Calls))
	}
	if !strings.Contains(mock.Calls[0].User, "e1") || !strings.Contains(mock.Calls[0].User, "INVALID_PROJECT_KEY") {
		t.Fatalf("prompt missing evidence: %s", mock.Calls[0].User)
	}
}

func TestLLMPatternGeneralizerRetriesInvalidJSON(t *testing.T) {
	t.Parallel()
	mock := &provider.MockLLM{Responses: []string{
		`not json`,
		`{"type":"PROCEDURAL","scope":"TOOL","scope_key":"jira","trigger":"when jira","content":"resolve key first","confidence":0.7,"utility":0.7}`,
	}}
	gen, err := experience.NewLLMPatternGeneralizer(mock)
	if err != nil {
		t.Fatal(err)
	}
	draft, err := gen.Generalize(context.Background(), []experience.Experience{{
		ID: "e1", Type: experience.TypeProcedural, Scope: experience.ScopeTool, ScopeKey: "jira",
		Trigger: "t", Content: "c", Confidence: 0.7, Utility: 0.7,
	}})
	if err != nil {
		t.Fatal(err)
	}
	if draft.Content != "resolve key first" {
		t.Fatalf("%#v", draft)
	}
	if len(mock.Calls) != 2 {
		t.Fatalf("want retry, calls=%d", len(mock.Calls))
	}
}

func TestNewLLMPatternGeneralizerRequiresLLM(t *testing.T) {
	t.Parallel()
	if _, err := experience.NewLLMPatternGeneralizer(nil); err == nil {
		t.Fatal("expected error")
	}
}
