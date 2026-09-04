package evolutionjob

import (
	"context"
	"errors"
	"time"

	"github.com/agent-experience-engine/agent-experience-engine/internal/experience"
)

// Status tracks evolution job progress (mirrors episode learning jobs).
type Status string

const (
	StatusPending    Status = "PENDING"
	StatusProcessing Status = "PROCESSING"
	StatusApplied    Status = "APPLIED"
	StatusFailed     Status = "FAILED"
)

// ErrNotFound is returned when an evolution job is missing.
var ErrNotFound = errors.New("evolution job not found")

// DirtyGroup is a type/scope/scope_key family that may need Pattern generalization.
type DirtyGroup struct {
	TenantID  string          `json:"tenant_id"`
	Type      experience.Type `json:"type"`
	Scope     experience.Scope `json:"scope"`
	ScopeKey  string          `json:"scope_key"`
	UpdatedAt time.Time       `json:"updated_at"`
}

// Job is the durable process state for one dirty family.
type Job struct {
	ID           string           `json:"id"`
	TenantID     string           `json:"tenant_id"`
	Type         experience.Type  `json:"type"`
	Scope        experience.Scope `json:"scope"`
	ScopeKey     string           `json:"scope_key"`
	Status       Status           `json:"status"`
	LastError    string           `json:"last_error,omitempty"`
	CreatedCount int              `json:"created_count"`
	CreatedAt    time.Time        `json:"created_at"`
	UpdatedAt    time.Time        `json:"updated_at"`
}

// DirtyMarker records that a family should be scanned for Pattern evolution (V2.2-3).
type DirtyMarker interface {
	MarkDirty(ctx context.Context, tenantID string, typ experience.Type, scope experience.Scope, scopeKey string) error
}

// Repository persists dirty groups and evolution jobs.
type Repository interface {
	DirtyMarker

	ListDirty(ctx context.Context, limit int) ([]DirtyGroup, error)
	ClearDirty(ctx context.Context, g DirtyGroup) error

	UpsertPending(ctx context.Context, g DirtyGroup) (Job, error)
	GetByFamily(ctx context.Context, tenantID string, typ experience.Type, scope experience.Scope, scopeKey string) (Job, error)
	MarkProcessing(ctx context.Context, tenantID string, typ experience.Type, scope experience.Scope, scopeKey string) error
	MarkApplied(ctx context.Context, tenantID string, typ experience.Type, scope experience.Scope, scopeKey string, createdCount int) error
	MarkFailed(ctx context.Context, tenantID string, typ experience.Type, scope experience.Scope, scopeKey, lastError string) error
	ListStaleProcessing(ctx context.Context, cutoff time.Time) ([]Job, error)
}
