package impl

import (
	"context"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"

	"github.com/TransformerOptimus/SuperCoder/services/context-engine/config"
	"github.com/TransformerOptimus/SuperCoder/services/context-engine/models/dto"
	"github.com/TransformerOptimus/SuperCoder/services/context-engine/repositories"
	"github.com/TransformerOptimus/SuperCoder/services/context-engine/services"
)

type Pipeline struct {
	anthropicCfg config.AnthropicConfig
	reviewerCfg  config.ReviewerConfig
	openAICfg    config.OpenAIConfig
	indexerCfg   config.IndexerConfig
	store        repositories.VectorRepository
	graph        repositories.GraphRepository
	textRepo     repositories.TextSearchRepository
	repoRepo     repositories.RepoRepository
	shardRepo    repositories.ShardRepository
	embedder     services.EmbedderService
	indexer      services.IndexerService
	merkle       services.MerkleService
	prompts      *PromptProvider
	logger       *zap.Logger
}

func NewPipeline(
	anthropicCfg config.AnthropicConfig,
	reviewerCfg config.ReviewerConfig,
	openAICfg config.OpenAIConfig,
	indexerCfg config.IndexerConfig,
	store repositories.VectorRepository,
	graph repositories.GraphRepository,
	textRepo repositories.TextSearchRepository,
	repoRepo repositories.RepoRepository,
	shardRepo repositories.ShardRepository,
	embedder services.EmbedderService,
	indexer services.IndexerService,
	merkle services.MerkleService,
	prompts *PromptProvider,
	logger *zap.Logger,
) *Pipeline {
	return &Pipeline{
		anthropicCfg: anthropicCfg,
		reviewerCfg:  reviewerCfg,
		openAICfg:    openAICfg,
		indexerCfg:   indexerCfg,
		store:        store,
		graph:        graph,
		textRepo:     textRepo,
		repoRepo:     repoRepo,
		shardRepo:    shardRepo,
		embedder:     embedder,
		indexer:      indexer,
		merkle:       merkle,
		prompts:      prompts,
		logger:       logger.Named("pipeline"),
	}
}

// resolveCollection resolves the shard collection name and repo DB ID for the given request.
func (p *Pipeline) resolveCollection(ctx context.Context, req *dto.IndexRequest) (string, uint, error) {
	if p.repoRepo == nil || p.shardRepo == nil {
		return "", 0, fmt.Errorf("repo or shard repository not configured")
	}

	repo, err := p.repoRepo.FindOrCreate(ctx, req.UserID, req.WorkspaceID, req.MachineID, req.RepoPath, req.RepoURL)
	if err != nil {
		return "", 0, fmt.Errorf("failed to resolve repo: %w", err)
	}

	assignment, err := p.shardRepo.AssignShard(ctx, repo.ID)
	if err != nil {
		return "", 0, fmt.Errorf("failed to assign shard: %w", err)
	}

	return assignment.CollectionName, repo.ID, nil
}

// resolveCollectionForReview resolves collection for a review request.
func (p *Pipeline) resolveCollectionForReview(ctx context.Context, req *dto.ReviewRequest) (string, uint, error) {
	if p.repoRepo == nil || p.shardRepo == nil {
		return "", 0, fmt.Errorf("repo or shard repository not configured")
	}

	if req.RepoDBID > 0 {
		assignment, err := p.shardRepo.GetAssignment(ctx, req.RepoDBID)
		if err != nil {
			return "", 0, fmt.Errorf("failed to get shard assignment: %w", err)
		}
		return assignment.CollectionName, req.RepoDBID, nil
	}

	repo, err := p.repoRepo.FindOrCreate(ctx, req.UserID, req.WorkspaceID, req.MachineID, req.RepoID, "")
	if err != nil {
		return "", 0, fmt.Errorf("failed to resolve repo: %w", err)
	}

	assignment, err := p.shardRepo.AssignShard(ctx, repo.ID)
	if err != nil {
		return "", 0, fmt.Errorf("failed to assign shard: %w", err)
	}

	return assignment.CollectionName, repo.ID, nil
}

// IsCollectionPopulated reports whether the repo's vector collection has any
// points. Errors are treated as cold so the caller falls into the sync-index
// path rather than running a review without context.
func (p *Pipeline) IsCollectionPopulated(ctx context.Context, req *dto.IndexRequest) bool {
	collectionName, _, err := p.resolveCollection(ctx, req)
	if err != nil {
		return false
	}
	empty, err := p.store.IsEmpty(ctx, collectionName)
	if err != nil {
		return false
	}
	return !empty
}

// checkCollectionReadiness is an advisory log on whether the collection has
// data. The review proceeds either way.
func (p *Pipeline) checkCollectionReadiness(ctx context.Context, collectionName, repoSlug string) {
	empty, err := p.store.IsEmpty(ctx, collectionName)
	if err != nil {
		p.logger.Warn("vector store readiness probe failed — codebase context may be limited",
			zap.String("repo", repoSlug),
			zap.String("collection", collectionName),
			zap.Error(err))
		return
	}
	if empty {
		p.logger.Warn("vector store is empty — codebase context will be limited (async index may still be running)",
			zap.String("repo", repoSlug),
			zap.String("collection", collectionName))
		return
	}
	p.logger.Info("vector store has data — codebase tools available",
		zap.String("repo", repoSlug),
		zap.String("collection", collectionName))
}

// resolveSourceType normalises req.SourceType with the same fallback
// logic resolveSourceProvider uses. Extracted so callers that need the
// resolved type (e.g. to decide whether BuildTree should pass an empty
// root or req.RepoPath) can stay in sync with the provider factory
// without duplicating the switch.
func resolveSourceType(req *dto.IndexRequest) string {
	if req.SourceType != "" {
		return req.SourceType
	}
	switch {
	case req.S3Bucket != "":
		return "s3"
	case req.RepoURL != "":
		return "github"
	default:
		return "local"
	}
}

// resolveSourceProvider picks the appropriate SourceProvider based on request fields.
func (p *Pipeline) resolveSourceProvider(req *dto.IndexRequest) (services.SourceProvider, error) {
	switch resolveSourceType(req) {
	case "s3":
		if req.S3Bucket != "" {
			return NewS3SourceProviderWithParams(req.S3Bucket, req.S3Prefix, "", p.indexerCfg.S3Endpoint(), p.logger)
		}
		return NewS3SourceProvider(p.indexerCfg, p.logger)
	case "github":
		return nil, fmt.Errorf("github source type is not supported; use S3 archive instead")
	default:
		return NewLocalSourceProvider(p.indexerCfg), nil
	}
}

// legacyIndexRoot returns the root that Pipeline.Index should pass into
// merkle.BuildTree and IndexChangedFiles. S3 uses "" so the S3 provider
// falls back to its configured bucket prefix; every other source type
// uses req.RepoPath so the local/github walker operates on the caller's
// repo instead of whatever allowed-root the provider was built with.
func legacyIndexRoot(req *dto.IndexRequest) string {
	if resolveSourceType(req) == "s3" {
		return ""
	}
	return req.RepoPath
}

