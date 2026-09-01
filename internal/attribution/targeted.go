package attribution

import (
	"github.com/agent-experience-engine/agent-experience-engine/internal/action"
)

// Targeted attributes reward using FeedbackTarget + ExperienceActionLink evidence.
// Falls back to ScoreProportional only for episode-level (untargeted) feedback.
// Precise targets with no matching evidence return no credits (fail closed).
type Targeted struct {
	Fallback Strategy
}

// NewDefault returns the V2 attribution strategy (target/link-aware).
func NewDefault() Strategy {
	return Targeted{Fallback: ScoreProportional{}}
}

// NewV1 returns score-proportional attribution only.
func NewV1() Strategy {
	return ScoreProportional{}
}

func (t Targeted) fallback() Strategy {
	if t.Fallback == nil {
		return ScoreProportional{}
	}
	return t.Fallback
}

// Attribute implements Strategy.
func (t Targeted) Attribute(req Request) ([]Credit, error) {
	if req.Target == nil || trim(req.Target.Type) == "" || trim(req.Target.Type) == "EPISODE" {
		return t.fallback().Attribute(req)
	}

	switch trim(req.Target.Type) {
	case "EXPERIENCE":
		return creditExactExperience(req)
	case "ACTION":
		return creditByAction(req, trim(req.Target.ActionID))
	case "ACTION_FIELD":
		return creditByActionField(req, trim(req.Target.ActionID), trim(req.Target.Field))
	case "TOOL":
		return creditByTool(req, trim(req.Target.ToolName))
	default:
		// Unknown target type: fail closed (do not spread blame by retrieval score).
		return nil, nil
	}
}

func creditExactExperience(req Request) ([]Credit, error) {
	id := trim(req.Target.ExperienceID)
	if id == "" {
		return nil, nil
	}
	for _, u := range req.Usages {
		if u.ExperienceID == id {
			return []Credit{{ExperienceID: id, Weight: 1, Score: 1}}, nil
		}
	}
	return nil, nil
}

func creditByAction(req Request, actionID string) ([]Credit, error) {
	if actionID == "" {
		return nil, nil
	}
	used := usageSet(req.Usages)
	weights := map[string]float64{}
	for _, link := range req.Links {
		if trim(link.ActionID) != actionID {
			continue
		}
		expID := trim(link.ExperienceID)
		if _, ok := used[expID]; !ok {
			continue
		}
		inf := link.Influence
		if inf <= 0 {
			inf = 1
		}
		weights[expID] += inf
	}
	return normalizeInfluenceCredits(weights), nil
}

// creditByActionField assigns credit only to experiences whose link declares the target field
// in AffectedFields (V2.1). Links with empty AffectedFields do not match ACTION_FIELD targets.
func creditByActionField(req Request, actionID, field string) ([]Credit, error) {
	if actionID == "" || field == "" {
		return nil, nil
	}
	used := usageSet(req.Usages)
	weights := map[string]float64{}
	for _, link := range req.Links {
		if trim(link.ActionID) != actionID {
			continue
		}
		if !action.FieldPathMatches(field, link.AffectedFields) {
			continue
		}
		expID := trim(link.ExperienceID)
		if _, ok := used[expID]; !ok {
			continue
		}
		inf := link.Influence
		if inf <= 0 {
			inf = 1
		}
		weights[expID] += inf
	}
	return normalizeInfluenceCredits(weights), nil
}

func creditByTool(req Request, toolName string) ([]Credit, error) {
	if toolName == "" {
		return nil, nil
	}
	used := usageSet(req.Usages)
	weights := map[string]float64{}
	for _, link := range req.Links {
		if trim(link.ToolName) != toolName {
			continue
		}
		expID := trim(link.ExperienceID)
		if _, ok := used[expID]; !ok {
			continue
		}
		inf := link.Influence
		if inf <= 0 {
			inf = 1
		}
		weights[expID] += inf
	}
	return normalizeInfluenceCredits(weights), nil
}

var _ Strategy = Targeted{}
