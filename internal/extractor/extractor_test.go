package extractor_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/agent-experience-engine/agent-experience-engine/internal/attempt"
	"github.com/agent-experience-engine/agent-experience-engine/internal/episode"
	"github.com/agent-experience-engine/agent-experience-engine/internal/experience"
	"github.com/agent-experience-engine/agent-experience-engine/internal/extractor"
	"github.com/agent-experience-engine/agent-experience-engine/internal/outcome"
	"github.com/agent-experience-engine/agent-experience-engine/internal/provider"
)

func TestValidateCandidatesJSONAcceptsValidPayload(t *testing.T) {
	t.Parallel()
	raw := []byte(`{
	  "experiences": [
	    {
	      "type": "PROCEDURAL",
	      "trigger": "create jira issue when project key unknown",
	      "content": "Resolve the Jira project key before calling create_issue.",
	      "confidence": 0.91,
	      "scope": "TOOL",
	      "scope_key": "jira"
	    }
	  ]
	}`)
	got, err := extractor.ValidateCandidatesJSON(raw)
	if err != nil {
		t.Fatalf("ValidateCandidatesJSON: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len = %d", len(got))
	}
	if got[0].Type != experience.TypeProcedural {
		t.Fatalf("type = %s", got[0].Type)
	}
}

func TestValidateCandidatesJSONRejectsInvalidType(t *testing.T) {
	t.Parallel()
	_, err := extractor.ValidateCandidatesJSON([]byte(`{
	  "experiences": [{"type":"NOPE","trigger":"t","content":"c","confidence":0.5,"scope":"TOOL"}]
	}`))
	if err == nil {
		t.Fatal("expected schema error")
	}
}

func TestExtractJiraStyleWithMockLLM(t *testing.T) {
	t.Parallel()
	mock := &provider.MockLLM{
		Responses: []string{validJiraExtractionJSON()},
	}
	ext, err := extractor.New(mock)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	in := sampleJiraTrace()
	got, err := ext.Extract(context.Background(), in)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if len(got) < 2 {
		t.Fatalf("expected multiple experiences, got %d", len(got))
	}
	if len(mock.Calls) != 1 {
		t.Fatalf("llm calls = %d, want 1", len(mock.Calls))
	}

	var sawProcedural, sawFailure, sawSemantic bool
	for _, c := range got {
		switch c.Type {
		case experience.TypeProcedural:
			sawProcedural = true
		case experience.TypeFailure:
			sawFailure = true
		case experience.TypeSemantic:
			sawSemantic = true
		}
	}
	if !sawProcedural || !sawFailure || !sawSemantic {
		t.Fatalf("missing expected types: procedural=%v failure=%v semantic=%v", sawProcedural, sawFailure, sawSemantic)
	}
}

func TestExtractRetriesOnceOnInvalidJSON(t *testing.T) {
	t.Parallel()
	mock := &provider.MockLLM{
		Responses: []string{
			`not-json`,
			validJiraExtractionJSON(),
		},
	}
	ext, err := extractor.New(mock)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	got, err := ext.Extract(context.Background(), sampleJiraTrace())
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if len(got) == 0 {
		t.Fatal("expected candidates after retry")
	}
	if len(mock.Calls) != 2 {
		t.Fatalf("llm calls = %d, want 2", len(mock.Calls))
	}
	if !strings.Contains(mock.Calls[1].User, "Previous response was invalid") {
		t.Fatalf("retry prompt missing error context: %q", mock.Calls[1].User)
	}
}

func TestExtractFailsAfterRetry(t *testing.T) {
	t.Parallel()
	mock := &provider.MockLLM{
		Responses: []string{`{"experiences":[]}`, `still-bad`},
		// first response is valid empty — change to invalid both times
	}
	mock.Responses = []string{`{bad`, `{also-bad`}
	ext, err := extractor.New(mock)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	_, err = ext.Extract(context.Background(), sampleJiraTrace())
	if err == nil {
		t.Fatal("expected error after retry")
	}
	if !strings.Contains(err.Error(), "after retry") {
		t.Fatalf("error = %v", err)
	}
	if len(mock.Calls) != 2 {
		t.Fatalf("calls = %d, want 2", len(mock.Calls))
	}
}

