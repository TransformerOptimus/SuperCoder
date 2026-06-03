package impl

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/TransformerOptimus/SuperCoder/services/context-engine/models/db/postgres"
	"github.com/TransformerOptimus/SuperCoder/services/context-engine/repositories"
)

type syncOutboxRepositoryImpl struct {
	db     *gorm.DB
	logger *zap.Logger
}

func NewSyncOutboxRepository(db *gorm.DB, logger *zap.Logger) repositories.SyncOutboxRepository {
	return &syncOutboxRepositoryImpl{
		db:     db,
		logger: logger.Named("sync-outbox-repo"),
	}
}

func (r *syncOutboxRepositoryImpl) Insert(ctx context.Context, tx *gorm.DB, row *postgres.SyncOutbox) error {
	if err := tx.WithContext(ctx).Create(row).Error; err != nil {
		return fmt.Errorf("insert sync_outbox: %w", err)
	}
	return nil
}

func (r *syncOutboxRepositoryImpl) InsertIfNotExists(ctx context.Context, tx *gorm.DB, row *postgres.SyncOutbox) error {
	// ON CONFLICT DO NOTHING on the (sync_id, batch_id, task_type) unique
	// index. Concurrent finalizer triggers produce the same row; the
	// caller treats "already exists" as success because another worker
	// already enqueued the finalize task.
	if err := tx.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns: []clause.Column{
				{Name: "sync_id"},
				{Name: "batch_id"},
				{Name: "task_type"},
			},
			DoNothing: true,
		}).
		Create(row).Error; err != nil {
		return fmt.Errorf("insert sync_outbox (if not exists): %w", err)
	}
	return nil
}

// claimPendingSQL atomically promotes up to :limit pending outbox rows to
// enqueuing and returns them. The single-statement CTE is load-bearing: the
// UPDATE would hold row locks on unrelated pending rows if split into a
// SELECT + UPDATE, which defeats FOR UPDATE SKIP LOCKED and prevents
// multiple dispatchers from partitioning work (plan §8.4).
const claimPendingSQL = `
WITH claimed AS (
  SELECT id
  FROM sync_outbox
  WHERE status = 'pending'
  ORDER BY id
  LIMIT ?
  FOR UPDATE SKIP LOCKED
)
UPDATE sync_outbox o
SET status = 'enqueuing', claimed_at = now()
FROM claimed
WHERE o.id = claimed.id
RETURNING o.id, o.sync_id, o.batch_id, o.redis_key, o.task_type, o.status, o.claimed_at, o.created_at, o.enqueued_at
`

func (r *syncOutboxRepositoryImpl) ClaimPending(ctx context.Context, limit int) ([]*postgres.SyncOutbox, error) {
	var rows []*postgres.SyncOutbox
	if err := r.db.WithContext(ctx).
		Raw(claimPendingSQL, limit).
		Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("claim pending outbox rows: %w", err)
	}
	return rows, nil
}

// MarkEnqueued and ResetToPending both add `AND status = 'enqueuing'` to
// their WHERE clause so a concurrent reaper (or another dispatcher's
// delayed call) cannot stomp a row that has since been reclaimed — without
// the guard, a stale MarkEnqueued could promote a pending row that has no
// in-flight Asynq task, or a stale ResetToPending could reset a row that
// another worker just claimed. RowsAffected == 0 is silent success: it
// means someone else already transitioned the row, so there is nothing for
// us to do. ReapStuck uses the same pattern.
func (r *syncOutboxRepositoryImpl) MarkEnqueued(ctx context.Context, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	result := r.db.WithContext(ctx).
		Model(&postgres.SyncOutbox{}).
		Where("id IN ? AND status = ?", ids, postgres.OutboxEnqueuing).
		Updates(map[string]any{
			"status":      postgres.OutboxEnqueued,
			"enqueued_at": time.Now(),
		})
	if result.Error != nil {
		return fmt.Errorf("mark outbox enqueued: %w", result.Error)
	}
	return nil
}

func (r *syncOutboxRepositoryImpl) ResetToPending(ctx context.Context, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	result := r.db.WithContext(ctx).
		Model(&postgres.SyncOutbox{}).
		Where("id IN ? AND status = ?", ids, postgres.OutboxEnqueuing).
		Updates(map[string]any{
			"status":     postgres.OutboxPending,
			"claimed_at": gorm.Expr("NULL"),
		})
	if result.Error != nil {
		return fmt.Errorf("reset outbox to pending: %w", result.Error)
	}
	return nil
}

func (r *syncOutboxRepositoryImpl) ReapStuck(ctx context.Context, olderThan time.Duration) (int64, error) {
	cutoff := time.Now().Add(-olderThan)
	result := r.db.WithContext(ctx).
		Model(&postgres.SyncOutbox{}).
		Where("status = ? AND claimed_at < ?", postgres.OutboxEnqueuing, cutoff).
		Updates(map[string]any{
			"status":     postgres.OutboxPending,
			"claimed_at": gorm.Expr("NULL"),
		})
	if result.Error != nil {
		return 0, fmt.Errorf("reap stuck outbox rows: %w", result.Error)
	}
	return result.RowsAffected, nil
}
