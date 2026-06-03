package repositories

import (
	"context"
	"time"

	"gorm.io/gorm"

	"github.com/TransformerOptimus/SuperCoder/services/context-engine/models/db/postgres"
)

// SyncOutboxRepository is the persistence surface for sync_outbox. The
// dispatcher relies on ClaimPending being a single-statement atomic
// transition — splitting it into SELECT + UPDATE would break the
// FOR UPDATE SKIP LOCKED guarantee that partitions work across API
// replicas without leader election (plan §8.4).
type SyncOutboxRepository interface {
	// Insert persists a new outbox row inside the caller-supplied
	// transaction. /stream inserts a stream_batch row alongside the
	// session/batch mutations; the finalizer insert path inserts a
	// finalize row atomically with the status flip to finalizing.
	Insert(ctx context.Context, tx *gorm.DB, row *postgres.SyncOutbox) error

	// InsertIfNotExists persists a new outbox row, suppressing conflicts
	// on the (sync_id, batch_id, task_type) unique index. Used by the
	// WS5 finalizer trigger, where concurrent workers racing to flip the
	// session to finalizing must not produce two finalize outbox rows.
	// Returns nil on success AND on conflict — the caller treats both as
	// "a row exists" (plan §8.6 v3.1).
	InsertIfNotExists(ctx context.Context, tx *gorm.DB, row *postgres.SyncOutbox) error

	// ClaimPending atomically transitions up to `limit` pending rows
	// into enqueuing (with claimed_at = now()) and returns the affected
	// rows. Implemented as a single UPDATE ... WHERE id IN (SELECT ...
	// FOR UPDATE SKIP LOCKED) ... RETURNING statement so concurrent
	// dispatchers partition work without contention.
	ClaimPending(ctx context.Context, limit int) ([]*postgres.SyncOutbox, error)

	// MarkEnqueued transitions rows from enqueuing to enqueued after a
	// successful Asynq enqueue.
	MarkEnqueued(ctx context.Context, ids []int64) error

	// ResetToPending transitions rows back to pending. Used both by the
	// dispatcher on Asynq failure and by the reaper for stuck rows.
	ResetToPending(ctx context.Context, ids []int64) error

	// ReapStuck resets any row stuck in enqueuing for longer than
	// olderThan back to pending so another dispatcher can pick it up.
	// Returns the affected row count.
	ReapStuck(ctx context.Context, olderThan time.Duration) (int64, error)
}
