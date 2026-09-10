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
	codes, msgs, evidence, stepTrace := s.collectVersionEvidence(ctx, tenantID, versionID)
	tools, schemaNotes := s.toolAllowList()
	in := Input{
		Current:         ver.Spec,
		FailureCodes:    codes,
		FailureMessages: msgs,
		StepTrace:       stepTrace,
		ToolAllowList:   tools,
		EvidenceIDs:     evidence,
		ExperienceText:  schemaNotes,
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

func (s *Service) collectVersionEvidence(ctx context.Context, tenantID, versionID string) (codes, msgs, evidence []string, stepTrace string) {
	if s.ExecStore == nil {
		return nil, nil, nil, ""
	}
	failed, err := s.ExecStore.ListFailedByVersion(ctx, tenantID, versionID, 5)
	if err != nil || len(failed) == 0 {
		return nil, nil, nil, ""
	}
	var b strings.Builder
	for _, ex := range failed {
		evidence = append(evidence, ex.ID)
		if ex.ErrorCode != "" {
			codes = append(codes, ex.ErrorCode)
		}
		if ex.ErrorMessage != "" {
			msgs = append(msgs, ex.ErrorMessage)
		}
		fmt.Fprintf(&b, "execution=%s status=%s code=%s msg=%s\n", ex.ID, ex.Status, ex.ErrorCode, ex.ErrorMessage)
		steps, listErr := s.ExecStore.ListSteps(ctx, tenantID, ex.ID)
		if listErr != nil {
			continue
		}
		for _, st := range steps {
			fmt.Fprintf(&b, "  step=%s tool=%s status=%s err=%s attempt=%d op=%s\n",
				st.StepID, st.Tool, st.Status, st.ErrorCode, st.Attempt, st.OperationKey)
			if st.ErrorCode != "" {
				codes = append(codes, st.ErrorCode)
			}
			evidence = append(evidence, st.ID)
		}
	}
	return codes, msgs, evidence, b.String()
}

func (s *Service) toolAllowList() (tools []string, schemaNotes string) {
	if s.Tools == nil {
		return nil, ""
	}
	var b strings.Builder
	for _, d := range s.Tools.List() {
		tools = append(tools, d.Name)
		if len(d.InputSchema) == 0 {
			continue
		}
		fmt.Fprintf(&b, "tool %s inputs:", d.Name)
		for name, p := range d.InputSchema {
			req := ""
			if p.Required {
				req = "*"
			}
			fmt.Fprintf(&b, " %s%s(%s)", name, req, p.Type)
		}
		b.WriteByte('\n')
	}
	return tools, b.String()
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
		fmt.Fprintf(&b, "step=%s tool=%s status=%s err=%s attempt=%d op=%s\n",
			st.StepID, st.Tool, st.Status, st.ErrorCode, st.Attempt, st.OperationKey)
		if st.ErrorCode != "" {
			codes = append(codes, st.ErrorCode)
		}
		evidence = append(evidence, st.ID)
	}
	tools, schemaNotes := s.toolAllowList()
	in := Input{
		Current: ver.Spec, FailureCodes: codes, FailureMessages: msgs,
		StepTrace: b.String(), ToolAllowList: tools, EvidenceIDs: evidence,
		ExperienceText: schemaNotes,
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
