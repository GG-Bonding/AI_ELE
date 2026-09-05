package skillruntime

import (
	"context"
	"fmt"
	"strings"

	"github.com/agent-experience-engine/agent-experience-engine/internal/skill"
)

// Decision is the policy gate outcome for a Skill run.
type Decision string

const (
	DecisionAllow           Decision = "ALLOW"
	DecisionDeny            Decision = "DENY"
	DecisionRequireApproval Decision = "REQUIRE_APPROVAL"
	DecisionShadowOnly      Decision = "SHADOW_ONLY"
)

// ExecutionPolicy decides whether a Skill may run (and how).
type ExecutionPolicy interface {
	Decide(ctx context.Context, req PolicyRequest) (Decision, string, error)
}

// PolicyRequest is the input to ExecutionPolicy.Decide.
type PolicyRequest struct {
	Mode             skill.ExecutionMode // desired
	Risk             skill.Risk
	RequiresApproval bool
	RuntimeEnabled   bool
	AvailableTools   []string // agent tools
	SpecTools        []string
	// ApprovalApproved is true only after a persisted server-side approval (Resume).
	ApprovalApproved bool
}

// DefaultPolicy implements the V3 risk / tool gate.
// AllowMedium permits LIVE execution for MEDIUM risk skills.
type DefaultPolicy struct {
	AllowMedium bool
}

// Decide implements ExecutionPolicy.
func (p DefaultPolicy) Decide(ctx context.Context, req PolicyRequest) (Decision, string, error) {
	_ = ctx
	if !req.RuntimeEnabled {
		return DecisionDeny, "skill runtime disabled", nil
	}

	available := make(map[string]struct{}, len(req.AvailableTools))
	for _, t := range req.AvailableTools {
		t = strings.TrimSpace(t)
		if t == "" {
			continue
		}
		available[t] = struct{}{}
	}
	for _, tool := range req.SpecTools {
		tool = strings.TrimSpace(tool)
		if tool == "" {
			continue
		}
		if _, ok := available[tool]; !ok {
			return DecisionDeny, fmt.Sprintf("missing tool %q", tool), nil
		}
	}

	risk := req.Risk
	if risk == "" {
		risk = skill.RiskReadOnly
	}

	switch risk {
	case skill.RiskCritical:
		return DecisionDeny, "critical risk cannot auto-execute", nil
	case skill.RiskHigh:
		if req.Mode == skill.ModeLive {
			if req.ApprovalApproved {
				return DecisionAllow, "", nil
			}
			return DecisionRequireApproval, "high risk live requires approval", nil
		}
		return DecisionAllow, "", nil
	case skill.RiskMedium:
		if req.Mode == skill.ModeLive && !p.AllowMedium {
			return DecisionDeny, "medium risk live denied (AllowMedium=false)", nil
		}
		return DecisionAllow, "", nil
	case skill.RiskLow, skill.RiskReadOnly:
		// fall through to RequiresApproval check
	default:
		return DecisionDeny, fmt.Sprintf("unknown risk %q", risk), nil
	}

	if req.RequiresApproval && req.Mode == skill.ModeLive && !req.ApprovalApproved {
		return DecisionRequireApproval, "skill requires approval for live", nil
	}
	return DecisionAllow, "", nil
}
