package toolregistry_test

import (
	"testing"

	"github.com/agent-experience-engine/agent-experience-engine/internal/toolregistry"
)

func TestValidateInputRequiredAndTypes(t *testing.T) {
	t.Parallel()
	schema := map[string]toolregistry.ParamSchema{
		"project": {Type: toolregistry.ParamString, Required: true},
		"count":   {Type: toolregistry.ParamNumber},
	}
	if err := toolregistry.ValidateInput(schema, map[string]any{"count": 1}); err == nil {
		t.Fatal("expected missing project")
	}
	if err := toolregistry.ValidateInput(schema, map[string]any{"project": "PAY", "count": "x"}); err == nil {
		t.Fatal("expected wrong type")
	}
	if err := toolregistry.ValidateInput(schema, map[string]any{"project": "PAY", "count": float64(2)}); err != nil {
		t.Fatal(err)
	}
}
