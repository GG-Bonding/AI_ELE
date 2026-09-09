package toolprovider_test

import (
	"context"
	"testing"

	"github.com/agent-experience-engine/agent-experience-engine/internal/skillruntime"
	"github.com/agent-experience-engine/agent-experience-engine/internal/toolprovider"
	"github.com/agent-experience-engine/agent-experience-engine/internal/toolprovider/simulator"
	"github.com/agent-experience-engine/agent-experience-engine/internal/toolregistry"
)

func TestRouterSimulatorExecute(t *testing.T) {
	t.Parallel()
	tools := toolregistry.Default()
	router := toolprovider.NewRouter([]toolprovider.Provider{
		&simulator.JiraProvider{Registry: tools},
	}, tools, nil)
	if err := router.SyncRegistry(context.Background()); err != nil {
		t.Fatal(err)
	}
	got, err := router.ExecuteToolCall(context.Background(), skillruntime.ToolCall{
		Tool: "jira.search_projects", Input: map[string]any{"query": "Payment"},
		IdempotencyKey: toolprovider.ToolIdempotencyKey("ex1", "s1", 1),
	})
	if err != nil || !got.OK {
		t.Fatalf("%#v err=%v", got, err)
	}
	if got.Output["key"] == nil && got.Output["projects"] == nil {
		t.Fatalf("expected project payload %#v", got.Output)
	}
}
