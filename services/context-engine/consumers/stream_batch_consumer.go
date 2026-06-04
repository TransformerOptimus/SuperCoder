package consumers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/hibiken/asynq"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/TransformerOptimus/SuperCoder/services/context-engine/repositories"
	"github.com/TransformerOptimus/SuperCoder/services/context-engine/services"
)

// finalizerEnqueuer is the narrow contract StreamBatchConsumer needs from
// the finalizer trigger. *FinalizerTrigger satisfies it structurally; tests
// substitute a fake. Keeping this interface package-private avoids leaking
// implementation details outside `consumers`.
type finalizerEnqueuer interface {
	TryEnqueue(ctx context.Context, syncID string)
}

// asynqRetryInfo extracts the retry counters from an Asynq handler context.
// It is a package-level variable so unit tests can override it — Asynq's
// own GetRetryCount / GetMaxRetry helpers read from unexported context keys
// that tests cannot populate directly. The default implementation calls
// asynq.GetRetryCount / asynq.GetMaxRetry verbatim; both return 0 when the
// context wasn't populated by an Asynq processor.
var asynqRetryInfo = func(ctx context.Context) (retryCount, maxRetry int) {
	rc, _ := asynq.GetRetryCount(ctx)
	mr, _ := asynq.GetMaxRetry(ctx)
	return rc, mr
}

// StreamBatchConsumer is the Asynq handler for services.TaskTypeStreamBatch
// ("index:stream_batch"). One task == one /stream batch. The lifecycle
// invariants are:
//
//  1. Workers MUST NOT flip session.status. The /stream handler is the only
//     path that transitions receiving → processing, atomically with the
//     last batch's manifest merge (plan §3.13 v3.2 / §8.5 v3.2). PR5 moved
//     per-batch state to sync_batches, so MarkBatchSucceeded /
//     MarkBatchFailed are status-agnostic by construction — they key on
//     state='pending' on the batch row, not on the session status.
//
//  2. finishBatch (Redis.Del + FinalizerTrigger.TryEnqueue) MUST run after
//     BOTH the success path AND the terminal-failure path (plan v3.2 §8.5).
//     It MUST NOT run on a retryable non-final failure — the Redis content
//     must survive for the next Asynq retry.
//
//  3. On any terminal failure path, markBatchTerminallyFailed is called
//     BEFORE finishBatch so the failed_files manifest lands before the
//     finalizer might run. The finalizer applies only processed_files to
//     Merkle, so a missed failed_files stamp would leak into "no change
//     needed" and the next /diff would wrongly consider the file current.
//
//  4. Terminal errors (services.IsTerminal) are never retried, regardless
//     of Asynq's retry count. Retryable errors return the error to Asynq
//     on attempts < maxRetry and convert to terminal on the final attempt
//     so the manifest gets stamped before the task is dropped.
type StreamBatchConsumer struct {
	logger      *zap.Logger
	redis       *services.StreamContentRedisClient
	indexer     services.StreamingIndexer
	sessionRepo repositories.SyncSessionRepository
	batchRepo   repositories.SyncBatchRepository
	finalizer   finalizerEnqueuer
}

func NewStreamBatchConsumer(
	logger *zap.Logger,
	redisClient *services.StreamContentRedisClient,
	indexer services.StreamingIndexer,
	sessionRepo repositories.SyncSessionRepository,
	batchRepo repositories.SyncBatchRepository,
	finalizer *FinalizerTrigger,
) *StreamBatchConsumer {
	return &StreamBatchConsumer{
		logger:      logger.Named("consumers.stream_batch"),
		redis:       redisClient,
		indexer:     indexer,
		sessionRepo: sessionRepo,
		batchRepo:   batchRepo,
		finalizer:   finalizer,
	}
}

