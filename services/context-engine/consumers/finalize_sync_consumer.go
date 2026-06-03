package consumers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/hibiken/asynq"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/TransformerOptimus/SuperCoder/services/context-engine/models/db/postgres"
	"github.com/TransformerOptimus/SuperCoder/services/context-engine/repositories"
	"github.com/TransformerOptimus/SuperCoder/services/context-engine/services"
)

// maxMerkleCASAttempts caps the Merkle compare-and-swap retry loop. Five
// attempts absorbs the realistic worst case under WS5: (a) a post-crash
// Asynq retry that hits one false-conflict ErrVersionMismatch because a
// previous attempt wrote the tree but failed before MarkDone, plus (b) a
// couple of concurrent finalizers racing legitimately. The fast-path hash
// check at the top of the loop short-circuits case (a) when the reloaded
// tree already contains our delta, but the budget bump is defense in
// depth for the case where ApplyChanges' result diverges by some
// implementation detail we didn't anticipate. Still bounded so a
// permanently-wedged tree surfaces as a failed session.
const maxMerkleCASAttempts = 5

// FinalizeSyncConsumer is the Asynq handler for services.TaskTypeFinalize
// ("finalize:sync"). It runs the Merkle compare-and-swap commit loop,
// applying ONLY the processed_files from the session (failed files are
// deliberately left out so the next /diff re-requests them — plan §3.20).
//
// Idempotency: the deterministic task ID (services.FinalizeTaskID) collapses
// Asynq retries to a single task per sync, and the status guard in step 3
// makes redundant deliveries a no-op.
//
// The CAS loop re-reads the tree from Merkle storage on every attempt,
// never caching the merkle_version_at_diff snapshot — plan §3.1 explicitly
// forbids using the stale snapshot as the CAS token.
type FinalizeSyncConsumer struct {
	logger      *zap.Logger
	merkle      services.MerkleService
	sessionRepo repositories.SyncSessionRepository
	batchRepo   repositories.SyncBatchRepository
}

func NewFinalizeSyncConsumer(
	logger *zap.Logger,
	merkle services.MerkleService,
	sessionRepo repositories.SyncSessionRepository,
	batchRepo repositories.SyncBatchRepository,
) *FinalizeSyncConsumer {
	return &FinalizeSyncConsumer{
		logger:      logger.Named("consumers.finalize_sync"),
		merkle:      merkle,
		sessionRepo: sessionRepo,
		batchRepo:   batchRepo,
	}
}