// Index is the legacy repo-level entry point used by /index/trigger and
// codereview. It resolves the collection, builds a fresh Merkle tree from
// the source provider, diffs it against the stored tree, and delegates the
// actual indexing work to IndexChangedFiles. The Merkle tree is saved only
// when the delegate reports zero per-file failures (Delta A in the WS2 plan)
// so any failed files are automatically retried on the next Index call.
func (p *Pipeline) Index(ctx context.Context, req *dto.IndexRequest) error {
	m := services.NewIndexMetrics()

	collectionName, repoID, err := p.resolveCollection(ctx, req)
	if err != nil {
		return fmt.Errorf("resolve collection failed: %w", err)
	}

	dim := uint64(p.openAICfg.EmbeddingDimensions())

	p.logger.Info("Starting indexing",
		zap.String("path", req.RepoPath),
		zap.String("collection", collectionName),
		zap.Uint("repo_id", repoID),
		zap.Bool("reindex", req.Reindex))

	// Ensure vector collection exists.
	if err := p.store.EnsureCollection(ctx, collectionName, dim); err != nil {
		return fmt.Errorf("ensure collection failed: %w", err)
	}

	// Ensure payload indexes. Retry once — collection may need a moment after creation.
	if err := p.store.EnsurePayloadIndexes(ctx, collectionName); err != nil {
		p.logger.Debug("Payload index creation failed, retrying after delay", zap.Error(err))
		time.Sleep(2 * time.Second)
		if err := p.store.EnsurePayloadIndexes(ctx, collectionName); err != nil {
			p.logger.Warn("Failed to ensure payload indexes after retry", zap.Error(err))
		}
	}

	// Ensure text search index exists.
	if err := p.textRepo.EnsureIndex(ctx, collectionName); err != nil {
		return fmt.Errorf("ensure text index failed: %w", err)
	}

	merkleKey := fmt.Sprintf("repo_%d", repoID)

	// Handle full reindex — delete only this repo's data from the shared shard.
	if req.Reindex {
		p.logger.Warn("Deleting repo data for reindex",
			zap.String("collection", collectionName),
			zap.Uint("repo_id", repoID))
		if err := p.store.DeleteByRepoID(ctx, collectionName, repoID); err != nil {
			p.logger.Warn("failed to delete vectors for reindex", zap.String("collection", collectionName), zap.Error(err))
		}
		if err := p.graph.DeleteByRepoID(ctx, collectionName, repoID); err != nil {
			p.logger.Warn("failed to delete graph for reindex", zap.String("collection", collectionName), zap.Error(err))
		}
		if err := p.textRepo.DeleteByRepoID(ctx, collectionName, repoID); err != nil {
			p.logger.Warn("failed to delete text index for reindex", zap.String("collection", collectionName), zap.Error(err))
		}
		if err := p.merkle.DeleteTree(ctx, merkleKey); err != nil {
			p.logger.Warn("Failed to delete merkle tree for reindex", zap.Error(err))
		}
	}

	// Resolve source provider.
	provider, err := p.resolveSourceProvider(req)
	if err != nil {
		return fmt.Errorf("failed to resolve source provider: %w", err)
	}
	defer provider.Close()

	// Build Merkle tree and compute diff. S3 passes "" so the provider
	// falls back to its configured bucket prefix; local/github pass
	// req.RepoPath so the walker operates on the caller's repo rather
	// than whatever allowed-root LocalSourceProvider was built with.
	// The same indexRoot is reused below for IndexChangedFiles so
	// per-file hash lookups resolve against the same directory.
	indexRoot := legacyIndexRoot(req)
	newTree, err := p.merkle.BuildTree(ctx, provider, indexRoot)
	if err != nil {
		return fmt.Errorf("failed to build merkle tree: %w", err)
	}

	var diff *services.MerkleDiff
	if req.Reindex {
		// Force all files to be re-indexed.
		var allFiles []string
		collectAllFiles(newTree, &allFiles)
		p.logger.Info("Reindex: collected files from merkle tree", zap.Int("files", len(allFiles)))
		diff = &services.MerkleDiff{Added: allFiles}
	} else {
		oldTree, err := p.merkle.LoadTree(ctx, merkleKey)
		if err != nil {
			p.logger.Warn("Failed to load old merkle tree, treating as full index", zap.Error(err))
		}
		diff = p.merkle.DiffTrees(oldTree, newTree)
	}

	changedFiles := append(diff.Added, diff.Changed...)
	if len(changedFiles) == 0 && len(diff.Deleted) == 0 {
		p.logger.Info("Codebase is up to date. No changes detected.")
		return nil
	}

	p.logger.Info("Merkle diff computed",
		zap.Int("added", len(diff.Added)),
		zap.Int("changed", len(diff.Changed)),
		zap.Int("deleted", len(diff.Deleted)))

	identity := services.IndexIdentity{
		UserID:      req.UserID,
		WorkspaceID: req.WorkspaceID,
		MachineID:   req.MachineID,
		GithubOrgID: req.GithubOrgID,
		RepoID:      repoID,
	}

	stats, err := p.IndexChangedFiles(ctx, collectionName, indexRoot, identity, provider, changedFiles, diff.Deleted)
	if err != nil {
		return err
	}

	// Delta A: skip Merkle save when any per-file failure occurred. If we
	// saved the tree with failed files recorded as "indexed at hash X", the
	// next Index call wouldn't re-attempt them until their content changed.
	// Leaving the tree unsaved keeps the failed files in the next diff so
	// they retry automatically.
	failedFiles := len(stats.FailedFiles)
	failedDeletes := len(stats.FailedDeletes)
	if failedFiles > 0 || failedDeletes > 0 {
		p.logger.Warn("legacy index had per-file failures; skipping merkle save so failed files retry next run",
			zap.Int("failed_files", failedFiles),
			zap.Int("failed_deletes", failedDeletes))
	} else if err := p.merkle.SaveTree(ctx, merkleKey, newTree); err != nil {
		p.logger.Warn("Failed to save merkle tree", zap.Error(err))
	}

	m.FilesIndexed = len(stats.ProcessedFiles)
	m.Complete()
	p.logger.Info("Indexing complete",
		zap.Int("files_indexed", m.FilesIndexed),
		zap.Int("failed_files", failedFiles),
		zap.Int("processed_deletes", len(stats.ProcessedDeletes)),
		zap.Duration("duration", m.TotalDuration))
	return nil
}

