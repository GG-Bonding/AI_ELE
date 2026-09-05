package feedback_test

import (
	"context"
	"errors"
	"testing"

	"github.com/agent-experience-engine/agent-experience-engine/internal/feedback"
)

func TestValidateTarget(t *testing.T) {
	t.Parallel()

	if err := feedback.ValidateTarget(nil); err != nil {
		t.Fatalf("nil target: %v", err)
	}

	cases := []struct {
		name    string
		target  feedback.Target
		wantErr bool
	}{
		{name: "episode", target: feedback.Target{Type: feedback.TargetEpisode}},
		{name: "action ok", target: feedback.Target{Type: feedback.TargetAction, ActionID: "a1"}},
		{name: "action missing id", target: feedback.Target{Type: feedback.TargetAction}, wantErr: true},
		{name: "action field ok", target: feedback.Target{Type: feedback.TargetActionField, ActionID: "a1", Field: "priority"}},
		{name: "action field missing field", target: feedback.Target{Type: feedback.TargetActionField, ActionID: "a1"}, wantErr: true},
		{name: "tool ok", target: feedback.Target{Type: feedback.TargetTool, ToolName: "jira.create_issue"}},
		{name: "experience ok", target: feedback.Target{Type: feedback.TargetExperience, ExperienceID: "e1"}},
		{name: "skill execution ok", target: feedback.Target{Type: feedback.TargetSkillExecution, SkillVersionID: "sv1"}},
		{name: "skill execution missing version", target: feedback.Target{Type: feedback.TargetSkillExecution}, wantErr: true},
		{name: "bad type", target: feedback.Target{Type: "NOPE"}, wantErr: true},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := feedback.ValidateTarget(&tc.target)
			if tc.wantErr && !errors.Is(err, feedback.ErrInvalidInput) {
				t.Fatalf("err=%v want ErrInvalidInput", err)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected err=%v", err)
			}
		})
	}
}

type stubActionVerifier struct {
	ok  bool
	err error
}

func (s stubActionVerifier) ActionInEpisode(context.Context, string, string, string) (bool, error) {
	return s.ok, s.err
}

func TestSubmitPersistsActionFieldTarget(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	svc := feedback.NewService(
		feedback.NewMemoryRepository(),
		stubEpisodes{exists: map[string]bool{"t1/ep1": true}},
		nil,
	).WithActionVerifier(stubActionVerifier{ok: true})

	reward := -1.0
	res, err := svc.Submit(ctx, feedback.SubmitInput{
		TenantID:  "t1",
		EpisodeID: "ep1",
		Source:    "user_explicit",
		Reward:    &reward,
		Target: &feedback.Target{
			Type:     feedback.TargetActionField,
			ActionID: "action_03",
			Field:    "priority",
		},
	})
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if res.Feedback.Target == nil || res.Feedback.Target.Field != "priority" {
		t.Fatalf("target=%#v", res.Feedback.Target)
	}

	_, rows, err := svc.GetEpisodeReward(ctx, "t1", "ep1")
	if err != nil {
		t.Fatalf("GetEpisodeReward: %v", err)
	}
	if len(rows) != 1 || rows[0].Target == nil || rows[0].Target.ActionID != "action_03" {
		t.Fatalf("rows=%#v", rows)
	}
}

func TestSubmitRejectsUnknownActionWhenVerifierSet(t *testing.T) {
	t.Parallel()
	svc := feedback.NewService(
		feedback.NewMemoryRepository(),
		stubEpisodes{exists: map[string]bool{"t1/ep1": true}},
		nil,
	).WithActionVerifier(stubActionVerifier{ok: false})

	reward := -1.0
	_, err := svc.Submit(context.Background(), feedback.SubmitInput{
		TenantID:  "t1",
		EpisodeID: "ep1",
		Source:    "user_explicit",
		Reward:    &reward,
		Target:    &feedback.Target{Type: feedback.TargetAction, ActionID: "missing"},
	})
	if !errors.Is(err, feedback.ErrNotFound) {
		t.Fatalf("err=%v want ErrNotFound", err)
	}
}
