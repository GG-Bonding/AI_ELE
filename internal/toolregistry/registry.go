// Package toolregistry is the V3 Tool Contract catalog used by Skill validation and runtime.
package toolregistry

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

// Risk classifies tool side-effect severity (aligned with skill.Risk string values).
type Risk string

const (
	RiskReadOnly Risk = "READ_ONLY"
	RiskLow      Risk = "LOW"
	RiskMedium   Risk = "MEDIUM"
	RiskHigh     Risk = "HIGH"
	RiskCritical Risk = "CRITICAL"
)

// Valid reports whether r is a known risk level.
func (r Risk) Valid() bool {
	switch r {
	case RiskReadOnly, RiskLow, RiskMedium, RiskHigh, RiskCritical:
		return true
	default:
		return false
	}
}

// Rank returns a comparable severity (higher = more dangerous).
func (r Risk) Rank() int {
	switch r {
	case RiskReadOnly:
		return 0
	case RiskLow:
		return 1
	case RiskMedium:
		return 2
	case RiskHigh:
		return 3
	case RiskCritical:
		return 4
	default:
		return -1
	}
}

// Max returns the higher of two risks.
func Max(a, b Risk) Risk {
	if a.Rank() >= b.Rank() {
		return a
	}
	return b
}

// ParamType is a simple tool parameter kind.
type ParamType string

const (
	ParamString  ParamType = "string"
	ParamNumber  ParamType = "number"
	ParamBoolean ParamType = "boolean"
	ParamObject  ParamType = "object"
	ParamArray   ParamType = "array"
	ParamAny     ParamType = "any"
)

// ParamSchema describes one tool input or output field.
type ParamSchema struct {
	Type        ParamType `json:"type"`
	Description string    `json:"description,omitempty"`
	Required    bool      `json:"required,omitempty"`
}

// Definition is the contract for one registered tool.
type Definition struct {
	Name string

	InputSchema  map[string]ParamSchema
	OutputSchema map[string]ParamSchema

	Risk Risk

	Idempotent bool
	SideEffect bool

	Timeout time.Duration

	// AllowedTenants restricts usage; empty means all tenants.
	AllowedTenants []string
}

// Registry is a process-local tool capability catalog.
type Registry struct {
	mu    sync.RWMutex
	tools map[string]Definition
}

// New constructs an empty registry.
func New() *Registry {
	return &Registry{tools: map[string]Definition{}}
}

// Register adds or replaces a tool definition.
func (r *Registry) Register(def Definition) error {
	name := strings.TrimSpace(def.Name)
	if name == "" {
		return fmt.Errorf("tool name is required")
	}
	if !def.Risk.Valid() {
		return fmt.Errorf("tool %q: invalid risk %q", name, def.Risk)
	}
	def.Name = name
	if def.InputSchema == nil {
		def.InputSchema = map[string]ParamSchema{}
	}
	if def.OutputSchema == nil {
		def.OutputSchema = map[string]ParamSchema{}
	}
	if def.Timeout <= 0 {
		def.Timeout = 15 * time.Second
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tools[name] = def
	return nil
}

// Get returns a tool by name.
func (r *Registry) Get(name string) (Definition, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	def, ok := r.tools[strings.TrimSpace(name)]
	return def, ok
}

// Has reports whether a tool is registered.
func (r *Registry) Has(name string) bool {
	_, ok := r.Get(name)
	return ok
}

// List returns all registered tools (unsorted).
func (r *Registry) List() []Definition {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Definition, 0, len(r.tools))
	for _, def := range r.tools {
		out = append(out, def)
	}
	return out
}

// AllowedForTenant reports whether the tool may be used by tenantID.
func (d Definition) AllowedForTenant(tenantID string) bool {
	if len(d.AllowedTenants) == 0 {
		return true
	}
	tenantID = strings.TrimSpace(tenantID)
	for _, t := range d.AllowedTenants {
		if t == tenantID {
			return true
		}
	}
	return false
}

// Default builds the built-in tool catalog used by eval / jira simulators.
func Default() *Registry {
	r := New()
	_ = r.Register(Definition{
		Name: "jira.search_projects",
		InputSchema: map[string]ParamSchema{
			"query": {Type: ParamString, Required: true},
		},
		OutputSchema: map[string]ParamSchema{
			"projects": {Type: ParamArray, Required: true},
			"key":      {Type: ParamString},
			"name":     {Type: ParamString},
		},
		Risk:       RiskReadOnly,
		Idempotent: true,
		SideEffect: false,
		Timeout:    10 * time.Second,
	})
	_ = r.Register(Definition{
		Name: "jira.create_issue",
		InputSchema: map[string]ParamSchema{
			"project": {Type: ParamString, Required: true},
			"summary": {Type: ParamString},
			"title":   {Type: ParamString},
		},
		OutputSchema: map[string]ParamSchema{
			"issue_key": {Type: ParamString, Required: true},
			"key":       {Type: ParamString},
		},
		Risk:       RiskLow,
		Idempotent: false,
		SideEffect: true,
		Timeout:    15 * time.Second,
	})
	_ = r.Register(Definition{
		Name: "jira.delete_issue",
		InputSchema: map[string]ParamSchema{
			"issue_key": {Type: ParamString, Required: true},
		},
		OutputSchema: map[string]ParamSchema{},
		Risk:         RiskHigh,
		Idempotent:   true,
		SideEffect:   true,
		Timeout:      15 * time.Second,
	})
	return r
}
