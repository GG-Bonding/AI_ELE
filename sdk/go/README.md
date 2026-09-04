# Go SDK

Thin client for the Agent Experience Learning Engine HTTP API (V1 + V2 surfaces).

```go
import experienceclient "github.com/agent-experience-engine/agent-experience-engine/sdk/go"

client, _ := experienceclient.New("http://localhost:8080",
    experienceclient.WithTenant("tenant_a"),
    experienceclient.WithAgent("agent_01"),
    experienceclient.WithUser("user_01"),
)

handle, _, _ := client.StartEpisode(ctx, experienceclient.StartEpisodeInput{
    Goal: "Create Jira issue",
})
ctxPayload, _ := handle.GetContext(ctx, experienceclient.GetContextInput{
    Task: "Create a Jira issue", Tools: []string{"jira"},
})
call, _ := handle.ToolCall(ctx, ctxPayload.ContextID, "jira.create_issue",
    map[string]any{"project": "PAY", "priority": "High"}, "SUCCESS")
reward := -1.0
_, _ = handle.Feedback(ctx, experienceclient.FeedbackInput{
    Source: "human", Reward: &reward, Confidence: 1,
    Target: call.Field("priority"),
})
_, _ = client.EvolvePatterns(ctx, "")
```

Runnable example:

```bash
EXPERIENCE_ENGINE_URL=http://localhost:8080 go run ./sdk/go/example
```