// Handle implements the Asynq handler signature for TaskTypeFinalize.
func (c *FinalizeSyncConsumer) Handle(ctx context.Context, task *asynq.Task) error {
	var payload services.FinalizePayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		c.logger.Error("failed to unmarshal finalize payload", zap.Error(err))
		return nil
	}

	sess, err := c.sessionRepo.Load(ctx, payload.SyncID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// Session GC'd. Nothing to finalize.
			c.logger.Warn("finalize: session not found",
				zap.String("sync_id", payload.SyncID))
			return nil
		}
		return fmt.Errorf("load session: %w", err)
	}

	// Idempotency guards. Deterministic task ID means this handler can be
	// called redundantly on Asynq retries after a crash mid-commit.
	if sess.Status == postgres.SyncStatusDone {
		return nil
	}
	if sess.Status != postgres.SyncStatusFinalizing {
		// Called on a session that isn't ready to finalize. Most likely
		// cause: a stale task that survived after the session was marked
		// failed/expired. Warn and drop.
		c.logger.Warn("finalize called for non-finalizing session",
			zap.String("sync_id", payload.SyncID),
			zap.String("status", string(sess.Status)))
		return nil
	}

	// PR5: per-batch state lives on sync_batches, not sync_sessions. Load
	// every non-pending batch row for this session and reconstruct the
	// processed sets as (accepted \ failed) per row, unioned across rows.
	// The partition invariant (every input path is either processed or
	// failed, never both — see pipeline.go:375,484-502) makes this
	// subtraction lossless. A state='failed' row will have failed_files
	// covering every accepted path (stamped by MarkBatchFailed), so it
	// contributes nothing to the processed set — no special case needed.
	terminal, err := c.batchRepo.LoadTerminalResults(ctx, payload.SyncID)
	if err != nil {
		return fmt.Errorf("load terminal batch results: %w", err)
	}

	processedFiles := map[string]string{}
	processedDeletes := []string{}
	for _, row := range terminal {
		for path, hash := range row.AcceptedFiles {
			if _, failed := row.FailedFiles[path]; failed {
				continue
			}
			processedFiles[path] = hash
		}
		if len(row.AcceptedDeletes) == 0 {
			continue
		}
		failedDelSet := make(map[string]struct{}, len(row.FailedDeletes))
		for _, p := range row.FailedDeletes {
			failedDelSet[p] = struct{}{}
		}
		for _, p := range row.AcceptedDeletes {
			if _, failed := failedDelSet[p]; failed {
				continue
			}
			processedDeletes = append(processedDeletes, p)
		}
	}

	// S11: a session where every file and every delete failed has
	// nothing to apply to Merkle. Running the CAS loop in this case
	// would write a no-op tree (burning an S3 PUT) and then MarkDone —
	// misleading to clients that then see status=done with a
	// failed_files manifest covering the entire request. Stamp it as
	// MarkFailed with a stable reason so /sync-complete surfaces the
	// all-failed outcome unambiguously. The failed_files manifest
	// remains available for inspection.
	if len(processedFiles) == 0 && len(processedDeletes) == 0 {
		c.logger.Warn("finalize: all operations failed; marking session failed",
			zap.String("sync_id", payload.SyncID))
		if err := c.sessionRepo.MarkFailed(ctx, payload.SyncID, "all_operations_failed"); err != nil {
			return fmt.Errorf("mark session failed: %w", err)
		}
		return nil
	}

	// Merkle tree key is repo-scoped, NOT the collection name. See the
	// convention at services/impl/sync_session_service_impl.go:96 and
	// pipeline.go:177. CollectionName is the shard name (e.g. "shard_N")
	// and would store the wrong tree.
	merkleKey := fmt.Sprintf("repo_%d", sess.RepoID)

	// ─── CAS loop ───
	//
	// Exit paths:
	//   - committed=true via break  → fall through to MarkDone
	//   - permanent storage error   → MarkFailed(merkle_storage_permanent), return nil
	//   - other transient error     → return err, let Asynq retry
	//   - loop exhausted             → MarkFailed(merkle_cas_exhausted), return nil
	committed := false
	for attempt := 0; attempt < maxMerkleCASAttempts; attempt++ {
		tree, version, err := c.merkle.LoadTreeWithVersion(ctx, merkleKey)
		if err != nil {
			// S8: permanent storage errors (revoked creds, missing
			// bucket, access denied) cannot be fixed by retrying.
			// Stamp terminal and return nil so Asynq drops the task
			// without burning the retry budget.
			if errors.Is(err, services.ErrStoragePermanent) {
				c.logger.Error("finalize: permanent storage error on load",
					zap.String("sync_id", payload.SyncID),
					zap.Error(err))
				if mErr := c.sessionRepo.MarkFailed(ctx, payload.SyncID, "merkle_storage_permanent"); mErr != nil {
					return fmt.Errorf("mark session failed: %w", mErr)
				}
				return nil
			}
			return fmt.Errorf("load merkle tree: %w", err)
		}

		updated := c.merkle.ApplyChanges(tree, processedFiles, processedDeletes)

		// S5 fast-path: if the reloaded tree already has our delta
		// baked in, a previous Asynq attempt successfully wrote the
		// tree but crashed before MarkDone. Re-applying the same delta
		// to the already-updated tree produces the same hash (SHA
		// overwrites with same value, delete-missing is a no-op), so
		// an equal root hash is the authoritative "delta already
		// committed" signal. Skip the no-op SaveTreeIfUnchanged (which
		// would burn a CAS slot on a false ErrVersionMismatch when the
		// ETag has rotated) and go straight to MarkDone.
		if updated.Hash == tree.Hash {
			c.logger.Info("finalize: delta already reflected in tree; skipping CAS write",
				zap.String("sync_id", payload.SyncID),
				zap.Int("attempt", attempt))
			committed = true
			break
		}

		err = c.merkle.SaveTreeIfUnchanged(ctx, merkleKey, updated, version)
		if errors.Is(err, services.ErrVersionMismatch) {
			c.logger.Info("merkle CAS conflict; retrying",
				zap.String("sync_id", payload.SyncID),
				zap.Int("attempt", attempt))
			continue
		}
		if errors.Is(err, services.ErrStoragePermanent) {
			c.logger.Error("finalize: permanent storage error on save",
				zap.String("sync_id", payload.SyncID),
				zap.Error(err))
			if mErr := c.sessionRepo.MarkFailed(ctx, payload.SyncID, "merkle_storage_permanent"); mErr != nil {
				return fmt.Errorf("mark session failed: %w", mErr)
			}
			return nil
		}
		if err != nil {
			return fmt.Errorf("save merkle tree: %w", err)
		}

		committed = true
		break
	}

	if !committed {
		// CAS exhausted. Mark the session failed so the client sees a
		// deterministic terminal state on the next /sync-complete poll.
		c.logger.Error("merkle CAS exhausted",
			zap.String("sync_id", payload.SyncID),
			zap.Int("attempts", maxMerkleCASAttempts))
		if err := c.sessionRepo.MarkFailed(ctx, payload.SyncID, "merkle_cas_exhausted"); err != nil {
			return fmt.Errorf("mark session failed: %w", err)
		}
		return nil
	}

	// Success. Flip session to done. MarkDone is idempotent
	// (WHERE status='finalizing'); a concurrent finalizer that already
	// flipped the row is silent-success.
	if err := c.sessionRepo.MarkDone(ctx, payload.SyncID); err != nil {
		return fmt.Errorf("mark session done: %w", err)
	}
	c.logger.Info("sync finalized",
		zap.String("sync_id", payload.SyncID),
		zap.Int("processed_files", len(processedFiles)),
		zap.Int("processed_deletes", len(processedDeletes)))
	return nil
}

// Compile-time check that *FinalizeSyncConsumer has the Asynq handler
// signature. Keeps the consumer wiring in injection/worker_container.go
// honest if the signature ever changes.
var _ asynq.HandlerFunc = (&FinalizeSyncConsumer{}).Handle
