package replay

import (
	"context"

	"github.com/agent-experience-engine/agent-experience-engine/internal/skill"
)

// Task is a replay workload unit.
type Task struct {
	ID     string
	Prompt string
	Inputs map[string]any
	Tools  []string
}

// Result is one arm outcome.
type Result struct {
	Success bool
	Steps   int
	Reward  float64
	Notes   string
}

// Environment replays a task under with-skill vs without-skill policies (V3.3).
type Environment interface {
	Name() string
	ReplayWithSkill(ctx context.Context, task Task, skillYAML string) (Result, error)
	ReplayWithoutSkill(ctx context.Context, task Task, patternTips []string) (Result, error)
}

// Compare runs matched arms and returns ACE.
func Compare(ctx context.Context, env Environment, task Task, skillYAML string, tips []string) (skill.CounterfactualResult, error) {
	with, err := env.ReplayWithSkill(ctx, task, skillYAML)
	if err != nil {
		return skill.CounterfactualResult{}, err
	}
	without, err := env.ReplayWithoutSkill(ctx, task, tips)
	if err != nil {
		return skill.CounterfactualResult{}, err
	}
	return skill.CompareReplayArms(
		skill.ReplayArm{Success: with.Success, Steps: with.Steps, Reward: with.Reward},
		skill.ReplayArm{Success: without.Success, Steps: without.Steps, Reward: without.Reward},
	), nil
}
