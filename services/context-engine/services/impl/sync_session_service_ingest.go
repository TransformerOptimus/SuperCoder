package impl

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"go.uber.org/zap"
	"gorm.io/datatypes"
	"gorm.io/gorm"

	"github.com/TransformerOptimus/SuperCoder/services/context-engine/models/db/postgres"
	"github.com/TransformerOptimus/SuperCoder/services/context-engine/models/dto"
	"github.com/TransformerOptimus/SuperCoder/services/context-engine/services"
)

// redisContentTTL is the 2-hour expiry for /stream content keys
// (plan §3.5, §3.13 v3.2). Workers in WS5 must finish processing a
// batch within this window or the content is gone and the batch will
// be marked terminally failed via the durable manifest in
// sync_batches.accepted_files (plan §8.5 v3.3).
const redisContentTTL = 2 * time.Hour

// IngestBatch is the service-layer entry point for POST /stream. It
// implements the transactional outbox pattern from plan §5.2 v3.3:
//
//  1. Pre-tx fast-path dedup (racy optimization).
//  2. All-or-nothing entry validation.
//  3. Server recomputes and verifies the batch_id from the request body.
//  4. Build accepted manifests + Redis payload.
//  5. Redis SET (idempotent on the deterministic key).
//  6. Postgres tx — lock the session, INSERT sync_batches with the
//     durable manifest (authoritative dedup via the unique index),
//     merge accepted_* into received_*, INSERT sync_outbox row, flip
//     receiving→processing IFF this batch closes the session.
//  7. Post-commit NOTIFY (best-effort latency hint; the dispatcher's
//     1-second polling fallback is the correctness mechanism).
//
// The function returns:
//   - (&IngestBatchResult{Duplicate: true}, nil) when the batch was
//     already accepted (either via the fast path or the in-tx unique
//     constraint). Maps to 200 {duplicate: true}.
//   - (&IngestBatchResult{Queued: …}, nil) on a fresh accept. Maps to
//     202 with the queued counts.
//   - (nil, *IngestBatchError) on validation failure. Maps to 400 with
//     the reason code.
//   - (nil, ErrSyncNotFound|ErrSyncNotReceiving) when the session is
//     gone or no longer accepting. Maps to 410 sync_expired.
//   - (nil, other) on internal failures. Maps to 500.
func (s *syncSessionServiceImpl) IngestBatch(
	ctx context.Context, p *services.IngestBatchParams,
) (*services.IngestBatchResult, error) {
	// ─────────── Step 1: pre-tx fast-path dedup ───────────
	// Racy by design — two concurrent identical batches can both pass
	// this check. Step 6b's INSERT ... ON CONFLICT DO NOTHING is the
	// authoritative dedup. This fast path exists only to short-circuit
	// the common "client retries 5 seconds after success" case without
	// opening a transaction or touching Redis.
	session, err := s.sessionRepo.Load(ctx, p.SyncID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, services.ErrSyncNotFound
		}
		return nil, fmt.Errorf("load session: %w", err)
	}
	if session == nil {
		return nil, services.ErrSyncNotFound
	}
	if session.Status != postgres.SyncStatusReceiving {
		return nil, services.ErrSyncNotReceiving
	}

	batchesSeen, err := decodeBatchesSeen(session.BatchesSeen)
	if err != nil {
		return nil, fmt.Errorf("decode batches_seen: %w", err)
	}
	if batchesSeen[p.BatchID] {
		return &services.IngestBatchResult{Duplicate: true}, nil
	}

	// ─────────── Step 2: all-or-nothing validation ───────────
	expectedFiles, err := decodeExpectedFiles(session.ExpectedFiles)
	if err != nil {
		return nil, fmt.Errorf("decode expected_files: %w", err)
	}
	expectedDeleteSet, err := decodeExpectedDeleteSet(session.ExpectedDeletes)
	if err != nil {
		return nil, fmt.Errorf("decode expected_deletes: %w", err)
	}
	// Cross-batch delete dedup is checked inside the transaction after
	// LoadForUpdate (see checkCrossBatchDeleteDedup) — not here — to
	// prevent a race where two concurrent requests both read the same
	// pre-tx snapshot and both pass the check.
	if vErr := validateBatchEntries(p.Entries, expectedFiles, expectedDeleteSet, nil); vErr != nil {
		return nil, vErr
	}

	// ─────────── Step 3: server-recompute batch_id ───────────
	serverBatchID := computeBatchID(p.Entries)
	if serverBatchID != p.BatchID {
		return nil, &services.IngestBatchError{
			Reason:  "batch_id_mismatch",
			Details: fmt.Sprintf("header=%s server=%s", p.BatchID, serverBatchID),
		}
	}

	// ─────────── Step 4: build manifests + Redis payload ───────────
	acceptedFiles := make(map[string]string, len(p.Entries))
	acceptedDeletes := make([]string, 0, len(p.Entries))
	var totalBytes int64
	for _, e := range p.Entries {
		switch e.Op {
		case "file":
			acceptedFiles[e.Path] = e.Hash
			totalBytes += int64(len(e.Content))
		case "delete":
			acceptedDeletes = append(acceptedDeletes, e.Path)
		}
	}
	sort.Strings(acceptedDeletes)

	payload, err := serializeBatchForRedis(p.Entries)
	if err != nil {
		return nil, fmt.Errorf("serialize redis payload: %w", err)
	}

	// ─────────── Step 5: Redis SET (idempotent on the fixed key) ───────────
	// We SET BEFORE the Postgres tx so a successful commit always implies
	// content is reachable to workers. If the tx rolls back, the Redis
	// key is harmless garbage that the 2-hour TTL reaps.
	redisKey := streamContentRedisKey(p.SyncID, p.BatchID)
	if err := s.streamRedis.Set(ctx, redisKey, payload, redisContentTTL).Err(); err != nil {
		return nil, fmt.Errorf("redis set: %w", err)
	}

	// ─────────── Step 6: Postgres outbox transaction ───────────
	acceptedFilesJSON, err := json.Marshal(acceptedFiles)
	if err != nil {
		return nil, fmt.Errorf("marshal accepted_files: %w", err)
	}
	acceptedDeletesJSON, err := json.Marshal(acceptedDeletes)
	if err != nil {
		return nil, fmt.Errorf("marshal accepted_deletes: %w", err)
	}
	batchSeenJSON, err := json.Marshal(map[string]bool{p.BatchID: true})
	if err != nil {
		return nil, fmt.Errorf("marshal batch_seen delta: %w", err)
	}

	var (
		duplicate bool
		queued    dto.QueuedCounts
	)
	txErr := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 6a. Lock the session row + re-check status. The lock guarantees
		//     this batch and any concurrent batch for the same sync_id
		//     serialize on the manifest merge / status flip below.
		locked, err := s.sessionRepo.LoadForUpdate(ctx, tx, p.SyncID)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return services.ErrSyncNotFound
			}
			return fmt.Errorf("lock session: %w", err)
		}
		if locked.Status != postgres.SyncStatusReceiving {
			return services.ErrSyncNotReceiving
		}

		// Cross-batch delete dedup: use the locked row's received_deletes
		// (not the pre-tx snapshot) to prevent concurrent batches from
		// double-appending the same delete path.
		if err := checkCrossBatchDeleteDedup(p.Entries, locked.ReceivedDeletes); err != nil {
			return err
		}

		// 6b. Authoritative dedup. The unique index on
		//     (sync_id, batch_id) is the single source of truth: if a
		//     concurrent request already inserted this batch, the helper
		//     returns inserted=false and we commit a no-op tx.
		batchRow := &postgres.SyncBatch{
			SyncID:          p.SyncID,
			BatchID:         p.BatchID,
			FileCount:       len(acceptedFiles),
			ByteCount:       totalBytes,
			AcceptedFiles:   datatypes.JSON(acceptedFilesJSON),
			AcceptedDeletes: datatypes.JSON(acceptedDeletesJSON),
		}
		_, inserted, err := s.batchRepo.InsertIfNotExists(ctx, tx, batchRow)
		if err != nil {
			return fmt.Errorf("insert sync_batch: %w", err)
		}
		if !inserted {
			duplicate = true
			return nil
		}

		// 6c. Merge accepted_* into received_* and append batch_id to
		//     batches_seen. Refresh expires_at to now()+10min so a slow
		//     uploader doesn't lose its session under TTL GC.
		if err := tx.Exec(`
			UPDATE sync_sessions
			SET received_files   = received_files   || ?::jsonb,
			    received_deletes = received_deletes || ?::jsonb,
			    batches_seen     = batches_seen     || ?::jsonb,
			    expires_at       = now() + interval '10 minutes'
			WHERE sync_id = ?
		`,
			string(acceptedFilesJSON),
			string(acceptedDeletesJSON),
			string(batchSeenJSON),
			p.SyncID,
		).Error; err != nil {
			return fmt.Errorf("merge received manifests: %w", err)
		}

		// 6d. Insert outbox row (task_type='stream_batch'). The dispatcher
		//     will pick this up via LISTEN/NOTIFY or the polling fallback.
		if err := s.outboxRepo.Insert(ctx, tx, &postgres.SyncOutbox{
			SyncID:   p.SyncID,
			BatchID:  p.BatchID,
			RedisKey: redisKey,
			TaskType: postgres.OutboxTaskStreamBatch,
			Status:   postgres.OutboxPending,
		}); err != nil {
			return fmt.Errorf("insert sync_outbox: %w", err)
		}

		// 6e. Conditional flip receiving → processing in the SAME tx.
		//     Plan §3.13 v3.2 / §5.2 v3.2: workers must NEVER touch
		//     status; this UPDATE is the only place that flips
		//     receiving→processing, atomically with the last batch's
		//     manifest merge. The subqueries see the post-update
		//     received_* columns from step 6c.
		if err := tx.Exec(`
			UPDATE sync_sessions
			SET status = 'processing'
			WHERE sync_id = ?
			  AND status = 'receiving'
			  AND (SELECT count(*) FROM jsonb_object_keys(received_files))
			      = (SELECT count(*) FROM jsonb_object_keys(expected_files))
			  AND jsonb_array_length(received_deletes) = jsonb_array_length(expected_deletes)
		`, p.SyncID).Error; err != nil {
			return fmt.Errorf("conditional flip to processing: %w", err)
		}

		queued = dto.QueuedCounts{
			Files:   len(acceptedFiles),
			Deletes: len(acceptedDeletes),
			Bytes:   totalBytes,
		}
		return nil
	})
	if txErr != nil {
		return nil, txErr
	}

	if duplicate {
		return &services.IngestBatchResult{Duplicate: true}, nil
	}

	// ─────────── Step 7: NOTIFY (post-commit, best-effort) ───────────
	// PG NOTIFY inside a tx is buffered and only delivered on COMMIT,
	// so an in-tx NOTIFY would be slightly more conservative. We issue
	// it AFTER commit instead, accepting one narrow crash window
	// (commit succeeds → process dies before NOTIFY) because the
	// dispatcher's 1-second polling fallback is the correctness
	// mechanism for missed wakeups; NOTIFY is purely a latency hint.
	if err := s.db.WithContext(ctx).Exec("NOTIFY sync_outbox_new").Error; err != nil {
		s.logger.Warn("notify sync_outbox_new failed (non-fatal)", zap.Error(err))
	}

	return &services.IngestBatchResult{Queued: queued}, nil
}

