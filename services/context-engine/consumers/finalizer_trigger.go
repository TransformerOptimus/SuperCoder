package consumers

import (
	"context"
	"errors"

	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/TransformerOptimus/SuperCoder/services/context-engine/models/db/postgres"
	"github.com/TransformerOptimus/SuperCoder/services/context-engine/repositories"
)

// FinalizerTrigger is the "maybe-finalize" helper called after every batch
// transition (success or terminal failure). It atomically:
//
//  1. Locks the session row inside a tx.
//  2. Verifies status is still 'processing'.
//  3. Checks whether every expected file/delete has reached a terminal state
//     (processed OR failed).
//  4. Flips status 'processing' → 'finalizing' with a 15-minute TTL.
//  5. Inserts a finalize outbox row (ON CONFLICT DO NOTHING) in the SAME tx.
//
// The in-tx status flip + outbox insert closes the v3.1 finalizer enqueue
// race: we cannot commit a 'finalizing' state without also committing the
// outbox row that will actually run the Merkle CAS commit. After COMMIT a
// best-effort NOTIFY wakes the dispatcher immediately; the dispatcher's
// polling fallback is the correctness mechanism (plan §3.14 v3.1, §8.6 v3.1).
//
// Called from stream_batch_consumer.finishBatch on BOTH the success and
// terminal-failure paths (plan v3.2 §8.5). Concurrent calls for the same
// sync_id all serialize on the row lock; at most one wins the status flip
// because the UPDATE ... WHERE status='processing' predicate matches only
// once. The losers see flipped=false and return cleanly.
type FinalizerTrigger struct {
	db          *gorm.DB
	sessionRepo repositories.SyncSessionRepository
	outboxRepo  repositories.SyncOutboxRepository
	logger      *zap.Logger
}

func NewFinalizerTrigger(
	db *gorm.DB,
	sessionRepo repositories.SyncSessionRepository,
	outboxRepo repositories.SyncOutboxRepository,
	logger *zap.Logger,
) *FinalizerTrigger {
	return &FinalizerTrigger{
		db:          db,
		sessionRepo: sessionRepo,
		outboxRepo:  outboxRepo,
		logger:      logger.Named("finalizer-trigger"),
	}
}

// TryEnqueue runs the maybe-finalize transaction described above. Errors
// during the tx are logged and swallowed — the caller (finishBatch) is on
// the terminal path of a batch and has nothing useful to do with an error.
// If the tx fails, the next worker's finishBatch will retry; if no worker
// retries, the dispatcher's polling fallback and the reaper will eventually
// pick up any orphaned outbox row when /sync-complete or TTL GC fires.
func (t *FinalizerTrigger) TryEnqueue(ctx context.Context, syncID string) {
	err := t.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		sess, err := t.sessionRepo.LoadForUpdate(ctx, tx, syncID)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil
			}
			return err
		}
		if sess.Status != postgres.SyncStatusProcessing {
			// Not ready (still receiving more batches) or already past
			// processing (another worker won, or finalizer already ran).
			return nil
		}

		ready, err := t.sessionRepo.CheckAllBatchesTerminal(ctx, tx, syncID)
		if err != nil {
			return err
		}
		if !ready {
			return nil
		}

		flipped, err := t.sessionRepo.FlipToFinalizing(ctx, tx, syncID)
		if err != nil {
			return err
		}
		if !flipped {
			// Lost the race to another worker — session is already
			// finalizing. The winning tx already inserted the outbox row.
			return nil
		}

		// In-tx outbox insert. ON CONFLICT DO NOTHING is defensive: the
		// unique (sync_id, batch_id='-', task_type='finalize') index
		// already guarantees single-insert, but a pathological replay on
		// a session the GC hasn't cleaned up shouldn't error.
		return t.outboxRepo.InsertIfNotExists(ctx, tx, &postgres.SyncOutbox{
			SyncID:   syncID,
			BatchID:  "-",
			RedisKey: "",
			TaskType: postgres.OutboxTaskFinalize,
			Status:   postgres.OutboxPending,
		})
	})
	if err != nil {
		t.logger.Error("try enqueue finalizer",
			zap.String("sync_id", syncID), zap.Error(err))
		return
	}

	// Post-commit NOTIFY (best-effort). An in-tx NOTIFY would be slightly
	// more conservative, but the dispatcher's 1-second polling fallback is
	// the correctness mechanism — NOTIFY is purely a latency hint (plan
	// §3.14). Matches the precedent in sync_session_service_ingest.go:270.
	if err := t.db.WithContext(ctx).Exec("NOTIFY sync_outbox_new").Error; err != nil {
		t.logger.Warn("notify sync_outbox_new failed (non-fatal)",
			zap.String("sync_id", syncID), zap.Error(err))
	}
}
