package services

import (
	"context"
	"fmt"
	"sync"
)

// QueueFile is the unit the streaming Asynq worker (WS5) pulls out of Redis
// and feeds to InMemorySourceProvider. Hash is the client-computed sha256
// that /stream validated; Content is the raw bytes.
//
// This type and InMemorySourceProvider live in package `services` (not
// `services/impl`) so the `consumers` package can construct them without
// importing `services/impl`, which would close an import cycle
// (services/impl → producers → consumers → services/impl).
type QueueFile struct {
	Path    string
	Hash    string
	Content []byte
}

// InMemorySourceProvider implements SourceProvider with no I/O. It is the
// provider the streaming worker hands to Pipeline.IndexChangedFiles so the
// pipeline can reuse the existing tree-sitter + enrichment + embed flow
// without touching disk or S3.
//
// WARNING: Never call MerkleService.BuildTree with this provider. The
// streaming path calls Pipeline.IndexChangedFiles directly and skips Merkle
// entirely. BuildTree would walk the partial file set, produce a Merkle tree
// of only the batch's files, and — when diffed against the full stored tree —
// conclude that every file not in the batch was "deleted", corrupting the
// index. The legacy Pipeline.Index path uses LocalSourceProvider; this
// provider is for the streaming path only.
type InMemorySourceProvider struct {
	mu    sync.RWMutex
	files map[string]inMemFile
}

type inMemFile struct {
	content []byte
	hash    string
	size    int64
}

// NewInMemorySourceProvider builds a provider from a snapshot of queued files.
// The caller is responsible for Close()ing the provider when indexing is done
// to let the content bytes be garbage-collected promptly.
func NewInMemorySourceProvider(files []QueueFile) *InMemorySourceProvider {
	m := make(map[string]inMemFile, len(files))
	for _, f := range files {
		m[f.Path] = inMemFile{
			content: f.Content,
			hash:    f.Hash,
			size:    int64(len(f.Content)),
		}
	}
	return &InMemorySourceProvider{files: m}
}

// ListFiles returns every file held by the provider. The root argument is
// ignored — the provider is already scoped to exactly the files the worker
// wants to index.
func (p *InMemorySourceProvider) ListFiles(_ context.Context, _ string) ([]FileInfo, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	out := make([]FileInfo, 0, len(p.files))
	for path, f := range p.files {
		out = append(out, FileInfo{
			Path:         path,
			RelativePath: path,
			Size:         f.size,
			IsDir:        false,
		})
	}
	return out, nil
}

// ReadFile returns the cached content for path.
func (p *InMemorySourceProvider) ReadFile(_ context.Context, path string) ([]byte, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	f, ok := p.files[path]
	if !ok {
		return nil, fmt.Errorf("in-memory provider: file not found: %s", path)
	}
	return f.content, nil
}

// GetFileHash returns the client-computed sha256 for path (the same hash
// that /stream validated before the worker was enqueued).
func (p *InMemorySourceProvider) GetFileHash(_ context.Context, path string) (string, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	f, ok := p.files[path]
	if !ok {
		return "", fmt.Errorf("in-memory provider: file not found: %s", path)
	}
	return f.hash, nil
}

// Type returns the provider kind. Used by logging and the SourceProvider
// interface contract.
func (p *InMemorySourceProvider) Type() string { return "memory" }

// Close zeroes the file map to release content bytes promptly. Subsequent
// ReadFile / GetFileHash calls will return "file not found" errors.
func (p *InMemorySourceProvider) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	for k := range p.files {
		delete(p.files, k)
	}
	return nil
}
