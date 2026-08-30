package experience

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"
)

// DefaultAuthorityMargin is the minimum authority gap required to auto-supersede.
// Below this margin both experiences stay ACTIVE with an unresolved CONFLICTS edge.
const DefaultAuthorityMargin = 0.12

// ConflictResolutionKind is the outcome of authority-based conflict resolution (V2-6).
type ConflictResolutionKind string

const (
	ConflictUnresolved ConflictResolutionKind = "UNRESOLVED"
	ConflictSuperseded ConflictResolutionKind = "SUPERSEDED"
)

// ConflictResolution reports how a store-time polarity conflict was handled.
type ConflictResolution struct {
	Kind             ConflictResolutionKind `json:"kind"`
	WinnerID         string                 `json:"winner_id,omitempty"`
	LoserID          string                 `json:"loser_id,omitempty"`
	WinnerAuthority  float64                `json:"winner_authority,omitempty"`
	LoserAuthority   float64                `json:"loser_authority,omitempty"`
	Relation         ExperienceRelation     `json:"relation"`
}

// AuthorityScore estimates how trustworthy an experience is for supersession decisions.
//
//	Authority ≈ Evidence + SuccessRatio + Confidence + Utility + Freshness
//
// It intentionally does not treat Utility alone as truth — fresh low-utility evidence
// can still outrank a stale high-utility conflicting rule when the margin is clear.
func AuthorityScore(exp Experience, now time.Time) float64 {
	evidence := clamp01(float64(exp.Evidence.SupportCount()) / 5.0)

	success := float64(exp.Evidence.SuccessAttemptCount)
	failed := float64(exp.Evidence.FailedAttemptCount)
	successRatio := 0.5
	if success+failed > 0 {
		successRatio = success / (success + failed)
	}
	if exp.Evidence.HasFailureContrast {
		successRatio = clamp01(successRatio + 0.05)
	}

	freshness := 1.0
	ref := exp.UpdatedAt
	if ref.IsZero() {
		ref = exp.CreatedAt
	}
	if !ref.IsZero() && !now.IsZero() {
		ageDays := now.Sub(ref).Hours() / 24
		if ageDays < 0 {
			ageDays = 0
		}
		freshness = math.Exp(-ageDays / 90.0)
	}

	return clamp01(
		0.30*evidence +
			0.20*successRatio +
			0.25*clamp01(exp.Confidence) +
			0.15*clamp01(exp.Utility) +
			0.10*freshness,
	)
}

// ResolveConflict compares authorities of two conflicting experiences.
// Clear winner → SUPERSEDES (loser DEPRECATED). Unclear → CONFLICTS (both ACTIVE).
func (s *Service) ResolveConflict(ctx context.Context, tenantID, leftID, rightID string, similarity float64) (ConflictResolution, error) {
	if err := requireNonEmpty("tenant_id", tenantID, "left_id", leftID, "right_id", rightID); err != nil {
		return ConflictResolution{}, err
	}
	leftID = strings.TrimSpace(leftID)
	rightID = strings.TrimSpace(rightID)
	if leftID == rightID {
		return ConflictResolution{}, fmt.Errorf("%w: conflict endpoints must differ", ErrInvalidInput)
	}

	left, err := s.repo.Get(ctx, tenantID, leftID)
	if err != nil {
		return ConflictResolution{}, fmt.Errorf("get conflict left %s: %w", leftID, err)
	}
	right, err := s.repo.Get(ctx, tenantID, rightID)
	if err != nil {
		return ConflictResolution{}, fmt.Errorf("get conflict right %s: %w", rightID, err)
	}

	now := s.now()
	leftAuth := AuthorityScore(left, now)
	rightAuth := AuthorityScore(right, now)
	margin := math.Abs(leftAuth - rightAuth)

	if margin < DefaultAuthorityMargin {
		rel, err := s.RecordConflict(ctx, tenantID, RecordConflictInput{
			FromExperienceID: leftID,
			ToExperienceID:   rightID,
			Confidence:       similarity,
			Reason:           fmt.Sprintf("unresolved conflict: authority gap %.3f < %.3f", margin, DefaultAuthorityMargin),
		})
		if err != nil {
			return ConflictResolution{}, err
		}
		return ConflictResolution{
			Kind:            ConflictUnresolved,
			WinnerAuthority: math.Max(leftAuth, rightAuth),
			LoserAuthority:  math.Min(leftAuth, rightAuth),
			Relation:        rel,
		}, nil
	}

	winnerID, loserID := leftID, rightID
	winnerAuth, loserAuth := leftAuth, rightAuth
	if rightAuth > leftAuth {
		winnerID, loserID = rightID, leftID
		winnerAuth, loserAuth = rightAuth, leftAuth
	}

	if err := s.Supersede(ctx, tenantID, loserID, winnerID); err != nil {
		return ConflictResolution{}, fmt.Errorf("supersede %s with %s: %w", loserID, winnerID, err)
	}
	rel, err := s.recordSupersedes(ctx, tenantID, winnerID, loserID, similarity, winnerAuth, loserAuth)
	if err != nil {
		return ConflictResolution{}, err
	}
	return ConflictResolution{
		Kind:            ConflictSuperseded,
		WinnerID:        winnerID,
		LoserID:         loserID,
		WinnerAuthority: winnerAuth,
		LoserAuthority:  loserAuth,
		Relation:        rel,
	}, nil
}

func (s *Service) recordSupersedes(ctx context.Context, tenantID, winnerID, loserID string, similarity, winnerAuth, loserAuth float64) (ExperienceRelation, error) {
	if s.relations == nil {
		return ExperienceRelation{}, nil
	}
	confidence := similarity
	if confidence <= 0 {
		confidence = clamp01(winnerAuth - loserAuth)
	}
	if confidence > 1 {
		confidence = 1
	}
	rel := ExperienceRelation{
		ID:               s.id(),
		TenantID:         strings.TrimSpace(tenantID),
		FromExperienceID: winnerID,
		ToExperienceID:   loserID,
		Type:             RelationSupersedes,
		Confidence:       confidence,
		Reason:           fmt.Sprintf("authority %.3f supersedes %.3f", winnerAuth, loserAuth),
		CreatedAt:        s.now(),
	}
	saved, err := s.relations.Upsert(ctx, rel)
	if err != nil {
		return ExperienceRelation{}, fmt.Errorf("upsert supersedes relation: %w", err)
	}
	return saved, nil
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}
