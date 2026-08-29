package experience

// Type classifies what kind of reusable knowledge an experience represents.
type Type string

const (
	TypeEpisodic   Type = "EPISODIC"
	TypeSemantic   Type = "SEMANTIC"
	TypeProcedural Type = "PROCEDURAL"
	TypeFailure    Type = "FAILURE"
	TypeConstraint Type = "CONSTRAINT"
	TypePreference Type = "PREFERENCE"
)

// Valid reports whether t is a known experience type.
func (t Type) Valid() bool {
	switch t {
	case TypeEpisodic, TypeSemantic, TypeProcedural, TypeFailure, TypeConstraint, TypePreference:
		return true
	default:
		return false
	}
}

// Scope bounds where an experience applies.
type Scope string

const (
	ScopeGlobal   Scope = "GLOBAL"
	ScopeTenant   Scope = "TENANT"
	ScopeTeam     Scope = "TEAM"
	ScopeUser     Scope = "USER"
	ScopeAgent    Scope = "AGENT"
	ScopeTool     Scope = "TOOL"
	ScopeTaskType Scope = "TASK_TYPE"
)

// Valid reports whether s is a known experience scope.
func (s Scope) Valid() bool {
	switch s {
	case ScopeGlobal, ScopeTenant, ScopeTeam, ScopeUser, ScopeAgent, ScopeTool, ScopeTaskType:
		return true
	default:
		return false
	}
}

// Candidate is a structured experience proposal from the extractor.
// It is not yet a long-lived Experience (evaluator/store come in later phases).
type Candidate struct {
	Type       Type    `json:"type"`
	Trigger    string  `json:"trigger"`
	Content    string  `json:"content"`
	Confidence float64 `json:"confidence"`
	Scope      Scope   `json:"scope"`
	ScopeKey   string  `json:"scope_key,omitempty"`
}
