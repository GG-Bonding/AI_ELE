package skillvalidator

import "github.com/agent-experience-engine/agent-experience-engine/internal/skill"

// Adapt exposes *Validator as skill.SpecValidator for RegistryService wiring.
func Adapt(v *Validator) skill.SpecValidator {
	if v == nil {
		v = New(nil, Options{})
	}
	return specAdapter{v: v}
}

type specAdapter struct {
	v *Validator
}

func (a specAdapter) Validate(spec skill.Spec) skill.ValidationReport {
	rep := a.v.Validate(spec)
	return skill.ValidationReport{
		OK:               rep.OK,
		Normalized:       rep.Normalized,
		ComputedRisk:     rep.ComputedRisk,
		RequiresApproval: rep.RequiresApproval,
	}
}
