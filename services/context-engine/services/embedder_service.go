package services

import "context"

// EmbedderService generates vector embeddings for text content.
type EmbedderService interface {
	Embed(ctx context.Context, texts []string) ([][]float32, error)
	EmbedSingle(ctx context.Context, text string) ([]float32, error)
}
