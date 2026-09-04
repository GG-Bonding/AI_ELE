package experienceclient

import (
	"context"
	"encoding/json"
	"net/url"
)

// FeedbackTarget locates what a feedback signal applies to (V2).
type FeedbackTarget struct {
	Type         string `json:"type"`
	ActionID     string `json:"action_id,omitempty"`
	ToolName     string `json:"tool_name,omitempty"`
	Field        string `json:"field,omitempty"`
	ExperienceID string `json:"experience_id,omitempty"`
}

// TargetEpisode is whole-episode feedback.
func TargetEpisode() *FeedbackTarget {
	return &FeedbackTarget{Type: "EPISODE"}
}

// TargetAction points at one recorded action.
func TargetAction(actionID string) *FeedbackTarget {
	return &FeedbackTarget{Type: "ACTION", ActionID: actionID}
}

// TargetActionField points at a field inside an action input/decision.
func TargetActionField(actionID, field string) *FeedbackTarget {
	return &FeedbackTarget{Type: "ACTION_FIELD", ActionID: actionID, Field: field}
}

// TargetTool points at a tool name when the action id is unknown.
func TargetTool(toolName string) *FeedbackTarget {
	return &FeedbackTarget{Type: "TOOL", ToolName: toolName}
}

// TargetExperience points at a specific experience id.
func TargetExperience(experienceID string) *FeedbackTarget {
	return &FeedbackTarget{Type: "EXPERIENCE", ExperienceID: experienceID}
}

// Action is one agent tool/decision step recorded against an episode.
type Action struct {
	ID        string          `json:"id"`
	EpisodeID string          `json:"episode_id"`
	TenantID  string          `json:"tenant_id"`
	Type      string          `json:"type"`
	ToolName  string          `json:"tool_name"`
	Input     json.RawMessage `json:"input"`
	Output    json.RawMessage `json:"output"`
	Status    string          `json:"status"`
	AttemptID string          `json:"attempt_id,omitempty"`
	Sequence  int             `json:"sequence"`
	ContextID string          `json:"context_id,omitempty"`
}

// RecordActionInput records a tool/decision action (optionally bound to a context_id).
type RecordActionInput struct {
	Type      string
	ToolName  string
	Input     json.RawMessage
	Output    json.RawMessage
	Status    string
	AttemptID string
	Sequence  int
	ContextID string
}

// ActionLink binds an experience to an action with optional field attribution.
type ActionLink struct {
	ID             string   `json:"id"`
	ActionID       string   `json:"action_id"`
	ExperienceID   string   `json:"experience_id"`
	Influence      float64  `json:"influence"`
	AffectedFields []string `json:"affected_fields,omitempty"`
	Evidence       string   `json:"evidence,omitempty"`
}

// LinkExperienceInput creates an experience→action link.
type LinkExperienceInput struct {
	ExperienceID   string
	Influence      *float64
	AffectedFields []string
	Evidence       string
}

// Pattern is a generalized reusable rule.
type Pattern struct {
	ID         string  `json:"id"`
	TenantID   string  `json:"tenant_id"`
	Type       string  `json:"type"`
	Scope      string  `json:"scope"`
	ScopeKey   string  `json:"scope_key,omitempty"`
	Trigger    string  `json:"trigger"`
	Content    string  `json:"content"`
	Confidence float64 `json:"confidence"`
	Utility    float64 `json:"utility"`
	Status     string  `json:"status"`
}

// Skill is a proposed executable skill derived from a Pattern.
type Skill struct {
	ID        string `json:"id"`
	TenantID  string `json:"tenant_id"`
	PatternID string `json:"pattern_id"`
	Name      string `json:"name"`
	Status    string `json:"status"`
}

// RecordAction records an action on this episode.
func (h *EpisodeHandle) RecordAction(ctx context.Context, in RecordActionInput) (Action, error) {
	body := map[string]any{
		"tenant_id":  h.TenantID,
		"type":       in.Type,
		"tool_name":  in.ToolName,
		"input":      in.Input,
		"output":     in.Output,
		"status":     in.Status,
		"attempt_id": in.AttemptID,
		"sequence":   in.Sequence,
		"context_id": in.ContextID,
	}
	var a Action
	err := h.client.doJSON(ctx, "POST", "/api/v1/episodes/"+url.PathEscape(h.ID)+"/actions", nil, body, &a)
	return a, err
}

// ListActions lists actions for this episode.
func (h *EpisodeHandle) ListActions(ctx context.Context) ([]Action, error) {
	q := url.Values{"tenant_id": {h.TenantID}}
	var wrap struct {
		Actions []Action `json:"actions"`
	}
	err := h.client.doJSON(ctx, "GET", "/api/v1/episodes/"+url.PathEscape(h.ID)+"/actions", q, nil, &wrap)
	return wrap.Actions, err
}

