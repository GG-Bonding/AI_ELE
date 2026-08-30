package httpserver

import (
	"errors"
	"net/http"

	"github.com/agent-experience-engine/agent-experience-engine/internal/feedback"
)

type submitFeedbackRequest struct {
	TenantID       string            `json:"tenant_id"`
	EpisodeID      string            `json:"episode_id"`
	Source         string            `json:"source"`
	Signal         string            `json:"signal"`
	Reward         *float64          `json:"reward"`
	Confidence     float64           `json:"confidence"`
	Evidence       string            `json:"evidence"`
	Target         *feedback.Target  `json:"target"`
	IdempotencyKey string            `json:"idempotency_key"`
}

func (s *Server) handleSubmitFeedback(w http.ResponseWriter, r *http.Request) {
	if s.feedbacks == nil {
		writeError(w, http.StatusServiceUnavailable, "feedback service not configured")
		return
	}

	var req submitFeedbackRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}

	res, err := s.feedbacks.Submit(r.Context(), feedback.SubmitInput{
		TenantID:       req.TenantID,
		EpisodeID:      req.EpisodeID,
		Source:         req.Source,
		Signal:         req.Signal,
		Reward:         req.Reward,
		Confidence:     req.Confidence,
		Evidence:       req.Evidence,
		Target:         req.Target,
		IdempotencyKey: req.IdempotencyKey,
	})
	if err != nil {
		s.writeFeedbackError(w, r, err)
		return
	}
	status := http.StatusCreated
	if res.IdempotentReplay {
		status = http.StatusOK
	}
	writeJSON(w, status, res)
}

func (s *Server) handleGetEpisodeReward(w http.ResponseWriter, r *http.Request) {
	if s.feedbacks == nil {
		writeError(w, http.StatusServiceUnavailable, "feedback service not configured")
		return
	}
	tenantID := r.URL.Query().Get("tenant_id")
	episodeID := r.PathValue("id")
	agg, rows, err := s.feedbacks.GetEpisodeReward(r.Context(), tenantID, episodeID)
	if err != nil {
		s.writeFeedbackError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"episode_reward": agg,
		"feedbacks":      rows,
	})
}

func (s *Server) writeFeedbackError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, feedback.ErrInvalidInput):
		writeError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, feedback.ErrNotFound):
		writeError(w, http.StatusNotFound, err.Error())
	default:
		s.logger.Error("feedback handler error",
			"request_id", requestIDFrom(r.Context()),
			"path", r.URL.Path,
			"error", err.Error(),
		)
		writeError(w, http.StatusInternalServerError, "internal error")
	}
}
