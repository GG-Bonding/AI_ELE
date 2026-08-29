package provider

import (
	"context"
	"fmt"
	"sync"
)

// MockLLM is a test double for LLMProvider.
type MockLLM struct {
	mu        sync.Mutex
	Responses []string
	Errs      []error
	Calls     []CompletionRequest
}

// Complete returns the next scripted response or error.
func (m *MockLLM) Complete(ctx context.Context, req CompletionRequest) (CompletionResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Calls = append(m.Calls, req)

	var err error
	if len(m.Errs) > 0 {
		err = m.Errs[0]
		m.Errs = m.Errs[1:]
	}
	if err != nil {
		return CompletionResponse{}, err
	}
	if len(m.Responses) == 0 {
		return CompletionResponse{}, fmt.Errorf("mock llm: no scripted responses left")
	}
	content := m.Responses[0]
	m.Responses = m.Responses[1:]
	return CompletionResponse{Content: content, Model: "mock"}, nil
}
