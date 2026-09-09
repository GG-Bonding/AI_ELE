package auth

import (
	"context"
	"strings"
)

type ctxKey struct{}

// Principal is the authenticated actor (V3.4). Never trust client body for identity.
type Principal struct {
	TenantID    string
	ActorID     string
	Roles       []string
	Permissions []string
}

// WithPrincipal stores principal on context.
func WithPrincipal(ctx context.Context, p Principal) context.Context {
	return context.WithValue(ctx, ctxKey{}, p)
}

// FromContext returns principal if present.
func FromContext(ctx context.Context) (Principal, bool) {
	p, ok := ctx.Value(ctxKey{}).(Principal)
	return p, ok
}

// HasRole reports whether principal includes role.
func (p Principal) HasRole(role string) bool {
	role = strings.ToLower(strings.TrimSpace(role))
	for _, r := range p.Roles {
		if strings.ToLower(strings.TrimSpace(r)) == role {
			return true
		}
	}
	return false
}

// CanApprove reports whether principal may approve high-risk skills.
func (p Principal) CanApprove() bool {
	if p.HasRole("approver") || p.HasRole("admin") || p.HasRole("cto") {
		return true
	}
	for _, perm := range p.Permissions {
		if strings.EqualFold(strings.TrimSpace(perm), "skill:approve") {
			return true
		}
	}
	return false
}
