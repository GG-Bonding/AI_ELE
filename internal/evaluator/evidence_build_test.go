package evaluator_test

import (
	"testing"

	"github.com/agent-experience-engine/agent-experience-engine/internal/attempt"
	"github.com/agent-experience-engine/agent-experience-engine/internal/evaluator"
)

func TestFromAttemptsBuildsEvidence(t *testing.T) {
	t.Parallel()
	ev := evaluator.FromAttempts("ep1", "out1", []attempt.Attempt{
		{ID: "a1", Status: attempt.StatusFailed, ErrorCode: "INVALID_PROJECT_KEY"},
		{ID: "a2", Status: attempt.StatusSuccess},
		{ID: "a3", Status: attempt.StatusSuccess},
	})
	if ev.FailedAttemptCount != 1 || ev.SuccessAttemptCount != 2 {
		t.Fatalf("counts: %#v", ev)
	}
	if !ev.HasFailureContrast || !ev.HasToolErrorCode {
		t.Fatalf("contrast/tool flags: %#v", ev)
	}
	if ev.SourceEpisodeID != "ep1" || ev.OutcomeID != "out1" {
		t.Fatalf("ids: %#v", ev)
	}
	if len(ev.AttemptIDs) != 3 {
		t.Fatalf("attempt ids: %#v", ev.AttemptIDs)
	}
}
