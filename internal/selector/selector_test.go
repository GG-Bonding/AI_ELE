package selector_test

import (
	"strings"
	"testing"
	"time"

	"github.com/agent-experience-engine/agent-experience-engine/internal/experience"
	"github.com/agent-experience-engine/agent-experience-engine/internal/retrieval"
	"github.com/agent-experience-engine/agent-experience-engine/internal/selector"
)

func ranked(id, content string, score retrieval.ScoreBreakdown, opts ...func(*experience.Experience)) retrieval.RankedExperience {
	exp := experience.Experience{
		ID: id, Type: experience.TypeProcedural, Scope: experience.ScopeTool, ScopeKey: "jira",
		Trigger: "create jira issue", Content: content,
		Confidence: 0.9, Utility: 0.8, Status: experience.StatusActive,
		UpdatedAt: time.Now().UTC(),
	}
	for _, opt := range opts {
		opt(&exp)
	}
	return retrieval.RankedExperience{Experience: exp, Score: score}
}

func TestSelectorKeepRelevant(t *testing.T) {
	t.Parallel()
	sel := selector.New(selector.DefaultConfig())
	got := sel.Select("Create a Jira issue for payment", []retrieval.RankedExperience{
		ranked("e1", "Resolve project key before create_issue.", retrieval.ScoreBreakdown{
			Similarity: 0.8, Utility: 0.8, Confidence: 0.9, Freshness: 1, ScopeMatch: 1, FinalScore: 0.5,
		}),
	})
	if got[0].Decision != selector.DecisionKeep {
		t.Fatalf("decision=%s reason=%s", got[0].Decision, got[0].Reason)
	}
}

func TestSelectorIgnoreIrrelevant(t *testing.T) {
	t.Parallel()
	sel := selector.New(selector.DefaultConfig())
	got := sel.Select("deploy kubernetes cluster", []retrieval.RankedExperience{
		ranked("e1", "Resolve Jira project key before create_issue.", retrieval.ScoreBreakdown{
			Similarity: 0.2, Utility: 0.8, Confidence: 0.9, Freshness: 1, ScopeMatch: 0.9, FinalScore: 0.2,
		}),
	})
	if got[0].Decision != selector.DecisionIgnore {
		t.Fatalf("decision=%s reason=%s", got[0].Decision, got[0].Reason)
	}
}

func TestSelectorBlockLowUtility(t *testing.T) {
	t.Parallel()
	sel := selector.New(selector.DefaultConfig())
	got := sel.Select("Create Jira issue", []retrieval.RankedExperience{
		ranked("e1", "Resolve project key first", retrieval.ScoreBreakdown{
			Similarity: 0.9, Utility: 0.1, Confidence: 0.9, Freshness: 1, ScopeMatch: 1, FinalScore: 0.08,
		}, func(e *experience.Experience) { e.Utility = 0.1 }),
	})
	if got[0].Decision != selector.DecisionBlock {
		t.Fatalf("decision=%s reason=%s", got[0].Decision, got[0].Reason)
	}
}

func TestSelectorAbstractEpisodeSpecific(t *testing.T) {
	t.Parallel()
	sel := selector.New(selector.DefaultConfig())
	long := "In episode ep_abc123456 when creating PAY-999 we learned that " + strings.Repeat("detail ", 40)
	got := sel.Select("Create Jira issue", []retrieval.RankedExperience{
		ranked("e1", long, retrieval.ScoreBreakdown{
			Similarity: 0.85, Utility: 0.8, Confidence: 0.9, Freshness: 1, ScopeMatch: 1, FinalScore: 0.5,
		}),
	})
	if got[0].Decision != selector.DecisionAbstract {
		t.Fatalf("decision=%s reason=%s", got[0].Decision, got[0].Reason)
	}
	if strings.Contains(got[0].Content, "ep_abc123456") {
		t.Fatalf("abstracted content still has episode id: %s", got[0].Content)
	}
}
