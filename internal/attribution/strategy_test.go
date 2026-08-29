package attribution_test

import (
	"math"
	"testing"

	"github.com/agent-experience-engine/agent-experience-engine/internal/attribution"
	"github.com/agent-experience-engine/agent-experience-engine/internal/experience"
)

func TestScoreProportionalSingleGetsFullCredit(t *testing.T) {
	t.Parallel()
	credits, err := attribution.ScoreProportional{}.Attribute([]experience.Usage{{
		ExperienceID: "e1", FinalScore: 0.4,
	}}, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(credits) != 1 || credits[0].Weight != 1 {
		t.Fatalf("%#v", credits)
	}
}

func TestScoreProportionalSplitsByScore(t *testing.T) {
	t.Parallel()
	credits, err := attribution.ScoreProportional{}.Attribute([]experience.Usage{
		{ExperienceID: "a", FinalScore: 0.2},
		{ExperienceID: "b", FinalScore: 0.6},
	}, 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := attribution.ValidateCredits(credits); err != nil {
		t.Fatal(err)
	}
	if math.Abs(credits[0].Weight-0.25) > 1e-9 || math.Abs(credits[1].Weight-0.75) > 1e-9 {
		t.Fatalf("%#v", credits)
	}
}
