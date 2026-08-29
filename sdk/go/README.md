# Go SDK

Thin client for the Agent Experience Learning Engine HTTP API.

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
_ = handle.AddAttempt(ctx, experienceclient.AddAttemptInput{Status: "FAILED"})
_, _ = handle.Complete(ctx, experienceclient.CompleteInput{Status: "SUCCESS"})

payload, _ := client.GetContext(ctx, experienceclient.GetContextInput{
    EpisodeID: handle.ID,
    Task:      "Create a Jira issue",
    Tools:     []string{"jira"},
})
reward := 1.0
_, _ = client.Feedback(ctx, experienceclient.FeedbackInput{
    EpisodeID: handle.ID, Source: "business", Reward: &reward, Confidence: 1,
})
_ = payload
```

Runnable example:

```bash
EXPERIENCE_ENGINE_URL=http://localhost:8080 go run ./sdk/go/example
```
