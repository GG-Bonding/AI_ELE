package provider

import (
	"context"
	"fmt"
	"hash/fnv"
	"math"
)

// MockEmbedding produces deterministic unit vectors for tests.
type MockEmbedding struct {
	Dim int
}

// Dimensions implements EmbeddingProvider.
func (m *MockEmbedding) Dimensions() int {
	if m.Dim <= 0 {
		return 8
	}
	return m.Dim
}

// Embed implements EmbeddingProvider.
func (m *MockEmbedding) Embed(_ context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, fmt.Errorf("mock embed: texts is empty")
	}
	dim := m.Dimensions()
	out := make([][]float32, len(texts))
	for i, text := range texts {
		out[i] = hashEmbed(text, dim)
	}
	return out, nil
}

func hashEmbed(text string, dim int) []float32 {
	vec := make([]float32, dim)
	h := fnv.New64a()
	_, _ = h.Write([]byte(text))
	seed := h.Sum64()
	var sum float64
	for i := 0; i < dim; i++ {
		seed = seed*6364136223846793005 + 1
		v := float64(int64(seed%2001)-1000) / 1000.0
		vec[i] = float32(v)
		sum += v * v
	}
	norm := math.Sqrt(sum)
	if norm == 0 {
		vec[0] = 1
		return vec
	}
	for i := range vec {
		vec[i] = float32(float64(vec[i]) / norm)
	}
	return vec
}
