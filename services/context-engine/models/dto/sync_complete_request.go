package dto

// SyncCompleteRequest is the body of POST /api/v1/index/sync-complete.
// It's a status-only query — the controller translates the session's
// current status to an HTTP code (409/202/200/410) and reads counts from
// the row. Zero side effects.
type SyncCompleteRequest struct {
	SyncID string `json:"sync_id" binding:"required"`
}
