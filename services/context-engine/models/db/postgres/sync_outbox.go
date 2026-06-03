package postgres

import "time"

// OutboxTaskType discriminates the kind of Asynq task a row represents.
// A stream_batch row enqueues a per-batch indexing task; a finalize row
// enqueues the session finalizer. Both task types share the same outbox
// table so /stream can insert either kind inside the same transaction that
// mutates sync_sessions (plan §3.14 v3.1, §8.6 v3.1).
type OutboxTaskType string

const (
	OutboxTaskStreamBatch OutboxTaskType = "stream_batch"
	OutboxTaskFinalize    OutboxTaskType = "finalize"
)

// OutboxStatus tracks the three-state dispatcher lifecycle. A row is inserted
// as pending, atomically claimed into enqueuing (with claimed_at set), and
// finally marked enqueued once the Asynq enqueue succeeds. The reaper resets
// rows stuck in enqueuing back to pending after a timeout (plan §3.14 v3.1,
// §8.4).
type OutboxStatus string

const (
	OutboxPending   OutboxStatus = "pending"
	OutboxEnqueuing OutboxStatus = "enqueuing"
	OutboxEnqueued  OutboxStatus = "enqueued"
)

// SyncOutbox is the transactional outbox row that backs the dispatcher. The
// (sync_id, batch_id, task_type) unique constraint lets a stream_batch row
// and a finalize row coexist for the same sync: the stream_batch row carries
// the real batch_id; the finalize row uses "-" as a sentinel.
type SyncOutbox struct {
	ID         int64          `gorm:"primaryKey;autoIncrement"`
	SyncID     string         `gorm:"type:uuid;not null;uniqueIndex:idx_sync_outbox_sync_batch_task,priority:1"`
	BatchID    string         `gorm:"type:text;not null;uniqueIndex:idx_sync_outbox_sync_batch_task,priority:2"`
	RedisKey   string         `gorm:"type:text;not null"`
	TaskType   OutboxTaskType `gorm:"type:varchar(20);not null;uniqueIndex:idx_sync_outbox_sync_batch_task,priority:3;check:task_type IN ('stream_batch','finalize')"`
	Status     OutboxStatus   `gorm:"type:varchar(20);not null;check:status IN ('pending','enqueuing','enqueued')"`
	ClaimedAt  *time.Time
	CreatedAt  time.Time `gorm:"not null;default:CURRENT_TIMESTAMP"`
	EnqueuedAt *time.Time
}

func (SyncOutbox) TableName() string { return "sync_outbox" }
