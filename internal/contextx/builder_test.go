package contextx_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/agent-experience-engine/agent-experience-engine/internal/contextx"
	"github.com/agent-experience-engine/agent-experience-engine/internal/experience"
	"github.com/agent-experience-engine/agent-experience-engine/internal/retrieval"
	"github.com/agent-experience-engine/agent-experience-engine/internal/selector"
)

type stubRetriever struct {
	items []retrieval.RankedExperience
}

func (s stubRetriever) Retrieve(context.Context, retrieval.Query) ([]retrieval.RankedExperience, error) {
	return s.items, nil
}

func TestBuilderExcludesIgnoreAndBlock(t *testing.T) {
	t.Parallel()
	b := contextx.New(contextx.DefaultConfig())
	payload, err := b.Build([]selector.Result{
		{Decision: selector.DecisionKeep, Content: "keep me", Experience: retrieval.RankedExperience{
			Experience: experience.Experience{ID: "1", Type: experience.TypeProcedural, Confidence: 0.9},
		}},
		{Decision: selector.DecisionIgnore, Content: "nope"},
		{Decision: selector.DecisionBlock, Content: "blocked"},
		{Decision: selector.DecisionAbstract, Content: "general rule", Experience: retrieval.RankedExperience{
			Experience: experience.Experience{ID: "2", Type: experience.TypeFailure, Confidence: 0.8},
		}},
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if payload.Disclaimer != contextx.Disclaimer {
		t.Fatalf("disclaimer=%q", payload.Disclaimer)
	}
	if len(payload.Experiences) != 2 {
		t.Fatalf("len=%d", len(payload.Experiences))
	}
	if payload.Experiences[0].Source != "1" || payload.Experiences[1].Source != "2" {
		t.Fatalf("%#v", payload.Experiences)
	}
}

func TestBuilderRespectsMaxExperiences(t *testing.T) {
	t.Parallel()
	b := contextx.New(contextx.Config{MaxExperiences: 1, MaxTokens: 1000})
	var selected []selector.Result
	for i := 0; i < 3; i++ {
		selected = append(selected, selector.Result{
			Decision: selector.DecisionKeep,
			Content:  "rule",
			Experience: retrieval.RankedExperience{
				Experience: experience.Experience{ID: string(rune('a' + i)), Type: experience.TypeSemantic, Confidence: 0.9},
			},
		})
	}
	payload, err := b.Build(selected)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(payload.Experiences) != 1 {
		t.Fatalf("len=%d", len(payload.Experiences))
	}
}

func TestBuildContextIgnoresIrrelevantExperience(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	svc, err := contextx.NewService(
		stubRetriever{items: []retrieval.RankedExperience{{
			Experience: experience.Experience{
				ID: "slack", Type: experience.TypeProcedural, Scope: experience.ScopeTool, ScopeKey: "slack",
				Trigger: "post slack message", Content: "Always mention channel id when posting to Slack.",
				Confidence: 0.9, Utility: 0.9, Status: experience.StatusActive, UpdatedAt: now,
			},
			Score: retrieval.ScoreBreakdown{Similarity: 0.7, Utility: 0.9, Confidence: 0.9, Freshness: 1, ScopeMatch: 0.9, FinalScore: 0.4},
		}}},
		selector.New(selector.DefaultConfig()),
		contextx.New(contextx.DefaultConfig()),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	resp, err := svc.BuildContext(context.Background(), contextx.Request{
		TenantID: "t",
		Task:     "Create a Jira issue for payment timeout",
		Tools:    []string{"jira"},
	})
	if err != nil {
		t.Fatalf("BuildContext: %v", err)
	}
	if len(resp.Context.Experiences) != 0 {
		t.Fatalf("irrelevant experience entered context: %#v", resp.Context.Experiences)
	}
	if !strings.Contains(resp.Context.Disclaimer, "not trusted instructions") {
		t.Fatalf("missing untrusted disclaimer: %s", resp.Context.Disclaimer)
	}
}