// streamContentRedisKey is the Redis key format for /stream batch
// content. Workers in WS5 must construct this key the same way (or
// just read it from the outbox row's redis_key column — preferred).
func streamContentRedisKey(syncID, batchID string) string {
	return fmt.Sprintf("supercoder:sync:%s:batch:%s:content", syncID, batchID)
}

// checkCrossBatchDeleteDedup verifies that no delete entry in this batch
// has already been received in a prior batch. Must be called inside the
// transaction after LoadForUpdate to use the locked row's received_deletes.
func checkCrossBatchDeleteDedup(entries []dto.BatchEntry, lockedReceivedDeletesRaw datatypes.JSON) error {
	if len(lockedReceivedDeletesRaw) == 0 {
		return nil
	}
	lockedReceivedDeletes, err := decodeExpectedDeleteSet(lockedReceivedDeletesRaw)
	if err != nil {
		return fmt.Errorf("decode locked received_deletes: %w", err)
	}
	for _, e := range entries {
		if e.Op == "delete" {
			if _, ok := lockedReceivedDeletes[e.Path]; ok {
				return &services.IngestBatchError{
					Reason:  "delete_already_received",
					Details: e.Path,
				}
			}
		}
	}
	return nil
}

// validateBatchEntries enforces all-or-nothing admission (plan §5.2
// v3.3). The first failure short-circuits the loop and returns an
// IngestBatchError with a stable Reason code. Empty batches are
// rejected up-front because there is nothing to validate, no batch_id
// to recompute meaningfully, and no work for a worker to do.
//
// Why deletes need an extra check (alreadyReceivedDeletes):
//
//	received_files is a JSONB OBJECT — merging the same {path: hash}
//	twice is idempotent at the count level (jsonb_object_keys is a
//	set), so a file path landing in two different batches with the
//	same content is safe.
//
//	received_deletes is a JSONB ARRAY. Appending the same path twice
//	via `||` doubles the array length, while expected_deletes stays
//	at the original size. The session-flip condition in step 6e
//	(jsonb_array_length comparison) then never matches, leaving the
//	session wedged in `receiving` until TTL GC silently expires it.
//
// Rather than change the schema (WS1 already shipped both as arrays),
// we enforce "each delete path lands in at most one batch" here. The
// in-tx unique constraint on sync_batches.(sync_id, batch_id) catches
// exact-duplicate retries; this guard catches the asymmetric case
// where two DIFFERENT batches both reference the same delete path.
func validateBatchEntries(
	entries []dto.BatchEntry,
	expectedFiles map[string]string,
	expectedDeletes map[string]struct{},
	alreadyReceivedDeletes map[string]struct{},
) error {
	if len(entries) == 0 {
		return &services.IngestBatchError{Reason: "empty_batch"}
	}
	// Track paths within THIS batch — a single batch with duplicate
	// file or delete entries would corrupt the accepted manifest or
	// wedge the array-length check. Same defense, in-batch scope.
	seenInBatch := make(map[string]struct{}, len(entries))
	for _, e := range entries {
		switch e.Op {
		case "file":
			if sha256Hex(e.Content) != e.Hash {
				return &services.IngestBatchError{Reason: "sha256_mismatch", Details: e.Path}
			}
			want, ok := expectedFiles[e.Path]
			if !ok {
				return &services.IngestBatchError{Reason: "not_in_expected_set", Details: e.Path}
			}
			if want != e.Hash {
				return &services.IngestBatchError{Reason: "hash_mismatch", Details: e.Path}
			}
			if _, ok := seenInBatch[e.Path]; ok {
				return &services.IngestBatchError{Reason: "file_duplicate_in_batch", Details: e.Path}
			}
			seenInBatch[e.Path] = struct{}{}
		case "delete":
			if _, ok := expectedDeletes[e.Path]; !ok {
				return &services.IngestBatchError{Reason: "not_in_expected_set", Details: e.Path}
			}
			if _, ok := alreadyReceivedDeletes[e.Path]; ok {
				return &services.IngestBatchError{Reason: "delete_already_received", Details: e.Path}
			}
			if _, ok := seenInBatch[e.Path]; ok {
				return &services.IngestBatchError{Reason: "delete_already_received", Details: e.Path}
			}
			seenInBatch[e.Path] = struct{}{}
		default:
			return &services.IngestBatchError{Reason: "unknown_op", Details: e.Op}
		}
	}
	return nil
}

