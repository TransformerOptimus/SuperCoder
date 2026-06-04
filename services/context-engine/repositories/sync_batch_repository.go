package repositories

import (
	"context"

	"gorm.io/gorm"

	"github.com/TransformerOptimus/SuperCoder/services/context-engine/models/db/postgres"
)

// TerminalBatchResult is the per-row payload returned by LoadTerminalResults.
// The finalizer derives processed_files = accepted_files \ failed_files and
// processed_deletes = accepted_deletes \ failed_deletes at read time; the
// partition invariant (every input path ends up in exactly one of processed
// or failed — see pipeline.go) makes the subtraction lossless.
//
// State is one of 'succeeded' or 'failed'. On 'failed' rows, failed_files
// covers every accepted path mapped to the terminal reason code, so the
// subtraction yields an empty processed set.
type TerminalBatchResult struct {
	BatchID         string
	State           string
	AcceptedFiles   map[string]string
	AcceptedDeletes []string
	FailedFiles     map[string]string
	FailedDeletes   []string
}

// SyncBatchRepository is the persistence surface for sync_batches. /stream
// uses InsertIfNotExists as the authoritative dedup check (the in-transaction
// unique constraint is the single source of truth); markBatchTerminallyFailed
// uses LoadManifest to recover the accepted-paths manifest when Redis
// content is gone (plan §5.2 v3.1, §8.5 v3.3).
//
// PR5: per-batch state (succeeded/failed + failed_files/deletes diff) lives
// on the batch row instead of sync_sessions. MarkBatchSucceeded and
// MarkBatchFailed are the only writers of that state, mutually exclusive via
// a "state='pending'" guard in the WHERE clause (no explicit row lock needed
// — preserves WS5 invariant #6). LoadTerminalResults is the finalizer and
// GetStatus read path.
type SyncBatchRepository interface {
	// InsertIfNotExists attempts to persist a new batch row inside the
	// caller-supplied transaction. Returns (id, true, nil) if the row was
	// newly inserted, (0, false, nil) if a row with the same
	// (sync_id, batch_id) already existed. Any error other than the
	// unique-constraint conflict is returned verbatim.
	InsertIfNotExists(ctx context.Context, tx *gorm.DB, batch *postgres.SyncBatch) (id int64, inserted bool, err error)

	// LoadManifest returns the durable accepted-paths manifest for a
	// batch. Returns (nil, nil, nil) if no batch row exists for the
	// given (sync_id, batch_id). Callers use this to drive
	// markBatchTerminallyFailed on any path that doesn't depend on Redis.
	LoadManifest(ctx context.Context, syncID, batchID string) (acceptedFiles map[string]string, acceptedDeletes []string, err error)

	// MarkBatchSucceeded flips a pending batch row to state='succeeded'
	// and stamps the per-file bisect failures (if any) into failed_files
	// / failed_deletes. The happy path (no per-file failures) writes empty
	// JSONB literals. Also refreshes the session expires_at keepalive
	// (conditional: only when expires_at < now()+20m), tying the keepalive
	// to actual progress without hammering the session row.
	//
	// Returns applied=false (no error) when RowsAffected==0: the row is
	// either already terminal (mutual exclusion with MarkBatchFailed — the
	// other terminal branch won the race) or the session was GC'd. Both
	// are silent-success; the worker should drop the work. Mirrors the S2
	// silent-skip pattern used by MarkEnqueued.
	MarkBatchSucceeded(
		ctx context.Context,
		syncID, batchID string,
		failedFilesInBatch map[string]string,
		failedDeletesInBatch []string,
	) (applied bool, err error)

	// MarkBatchFailed flips a pending batch row to state='failed' and
	// copies every accepted path into failed_files mapped to the given
	// terminal reason code (content_missing, decode_error,
	// retries_exhausted, pipeline_terminal, db_merge_exhausted, ...). The
	// copy happens in a single UPDATE (jsonb_object_agg over
	// accepted_files keys), so no app-side read of accepted_files is
	// needed. Also refreshes the session expires_at keepalive.
	//
	// Same applied=false / state='pending' guard semantics as
	// MarkBatchSucceeded.
	MarkBatchFailed(
		ctx context.Context,
		syncID, batchID, reason string,
	) (applied bool, err error)

	// LoadTerminalResults returns every non-pending batch row for the
	// given session. Used by the finalizer to derive the processed/failed
	// union across batches, and by GetStatus to compute aggregate counts
	// for /sync-complete polls.
	//
	// Results are not ordered — callers must not assume any particular
	// sequence. Empty JSONB values are returned as empty Go maps / slices
	// so callers can range over them unconditionally.
	LoadTerminalResults(ctx context.Context, syncID string) ([]TerminalBatchResult, error)
}
