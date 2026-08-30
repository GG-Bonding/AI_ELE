package feedback

import (
	"fmt"
	"strings"
)

// TargetType classifies what a feedback signal is correcting or confirming.
type TargetType string

const (
	// TargetEpisode is whole-episode feedback (V1 default when Target is nil).
	TargetEpisode TargetType = "EPISODE"
	// TargetAction points at one AgentAction.
	TargetAction TargetType = "ACTION"
	// TargetActionField points at a field inside an action's input/decision.
	TargetActionField TargetType = "ACTION_FIELD"
	// TargetTool points at a tool name (when action id is unknown).
	TargetTool TargetType = "TOOL"
	// TargetExperience points at a specific experience id.
	TargetExperience TargetType = "EXPERIENCE"
)

// Valid reports whether t is a known target type.
func (t TargetType) Valid() bool {
	switch t {
	case TargetEpisode, TargetAction, TargetActionField, TargetTool, TargetExperience:
		return true
	default:
		return false
	}
}

// Target locates what feedback applies to inside an episode.
// This upgrades feedback from "task went poorly" to "this action/field was wrong".
type Target struct {
	Type TargetType `json:"type"`

	ActionID     string `json:"action_id,omitempty"`
	ToolName     string `json:"tool_name,omitempty"`
	Field        string `json:"field,omitempty"`
	ExperienceID string `json:"experience_id,omitempty"`
}

// ValidateTarget checks structural rules for a feedback target.
// nil is allowed (episode-level feedback).
func ValidateTarget(t *Target) error {
	if t == nil {
		return nil
	}
	typ := TargetType(strings.TrimSpace(string(t.Type)))
	if !typ.Valid() {
		return fmt.Errorf("%w: invalid feedback target type %q", ErrInvalidInput, t.Type)
	}
	t.Type = typ
	t.ActionID = strings.TrimSpace(t.ActionID)
	t.ToolName = strings.TrimSpace(t.ToolName)
	t.Field = strings.TrimSpace(t.Field)
	t.ExperienceID = strings.TrimSpace(t.ExperienceID)

	switch typ {
	case TargetEpisode:
		// no extra fields required
	case TargetAction:
		if t.ActionID == "" {
			return fmt.Errorf("%w: action_id is required for ACTION target", ErrInvalidInput)
		}
	case TargetActionField:
		if t.ActionID == "" {
			return fmt.Errorf("%w: action_id is required for ACTION_FIELD target", ErrInvalidInput)
		}
		if t.Field == "" {
			return fmt.Errorf("%w: field is required for ACTION_FIELD target", ErrInvalidInput)
		}
	case TargetTool:
		if t.ToolName == "" {
			return fmt.Errorf("%w: tool_name is required for TOOL target", ErrInvalidInput)
		}
	case TargetExperience:
		if t.ExperienceID == "" {
			return fmt.Errorf("%w: experience_id is required for EXPERIENCE target", ErrInvalidInput)
		}
	}
	return nil
}