// Handle implements the Asynq handler signature for TaskTypeStreamBatch.
func (c *StreamBatchConsumer) Handle(ctx context.Context, task *asynq.Task) error {
	var payload services.StreamBatchPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		// Malformed payload. We have no sync_id so there's nothing to
		// mark failed — just log and drop. Returning nil tells Asynq the
		// task is done (no retry).
		c.logger.Error("failed to unmarshal stream_batch payload", zap.Error(err))
		return nil
	}

	// finishBatch runs after both success AND terminal-failure paths. It
	// is NOT called on retryable non-final errors — Redis content must
	// survive for the next retry.
	finishBatch := func() {
		if err := c.redis.Del(ctx, payload.RedisKey).Err(); err != nil {
			c.logger.Warn("redis del (non-fatal)",
				zap.String("sync_id", payload.SyncID),
				zap.String("batch_id", payload.BatchID),
				zap.String("redis_key", payload.RedisKey),
				zap.Error(err))
		}
		c.finalizer.TryEnqueue(ctx, payload.SyncID)
	}

	// ─── Step 1: load batch content from Redis ───
	data, err := c.redis.Get(ctx, payload.RedisKey).Bytes()
	if errors.Is(err, redis.Nil) {
		// Content missing — 2h TTL expired or the key was deleted by a
		// prior attempt. The durable manifest in sync_batches lets us
		// still mark every accepted path as failed (plan v3.3).
		if mbErr := markBatchTerminallyFailed(ctx, c.batchRepo, c.logger,
			payload.SyncID, payload.BatchID, "content_missing"); mbErr != nil {
			c.logger.Error("markBatchTerminallyFailed failed",
				zap.String("sync_id", payload.SyncID), zap.Error(mbErr))
		}
		finishBatch()
		return nil // terminal — do not retry
	}
	if err != nil {
		// Transient Redis error — let Asynq retry.
		return fmt.Errorf("redis get: %w", err)
	}

	// ─── Step 2: decode NDJSON ───
	files, deletes, err := decodeBatchContent(data)
	if err != nil {
		// Malformed content can't be fixed by retrying.
		if mbErr := markBatchTerminallyFailed(ctx, c.batchRepo, c.logger,
			payload.SyncID, payload.BatchID, "decode_error"); mbErr != nil {
			c.logger.Error("markBatchTerminallyFailed failed",
				zap.String("sync_id", payload.SyncID), zap.Error(mbErr))
		}
		finishBatch()
		return nil
	}

	// ─── Step 3: load session ───
	sess, err := c.sessionRepo.Load(ctx, payload.SyncID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// Session was GC'd or deleted. Nothing to do — no manifest
			// worth stamping since the session row is gone (cascade
			// deleted sync_batches and sync_outbox too).
			c.logger.Warn("stream_batch: session not found",
				zap.String("sync_id", payload.SyncID))
			finishBatch()
			return nil
		}
		return fmt.Errorf("load session: %w", err)
	}
	// Only process batches whose session is still in a state where per-file
	// results are meaningful. finalizing/done/failed/expired are all terminal
	// from the worker's perspective.
	if sess.Status != "receiving" && sess.Status != "processing" {
		c.logger.Info("stream_batch: session no longer updatable; dropping",
			zap.String("sync_id", payload.SyncID),
			zap.String("status", string(sess.Status)))
		_ = markBatchTerminallyFailed(ctx, c.batchRepo, c.logger,
			payload.SyncID, payload.BatchID, "session_no_longer_updatable")
		finishBatch()
		return nil
	}

	// ─── Step 4: build provider + identity ───
	provider := services.NewInMemorySourceProvider(files)
	defer func() { _ = provider.Close() }()

	identity := services.IndexIdentity{
		UserID:      sess.UserID,
		WorkspaceID: sess.WorkspaceID,
		MachineID:   sess.MachineID,
		GithubOrgID: sess.GithubOrgID,
		RepoID:      sess.RepoID,
	}
	changedPaths := make([]string, 0, len(files))
	for _, f := range files {
		changedPaths = append(changedPaths, f.Path)
	}

	// ─── Step 5: run the pipeline ───
	// indexRoot="" because InMemorySourceProvider ignores it — see the
	// contract on pipeline.go:298.
	stats, err := c.indexer.IndexChangedFiles(
		ctx,
		sess.CollectionName,
		"",
		identity,
		provider,
		changedPaths,
		deletes,
	)

	// ─── Step 6: classify result ───
	if err != nil {
		if services.IsTerminal(err) {
			reason := services.TerminalReason(err)
			if reason == "" {
				reason = "pipeline_terminal"
			}
			if mbErr := markBatchTerminallyFailed(ctx, c.batchRepo, c.logger,
				payload.SyncID, payload.BatchID, reason); mbErr != nil {
				c.logger.Error("markBatchTerminallyFailed failed",
					zap.String("sync_id", payload.SyncID), zap.Error(mbErr))
			}
			finishBatch()
			return nil // terminal — do not retry
		}

		// Retryable. On the final attempt convert to terminal so the
		// manifest gets stamped; otherwise return err and let Asynq retry.
		//
		// Asynq checks `Retried >= MaxRetry` AFTER the handler returns,
		// using the pre-increment count. GetRetryCount() returns that same
		// value inside the handler, so retryCount == maxRetry IS the last
		// attempt. A task with MaxRetry(3) runs four times — retryCount
		// 0,1,2,3 — and is archived only after the fourth fails.
		//
		// Edge case: if the dispatcher ever registered stream_batch with
		// MaxRetry(0) there's only one attempt. retryCount(0) >= maxRetry(0)
		// is already true, so the guard is redundant — but keep it explicit
		// to make the "single-attempt → final already" intent obvious.
		retryCount, maxRetry := asynqRetryInfo(ctx)
		if maxRetry == 0 || retryCount >= maxRetry {
			reason := classifyExhaustionReason(err)
			// Raw error stays in logs only; failed_files / zap.String("reason")
			// carry a stable code so customer escalations can grep by cause
			// without parsing error text.
			c.logger.Error("stream batch retries exhausted",
				zap.String("sync_id", payload.SyncID),
				zap.String("batch_id", payload.BatchID),
				zap.String("reason", reason),
				zap.Int("retry_count", retryCount),
				zap.Int("max_retry", maxRetry),
				zap.Error(err))
			if mbErr := markBatchTerminallyFailed(ctx, c.batchRepo, c.logger,
				payload.SyncID, payload.BatchID, reason); mbErr != nil {
				c.logger.Error("markBatchTerminallyFailed failed",
					zap.String("sync_id", payload.SyncID), zap.Error(mbErr))
			}
			finishBatch()
			return nil
		}

		// Not the final attempt — let Asynq retry. Do NOT call
		// finishBatch: the Redis content must survive for the next
		// attempt, and the finalizer must not observe a half-finished
		// batch.
		c.logger.Warn("stream_batch retryable failure",
			zap.String("sync_id", payload.SyncID),
			zap.String("batch_id", payload.BatchID),
			zap.Int("retry_count", retryCount),
			zap.Int("max_retry", maxRetry),
			zap.Error(err))
		return fmt.Errorf("index changed files: %w", err)
	}

	// ─── Step 7: success path ───
	if mbErr := markBatchProcessed(ctx, c.batchRepo, c.logger, payload.SyncID, payload.BatchID, stats); mbErr != nil {
		// DB error on the success merge. On non-final attempts this is
		// retryable — Redis content still holds and IndexChangedFiles is
		// idempotent enough on re-run. Do NOT finishBatch here: the next
		// attempt must be able to reload Redis.
		//
		// On the FINAL attempt the story is different. If we just return
		// the error, Asynq drops the task silently: the retry-exhaustion
		// branch at L227-236 runs only when IndexChangedFiles itself
		// errors, not when the post-success DB merge errors. The batch
		// manifest would never be stamped and finishBatch would never
		// fire, wedging the session in 'receiving' until TTL GC. Mirror
		// the retry-exhaustion branch here so the manifest lands.
		retryCount, maxRetry := asynqRetryInfo(ctx)
		if maxRetry == 0 || retryCount >= maxRetry {
			const reason = "db_merge_exhausted"
			c.logger.Error("stream batch db merge exhausted",
				zap.String("sync_id", payload.SyncID),
				zap.String("batch_id", payload.BatchID),
				zap.String("reason", reason),
				zap.Int("retry_count", retryCount),
				zap.Int("max_retry", maxRetry),
				zap.Error(mbErr))
			if tErr := markBatchTerminallyFailed(ctx, c.batchRepo, c.logger,
				payload.SyncID, payload.BatchID, reason); tErr != nil {
				c.logger.Error("markBatchTerminallyFailed failed",
					zap.String("sync_id", payload.SyncID), zap.Error(tErr))
			}
			finishBatch()
			return nil
		}
		return fmt.Errorf("mark batch processed: %w", mbErr)
	}

	finishBatch()
	return nil
}
