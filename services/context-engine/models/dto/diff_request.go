package dto

// DiffFileHash is a single (path, sha256) entry in a /diff request's Files
// list. The path is repo-relative and the hash is the client's claim about
// the current file content. The server compares the claim against the
// Merkle tree to compute `need[]`; stale-hash detection (client claim vs.
// what they actually upload at /stream time) lives in /stream's per-line
// validation, not here.
type DiffFileHash struct {
	Path   string `json:"path" binding:"required"`
	SHA256 string `json:"sha256" binding:"required"`
}

// DiffRequest is the body of POST /api/v1/index/diff. Identity is carried
// in the body (matching dto.IndexRequest) rather than headers — supercoder
// has no in-process auth and trusts the upstream APISIX-rewritten headers,
// but the streaming-sync tables (sync_sessions.user_id varchar, workspace_id
// uint64) are locked into the body convention.
//
// Incremental semantics (plan §3.8 v3.4):
//
//   - Incremental=false (full sync): Files is the client's complete repo
//     view. Server computes delete[] as "every Merkle path absent from
//     Files". Deletes field MUST be empty in this mode.
//
//   - Incremental=true (watcher delta): Files is ONLY the changed files.
//     Server returns need[] computed against the Merkle tree over those
//     paths only, and delete[] is exactly Deletes. Pure delete events
//     (Files=[], Deletes=[old.rs]) are valid and create a session with
//     expected_files=[].
type DiffRequest struct {
	UserID      string `json:"user_id" binding:"required"`
	WorkspaceID uint64 `json:"workspace_id" binding:"required"`
	MachineID   string `json:"machine_id" binding:"required"`
	RepoPath    string `json:"repo_path" binding:"required"`
	GithubOrgID string `json:"github_org_id,omitempty"`

	// Files may be empty in incremental pure-delete mode — not `binding:"required"`.
	Files       []DiffFileHash `json:"files"`
	Incremental bool           `json:"incremental,omitempty"`
	Deletes     []string       `json:"deletes,omitempty"`
}
