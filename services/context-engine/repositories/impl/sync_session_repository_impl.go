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

type syncSessionRepositoryImpl struct {
	db     *gorm.DB
	logger *zap.Logger
}

func NewSyncSessionRepository(db *gorm.DB, logger *zap.Logger) repositories.SyncSessionRepository {
	return &syncSessionRepositoryImpl{
		db:     db,
		logger: logger.Named("sync-session-repo"),
	}
}

func (r *syncSessionRepositoryImpl) Insert(ctx context.Context, session *postgres.SyncSession) error {
	if err := r.db.WithContext(ctx).Create(session).Error; err != nil {
		return fmt.Errorf("insert sync_session: %w", err)
	}
	return nil
}

func (r *syncSessionRepositoryImpl) CreateSessionExclusive(
	ctx context.Context, lockKey int64, session *postgres.SyncSession,
) error {
	// Advisory lock + existence check + insert all happen inside a single
	// tx so the lock is held for the whole decision. The lock is released
	// by Postgres on COMMIT/ROLLBACK, not by our code — hence the
	// pg_advisory_xact_lock variant.
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SELECT pg_advisory_xact_lock(?)", lockKey).Error; err != nil {
			return fmt.Errorf("acquire advisory lock: %w", err)
		}

		var count int64
		if err := tx.Model(&postgres.SyncSession{}).
			Where(`user_id = ? AND workspace_id = ? AND machine_id = ? AND repo_path = ? AND status IN ?`,
				session.UserID, session.WorkspaceID, session.MachineID, session.RepoPath,
				[]postgres.SyncSessionStatus{
					postgres.SyncStatusReceiving,
					postgres.SyncStatusProcessing,
					postgres.SyncStatusFinalizing,
				}).
			Count(&count).Error; err != nil {
			return fmt.Errorf("check active sessions: %w", err)
		}
		if count > 0 {
			return repositories.ErrConcurrentSyncInProgress
		}

		if err := tx.Create(session).Error; err != nil {
			return fmt.Errorf("insert sync_session: %w", err)
		}
		return nil
	})
}

func (r *syncSessionRepositoryImpl) Load(ctx context.Context, syncID string) (*postgres.SyncSession, error) {
	var session postgres.SyncSession
	if err := r.db.WithContext(ctx).
		Where("sync_id = ?", syncID).
		First(&session).Error; err != nil {
		return nil, err
	}
	return &session, nil
}

func (r *syncSessionRepositoryImpl) LoadForUpdate(ctx context.Context, tx *gorm.DB, syncID string) (*postgres.SyncSession, error) {
	var session postgres.SyncSession
	if err := tx.WithContext(ctx).
		Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("sync_id = ?", syncID).
		First(&session).Error; err != nil {
		return nil, err
	}
	return &session, nil
}

func (r *syncSessionRepositoryImpl) UpdateStatus(ctx context.Context, syncID string, from, to postgres.SyncSessionStatus) (bool, error) {
	result := r.db.WithContext(ctx).
		Model(&postgres.SyncSession{}).
		Where("sync_id = ? AND status = ?", syncID, from).
		Update("status", to)
	if result.Error != nil {
		return false, fmt.Errorf("update sync_session status %s->%s: %w", from, to, result.Error)
	}
	return result.RowsAffected > 0, nil
}

