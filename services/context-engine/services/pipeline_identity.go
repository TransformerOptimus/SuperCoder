package services

// IndexIdentity carries the metadata stamped onto every CodeElement during
// enrichment. Pipeline.Index builds it from *dto.IndexRequest; the streaming
// worker (WS5) builds it from a SyncSession row. Keeping IndexChangedFiles'
// signature free of dto types lets both callers reuse the same code path.
type IndexIdentity struct {
	UserID      string
	WorkspaceID uint64
	MachineID   string
	GithubOrgID string
	RepoID      uint
}

// IndexStats reports per-file outcomes from Pipeline.IndexChangedFiles.
//
// Contract:
//
//	(stats, nil) → batch reached a commit-worthy state.
//	               Streaming workers call markBatchProcessed(stats);
//	               ProcessedFiles + FailedFiles are both meaningful.
//
//	(nil,  err)  → whole-batch failure. stats is not consulted;
//	               streaming workers rely on the durable batch manifest
//	               in sync_batches.accepted_files / accepted_deletes to
//	               mark the batch terminally failed.
//
// The legacy Pipeline.Index wrapper inspects FailedFiles / FailedDeletes to
// decide whether to save the Merkle tree (skips save when anything failed so
// the next legacy Index call automatically retries).
type IndexStats struct {
	ProcessedFiles   map[string]string // path -> sha256 (used by streaming Merkle commit)
	FailedFiles      map[string]string // path -> reason
	ProcessedDeletes []string
	FailedDeletes    []string
}

// NewIndexStats returns an IndexStats with all collections initialised.
func NewIndexStats() *IndexStats {
	return &IndexStats{
		ProcessedFiles:   map[string]string{},
		FailedFiles:      map[string]string{},
		ProcessedDeletes: []string{},
		FailedDeletes:    []string{},
	}
}