// IndexChangedFiles indexes exactly the specified files and removes the
// specified paths from Qdrant, FalkorDB, and the text store. It does NOT
// touch the Merkle tree, resolve the collection name, or decide which files
// are "changed" — all of that is the caller's responsibility.
//
// Contract (see services/pipeline_identity.go for the full description):
//
//	return (stats, nil) → batch reached a commit-worthy state; streaming
//	                      workers call markBatchProcessed(stats).
//	return (nil,   err) → whole-batch failure. TerminalError wraps errors
//	                      that must not be retried (markBatchTerminallyFailed
//	                      via the durable manifest); ordinary errors bubble
//	                      up for Asynq retry.
//	never (stats, err)  → forbidden; would double-account.
//
// Callers supplying an InMemorySourceProvider MUST pass indexRoot="". The
// legacy Pipeline.Index wrapper passes req.RepoPath (or "" for S3) to match
// the pre-refactor behavior.
func (p *Pipeline) IndexChangedFiles(
	ctx context.Context,
	collectionName string,
	indexRoot string,
	identity services.IndexIdentity,
	provider services.SourceProvider,
	changedFiles []string,
	deletedFiles []string,
) (*services.IndexStats, error) {
	stats := services.NewIndexStats()

	// --- Phase 1: explicit deletes -------------------------------------------------
	// Vector delete failure is the authoritative per-path failure signal
	// (it's the only store where stale chunks would corrupt search). Graph
	// and text-index failures log and continue — matches today's pipeline.
	for _, fp := range deletedFiles {
		if err := p.store.DeleteByFilePath(ctx, collectionName, fp); err != nil {
			p.logger.Warn("vector delete failed", zap.String("file", fp), zap.Error(err))
			stats.FailedDeletes = append(stats.FailedDeletes, fp)
			continue
		}
		if err := p.graph.DeleteByFilePath(ctx, collectionName, fp); err != nil {
			p.logger.Warn("graph delete failed (non-fatal)", zap.String("file", fp), zap.Error(err))
		}
		if err := p.textRepo.DeleteByFilePath(ctx, collectionName, fp); err != nil {
			p.logger.Warn("text delete failed (non-fatal)", zap.String("file", fp), zap.Error(err))
		}
		stats.ProcessedDeletes = append(stats.ProcessedDeletes, fp)
	}

	// --- Phase 2: pre-delete changed files ---------------------------------------
	// We must remove existing chunks for a changed path before upserting new
	// ones, otherwise stale chunks would linger in Qdrant alongside fresh
	// ones. If the vector delete fails we mark the file failed and exclude
	// it from the embed batch — upserting fresh chunks over stale ones would
	// be worse than skipping.
	for _, fp := range changedFiles {
		if err := p.store.DeleteByFilePath(ctx, collectionName, fp); err != nil {
			// Raw error text stays in logs via zap.Error; failed_files value
			// is a stable bare code so consumers can rely on it for routing /
			// grep without parsing.
			p.logger.Warn("vector pre-delete failed", zap.String("file", fp), zap.Error(err))
			stats.FailedFiles[fp] = "qdrant_pre_delete"
			continue
		}
		if err := p.graph.DeleteByFilePath(ctx, collectionName, fp); err != nil {
			p.logger.Warn("graph pre-delete failed (non-fatal)", zap.String("file", fp), zap.Error(err))
		}
		if err := p.textRepo.DeleteByFilePath(ctx, collectionName, fp); err != nil {
			p.logger.Warn("text pre-delete failed (non-fatal)", zap.String("file", fp), zap.Error(err))
		}
	}

	// Filter out files whose pre-delete failed.
	remaining := make([]string, 0, len(changedFiles))
	for _, fp := range changedFiles {
		if _, failed := stats.FailedFiles[fp]; !failed {
			remaining = append(remaining, fp)
		}
	}
	if len(remaining) == 0 {
		// Deletes only (or everything pre-failed) — nothing to parse or embed.
		return stats, nil
	}

	// --- Phase 3: tree-sitter parse ----------------------------------------------
	// Whole-batch parse errors bubble up as retryable. Per-file parse skips
	// are already handled inside the indexer (it drops unparseable files
	// from the elements slice).
	result, err := p.indexer.IndexFiles(ctx, provider, indexRoot, remaining)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", services.ErrTreeSitterParse, err)
	}
	elements := result.Elements
	relationships := result.Relationships

	// --- Phase 4: identity enrichment --------------------------------------------
	// Mirrors pipeline.go:281-287 exactly, including the uint64→string
	// conversion required by CodeElement.WorkspaceID's declared type.
	workspaceStr := fmt.Sprintf("%d", identity.WorkspaceID)
	for i := range elements {
		elements[i].GithubOrgID = identity.GithubOrgID
		elements[i].WorkspaceID = workspaceStr
		elements[i].UserID = identity.UserID
		elements[i].MachineID = identity.MachineID
		elements[i].RepoID = identity.RepoID
	}

	p.logger.Info("Parsed batch",
		zap.Int("files", len(remaining)),
		zap.Int("elements", len(elements)),
		zap.Int("relationships", len(relationships)))

	// --- Phase 5: chunk ------------------------------------------------------------
	chunker := p.newChunker()
	allChunks := chunker.ChunkElements(elements)

	remainingSet := make(map[string]bool, len(remaining))
	for _, fp := range remaining {
		remainingSet[fp] = true
	}
	var chunksToIndex []services.CodeChunk
	for _, c := range allChunks {
		if fp, ok := c.Metadata["file_path"].(string); ok && remainingSet[fp] {
			chunksToIndex = append(chunksToIndex, c)
		}
	}

	// --- Phase 6: batched embed + upsert + text-index ----------------------------
	if len(chunksToIndex) > 0 {
		const batchSize = 1000
		for i := 0; i < len(chunksToIndex); i += batchSize {
			end := i + batchSize
			if end > len(chunksToIndex) {
				end = len(chunksToIndex)
			}
			batch := chunksToIndex[i:end]
			if err := p.embedAndUpsertChunkBatch(ctx, collectionName, batch, stats); err != nil {
				return nil, err
			}
		}
	}

	// --- Phase 7: graph populate (all non-fatal, matches today) ------------------
	filteredElements := make([]services.CodeElement, 0, len(elements))
	for _, el := range elements {
		if _, failed := stats.FailedFiles[el.FilePath]; failed {
			continue
		}
		filteredElements = append(filteredElements, el)
	}
	filteredRelationships := make([]services.CodeRelationship, 0, len(relationships))
	for _, rel := range relationships {
		if _, failed := stats.FailedFiles[rel.FromFile]; failed {
			continue
		}
		if rel.ToFile != "" {
			if _, failed := stats.FailedFiles[rel.ToFile]; failed {
				continue
			}
		}
		filteredRelationships = append(filteredRelationships, rel)
	}
	p.logger.Debug("FalkorDB graph populate starting",
		zap.String("collection", collectionName),
		zap.Int("elements", len(filteredElements)),
		zap.Int("relationships", len(filteredRelationships)),
	)
	if err := p.graph.EnsureIndices(ctx, collectionName); err != nil {
		p.logger.Warn("graph EnsureIndices failed (non-fatal)", zap.String("collection", collectionName), zap.Error(err))
	}
	if err := p.graph.UpsertElements(ctx, collectionName, filteredElements); err != nil {
		p.logger.Warn("graph UpsertElements failed (non-fatal)", zap.String("collection", collectionName), zap.Int("count", len(filteredElements)), zap.Error(err))
	}
	if err := p.graph.UpsertFileNodes(ctx, collectionName, filteredElements); err != nil {
		p.logger.Warn("graph UpsertFileNodes failed (non-fatal)", zap.String("collection", collectionName), zap.Int("count", len(filteredElements)), zap.Error(err))
	}
	if err := p.graph.UpsertRelationships(ctx, collectionName, filteredRelationships, filteredElements); err != nil {
		p.logger.Warn("graph UpsertRelationships failed (non-fatal)", zap.String("collection", collectionName), zap.Int("count", len(filteredRelationships)), zap.Error(err))
	}
	p.logger.Debug("FalkorDB graph populate complete", zap.String("collection", collectionName))

	// --- Phase 8: mark processed files with their hashes -------------------------
	// The streaming Merkle commit path uses these hashes as the authoritative
	// post-commit state; recording an empty-string hash would make the next
	// /diff spuriously re-index the file forever. Treat hash lookup failure
	// as a per-file failure instead — the file will be retried on the next
	// sync rather than committed with a bogus hash. The legacy wrapper
	// ignores these hashes but still benefits from the failure signal via
	// Delta A (skips SaveTree so the file retries on the next Index call).
	for _, fp := range remaining {
		if _, failed := stats.FailedFiles[fp]; failed {
			continue
		}
		hashPath := fp
		if indexRoot != "" && !filepath.IsAbs(fp) && provider.Type() != "memory" {
			hashPath = filepath.Join(indexRoot, fp)
		}
		hash, hErr := provider.GetFileHash(ctx, hashPath)
		if hErr != nil {
			p.logger.Warn("hash lookup failed; marking file failed", zap.String("file", fp), zap.Error(hErr))
			stats.FailedFiles[fp] = "hash_lookup"
			continue
		}
		if hash == "" {
			p.logger.Warn("provider returned empty hash; marking file failed", zap.String("file", fp))
			stats.FailedFiles[fp] = "hash_empty"
			continue
		}
		stats.ProcessedFiles[fp] = hash
	}

	return stats, nil
}

