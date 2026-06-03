package services

import (
	"context"
	"errors"
)

// ErrVersionMismatch is returned by SaveTreeIfUnchanged when the caller's
// expected version token does not match the persisted version — somebody
// else committed a newer tree in between. Callers re-read and retry, up to
// the retry budget, before marking the session failed (plan §3.1, §8.6).
var ErrVersionMismatch = errors.New("merkle version mismatch")

// ErrStoragePermanent marks a permanent object-store failure that cannot
// be fixed by retrying (e.g. revoked credentials, missing bucket,
// permission denied). The finalizer checks for this via errors.Is and
// stamps the session as MarkFailed(reason="merkle_storage_permanent")
// instead of burning the Asynq retry budget on a doomed task. Backend
// implementations wrap the underlying SDK error with errors.Join or a
// custom wrapper that preserves both the sentinel and the original
// cause (keep the original available via errors.Unwrap / zap.Error).
var ErrStoragePermanent = errors.New("merkle storage permanent error")

// MerkleNode represents a node in a Merkle tree built from the repository file structure.
type MerkleNode struct {
	Path     string        `json:"path"`
	Hash     string        `json:"hash"`
	IsDir    bool          `json:"is_dir"`
	Children []*MerkleNode `json:"children,omitempty"`
}

// MerkleDiff captures the differences between two Merkle trees.
type MerkleDiff struct {
	Added   []string // new files
	Changed []string // modified files
	Deleted []string // removed files
}

// MerkleService builds and compares Merkle trees for incremental indexing.
type MerkleService interface {
	BuildTree(ctx context.Context, provider SourceProvider, root string) (*MerkleNode, error)
	DiffTrees(old, new *MerkleNode) *MerkleDiff
	LoadTree(ctx context.Context, collection string) (*MerkleNode, error)
	SaveTree(ctx context.Context, collection string, tree *MerkleNode) error
	DeleteTree(ctx context.Context, collection string) error

	// LoadTreeWithVersion returns the persisted tree together with an
	// opaque version token that identifies the current committed state.
	// When no tree exists the call returns (empty-tree, "", nil): callers
	// can pass the empty string as expectedVersion to SaveTreeIfUnchanged
	// to perform a first-write. The version semantics are backend
	// specific — S3 uses ETag, the local filesystem uses the sha256 of
	// the serialized bytes (plan §3.1, §8.7).
	LoadTreeWithVersion(ctx context.Context, collection string) (*MerkleNode, string, error)

	// SaveTreeIfUnchanged commits the tree iff the persisted version
	// still matches expectedVersion. It returns ErrVersionMismatch when
	// another writer has committed in between, letting the caller
	// re-read and retry. Pass "" as expectedVersion for a first-write
	// (fails if any tree already exists).
	SaveTreeIfUnchanged(ctx context.Context, collection string, tree *MerkleNode, expectedVersion string) error

	// ApplyChanges returns a new tree obtained by merging updates (path →
	// sha256) into the leaves of tree and removing every path in deletes.
	// Used by the WS5 finalizer to commit only the successfully-indexed
	// files from a sync session — failed files are deliberately left out
	// so the next /diff naturally re-requests them. The returned tree has
	// recomputed hashes all the way to the root (plan §3.20, §8.6).
	//
	// Does not mutate tree. A nil tree is treated as the empty tree; an
	// empty updates/deletes delta returns a freshly-computed copy.
	ApplyChanges(tree *MerkleNode, updates map[string]string, deletes []string) *MerkleNode
}
