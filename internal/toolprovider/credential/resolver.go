package credential

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
)

// Principal identifies the actor requesting tool credentials / approvals (V3.3).
type Principal struct {
	ID          string
	TenantID    string
	Role        string
	Permissions []string
}

// MapResolver looks up credentials from an in-memory map keyed by tenant|tool or tenant|principal|tool.
type MapResolver struct {
	mu   sync.RWMutex
	data map[string]map[string]string
}

func NewMapResolver() *MapResolver {
	return &MapResolver{data: map[string]map[string]string{}}
}

func (r *MapResolver) Put(tenantID, principalID, toolName string, headers map[string]string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.data[key(tenantID, principalID, toolName)] = clone(headers)
	r.data[key(tenantID, "", toolName)] = clone(headers)
}

func (r *MapResolver) Resolve(_ context.Context, tenantID, principalID, toolName string) (map[string]string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if h, ok := r.data[key(tenantID, principalID, toolName)]; ok {
		return clone(h), nil
	}
	if h, ok := r.data[key(tenantID, "", toolName)]; ok {
		return clone(h), nil
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

func key(tenantID, principalID, toolName string) string {
	return tenantID + "|" + principalID + "|" + toolName
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