func (p *Pipeline) embedAndUpsertChunkBatch(
	ctx context.Context,
	collectionName string,
	batch []services.CodeChunk,
	stats *services.IndexStats,
) error {
	batch = p.filterFailedChunks(batch, stats)
	if len(batch) == 0 {
		return nil
	}

	texts := make([]string, len(batch))
	for i, c := range batch {
		texts[i] = c.Content
	}

	vectors, err := p.embedder.Embed(ctx, texts)
	if err != nil {
		if services.IsTerminal(err) || services.IsTerminalEmbedError(err) {
			return p.isolateTerminalEmbedFailure(ctx, collectionName, batch, err, stats)
		}
		return fmt.Errorf("%w: %w", services.ErrOpenAIEmbed, err)
	}

	if err := p.store.Upsert(ctx, collectionName, batch, vectors); err != nil {
		return fmt.Errorf("%w: %w", services.ErrQdrantUpsert, err)
	}
	if err := p.textRepo.IndexChunks(ctx, collectionName, batch); err != nil {
		p.logger.Warn("text index batch failed (non-fatal)", zap.Error(err))
	}
	return nil
}

func (p *Pipeline) isolateTerminalEmbedFailure(
	ctx context.Context,
	collectionName string,
	batch []services.CodeChunk,
	embedErr error,
	stats *services.IndexStats,
) error {
	batch = p.filterFailedChunks(batch, stats)
	if len(batch) == 0 {
		return nil
	}

	// Fast path: if the error is uniform across the whole batch (401 auth,
	// 403 permission) every sub-batch would return the same error, so
	// bisect recursion is log2(N)*N embed calls of pointless work. Mark
	// every unique file in the batch failed in one pass and return with
	// the cost of the single embed call that already fired.
	//
	// Critical for outage/credential-misconfiguration scenarios where N is
	// thousands of chunks: without this short-circuit, a global 401 burns
	// through OpenAI quota and saturates the worker rate limit on doomed
	// retries, delaying recovery long after the upstream issue clears.
	if len(batch) > 1 && services.IsGloballyTerminalEmbedError(embedErr) {
		reason := services.TerminalReason(embedErr)
		if reason == "" {
			reason = "openai_embed"
		}
		seen := make(map[string]struct{}, len(batch))
		for _, chunk := range batch {
			fp := chunkFilePath(chunk)
			if fp == "" {
				continue
			}
			if _, dup := seen[fp]; dup {
				continue
			}
			seen[fp] = struct{}{}
			if _, already := stats.FailedFiles[fp]; already {
				continue
			}
			p.markChunkFileFailed(ctx, collectionName, fp, reason, embedErr, stats,
				"globally terminal embed failure; marking file failed without bisect")
		}
		return nil
	}

	if len(batch) == 1 {
		filePath := chunkFilePath(batch[0])
		if filePath == "" {
			return services.Terminal("openai_embed", fmt.Errorf("embed batch: %w", embedErr))
		}
		reason := services.TerminalReason(embedErr)
		if reason == "" {
			reason = "openai_embed"
		}
		p.markChunkFileFailed(ctx, collectionName, filePath, reason, embedErr, stats,
			"isolated terminal embed failure to file; marking file failed")
		return nil
	}

	mid := len(batch) / 2
	if err := p.embedAndUpsertChunkBatch(ctx, collectionName, batch[:mid], stats); err != nil {
		return err
	}
	if err := p.embedAndUpsertChunkBatch(ctx, collectionName, batch[mid:], stats); err != nil {
		return err
	}
	return nil
}

// markChunkFileFailed stamps a file as failed in stats and performs the
// triple cleanup (vector/graph/text). Shared between the bisect single-file
// leaf and the IsGloballyTerminalEmbedError short-circuit. Idempotent — safe
// to call multiple times for the same file path.
func (p *Pipeline) markChunkFileFailed(
	ctx context.Context,
	collectionName, filePath, reason string,
	embedErr error,
	stats *services.IndexStats,
	logMsg string,
) {
	stats.FailedFiles[filePath] = reason
	p.logger.Warn(logMsg,
		zap.String("file", filePath),
		zap.String("reason", reason),
		zap.Error(embedErr))
	if err := p.store.DeleteByFilePath(ctx, collectionName, filePath); err != nil {
		p.logger.Warn("vector cleanup after terminal embed failure failed", zap.String("file", filePath), zap.Error(err))
	}
	if err := p.graph.DeleteByFilePath(ctx, collectionName, filePath); err != nil {
		p.logger.Warn("graph cleanup after terminal embed failure failed", zap.String("file", filePath), zap.Error(err))
	}
	if err := p.textRepo.DeleteByFilePath(ctx, collectionName, filePath); err != nil {
		p.logger.Warn("text cleanup after terminal embed failure failed", zap.String("file", filePath), zap.Error(err))
	}
}

