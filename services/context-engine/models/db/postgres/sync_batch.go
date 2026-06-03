package postgres

import (
	"time"

	"gorm.io/datatypes"
)

// SyncBatchState enumerates the per-batch lifecycle on sync_batches. PR5
// moved per-batch state off sync_sessions (was: O(N) JSONB merge per batch,
// O(N²) total) onto the batch row itself. Each row transitions pending →
// succeeded | failed exactly once; CheckAllBatchesTerminal and the finalizer
// read path both drive off this column (plan PR5 §P1).
type SyncBatchState string

const (
	SyncBatchStatePending   SyncBatchState = "pending"
	SyncBatchStateSucceeded SyncBatchState = "succeeded"
	SyncBatchStateFailed    SyncBatchState = "failed"
)

// SyncBatch is one row per /stream batch. AcceptedFiles and AcceptedDeletes
// form the durable manifest used by markBatchTerminallyFailed to recover the
// set of paths that should be marked failed, even when Redis-held content is
// missing (plan §3.2 v3.3, §6.3 v3.3, §8.5 v3.3).
//
// The (sync_id, batch_id) unique constraint is the authoritative dedup for
// /stream: the transactional insert with ON CONFLICT DO NOTHING RETURNING id
// is the single source of truth for whether a batch has been accepted.
//
// PR5 additions — State/FailedFiles/FailedDeletes/TerminalAt — store the
// per-batch result as a diff against AcceptedFiles/AcceptedDeletes. The
// partition invariant (every accepted path ends up either processed or
// failed, never both — see pipeline.go) lets the finalizer reconstruct
// processed_files = accepted_files \ failed_files at read time, avoiding
// duplication with the accepted manifest.
//
// FailedFiles is populated in two cases:
//   - state='succeeded' when the pipeline's bisect logic isolated per-file
//     failures inside an otherwise-successful batch
//   - state='failed' at terminal time, with every accepted path mapped to
//     the stable failure reason code (content_missing, decode_error,
//     retries_exhausted, pipeline_terminal, db_merge_exhausted, …)
type SyncBatch struct {
	ID              int64          `gorm:"primaryKey;autoIncrement"`
	SyncID          string         `gorm:"type:uuid;not null;uniqueIndex:idx_sync_batches_sync_batch,priority:1;index:idx_sync_batches_sync_id;index:idx_sync_batches_sync_state,priority:1"`
	BatchID         string         `gorm:"type:text;not null;uniqueIndex:idx_sync_batches_sync_batch,priority:2"`
	FileCount       int            `gorm:"not null"`
	ByteCount       int64          `gorm:"not null"`
	AcceptedFiles   datatypes.JSON `gorm:"type:jsonb;not null;default:'{}'"`
	AcceptedDeletes datatypes.JSON `gorm:"type:jsonb;not null;default:'[]'"`
	State           SyncBatchState `gorm:"type:varchar(16);not null;default:'pending';check:state IN ('pending','succeeded','failed');index:idx_sync_batches_sync_state,priority:2"`
	FailedFiles     datatypes.JSON `gorm:"type:jsonb;not null;default:'{}'"`
	FailedDeletes   datatypes.JSON `gorm:"type:jsonb;not null;default:'[]'"`
	TerminalAt      *time.Time
	CreatedAt       time.Time `gorm:"not null;default:CURRENT_TIMESTAMP"`
}

func (SyncBatch) TableName() string { return "sync_batches" }
