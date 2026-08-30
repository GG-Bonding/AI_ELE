package experience

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// ReinforceInput adds corroborating evidence to an existing experience without changing Utility.
type ReinforceInput struct {
	EpisodeID  string
	Evidence   Evidence
	Confidence float64 // incoming candidate quality ∈ [0,1]
}

// Reinforce merges episode evidence into an existing experience and raises Confidence.
// Utility / alpha / beta are intentionally unchanged (V2-4).
func (s *Service) Reinforce(ctx context.Context, tenantID, experienceID string, in ReinforceInput) (Experience, error) {
	if err := requireNonEmpty("tenant_id", tenantID, "experience_id", experienceID, "episode_id", in.EpisodeID); err != nil {
		return Experience{}, err
	}
	if in.Confidence < 0 || in.Confidence > 1 {
		return Experience{}, fmt.Errorf("%w: confidence out of range", ErrInvalidInput)
	}

	const maxAttempts = 5
	for attempt := 0; attempt < maxAttempts; attempt++ {
		exp, err := s.repo.Get(ctx, tenantID, experienceID)
		if err != nil {
			return Experience{}, fmt.Errorf("reinforce get experience %s: %w", experienceID, err)
		}
		utilityBefore := exp.Utility
		alphaBefore, betaBefore := exp.Alpha, exp.Beta

		exp.Evidence = mergeEvidence(exp.Evidence, in.Evidence, strings.TrimSpace(in.EpisodeID))
		exp.Confidence = bumpConfidence(exp.Confidence, in.Confidence)
		exp.UpdatedAt = s.now()

		updated, err := s.repo.Update(ctx, exp)
		if err != nil {
			if errors.Is(err, ErrConflict) {
				continue
			}
			return Experience{}, fmt.Errorf("reinforce update experience %s: %w", experienceID, err)
		}
		// Defensive: never let a buggy Update path mutate utility during reinforce.
		if updated.Utility != utilityBefore || updated.Alpha != alphaBefore || updated.Beta != betaBefore {
			updated.Utility = utilityBefore
			updated.Alpha = alphaBefore
			updated.Beta = betaBefore
		}
		return updated, nil
	}
	return Experience{}, fmt.Errorf("reinforce experience %s: %w", experienceID, ErrConflict)
}

func bumpConfidence(current, incoming float64) float64 {
	base := current
	if incoming > base {
		base = incoming
	}
	// Asymptotic evidence bump toward 1.0 without treating confidence as utility.
	bumped := base + (1-base)*0.15
	if bumped > 1 {
		return 1
	}
	if bumped < 0 {
		return 0
	}
	return bumped
}

func mergeEvidence(dst, src Evidence, episodeID string) Evidence {
	out := dst
	if out.SourceEpisodeID == "" {
		out.SourceEpisodeID = src.SourceEpisodeID
	}
	if out.SourceEpisodeID == "" {
		out.SourceEpisodeID = episodeID
	}
	out.SupportEpisodeIDs = appendSupportEpisode(out.SupportEpisodeIDs, out.SourceEpisodeID)
	out.SupportEpisodeIDs = appendSupportEpisode(out.SupportEpisodeIDs, episodeID)
	if src.SourceEpisodeID != "" {
		out.SupportEpisodeIDs = appendSupportEpisode(out.SupportEpisodeIDs, src.SourceEpisodeID)
	}
	for _, id := range src.SupportEpisodeIDs {
		out.SupportEpisodeIDs = appendSupportEpisode(out.SupportEpisodeIDs, id)
	}

	out.FailedAttemptCount += src.FailedAttemptCount
	out.SuccessAttemptCount += src.SuccessAttemptCount
	out.HasFailureContrast = out.HasFailureContrast || src.HasFailureContrast
	out.HasToolErrorCode = out.HasToolErrorCode || src.HasToolErrorCode
	if out.OutcomeID == "" {
		out.OutcomeID = src.OutcomeID
	}
	out.AttemptIDs = appendUniqueStrings(out.AttemptIDs, src.AttemptIDs...)
	return out
}

func appendSupportEpisode(ids []string, episodeID string) []string {
	episodeID = strings.TrimSpace(episodeID)
	if episodeID == "" {
		return ids
	}
	return appendUniqueStrings(ids, episodeID)
}

func appendUniqueStrings(dst []string, add ...string) []string {
	seen := make(map[string]struct{}, len(dst)+len(add))
	out := make([]string, 0, len(dst)+len(add))
	for _, s := range dst {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	for _, s := range add {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}
