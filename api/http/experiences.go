package httpserver

import (
	"errors"
	"net/http"

	"github.com/agent-experience-engine/agent-experience-engine/internal/experience"
	"github.com/agent-experience-engine/agent-experience-engine/internal/retrieval"
)

type searchExperiencesRequest struct {
	TenantID string   `json:"tenant_id"`
	Task     string   `json:"task"`
	AgentID  string   `json:"agent_id"`
	UserID   string   `json:"user_id"`
	Types    []string `json:"types"`
	Scopes   []string `json:"scopes"`
	ScopeKey string   `json:"scope_key"`
	Tools    []string `json:"tools"`
	TopK     int      `json:"top_k"`
}

type scoredExperienceResponse struct {
	ID         string                   `json:"id"`
	Type       string                   `json:"type"`
	Scope      string                   `json:"scope"`
	ScopeKey   string                   `json:"scope_key,omitempty"`
	Trigger    string                   `json:"trigger"`
	Content    string                   `json:"content"`
	Confidence float64                  `json:"confidence"`
	Utility    float64                  `json:"utility"`
	Status     string                   `json:"status"`
	Score      retrieval.ScoreBreakdown `json:"score"`
}

func (s *Server) handleGetExperience(w http.ResponseWriter, r *http.Request) {
	if s.experiences == nil {
		writeError(w, http.StatusServiceUnavailable, "experience service not configured")
		return
	}
	tenantID := r.URL.Query().Get("tenant_id")
	id := r.PathValue("id")
	exp, err := s.experiences.Get(r.Context(), tenantID, id)
	if err != nil {
		s.writeExperienceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, exp)
}

func (s *Server) handleSearchExperiences(w http.ResponseWriter, r *http.Request) {
	if s.retriever == nil {
		writeError(w, http.StatusServiceUnavailable, "experience retrieval not configured")
		return
	}

	var req searchExperiencesRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}

	types := make([]experience.Type, 0, len(req.Types))
	for _, t := range req.Types {
		types = append(types, experience.Type(t))
	}
	scopes := make([]experience.Scope, 0, len(req.Scopes))
	for _, sc := range req.Scopes {
		scopes = append(scopes, experience.Scope(sc))
	}

	results, err := s.retriever.Retrieve(r.Context(), retrieval.Query{
		TenantID: req.TenantID,
		Task:     req.Task,
		AgentID:  req.AgentID,
		UserID:   req.UserID,
		Types:    types,
		Scopes:   scopes,
		ScopeKey: req.ScopeKey,
		Tools:    req.Tools,
		TopK:     req.TopK,
	})
	if err != nil {
		s.writeExperienceError(w, r, err)
		return
	}

	out := make([]scoredExperienceResponse, 0, len(results))
	for _, item := range results {
		out = append(out, scoredExperienceResponse{
			ID:         item.Experience.ID,
			Type:       string(item.Experience.Type),
			Scope:      string(item.Experience.Scope),
			ScopeKey:   item.Experience.ScopeKey,
			Trigger:    item.Experience.Trigger,
			Content:    item.Experience.Content,
			Confidence: item.Experience.Confidence,
			Utility:    item.Experience.Utility,
			Status:     string(item.Experience.Status),
			Score:      item.Score,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"experiences": out})
}

func (s *Server) writeExperienceError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, experience.ErrInvalidInput):
		writeError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, experience.ErrNotFound):
		writeError(w, http.StatusNotFound, err.Error())
	default:
		s.logger.Error("experience handler error",
			"request_id", requestIDFrom(r.Context()),
			"path", r.URL.Path,
			"error", err.Error(),
		)
		writeError(w, http.StatusInternalServerError, "internal error")
	}
}