func TestExtractSurfacesLLMError(t *testing.T) {
	t.Parallel()
	mock := &provider.MockLLM{
		Errs: []error{errors.New("upstream down"), errors.New("upstream down")},
	}
	ext, err := extractor.New(mock)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	_, err = ext.Extract(context.Background(), sampleJiraTrace())
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "extract experiences for episode") {
		t.Fatalf("error should include episode context: %v", err)
	}
}

func TestEmbeddedSchemaPresent(t *testing.T) {
	t.Parallel()
	if err := extractor.EnsureSchemaPresent(); err != nil {
		t.Fatal(err)
	}
	raw := extractor.SchemaJSON()
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("schema json: %v", err)
	}
	if doc["title"] != "ExperienceCandidates" {
		t.Fatalf("title = %v", doc["title"])
	}
}

func sampleJiraTrace() extractor.ExtractInput {
	now := time.Now().UTC()
	ep := episode.Episode{
		ID:       "ep_jira_1",
		TenantID: "tenant_a",
		AgentID:  "agent_01",
		UserID:   "user_01",
		TaskType: "jira.create_issue",
		Goal:     "Create Jira issue",
		Input:    `project="Payment"`,
		Status:   episode.StatusSuccess,
	}
	attempts := []attempt.Attempt{
		{
			ID: "a1", EpisodeID: ep.ID, TenantID: ep.TenantID, Sequence: 1,
			Hypothesis: "Payment is project key", Action: "create_issue", ToolName: "jira.create_issue",
			Status: attempt.StatusFailed, ErrorCode: "INVALID_PROJECT_KEY", ErrorMessage: "not found",
			StartedAt: now, CompletedAt: &now,
		},
		{
			ID: "a2", EpisodeID: ep.ID, TenantID: ep.TenantID, Sequence: 2,
			Action: "search_projects", ToolName: "jira.search_projects", Status: attempt.StatusSuccess,
			Output: json.RawMessage(`{"key":"PAY"}`), StartedAt: now, CompletedAt: &now,
		},
		{
			ID: "a3", EpisodeID: ep.ID, TenantID: ep.TenantID, Sequence: 3,
			Action: "create_issue", ToolName: "jira.create_issue", Status: attempt.StatusSuccess,
			Input: json.RawMessage(`{"project":"PAY"}`), StartedAt: now, CompletedAt: &now,
		},
	}
	out := outcome.Outcome{
		ID: "o1", EpisodeID: ep.ID, TenantID: ep.TenantID,
		Status: string(episode.StatusSuccess), Verified: true, Verifier: "tool", CreatedAt: now,
	}
	return extractor.ExtractInput{Episode: ep, Attempts: attempts, Outcome: out}
}

func validJiraExtractionJSON() string {
	return `{
  "experiences": [
    {
      "type": "SEMANTIC",
      "trigger": "referencing the Payment Jira project",
      "content": "The Jira project key for Payment is PAY.",
      "confidence": 0.95,
      "scope": "TOOL",
      "scope_key": "jira"
    },
    {
      "type": "PROCEDURAL",
      "trigger": "create jira issue when project key unknown",
      "content": "Resolve the Jira project key before calling create_issue.",
      "confidence": 0.91,
      "scope": "TOOL",
      "scope_key": "jira"
    },
    {
      "type": "FAILURE",
      "trigger": "creating a Jira issue with a display name as project key",
      "content": "Do not use a Jira project display name as the project key.",
      "confidence": 0.93,
      "scope": "TOOL",
      "scope_key": "jira"
    }
  ]
}`
}
