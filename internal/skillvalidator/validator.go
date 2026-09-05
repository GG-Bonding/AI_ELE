// Package skillvalidator performs static checks on Skill Spec IR (V3-2).
package skillvalidator

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/agent-experience-engine/agent-experience-engine/internal/skill"
	"github.com/agent-experience-engine/agent-experience-engine/internal/toolregistry"
)

// HardMaxSteps is the absolute ceiling regardless of Spec.MaxSteps.
const HardMaxSteps = 20

var (
	templateRef = regexp.MustCompile(`\{\{\s*([^}]+?)\s*\}\}`)
	// Forbidden tool name fragments — never executable even if registered later.
	forbiddenToolSubstrings = []string{
		"shell", "bash", "python", "eval", "exec", "sql", "http.request", "fetch_url", "download",
	}
)

// Severity classifies a validation issue.
type Severity string

const (
	SeverityError   Severity = "error"
	SeverityWarning Severity = "warning"
)

// Issue is one static-check finding.
type Issue struct {
	Code     string   `json:"code"`
	Path     string   `json:"path,omitempty"`
	Message  string   `json:"message"`
	Severity Severity `json:"severity"`
}

// Report is the full validation outcome.
type Report struct {
	OK               bool              `json:"ok"`
	Issues           []Issue           `json:"issues,omitempty"`
	ComputedRisk     skill.Risk        `json:"computed_risk"`
	RequiresApproval bool              `json:"requires_approval"`
	Tools            []string          `json:"tools,omitempty"`
	Normalized       skill.Spec        `json:"normalized"`
}

// Options configures validation policy.
type Options struct {
	TenantID string
	// MaxStepsHardCap overrides HardMaxSteps when > 0.
	MaxStepsHardCap int
	// AllowCritical permits CRITICAL risk tools/skills to pass (still sets RequiresApproval).
	AllowCritical bool
}

// Validator checks Spec against a Tool Registry.
type Validator struct {
	Tools *toolregistry.Registry
	Opts  Options
}

// New constructs a validator. tools defaults to toolregistry.Default().
func New(tools *toolregistry.Registry, opts Options) *Validator {
	if tools == nil {
		tools = toolregistry.Default()
	}
	return &Validator{Tools: tools, Opts: opts}
}

// Validate runs schema / tool / control-flow / security checks.
func (v *Validator) Validate(spec skill.Spec) Report {
	norm, err := skill.Normalize(spec)
	rep := Report{Normalized: norm, ComputedRisk: skill.RiskReadOnly}
	if err != nil {
		rep.Issues = append(rep.Issues, Issue{
			Code: "SPEC_NORMALIZE", Message: err.Error(), Severity: SeverityError,
		})
		return finalize(rep)
	}

	hardCap := HardMaxSteps
	if v.Opts.MaxStepsHardCap > 0 {
		hardCap = v.Opts.MaxStepsHardCap
	}

	v.checkSchema(&rep, norm, hardCap)
	v.checkToolsAndSecurity(&rep, norm)
	v.checkControlFlowAndRefs(&rep, norm)

	return finalize(rep)
}

func finalize(rep Report) Report {
	rep.OK = true
	for _, iss := range rep.Issues {
		if iss.Severity == SeverityError {
			rep.OK = false
			break
		}
	}
	return rep
}