func (p *Pipeline) filterFailedChunks(batch []services.CodeChunk, stats *services.IndexStats) []services.CodeChunk {
	if len(batch) == 0 || len(stats.FailedFiles) == 0 {
		return batch
	}
	filtered := make([]services.CodeChunk, 0, len(batch))
	for _, chunk := range batch {
		filePath := chunkFilePath(chunk)
		if _, failed := stats.FailedFiles[filePath]; failed {
			continue
		}
		filtered = append(filtered, chunk)
	}
	return filtered
}

func chunkFilePath(chunk services.CodeChunk) string {
	if chunk.Metadata == nil {
		return ""
	}
	filePath, _ := chunk.Metadata["file_path"].(string)
	return filePath
}

// newChunker selects the chunker implementation based on config. Extracted
// so both Pipeline.Index (via IndexChangedFiles) and any future callers share
// the same selection logic.
func (p *Pipeline) newChunker() services.ChunkerService {
	if p.indexerCfg.SmartChunkingEnabled() {
		return NewSmartChunker(p.indexerCfg)
	}
	return services.NewChunkerService(p.indexerCfg.ChunkMaxTokens())
}

func collectAllFiles(node *services.MerkleNode, paths *[]string) {
	if node == nil {
		return
	}
	if !node.IsDir {
		*paths = append(*paths, node.Path)
		return
	}
	for _, c := range node.Children {
		collectAllFiles(c, paths)
	}
}

func (p *Pipeline) Review(ctx context.Context, req *dto.ReviewRequest) (*services.ReviewOutput, *services.ReviewMetrics, error) {
	collectionName, repoDBID, err := p.resolveCollectionForReview(ctx, req)
	if err != nil {
		return nil, nil, fmt.Errorf("resolve collection failed: %w", err)
	}

	repoSlug := req.RepoID
	p.logger.Info("Starting review (v2 pipeline)",
		zap.String("repo", repoSlug),
		zap.String("collection", collectionName),
		zap.String("model", p.reviewerCfg.LLMModel()),
		zap.Bool("trivial", req.IsTrivial))

	files := dtoFilesToPRFileInfo(req.Files)
	existingComments := dtoExistingCommentsToService(req.ExistingComments)

	// Detect signature-only changes for risk + prompt — heuristic, single-line decls only.
	contractDiffs := detectContractDiffs(files)

	// Graph context runs unconditionally — queries are cheap and a small change to
	// a widely-called function (the trivial-classified case) is precisely when
	// blast-radius signal matters most.
	dim := uint64(p.openAICfg.EmbeddingDimensions())
	if err := p.store.EnsureCollection(ctx, collectionName, dim); err != nil {
		p.logger.Warn("Failed to ensure collection, graph context may be unavailable", zap.Error(err))
	}
	p.checkCollectionReadiness(ctx, collectionName, repoSlug)

	filter := services.SearchFilter{
		UserID:    req.UserID,
		OrgID:     req.GithubOrgID,
		MachineID: req.MachineID,
	}
	graphCtx := AssembleGraphContext(ctx, p.graph, collectionName, files, contractDiffs, filter, p.logger)

	fileContents := p.fetchFileContents(ctx, req.S3Bucket, req.S3Prefix, files)

	// Concentrated PRs (< 5 changed files) regress when callee bodies dilute attention.
	const calleeBodyMinFiles = 5
	reviewable := 0
	for _, f := range files {
		if len(f.Patch) > 0 {
			reviewable++
		}
	}
	var calleeBodies map[string]string
	if reviewable >= calleeBodyMinFiles {
		calleeBodies = p.fetchCalleeBodies(ctx, req.S3Bucket, req.S3Prefix, graphCtx)
	} else {
		p.logger.Info("Skipping callee bodies for concentrated PR",
			zap.Int("reviewable_files", reviewable))
	}

	repoFilter := services.SearchFilter{RepoID: repoDBID}
	similarCode := p.findSimilarExisting(ctx, collectionName, files, fileContents, repoFilter)

	output, metrics, err := p.reviewChunked(ctx, files, req.Description, graphCtx, existingComments, fileContents, calleeBodies, similarCode)
	if err != nil {
		return nil, nil, err
	}

	// Override LLM-emitted risk with deterministic structural risk, then escalate by findings.
	risk := escalateRiskWithFindings(computeRisk(files, graphCtx, contractDiffs), output.Issues)
	output.RiskLevel = risk.Level
	output.RiskScore = risk.Score
	output.RiskReason = risk.Reason
	output.RiskFactors = risk.Factors

	p.logger.Info("Risk assessed",
		zap.String("level", risk.Level),
		zap.Float64("score", risk.Score),
		zap.Int("contract_diffs", len(contractDiffs)))

	return output, metrics, nil
}

const (
	maxFileContentBytes  = 32_000  // ~8K tokens per file
	maxTotalContentBytes = 120_000 // ~30K tokens total — prevents context overflow
)

// fetchFileContents reads full source from S3 for each changed file.
// Returns empty map if S3 info unavailable. Non-fatal on per-file errors.
func (p *Pipeline) fetchFileContents(ctx context.Context, bucket, prefix string, files []services.PRFileInfo) map[string]string {
	if bucket == "" || prefix == "" {
		return nil
	}

	provider, err := NewS3SourceProviderWithParams(bucket, prefix, "", p.indexerCfg.S3Endpoint(), p.logger)
	if err != nil {
		p.logger.Warn("Failed to create S3 provider for file contents", zap.Error(err))
		return nil
	}
	defer provider.Close()

	contents := make(map[string]string, len(files))
	totalBytes := 0
	for _, f := range files {
		if f.Status == "removed" || f.Patch == "" {
			continue
		}
		if totalBytes >= maxTotalContentBytes {
			p.logger.Debug("Total content cap reached, skipping remaining files",
				zap.Int("total_bytes", totalBytes))
			break
		}
		data, err := provider.ReadFile(ctx, f.Filename)
		if err != nil {
			p.logger.Debug("Failed to read file from S3, skipping",
				zap.String("file", f.Filename), zap.Error(err))
			continue
		}
		if len(data) > maxFileContentBytes {
			p.logger.Debug("File too large, skipping",
				zap.String("file", f.Filename), zap.Int("bytes", len(data)))
			continue
		}
		contents[f.Filename] = string(data)
		totalBytes += len(data)
	}

	if len(contents) > 0 {
		p.logger.Info("Fetched full file contents from S3",
			zap.Int("files", len(contents)),
			zap.Int("total_bytes", totalBytes))
	}
	return contents
}

const (
	maxCalleeBodyLines  = 80
	maxTotalCalleeBytes = 60_000
)

