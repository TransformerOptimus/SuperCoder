package repositories

import "errors"

// ErrConcurrentSyncInProgress is returned by CreateSessionExclusive when a
// session for the same (user_id, workspace_id, machine_id, repo_path) is
// already in a non-terminal state. Controllers translate this to HTTP 429
// with a Retry-After header; the client should debounce and retry.
var ErrConcurrentSyncInProgress = errors.New("concurrent sync in progress")
