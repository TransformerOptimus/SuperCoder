package repositories

import (
	"context"

	"github.com/TransformerOptimus/SuperCoder/services/context-engine/services"
)

type SearchResult struct {
	ChunkID  string
	Content  string
	FilePath string
	Language string
	Score    float32
}

type VectorRepository interface {
	EnsureCollection(ctx context.Context, collection string, dim uint64) error
	EnsurePayloadIndexes(ctx context.Context, collection string) error
	Upsert(ctx context.Context, collection string, chunks []services.CodeChunk, vectors [][]float32) error
	Search(ctx context.Context, collection string, queryVec []float32, limit int, filter services.SearchFilter) ([]SearchResult, error)
	IsEmpty(ctx context.Context, collection string) (bool, error)
	GetByChunkID(ctx context.Context, collection string, chunkID string) (string, error)
	DeleteByFilePath(ctx context.Context, collection string, filePath string) error
	DeleteByRepoID(ctx context.Context, collection string, repoID uint) error
	DeleteCollection(ctx context.Context, collection string) error
	ListFilePaths(ctx context.Context, collection string, filter services.SearchFilter) ([]string, error)
	GetChunksByFilePath(ctx context.Context, collection string, filePath string) ([]string, error)
}
