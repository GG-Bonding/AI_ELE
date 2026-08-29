package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// OpenAICompatEmbeddingConfig configures OpenAI-compatible embeddings.
type OpenAICompatEmbeddingConfig struct {
	BaseURL    string
	APIKey     string
	Model      string
	Dimensions int
	HTTPClient *http.Client
}

// OpenAICompatEmbedding calls /v1/embeddings.
type OpenAICompatEmbedding struct {
	baseURL    string
	apiKey     string
	model      string
	dimensions int
	client     *http.Client
}

// NewOpenAICompatEmbedding constructs an embedding provider.
func NewOpenAICompatEmbedding(cfg OpenAICompatEmbeddingConfig) (*OpenAICompatEmbedding, error) {
	base := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	if base == "" {
		return nil, fmt.Errorf("embedding base_url is required")
	}
	if strings.TrimSpace(cfg.Model) == "" {
		return nil, fmt.Errorf("embedding model is required")
	}
	dims := cfg.Dimensions
	if dims <= 0 {
		dims = 1536
	}
	client := cfg.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 60 * time.Second}
	}
	return &OpenAICompatEmbedding{
		baseURL:    base,
		apiKey:     cfg.APIKey,
		model:      cfg.Model,
		dimensions: dims,
		client:     client,
	}, nil
}

func (p *OpenAICompatEmbedding) Dimensions() int { return p.dimensions }

type embeddingRequest struct {
	Model string   `json:"model"`
	Input []string `json:"input"`
}

type embeddingResponse struct {
	Data []struct {
		Embedding []float32 `json:"embedding"`
		Index     int       `json:"index"`
	} `json:"data"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// Embed implements EmbeddingProvider.
func (p *OpenAICompatEmbedding) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, fmt.Errorf("embed: texts is empty")
	}
	body, err := json.Marshal(embeddingRequest{Model: p.model, Input: texts})
	if err != nil {
		return nil, fmt.Errorf("marshal embedding request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/embeddings", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build embedding request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if p.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)
	}

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("embeddings request: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read embeddings response: %w", err)
	}

	var parsed embeddingResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("decode embeddings response: %w", err)
	}
	if parsed.Error != nil && parsed.Error.Message != "" {
		return nil, fmt.Errorf("embedding api error: %s", parsed.Error.Message)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("embedding api status %d: %s", resp.StatusCode, string(raw))
	}
	if len(parsed.Data) != len(texts) {
		return nil, fmt.Errorf("embedding api returned %d vectors for %d texts", len(parsed.Data), len(texts))
	}

	out := make([][]float32, len(texts))
	for _, item := range parsed.Data {
		if item.Index < 0 || item.Index >= len(out) {
			return nil, fmt.Errorf("embedding api returned invalid index %d", item.Index)
		}
		if len(item.Embedding) == 0 {
			return nil, fmt.Errorf("embedding api returned empty vector at index %d", item.Index)
		}
		out[item.Index] = item.Embedding
	}
	for i, v := range out {
		if v == nil {
			return nil, fmt.Errorf("embedding api missing vector for index %d", i)
		}
	}
	return out, nil
}
