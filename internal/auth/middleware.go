package auth

import (
	"errors"
	"net/http"
	"strings"
)

// ErrUnauthenticated means the request has no valid principal.
var ErrUnauthenticated = errors.New("auth: unauthenticated")

// PrincipalProvider authenticates an HTTP request into a Principal (V3.4.1).
// Raw X-AEE-* headers are not authentication unless an explicit DevHeader provider is configured.
type PrincipalProvider interface {
	Authenticate(r *http.Request) (Principal, error)
}

// Middleware injects Principal from provider when authentication succeeds.
// Missing/invalid credentials leave context without Principal (handlers may 401).
func Middleware(provider PrincipalProvider) func(http.Handler) http.Handler {
	if provider == nil {
		provider = NoneProvider{}
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			p, err := provider.Authenticate(r)
			if err == nil && strings.TrimSpace(p.TenantID) != "" && strings.TrimSpace(p.ActorID) != "" {
				r = r.WithContext(WithPrincipal(r.Context(), p))
			}
			next.ServeHTTP(w, r)
		})
	}
}

// NoneProvider never authenticates (production-safe default when auth.mode=none).
type NoneProvider struct{}

func (NoneProvider) Authenticate(*http.Request) (Principal, error) {
	return Principal{}, ErrUnauthenticated
}

// DevHeaderProvider trusts X-AEE-* headers. Only for local/dev behind a trusted network.
// Production must use JWTProvider (or a gateway that strips client headers first).
type DevHeaderProvider struct{}

func (DevHeaderProvider) Authenticate(r *http.Request) (Principal, error) {
	tenant := strings.TrimSpace(r.Header.Get("X-AEE-Tenant-ID"))
	actor := strings.TrimSpace(r.Header.Get("X-AEE-Actor-ID"))
	if tenant == "" || actor == "" {
		return Principal{}, ErrUnauthenticated
	}
	return Principal{
		TenantID:    tenant,
		ActorID:     actor,
		Roles:       splitCSV(r.Header.Get("X-AEE-Roles")),
		Permissions: splitCSV(r.Header.Get("X-AEE-Permissions")),
	}, nil
}

// HeaderMiddleware is the legacy alias for DevHeaderProvider middleware.
// Prefer Middleware(DevHeaderProvider{}) or JWT in new wiring.
func HeaderMiddleware(next http.Handler) http.Handler {
	return Middleware(DevHeaderProvider{})(next)
}

func splitCSV(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
