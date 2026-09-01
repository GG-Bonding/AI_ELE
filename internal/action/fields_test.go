package action_test

import (
	"testing"

	"github.com/agent-experience-engine/agent-experience-engine/internal/action"
)

func TestFieldPathMatches(t *testing.T) {
	t.Parallel()
	cases := []struct {
		target   string
		affected []string
		want     bool
	}{
		{"input.priority", []string{"input.priority"}, true},
		{"priority", []string{"input.priority"}, true},
		{"input.priority", []string{"priority"}, true},
		{"input.priority", []string{"input.project"}, false},
		{"priority", nil, false},
		{"", []string{"input.priority"}, false},
	}
	for _, tc := range cases {
		if got := action.FieldPathMatches(tc.target, tc.affected); got != tc.want {
			t.Fatalf("FieldPathMatches(%q, %v)=%v want %v", tc.target, tc.affected, got, tc.want)
		}
	}
}

func TestNormalizeAffectedFields(t *testing.T) {
	t.Parallel()
	got := action.NormalizeAffectedFields([]string{" Input.Priority ", "input.priority", "", "input.project"})
	if len(got) != 2 || got[0] != "input.priority" || got[1] != "input.project" {
		t.Fatalf("%#v", got)
	}
}
