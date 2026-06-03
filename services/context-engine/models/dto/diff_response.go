package dto

// DiffResponse is the successful (HTTP 200) body of POST /api/v1/index/diff.
// Need and Delete are always non-nil arrays — the controller normalises
// empty results to [] so clients never have to special-case JSON null.
type DiffResponse struct {
	SyncID string   `json:"sync_id"`
	Need   []string `json:"need"`
	Delete []string `json:"delete"`
}
