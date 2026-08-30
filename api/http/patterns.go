package httpserver

import (
	"net/http"

	"github.com/agent-experience-engine/agent-experience-engine/internal/experience"
)

type generalizeRequest struct {
	TenantID      string   `json:"tenant_id"`
	ExperienceIDs []string `json:"experience_ids"`
}

func (s *Server) handleGeneralize(w http.ResponseWriter, r *http.Request) {
	if s.experiences == nil {
		writeError(w, http.StatusServiceUnavailable, "experience service not configured")
		return
	}
	var req generalizeRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	res, err := s.experiences.Generalize(r.Context(), req.TenantID, experience.GeneralizeInput{
		ExperienceIDs: req.ExperienceIDs,
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
	if res.Pattern.ID != "" {
		body["pattern"] = res.Pattern
	}
	writeJSON(w, status, body)
}

func (s *Server) handleGetPattern(w http.ResponseWriter, r *http.Request) {
	if s.experiences == nil {
		writeError(w, http.StatusServiceUnavailable, "experience service not configured")
		return
	}
	tenantID := r.URL.Query().Get("tenant_id")
	id := r.PathValue("id")
	p, err := s.experiences.GetPattern(r.Context(), tenantID, id)
	if err != nil {
		s.writeExperienceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (s *Server) handleListPatternEvidence(w http.ResponseWriter, r *http.Request) {
	if s.experiences == nil {
		writeError(w, http.StatusServiceUnavailable, "experience service not configured")
		return
	}
	tenantID := r.URL.Query().Get("tenant_id")
	id := r.PathValue("id")
	ev, err := s.experiences.ListPatternEvidence(r.Context(), tenantID, id)
	if err != nil {
		s.writeExperienceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"evidence": ev})
}
