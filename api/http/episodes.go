package httpserver

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/agent-experience-engine/agent-experience-engine/internal/attempt"
	"github.com/agent-experience-engine/agent-experience-engine/internal/episode"
	"github.com/agent-experience-engine/agent-experience-engine/internal/episodelearn"
	"github.com/agent-experience-engine/agent-experience-engine/internal/evaluator"
	"github.com/agent-experience-engine/agent-experience-engine/internal/experience"
	"github.com/agent-experience-engine/agent-experience-engine/internal/extractor"
	"github.com/agent-experience-engine/agent-experience-engine/internal/outcome"
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
	Episode               episode.Episode         `json:"episode"`
	Outcome               episode.Outcome         `json:"outcome"`
	LearningStatus        string                  `json:"learning_status,omitempty"`
	LearningError         string                  `json:"learning_error,omitempty"`
	ExperienceCandidates  []experience.Candidate  `json:"experience_candidates,omitempty"`
	StoredExperiences     []experience.Experience `json:"stored_experiences,omitempty"`
	ReinforcedExperiences []experience.Experience `json:"reinforced_experiences,omitempty"`
}

type retryLearningRequest struct {
	TenantID string `json:"tenant_id"`
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

	resp := completeOutcomeResponse{Episode: ep, Outcome: out}
	if s.learning != nil {
		attempts, listErr := s.episodes.ListAttempts(r.Context(), req.TenantID, ep.ID)
		if listErr != nil {
			s.logger.Error("list attempts for learning failed",
				slog.String("request_id", requestIDFrom(r.Context())),
				slog.String("tenant_id", req.TenantID),
				slog.String("episode_id", ep.ID),
				slog.String("error", listErr.Error()),
			)
			writeJSON(w, http.StatusInternalServerError, map[string]any{
				"error":   fmt.Sprintf("list attempts for learning on episode %s: %v", ep.ID, listErr),
				"episode": ep,
				"outcome": out,
			})
			return
		}
		learned, learnErr := s.learning.Process(r.Context(), episodelearn.ProcessInput{
			TenantID: req.TenantID,
			Episode:  ep,
			Attempts: attempts,
			Outcome:  out,
		})
		if learnErr != nil {
			s.logger.Error("episode learning failed",
				slog.String("request_id", requestIDFrom(r.Context())),
				slog.String("tenant_id", req.TenantID),
				slog.String("episode_id", ep.ID),
				slog.String("error", learnErr.Error()),
			)
			resp.LearningStatus = string(episodelearn.StatusFailed)
			resp.LearningError = learnErr.Error()
		} else {
			resp.ExperienceCandidates = learned.Candidates
			resp.StoredExperiences = learned.Stored
			resp.ReinforcedExperiences = learned.Reinforced
			resp.LearningStatus = string(learned.LearningStatus)
			resp.LearningError = learned.LearningLastError
		}
		writeJSON(w, http.StatusCreated, resp)
		return
	}

	if s.extractor != nil {
		attempts, listErr := s.episodes.ListAttempts(r.Context(), req.TenantID, ep.ID)
		if listErr != nil {
			s.logger.Error("list attempts for extraction failed",
				slog.String("request_id", requestIDFrom(r.Context())),
				slog.String("tenant_id", req.TenantID),
				slog.String("episode_id", ep.ID),
				slog.String("error", listErr.Error()),
			)
			writeJSON(w, http.StatusInternalServerError, map[string]any{
				"error":   fmt.Sprintf("list attempts for extraction on episode %s: %v", ep.ID, listErr),
				"episode": ep,
				"outcome": out,
			})
			return
		}
		candidates, extractErr := s.extractor.Extract(r.Context(), extractor.ExtractInput{
			Episode:  ep,
			Attempts: attempts,
			Outcome:  out,
		})
		if extractErr != nil {
			s.logger.Error("experience extraction failed",
				slog.String("request_id", requestIDFrom(r.Context())),
				slog.String("tenant_id", req.TenantID),
				slog.String("episode_id", ep.ID),
				slog.String("error", extractErr.Error()),
			)
			writeJSON(w, http.StatusInternalServerError, map[string]any{
				"error":   extractErr.Error(),
				"episode": ep,
				"outcome": out,
			})
			return
		}
		resp.ExperienceCandidates = candidates
		if s.storePipeline != nil {
			ev := evaluator.FromAttempts(ep.ID, out.ID, attempts)
			stored, storeErr := s.storePipeline.StoreCandidatesWithOptions(r.Context(), req.TenantID, ep.ID, candidates, experience.StoreOptions{
				Outcome:  outcome.Outcome(out),
				Evidence: ev,
			})
			if storeErr != nil {
				s.logger.Error("experience store failed",
					slog.String("request_id", requestIDFrom(r.Context())),
					slog.String("tenant_id", req.TenantID),
					slog.String("episode_id", ep.ID),
					slog.String("error", storeErr.Error()),
				)
				writeJSON(w, http.StatusInternalServerError, map[string]any{
					"error":                 storeErr.Error(),
					"episode":               ep,
					"outcome":               out,
					"experience_candidates": candidates,
				})
				return
			}
			resp.StoredExperiences = stored.Stored
			resp.ReinforcedExperiences = stored.Reinforced
		}
	}

	writeJSON(w, http.StatusCreated, resp)
}

func (s *Server) handleRetryEpisodeLearning(w http.ResponseWriter, r *http.Request) {
	if s.episodes == nil || s.learning == nil {
		writeError(w, http.StatusServiceUnavailable, "episode learning not configured")
		return
	}

	var req retryLearningRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}

	episodeID := r.PathValue("id")
	ep, err := s.episodes.GetEpisode(r.Context(), req.TenantID, episodeID)
	if err != nil {
		s.writeEpisodeError(w, r, err)
		return
	}
	if !ep.Status.Terminal() {
		writeError(w, http.StatusBadRequest, "episode is not completed")
		return
	}

	attempts, err := s.episodes.ListAttempts(r.Context(), req.TenantID, episodeID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "list attempts failed")
		return
	}

	out, err := s.episodes.GetOutcome(r.Context(), req.TenantID, episodeID)
	if err != nil {
		s.writeEpisodeError(w, r, err)
		return
	}

	learned, err := s.learning.Retry(r.Context(), req.TenantID, ep, attempts, out)
	if err != nil {
		if errors.Is(err, episodelearn.ErrNotFound) {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]any{
			"error":           err.Error(),
			"learning_status": string(episodelearn.StatusFailed),
		})
		return
	}

	writeJSON(w, http.StatusOK, completeOutcomeResponse{
		Episode:               ep,
		Outcome:               out,
		LearningStatus:        string(learned.LearningStatus),
		LearningError:         learned.LearningLastError,
		ExperienceCandidates:  learned.Candidates,
		StoredExperiences:     learned.Stored,
		ReinforcedExperiences: learned.Reinforced,
	})
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
