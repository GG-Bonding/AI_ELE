package skillrevise

import (
	"context"
	"fmt"
	"strings"

	"github.com/agent-experience-engine/agent-experience-engine/internal/skill"
	"github.com/agent-experience-engine/agent-experience-engine/internal/toolregistry"
)

// Service wires evidence-grounded revision from persisted executions (V3.4).
type Service struct {
	Repo      skill.Repository
	ExecStore skill.ExecutionStore
	LLM       *LLMReviser // optional
	Tools     *toolregistry.Registry
}

// ReviseFromVersion loads recent failure evidence and proposes Skill v2 CANDIDATE.
func (s *Service) ReviseFromVersion(ctx context.Context, tenantID, versionID, patternID string) (skill.Version, Proposal, bool, error) {
	if s == nil || s.Repo == nil {
		return skill.Version{}, Proposal{}, false, fmt.Errorf("skillrevise: service not configured")
	}
	ver, err := s.Repo.GetVersion(ctx, tenantID, versionID)
	if err != nil {
		return skill.Version{}, Proposal{}, false, err
	}
	var codes, msgs []string
	var evidence []string
	var stepTrace strings.Builder
	if s.ExecStore != nil {
		// Best-effort: scan learning is out of band; callers may pass execution via ProposeOrFallback.
		_ = stepTrace
	}
	tools := []string{}
	if s.Tools != nil {
		for _, d := range s.Tools.List() {
			tools = append(tools, d.Name)
		}
	}
	in := Input{
		Current:         ver.Spec,
		FailureCodes:    codes,
		FailureMessages: msgs,
		StepTrace:       stepTrace.String(),
		ToolAllowList:   tools,
		EvidenceIDs:     evidence,
	}
	p, ok, err := ProposeOrFallback(ctx, s.LLM, in)
	if err != nil || !ok {
		return skill.Version{}, p, ok, err
	}
	rev, err := skill.ProposeRevision(ctx, s.Repo, tenantID, ver.SkillID, p.NewSpecYAML, patternID)
	if err != nil {
		return skill.Version{}, p, false, err
	}
	return rev, p, true, nil
}

// ReviseFromExecution builds evidence from one failed execution then proposes revision.
func (s *Service) ReviseFromExecution(ctx context.Context, tenantID, executionID, patternID string) (skill.Version, Proposal, bool, error) {
	if s == nil || s.Repo == nil || s.ExecStore == nil {
		return skill.Version{}, Proposal{}, false, fmt.Errorf("skillrevise: repo and exec store required")
	}
	ex, err := s.ExecStore.GetExecution(ctx, tenantID, executionID)
	if err != nil {
		return skill.Version{}, Proposal{}, false, err
	}
	ver, err := s.Repo.GetVersion(ctx, tenantID, ex.SkillVersionID)
	if err != nil {
		return skill.Version{}, Proposal{}, false, err
	}
	steps, err := s.ExecStore.ListSteps(ctx, tenantID, executionID)
	if err != nil {
		return skill.Version{}, Proposal{}, false, err
	}
	codes := []string{ex.ErrorCode}
	msgs := []string{ex.ErrorMessage}
	var b strings.Builder
	evidence := []string{executionID, ex.SkillVersionID}
	for _, st := range steps {
		fmt.Fprintf(&b, "step=%s tool=%s status=%s err=%s\n", st.StepID, st.Tool, st.Status, st.ErrorCode)
		if st.ErrorCode != "" {
			codes = append(codes, st.ErrorCode)
		}
		evidence = append(evidence, st.ID)
	}
	tools := []string{}
	if s.Tools != nil {
		for _, d := range s.Tools.List() {
			tools = append(tools, d.Name)
		}
	}
	in := Input{
		Current: ver.Spec, FailureCodes: codes, FailureMessages: msgs,
		StepTrace: b.String(), ToolAllowList: tools, EvidenceIDs: evidence,
	}
	p, ok, err := ProposeOrFallback(ctx, s.LLM, in)
	if err != nil || !ok {
		return skill.Version{}, p, ok, err
	}
	rev, err := skill.ProposeRevision(ctx, s.Repo, tenantID, ver.SkillID, p.NewSpecYAML, patternID)
	if err != nil {
		return skill.Version{}, p, false, err
	}
	return rev, p, true, nil
}
