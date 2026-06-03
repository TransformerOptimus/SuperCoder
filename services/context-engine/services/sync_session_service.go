package services

import (
	"context"
	"time"
)

// DiffParams is the input to SyncSessionService.Diff. It carries the
// identity read from the /diff request body, the client's claimed file
// hashes, and the incremental-mode flag plus explicit deletes.
type DiffParams struct {
	UserID      string
	WorkspaceID uint64
	MachineID   string
	RepoPath    string
	GithubOrgID string

	// ClientHashes is {repo-relative path -> client-claimed sha256}. The
	// service compares against the Merkle tree to compute `need[]`.
	ClientHashes map[string]string

	// Incremental=false → full diff; ExplicitDeletes must be empty (the
	// controller enforces this with a 400).
	// Incremental=true  → only ExplicitDeletes is used for delete[]; the
	// server MUST NOT infer deletes from tree paths missing from
	// ClientHashes. This is the critical invariant preventing watcher
	// events from wiping the index.
	Incremental     bool
	ExplicitDeletes []string
}

// DiffResult is the output of SyncSessionService.Diff — the server-issued
// sync_id plus the two path lists the client will act on. Need and Delete
// are always non-nil slices (may be empty).
type DiffResult struct {
	SyncID string
	Need   []string
	Delete []string
}

// Counts is the lengths of the four JSONB manifests for either files or
// deletes in a sync_sessions row.
type Counts struct {
	Expected  int
	Received  int
	Processed int
	Failed    int
}

// StatusSnapshot is the read-only projection of a sync_sessions row used
// by /sync-complete. No side effects, no locking.
type StatusSnapshot struct {
	Status       string
	FileCounts   Counts
	DeleteCounts Counts
	FailedReason string
}

// SyncSessionService is the request-time service surface for the
// streaming indexing flow:
//
//	Diff         — POST /api/v1/index/diff           (WS3)
//	GetStatus    — POST /api/v1/index/sync-complete  (WS3)
//	IngestBatch  — POST /api/v1/index/stream         (WS4)
//	RunTTLGCLoop — background janitor                (WS3)
//
// IngestBatch owns the transactional outbox write that admits a /stream
// batch: validation, Redis SET, sync_batches insert, sync_outbox insert,
// session manifest merge, and the atomic receiving→processing flip when
// the batch closes the session. The outbox dispatcher (WS4) and the
// Asynq workers (WS5) deliberately do NOT go through this service —
// they mutate sync_sessions / sync_batches via the repositories
// directly so the service stays free of worker-only state machines.
type SyncSessionService interface {
	Diff(ctx context.Context, params *DiffParams) (*DiffResult, error)
	GetStatus(ctx context.Context, syncID string) (*StatusSnapshot, error)
	RunTTLGCLoop(ctx context.Context, interval time.Duration)
	IngestBatch(ctx context.Context, params *IngestBatchParams) (*IngestBatchResult, error)
}
