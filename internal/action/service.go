package action

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Service owns Action + ExperienceActionLink lifecycle rules.
type Service struct {
	repo     Repository
	episodes EpisodeChecker
	contexts ContextLookup // optional; V2.2-2 provenance auto-link
	now      func() time.Time
	id       func() string
}

// NewService constructs an action tracking service.
func NewService(repo Repository, episodes EpisodeChecker) *Service {
	return &Service{
		repo:     repo,
		episodes: episodes,
		now:      time.Now().UTC,
		id:       func() string { return uuid.NewString() },
	}
}

// WithContexts enables automatic Experience/Pattern→Action links from a context snapshot (V2.2-2).
func (s *Service) WithContexts(lookup ContextLookup) *Service {
	s.contexts = lookup
	return s
}

// RecordInput records one agent action under an episode.
type RecordInput struct {
	TenantID  string
	EpisodeID string
	Type      Type
	ToolName  string
	Input     json.RawMessage
	Output    json.RawMessage
	Status    Status
	AttemptID string
	Sequence  int    // optional; 0 means auto-assign next
	ContextID string // optional; V2.2-2 auto provenance from context snapshot
}

// LinkInput asserts that an experience influenced an action.
type LinkInput struct {
	TenantID       string
	EpisodeID      string
	ActionID       string
	ExperienceID   string
	Influence      *float64 // optional; default 1.0
	AffectedFields []string // optional JSON paths for ACTION_FIELD attribution (V2.1)
	Evidence       string
}

// RecordAction appends an action to an existing episode.
func (s *Service) RecordAction(ctx context.Context, in RecordInput) (AgentAction, error) {
	if err := requireNonEmpty("tenant_id", in.TenantID, "episode_id", in.EpisodeID); err != nil {
		return AgentAction{}, err
	}
	if !in.Type.Valid() {
		return AgentAction{}, fmt.Errorf("%w: invalid action type %q", ErrInvalidInput, in.Type)
	}
	if in.Status == "" {
		in.Status = StatusSuccess
	}
	if !in.Status.Valid() {
		return AgentAction{}, fmt.Errorf("%w: invalid action status %q", ErrInvalidInput, in.Status)
	}
	if in.Type == TypeToolCall && strings.TrimSpace(in.ToolName) == "" {
		return AgentAction{}, fmt.Errorf("%w: tool_name is required for TOOL_CALL", ErrInvalidInput)
	}

	exists, err := s.episodes.EpisodeExists(ctx, in.TenantID, in.EpisodeID)
	if err != nil {
		return AgentAction{}, fmt.Errorf("check episode %s: %w", in.EpisodeID, err)
	}
	if !exists {
		return AgentAction{}, fmt.Errorf("%w: episode %s", ErrEpisodeNotFound, in.EpisodeID)
	}

	contextID := strings.TrimSpace(in.ContextID)
	var snap ContextSnapshot
	if contextID != "" {
		if s.contexts == nil {
			return AgentAction{}, fmt.Errorf("%w: context provenance not configured", ErrInvalidInput)
		}
		snap, err = s.contexts.GetSnapshot(ctx, in.TenantID, contextID)
		if err != nil {
			if errors.Is(err, ErrContextNotFound) {
				return AgentAction{}, err
			}
			return AgentAction{}, fmt.Errorf("%w: %v", ErrContextNotFound, err)
		}
		if snap.EpisodeID != "" && snap.EpisodeID != strings.TrimSpace(in.EpisodeID) {
			return AgentAction{}, fmt.Errorf("%w: context %s belongs to episode %s", ErrInvalidInput, contextID, snap.EpisodeID)
		}
	}

	seq := in.Sequence
	if seq <= 0 {
		seq, err = s.repo.NextActionSequence(ctx, in.TenantID, in.EpisodeID)
		if err != nil {
			return AgentAction{}, fmt.Errorf("next action sequence for episode %s: %w", in.EpisodeID, err)
		}
	}

	now := s.now()
	a := AgentAction{
		ID:        s.id(),
		TenantID:  strings.TrimSpace(in.TenantID),
		EpisodeID: strings.TrimSpace(in.EpisodeID),
		Sequence:  seq,
		Type:      in.Type,
		ToolName:  strings.TrimSpace(in.ToolName),
		Input:     cloneJSON(in.Input),
		Output:    cloneJSON(in.Output),
		Status:    in.Status,
		AttemptID: strings.TrimSpace(in.AttemptID),
		ContextID: contextID,
		StartedAt: now,
		CreatedAt: now,
	}
	if in.Status != StatusRunning {
		completed := now
		a.CompletedAt = &completed
	}

	created, err := s.repo.CreateAction(ctx, a)
	if err != nil {
		return AgentAction{}, fmt.Errorf("create action for episode %s: %w", in.EpisodeID, err)
	}

	if contextID != "" {
		if err := s.bindContextProvenance(ctx, created, snap); err != nil {
			return AgentAction{}, err
		}
	}
	return created, nil
}

