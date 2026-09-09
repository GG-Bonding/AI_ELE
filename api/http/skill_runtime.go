package httpserver

import (
	"math/rand"
	"net/http"
	"strings"
	"time"

	"github.com/agent-experience-engine/agent-experience-engine/internal/auth"
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
	RequesterID    string         `json:"requester_id"`
}

type retrieveSkillsRequest struct {
	TenantID string   `json:"tenant_id"`
	Task     string   `json:"task"`
	Tools    []string `json:"tools"`
	TopK     int      `json:"top_k"`
	Select   string   `json:"select"` // empty | "thompson"
}

type reviseSkillRequest struct {
	TenantID        string   `json:"tenant_id"`
	SkillID         string   `json:"skill_id"`
	VersionID       string   `json:"version_id"`
	PatternID       string   `json:"pattern_id"`
	FailureCodes    []string `json:"failure_codes"`
	FailureMessages []string `json:"failure_messages"`
	PatternContent  string   `json:"pattern_content"`
}

type abCompareRequest struct {
	VersionAID string              `json:"version_a_id"`
	VersionBID string              `json:"version_b_id"`
	TrialsA    []skill.ShadowTrial `json:"trials_a"`
	TrialsB    []skill.ShadowTrial `json:"trials_b"`
	MinTrials  int                 `json:"min_trials"`
	Promote    bool                `json:"promote"`
	TenantID   string              `json:"tenant_id"`
}

type resumeSkillRequest struct {
	TenantID       string   `json:"tenant_id"`
	AvailableTools []string `json:"available_tools"`
}

type approvalActionRequest struct {
	TenantID   string `json:"tenant_id"`
	Reason     string `json:"reason"`
	ApprovedBy string `json:"approved_by"`
	ActorID    string `json:"actor_id"`
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
	requesterID := req.RequesterID
	tenantID := req.TenantID
	if p, ok := auth.FromContext(r.Context()); ok {
		requesterID = p.ActorID
		if p.TenantID != "" {
			tenantID = p.TenantID
		}
	}
	ex, steps, err := s.skillExec.Execute(r.Context(), skill.ExecuteInput{
		TenantID:       tenantID,
		EpisodeID:      req.EpisodeID,
		SkillID:        req.SkillID,
		VersionID:      req.VersionID,
		Mode:           mode,
		Inputs:         req.Inputs,
		AvailableTools: req.AvailableTools,
		IdempotencyKey: req.IdempotencyKey,
		RequesterID:    requesterID,
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
	approvedBy := firstNonEmpty(req.ApprovedBy, req.ActorID)
	tenantID := req.TenantID
	if p, ok := auth.FromContext(r.Context()); ok {
		// Trusted principal wins — body approved_by is ignored (anti-spoof).
		approvedBy = p.ActorID
		if p.TenantID != "" {
			tenantID = p.TenantID
		}
		if s.requireSeparateApprover && !p.CanApprove() {
			writeError(w, http.StatusForbidden, "principal lacks skill:approve permission")
			return
		}
	} else if s.requireSeparateApprover && approvedBy == "" {
		writeError(w, http.StatusUnauthorized, "authenticated principal required")
		return
	}
	appr, err := s.skillExec.ApproveApproval(r.Context(), tenantID, r.PathValue("approval_id"),
		approvedBy, s.requireSeparateApprover)
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
	retriever := s.skillRetriever
	if retriever == nil {
		retriever = &skill.Retriever{Repo: s.skillRepo, Tools: tools}
	}
	ranked, err := retriever.Retrieve(r.Context(), skill.RetrieveQuery{
		TenantID: req.TenantID, Task: req.Task, Tools: req.Tools, TopK: req.TopK,
	})
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	out := map[string]any{"skills": ranked}
	selectMode := strings.TrimSpace(req.Select)
	if selectMode == "" {
		selectMode = "policy" // use configured default SelectionPolicy
	}
	if strings.EqualFold(selectMode, "thompson") || strings.EqualFold(selectMode, "policy") ||
		strings.EqualFold(selectMode, "greedy") || strings.EqualFold(selectMode, "epsilon_greedy") {
		var picked skill.RankedSkill
		var ok bool
		switch strings.ToLower(selectMode) {
		case "thompson":
			picked, ok = skill.SelectThompson(ranked, rand.New(rand.NewSource(time.Now().UnixNano())))
		case "greedy":
			picked, ok = skill.GreedyPolicy{}.Select(ranked, nil)
		case "epsilon_greedy":
			picked, ok = skill.EpsilonGreedyPolicy{Epsilon: 0.1}.Select(ranked, rand.New(rand.NewSource(time.Now().UnixNano())))
		default:
			picked, ok = retriever.Select(ranked, rand.New(rand.NewSource(time.Now().UnixNano())))
		}
		if ok {
			out["selected"] = picked
		}
	}
	writeJSON(w, http.StatusOK, out)
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func (s *Server) handleReviseSkill(w http.ResponseWriter, r *http.Request) {
	if s.skillRepo == nil {
		writeError(w, http.StatusServiceUnavailable, "skill runtime not enabled")
		return
	}
	var req reviseSkillRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	ver, err := s.skillRepo.GetVersion(r.Context(), req.TenantID, req.VersionID)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	skillID := req.SkillID
	if skillID == "" {
		skillID = ver.SkillID
	}
	rev, ok, err := skill.AutoRevise(r.Context(), s.skillRepo, req.TenantID, skillID, req.PatternID, ver.Spec, skill.RevisionHint{
		FailureCodes:    req.FailureCodes,
		FailureMessages: req.FailureMessages,
		PatternContent:  req.PatternContent,
	})
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"revised": ok, "version": rev})
}

func (s *Server) handleCompareShadowAB(w http.ResponseWriter, r *http.Request) {
	if s.skillRegistry == nil {
		writeError(w, http.StatusServiceUnavailable, "skill runtime not enabled")
		return
	}
	var req abCompareRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	result := skill.CompareShadowAB(req.VersionAID, req.VersionBID, req.TrialsA, req.TrialsB, req.MinTrials)
	out := map[string]any{"result": result}
	if req.Promote && result.WinnerID != "" {
		ver, err := skill.PromoteABWinner(r.Context(), s.skillRegistry, req.TenantID, result.WinnerID, s.skillPromote)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		out["promoted"] = ver
	}
	writeJSON(w, http.StatusOK, out)
}