func (r *syncSessionRepositoryImpl) MarkDone(ctx context.Context, syncID string) error {
	result := r.db.WithContext(ctx).
		Model(&postgres.SyncSession{}).
		Where("sync_id = ? AND status = ?", syncID, postgres.SyncStatusFinalizing).
		Updates(map[string]any{
			"status":       postgres.SyncStatusDone,
			"completed_at": time.Now(),
		})
	if result.Error != nil {
		return fmt.Errorf("mark sync_session done: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		// Idempotent: either the session is already done (Asynq retry
		// of the finalizer after a crash between SaveTreeIfUnchanged and
		// MarkDone — the previous attempt already succeeded), or the row
		// was GC'd. Both cases are acceptable: callers only need the
		// final state to be "done or gone". Log and return nil so the
		// finalizer consumer doesn't see a spurious failure.
		r.logger.Warn("mark_done: no row in finalizing state (already done or gone)",
			zap.String("sync_id", syncID))
		return nil
	}
	return nil
}

func (r *syncSessionRepositoryImpl) MarkFailed(ctx context.Context, syncID string, reason string) error {
	result := r.db.WithContext(ctx).
		Model(&postgres.SyncSession{}).
		Where("sync_id = ? AND status NOT IN ?", syncID, []postgres.SyncSessionStatus{
			postgres.SyncStatusDone,
			postgres.SyncStatusFailed,
			postgres.SyncStatusExpired,
		}).
		Updates(map[string]any{
			"status":        postgres.SyncStatusFailed,
			"failed_reason": reason,
			"completed_at":  time.Now(),
		})
	if result.Error != nil {
		return fmt.Errorf("mark sync_session failed: %w", result.Error)
	}
	return nil
}

func (r *syncSessionRepositoryImpl) ExpireOldSessions(ctx context.Context) (int64, error) {
	result := r.db.WithContext(ctx).
		Model(&postgres.SyncSession{}).
		Where("status IN ? AND expires_at < now()", []postgres.SyncSessionStatus{
			postgres.SyncStatusReceiving,
			postgres.SyncStatusProcessing,
			postgres.SyncStatusFinalizing,
		}).
		Updates(map[string]any{
			"status":        postgres.SyncStatusExpired,
			"failed_reason": "ttl_expired",
			"completed_at":  time.Now(),
		})
	if result.Error != nil {
		return 0, fmt.Errorf("expire old sync_sessions: %w", result.Error)
	}
	return result.RowsAffected, nil
}

func (r *syncSessionRepositoryImpl) FlipToFinalizing(
	ctx context.Context, tx *gorm.DB, syncID string,
) (bool, error) {
	result := tx.WithContext(ctx).Exec(`
		UPDATE sync_sessions
		SET status = 'finalizing',
		    expires_at = now() + interval '15 minutes'
		WHERE sync_id = ?
		  AND status = 'processing'
	`, syncID)
	if result.Error != nil {
		return false, fmt.Errorf("flip to finalizing: %w", result.Error)
	}
	return result.RowsAffected == 1, nil
}

func (r *syncSessionRepositoryImpl) CheckAllBatchesTerminal(
	ctx context.Context, tx *gorm.DB, syncID string,
) (bool, error) {
	// PR5: per-batch state lives on sync_batches.state, so the "are we
	// ready to finalize?" gate is a pure count over the batch rows — no
	// JSONB scanning, no session row lock contention. The gate is:
	//
	//   * at least one batch row exists (prevents finalizing a session
	//     that has no batches at all — the /stream admission path hasn't
	//     even started), AND
	//   * zero rows are still in state='pending'.
	//
	// S12 (>= vs =) dissolves under this shape: the predicate is an
	// equality on count(pending)=0 by construction, and the expected-file
	// coverage check is absorbed by the admission-path validator in
	// sync_session_service_ingest.go:validateBatchEntries (no phantom path
	// can enter a batch row). CheckAllBatchesTerminal is still called
	// inside the tx that holds the session row lock so the caller sees a
	// consistent snapshot.
	var result struct {
		Pending int64
		Total   int64
	}
	err := tx.WithContext(ctx).Raw(`
		SELECT
		    count(*) FILTER (WHERE state = 'pending') AS pending,
		    count(*)                                   AS total
		FROM sync_batches
		WHERE sync_id = ?
	`, syncID).Scan(&result).Error
	if err != nil {
		return false, fmt.Errorf("check all batches terminal: %w", err)
	}
	return result.Total > 0 && result.Pending == 0, nil
}

func (r *syncSessionRepositoryImpl) DeleteTerminalOlderThan(ctx context.Context, age time.Duration) (int64, error) {
	// The cutoff is computed on the DB clock (`now() - interval`) to stay
	// consistent with ExpireOldSessions and ReapStuck. Computing it on the
	// app clock would let pod-vs-DB skew shift the retention window by a
	// few seconds in either direction.
	ageSeconds := age.Seconds()
	result := r.db.WithContext(ctx).
		Where(
			"status IN ? AND COALESCE(completed_at, created_at) < now() - make_interval(secs => ?)",
			[]postgres.SyncSessionStatus{
				postgres.SyncStatusDone,
				postgres.SyncStatusFailed,
				postgres.SyncStatusExpired,
			},
			ageSeconds,
		).
		Delete(&postgres.SyncSession{})
	if result.Error != nil {
		return 0, fmt.Errorf("delete terminal sync_sessions older than %s: %w", age, result.Error)
	}
	return result.RowsAffected, nil
}