func (s *Service) bindContextProvenance(ctx context.Context, a AgentAction, snap ContextSnapshot) error {
	evidence := "context:" + a.ContextID
	for _, expID := range snap.ExperienceIDs {
		expID = strings.TrimSpace(expID)
		if expID == "" {
			continue
		}
		_, err := s.repo.CreateLink(ctx, ExperienceActionLink{
			ID:           s.id(),
			TenantID:     a.TenantID,
			EpisodeID:    a.EpisodeID,
			ExperienceID: expID,
			ActionID:     a.ID,
			Influence:    1.0,
			Evidence:     evidence,
			CreatedAt:    s.now(),
		})
		if err != nil && !errors.Is(err, ErrDuplicateLink) {
			return fmt.Errorf("auto-link experience %s to action %s: %w", expID, a.ID, err)
		}
	}
	for _, patID := range snap.PatternIDs {
		patID = strings.TrimSpace(patID)
		if patID == "" {
			continue
		}
		_, err := s.repo.CreatePatternLink(ctx, PatternActionLink{
			ID:        s.id(),
			TenantID:  a.TenantID,
			EpisodeID: a.EpisodeID,
			PatternID: patID,
			ActionID:  a.ID,
			Influence: 1.0,
			Evidence:  evidence,
			CreatedAt: s.now(),
		})
		if err != nil && !errors.Is(err, ErrDuplicatePatternLink) {
			return fmt.Errorf("auto-link pattern %s to action %s: %w", patID, a.ID, err)
		}
	}
	return nil
}

// LinkExperience records that an experience influenced an action in this episode.
func (s *Service) LinkExperience(ctx context.Context, in LinkInput) (ExperienceActionLink, error) {
	if err := requireNonEmpty(
		"tenant_id", in.TenantID,
		"episode_id", in.EpisodeID,
		"action_id", in.ActionID,
		"experience_id", in.ExperienceID,
	); err != nil {
		return ExperienceActionLink{}, err
	}

	action, err := s.repo.GetAction(ctx, in.TenantID, in.ActionID)
	if err != nil {
		return ExperienceActionLink{}, err
	}
	if action.EpisodeID != strings.TrimSpace(in.EpisodeID) {
		return ExperienceActionLink{}, fmt.Errorf("%w: action %s does not belong to episode %s", ErrInvalidInput, in.ActionID, in.EpisodeID)
	}

	influence := 1.0
	if in.Influence != nil {
		influence = *in.Influence
	}
	if influence < 0 || influence > 1 {
		return ExperienceActionLink{}, fmt.Errorf("%w: influence must be in [0,1]", ErrInvalidInput)
	}

	link := ExperienceActionLink{
		ID:             s.id(),
		TenantID:       strings.TrimSpace(in.TenantID),
		EpisodeID:      strings.TrimSpace(in.EpisodeID),
		ExperienceID:   strings.TrimSpace(in.ExperienceID),
		ActionID:       strings.TrimSpace(in.ActionID),
		Influence:      influence,
		AffectedFields: NormalizeAffectedFields(in.AffectedFields),
		Evidence:       strings.TrimSpace(in.Evidence),
		CreatedAt:      s.now(),
	}

	created, err := s.repo.CreateLink(ctx, link)
	if err != nil {
		return ExperienceActionLink{}, err
	}
	return created, nil
}

