package episode

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Service owns Episode lifecycle rules (not agent planning/execution).
type Service struct {
	repo Repository
	now  func() time.Time
	id   func() string
}

// NewService constructs a lifecycle service.
func NewService(repo Repository) *Service {
	return &Service{
		repo: repo,
		now:  time.Now().UTC,
		id:   func() string { return uuid.NewString() },
	}
}

// CreateEpisodeInput starts a new RUNNING episode.
type CreateEpisodeInput struct {
	TenantID string
	AgentID  string
	UserID   string
	TaskType string
	Goal     string
	Input    string
}

// CreateEpisode validates and persists a new episode.
func (s *Service) CreateEpisode(ctx context.Context, in CreateEpisodeInput) (Episode, error) {
	if err := requireNonEmpty(
		"tenant_id", in.TenantID,
		"agent_id", in.AgentID,
		"user_id", in.UserID,
		"goal", in.Goal,
	); err != nil {
		return Episode{}, err
	}

	now := s.now()
	ep := Episode{
		ID:        s.id(),
		TenantID:  strings.TrimSpace(in.TenantID),
		AgentID:   strings.TrimSpace(in.AgentID),
		UserID:    strings.TrimSpace(in.UserID),
		TaskType:  strings.TrimSpace(in.TaskType),
		Goal:      strings.TrimSpace(in.Goal),
		Input:     in.Input,
		Status:    StatusRunning,
		StartedAt: now,
		CreatedAt: now,
		UpdatedAt: now,
	}

	created, err := s.repo.CreateEpisode(ctx, ep)
	if err != nil {
		return Episode{}, fmt.Errorf("create episode: %w", err)
	}
	return created, nil
}

// AddAttemptInput records one attempt under an episode.
type AddAttemptInput struct {
	TenantID     string
	EpisodeID    string
	Hypothesis   string
	Action       string
	ToolName     string
	Input        json.RawMessage
	Output       json.RawMessage
	Status       AttemptStatus
	ErrorCode    string
	ErrorMessage string
	Sequence     int // optional; 0 means auto-assign next
}

// AddAttempt appends an attempt to a RUNNING episode.
func (s *Service) AddAttempt(ctx context.Context, in AddAttemptInput) (Attempt, error) {
	if err := requireNonEmpty("tenant_id", in.TenantID, "episode_id", in.EpisodeID); err != nil {
		return Attempt{}, err
	}
	if in.Status == "" {
		in.Status = AttemptStatusSuccess
	}
	if !in.Status.Valid() {
		return Attempt{}, fmt.Errorf("%w: invalid attempt status %q", ErrInvalidInput, in.Status)
	}

	ep, err := s.repo.GetEpisode(ctx, in.TenantID, in.EpisodeID)
	if err != nil {
		return Attempt{}, fmt.Errorf("get episode %s: %w", in.EpisodeID, err)
	}
	if ep.Status.Terminal() {
		return Attempt{}, fmt.Errorf("%w: episode %s status %s", ErrAlreadyCompleted, ep.ID, ep.Status)
	}

	seq := in.Sequence
	if seq <= 0 {
		seq, err = s.repo.NextAttemptSequence(ctx, in.TenantID, in.EpisodeID)
		if err != nil {
			return Attempt{}, fmt.Errorf("next attempt sequence for episode %s: %w", in.EpisodeID, err)
		}
	}

	now := s.now()
	completed := now
	a := Attempt{
		ID:           s.id(),
		EpisodeID:    ep.ID,
		TenantID:     ep.TenantID,
		Sequence:     seq,
		Hypothesis:   in.Hypothesis,
		Action:       in.Action,
		ToolName:     in.ToolName,
		Input:        cloneJSON(in.Input),
		Output:       cloneJSON(in.Output),
		Status:       in.Status,
		ErrorCode:    in.ErrorCode,
		ErrorMessage: in.ErrorMessage,
		StartedAt:    now,
		CompletedAt:  &completed,
	}

	created, err := s.repo.CreateAttempt(ctx, a)
	if err != nil {
		return Attempt{}, fmt.Errorf("create attempt for episode %s: %w", ep.ID, err)
	}
	return created, nil
}

// CompleteEpisodeInput records the terminal outcome and closes the episode.
type CompleteEpisodeInput struct {
	TenantID  string
	EpisodeID string
	Status    Status
	Result    json.RawMessage
	Verified  bool
	Verifier  string
	Metrics   map[string]float64
}

