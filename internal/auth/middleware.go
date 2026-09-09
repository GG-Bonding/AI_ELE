package auth

import (
	"net/http"
	"strings"
)

// HeaderMiddleware extracts Principal from trusted headers (gateway / IdP injects these).
// Headers: X-AEE-Tenant-ID, X-AEE-Actor-ID, X-AEE-Roles (comma-separated), X-AEE-Permissions.
// Body tenant_id / requester_id / approved_by must not be treated as auth truth.
func HeaderMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tenant := strings.TrimSpace(r.Header.Get("X-AEE-Tenant-ID"))
		actor := strings.TrimSpace(r.Header.Get("X-AEE-Actor-ID"))
		if tenant != "" && actor != "" {
			p := Principal{
				TenantID:    tenant,
				ActorID:     actor,
				Roles:       splitCSV(r.Header.Get("X-AEE-Roles")),
				Permissions: splitCSV(r.Header.Get("X-AEE-Permissions")),
			}
			r = r.WithContext(WithPrincipal(r.Context(), p))
		}
		next.ServeHTTP(w, r)
	})
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
