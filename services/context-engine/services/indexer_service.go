package services

import "context"

// IndexResult holds the output of an indexing operation.
type IndexResult struct {
	Elements      []CodeElement
	Relationships []CodeRelationship
}

// IndexerService abstracts code indexing backends (tree-sitter, etc.).
type IndexerService interface {
	IndexDirectory(ctx context.Context, provider SourceProvider, root string) (*IndexResult, error)
	IndexFiles(ctx context.Context, provider SourceProvider, root string, changedFiles []string) (*IndexResult, error)
	SupportedLanguages() []string
}
