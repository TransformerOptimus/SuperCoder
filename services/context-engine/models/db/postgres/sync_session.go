package postgres

import (
	"time"

	"gorm.io/datatypes"
)

// SyncSessionStatus enumerates the lifecycle states of a streaming sync session.
// The state machine is: receiving -> processing -> finalizing -> done|failed.
// expired is a terminal state set by the TTL GC for sessions that stalled in
// a non-terminal state past expires_at.
type SyncSessionStatus string

const (
	SyncStatusReceiving  SyncSessionStatus = "receiving"
	SyncStatusProcessing SyncSessionStatus = "processing"
	SyncStatusFinalizing SyncSessionStatus = "finalizing"
	SyncStatusDone       SyncSessionStatus = "done"
	SyncStatusFailed     SyncSessionStatus = "failed"
	SyncStatusExpired    SyncSessionStatus = "expired"
)

// SyncSession is one row per /diff call. It tracks the full lifecycle of a
// streaming sync: what the client promised to send (expected_*), and what
// was received on /stream (received_*). The merkle version at diff time is
// stored for diagnostics only; the finalizer re-reads the merkle tree fresh
// on every commit attempt (see plan §3.1, §8.6).
//
// PR5 removed processed_files / processed_deletes / failed_files /
// failed_deletes from this row. Per-batch result state now lives on
// sync_batches (state enum + failed_files/failed_deletes diff columns). The
// old JSONB merge on sync_sessions was O(N) per batch → O(N²) over a
// session and dominated /stream write latency at scale (~1227 ms on a
// 4576-file sync). Finalizer and GetStatus reconstruct the per-session
// totals by aggregating over sync_batches via SyncBatchRepository
// .LoadTerminalResults.
type SyncSession struct {
	SyncID         string `gorm:"type:uuid;primaryKey"`
	UserID         string `gorm:"type:varchar(255);not null;index:idx_sync_sessions_identity,priority:1"`
	WorkspaceID    uint64 `gorm:"not null;index:idx_sync_sessions_identity,priority:2"`
	MachineID      string `gorm:"type:varchar(255);not null;index:idx_sync_sessions_identity,priority:3"`
	RepoPath       string `gorm:"type:text;not null;index:idx_sync_sessions_identity,priority:4"`
	RepoID         uint   `gorm:"not null;index"`
	Repo           Repo   `gorm:"foreignKey:RepoID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	CollectionName string `gorm:"type:varchar(255);not null"`
	GithubOrgID    string `gorm:"type:varchar(255)"`

	ExpectedFiles   datatypes.JSON `gorm:"type:jsonb;not null;default:'{}'"`
	ExpectedDeletes datatypes.JSON `gorm:"type:jsonb;not null;default:'[]'"`

	ReceivedFiles   datatypes.JSON `gorm:"type:jsonb;not null;default:'{}'"`
	ReceivedDeletes datatypes.JSON `gorm:"type:jsonb;not null;default:'[]'"`

	BatchesSeen datatypes.JSON `gorm:"type:jsonb;not null;default:'{}'"`

	MerkleVersionAtDiff string `gorm:"type:text"`
	FailedReason        string `gorm:"type:text"`

	Status      SyncSessionStatus `gorm:"type:varchar(20);not null;check:status IN ('receiving','processing','finalizing','done','failed','expired')"`
	CreatedAt   time.Time         `gorm:"not null;default:CURRENT_TIMESTAMP"`
	ExpiresAt   time.Time         `gorm:"not null;index"`
	CompletedAt *time.Time
}

func (SyncSession) TableName() string { return "sync_sessions" }
