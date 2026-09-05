// Package skill defines executable Skill Spec IR and versioned Skill assets (V3-1).
// Runtime execution is gated by config skill_runtime.enabled; this package is data + parse only.
package skill

import (
	"context"
	"encoding/json"
	"time"
)

// Status is the lifecycle of a logical Skill (aggregate over versions).
type Status string

const (
	StatusCandidate  Status = "CANDIDATE"
	StatusValidated  Status = "VALIDATED"
	StatusShadow     Status = "SHADOW"
	StatusActive     Status = "ACTIVE"
	StatusSuspended  Status = "SUSPENDED"
	StatusDeprecated Status = "DEPRECATED"
	StatusArchived   Status = "ARCHIVED"
)

// Valid reports whether s is a known Skill status.
func (s Status) Valid() bool {
	switch s {
	case StatusCandidate, StatusValidated, StatusShadow, StatusActive,
		StatusSuspended, StatusDeprecated, StatusArchived:
		return true
	default:
		return false
	}
}

// VersionStatus is the lifecycle of an immutable SkillVersion.
type VersionStatus string

const (
	VersionCandidate  VersionStatus = "CANDIDATE"
	VersionValidated  VersionStatus = "VALIDATED"
	VersionShadow     VersionStatus = "SHADOW"
	VersionActive     VersionStatus = "ACTIVE"
	VersionSuspended  VersionStatus = "SUSPENDED"
	VersionDeprecated VersionStatus = "DEPRECATED"
	VersionArchived   VersionStatus = "ARCHIVED"
)

// Valid reports whether s is a known SkillVersion status.
func (s VersionStatus) Valid() bool {
	switch s {
	case VersionCandidate, VersionValidated, VersionShadow, VersionActive,
		VersionSuspended, VersionDeprecated, VersionArchived:
		return true
	default:
		return false
	}
}

// ValidationStatus is the static-check outcome for a version (V3-2 fills PASSED/FAILED).
type ValidationStatus string

const (
	ValidationPending ValidationStatus = "PENDING"
	ValidationPassed  ValidationStatus = "PASSED"
	ValidationFailed  ValidationStatus = "FAILED"
)

// Valid reports whether v is a known validation status.
func (v ValidationStatus) Valid() bool {
	switch v {
	case ValidationPending, ValidationPassed, ValidationFailed:
		return true
	default:
		return false
	}
}

// Risk classifies side-effect severity for tools and skills.
type Risk string

const (
	RiskReadOnly  Risk = "READ_ONLY"
	RiskLow       Risk = "LOW"
	RiskMedium    Risk = "MEDIUM"
	RiskHigh      Risk = "HIGH"
	RiskCritical  Risk = "CRITICAL"
)

// Valid reports whether r is a known risk level.
func (r Risk) Valid() bool {
	switch r {
	case RiskReadOnly, RiskLow, RiskMedium, RiskHigh, RiskCritical, "":
		return true
	default:
		return false
	}
}

// FieldType is a Skill input/output field kind.
type FieldType string

const (
	FieldString  FieldType = "string"
	FieldNumber  FieldType = "number"
	FieldBoolean FieldType = "boolean"
	FieldObject  FieldType = "object"
	FieldArray   FieldType = "array"
)

// Valid reports whether t is a known field type.
func (t FieldType) Valid() bool {
	switch t {
	case FieldString, FieldNumber, FieldBoolean, FieldObject, FieldArray:
		return true
	default:
		return false
	}
}

// FieldSchema describes one input or output field.
type FieldSchema struct {
	Type        FieldType `json:"type" yaml:"type"`
	Description string    `json:"description,omitempty" yaml:"description,omitempty"`
	Required    bool      `json:"required,omitempty" yaml:"required,omitempty"`
	Default     any       `json:"default,omitempty" yaml:"default,omitempty"`
}

// Condition is a placeholder for preconditions / when-clauses (evaluator lands in V3-2+).
type Condition struct {
	Expr string `json:"expr,omitempty" yaml:"expr,omitempty"`
}

// RetryPolicy controls per-step retries (runtime honors later).
type RetryPolicy struct {
	MaxAttempts int           `json:"max_attempts,omitempty" yaml:"max_attempts,omitempty"`
	BackoffMs   int           `json:"backoff_ms,omitempty" yaml:"backoff_ms,omitempty"`
}

// ErrorPolicy controls failure handling for a step.
type ErrorPolicy struct {
	// Action: fail | continue | retry (default fail).
	Action string `json:"action,omitempty" yaml:"action,omitempty"`
}

