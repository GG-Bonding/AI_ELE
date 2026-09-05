package httpserver

import (
	"net/http"
	"strings"

	"github.com/agent-experience-engine/agent-experience-engine/internal/skill"
	"github.com/agent-experience-engine/agent-experience-engine/internal/toolregistry"
)

type compileSkillRequest struct {
	TenantID    string  `json:"tenant_id"`
	Name        string  `json:"name"`
	Description string  `json:"description"`
	PatternID   string  `json:"pattern_id"`
	SpecYAML    string  `json:"spec_yaml"`
	Confidence  float64 `json:"confidence"`
	Utility     float64 `json:"utility"`
}

type executeSkillRequest struct {
	TenantID       string         `json:"tenant_id"`
	EpisodeID      string         `json:"episode_id"`
	SkillID        string         `json:"skill_id"`
	VersionID      string         `json:"version_id"`
	Mode           string         `json:"mode"`
	Inputs         map[string]any `json:"inputs"`
	AvailableTools []string       `json:"available_tools"`
	IdempotencyKey string         `json:"idempotency_key"`
}

type retrieveSkillsRequest struct {
	TenantID string   `json:"tenant_id"`
	Task     string   `json:"task"`
	Tools    []string `json:"tools"`
	TopK     int      `json:"top_k"`
}

type resumeSkillRequest struct {
	TenantID       string   `json:"tenant_id"`
	AvailableTools []string `json:"available_tools"`
}

type approvalActionRequest struct {
	TenantID string `json:"tenant_id"`
	Reason   string `json:"reason"`
}

func (s *Server) handleCompileSkill(w http.ResponseWriter, r *http.Request) {
	if s.skillRegistry == nil {
		writeError(w, http.StatusServiceUnavailable, "skill runtime not enabled")
		return
	}
	var req compileSkillRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	sk, ver, rep, err := s.skillRegistry.CompileAndCreate(
		r.Context(), req.TenantID, req.Name, req.Description, req.PatternID, req.SpecYAML, req.Confidence, req.Utility,
	)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	status := http.StatusCreated
	if !rep.OK {
		status = http.StatusUnprocessableEntity
	}
	writeJSON(w, status, map[string]any{
		"skill": sk, "version": ver, "validation_ok": rep.OK,
		"computed_risk": rep.ComputedRisk, "requires_approval": rep.RequiresApproval,
	})
}

func (s *Server) handleShadowSkillVersion(w http.ResponseWriter, r *http.Request) {
	if s.skillRegistry == nil {
		writeError(w, http.StatusServiceUnavailable, "skill runtime not enabled")
		return
	}
	tenantID := r.URL.Query().Get("tenant_id")
	ver, err := s.skillRegistry.MoveToShadow(r.Context(), tenantID, r.PathValue("version_id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, ver)
}

func (s *Server) handleActivateSkillVersion(w http.ResponseWriter, r *http.Request) {
	if s.skillRegistry == nil {
		writeError(w, http.StatusServiceUnavailable, "skill runtime not enabled")
		return
	}
	tenantID := r.URL.Query().Get("tenant_id")
	ver, err := s.skillRegistry.Activate(r.Context(), tenantID, r.PathValue("version_id"), s.skillPromote)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, ver)
}

func (s *Server) handleExecuteSkill(w http.ResponseWriter, r *http.Request) {
	if s.skillExec == nil {
		writeError(w, http.StatusServiceUnavailable, "skill runtime not enabled")
		return
	}
	var req executeSkillRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	mode := skill.ExecutionMode(strings.ToUpper(strings.TrimSpace(req.Mode)))
	if mode == "" {
		mode = skill.ModeShadow
	}
	ex, steps, err := s.skillExec.Execute(r.Context(), skill.ExecuteInput{
		TenantID:       req.TenantID,
		EpisodeID:      req.EpisodeID,
		SkillID:        req.SkillID,
		VersionID:      req.VersionID,
		Mode:           mode,
		Inputs:         req.Inputs,
		AvailableTools: req.AvailableTools,
		IdempotencyKey: req.IdempotencyKey,
		RuntimeEnabled: true,
	})
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"execution": ex, "steps": steps})
}

func (s *Server) handleResumeSkillExecution(w http.ResponseWriter, r *http.Request) {
	if s.skillExec == nil {
		writeError(w, http.StatusServiceUnavailable, "skill runtime not enabled")
		return
	}
	var req resumeSkillRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	ex, steps, err := s.skillExec.Resume(r.Context(), req.TenantID, r.PathValue("execution_id"), req.AvailableTools, true)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"execution": ex, "steps": steps})
}

func (s *Server) handleApproveSkillApproval(w http.ResponseWriter, r *http.Request) {
	if s.skillExec == nil {
		writeError(w, http.StatusServiceUnavailable, "skill runtime not enabled")
		return
	}
	var req approvalActionRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	appr, err := s.skillExec.ApproveApproval(r.Context(), req.TenantID, r.PathValue("approval_id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, appr)
}

func (s *Server) handleRejectSkillApproval(w http.ResponseWriter, r *http.Request) {
	if s.skillExec == nil {
		writeError(w, http.StatusServiceUnavailable, "skill runtime not enabled")
		return
	}
	var req approvalActionRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	appr, err := s.skillExec.RejectApproval(r.Context(), req.TenantID, r.PathValue("approval_id"), req.Reason)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, appr)
}

func (s *Server) handleRetrieveSkills(w http.ResponseWriter, r *http.Request) {
	if s.skillRepo == nil {
		writeError(w, http.StatusServiceUnavailable, "skill runtime not enabled")
		return
	}
	tools := s.toolRegistry
	if tools == nil {
		tools = toolregistry.Default()
	}
	var req retrieveSkillsRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	ranked, err := skill.Retrieve(r.Context(), s.skillRepo, tools, skill.RetrieveQuery{
		TenantID: req.TenantID, Task: req.Task, Tools: req.Tools, TopK: req.TopK,
	})
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"skills": ranked})
}
