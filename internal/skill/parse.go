package skill

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// DefaultMaxSteps is applied when Spec.MaxSteps is unset.
const DefaultMaxSteps = 10

// DefaultTimeoutMs is applied when Spec.TimeoutMs is unset (30s).
const DefaultTimeoutMs = 30_000

// ParseYAML decodes a declarative Skill YAML document into a Spec IR.
// Runtime must not interpret YAML directly — always go through ParseYAML → Normalize.
func ParseYAML(doc string) (Spec, error) {
	doc = strings.TrimSpace(doc)
	if doc == "" {
		return Spec{}, fmt.Errorf("%w: empty skill yaml", ErrInvalidSpec)
	}
	var raw Spec
	if err := yaml.Unmarshal([]byte(doc), &raw); err != nil {
		return Spec{}, fmt.Errorf("%w: parse yaml: %v", ErrInvalidSpec, err)
	}
	return Normalize(raw)
}

// Normalize fills defaults and trims fields; does not perform deep validation (V3-2).
func Normalize(spec Spec) (Spec, error) {
	spec.Name = strings.TrimSpace(spec.Name)
	spec.Description = strings.TrimSpace(spec.Description)
	if spec.Name == "" {
		return Spec{}, fmt.Errorf("%w: name is required", ErrInvalidSpec)
	}
	if len(spec.Steps) == 0 {
		return Spec{}, fmt.Errorf("%w: at least one step is required", ErrInvalidSpec)
	}
	if spec.MaxSteps <= 0 {
		spec.MaxSteps = DefaultMaxSteps
	}
	if spec.TimeoutMs <= 0 {
		spec.TimeoutMs = DefaultTimeoutMs
	}
	if spec.Risk.Level == "" {
		spec.Risk.Level = RiskLow
	}
	if !spec.Risk.Level.Valid() {
		return Spec{}, fmt.Errorf("%w: invalid risk level %q", ErrInvalidSpec, spec.Risk.Level)
	}
	if spec.Inputs == nil {
		spec.Inputs = map[string]FieldSchema{}
	}
	if spec.Outputs == nil {
		spec.Outputs = map[string]FieldSchema{}
	}
	for i := range spec.Steps {
		st := &spec.Steps[i]
		st.ID = strings.TrimSpace(st.ID)
		st.Tool = strings.TrimSpace(st.Tool)
		st.SaveAs = strings.TrimSpace(st.SaveAs)
		if st.ID == "" {
			return Spec{}, fmt.Errorf("%w: step[%d] id is required", ErrInvalidSpec, i)
		}
		if st.Tool == "" {
			return Spec{}, fmt.Errorf("%w: step %q tool is required", ErrInvalidSpec, st.ID)
		}
		if st.Args == nil {
			st.Args = map[string]any{}
		}
	}
	for name, field := range spec.Inputs {
		field.Type = FieldType(strings.TrimSpace(string(field.Type)))
		if field.Type == "" {
			field.Type = FieldString
		}
		if !field.Type.Valid() {
			return Spec{}, fmt.Errorf("%w: input %q has invalid type %q", ErrInvalidSpec, name, field.Type)
		}
		spec.Inputs[name] = field
	}
	for name, field := range spec.Outputs {
		field.Type = FieldType(strings.TrimSpace(string(field.Type)))
		if field.Type == "" {
			field.Type = FieldString
		}
		if !field.Type.Valid() {
			return Spec{}, fmt.Errorf("%w: output %q has invalid type %q", ErrInvalidSpec, name, field.Type)
		}
		spec.Outputs[name] = field
	}
	return spec, nil
}

// HashSpec returns a stable SHA-256 hex digest of canonical Spec JSON.
func HashSpec(spec Spec) (string, error) {
	b, err := json.Marshal(spec)
	if err != nil {
		return "", fmt.Errorf("marshal spec for hash: %w", err)
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}

// NewVersion builds an immutable Version from a Spec (CANDIDATE / PENDING).
func NewVersion(tenantID, skillID, patternID string, version int64, spec Spec, specYAML string, confidence, utility float64, now time.Time) (Version, error) {
	norm, err := Normalize(spec)
	if err != nil {
		return Version{}, err
	}
	hash, err := HashSpec(norm)
	if err != nil {
		return Version{}, err
	}
	if version <= 0 {
		version = 1
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	return Version{
		TenantID:         strings.TrimSpace(tenantID),
		SkillID:          strings.TrimSpace(skillID),
		Version:          version,
		PatternID:        strings.TrimSpace(patternID),
		Spec:             norm,
		SpecYAML:         specYAML,
		SpecHash:         hash,
		Confidence:       clamp01(confidence),
		Utility:          clamp01(utility),
		Status:           VersionCandidate,
		ValidationStatus: ValidationPending,
		CreatedAt:        now,
	}, nil
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}