func (v *Validator) checkSchema(rep *Report, spec skill.Spec, hardCap int) {
	if len(spec.Steps) > hardCap {
		rep.Issues = append(rep.Issues, Issue{
			Code: "MAX_STEPS_HARD", Path: "steps",
			Message:  fmt.Sprintf("step count %d exceeds hard cap %d", len(spec.Steps), hardCap),
			Severity: SeverityError,
		})
	}
	if len(spec.Steps) > spec.MaxSteps {
		rep.Issues = append(rep.Issues, Issue{
			Code: "MAX_STEPS", Path: "steps",
			Message:  fmt.Sprintf("step count %d exceeds max_steps %d", len(spec.Steps), spec.MaxSteps),
			Severity: SeverityError,
		})
	}
	if spec.TimeoutMs <= 0 {
		rep.Issues = append(rep.Issues, Issue{
			Code: "TIMEOUT", Path: "timeout_ms", Message: "timeout_ms must be > 0", Severity: SeverityError,
		})
	}

	seenID := map[string]int{}
	seenSave := map[string]string{}
	for i, st := range spec.Steps {
		path := fmt.Sprintf("steps[%d]", i)
		if prev, ok := seenID[st.ID]; ok {
			rep.Issues = append(rep.Issues, Issue{
				Code: "DUP_STEP_ID", Path: path + ".id",
				Message:  fmt.Sprintf("duplicate step id %q (also steps[%d])", st.ID, prev),
				Severity: SeverityError,
			})
		} else {
			seenID[st.ID] = i
		}
		if st.SaveAs != "" {
			if prev, ok := seenSave[st.SaveAs]; ok {
				rep.Issues = append(rep.Issues, Issue{
					Code: "DUP_SAVE_AS", Path: path + ".save_as",
					Message:  fmt.Sprintf("save_as %q conflicts with step %q", st.SaveAs, prev),
					Severity: SeverityError,
				})
			} else {
				seenSave[st.SaveAs] = st.ID
			}
			if _, isInput := spec.Inputs[st.SaveAs]; isInput {
				rep.Issues = append(rep.Issues, Issue{
					Code: "SAVE_AS_SHADOWS_INPUT", Path: path + ".save_as",
					Message:  fmt.Sprintf("save_as %q shadows an input name", st.SaveAs),
					Severity: SeverityError,
				})
			}
		}
	}
}

func (v *Validator) checkToolsAndSecurity(rep *Report, spec skill.Spec) {
	toolRisk := toolregistry.RiskReadOnly
	seenTools := map[string]struct{}{}
	var tools []string

	for i, st := range spec.Steps {
		path := fmt.Sprintf("steps[%d].tool", i)
		lower := strings.ToLower(st.Tool)
		for _, bad := range forbiddenToolSubstrings {
			if strings.Contains(lower, bad) {
				rep.Issues = append(rep.Issues, Issue{
					Code: "FORBIDDEN_TOOL", Path: path,
					Message:  fmt.Sprintf("tool %q is forbidden in V3 skill DSL", st.Tool),
					Severity: SeverityError,
				})
				break
			}
		}

		def, ok := v.Tools.Get(st.Tool)
		if !ok {
			rep.Issues = append(rep.Issues, Issue{
				Code: "TOOL_NOT_REGISTERED", Path: path,
				Message:  fmt.Sprintf("tool %q is not registered", st.Tool),
				Severity: SeverityError,
			})
			continue
		}
		if !def.AllowedForTenant(v.Opts.TenantID) {
			rep.Issues = append(rep.Issues, Issue{
				Code: "TOOL_TENANT_DENIED", Path: path,
				Message:  fmt.Sprintf("tool %q is not allowed for tenant %q", st.Tool, v.Opts.TenantID),
				Severity: SeverityError,
			})
		}
		toolRisk = toolregistry.Max(toolRisk, def.Risk)

		if _, dup := seenTools[st.Tool]; !dup {
			seenTools[st.Tool] = struct{}{}
			tools = append(tools, st.Tool)
		}

		for name, param := range def.InputSchema {
			if !param.Required {
				continue
			}
			if _, has := st.Args[name]; !has {
				rep.Issues = append(rep.Issues, Issue{
					Code: "TOOL_ARG_REQUIRED", Path: fmt.Sprintf("steps[%d].args.%s", i, name),
					Message:  fmt.Sprintf("tool %q requires arg %q", st.Tool, name),
					Severity: SeverityError,
				})
			}
		}
		for name := range st.Args {
			if _, known := def.InputSchema[name]; !known && len(def.InputSchema) > 0 {
				rep.Issues = append(rep.Issues, Issue{
					Code: "TOOL_ARG_UNKNOWN", Path: fmt.Sprintf("steps[%d].args.%s", i, name),
					Message:  fmt.Sprintf("tool %q has no input %q", st.Tool, name),
					Severity: SeverityWarning,
				})
			}
		}
	}
	rep.Tools = tools

	computed := skill.Risk(toolRisk)
	if spec.Risk.Level.Valid() && spec.Risk.Level != "" {
		if skillRiskRank(spec.Risk.Level) < skillRiskRank(computed) {
			rep.Issues = append(rep.Issues, Issue{
				Code: "RISK_UNDERSTATED", Path: "risk.level",
				Message: fmt.Sprintf("declared risk %s is below tool-derived risk %s", spec.Risk.Level, computed),
				Severity: SeverityError,
			})
		}
		if skillRiskRank(spec.Risk.Level) > skillRiskRank(computed) {
			computed = spec.Risk.Level
		}
	}
	rep.ComputedRisk = computed

	if computed == skill.RiskHigh || computed == skill.RiskCritical || spec.Risk.RequiresApproval {
		rep.RequiresApproval = true
	}
	if computed == skill.RiskCritical && !v.Opts.AllowCritical {
		rep.Issues = append(rep.Issues, Issue{
			Code: "CRITICAL_DENIED", Path: "risk.level",
			Message:  "CRITICAL risk skills cannot pass validation in V3 without AllowCritical",
			Severity: SeverityError,
		})
	}
}

