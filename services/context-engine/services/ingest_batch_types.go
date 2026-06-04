package services

import (
	"errors"

	"github.com/TransformerOptimus/SuperCoder/services/context-engine/models/dto"
)

// IngestBatchParams is the input to SyncSessionService.IngestBatch — the
// /stream service-layer entry point. SyncID and BatchID are pulled from
// the X-Sync-Id / X-Batch-Id request headers; Entries is the parsed
// gzipped NDJSON body. Identity is NOT carried here: the session row
// already pins (user_id, workspace_id, machine_id, repo_path) and
// supercoder has no in-process auth (per CLAUDE.md / MEMORY.md).
type IngestBatchParams struct {
	SyncID  string
	BatchID string
	Entries []dto.BatchEntry
}

// IngestBatchResult is the success response from IngestBatch. Exactly one
// of Duplicate / Queued is meaningful per result:
//
//	Duplicate=true → 200 {duplicate: true}     (idempotent retry path)
//	Duplicate=false → 202 {queued: {…}}         (newly accepted batch)
type IngestBatchResult struct {
	Duplicate bool
	Queued    dto.QueuedCounts
}

// IngestBatchError is the structured rejection type returned for any
// validation failure that the controller maps to a 400 response. Reason
// is a stable public string; Details only ever holds path-shaped data
// (never file content) so it is safe to include in error responses and
// logs (plan §3.23).
type IngestBatchError struct {
	Reason  string
	Details string
}

func (e *IngestBatchError) Error() string {
	if e.Details == "" {
		return e.Reason
	}
	return e.Reason + ": " + e.Details
}

// Sentinel errors mapped to HTTP status by the /stream controller.
//
//	ErrSyncNotFound      → 410 Gone (sync_expired)
//	ErrSyncNotReceiving  → 410 Gone (sync_expired)
//
// They live in the services package (not repositories) because they
// represent service-layer state machine assertions, not persistence
// errors. The controller imports them via errors.Is.
var (
	ErrSyncNotFound     = errors.New("sync_not_found")
	ErrSyncNotReceiving = errors.New("sync_not_receiving")
)