// CompleteEpisode stores Outcome and transitions Episode out of RUNNING.
func (s *Service) CompleteEpisode(ctx context.Context, in CompleteEpisodeInput) (Episode, Outcome, error) {
	if err := requireNonEmpty("tenant_id", in.TenantID, "episode_id", in.EpisodeID); err != nil {
		return Episode{}, Outcome{}, err
	}
	if !in.Status.Valid() || in.Status == StatusRunning {
		return Episode{}, Outcome{}, fmt.Errorf("%w: outcome status must be terminal, got %q", ErrInvalidInput, in.Status)
	}

	ep, err := s.repo.GetEpisode(ctx, in.TenantID, in.EpisodeID)
	if err != nil {
		return Episode{}, Outcome{}, fmt.Errorf("get episode %s: %w", in.EpisodeID, err)
	}
	if ep.Status.Terminal() {
		return Episode{}, Outcome{}, fmt.Errorf("%w: episode %s status %s", ErrAlreadyCompleted, ep.ID, ep.Status)
	}

	if _, err := s.repo.GetOutcome(ctx, in.TenantID, in.EpisodeID); err == nil {
		return Episode{}, Outcome{}, fmt.Errorf("%w: episode %s", ErrOutcomeExists, in.EpisodeID)
	} else if !errors.Is(err, ErrNotFound) {
		return Episode{}, Outcome{}, fmt.Errorf("get outcome for episode %s: %w", in.EpisodeID, err)
	}

	now := s.now()
	outcome := Outcome{
		ID:        s.id(),
		EpisodeID: ep.ID,
		TenantID:  ep.TenantID,
		Status:    string(in.Status),
		Result:    cloneJSON(in.Result),
		Verified:  in.Verified,
		Verifier:  in.Verifier,
		Metrics:   cloneMetrics(in.Metrics),
		CreatedAt: now,
	}

	createdOutcome, err := s.repo.CreateOutcome(ctx, outcome)
	if err != nil {
		return Episode{}, Outcome{}, fmt.Errorf("create outcome for episode %s: %w", ep.ID, err)
	}

	ep.Status = in.Status
	ep.CompletedAt = &now
	ep.UpdatedAt = now

	updated, err := s.repo.UpdateEpisode(ctx, ep)
	if err != nil {
		return Episode{}, Outcome{}, fmt.Errorf("update episode %s after outcome: %w", ep.ID, err)
	}
	return updated, createdOutcome, nil
}

// GetEpisode returns an episode for the given tenant.
func (s *Service) GetEpisode(ctx context.Context, tenantID, id string) (Episode, error) {
	if err := requireNonEmpty("tenant_id", tenantID, "id", id); err != nil {
		return Episode{}, err
	}
	ep, err := s.repo.GetEpisode(ctx, tenantID, id)
	if err != nil {
		return Episode{}, fmt.Errorf("get episode %s: %w", id, err)
	}
	return ep, nil
}

// GetOutcome returns the stored outcome for a completed episode.
func (s *Service) GetOutcome(ctx context.Context, tenantID, episodeID string) (Outcome, error) {
	if err := requireNonEmpty("tenant_id", tenantID, "episode_id", episodeID); err != nil {
		return Outcome{}, err
	}
	out, err := s.repo.GetOutcome(ctx, tenantID, episodeID)
	if err != nil {
		return Outcome{}, fmt.Errorf("get outcome for episode %s: %w", episodeID, err)
	}
	return out, nil
}

// ListAttempts returns attempts for an episode within a tenant.
func (s *Service) ListAttempts(ctx context.Context, tenantID, episodeID string) ([]Attempt, error) {
	if err := requireNonEmpty("tenant_id", tenantID, "episode_id", episodeID); err != nil {
		return nil, err
	}
	if _, err := s.repo.GetEpisode(ctx, tenantID, episodeID); err != nil {
		return nil, fmt.Errorf("get episode %s: %w", episodeID, err)
	}
	attempts, err := s.repo.ListAttempts(ctx, tenantID, episodeID)
	if err != nil {
		return nil, fmt.Errorf("list attempts for episode %s: %w", episodeID, err)
	}
	return attempts, nil
}

func requireNonEmpty(pairs ...string) error {
	if len(pairs)%2 != 0 {
		return fmt.Errorf("requireNonEmpty: odd argument count")
	}
	for i := 0; i < len(pairs); i += 2 {
		if strings.TrimSpace(pairs[i+1]) == "" {
			return fmt.Errorf("%w: %s is required", ErrInvalidInput, pairs[i])
		}
	}
	return nil
}

func cloneJSON(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return nil
	}
	out := make(json.RawMessage, len(raw))
	copy(out, raw)
	return out
}

func cloneMetrics(in map[string]float64) map[string]float64 {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]float64, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
