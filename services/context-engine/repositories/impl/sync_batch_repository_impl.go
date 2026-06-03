package impl

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"go.uber.org/zap"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/TransformerOptimus/SuperCoder/services/context-engine/models/db/postgres"
	"github.com/TransformerOptimus/SuperCoder/services/context-engine/repositories"
)

type syncBatchRepositoryImpl struct {
	db     *gorm.DB
	logger *zap.Logger
}

func NewSyncBatchRepository(db *gorm.DB, logger *zap.Logger) repositories.SyncBatchRepository {
	return &syncBatchRepositoryImpl{
		db:     db,
		logger: logger.Named("sync-batch-repo"),
	}
}

func (r *syncBatchRepositoryImpl) InsertIfNotExists(ctx context.Context, tx *gorm.DB, batch *postgres.SyncBatch) (int64, bool, error) {
	result := tx.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "sync_id"}, {Name: "batch_id"}},
			DoNothing: true,
		}).
		Create(batch)
	if result.Error != nil {
		return 0, false, fmt.Errorf("insert sync_batch: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		// A row with the same (sync_id, batch_id) already existed. This
		// is the authoritative duplicate signal — callers treat it as a
		// no-op replay.
		return 0, false, nil
	}
	return batch.ID, true, nil
}

func (r *syncBatchRepositoryImpl) LoadManifest(ctx context.Context, syncID, batchID string) (map[string]string, []string, error) {
	var batch postgres.SyncBatch
	err := r.db.WithContext(ctx).
		Select("accepted_files", "accepted_deletes").
		Where("sync_id = ? AND batch_id = ?", syncID, batchID).
		First(&batch).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, nil
		}
		return nil, nil, fmt.Errorf("load sync_batch manifest: %w", err)
	}

	files := map[string]string{}
	if len(batch.AcceptedFiles) > 0 {
		if err := json.Unmarshal(batch.AcceptedFiles, &files); err != nil {
			return nil, nil, fmt.Errorf("decode accepted_files: %w", err)
		}
	}

	var deletes []string
	if len(batch.AcceptedDeletes) > 0 {
		if err := json.Unmarshal(batch.AcceptedDeletes, &deletes); err != nil {
			return nil, nil, fmt.Errorf("decode accepted_deletes: %w", err)
		}
	}

	return files, deletes, nil
}

func (r *syncBatchRepositoryImpl) MarkBatchSucceeded(
	ctx context.Context,
	syncID, batchID string,
	failedFilesInBatch map[string]string,
	failedDeletesInBatch []string,
) (bool, error) {
	// Force nil maps/slices to the empty JSONB literal so the `||` operator
	// and the jsonb type checks in tests never see a "null" value. Mirrors
	// the marshalling pattern used across the streaming repo methods.
	if failedFilesInBatch == nil {
		failedFilesInBatch = map[string]string{}
	}
	if failedDeletesInBatch == nil {
		failedDeletesInBatch = []string{}
	}
	failedFilesJSON, err := json.Marshal(failedFilesInBatch)
	if err != nil {
		return false, fmt.Errorf("marshal failed_files: %w", err)
	}
	failedDeletesJSON, err := json.Marshal(failedDeletesInBatch)
	if err != nil {
		return false, fmt.Errorf("marshal failed_deletes: %w", err)
	}

	// State guard (AND state='pending') is the mutual-exclusion primitive
	// with MarkBatchFailed. Without it, an Asynq retry sequence where an
	// earlier attempt succeeded but crashed before the Ack could be marked
	// terminal by a later attempt's failure branch, overwriting the good
	// result. RowsAffected==0 is silent success (matches S2 pattern).
	//
	// Both the batch flip and the session keepalive run in the same tx so
	// ExpireOldSessions cannot expire the session between the two writes.
	var applied bool
	txErr := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Exec(`
			UPDATE sync_batches
			   SET state          = 'succeeded',
			       failed_files   = ?::jsonb,
			       failed_deletes = ?::jsonb,
			       terminal_at    = now()
			 WHERE sync_id = ?
			   AND batch_id = ?
			   AND state = 'pending'
		`,
			string(failedFilesJSON),
			string(failedDeletesJSON),
			syncID,
			batchID,
		)
		if result.Error != nil {
			return fmt.Errorf("mark batch succeeded: %w", result.Error)
		}
		if result.RowsAffected == 0 {
			return nil
		}
		applied = true
		return tx.Exec(`
			UPDATE sync_sessions
			   SET expires_at = now() + interval '30 minutes'
			 WHERE sync_id = ?
			   AND expires_at < now() + interval '20 minutes'
			   AND status IN ('receiving','processing')
		`, syncID).Error
	})
	if txErr != nil {
		return applied, txErr
	}
	return applied, nil
}

