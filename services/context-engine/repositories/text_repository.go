package repositories

import (
	"context"

	"github.com/TransformerOptimus/SuperCoder/services/context-engine/services"
)

// TextSearchResult represents a BM25 text search hit.
type TextSearchResult struct {
	ChunkID  string
	Content  string
	FilePath string
	Language string
	Score    float64
}

// TextSearchRepository provides keyword (BM25) search over indexed code chunks.
type TextSearchRepository interface {
	EnsureIndex(ctx context.Context, collection string) error
	IndexChunks(ctx context.Context, collection string, chunks []services.CodeChunk) error
	Search(ctx context.Context, collection string, query string, limit int, filter services.SearchFilter) ([]TextSearchResult, error)
	DeleteByFilePath(ctx context.Context, collection string, filePath string) error
	DeleteByRepoID(ctx context.Context, collection string, repoID uint) error
	DeleteIndex(ctx context.Context, collection string) error
}