// fetchCalleeBodies reads each callee's source from S3 and slices the line range.
// Skips bodies with missing/zero ranges, excessive size, or read errors.
func (p *Pipeline) fetchCalleeBodies(ctx context.Context, bucket, prefix string, gc *GraphContext) map[string]string {
	if gc == nil || len(gc.CalleeRefs) == 0 || bucket == "" || prefix == "" {
		return nil
	}

	provider, err := NewS3SourceProviderWithParams(bucket, prefix, "", p.indexerCfg.S3Endpoint(), p.logger)
	if err != nil {
		p.logger.Warn("Failed to create S3 provider for callee bodies", zap.Error(err))
		return nil
	}
	defer provider.Close()

	fileCache := make(map[string]string)
	bodies := make(map[string]string)
	totalBytes := 0

	for _, ref := range gc.CalleeRefs {
		if totalBytes >= maxTotalCalleeBytes {
			break
		}
		if ref.StartLine <= 0 || ref.EndLine < ref.StartLine {
			continue
		}
		if ref.EndLine-ref.StartLine+1 > maxCalleeBodyLines {
			continue
		}

		src, ok := fileCache[ref.FilePath]
		if !ok {
			data, err := provider.ReadFile(ctx, ref.FilePath)
			if err != nil {
				p.logger.Debug("callee body fetch failed", zap.String("file", ref.FilePath), zap.Error(err))
				fileCache[ref.FilePath] = ""
				continue
			}
			src = string(data)
			fileCache[ref.FilePath] = src
		}
		if src == "" {
			continue
		}

		body := sliceLines(src, ref.StartLine, ref.EndLine)
		if body == "" {
			continue
		}
		key := ref.FilePath + ":" + ref.Name
		bodies[key] = body
		totalBytes += len(body)
	}

	if len(bodies) > 0 {
		p.logger.Info("Fetched callee bodies",
			zap.Int("callees", len(bodies)),
			zap.Int("total_bytes", totalBytes))
	}
	return bodies
}

// sliceLines returns lines [start, end] (1-indexed, inclusive) joined by \n.
func sliceLines(src string, start, end int) string {
	lines := strings.Split(src, "\n")
	if start < 1 {
		start = 1
	}
	if end > len(lines) {
		end = len(lines)
	}
	if start > end {
		return ""
	}
	return strings.Join(lines[start-1:end], "\n")
}

const similarityThreshold = 0.82 // only surface matches above this score

// findSimilarExisting searches Qdrant for code similar to newly added files.
// Returns a formatted string block to inject into the prompt, or empty if
// nothing found. Query embeddings are batched into a single Embed call.
func (p *Pipeline) findSimilarExisting(ctx context.Context, collection string, files []services.PRFileInfo, fileContents map[string]string, filter services.SearchFilter) string {
	if p.embedder == nil || p.store == nil {
		return ""
	}

	// Only check newly added files — modifications aren't duplication.
	var newFiles []services.PRFileInfo
	for _, f := range files {
		if f.Status == "added" && fileContents[f.Filename] != "" {
			newFiles = append(newFiles, f)
		}
	}
	if len(newFiles) == 0 {
		return ""
	}

	queries := make([]string, len(newFiles))
	for i, f := range newFiles {
		content := fileContents[f.Filename]
		if len(content) > 1500 {
			content = content[:1500]
		}
		queries[i] = content
	}

	vecs, err := p.embedder.Embed(ctx, queries)
	if err != nil || len(vecs) != len(newFiles) {
		p.logger.Debug("batched embed failed — skipping similar-code block",
			zap.Int("files", len(newFiles)),
			zap.Error(err))
		return ""
	}

	var sb strings.Builder
	matchCount := 0

	for i, f := range newFiles {
		results, err := p.store.Search(ctx, collection, vecs[i], 5, filter)
		if err != nil {
			p.logger.Debug("Similarity search failed", zap.String("file", f.Filename), zap.Error(err))
			continue
		}

		// Filter: must be from a different file and above threshold.
		for _, r := range results {
			if r.FilePath == f.Filename || r.Score < similarityThreshold {
				continue
			}
			if matchCount == 0 {
				sb.WriteString("\n\n<similar_existing_code>\n")
				sb.WriteString("The following existing code in the repository is similar to newly added files.\n")
				sb.WriteString("Check if the new code duplicates functionality that already exists.\n\n")
			}
			// Truncate long snippets.
			snippet := r.Content
			if len(snippet) > 800 {
				snippet = snippet[:800] + "\n... (truncated)"
			}
			sb.WriteString(fmt.Sprintf("New file `%s` is similar to existing `%s` (score: %.2f):\n```\n%s\n```\n\n",
				f.Filename, r.FilePath, r.Score, snippet))
			matchCount++
			if matchCount >= 10 {
				break
			}
		}
		if matchCount >= 10 {
			break
		}
	}

	if matchCount > 0 {
		sb.WriteString("</similar_existing_code>")
		p.logger.Info("Found similar existing code",
			zap.Int("matches", matchCount))
	}
	return sb.String()
}

// dtoExistingCommentsToService converts dto.ExistingComment to services.ExistingComment.
func dtoExistingCommentsToService(comments []dto.ExistingComment) []services.ExistingComment {
	if len(comments) == 0 {
		return nil
	}
	result := make([]services.ExistingComment, len(comments))
	for i, c := range comments {
		result[i] = services.ExistingComment{
			ID:         c.ID,
			FilePath:   c.FilePath,
			LineNumber: c.LineNumber,
			Severity:   c.Severity,
			Category:   c.Category,
			Body:       c.Body,
		}
	}
	return result
}

// metricsReviewer wraps a Reviewer and exposes its metrics regardless of concrete type.
type metricsReviewer struct {
	services.Reviewer
	metrics *services.ReviewMetrics
}

// newSinglePassReviewer creates a reviewer for one chunk. When executor has a
// retriever backend the reviewer can call codebase tools across multiple turns
// (capped at maxTurns); otherwise it runs as a single-pass diff-only review.
// Each call returns an independent instance — safe for concurrent use across
// chunk goroutines.
func (p *Pipeline) newSinglePassReviewer(executor *ToolExecutor, maxTurns int) *metricsReviewer {
	if executor == nil {
		executor = NewToolExecutor(nil, nil, "", "", services.SearchFilter{}, p.logger)
	}
	if maxTurns < 1 {
		maxTurns = 1
	}
	var rev services.Reviewer
	switch p.reviewerCfg.LLMProvider() {
	case "openai":
		rev = NewOpenAIReviewer(
			p.openAICfg.APIKey(),
			p.openAICfg.Model(),
			p.openAICfg.MaxTokens(),
			maxTurns,
			executor,
			p.prompts,
			p.logger,
		)
	default:
		rev = NewAnthropicReviewer(
			p.anthropicCfg.APIKey(),
			p.anthropicCfg.Model(),
			p.anthropicCfg.MaxTokens(),
			maxTurns,
			executor,
			p.prompts,
			p.logger,
		)
	}
	// Extract metrics handle from the concrete type.
	var m *services.ReviewMetrics
	switch r := rev.(type) {
	case *AnthropicReviewer:
		m = r.GetMetrics()
	case *OpenAIReviewer:
		m = r.GetMetrics()
	}
	return &metricsReviewer{Reviewer: rev, metrics: m}
}

