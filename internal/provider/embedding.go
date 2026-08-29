package provider

import "context"

// EmbeddingProvider turns text into dense vectors.
type EmbeddingProvider interface {
	Embed(ctx context.Context, texts []string) ([][]float32, error)
	Dimensions() int
}
