package services

import "context"

// SearchFilter carries all IDs for scoped retrieval. Callers choose which to filter by.
type SearchFilter struct {
	UserID      string
	WorkspaceID string
	MachineID   string
	RepoID      uint
	OrgID       string
}

// RetrieverResult is a unified retrieval result regardless of source.
type RetrieverResult struct {
	ChunkID  string
	Content  string
	FilePath string
	Language string
	Score    float32
	Source   string // "vector", "graph", "hybrid"
}

// RetrieverService is the pluggable interface for context retrieval strategies.
type RetrieverService interface {
	Search(ctx context.Context, query string, limit int, filter SearchFilter) ([]RetrieverResult, error)
	FindFunction(ctx context.Context, functionName string, filter SearchFilter) ([]RetrieverResult, error)
	ListFiles(ctx context.Context, filter SearchFilter) ([]string, error)
	ReadFile(ctx context.Context, path string, filter SearchFilter) (string, error)
	Name() string
}
