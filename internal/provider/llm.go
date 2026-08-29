package provider

import "context"

// CompletionRequest is a provider-agnostic chat completion request.
type CompletionRequest struct {
	Model       string
	System      string
	User        string
	Temperature float64
	MaxTokens   int
}

// CompletionResponse is the model output text.
type CompletionResponse struct {
	Content string
	Model   string
}

// LLMProvider generates text completions without binding to a vendor SDK.
type LLMProvider interface {
	Complete(ctx context.Context, req CompletionRequest) (CompletionResponse, error)
}
