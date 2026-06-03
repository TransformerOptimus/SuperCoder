package repositories

import (
	"context"

	"github.com/TransformerOptimus/SuperCoder/services/context-engine/services"
)

type GraphResult struct {
	Name      string
	ChunkID   string
	FilePath  string
	Depth     int
	StartLine int
	EndLine   int
}

type GraphRepository interface {
	EnsureIndices(ctx context.Context, graphName string) error
	UpsertElements(ctx context.Context, graphName string, elements []services.CodeElement) error
	UpsertFileNodes(ctx context.Context, graphName string, elements []services.CodeElement) error
	UpsertRelationships(ctx context.Context, graphName string, rels []services.CodeRelationship, elements []services.CodeElement) error
	GetBlastRadius(ctx context.Context, graphName string, functionName, filePath string, filter services.SearchFilter) ([]GraphResult, error)
	GetDependencies(ctx context.Context, graphName string, functionName, filePath string, filter services.SearchFilter) ([]GraphResult, error)
	GetImporters(ctx context.Context, graphName string, filePath string, filter services.SearchFilter) ([]GraphResult, error)
	GetFunctionsByFile(ctx context.Context, graphName string, filePath string, filter services.SearchFilter) ([]GraphResult, error)
	SearchFunctions(ctx context.Context, graphName string, query string, filter services.SearchFilter, limit int) ([]GraphResult, error)
	DeleteByFilePath(ctx context.Context, graphName string, filePath string) error
	DeleteByRepoID(ctx context.Context, graphName string, repoID uint) error
	DeleteGraph(ctx context.Context, graphName string) error
	Exists(ctx context.Context, graphName string) (bool, error)
}
