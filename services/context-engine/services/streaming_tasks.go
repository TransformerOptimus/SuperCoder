package services

import (
	"crypto/sha256"
	"encoding/hex"
)

// streaming_tasks.go is the shared contract between WS4 (the outbox
// dispatcher that enqueues Asynq tasks) and WS5 (the worker handlers that
// consume them). Both halves import these constants + payload types so the
// task name and payload schema cannot drift between writer and reader.
//
// Task IDs are deterministic so Asynq's dedup window collapses retries —
// re-enqueuing the same outbox row after a transient failure produces the
// same task ID and lands as ErrTaskIDConflict (treated as success by the
// dispatcher).

const (
	// TaskTypeStreamBatch is the Asynq task name for one /stream batch.
	// Payload: StreamBatchPayload.
	TaskTypeStreamBatch = "index:stream_batch"

	// TaskTypeFinalize is the Asynq task name for the per-session finalizer.
	// Payload: FinalizePayload.
	TaskTypeFinalize = "finalize:sync"
)

// StreamBatchPayload is the Asynq payload for one stream_batch task. The
// worker reads RedisKey to fetch the file content, validates against the
// session's expected manifest, and runs IndexChangedFiles.
type StreamBatchPayload struct {
	SyncID   string `json:"sync_id"`
	BatchID  string `json:"batch_id"`
	RedisKey string `json:"redis_key"`
}

// FinalizePayload is the Asynq payload for the per-session finalizer task.
// The finalizer drives the Merkle CAS commit and flips the session to
// done/failed.
type FinalizePayload struct {
	SyncID string `json:"sync_id"`
}

// StreamBatchTaskID is sha256(sync_id || "|" || batch_id), hex-encoded.
// Plan §3.13 — deterministic so the dispatcher can retry an outbox row
// without producing duplicate Asynq tasks.
func StreamBatchTaskID(syncID, batchID string) string {
	sum := sha256.Sum256([]byte(syncID + "|" + batchID))
	return hex.EncodeToString(sum[:])
}

// FinalizeTaskID returns the deterministic Asynq task ID for the
// per-session finalizer. The "finalize:" prefix is purely cosmetic — Asynq
// only requires uniqueness — but it makes the task ID self-describing in
// asynqmon.
func FinalizeTaskID(syncID string) string {
	return "finalize:" + syncID
}
