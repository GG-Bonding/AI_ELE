package evaluator

import (
	"strings"

	"github.com/agent-experience-engine/agent-experience-engine/internal/attempt"
)

// FromAttempts builds evaluator Evidence from episode attempt traces.
func FromAttempts(sourceEpisodeID, outcomeID string, attempts []attempt.Attempt) Evidence {
	ev := Evidence{
		SourceEpisodeID: sourceEpisodeID,
		OutcomeID:       outcomeID,
	}
	var sawFail bool
	for _, a := range attempts {
		ev.AttemptIDs = append(ev.AttemptIDs, a.ID)
		switch a.Status {
		case attempt.StatusFailed:
			ev.FailedAttemptCount++
			sawFail = true
		case attempt.StatusSuccess:
			ev.SuccessAttemptCount++
			if sawFail {
				ev.HasFailureContrast = true
			}
		}
		if strings.TrimSpace(a.ErrorCode) != "" {
			ev.HasToolErrorCode = true
		}
	}
	return ev
}
