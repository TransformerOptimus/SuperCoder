package services

import "context"

// StreamingIndexer is the narrow contract the WS5 stream_batch worker uses to
// drive Pipeline.IndexChangedFiles without depending on the full Pipeline
// struct. *impl.Pipeline satisfies this interface structurally, so the dig
// container registers an adapter provider that returns the Pipeline as a
// StreamingIndexer (see injection/worker_container.go).
//
// The interface exists to keep the stream_batch consumer unit-testable: tests
// pass an in-memory mock instead of standing up the real pipeline with its
// Qdrant / FalkorDB / embedder dependencies.
type StreamingIndexer interface {
	IndexChangedFiles(
		ctx context.Context,
		collectionName string,
		indexRoot string,
		identity IndexIdentity,
		provider SourceProvider,
		changedFiles []string,
		deletedFiles []string,
	) (*IndexStats, error)
}
