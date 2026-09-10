package mcp_test

import (
	"context"
	"testing"

	"github.com/agent-experience-engine/agent-experience-engine/internal/toolprovider"
	"github.com/agent-experience-engine/agent-experience-engine/internal/toolprovider/mcp"
	"github.com/agent-experience-engine/agent-experience-engine/internal/toolregistry"
)

func TestMCPPreviewValidatesInputSchema(t *testing.T) {
	t.Parallel()
	p, err := mcp.New(mcp.Config{
		BaseURL: "http://127.0.0.1:9", // unused for static preview
		StaticTools: []toolregistry.Definition{{
			Name: "jira.create_issue",
			InputSchema: map[string]toolregistry.ParamSchema{
				"project": {Type: toolregistry.ParamString, Required: true},
				"title":   {Type: toolregistry.ParamString, Required: true},
			},
			PreviewCapability: toolregistry.PreviewLocalValidate,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	// Seed schema cache via failed list → StaticTools path also used in Preview.
	_, _ = p.ListTools(context.Background())

	bad, err := p.Preview(context.Background(), toolprovider.Call{
		Tool: "jira.create_issue", Input: map[string]any{"project": "PAY"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if bad.OK || bad.ErrorCode != "SCHEMA_INVALID" {
		t.Fatalf("want SCHEMA_INVALID, got %+v", bad)
	}
	good, err := p.Preview(context.Background(), toolprovider.Call{
		Tool: "jira.create_issue", Input: map[string]any{"project": "PAY", "title": "x"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !good.OK {
		t.Fatalf("want ok preview, got %+v", good)
	}
}