func (r *syncBatchRepositoryImpl) MarkBatchFailed(
	ctx context.Context,
	syncID, batchID, reason string,
) (bool, error) {
	// Copy every accepted_files key into failed_files mapped to `reason` in
	// a single server-side aggregate. No app-side read of accepted_files is
	// needed; the per-batch manifest already lives next to the state flip.
	// failed_deletes is copied verbatim from accepted_deletes — a failed
	// batch fails every path it held, including the deletes.
	//
	// Both the batch flip and the session keepalive run in the same tx so
	// ExpireOldSessions cannot expire the session between the two writes.
	var applied bool
	txErr := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Exec(`
			UPDATE sync_batches
			   SET state = 'failed',
			       failed_files = COALESCE(
			           (SELECT jsonb_object_agg(k, ?::text)
			              FROM jsonb_object_keys(accepted_files) AS k),
			           '{}'::jsonb
			       ),
			       failed_deletes = accepted_deletes,
			       terminal_at    = now()
			 WHERE sync_id = ?
			   AND batch_id = ?
			   AND state = 'pending'
		`,
			reason,
			syncID,
			batchID,
		)
		if result.Error != nil {
			return fmt.Errorf("mark batch failed: %w", result.Error)
		}
		if result.RowsAffected == 0 {
			return nil
		}
		applied = true
		return tx.Exec(`
			UPDATE sync_sessions
			   SET expires_at = now() + interval '30 minutes'
			 WHERE sync_id = ?
			   AND expires_at < now() + interval '20 minutes'
			   AND status IN ('receiving','processing')
		`, syncID).Error
	})
	if txErr != nil {
		return applied, txErr
	}
	return applied, nil
}

func (r *syncBatchRepositoryImpl) LoadTerminalResults(
	ctx context.Context, syncID string,
) ([]repositories.TerminalBatchResult, error) {
	var rows []postgres.SyncBatch
	if err := r.db.WithContext(ctx).
		Select("batch_id", "state", "accepted_files", "accepted_deletes", "failed_files", "failed_deletes").
		Where("sync_id = ? AND state <> ?", syncID, postgres.SyncBatchStatePending).
		Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("load terminal sync_batches: %w", err)
	}

	out := make([]repositories.TerminalBatchResult, 0, len(rows))
	for i := range rows {
		row := &rows[i]
		accepted := map[string]string{}
		if len(row.AcceptedFiles) > 0 {
			if err := json.Unmarshal(row.AcceptedFiles, &accepted); err != nil {
				return nil, fmt.Errorf("decode accepted_files for %s: %w", row.BatchID, err)
			}
		}
		var acceptedDeletes []string
		if len(row.AcceptedDeletes) > 0 {
			if err := json.Unmarshal(row.AcceptedDeletes, &acceptedDeletes); err != nil {
				return nil, fmt.Errorf("decode accepted_deletes for %s: %w", row.BatchID, err)
			}
		}
		failed := map[string]string{}
		if len(row.FailedFiles) > 0 {
			if err := json.Unmarshal(row.FailedFiles, &failed); err != nil {
				return nil, fmt.Errorf("decode failed_files for %s: %w", row.BatchID, err)
			}
		}
		var failedDeletes []string
		if len(row.FailedDeletes) > 0 {
			if err := json.Unmarshal(row.FailedDeletes, &failedDeletes); err != nil {
				return nil, fmt.Errorf("decode failed_deletes for %s: %w", row.BatchID, err)
			}
		}
		out = append(out, repositories.TerminalBatchResult{
			BatchID:         row.BatchID,
			State:           string(row.State),
			AcceptedFiles:   accepted,
			AcceptedDeletes: acceptedDeletes,
			FailedFiles:     failed,
			FailedDeletes:   failedDeletes,
		})
	}
	return out, nil
}
