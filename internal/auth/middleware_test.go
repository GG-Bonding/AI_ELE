package auth_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/agent-experience-engine/agent-experience-engine/internal/auth"
)

func TestHeaderMiddlewarePrincipal(t *testing.T) {
	t.Parallel()
	var got auth.Principal
	h := auth.HeaderMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, ok := auth.FromContext(r.Context())
		if !ok {
			t.Fatal("missing principal")
		}
		got = p
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodPost, "/x", nil)
	req.Header.Set("X-AEE-Tenant-ID", "t1")
	req.Header.Set("X-AEE-Actor-ID", "alice")
	req.Header.Set("X-AEE-Roles", "approver,eng")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if got.ActorID != "alice" || got.TenantID != "t1" || !got.CanApprove() {
		t.Fatalf("%#v", got)
	}
}
