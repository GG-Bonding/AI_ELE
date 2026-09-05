package toolregistry_test

import (
	"testing"

	"github.com/agent-experience-engine/agent-experience-engine/internal/toolregistry"
)

func TestDefaultRegistryJiraTools(t *testing.T) {
	t.Parallel()
	reg := toolregistry.Default()
	for _, name := range []string{"jira.search_projects", "jira.create_issue", "jira.delete_issue"} {
		if !reg.Has(name) {
			t.Fatalf("missing tool %s", name)
		}
	}
	search, _ := reg.Get("jira.search_projects")
	if search.Risk != toolregistry.RiskReadOnly || search.SideEffect {
		t.Fatalf("%#v", search)
	}
	create, _ := reg.Get("jira.create_issue")
	if create.Risk != toolregistry.RiskLow || !create.SideEffect {
		t.Fatalf("%#v", create)
	}
	del, _ := reg.Get("jira.delete_issue")
	if del.Risk != toolregistry.RiskHigh {
		t.Fatalf("%#v", del)
	}
}

func TestRiskMaxAndTenantAllow(t *testing.T) {
	t.Parallel()
	if toolregistry.Max(toolregistry.RiskReadOnly, toolregistry.RiskHigh) != toolregistry.RiskHigh {
		t.Fatal("Max failed")
	}
	reg := toolregistry.New()
	if err := reg.Register(toolregistry.Definition{
		Name: "secret.tool", Risk: toolregistry.RiskMedium, AllowedTenants: []string{"t1"},
	}); err != nil {
		t.Fatal(err)
	}
	def, _ := reg.Get("secret.tool")
	if !def.AllowedForTenant("t1") || def.AllowedForTenant("t2") {
		t.Fatalf("%#v", def)
	}
}

func TestRegisterRejectsInvalidRisk(t *testing.T) {
	t.Parallel()
	reg := toolregistry.New()
	if err := reg.Register(toolregistry.Definition{Name: "x", Risk: "BOOM"}); err == nil {
		t.Fatal("expected error")
	}
}
