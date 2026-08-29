// Example: episode → context → feedback loop against a running Experience Engine.
//
//	EXPERIENCE_ENGINE_URL=http://localhost:8080 go run ./sdk/go/example
package main

import (
	"context"
	"fmt"
	"os"

	experienceclient "github.com/agent-experience-engine/agent-experience-engine/sdk/go"
)

func main() {
	base := os.Getenv("EXPERIENCE_ENGINE_URL")
	if base == "" {
		base = "http://localhost:8080"
	}
	client, err := experienceclient.New(base,
		experienceclient.WithTenant("tenant_a"),
		experienceclient.WithAgent("agent_01"),
		experienceclient.WithUser("user_01"),
	)
	if err != nil {
		panic(err)
	}
	ctx := context.Background()

	handle, ep, err := client.StartEpisode(ctx, experienceclient.StartEpisodeInput{
		TaskType: "jira.create_issue",
		Goal:     "Create Jira issue",
		Input:    "project=Payment",
	})
	if err != nil {
		panic(err)
	}
	fmt.Println("episode:", ep.ID)

	payload, err := client.GetContext(ctx, experienceclient.GetContextInput{
		EpisodeID:      handle.ID,
		Task:           "Create a Jira issue for payment timeout",
		Tools:          []string{"jira.search_projects", "jira.create_issue"},
		MaxExperiences: 5,
	})
	if err != nil {
		panic(err)
	}
	fmt.Printf("context experiences: %d (disclaimer=%q)\n", len(payload.Experiences), payload.Disclaimer)

	if _, err := handle.AddAttempt(ctx, experienceclient.AddAttemptInput{
		Action: "create_issue", ToolName: "jira.create_issue", Status: "FAILED", ErrorCode: "INVALID_PROJECT_KEY",
	}); err != nil {
		panic(err)
	}
	if _, err := handle.Complete(ctx, experienceclient.CompleteInput{
		Status: "SUCCESS", Verified: true, Verifier: "tool",
	}); err != nil {
		panic(err)
	}

	reward := 1.0
	fb, err := client.Feedback(ctx, experienceclient.FeedbackInput{
		EpisodeID: handle.ID, Source: "business", Reward: &reward, Confidence: 1,
	})
	if err != nil {
		panic(err)
	}
	fmt.Printf("feedback ok; utility_updates=%d\n", len(fb.UtilityUpdates))
}
