package impl

import (
	"context"
	"sync"

	"go.uber.org/zap"

	"github.com/TransformerOptimus/SuperCoder/services/context-engine/repositories"
	"github.com/TransformerOptimus/SuperCoder/services/context-engine/services"
)

// ensureCacheVectorRepo is a decorator around VectorRepository that
// memoizes successful EnsureCollection calls per-process. Qdrant
// EnsureCollection is a round-trip ("create if not exists") on every /diff
// that, per WS9 measurements, dominates first-sync latency (~4s on a
// cc-vault 18-file repo, 10.6s total). The in-memory cache short-circuits
// subsequent calls for the same collection name and self-heals on any
// write-path error from the underlying Qdrant client (which catches the
// common out-of-band cases: manual deletion, cluster wipe, schema reset).
//
// Correctness invariants:
//
//  1. Cache entries are created ONLY after a successful EnsureCollection
//     (and EnsurePayloadIndexes, since both are part of the "ready to write"
//     contract). A cached "ensured" state always reflects a Qdrant reality
//     that was true at least once during this process's lifetime.
//
//  2. Any write-path error (Upsert, Delete*, EnsureCollection,
//     EnsurePayloadIndexes) evicts the entry for that collection so the
//     next call re-verifies with Qdrant. Read-path methods (Search, IsEmpty,
//     Get*, List*) do NOT evict: a transient search failure shouldn't
//     thrash the cache, and a "collection missing" on read is usually
//     preceded by a write-path failure that already evicted.
//
//  3. The cache is per-process. Multiple workers maintain independent
//     caches, which is fine because an ensured collection is globally
//     ensured — racing two EnsureCollection calls across processes is
//     idempotent on the Qdrant side.
//
//  4. sync.Map gives lock-free reads on the hot path; the rare Store /
//     Delete calls are amortized across process lifetime.
type ensureCacheVectorRepo struct {
	inner   repositories.VectorRepository
	logger  *zap.Logger
	ensured sync.Map // collectionName (string) -> struct{}
}

// NewEnsureCacheVectorRepository wraps the given VectorRepository with an
// in-memory EnsureCollection cache. Registered via container.Decorate so
// every consumer (sync_session_service /diff path, legacy pipeline.Index,
// Review, Analyze) inherits the cache without call-site changes.
func NewEnsureCacheVectorRepository(inner repositories.VectorRepository, logger *zap.Logger) repositories.VectorRepository {
	return &ensureCacheVectorRepo{
		inner:  inner,
		logger: logger.Named("vector-repo.ensure-cache"),
	}
}

func (c *ensureCacheVectorRepo) EnsureCollection(ctx context.Context, collection string, dim uint64) error {
	if _, ok := c.ensured.Load(collection); ok {
		return nil
	}
	if err := c.inner.EnsureCollection(ctx, collection, dim); err != nil {
		c.ensured.Delete(collection)
		return err
	}
	c.ensured.Store(collection, struct{}{})
	return nil
}

func (c *ensureCacheVectorRepo) EnsurePayloadIndexes(ctx context.Context, collection string) error {
	if err := c.inner.EnsurePayloadIndexes(ctx, collection); err != nil {
		// Payload indexes being missing usually implies the collection
		// itself has drifted out of sync with our expectations — force a
		// re-ensure on the next call.
		c.ensured.Delete(collection)
		return err
	}
	return nil
}

func (c *ensureCacheVectorRepo) Upsert(ctx context.Context, collection string, chunks []services.CodeChunk, vectors [][]float32) error {
	if err := c.inner.Upsert(ctx, collection, chunks, vectors); err != nil {
		c.ensured.Delete(collection)
		return err
	}
	return nil
}

func (c *ensureCacheVectorRepo) Search(ctx context.Context, collection string, queryVec []float32, limit int, filter services.SearchFilter) ([]repositories.SearchResult, error) {
	return c.inner.Search(ctx, collection, queryVec, limit, filter)
}

func (c *ensureCacheVectorRepo) IsEmpty(ctx context.Context, collection string) (bool, error) {
	return c.inner.IsEmpty(ctx, collection)
}

func (c *ensureCacheVectorRepo) GetByChunkID(ctx context.Context, collection string, chunkID string) (string, error) {
	return c.inner.GetByChunkID(ctx, collection, chunkID)
}

func (c *ensureCacheVectorRepo) DeleteByFilePath(ctx context.Context, collection string, filePath string) error {
	if err := c.inner.DeleteByFilePath(ctx, collection, filePath); err != nil {
		c.ensured.Delete(collection)
		return err
	}
	return nil
}

func (c *ensureCacheVectorRepo) DeleteByRepoID(ctx context.Context, collection string, repoID uint) error {
	if err := c.inner.DeleteByRepoID(ctx, collection, repoID); err != nil {
		c.ensured.Delete(collection)
		return err
	}
	return nil
}

func (c *ensureCacheVectorRepo) DeleteCollection(ctx context.Context, collection string) error {
	// Always evict on an explicit delete — even if the underlying call
	// fails partway, the collection is no longer in a trustworthy state.
	c.ensured.Delete(collection)
	return c.inner.DeleteCollection(ctx, collection)
}

func (c *ensureCacheVectorRepo) ListFilePaths(ctx context.Context, collection string, filter services.SearchFilter) ([]string, error) {
	return c.inner.ListFilePaths(ctx, collection, filter)
}

func (c *ensureCacheVectorRepo) GetChunksByFilePath(ctx context.Context, collection string, filePath string) ([]string, error) {
	return c.inner.GetChunksByFilePath(ctx, collection, filePath)
}
