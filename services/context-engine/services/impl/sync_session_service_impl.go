package impl

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/TransformerOptimus/SuperCoder/services/context-engine/config"
	"github.com/TransformerOptimus/SuperCoder/services/context-engine/models/db/postgres"
	"github.com/TransformerOptimus/SuperCoder/services/context-engine/repositories"
	"github.com/TransformerOptimus/SuperCoder/services/context-engine/services"
)

// syncSessionServiceImpl is the backing implementation for
// SyncSessionService. WS3 added Diff / GetStatus / RunTTLGCLoop; WS4
// extends it with IngestBatch (the /stream service-layer hot path) and
// the dependencies that path needs: a *gorm.DB handle for the
// transactional outbox, the SyncBatchRepository / SyncOutboxRepository
// for in-tx writes, and a typed StreamContentRedisClient for Redis SETs.
//
// The /diff path deliberately does NOT wrap the per-identity advisory
// lock around repo/shard resolution: those operations are already
// independently race-safe and the lock's purpose is to serialize session
// INSERTs for the same identity.
type syncSessionServiceImpl struct {
	logger      *zap.Logger
	db          *gorm.DB
	openAICfg   config.OpenAIConfig
	merkle      services.MerkleService
	store       repositories.VectorRepository
	textRepo    repositories.TextSearchRepository
	repoRepo    repositories.RepoRepository
	shardRepo   repositories.ShardRepository
	sessionRepo repositories.SyncSessionRepository
	batchRepo   repositories.SyncBatchRepository
	outboxRepo  repositories.SyncOutboxRepository
	streamRedis *services.StreamContentRedisClient
}

// NewSyncSessionService wires the dependencies registered in
// injection/server_container.go. MerkleService is already provided on
// the server container alongside the indexer providers, and WS4 added
// dig providers for the SyncBatch/SyncOutbox repos and the typed
// StreamContentRedisClient.
func NewSyncSessionService(
	logger *zap.Logger,
	db *gorm.DB,
	openAICfg config.OpenAIConfig,
	merkle services.MerkleService,
	store repositories.VectorRepository,
	textRepo repositories.TextSearchRepository,
	repoRepo repositories.RepoRepository,
	shardRepo repositories.ShardRepository,
	sessionRepo repositories.SyncSessionRepository,
	batchRepo repositories.SyncBatchRepository,
	outboxRepo repositories.SyncOutboxRepository,
	streamRedis *services.StreamContentRedisClient,
) services.SyncSessionService {
	return &syncSessionServiceImpl{
		logger:      logger.Named("services.sync_session"),
		db:          db,
		openAICfg:   openAICfg,
		merkle:      merkle,
		store:       store,
		textRepo:    textRepo,
		repoRepo:    repoRepo,
		shardRepo:   shardRepo,
		sessionRepo: sessionRepo,
		batchRepo:   batchRepo,
		outboxRepo:  outboxRepo,
		streamRedis: streamRedis,
	}
}

