package httpserver

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/agent-experience-engine/agent-experience-engine/internal/attempt"
	"github.com/agent-experience-engine/agent-experience-engine/internal/episode"
)

type createEpisodeRequest struct {
	TenantID string `json:"tenant_id"`
	AgentID  string `json:"agent_id"`
	UserID   string `json:"user_id"`
	TaskType string `json:"task_type"`
	Goal     string `json:"goal"`
	Input    string `json:"input"`
}

type addAttemptRequest struct {
	TenantID     string          `json:"tenant_id"`
	Hypothesis   string          `json:"hypothesis"`
	Action       string          `json:"action"`
	ToolName     string          `json:"tool_name"`
	Input        json.RawMessage `json:"input"`
	Output       json.RawMessage `json:"output"`
	Status       string          `json:"status"`
	ErrorCode    string          `json:"error_code"`
	ErrorMessage string          `json:"error_message"`
	Sequence     int             `json:"sequence"`
}

type completeOutcomeRequest struct {
	TenantID string             `json:"tenant_id"`
	Status   string             `json:"status"`
	Result   json.RawMessage    `json:"result"`
	Verified bool               `json:"verified"`
	Verifier string             `json:"verifier"`
	Metrics  map[string]float64 `json:"metrics"`
}

type completeOutcomeResponse struct {
	Episode episode.Episode `json:"episode"`
	Outcome episode.Outcome `json:"outcome"`
}

func (s *Server) handleCreateEpisode(w http.ResponseWriter, r *http.Request) {
	if s.episodes == nil {
		writeError(w, http.StatusServiceUnavailable, "episode service not configured")
		return
	}

	var req createEpisodeRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}

	ep, err := s.episodes.CreateEpisode(r.Context(), episode.CreateEpisodeInput{
		TenantID: req.TenantID,
		AgentID:  req.AgentID,
		UserID:   req.UserID,
		TaskType: req.TaskType,
		Goal:     req.Goal,
		Input:    req.Input,
	})
	if err != nil {
		s.writeEpisodeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, ep)
}

func (s *Server) handleGetEpisode(w http.ResponseWriter, r *http.Request) {
	if s.episodes == nil {
		writeError(w, http.StatusServiceUnavailable, "episode service not configured")
		return
	}

	tenantID := r.URL.Query().Get("tenant_id")
	id := r.PathValue("id")
	ep, err := s.episodes.GetEpisode(r.Context(), tenantID, id)
	if err != nil {
		s.writeEpisodeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, ep)
}

func (s *Server) handleAddAttempt(w http.ResponseWriter, r *http.Request) {
	if s.episodes == nil {
		writeError(w, http.StatusServiceUnavailable, "episode service not configured")
		return
	}

	var req addAttemptRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}

	status := attempt.Status(req.Status)
	if req.Status == "" {
		status = attempt.StatusSuccess
	}

	a, err := s.episodes.AddAttempt(r.Context(), episode.AddAttemptInput{
		TenantID:     req.TenantID,
		EpisodeID:    r.PathValue("id"),
		Hypothesis:   req.Hypothesis,
		Action:       req.Action,
		ToolName:     req.ToolName,
		Input:        req.Input,
		Output:       req.Output,
		Status:       status,
		ErrorCode:    req.ErrorCode,
		ErrorMessage: req.ErrorMessage,
		Sequence:     req.Sequence,
	})
	if err != nil {
		s.writeEpisodeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, a)
}

func (s *Server) handleCompleteOutcome(w http.ResponseWriter, r *http.Request) {
	if s.episodes == nil {
		writeError(w, http.StatusServiceUnavailable, "episode service not configured")
		return
	}

	var req completeOutcomeRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}

	ep, out, err := s.episodes.CompleteEpisode(r.Context(), episode.CompleteEpisodeInput{
		TenantID:  req.TenantID,
		EpisodeID: r.PathValue("id"),
		Status:    episode.Status(req.Status),
		Result:    req.Result,
		Verified:  req.Verified,
		Verifier:  req.Verifier,
		Metrics:   req.Metrics,
	})
	if err != nil {
		s.writeEpisodeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, completeOutcomeResponse{Episode: ep, Outcome: out})
}

func decodeJSON(r *http.Request, dst any) error {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	return dec.Decode(dst)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func (s *Server) writeEpisodeError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, episode.ErrInvalidInput):
		writeError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, episode.ErrNotFound):
		writeError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, episode.ErrAlreadyCompleted), errors.Is(err, episode.ErrOutcomeExists):
		writeError(w, http.StatusConflict, err.Error())
	default:
		s.logger.Error("episode handler error",
			slog.String("request_id", requestIDFrom(r.Context())),
			slog.String("path", r.URL.Path),
			slog.String("error", err.Error()),
		)
		writeError(w, http.StatusInternalServerError, "internal error")
	}
}
