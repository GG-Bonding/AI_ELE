package experienceclient

import (
	"context"
	"net/url"
)

// ContextItem is one experience placed into agent context.
type ContextItem struct {
	Type       string  `json:"type"`
	Content    string  `json:"content"`
	Source     string  `json:"source"`
	Confidence float64 `json:"confidence"`
}

// SelectionDecision is a selector decision for diagnostics.
type SelectionDecision struct {
	ExperienceID string  `json:"experience_id"`
	Decision     string  `json:"decision"`
	Reason       string  `json:"reason"`
	FinalScore   float64 `json:"final_score"`
}

// ContextPayload is the safe context returned to an agent.
type ContextPayload struct {
	Disclaimer  string              `json:"disclaimer"`
	Experiences []ContextItem       `json:"experiences"`
	Selections  []SelectionDecision `json:"selections"`
}

// GetContextInput requests experience context for a task.
type GetContextInput struct {
	TenantID       string
	AgentID        string
	UserID         string
	EpisodeID      string
	Task           string
	Tools          []string
	MaxExperiences int
	MaxTokens      int
	TopK           int
}

// GetContext retrieves selected experiences for the current task.
func (c *Client) GetContext(ctx context.Context, in GetContextInput) (ContextPayload, error) {
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
		"tenant_id":       tenant,
		"agent_id":        agent,
		"user_id":         user,
		"episode_id":      in.EpisodeID,
		"task":            in.Task,
		"tools":           in.Tools,
		"max_experiences": in.MaxExperiences,
		"max_tokens":      in.MaxTokens,
		"top_k":           in.TopK,
	}
	var out ContextPayload
	err := c.doJSON(ctx, "POST", "/api/v1/context", nil, body, &out)
	return out, err
}

// FeedbackInput submits outcome feedback for an episode.
type FeedbackInput struct {
	TenantID   string
	EpisodeID  string
	Source     string
	Signal     string
	Reward     *float64
	Confidence float64
	Evidence   string
}

// FeedbackResult is returned after submitting feedback.
type FeedbackResult struct {
	Feedback       map[string]any   `json:"feedback"`
	EpisodeReward  map[string]any   `json:"episode_reward"`
	UtilityUpdates []map[string]any `json:"utility_updates,omitempty"`
}

// Feedback submits a reward / signal for an episode.
func (c *Client) Feedback(ctx context.Context, in FeedbackInput) (FeedbackResult, error) {
	tenant := c.resolveTenant(in.TenantID)
	body := map[string]any{
		"tenant_id":  tenant,
		"episode_id": in.EpisodeID,
		"source":     in.Source,
		"signal":     in.Signal,
		"reward":     in.Reward,
		"confidence": in.Confidence,
		"evidence":   in.Evidence,
	}
	var out FeedbackResult
	err := c.doJSON(ctx, "POST", "/api/v1/feedback", nil, body, &out)
	return out, err
}

// SearchInput searches experiences for a task.
type SearchInput struct {
	TenantID string
	Task     string
	AgentID  string
	UserID   string
	Tools    []string
	TopK     int
}

// ScoredExperience is one ranked search hit.
type ScoredExperience struct {
	ID         string         `json:"id"`
	Type       string         `json:"type"`
	Scope      string         `json:"scope"`
	ScopeKey   string         `json:"scope_key,omitempty"`
	Trigger    string         `json:"trigger"`
	Content    string         `json:"content"`
	Confidence float64        `json:"confidence"`
	Utility    float64        `json:"utility"`
	Status     string         `json:"status"`
	Score      map[string]any `json:"score"`
}

// SearchExperiences runs utility-aware retrieval.
func (c *Client) SearchExperiences(ctx context.Context, in SearchInput) ([]ScoredExperience, error) {
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
		"task":      in.Task,
		"agent_id":  agent,
		"user_id":   user,
		"tools":     in.Tools,
		"top_k":     in.TopK,
	}
	var wrap struct {
		Experiences []ScoredExperience `json:"experiences"`
	}
	if err := c.doJSON(ctx, "POST", "/api/v1/experiences/search", nil, body, &wrap); err != nil {
		return nil, err
	}
	return wrap.Experiences, nil
}

// Supersede replaces an old experience with a newer one.
func (c *Client) Supersede(ctx context.Context, tenantID, oldID, replacementID string) (map[string]any, error) {
	tenant := c.resolveTenant(tenantID)
	body := map[string]any{
		"tenant_id":      tenant,
		"replacement_id": replacementID,
	}
	var out map[string]any
	err := c.doJSON(ctx, "POST", "/api/v1/experiences/"+url.PathEscape(oldID)+"/supersede", nil, body, &out)
	return out, err
}

// Healthz calls GET /healthz.
func (c *Client) Healthz(ctx context.Context) (map[string]string, error) {
	var out map[string]string
	err := c.doJSON(ctx, "GET", "/healthz", nil, nil, &out)
	return out, err
}