// chunkedReviewMaxTurns caps per-chunk LLM turns. The system prompt directs
// the model to batch all tool calls into a single response, so the natural
// flow is: turn 1 emits tool_use blocks (or the final review if no tools are
// needed), turn 2 emits the final review after tool results return. Capping
// at 2 prevents runaway cost when a chunk drifts into chained tool calls.
const chunkedReviewMaxTurns = 3

// reviewChunked groups files into chunks and runs a single-shot LLM review per chunk in parallel.
func (p *Pipeline) reviewChunked(
	ctx context.Context,
	files []services.PRFileInfo,
	description string,
	graphCtx *GraphContext,
	existingComments []services.ExistingComment,
	fileContents map[string]string,
	calleeBodies map[string]string,
	similarCode string,
) (*services.ReviewOutput, *services.ReviewMetrics, error) {
	const maxParallelChunks = 3
	const maxTotalChunks = 10
	const perChunkTimeout = 8 * time.Minute

	// Compute chunk limits based on file count to stay within maxTotalChunks.
	// Small PRs: 15K chars, 3 files per chunk (focused).
	// Large PRs: scale up dynamically so we don't exceed maxTotalChunks.
	reviewableFiles := 0
	for _, f := range files {
		if len(f.Patch) > 0 {
			reviewableFiles++
		}
	}
	charsPerChunk := 15000
	filesPerChunk := 3
	if reviewableFiles > maxTotalChunks*filesPerChunk {
		filesPerChunk = (reviewableFiles + maxTotalChunks - 1) / maxTotalChunks
		charsPerChunk = 60000 // relax char limit for large PRs
	}

	// Group files into chunks by patch size AND file count.
	var chunks [][]services.PRFileInfo
	var currentChunk []services.PRFileInfo
	currentSize := 0

	for _, f := range files {
		if len(f.Patch) == 0 {
			continue
		}
		if len(currentChunk) > 0 && (currentSize+len(f.Patch) > charsPerChunk || len(currentChunk) >= filesPerChunk) {
			chunks = append(chunks, currentChunk)
			currentChunk = nil
			currentSize = 0
		}
		currentChunk = append(currentChunk, f)
		currentSize += len(f.Patch)
	}
	if len(currentChunk) > 0 {
		chunks = append(chunks, currentChunk)
	}

	aggregatedMetrics := services.NewReviewMetrics()

	if len(chunks) == 0 {
		aggregatedMetrics.Complete()
		return &services.ReviewOutput{Verdict: "APPROVE", Overview: "No reviewable changes."}, aggregatedMetrics, nil
	}

	// Pre-format graph context string once.
	var graphContextStr string
	if graphCtx != nil {
		graphContextStr = graphCtx.Formatted
	}

	// Build file→comments index for per-chunk scoping.
	commentsByFile := make(map[string][]services.ExistingComment)
	for _, c := range existingComments {
		commentsByFile[c.FilePath] = append(commentsByFile[c.FilePath], c)
	}

	p.logger.Info("Chunked review plan",
		zap.Int("total_files", len(files)),
		zap.Int("chunks", len(chunks)),
		zap.Bool("has_graph_context", graphCtx != nil),
		zap.Int("existing_comments", len(existingComments)))

	type chunkResult struct {
		output  *services.ReviewOutput
		metrics *services.ReviewMetrics
	}
	results := make([]chunkResult, len(chunks))

	g, gCtx := errgroup.WithContext(ctx)
	g.SetLimit(maxParallelChunks)

	for i, chunk := range chunks {
		i, chunk := i, chunk
		g.Go(func() error {
			chunkCtx, cancel := context.WithTimeout(gCtx, perChunkTimeout)
			defer cancel()

			chunkDiff := buildChunkDiff(chunk)

			chunkDesc := description
			if len(chunks) > 1 {
				chunkDesc = fmt.Sprintf("%s\n\n(Reviewing chunk %d/%d: %d files)",
					description, i+1, len(chunks), len(chunk))
			}

			// Scope existing comments to files in this chunk.
			var chunkComments []services.ExistingComment
			for _, f := range chunk {
				chunkComments = append(chunkComments, commentsByFile[f.Filename]...)
			}

			p.logger.Info("Reviewing chunk",
				zap.Int("chunk", i+1),
				zap.Int("of", len(chunks)),
				zap.Int("files", len(chunk)),
				zap.Int("diff_chars", len(chunkDiff)),
				zap.Int("scoped_comments", len(chunkComments)))

			// Single-shot per chunk: pre-computed graph + full file contents win on
			// recall vs the tool path at 3-5x lower wall clock and cost.
			rev := p.newSinglePassReviewer(nil, 1)
			var chunkContents map[string]string
			if len(fileContents) > 0 {
				chunkContents = make(map[string]string)
				for _, f := range chunk {
					if content, ok := fileContents[f.Filename]; ok {
						chunkContents[f.Filename] = content
					}
				}
			}

			chunkOutput, chunkErr := rev.Review(chunkCtx, services.ReviewInput{
				Diff:             chunkDiff,
				Files:            chunk,
				Description:      chunkDesc,
				FileContents:     chunkContents,
				CalleeBodies:     calleeBodies,
				GraphContext:     graphContextStr,
				SimilarCode:      similarCode,
				ExistingComments: chunkComments,
			})
			if chunkErr != nil {
				p.logger.Warn("Chunk review failed, skipping",
					zap.Int("chunk", i+1),
					zap.Error(chunkErr))
				return nil // don't fail the whole review
			}

			p.logger.Info("Chunk review done",
				zap.Int("chunk", i+1),
				zap.Int("issues", len(chunkOutput.Issues)),
				zap.String("verdict", chunkOutput.Verdict))

			results[i] = chunkResult{output: chunkOutput, metrics: rev.metrics}
			return nil
		})
	}

	_ = g.Wait()

	// If every chunk failed, return an error — never silently approve.
	successCount := 0
	for _, r := range results {
		if r.output != nil {
			successCount++
		}
	}
	if successCount == 0 {
		return nil, nil, fmt.Errorf("all %d review chunks failed (timeout or LLM error)", len(chunks))
	}

	// Merge successful chunk results.
	merged := &services.ReviewOutput{
		Verdict:   "APPROVE",
		RiskLevel: "Low",
	}
	var allOverviews []string
	var allRiskReasons []string
	var allAreas []string
	areasDedup := make(map[string]bool)

	for _, r := range results {
		if r.output == nil {
			continue
		}

		merged.Issues = append(merged.Issues, r.output.Issues...)
		merged.ResolvedCommentIDs = append(merged.ResolvedCommentIDs, r.output.ResolvedCommentIDs...)
		merged.Verdict = escalateVerdict(merged.Verdict, r.output.Verdict)
		merged.RiskLevel = escalateRisk(merged.RiskLevel, r.output.RiskLevel)

		if r.output.RiskReason != "" {
			allRiskReasons = append(allRiskReasons, r.output.RiskReason)
		}
		if r.output.Overview != "" {
			allOverviews = append(allOverviews, r.output.Overview)
		}
		if r.output.AreasAffected != "" {
			for _, area := range strings.Split(r.output.AreasAffected, ",") {
				area = strings.TrimSpace(area)
				if area != "" && !areasDedup[area] {
					areasDedup[area] = true
					allAreas = append(allAreas, area)
				}
			}
		}

		if r.metrics != nil {
			aggregatedMetrics.InputTokens += r.metrics.InputTokens
			aggregatedMetrics.OutputTokens += r.metrics.OutputTokens
			aggregatedMetrics.CacheCreationTokens += r.metrics.CacheCreationTokens
			aggregatedMetrics.CacheReadTokens += r.metrics.CacheReadTokens
			aggregatedMetrics.TotalTokens += r.metrics.TotalTokens
		}
	}

	// Deduplicate issues: collapse repeated patterns across files into one issue.
	merged.Issues = deduplicateIssues(merged.Issues)

	merged.AreasAffected = strings.Join(allAreas, ", ")

	// Single-chunk: pass through. Multi-chunk: synthesize, since concat produces walls of
	// text and last-wins bubbles per-chunk claims ("test files only") untrue of the whole PR.
	if len(chunks) == 1 {
		if len(allOverviews) == 1 {
			merged.Overview = allOverviews[0]
		}
		if len(allRiskReasons) == 1 {
			merged.RiskReason = allRiskReasons[0]
		}
	} else {
		ov, rr := p.synthesizeHLD(ctx, files, allOverviews, allRiskReasons, merged.Issues, aggregatedMetrics)
		merged.Overview = ov
		merged.RiskReason = rr
	}

	aggregatedMetrics.Complete()
	p.logger.Info("Chunked review complete",
		zap.Int("chunks_reviewed", len(chunks)),
		zap.Int("total_issues", len(merged.Issues)),
		zap.String("verdict", merged.Verdict),
		zap.Int("resolved_comments", len(merged.ResolvedCommentIDs)),
		zap.Int("total_input_tokens", aggregatedMetrics.InputTokens),
		zap.Int("total_output_tokens", aggregatedMetrics.OutputTokens))

	return merged, aggregatedMetrics, nil
}

