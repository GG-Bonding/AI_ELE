package credential_test

import (
	"context"
	"testing"

	"github.com/agent-experience-engine/agent-experience-engine/internal/toolprovider/credential"
)

func TestMapResolverDoesNotCrossWriteTenant(t *testing.T) {
	t.Parallel()
	r := credential.NewMapResolver(credential.Options{AllowTenantFallback: false})
	r.PutPrincipalCredential("t", "a", "pay.refund", map[string]string{"Authorization": "A"})
	tenant, _ := r.Resolve(context.Background(), "t", "", "pay.refund")
	if len(tenant) != 0 {
		t.Fatalf("principal put must not create tenant default: %v", tenant)
	}
}
