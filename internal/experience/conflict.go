package experience

import (
	"context"
	"fmt"
	"strings"
)

// RecordConflictInput records an unresolved CONFLICTS relation between two experiences.
// Used when authority scores are too close to auto-supersede (V2-6).
// Selector fail-closes by not auto-injecting either ACTIVE side.
type RecordConflictInput struct {
	FromExperienceID string
	ToExperienceID   string
	Confidence       float64
	Reason           string
}

// WithRelations attaches a relation store used for conflict detection (V2-5).
func (s *Service) WithRelations(repo RelationRepository) *Service {
	s.relations = repo
	return s
}

// RecordConflict upserts a CONFLICTS edge. No-op when relations are not configured.
func (s *Service) RecordConflict(ctx context.Context, tenantID string, in RecordConflictInput) (ExperienceRelation, error) {
	if s.relations == nil {
		return ExperienceRelation{}, nil
	}
	if err := requireNonEmpty("tenant_id", tenantID, "from_experience_id", in.FromExperienceID, "to_experience_id", in.ToExperienceID); err != nil {
		return ExperienceRelation{}, err
	}
	fromID := strings.TrimSpace(in.FromExperienceID)
	toID := strings.TrimSpace(in.ToExperienceID)
	if fromID == toID {
		return ExperienceRelation{}, fmt.Errorf("%w: conflict endpoints must differ", ErrInvalidInput)
	}
	confidence := in.Confidence
	if confidence <= 0 {
		confidence = 1
	}
	if confidence > 1 {
		confidence = 1
	}
	rel := ExperienceRelation{
		ID:               s.id(),
		TenantID:         strings.TrimSpace(tenantID),
		FromExperienceID: fromID,
		ToExperienceID:   toID,
		Type:             RelationConflicts,
		Confidence:       confidence,
		Reason:           strings.TrimSpace(in.Reason),
		CreatedAt:        s.now(),
	}
	saved, err := s.relations.Upsert(ctx, rel)
	if err != nil {
		return ExperienceRelation{}, fmt.Errorf("upsert conflict relation: %w", err)
	}
	return saved, nil
}

// ConflictPeers returns unresolved CONFLICTS peers for the given experience IDs.
// Pairs involving a non-retrievable (e.g. DEPRECATED) experience are omitted so
// supersession winners are not blocked from context.
// Returns nil when relations are not configured.
func (s *Service) ConflictPeers(ctx context.Context, tenantID string, experienceIDs []string) (map[string]string, error) {
	if s.relations == nil {
		return nil, nil
	}
	if err := requireNonEmpty("tenant_id", tenantID); err != nil {
		return nil, err
	}
	peers, err := s.relations.ConflictPeers(ctx, tenantID, experienceIDs)
	if err != nil {
		return nil, fmt.Errorf("list conflict peers: %w", err)
	}
	if len(peers) == 0 {
		return peers, nil
	}
	out := make(map[string]string, len(peers))
	for id, peerID := range peers {
		a, errA := s.repo.Get(ctx, tenantID, id)
		b, errB := s.repo.Get(ctx, tenantID, peerID)
		if errA != nil || errB != nil {
			continue
		}
		if !a.Status.Retrievable() || !b.Status.Retrievable() {
			continue
		}
		out[id] = peerID
	}
	return out, nil
}

// ListRelations returns all relations involving an experience.
func (s *Service) ListRelations(ctx context.Context, tenantID, experienceID string) ([]ExperienceRelation, error) {
	if s.relations == nil {
		return nil, nil
	}
	if err := requireNonEmpty("tenant_id", tenantID, "experience_id", experienceID); err != nil {
		return nil, err
	}
	rels, err := s.relations.ListByExperience(ctx, tenantID, experienceID)
	if err != nil {
		return nil, fmt.Errorf("list relations for experience %s: %w", experienceID, err)
	}
	return rels, nil
}
