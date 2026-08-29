package httpserver

import (
	"net/http"

	"github.com/agent-experience-engine/agent-experience-engine/internal/contextx"
)

type contextRequest struct {
	TenantID       string   `json:"tenant_id"`
	AgentID        string   `json:"agent_id"`
	UserID         string   `json:"user_id"`
	Task           string   `json:"task"`
	Tools          []string `json:"tools"`
	MaxExperiences int      `json:"max_experiences"`
	MaxTokens      int      `json:"max_tokens"`
	TopK           int      `json:"top_k"`
}

type contextExperienceResponse struct {
	Type       string  `json:"type"`
	Content    string  `json:"content"`
	Source     string  `json:"source"`
	Confidence float64 `json:"confidence"`
}

type selectionResponse struct {
	ExperienceID string  `json:"experience_id"`
	Decision     string  `json:"decision"`
	Reason       string  `json:"reason"`
	FinalScore   float64 `json:"final_score"`
}

func (s *Server) handleBuildContext(w http.ResponseWriter, r *http.Request) {
	if s.contexts == nil {
		writeError(w, http.StatusServiceUnavailable, "context service not configured")
		return
	}

	var req contextRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}

	resp, err := s.contexts.BuildContext(r.Context(), contextx.Request{
		TenantID:       req.TenantID,
		AgentID:        req.AgentID,
		UserID:         req.UserID,
		Task:           req.Task,
		Tools:          req.Tools,
		MaxExperiences: req.MaxExperiences,
		MaxTokens:      req.MaxTokens,
		TopK:           req.TopK,
	})
	if err != nil {
		s.logger.Error("build context failed",
			"request_id", requestIDFrom(r.Context()),
			"tenant_id", req.TenantID,
			"error", err.Error(),
		)
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	experiences := make([]contextExperienceResponse, 0, len(resp.Context.Experiences))
	for _, item := range resp.Context.Experiences {
		experiences = append(experiences, contextExperienceResponse{
			Type:       item.Type,
			Content:    item.Content,
			Source:     item.Source,
			Confidence: item.Confidence,
		})
	}

	selections := make([]selectionResponse, 0, len(resp.Selections))
	for _, sel := range resp.Selections {
		selections = append(selections, selectionResponse{
			ExperienceID: sel.Experience.Experience.ID,
			Decision:     string(sel.Decision),
			Reason:       sel.Reason,
			FinalScore:   sel.Experience.Score.FinalScore,
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"disclaimer":  resp.Context.Disclaimer,
		"experiences": experiences,
		"selections":  selections,
	})
}
