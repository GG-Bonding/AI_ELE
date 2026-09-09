package credential

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
)

// ErrNoCredential means no principal/tenant credential was found.
var ErrNoCredential = fmt.Errorf("credential: not found")

// Options controls credential resolution policy (V3.4).
type Options struct {
	// AllowTenantFallback permits falling back to tenant-default when principal miss.
	// Default false for side-effect / payment-class tools.
	AllowTenantFallback bool
}

// MapResolver isolates principal vs tenant credentials (V3.4).
type MapResolver struct {
	mu        sync.RWMutex
	principal map[string]map[string]string // tenant|principal|tool
	tenant    map[string]map[string]string // tenant|tool
	Opts      Options
}

func NewMapResolver(opts Options) *MapResolver {
	return &MapResolver{
		principal: map[string]map[string]string{},
		tenant:    map[string]map[string]string{},
		Opts:      opts,
	}
}

// PutPrincipalCredential stores credentials for one actor (never writes tenant default).
func (r *MapResolver) PutPrincipalCredential(tenantID, principalID, toolName string, headers map[string]string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.principal[pkey(tenantID, principalID, toolName)] = clone(headers)
}

// PutTenantCredential stores tenant-wide default credentials.
func (r *MapResolver) PutTenantCredential(tenantID, toolName string, headers map[string]string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tenant[tkey(tenantID, toolName)] = clone(headers)
}

// Put is deprecated alias for PutPrincipalCredential when principalID != ""; otherwise tenant.
// Does NOT cross-write principal → tenant (V3.4 isolation).
func (r *MapResolver) Put(tenantID, principalID, toolName string, headers map[string]string) {
	if strings.TrimSpace(principalID) != "" {
		r.PutPrincipalCredential(tenantID, principalID, toolName, headers)
		return
	}
	r.PutTenantCredential(tenantID, toolName, headers)
}

func (r *MapResolver) Resolve(_ context.Context, tenantID, principalID, toolName string) (map[string]string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if strings.TrimSpace(principalID) != "" {
		if h, ok := r.principal[pkey(tenantID, principalID, toolName)]; ok {
			return clone(h), nil
		}
	}
	if r.Opts.AllowTenantFallback || strings.TrimSpace(principalID) == "" {
		if h, ok := r.tenant[tkey(tenantID, toolName)]; ok {
			return clone(h), nil
		}
	}
	return nil, nil
}

// EnvResolver reads Authorization from env vars like AEE_TOOL_CRED_<TOOL>.
type EnvResolver struct {
	Prefix string // default AEE_TOOL_CRED_
}

func (r EnvResolver) Resolve(_ context.Context, _, _, toolName string) (map[string]string, error) {
	prefix := r.Prefix
	if prefix == "" {
		prefix = "AEE_TOOL_CRED_"
	}
	envKey := prefix + strings.ToUpper(strings.ReplaceAll(toolName, ".", "_"))
	if v := strings.TrimSpace(os.Getenv(envKey)); v != "" {
		return map[string]string{"Authorization": v}, nil
	}
	return nil, nil
}

// Chain tries resolvers in order; first non-empty headers wins.
type Chain []interface {
	Resolve(ctx context.Context, tenantID, principalID, toolName string) (map[string]string, error)
}

func (c Chain) Resolve(ctx context.Context, tenantID, principalID, toolName string) (map[string]string, error) {
	for _, r := range c {
		h, err := r.Resolve(ctx, tenantID, principalID, toolName)
		if err != nil {
			return nil, err
		}
		if len(h) > 0 {
			return h, nil
		}
	}
	return nil, nil
}

// RequireApproverSeparation fails when approver == requester (configurable).
func RequireApproverSeparation(requesterID, approverID string, enforce bool) error {
	if !enforce {
		return nil
	}
	requesterID = strings.TrimSpace(requesterID)
	approverID = strings.TrimSpace(approverID)
	if requesterID == "" || approverID == "" {
		return fmt.Errorf("credential: requester and approver identities are required")
	}
	if requesterID == approverID {
		return fmt.Errorf("credential: approver must differ from requester")
	}
	return nil
}

func pkey(tenantID, principalID, toolName string) string {
	return tenantID + "|" + principalID + "|" + toolName
}

func tkey(tenantID, toolName string) string {
	return tenantID + "|" + toolName
}

func clone(in map[string]string) map[string]string {
	if in == nil {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
