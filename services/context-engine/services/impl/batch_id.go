package impl

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strings"

	"github.com/TransformerOptimus/SuperCoder/services/context-engine/models/dto"
)

// computeBatchID is the server-side recomputation of the deterministic
// batch_id (plan §3.6 v3.2 / v3.3). It MUST stay byte-identical with the
// client's `deterministic_batch_id` so the X-Batch-Id header validation
// in IngestBatch never has spurious mismatches.
//
// Algorithm:
//
//  1. For each entry, format "<op>|<path>|<hash>". Hash is the empty
//     string for delete entries — the format is unconditional so the
//     client and server agree on byte layout.
//  2. Sort the strings lexicographically.
//  3. Join with "\n".
//  4. sha256, hex-encode (lowercase).
//
// The hash is included so two different file contents at the same path
// produce different batch IDs — preventing a stale-content retry from
// being silently coalesced with a fresher batch.
func computeBatchID(entries []dto.BatchEntry) string {
	parts := make([]string, len(entries))
	for i, e := range entries {
		parts[i] = e.Op + "|" + e.Path + "|" + e.Hash
	}
	sort.Strings(parts)
	sum := sha256.Sum256([]byte(strings.Join(parts, "\n")))
	return hex.EncodeToString(sum[:])
}

// sha256Hex is a tiny convenience used by IngestBatch to validate
// content hashes. Lowercase hex; matches what the client emits.
func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