// Diff executes the /diff preflight:
//
//  1. Resolve (or create) the repo row and its shard assignment.
//  2. Read the Merkle tree via the CAS API and compute need/delete against
//     the client's claimed hashes (full or incremental mode).
//  3. Persist a sync_sessions row with status=receiving and a 10-minute
//     TTL, guarded by a per-identity advisory lock + active-session
//     existence check so overlapping syncs for the same repo are rejected
//     with ErrConcurrentSyncInProgress.
//
// The Merkle key MUST be "repo_<id>" to match the legacy pipeline.go
// convention (pipeline.go:177). Using the collection name instead would
// silently read an empty tree for every repo that was indexed before
// streaming landed.
func (s *syncSessionServiceImpl) Diff(ctx context.Context, p *services.DiffParams) (*services.DiffResult, error) {
	repo, err := s.repoRepo.FindOrCreate(ctx, p.UserID, p.WorkspaceID, p.MachineID, p.RepoPath, "")
	if err != nil {
		return nil, fmt.Errorf("find or create repo: %w", err)
	}

	assignment, err := s.shardRepo.AssignShard(ctx, repo.ID)
	if err != nil {
		return nil, fmt.Errorf("assign shard: %w", err)
	}

	dim := uint64(s.openAICfg.EmbeddingDimensions())
	if err := s.store.EnsureCollection(ctx, assignment.CollectionName, dim); err != nil {
		return nil, fmt.Errorf("ensure collection: %w", err)
	}
	if err := s.store.EnsurePayloadIndexes(ctx, assignment.CollectionName); err != nil {
		s.logger.Warn("ensure payload indexes failed", zap.Error(err), zap.String("collection", assignment.CollectionName))
	}
	if err := s.textRepo.EnsureIndex(ctx, assignment.CollectionName); err != nil {
		s.logger.Warn("ensure text index failed", zap.Error(err), zap.String("collection", assignment.CollectionName))
	}

	merkleKey := fmt.Sprintf("repo_%d", repo.ID)
	tree, version, err := s.merkle.LoadTreeWithVersion(ctx, merkleKey)
	if err != nil {
		return nil, fmt.Errorf("load merkle tree: %w", err)
	}

	var need, deletes []string
	if p.Incremental {
		need = computeIncrementalNeed(tree, p.ClientHashes)
		// Copy ExplicitDeletes so sort doesn't mutate the caller's slice.
		deletes = append([]string{}, p.ExplicitDeletes...)
		sort.Strings(deletes)
	} else {
		need, deletes = computeDiff(tree, p.ClientHashes)
	}
	if need == nil {
		need = []string{}
	}
	if deletes == nil {
		deletes = []string{}
	}

	// expected_files stores only the paths the server will accept at
	// /stream time. Files the server already has must NOT appear — or
	// /stream's `received == expected` commit gate will never trip.
	expectedFiles := make(map[string]string, len(need))
	for _, path := range need {
		expectedFiles[path] = p.ClientHashes[path]
	}

	// No-op sync short-circuit. If the diff yields nothing to upload and
	// nothing to delete, the session is already "complete" at the moment
	// of creation — the `received == expected` commit gate would fire on
	// the first /stream call if the client made one, but the client is
	// entitled to skip /stream entirely when need+delete is empty
	// (streamer.rs does exactly this). Leaving the row in `receiving`
	// would strand it until the 10-minute TTL GC, and any follow-up
	// /diff for the same identity within that window would collide with
	// CreateSessionExclusive's active-session check and return 429.
	//
	// Transition the row straight to `done` inside the same insert so:
	//   - the active-session filter (receiving|processing|finalizing)
	//     immediately stops counting it, freeing the identity for the
	//     next /diff call
	//   - /sync-complete returns 200 with final counts instead of 409
	//     forever
	//   - DeleteTerminalOlderThan reaps it after 1 hour via the
	//     terminal-session GC path, using COALESCE(completed_at, created_at)
	isNoOp := len(need) == 0 && len(deletes) == 0
	now := time.Now()
	initialStatus := postgres.SyncStatusReceiving
	var completedAt *time.Time
	if isNoOp {
		initialStatus = postgres.SyncStatusDone
		t := now
		completedAt = &t
	}

	session := &postgres.SyncSession{
		SyncID:              uuid.NewString(),
		UserID:              p.UserID,
		WorkspaceID:         p.WorkspaceID,
		MachineID:           p.MachineID,
		RepoPath:            p.RepoPath,
		RepoID:              repo.ID,
		CollectionName:      assignment.CollectionName,
		GithubOrgID:         p.GithubOrgID,
		ExpectedFiles:       marshalJSONMapOrEmpty(expectedFiles),
		ExpectedDeletes:     marshalJSONSliceOrEmpty(deletes),
		ReceivedFiles:       emptyJSONObject(),
		ReceivedDeletes:     emptyJSONArray(),
		BatchesSeen:         emptyJSONObject(),
		MerkleVersionAtDiff: version, // "" is legal for first-write repos
		Status:              initialStatus,
		CreatedAt:           now,
		ExpiresAt:           now.Add(10 * time.Minute),
		CompletedAt:         completedAt,
	}

	lockKey := advisoryLockKey(p.UserID, p.WorkspaceID, p.MachineID, p.RepoPath)
	if err := s.sessionRepo.CreateSessionExclusive(ctx, lockKey, session); err != nil {
		if errors.Is(err, repositories.ErrConcurrentSyncInProgress) {
			return nil, err
		}
		return nil, fmt.Errorf("create sync session: %w", err)
	}

	s.logger.Debug("created sync session",
		zap.String("sync_id", session.SyncID),
		zap.String("user_id", p.UserID),
		zap.Uint64("workspace_id", p.WorkspaceID),
		zap.String("machine_id", p.MachineID),
		zap.String("repo_path", p.RepoPath),
		zap.Bool("incremental", p.Incremental),
		zap.Int("need_count", len(need)),
		zap.Int("delete_count", len(deletes)),
		zap.String("status", string(initialStatus)),
	)

	return &services.DiffResult{
		SyncID: session.SyncID,
		Need:   need,
		Delete: deletes,
	}, nil
}

