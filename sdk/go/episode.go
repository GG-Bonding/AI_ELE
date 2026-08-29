package experienceclient

import (
	"context"
	"encoding/json"
	"net/url"
	"time"
)

// Episode is a running or completed task trace.
type Episode struct {
	ID        string    `json:"id"`
	TenantID  string    `json:"tenant_id"`
	AgentID   string    `json:"agent_id"`
	UserID    string    `json:"user_id"`
	TaskType  string    `json:"task_type"`
	Goal      string    `json:"goal"`
	Input     string    `json:"input"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Attempt is one try within an episode.
type Attempt struct {
	ID           string          `json:"id"`
	EpisodeID    string          `json:"episode_id"`
	TenantID     string          `json:"tenant_id"`
	Sequence     int             `json:"sequence"`
	Hypothesis   string          `json:"hypothesis"`
	Action       string          `json:"action"`
	ToolName     string          `json:"tool_name"`
	Input        json.RawMessage `json:"input"`
	Output       json.RawMessage `json:"output"`
	Status       string          `json:"status"`
	ErrorCode    string          `json:"error_code"`
	ErrorMessage string          `json:"error_message"`
}

// Outcome is the terminal result of an episode.
type Outcome struct {
	ID        string             `json:"id"`
	EpisodeID string             `json:"episode_id"`
	TenantID  string             `json:"tenant_id"`
	Status    string             `json:"status"`
	Result    json.RawMessage    `json:"result"`
	Verified  bool               `json:"verified"`
	Verifier  string             `json:"verifier"`
	Metrics   map[string]float64 `json:"metrics"`
}

// StartEpisodeInput creates a new episode.
type StartEpisodeInput struct {
	TenantID string
	AgentID  string
	UserID   string
	TaskType string
	Goal     string
	Input    string
}

// AddAttemptInput records one attempt.
type AddAttemptInput struct {
	Hypothesis   string
	Action       string
	ToolName     string
	Input        json.RawMessage
	Output       json.RawMessage
	Status       string
	ErrorCode    string
	ErrorMessage string
	Sequence     int
}

// CompleteInput finishes an episode with an outcome.
type CompleteInput struct {
	Status   string
	Result   json.RawMessage
	Verified bool
	Verifier string
	Metrics  map[string]float64
}

// CompleteResult is returned by Complete.
type CompleteResult struct {
	Episode              Episode          `json:"episode"`
	Outcome              Outcome          `json:"outcome"`
	ExperienceCandidates []map[string]any `json:"experience_candidates,omitempty"`
	StoredExperiences    []map[string]any `json:"stored_experiences,omitempty"`
}

// EpisodeHandle binds an episode id to a client for fluent calls.
type EpisodeHandle struct {
	client   *Client
	ID       string
	TenantID string
}

// StartEpisode creates an episode and returns a handle.
func (c *Client) StartEpisode(ctx context.Context, in StartEpisodeInput) (*EpisodeHandle, Episode, error) {
	tenant := c.resolveTenant(in.TenantID)
	agent := in.AgentID
	if agent == "" {
		agent = c.agentID
	}
	user := in.UserID
	if user == "" {
		user = c.userID
	}
	body := map[string]any{
		"tenant_id": tenant,
		"agent_id":  agent,
		"user_id":   user,
		"task_type": in.TaskType,
		"goal":      in.Goal,
		"input":     in.Input,
	}
	var ep Episode
	if err := c.doJSON(ctx, "POST", "/api/v1/episodes", nil, body, &ep); err != nil {
		return nil, Episode{}, err
	}
	return &EpisodeHandle{client: c, ID: ep.ID, TenantID: ep.TenantID}, ep, nil
}

// GetEpisode loads an episode by id.
func (c *Client) GetEpisode(ctx context.Context, tenantID, episodeID string) (Episode, error) {
	tenant := c.resolveTenant(tenantID)
	q := url.Values{"tenant_id": {tenant}}
	var ep Episode
	err := c.doJSON(ctx, "GET", "/api/v1/episodes/"+url.PathEscape(episodeID), q, nil, &ep)
	return ep, err
}

// AddAttempt records an attempt on this episode.
func (h *EpisodeHandle) AddAttempt(ctx context.Context, in AddAttemptInput) (Attempt, error) {
	body := map[string]any{
		"tenant_id":     h.TenantID,
		"hypothesis":    in.Hypothesis,
		"action":        in.Action,
		"tool_name":     in.ToolName,
		"input":         in.Input,
		"output":        in.Output,
		"status":        in.Status,
		"error_code":    in.ErrorCode,
		"error_message": in.ErrorMessage,
		"sequence":      in.Sequence,
	}
	var a Attempt
	err := h.client.doJSON(ctx, "POST", "/api/v1/episodes/"+url.PathEscape(h.ID)+"/attempts", nil, body, &a)
	return a, err
}

// Complete finishes the episode with an outcome.
func (h *EpisodeHandle) Complete(ctx context.Context, in CompleteInput) (CompleteResult, error) {
	body := map[string]any{
		"tenant_id": h.TenantID,
		"status":    in.Status,
		"result":    in.Result,
		"verified":  in.Verified,
		"verifier":  in.Verifier,
		"metrics":   in.Metrics,
	}
	var out CompleteResult
	err := h.client.doJSON(ctx, "POST", "/api/v1/episodes/"+url.PathEscape(h.ID)+"/outcome", nil, body, &out)
	return out, err
}
