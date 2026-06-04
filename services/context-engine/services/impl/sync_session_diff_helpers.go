package impl

import (
	"encoding/json"
	"fmt"
	"hash/fnv"
	"sort"

	"gorm.io/datatypes"

	"github.com/TransformerOptimus/SuperCoder/services/context-engine/services"
)

// flattenTree walks a Merkle tree and returns a {path -> hash} map of the
// leaf files. Dir nodes are skipped. Nil input is tolerated and yields an
// empty map (first-ever sync for a repo has no stored tree).
func flattenTree(root *services.MerkleNode) map[string]string {
	out := map[string]string{}
	if root == nil {
		return out
	}
	var walk func(*services.MerkleNode)
	walk = func(n *services.MerkleNode) {
		if n == nil {
			return
		}
		if !n.IsDir {
			out[n.Path] = n.Hash
			return
		}
		for _, c := range n.Children {
			walk(c)
		}
	}
	walk(root)
	return out
}

// computeDiff is the full-sync diff. `need` = client paths whose hash is
// absent from or differs from the tree; `deletes` = tree paths absent from
// the client list. Both results are sorted and non-nil.
func computeDiff(tree *services.MerkleNode, client map[string]string) (need, deletes []string) {
	treeHashes := flattenTree(tree)
	need = []string{}
	deletes = []string{}

	for p, ch := range client {
		if th, ok := treeHashes[p]; !ok || th != ch {
			need = append(need, p)
		}
	}
	for p := range treeHashes {
		if _, ok := client[p]; !ok {
			deletes = append(deletes, p)
		}
	}
	sort.Strings(need)
	sort.Strings(deletes)
	return need, deletes
}

// computeIncrementalNeed is the incremental-mode diff. It iterates ONLY
// the client's submitted paths and never walks the tree for deletes —
// that invariant is what prevents watcher events from wiping the index.
// Deletes come exclusively from DiffParams.ExplicitDeletes in the caller.
func computeIncrementalNeed(tree *services.MerkleNode, client map[string]string) []string {
	treeHashes := flattenTree(tree)
	need := []string{}
	for p, ch := range client {
		if th, ok := treeHashes[p]; !ok || th != ch {
			need = append(need, p)
		}
	}
	sort.Strings(need)
	return need
}

// advisoryLockKey derives the int64 argument to pg_advisory_xact_lock from
// a client identity. Collisions between unrelated identities are harmless:
// they'd just occasionally serialize, and CreateSessionExclusive's in-tx
// existence check preserves correctness either way. fnv-64a is chosen for
// deterministic no-allocation hashing; no cryptographic properties needed.
func advisoryLockKey(userID string, workspaceID uint64, machineID, repoPath string) int64 {
	h := fnv.New64a()
	fmt.Fprintf(h, "%s|%d|%s|%s", userID, workspaceID, machineID, repoPath)
	return int64(h.Sum64()) //nolint:gosec // intentional reinterpret for pg_advisory_xact_lock arg
}

// marshalJSONMapOrEmpty encodes a map[string]string as a jsonb-compatible
// byte slice, falling back to "{}" for nil or empty maps. The sync_sessions
// jsonb columns are NOT NULL, so `json.Marshal(nil)` (which emits "null")
// would violate the constraint.
func marshalJSONMapOrEmpty(m map[string]string) datatypes.JSON {
	if len(m) == 0 {
		return emptyJSONObject()
	}
	b, err := json.Marshal(m)
	if err != nil || len(b) == 0 {
		return emptyJSONObject()
	}
	return datatypes.JSON(b)
}

// marshalJSONSliceOrEmpty is the slice counterpart to marshalJSONMapOrEmpty.
// Uses "[]" as the zero-value rather than "{}" — a list column fed "{}"
// would deserialize as a struct at the next reader.
func marshalJSONSliceOrEmpty(s []string) datatypes.JSON {
	if len(s) == 0 {
		return emptyJSONArray()
	}
	b, err := json.Marshal(s)
	if err != nil || len(b) == 0 {
		return emptyJSONArray()
	}
	return datatypes.JSON(b)
}

func emptyJSONObject() datatypes.JSON { return datatypes.JSON([]byte("{}")) }
func emptyJSONArray() datatypes.JSON  { return datatypes.JSON([]byte("[]")) }

// jsonObjectLen counts top-level keys in a jsonb object. Defensive against
// nil/empty/malformed input — returns 0 rather than panicking, so
// /sync-complete never fails on a legacy or hand-inserted row.
func jsonObjectLen(raw datatypes.JSON) int {
	if len(raw) == 0 {
		return 0
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		return 0
	}
	return len(m)
}

// jsonArrayLen counts elements in a jsonb array, with the same defensive
// behaviour as jsonObjectLen.
func jsonArrayLen(raw datatypes.JSON) int {
	if len(raw) == 0 {
		return 0
	}
	var a []json.RawMessage
	if err := json.Unmarshal(raw, &a); err != nil {
		return 0
	}
	return len(a)
}
