package httpserver

import (
	"net/http"

	"github.com/agent-experience-engine/agent-experience-engine/internal/experience"
)

type proposeSkillRequest struct {
	TenantID string `json:"tenant_id"`
}

func (s *Server) handleProposeSkill(w http.ResponseWriter, r *http.Request) {
	if s.experiences == nil {
		writeError(w, http.StatusServiceUnavailable, "experience service not configured")
		return
	}
	var req proposeSkillRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	patternID := r.PathValue("id")
	res, err := s.experiences.ProposeSkill(r.Context(), req.TenantID, experience.ProposeSkillInput{
		PatternID: patternID,
	})
	if err != nil {
		s.writeExperienceError(w, r, err)
		return
	}
	status := http.StatusOK
	if res.Created {
		status = http.StatusCreated
	}
	body := map[string]any{
		"created": res.Created,
		"skipped": res.Skipped,
	}
	if res.Skill.ID != "" {
		body["skill"] = res.Skill
	}
	writeJSON(w, status, body)
}

func (s *Server) handleGetSkill(w http.ResponseWriter, r *http.Request) {
	if s.experiences == nil {
		writeError(w, http.StatusServiceUnavailable, "experience service not configured")
		return
	}
	tenantID := r.URL.Query().Get("tenant_id")
	id := r.PathValue("id")
	sk, err := s.experiences.GetSkill(r.Context(), tenantID, id)
	if err != nil {
		s.writeExperienceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, sk)
}

func (s *Server) handleGetPatternSkill(w http.ResponseWriter, r *http.Request) {
	if s.experiences == nil {
		writeError(w, http.StatusServiceUnavailable, "experience service not configured")
		return
	}
	tenantID := r.URL.Query().Get("tenant_id")
	patternID := r.PathValue("id")
	sk, err := s.experiences.GetSkillByPattern(r.Context(), tenantID, patternID)
	if err != nil {
		s.writeExperienceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, sk)
}