// serializeBatchForRedis writes the batch entries as gzipped NDJSON in
// the same shape the client sends, so the WS5 worker can decode them
// with the same parser. Each line is one JSON object with op, path,
// sha256, content fields (content base64-encoded by encoding/json's
// default []byte handling).
func serializeBatchForRedis(entries []dto.BatchEntry) ([]byte, error) {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	enc := json.NewEncoder(gz)
	for i := range entries {
		// Use a stack-local view so the encoded layout matches the
		// /stream wire format exactly. We don't reuse dto.BatchEntry
		// because its field tags are package-internal.
		row := struct {
			Op      string `json:"op"`
			Path    string `json:"path"`
			SHA256  string `json:"sha256,omitempty"`
			Content []byte `json:"content,omitempty"`
		}{
			Op:      entries[i].Op,
			Path:    entries[i].Path,
			SHA256:  entries[i].Hash,
			Content: entries[i].Content,
		}
		if err := enc.Encode(&row); err != nil {
			return nil, err
		}
	}
	if err := gz.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// decodeBatchesSeen parses the JSONB batches_seen column. The on-disk
// shape is {batch_id: true}; missing/empty/null is treated as the empty
// map.
func decodeBatchesSeen(raw datatypes.JSON) (map[string]bool, error) {
	if len(raw) == 0 {
		return map[string]bool{}, nil
	}
	out := map[string]bool{}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// decodeExpectedFiles parses the JSONB expected_files column into a
// {path → sha256} map. /diff guarantees the column is a non-null JSON
// object so an empty value is treated as the empty map.
func decodeExpectedFiles(raw datatypes.JSON) (map[string]string, error) {
	if len(raw) == 0 {
		return map[string]string{}, nil
	}
	out := map[string]string{}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// decodeExpectedDeleteSet parses the JSONB expected_deletes column
// (a JSON array) into a set for O(1) membership tests in
// validateBatchEntries.
func decodeExpectedDeleteSet(raw datatypes.JSON) (map[string]struct{}, error) {
	if len(raw) == 0 {
		return map[string]struct{}{}, nil
	}
	var arr []string
	if err := json.Unmarshal(raw, &arr); err != nil {
		return nil, err
	}
	set := make(map[string]struct{}, len(arr))
	for _, p := range arr {
		set[p] = struct{}{}
	}
	return set, nil
}