func buildChunkDiff(files []services.PRFileInfo) string {
	// Delegates to the numbered formatter so the model sees `[NNNN]` prefixes
	// and can cite line numbers by copying rather than computing from @@.
	return numberedDiffForFiles(files)
}

func escalateVerdict(current, incoming string) string {
	order := map[string]int{"APPROVE": 0, "NEEDS_DISCUSSION": 1, "REQUEST_CHANGES": 2}
	if order[incoming] > order[current] {
		return incoming
	}
	return current
}

func escalateRisk(current, incoming string) string {
	order := map[string]int{"Low": 0, "Medium": 1, "High": 2}
	if order[incoming] > order[current] {
		return incoming
	}
	return current
}

// deduplicateIssues drops only true duplicates: same file, same line, same pattern.
// Distinct call sites pass through — each gets its own inline anchor.
func deduplicateIssues(issues []services.ReviewIssue) []services.ReviewIssue {
	seen := make(map[string]bool, len(issues))
	out := make([]services.ReviewIssue, 0, len(issues))
	for _, iss := range issues {
		key := iss.File + ":" + strconv.Itoa(iss.Line) + ":" + normalizePattern(iss.Pattern)
		if iss.Pattern == "" {
			// No pattern → treat as unique, never collapse on (file, line) alone.
			out = append(out, iss)
			continue
		}
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, iss)
	}
	return out
}

// normalizePattern canonicalizes LLM-assigned pattern tags by lowercasing
// and unifying separators, so variants like "sqlite_thread_safety" and
// "Sqlite Thread Safety" merge to the same key. Word order is preserved:
// "data-loss-on-error" and "error-on-data-loss" describe distinct patterns
// and must not collide.
func normalizePattern(pattern string) string {
	if pattern == "" {
		return ""
	}
	words := strings.FieldsFunc(strings.ToLower(pattern), func(r rune) bool {
		return r == '-' || r == '_' || r == ' '
	})
	return strings.Join(words, "-")
}

// dtoFilesToPRFileInfo converts dto.PRFile slice to services.PRFileInfo slice.
func dtoFilesToPRFileInfo(files []dto.PRFile) []services.PRFileInfo {
	if len(files) == 0 {
		return nil
	}
	result := make([]services.PRFileInfo, len(files))
	for i, f := range files {
		result[i] = services.PRFileInfo{
			Filename:  f.Filename,
			Status:    f.Status,
			Additions: f.Additions,
			Deletions: f.Deletions,
			Patch:     f.Patch,
		}
	}
	return result
}

func (p *Pipeline) Analyze(ctx context.Context, repoPath string, indexReq *dto.IndexRequest) (*services.ReviewOutput, *services.ReviewMetrics, error) {
	collectionName, repoID, err := p.resolveCollection(ctx, indexReq)
	if err != nil {
		return nil, nil, fmt.Errorf("resolve collection failed: %w", err)
	}

	dim := uint64(p.openAICfg.EmbeddingDimensions())

	p.logger.Info("Starting codebase analysis",
		zap.String("repo", repoPath),
		zap.String("collection", collectionName))

	if err := p.store.EnsureCollection(ctx, collectionName, dim); err != nil {
		return nil, nil, fmt.Errorf("failed to ensure collection: %w", err)
	}

	isEmpty, err := p.store.IsEmpty(ctx, collectionName)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to check collection status: %w", err)
	}

	if isEmpty {
		p.logger.Warn("Vector store is empty, auto-indexing repository", zap.String("repo", repoPath))
		if err := p.Index(ctx, indexReq); err != nil {
			return nil, nil, fmt.Errorf("auto-indexing failed: %w", err)
		}
	}

	filter := services.SearchFilter{
		UserID:    indexReq.UserID,
		RepoID:    repoID,
		OrgID:     indexReq.GithubOrgID,
		MachineID: indexReq.MachineID,
	}

	ret := p.buildRetriever(collectionName, repoPath)
	executor := NewToolExecutor(ret, p.graph, repoPath, collectionName, filter, p.logger)
	rev := NewAnthropicReviewer(
		p.anthropicCfg.APIKey(),
		p.anthropicCfg.Model(),
		p.anthropicCfg.MaxTokens(),
		p.reviewerCfg.MaxIterations(),
		executor,
		p.prompts,
		p.logger,
	).(*AnthropicReviewer)

	output, err := rev.Review(ctx, services.ReviewInput{
		Diff:        p.prompts.Audit(),
		Description: "Codebase Audit",
	})
	if err != nil {
		return nil, nil, err
	}
	return output, rev.GetMetrics(), nil
}

func (p *Pipeline) buildRetriever(collectionName, repoPath string) services.RetrieverService {
	// Use multi strategy with local source provider for retrieval.
	provider := NewLocalSourceProvider(p.indexerCfg)
	return NewRetriever("multi", p.embedder, p.store, p.graph, p.textRepo, provider, collectionName, repoPath, 0.7, p.openAICfg, p.logger)
}

