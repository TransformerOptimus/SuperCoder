package consumers

import (
	"context"

	"go.uber.org/zap"

	"github.com/TransformerOptimus/SuperCoder/services/context-engine/repositories"
	"github.com/TransformerOptimus/SuperCoder/services/context-engine/services"
)

// markBatchProcessed is the success-path helper called by
// StreamBatchConsumer.Handle after a successful IndexChangedFiles run. PR5
// moved per-batch state onto sync_batches, so the helper delegates to
// SyncBatchRepository.MarkBatchSucceeded which flips the row to
// state='succeeded', stamps any bisect-isolated per-file failures into
// failed_files/failed_deletes, and refreshes the session expires_at
// keepalive in the same repo-level operation.
//
// A non-applied update is logged and swallowed: the row has moved past
// updatable state (already terminal from a concurrent retry branch, or the
// session was GC'd) and the work should be dropped per plan §8.5 v3.2.
func markBatchProcessed(
	ctx context.Context,
	batchRepo repositories.SyncBatchRepository,
	logger *zap.Logger,
	syncID, batchID string,
	stats *services.IndexStats,
) error {
	failedFiles := stats.FailedFiles
	failedDeletes := stats.FailedDeletes
	applied, err := batchRepo.MarkBatchSucceeded(ctx, syncID, batchID, failedFiles, failedDeletes)
	if err != nil {
		return err
	}
	if !applied {
		logger.Warn("markBatchProcessed: batch not in pending state (already terminal or gone)",
			zap.String("sync_id", syncID),
			zap.String("batch_id", batchID))
	}
	return nil
}

// markBatchTerminallyFailed is the terminal-failure helper. PR5: instead of
// loading the accepted manifest app-side and stamping every path into a
// growing session-row JSONB column, the flip happens in a single
// server-side UPDATE that copies accepted_files keys into failed_files
// mapped to the reason code, and keeps the state mutually exclusive with
// MarkBatchSucceeded via a state='pending' guard.
//
// applied=false means the batch row is either already terminal (a
// concurrent success branch won the race) or the session was cascade-deleted
// by TTL GC. Both are silent-success from the worker's perspective.
func markBatchTerminallyFailed(
	ctx context.Context,
	batchRepo repositories.SyncBatchRepository,
	logger *zap.Logger,
	syncID, batchID, reason string,
) error {
	applied, err := batchRepo.MarkBatchFailed(ctx, syncID, batchID, reason)
	if err != nil {
		return err
	}
	if !applied {
		logger.Warn("markBatchTerminallyFailed: batch not in pending state (already terminal or gone)",
			zap.String("sync_id", syncID),
			zap.String("batch_id", batchID),
			zap.String("reason", reason))
		return nil
	}

	logger.Warn("batch marked terminally failed",
		zap.String("sync_id", syncID),
		zap.String("batch_id", batchID),
		zap.String("reason", reason))
	return nil
}