// GetStatus loads the session row and projects its expected/received JSONB
// manifests into length-only counts, then aggregates processed/failed
// counts from sync_batches (PR5 — per-batch state lives there now). A
// missing row returns (nil, nil); the controller maps that to HTTP 410
// sync_not_found. This is a pure read — no locks, no side effects.
//
// Observability note: this path is now O(batches) per call because of the
// LoadTerminalResults scan. Measured acceptable at 47 batches
// (super_sales-sized repo); if a single sync ever produces >500 batches,
// denormalize succeeded_file_count / failed_file_count onto sync_sessions
// as a follow-up. Premature to build now.
func (s *syncSessionServiceImpl) GetStatus(ctx context.Context, syncID string) (*services.StatusSnapshot, error) {
	session, err := s.sessionRepo.Load(ctx, syncID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("load sync session: %w", err)
	}

	terminal, err := s.batchRepo.LoadTerminalResults(ctx, syncID)
	if err != nil {
		return nil, fmt.Errorf("load terminal batch results: %w", err)
	}

	var processedFiles, failedFiles, processedDeletes, failedDeletes int
	for _, row := range terminal {
		// On a 'succeeded' row, the partition is (accepted - failed)
		// processed and |failed| failed. On a 'failed' row,
		// MarkBatchFailed copies every accepted path into failed_files,
		// so |failed|==|accepted| and processed==0. The arithmetic below
		// handles both cases without branching.
		processedFiles += len(row.AcceptedFiles) - len(row.FailedFiles)
		failedFiles += len(row.FailedFiles)
		processedDeletes += len(row.AcceptedDeletes) - len(row.FailedDeletes)
		failedDeletes += len(row.FailedDeletes)
	}

	return &services.StatusSnapshot{
		Status: string(session.Status),
		FileCounts: services.Counts{
			Expected:  jsonObjectLen(session.ExpectedFiles),
			Received:  jsonObjectLen(session.ReceivedFiles),
			Processed: processedFiles,
			Failed:    failedFiles,
		},
		DeleteCounts: services.Counts{
			Expected:  jsonArrayLen(session.ExpectedDeletes),
			Received:  jsonArrayLen(session.ReceivedDeletes),
			Processed: processedDeletes,
			Failed:    failedDeletes,
		},
		FailedReason: session.FailedReason,
	}, nil
}

// RunTTLGCLoop is the session janitor described in plan §6.5 v3.1. It
// runs two sweeps per tick: (1) flip non-terminal sessions past expires_at
// into `expired`; (2) delete terminal sessions older than 1 hour, which
// cascades to sync_batches and sync_outbox.
//
// The loop blocks until ctx is cancelled. WS3 does NOT start this loop —
// WS6 wires it into cmd/server/main.go alongside route registration.
func (s *syncSessionServiceImpl) RunTTLGCLoop(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if n, err := s.sessionRepo.ExpireOldSessions(ctx); err != nil {
				s.logger.Error("TTL expire sweep failed", zap.Error(err))
			} else if n > 0 {
				s.logger.Info("expired stale sync sessions", zap.Int64("count", n))
			}

			if n, err := s.sessionRepo.DeleteTerminalOlderThan(ctx, time.Hour); err != nil {
				s.logger.Error("TTL delete sweep failed", zap.Error(err))
			} else if n > 0 {
				s.logger.Info("deleted old terminal sync sessions", zap.Int64("count", n))
			}
		}
	}
}
