package auth_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/agent-experience-engine/agent-experience-engine/internal/auth"
)

func TestJWTProviderAcceptsValidToken(t *testing.T) {
	t.Parallel()
	secret := []byte("test-secret")
	tok, err := auth.IssueHS256JWT(secret, map[string]any{
		"tenant_id": "t1", "sub": "actor1", "roles": []string{"admin"},
		"exp": float64(time.Now().Add(time.Hour).Unix()),
	})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	p, err := (auth.JWTProvider{HMACSecret: secret}).Authenticate(req)
	if err != nil {
		t.Fatal(err)
	}
	if p.TenantID != "t1" || p.ActorID != "actor1" || !p.CanApprove() {
		t.Fatalf("%+v", p)
	}
}

func TestNoneProviderIgnoresDevHeaders(t *testing.T) {
	t.Parallel()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-AEE-Tenant-ID", "spoof")
	req.Header.Set("X-AEE-Actor-ID", "boss")
	_, err := (auth.NoneProvider{}).Authenticate(req)
	if err == nil {
		t.Fatal("expected unauthenticated")
	}
}

func TestMiddlewareJWTInjectsPrincipal(t *testing.T) {
	t.Parallel()
	secret := []byte("s")
	tok, _ := auth.IssueHS256JWT(secret, map[string]any{
		"tid": "ten", "actor_id": "act", "exp": float64(time.Now().Add(time.Hour).Unix()),
	})
	var got auth.Principal
	var ok bool
	h := auth.Middleware(auth.JWTProvider{HMACSecret: secret})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got, ok = auth.FromContext(r.Context())
	}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	h.ServeHTTP(httptest.NewRecorder(), req)
	if !ok || got.TenantID != "ten" || got.ActorID != "act" {
		t.Fatalf("ok=%v got=%+v", ok, got)
	}
}