// SkillRisk summarizes aggregate risk for a skill.
type SkillRisk struct {
	Level       Risk   `json:"level" yaml:"level"`
	RequiresApproval bool `json:"requires_approval,omitempty" yaml:"requires_approval,omitempty"`
	Notes       string `json:"notes,omitempty" yaml:"notes,omitempty"`
}

// SkillStep is one tool invocation in a declarative workflow.
type SkillStep struct {
	ID      string         `json:"id" yaml:"id"`
	Tool    string         `json:"tool" yaml:"tool"`
	Args    map[string]any `json:"args,omitempty" yaml:"args,omitempty"`
	SaveAs  string         `json:"save_as,omitempty" yaml:"save_as,omitempty"`
	When    *Condition     `json:"when,omitempty" yaml:"when,omitempty"`
	Retry   *RetryPolicy   `json:"retry,omitempty" yaml:"retry,omitempty"`
	OnError *ErrorPolicy   `json:"on_error,omitempty" yaml:"on_error,omitempty"`
}

// Spec is the executable Skill contract (internal IR — not YAML).
// YAML is only a serialization format; Runtime must consume Spec / NormalizedIR.
type Spec struct {
	Name        string                 `json:"name" yaml:"name"`
	Description string                 `json:"description,omitempty" yaml:"description,omitempty"`
	Version     int64                  `json:"version,omitempty" yaml:"version,omitempty"`

	Inputs  map[string]FieldSchema `json:"inputs,omitempty" yaml:"inputs,omitempty"`
	Outputs map[string]FieldSchema `json:"outputs,omitempty" yaml:"outputs,omitempty"`

	Preconditions []Condition `json:"preconditions,omitempty" yaml:"preconditions,omitempty"`
	Steps         []SkillStep `json:"steps" yaml:"steps"`

	Risk SkillRisk `json:"risk,omitempty" yaml:"risk,omitempty"`

	MaxSteps   int   `json:"max_steps,omitempty" yaml:"max_steps,omitempty"`
	TimeoutMs  int64 `json:"timeout_ms,omitempty" yaml:"timeout_ms,omitempty"`
	Idempotent bool  `json:"idempotent,omitempty" yaml:"idempotent,omitempty"`
}

// Skill is the logical Skill entity (stable id across immutable versions).
type Skill struct {
	ID          string  `json:"id"`
	TenantID    string  `json:"tenant_id"`
	Name        string  `json:"name"`
	Description string  `json:"description"`
	Status      Status  `json:"status"`
	ActiveVersionID *string `json:"active_version_id,omitempty"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Version is an immutable compiled SkillSpec revision.
type Version struct {
	ID      string `json:"id"`
	SkillID string `json:"skill_id"`
	TenantID string `json:"tenant_id"`
	Version int64  `json:"version"`

	PatternID string `json:"pattern_id,omitempty"`

	// Spec is the normalized executable IR.
	Spec Spec `json:"spec"`
	// SpecYAML is the source document when compiled from YAML.
	SpecYAML string `json:"spec_yaml,omitempty"`
	// SpecHash is a stable hash of the normalized Spec JSON (content-addressed).
	SpecHash string `json:"spec_hash"`

	Confidence float64 `json:"confidence"`
	Utility    float64 `json:"utility"`

	Status           VersionStatus    `json:"status"`
	ValidationStatus ValidationStatus `json:"validation_status"`

	CreatedAt time.Time `json:"created_at"`
}

// SpecJSON returns canonical JSON for Spec (used for hashing / persistence).
func (v Version) SpecJSON() (json.RawMessage, error) {
	return json.Marshal(v.Spec)
}

// Repository persists Skills and immutable Versions.
type Repository interface {
	CreateSkill(ctx context.Context, sk Skill) (Skill, error)
	GetSkill(ctx context.Context, tenantID, id string) (Skill, error)
	GetSkillByName(ctx context.Context, tenantID, name string) (Skill, error)
	UpdateSkill(ctx context.Context, sk Skill) (Skill, error)

	CreateVersion(ctx context.Context, ver Version) (Version, error)
	GetVersion(ctx context.Context, tenantID, id string) (Version, error)
	ListVersions(ctx context.Context, tenantID, skillID string) ([]Version, error)
	GetVersionByNumber(ctx context.Context, tenantID, skillID string, version int64) (Version, error)
}
