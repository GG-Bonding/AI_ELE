package attribution

import (
	"github.com/agent-experience-engine/agent-experience-engine/internal/experience"
)

// TargetHint is a feedback targeting hint without importing the feedback package.
type TargetHint struct {
	Type         string // EPISODE | ACTION | ACTION_FIELD | TOOL | EXPERIENCE
	ActionID     string
	ToolName     string
	Field        string
	ExperienceID string
}

// LinkHint is an experience→action influence edge for attribution.
type LinkHint struct {
	ExperienceID   string
	ActionID       string
	Influence      float64
	ToolName       string   // optional; filled when action tool_name is known
	AffectedFields []string // optional JSON paths for ACTION_FIELD matching (V2.1)
}

// Request is the input to an attribution Strategy.
type Request struct {
	Usages        []experience.Usage
	EpisodeReward float64
	Target        *TargetHint
	Links         []LinkHint
}

// Credit is the share of episode reward assigned to one experience.
type Credit struct {
	ExperienceID string
	Weight       float64 // normalized credit in (0,1], sums to 1 across credits
	Score        float64 // raw score / influence used for attribution
}

// Strategy assigns episode-level reward credit across used experiences.
type Strategy interface {
	Attribute(req Request) ([]Credit, error)
}
