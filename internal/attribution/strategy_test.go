package attribution_test

import (
	"testing"

	"github.com/agent-experience-engine/agent-experience-engine/internal/attribution"
	"github.com/agent-experience-engine/agent-experience-engine/internal/experience"
)

func TestScoreProportionalSingleGetsFullCredit(t *testing.T) {
	t.Parallel()
	credits, err := attribution.ScoreProportional{}.Attribute(attribution.Request{
		Usages: []experience.Usage{{ExperienceID: "e1", FinalScore: 0.8}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(credits) != 1 || credits[0].Weight != 1 || credits[0].ExperienceID != "e1" {
		t.Fatalf("%#v", credits)
	}
}

func TestScoreProportionalSplitsByScore(t *testing.T) {
	t.Parallel()
	credits, err := attribution.ScoreProportional{}.Attribute(attribution.Request{
		Usages: []experience.Usage{
			{ExperienceID: "e1", FinalScore: 1},
			{ExperienceID: "e2", FinalScore: 3},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := attribution.ValidateCredits(credits); err != nil {
		t.Fatal(err)
	}
	byID := map[string]float64{}
	for _, c := range credits {
		byID[c.ExperienceID] = c.Weight
	}
	if byID["e1"] != 0.25 || byID["e2"] != 0.75 {
		t.Fatalf("%#v", byID)
	}
}

func TestTargetedActionFieldCreditsOnlyMatchingField(t *testing.T) {
	t.Parallel()
	credits, err := attribution.NewDefault().Attribute(attribution.Request{
		Usages: []experience.Usage{
			{ExperienceID: "e1", FinalScore: 10}, // high score, wrong field
			{ExperienceID: "e2", FinalScore: 1},  // matching field
			{ExperienceID: "e3", FinalScore: 1},  // wrong field
		},
		Target: &attribution.TargetHint{Type: "ACTION_FIELD", ActionID: "a2", Field: "input.priority"},
		Links: []attribution.LinkHint{
			{ExperienceID: "e1", ActionID: "a2", Influence: 1, AffectedFields: []string{"input.project"}},
			{ExperienceID: "e2", ActionID: "a2", Influence: 0.95, AffectedFields: []string{"input.priority"}},
			{ExperienceID: "e3", ActionID: "a2", Influence: 1, AffectedFields: []string{"input.description"}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(credits) != 1 || credits[0].ExperienceID != "e2" || credits[0].Weight != 1 {
		t.Fatalf("want only e2@1, got %#v", credits)
	}
}

func TestTargetedActionFieldEmptyAffectedFieldsFailsClosed(t *testing.T) {
	t.Parallel()
	credits, err := attribution.NewDefault().Attribute(attribution.Request{
		Usages: []experience.Usage{{ExperienceID: "e3", FinalScore: 1}},
		Target: &attribution.TargetHint{Type: "ACTION_FIELD", ActionID: "a2", Field: "priority"},
		Links: []attribution.LinkHint{
			{ExperienceID: "e3", ActionID: "a2", Influence: 0.95}, // no AffectedFields
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(credits) != 0 {
		t.Fatalf("want no credits without AffectedFields, got %#v", credits)
	}
}

func TestTargetedActionStillCreditsWithoutAffectedFields(t *testing.T) {
	t.Parallel()
	credits, err := attribution.NewDefault().Attribute(attribution.Request{
		Usages: []experience.Usage{
			{ExperienceID: "e1", FinalScore: 10},
			{ExperienceID: "e2", FinalScore: 1},
		},
		Target: &attribution.TargetHint{Type: "ACTION", ActionID: "a2"},
		Links: []attribution.LinkHint{
			{ExperienceID: "e2", ActionID: "a2", Influence: 1},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(credits) != 1 || credits[0].ExperienceID != "e2" {
		t.Fatalf("want e2, got %#v", credits)
	}
}

func TestTargetedExperienceExact(t *testing.T) {
	t.Parallel()
	credits, err := attribution.NewDefault().Attribute(attribution.Request{
		Usages: []experience.Usage{
			{ExperienceID: "e1", FinalScore: 5},
			{ExperienceID: "e2", FinalScore: 5},
		},
		Target: &attribution.TargetHint{Type: "EXPERIENCE", ExperienceID: "e2"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(credits) != 1 || credits[0].ExperienceID != "e2" {
		t.Fatalf("%#v", credits)
	}
}

func TestTargetedActionNoLinkFailsClosed(t *testing.T) {
	t.Parallel()
	credits, err := attribution.NewDefault().Attribute(attribution.Request{
		Usages: []experience.Usage{{ExperienceID: "e1", FinalScore: 1}},
		Target: &attribution.TargetHint{Type: "ACTION", ActionID: "missing"},
		Links:  nil,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(credits) != 0 {
		t.Fatalf("want no credits, got %#v", credits)
	}
}

func TestTargetedToolSplitsByInfluence(t *testing.T) {
	t.Parallel()
	credits, err := attribution.NewDefault().Attribute(attribution.Request{
		Usages: []experience.Usage{
			{ExperienceID: "e1", FinalScore: 1},
			{ExperienceID: "e2", FinalScore: 1},
		},
		Target: &attribution.TargetHint{Type: "TOOL", ToolName: "jira.create_issue"},
		Links: []attribution.LinkHint{
			{ExperienceID: "e1", ActionID: "a1", Influence: 1, ToolName: "jira.create_issue"},
			{ExperienceID: "e2", ActionID: "a1", Influence: 3, ToolName: "jira.create_issue"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := attribution.ValidateCredits(credits); err != nil {
		t.Fatal(err)
	}
	byID := map[string]float64{}
	for _, c := range credits {
		byID[c.ExperienceID] = c.Weight
	}
	if byID["e1"] != 0.25 || byID["e2"] != 0.75 {
		t.Fatalf("%#v", byID)
	}
}

func TestUntargetedFallsBackToScore(t *testing.T) {
	t.Parallel()
	credits, err := attribution.NewDefault().Attribute(attribution.Request{
		Usages: []experience.Usage{
			{ExperienceID: "e1", FinalScore: 1},
			{ExperienceID: "e2", FinalScore: 1},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := attribution.ValidateCredits(credits); err != nil {
		t.Fatal(err)
	}
	if len(credits) != 2 {
		t.Fatalf("%#v", credits)
	}
}
