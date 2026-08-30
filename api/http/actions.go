package httpserver

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/agent-experience-engine/agent-experience-engine/internal/action"
)

type recordActionRequest struct {
	TenantID  string          `json:"tenant_id"`
	Type      string          `json:"type"`
	ToolName  string          `json:"tool_name"`
	Input     json.RawMessage `json:"input"`
	Output    json.RawMessage `json:"output"`
	Status    string          `json:"status"`
	AttemptID string          `json:"attempt_id"`
	Sequence  int             `json:"sequence"`
}

type linkExperienceRequest struct {
	TenantID     string   `json:"tenant_id"`
	ExperienceID string   `json:"experience_id"`
	Influence    *float64 `json:"influence"`
	Evidence     string   `json:"evidence"`
}

func (s *Server) handleRecordAction(w http.ResponseWriter, r *http.Request) {
	if s.actions == nil {
		writeError(w, http.StatusServiceUnavailable, "action service not configured")
		return
	}

	var req recordActionRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}

	a, err := s.actions.RecordAction(r.Context(), action.RecordInput{
		TenantID:  req.TenantID,
		EpisodeID: r.PathValue("id"),
		Type:      action.Type(req.Type),
		ToolName:  req.ToolName,
		Input:     req.Input,
		Output:    req.Output,
		Status:    action.Status(req.Status),
		AttemptID: req.AttemptID,
		Sequence:  req.Sequence,
	})
	if err != nil {
		s.writeActionError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, a)
}

func (s *Server) handleListActions(w http.ResponseWriter, r *http.Request) {
	if s.actions == nil {
		writeError(w, http.StatusServiceUnavailable, "action service not configured")
		return
	}

	tenantID := r.URL.Query().Get("tenant_id")
	actions, err := s.actions.ListActions(r.Context(), tenantID, r.PathValue("id"))
	if err != nil {
		s.writeActionError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"actions": actions})
}

func (s *Server) handleLinkExperienceToAction(w http.ResponseWriter, r *http.Request) {
	if s.actions == nil {
		writeError(w, http.StatusServiceUnavailable, "action service not configured")
		return
	}

	var req linkExperienceRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}

	link, err := s.actions.LinkExperience(r.Context(), action.LinkInput{
		TenantID:     req.TenantID,
		EpisodeID:    r.PathValue("id"),
		ActionID:     r.PathValue("action_id"),
		ExperienceID: req.ExperienceID,
		Influence:    req.Influence,
		Evidence:     req.Evidence,
	})
	if err != nil {
		s.writeActionError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, link)
}

func (s *Server) handleListActionLinks(w http.ResponseWriter, r *http.Request) {
	if s.actions == nil {
		writeError(w, http.StatusServiceUnavailable, "action service not configured")
		return
	}

	tenantID := r.URL.Query().Get("tenant_id")
	links, err := s.actions.ListLinks(r.Context(), tenantID, r.PathValue("id"))
	if err != nil {
		s.writeActionError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"links": links})
}

func (s *Server) writeActionError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, action.ErrEpisodeNotFound), errors.Is(err, action.ErrNotFound):
		writeError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, action.ErrInvalidInput):
		writeError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, action.ErrDuplicateLink):
		writeError(w, http.StatusConflict, err.Error())
	default:
		s.logger.Error("action handler failed",
			"request_id", requestIDFrom(r.Context()),
			"error", err.Error(),
		)
		writeError(w, http.StatusInternalServerError, "internal error")
	}
}