// LinkExperience links an experience to an action (field attribution).
func (h *EpisodeHandle) LinkExperience(ctx context.Context, actionID string, in LinkExperienceInput) (ActionLink, error) {
	body := map[string]any{
		"tenant_id":       h.TenantID,
		"experience_id":   in.ExperienceID,
		"influence":       in.Influence,
		"affected_fields": in.AffectedFields,
		"evidence":        in.Evidence,
	}
	var link ActionLink
	err := h.client.doJSON(ctx, "POST",
		"/api/v1/episodes/"+url.PathEscape(h.ID)+"/actions/"+url.PathEscape(actionID)+"/links",
		nil, body, &link)
	return link, err
}

// GetContext builds task context for this episode (includes context_id + patterns).
func (h *EpisodeHandle) GetContext(ctx context.Context, in GetContextInput) (ContextPayload, error) {
	in.TenantID = h.TenantID
	in.EpisodeID = h.ID
	return h.client.GetContext(ctx, in)
}

// ToolCallResult is returned by the ergonomic ToolCall helper.
type ToolCallResult struct {
	Action Action
	// Field returns an ACTION_FIELD feedback target for this action.
	Field func(field string) *FeedbackTarget
}

// ToolCall records a tool action optionally bound to a prior context_id.
func (h *EpisodeHandle) ToolCall(ctx context.Context, contextID, tool string, input any, status string) (ToolCallResult, error) {
	raw, err := json.Marshal(input)
	if err != nil {
		return ToolCallResult{}, err
	}
	a, err := h.RecordAction(ctx, RecordActionInput{
		Type:      "TOOL_CALL",
		ToolName:  tool,
		Input:     raw,
		Status:    status,
		ContextID: contextID,
	})
	if err != nil {
		return ToolCallResult{}, err
	}
	return ToolCallResult{
		Action: a,
		Field: func(field string) *FeedbackTarget {
			return TargetActionField(a.ID, field)
		},
	}, nil
}

// Feedback submits feedback for this episode (supports structured Target).
func (h *EpisodeHandle) Feedback(ctx context.Context, in FeedbackInput) (FeedbackResult, error) {
	in.TenantID = h.TenantID
	in.EpisodeID = h.ID
	return h.client.Feedback(ctx, in)
}

// GetPattern loads a pattern by id.
func (c *Client) GetPattern(ctx context.Context, tenantID, patternID string) (Pattern, error) {
	tenant := c.resolveTenant(tenantID)
	q := url.Values{"tenant_id": {tenant}}
	var p Pattern
	err := c.doJSON(ctx, "GET", "/api/v1/patterns/"+url.PathEscape(patternID), q, nil, &p)
	return p, err
}

// GeneralizePatterns asks the engine to generalize a cluster of experiences.
func (c *Client) GeneralizePatterns(ctx context.Context, tenantID string, experienceIDs []string) (map[string]any, error) {
	tenant := c.resolveTenant(tenantID)
	body := map[string]any{
		"tenant_id":       tenant,
		"experience_ids":  experienceIDs,
	}
	var out map[string]any
	err := c.doJSON(ctx, "POST", "/api/v1/patterns/generalize", nil, body, &out)
	return out, err
}

// EvolvePatterns triggers a tenant-wide auto-generalization scan (evolution fallback).
func (c *Client) EvolvePatterns(ctx context.Context, tenantID string) (map[string]any, error) {
	tenant := c.resolveTenant(tenantID)
	body := map[string]any{"tenant_id": tenant}
	var out map[string]any
	err := c.doJSON(ctx, "POST", "/api/v1/patterns/evolve", nil, body, &out)
	return out, err
}

// PatternRewardInput applies a direct pattern reward with optional idempotency.
type PatternRewardInput struct {
	TenantID       string
	Reward         float64
	Confidence     float64
	IdempotencyKey string
}

// ApplyPatternReward credits a pattern directly.
func (c *Client) ApplyPatternReward(ctx context.Context, patternID string, in PatternRewardInput) (Pattern, error) {
	tenant := c.resolveTenant(in.TenantID)
	body := map[string]any{
		"tenant_id":        tenant,
		"reward":           in.Reward,
		"confidence":       in.Confidence,
		"idempotency_key":  in.IdempotencyKey,
	}
	var p Pattern
	err := c.doJSON(ctx, "POST", "/api/v1/patterns/"+url.PathEscape(patternID)+"/reward", nil, body, &p)
	return p, err
}

// ProposeSkill derives a skill proposal from a pattern.
func (c *Client) ProposeSkill(ctx context.Context, tenantID, patternID string) (map[string]any, error) {
	tenant := c.resolveTenant(tenantID)
	body := map[string]any{"tenant_id": tenant}
	var out map[string]any
	err := c.doJSON(ctx, "POST", "/api/v1/patterns/"+url.PathEscape(patternID)+"/skill", nil, body, &out)
	return out, err
}

// GetSkill loads a skill by id.
func (c *Client) GetSkill(ctx context.Context, tenantID, skillID string) (Skill, error) {
	tenant := c.resolveTenant(tenantID)
	q := url.Values{"tenant_id": {tenant}}
	var sk Skill
	err := c.doJSON(ctx, "GET", "/api/v1/skills/"+url.PathEscape(skillID), q, nil, &sk)
	return sk, err
}
