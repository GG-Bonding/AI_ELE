package experience

import (
	"context"
	"fmt"
	"strings"
)

// WithPatterns attaches a pattern store used for generalization (V2-7).
func (s *Service) WithPatterns(repo PatternRepository) *Service {
	s.patterns = repo
	return s
}

// WithGeneralizer overrides the default HeuristicPatternGeneralizer.
func (s *Service) WithGeneralizer(g PatternGeneralizer) *Service {
	s.generalizer = g
	return s
}

// GeneralizeInput requests a Pattern from a cluster of experience IDs.
type GeneralizeInput struct {
	ExperienceIDs []string
}

// GeneralizeResult is the outcome of a generalization attempt.
type GeneralizeResult struct {
	Created bool
	Pattern Pattern
	Skipped string // reason when Created=false and no error
}

// Generalize builds a Pattern when the cluster passes V2-7 gates.
// On success it persists PatternEvidence and DERIVED_FROM relations (pattern ← experience).
func (s *Service) Generalize(ctx context.Context, tenantID string, in GeneralizeInput) (GeneralizeResult, error) {
	if s.patterns == nil {
		return GeneralizeResult{Skipped: "pattern repository not configured"}, nil
	}
	if err := requireNonEmpty("tenant_id", tenantID); err != nil {
		return GeneralizeResult{}, err
	}
	ids := uniqueNonEmpty(in.ExperienceIDs)
	if len(ids) < MinGeneralizeExperiences {
		return GeneralizeResult{Skipped: fmt.Sprintf("need ≥%d experiences, got %d", MinGeneralizeExperiences, len(ids))}, nil
	}

	exps := make([]Experience, 0, len(ids))
	for _, id := range ids {
		exp, err := s.repo.Get(ctx, tenantID, id)
		if err != nil {
			return GeneralizeResult{}, fmt.Errorf("get experience %s: %w", id, err)
		}
		if !exp.Status.Retrievable() {
			continue
		}
		exps = append(exps, exp)
	}
	if len(exps) < MinGeneralizeExperiences {
		return GeneralizeResult{Skipped: fmt.Sprintf("need ≥%d retrievable experiences, got %d", MinGeneralizeExperiences, len(exps))}, nil
	}

	if reason := generalizationGate(exps); reason != "" {
		return GeneralizeResult{Skipped: reason}, nil
	}

	if s.relations != nil {
		rate, err := s.clusterConflictRate(ctx, tenantID, exps)
		if err != nil {
			return GeneralizeResult{}, err
		}
		if rate > MaxGeneralizeConflictRate {
			return GeneralizeResult{Skipped: fmt.Sprintf("conflict rate %.2f exceeds %.2f", rate, MaxGeneralizeConflictRate)}, nil
		}
	}

	// Reuse an existing pattern that already covers this cluster.
	existing, err := s.patterns.FindByExperience(ctx, tenantID, ids)
	if err != nil {
		return GeneralizeResult{}, fmt.Errorf("find patterns by experience: %w", err)
	}
	for _, p := range existing {
		if p.Status.Retrievable() && samePatternFamily(p, exps[0]) {
			return GeneralizeResult{Created: false, Pattern: p, Skipped: "pattern already covers cluster"}, nil
		}
	}

	gen := s.generalizer
	if gen == nil {
		gen = HeuristicPatternGeneralizer{}
	}
	draft, err := gen.Generalize(ctx, exps)
	if err != nil {
		return GeneralizeResult{}, fmt.Errorf("generalize draft: %w", err)
	}

	now := s.now()
	pattern := Pattern{
		ID:           s.id(),
		TenantID:     strings.TrimSpace(tenantID),
		Type:         draft.Type,
		Scope:        draft.Scope,
		ScopeKey:     draft.ScopeKey,
		Trigger:      strings.TrimSpace(draft.Trigger),
		Content:      strings.TrimSpace(draft.Content),
		Confidence:   clamp01(draft.Confidence),
		Utility:      clamp01(draft.Utility),
		SupportCount: len(exps),
		Status:       PatternStatusCandidate,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if pattern.Trigger == "" || pattern.Content == "" {
		return GeneralizeResult{}, fmt.Errorf("%w: generalized trigger/content required", ErrInvalidInput)
	}

	created, err := s.patterns.Create(ctx, pattern)
	if err != nil {
		return GeneralizeResult{}, fmt.Errorf("create pattern: %w", err)
	}

	for _, exp := range exps {
		ev := PatternEvidence{PatternID: created.ID, ExperienceID: exp.ID, CreatedAt: now}
		if err := s.patterns.AddEvidence(ctx, ev); err != nil {
			return GeneralizeResult{}, fmt.Errorf("add pattern evidence %s: %w", exp.ID, err)
		}
		if s.relations != nil {
			_, err := s.relations.Upsert(ctx, ExperienceRelation{
				ID:               s.id(),
				TenantID:         created.TenantID,
				FromExperienceID: created.ID, // pattern id
				ToExperienceID:   exp.ID,
				Type:             RelationDerivedFrom,
				Confidence:       created.Confidence,
				Reason:           "pattern generalized from experience",
				CreatedAt:        now,
			})
			if err != nil {
				return GeneralizeResult{}, fmt.Errorf("record DERIVED_FROM for %s: %w", exp.ID, err)
			}
		}
	}

	return GeneralizeResult{Created: true, Pattern: created}, nil
}

// GetPattern returns one pattern for a tenant.
func (s *Service) GetPattern(ctx context.Context, tenantID, patternID string) (Pattern, error) {
	if s.patterns == nil {
		return Pattern{}, ErrNotFound
	}
	if err := requireNonEmpty("tenant_id", tenantID, "pattern_id", patternID); err != nil {
		return Pattern{}, err
	}
	p, err := s.patterns.Get(ctx, tenantID, patternID)
	if err != nil {
		return Pattern{}, fmt.Errorf("get pattern %s: %w", patternID, err)
	}
	return p, nil
}

// ListPatternEvidence returns supporting experiences for a pattern.
func (s *Service) ListPatternEvidence(ctx context.Context, tenantID, patternID string) ([]PatternEvidence, error) {
	if s.patterns == nil {
		return nil, ErrNotFound
	}
	if err := requireNonEmpty("tenant_id", tenantID, "pattern_id", patternID); err != nil {
		return nil, err
	}
	ev, err := s.patterns.ListEvidence(ctx, tenantID, patternID)
	if err != nil {
		return nil, fmt.Errorf("list pattern evidence %s: %w", patternID, err)
	}
	return ev, nil
}

func generalizationGate(exps []Experience) string {
	if len(exps) < MinGeneralizeExperiences {
		return fmt.Sprintf("need ≥%d experiences, got %d", MinGeneralizeExperiences, len(exps))
	}
	episodes := map[string]struct{}{}
	var utilSum float64
	base := exps[0]
	for _, e := range exps {
		if e.Type != base.Type || e.Scope != base.Scope || e.ScopeKey != base.ScopeKey {
			return "experiences must share type/scope/scope_key"
		}
		for _, ep := range episodeIDs(e) {
			episodes[ep] = struct{}{}
		}
		utilSum += e.Utility
	}
	if len(episodes) < MinGeneralizeEpisodes {
		return fmt.Sprintf("need ≥%d distinct episodes, got %d", MinGeneralizeEpisodes, len(episodes))
	}
	avgUtil := utilSum / float64(len(exps))
	if avgUtil < MinGeneralizeAvgUtility {
		return fmt.Sprintf("avg utility %.3f below %.2f", avgUtil, MinGeneralizeAvgUtility)
	}
	return ""
}

func episodeIDs(e Experience) []string {
	seen := map[string]struct{}{}
	var out []string
	add := func(id string) {
		id = strings.TrimSpace(id)
		if id == "" {
			return
		}
		if _, ok := seen[id]; ok {
			return
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	add(e.SourceEpisodeID)
	add(e.Evidence.SourceEpisodeID)
	for _, id := range e.Evidence.SupportEpisodeIDs {
		add(id)
	}
	return out
}

func (s *Service) clusterConflictRate(ctx context.Context, tenantID string, exps []Experience) (float64, error) {
	ids := make([]string, len(exps))
	for i, e := range exps {
		ids[i] = e.ID
	}
	peers, err := s.ConflictPeers(ctx, tenantID, ids)
	if err != nil {
		return 0, err
	}
	if len(exps) < 2 {
		return 0, nil
	}
	// Count unique unordered conflicting pairs within the cluster.
	pairSeen := map[string]struct{}{}
	for id, peer := range peers {
		inCluster := false
		for _, e := range exps {
			if e.ID == peer {
				inCluster = true
				break
			}
		}
		if !inCluster {
			continue
		}
		a, b := id, peer
		if a > b {
			a, b = b, a
		}
		pairSeen[a+"|"+b] = struct{}{}
	}
	possible := float64(len(exps) * (len(exps) - 1) / 2)
	if possible == 0 {
		return 0, nil
	}
	return float64(len(pairSeen)) / possible, nil
}

func samePatternFamily(p Pattern, exp Experience) bool {
	return p.Type == exp.Type && p.Scope == exp.Scope && p.ScopeKey == exp.ScopeKey
}

func uniqueNonEmpty(ids []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}
