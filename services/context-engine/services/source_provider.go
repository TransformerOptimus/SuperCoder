package services

import "context"

// FileInfo describes a file from any source (local, S3, GitHub sandbox).
type FileInfo struct {
	Path         string
	RelativePath string
	Size         int64
	IsDir        bool
}

// SourceProvider abstracts file access across different storage backends.
type SourceProvider interface {
	ListFiles(ctx context.Context, root string) ([]FileInfo, error)
	ReadFile(ctx context.Context, path string) ([]byte, error)
	GetFileHash(ctx context.Context, path string) (string, error)
	Type() string // "local", "s3", "github"
	Close() error
}
