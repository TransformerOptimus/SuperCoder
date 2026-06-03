package repositories

import (
	"context"
	"time"

	"gorm.io/gorm"

	"github.com/TransformerOptimus/SuperCoder/services/context-engine/models/db/postgres"
)

// SyncSessionRepository is the persistence surface for sync_sessions. It
// covers the atomic state transitions the plan's state machine relies on
// (plan §8.5 v3.2) and the TTL GC primitives used by WS3.
type SyncSessionRepository interface {
	// Insert persists a freshly-created session. Fails if sync_id already
	// exists. Callers are expected to generate the UUID server-side at
	// /diff time.
	Insert(ctx context.Context, session *postgres.SyncSession) error

	// CreateSessionExclusive serializes /diff calls for the same identity
	// and refuses to create a second active session while one already
	// exists. Inside a single tx it:
	//
	//   1. Takes pg_advisory_xact_lock(lockKey) — serializes concurrent
	//      callers with the same identity hash.
	//   2. Checks for any existing row matching (user_id, workspace_id,
	//      machine_id, repo_path) whose status is in
	//      (receiving, processing, finalizing). If found, returns
	//      ErrConcurrentSyncInProgress.
	//   3. INSERTs the provided session.
	//
	// The advisory lock is released by Postgres on COMMIT/ROLLBACK. Callers
	// whose ctx deadline fires while waiting on the lock receive the
	// wrapped ctx error. See plan §3.6 / §8.5.
	CreateSessionExclusive(ctx context.Context, lockKey int64, session *postgres.SyncSession) error

	// Load returns the session by id, or gorm.ErrRecordNotFound.
	Load(ctx context.Context, syncID string) (*postgres.SyncSession, error)

	// LoadForUpdate acquires a row-level lock on the session inside an
	// existing transaction. Use from /stream to serialize batch admission.
	LoadForUpdate(ctx context.Context, tx *gorm.DB, syncID string) (*postgres.SyncSession, error)

	// UpdateStatus performs the atomic state transition
	// (status = from) -> (status = to). Returns true iff exactly one row
	// was updated. Returns false (no error) if the session was in any
	// other state — callers treat this as "somebody else moved it".
	UpdateStatus(ctx context.Context, syncID string, from, to postgres.SyncSessionStatus) (bool, error)

	// MarkDone is the happy-path terminal transition from finalizing.
	MarkDone(ctx context.Context, syncID string) error

	// MarkFailed is the unhappy-path terminal transition. Records the
	// reason on the session row. Safe to call from any non-terminal state.
	MarkFailed(ctx context.Context, syncID string, reason string) error

	// ExpireOldSessions flips any non-terminal session whose expires_at
	// has passed into the expired state. Returns the affected row count.
	// Used by the WS3 TTL GC loop.
	ExpireOldSessions(ctx context.Context) (int64, error)

	// DeleteTerminalOlderThan deletes terminal sessions whose
	// completed_at (falling back to created_at) is older than age.
	// Cascades clean up sync_batches and sync_outbox. Returns the
	// affected row count.
	DeleteTerminalOlderThan(ctx context.Context, age time.Duration) (int64, error)

	// FlipToFinalizing atomically transitions status from 'processing' to
	// 'finalizing' and sets expires_at to now() + 15 minutes. Must be
	// called inside a transaction that already holds the session row
	// lock so the caller can insert a finalize outbox row in the same tx
	// (plan §3.14 v3.1, §8.6 v3.1). Returns flipped=true iff RowsAffected==1.
	FlipToFinalizing(ctx context.Context, tx *gorm.DB, syncID string) (flipped bool, err error)

	// CheckAllBatchesTerminal reports whether every sync_batches row for
	// the given session has transitioned out of state='pending'. Must be
	// called inside a transaction that holds the session row lock so the
	// caller sees a consistent snapshot.
	//
	// PR5: the check is a pure count query over sync_batches.state. It
	// returns true iff at least one batch row exists AND zero rows are
	// still pending. The expected-file coverage check is absorbed by the
	// admission path (validateBatchEntries rejects any path outside
	// expected_files), so we don't need a second consistency check here.
	CheckAllBatchesTerminal(ctx context.Context, tx *gorm.DB, syncID string) (ready bool, err error)
}