func (v *Validator) checkControlFlowAndRefs(rep *Report, spec skill.Spec) {
	available := map[string]struct{}{}
	for name := range spec.Inputs {
		available["inputs."+name] = struct{}{}
		available[name] = struct{}{} // allow bare input name in simple templates
	}

	for i, st := range spec.Steps {
		path := fmt.Sprintf("steps[%d]", i)
		refs := collectRefs(st.Args)
		if st.When != nil && st.When.Expr != "" {
			refs = append(refs, st.When.Expr)
		}
		for _, ref := range refs {
			if !refAvailable(ref, available, st.SaveAs) {
				rep.Issues = append(rep.Issues, Issue{
					Code: "UNKNOWN_REF", Path: path,
					Message:  fmt.Sprintf("unknown variable reference %q", ref),
					Severity: SeverityError,
				})
			}
		}
		if st.SaveAs != "" {
			available[st.SaveAs] = struct{}{}
			available[st.SaveAs+".*"] = struct{}{}
			// Allow field access: save_as.key etc.
			if def, ok := v.Tools.Get(st.Tool); ok {
				for outName := range def.OutputSchema {
					available[st.SaveAs+"."+outName] = struct{}{}
				}
			}
		}
	}

	for name, out := range spec.Outputs {
		_ = out
		// Output expressions are not yet a first-class Spec field map of templates;
		// reserved for future. Ensure output names don't collide with step ids.
		for _, st := range spec.Steps {
			if st.ID == name {
				rep.Issues = append(rep.Issues, Issue{
					Code: "OUTPUT_STEP_COLLISION", Path: "outputs." + name,
					Message:  fmt.Sprintf("output %q collides with step id", name),
					Severity: SeverityWarning,
				})
			}
		}
	}
}

func collectRefs(args map[string]any) []string {
	var out []string
	var walk func(v any)
	walk = func(v any) {
		switch t := v.(type) {
		case string:
			for _, m := range templateRef.FindAllStringSubmatch(t, -1) {
				out = append(out, strings.TrimSpace(m[1]))
			}
		case map[string]any:
			for _, child := range t {
				walk(child)
			}
		case []any:
			for _, child := range t {
				walk(child)
			}
		}
	}
	walk(args)
	return out
}

func refAvailable(ref string, available map[string]struct{}, currentSaveAs string) bool {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return false
	}
	// Disallow self-reference to current save_as before it exists.
	if currentSaveAs != "" && (ref == currentSaveAs || strings.HasPrefix(ref, currentSaveAs+".")) {
		return false
	}
	if _, ok := available[ref]; ok {
		return true
	}
	// Prefix match for save_as.* wildcard entries.
	if i := strings.IndexByte(ref, '.'); i > 0 {
		root := ref[:i]
		if _, ok := available[root]; ok {
			return true
		}
		if _, ok := available[root+".*"]; ok {
			return true
		}
	}
	if strings.HasPrefix(ref, "inputs.") {
		return false // already checked exact; missing input
	}
	return false
}

func skillRiskRank(r skill.Risk) int {
	return toolregistry.Risk(r).Rank()
}

// ApplyToVersion sets ValidationStatus / VersionStatus from a Report (immutable copy).
func ApplyToVersion(ver skill.Version, rep Report) skill.Version {
	out := ver
	out.Spec = rep.Normalized
	if hash, err := skill.HashSpec(rep.Normalized); err == nil {
		out.SpecHash = hash
	}
	if rep.OK {
		out.ValidationStatus = skill.ValidationPassed
		out.Status = skill.VersionValidated
		out.Spec.Risk.Level = rep.ComputedRisk
		out.Spec.Risk.RequiresApproval = rep.RequiresApproval
	} else {
		out.ValidationStatus = skill.ValidationFailed
		// Keep CANDIDATE (or prior) — do not promote.
		if out.Status == "" {
			out.Status = skill.VersionCandidate
		}
	}
	return out
}