// ListActions returns actions for an episode in sequence order.
func (s *Service) ListActions(ctx context.Context, tenantID, episodeID string) ([]AgentAction, error) {
	if err := requireNonEmpty("tenant_id", tenantID, "episode_id", episodeID); err != nil {
		return nil, err
	}
	exists, err := s.episodes.EpisodeExists(ctx, tenantID, episodeID)
	if err != nil {
		return nil, fmt.Errorf("check episode %s: %w", episodeID, err)
	}
	if !exists {
		return nil, fmt.Errorf("%w: episode %s", ErrEpisodeNotFound, episodeID)
	}
	return s.repo.ListActionsByEpisode(ctx, tenantID, episodeID)
}

// ListLinks returns experience→action links for an episode.
func (s *Service) ListLinks(ctx context.Context, tenantID, episodeID string) ([]ExperienceActionLink, error) {
	if err := requireNonEmpty("tenant_id", tenantID, "episode_id", episodeID); err != nil {
		return nil, err
	}
	exists, err := s.episodes.EpisodeExists(ctx, tenantID, episodeID)
	if err != nil {
		return nil, fmt.Errorf("check episode %s: %w", episodeID, err)
	}
	if !exists {
		return nil, fmt.Errorf("%w: episode %s", ErrEpisodeNotFound, episodeID)
	}
	return s.repo.ListLinksByEpisode(ctx, tenantID, episodeID)
}

// ListPatternLinks returns pattern→action links for an episode (V2.2-2).
func (s *Service) ListPatternLinks(ctx context.Context, tenantID, episodeID string) ([]PatternActionLink, error) {
	if err := requireNonEmpty("tenant_id", tenantID, "episode_id", episodeID); err != nil {
		return nil, err
	}
	exists, err := s.episodes.EpisodeExists(ctx, tenantID, episodeID)
	if err != nil {
		return nil, fmt.Errorf("check episode %s: %w", episodeID, err)
	}
	if !exists {
		return nil, fmt.Errorf("%w: episode %s", ErrEpisodeNotFound, episodeID)
	}
	return s.repo.ListPatternLinksByEpisode(ctx, tenantID, episodeID)
}

// GetAction returns one action by id.
func (s *Service) GetAction(ctx context.Context, tenantID, actionID string) (AgentAction, error) {
	if err := requireNonEmpty("tenant_id", tenantID, "action_id", actionID); err != nil {
		return AgentAction{}, err
	}
	return s.repo.GetAction(ctx, tenantID, actionID)
}

func requireNonEmpty(pairs ...string) error {
	if len(pairs)%2 != 0 {
		return fmt.Errorf("%w: requireNonEmpty pairs", ErrInvalidInput)
	}
	for i := 0; i < len(pairs); i += 2 {
		if strings.TrimSpace(pairs[i+1]) == "" {
			return fmt.Errorf("%w: %s is required", ErrInvalidInput, pairs[i])
		}
	}
	return nil
}

func cloneJSON(in json.RawMessage) json.RawMessage {
	if len(in) == 0 {
		return nil
	}
	out := make(json.RawMessage, len(in))
	copy(out, in)
	return out
}

// ActionInEpisode reports whether actionID belongs to the given episode (tenant-scoped).
// Implements feedback.ActionVerifier.
func (s *Service) ActionInEpisode(ctx context.Context, tenantID, episodeID, actionID string) (bool, error) {
	a, err := s.GetAction(ctx, tenantID, actionID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return false, nil
		}
		return false, err
	}
	return a.EpisodeID == strings.TrimSpace(episodeID), nil
}
