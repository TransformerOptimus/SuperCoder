package dto

// CountPair is a projection of sync_sessions jsonb columns into one set of
// counters. Which counters are populated depends on the response phase:
//
//	receiving   -> Expected, Received
//	processing  -> Expected, Received, Processed
//	done        -> Expected, Processed, Failed
//
// Unused counters are omitted via omitempty so clients don't have to
// interpret "zero" as "not applicable".
type CountPair struct {
	Expected  int `json:"expected"`
	Received  int `json:"received,omitempty"`
	Processed int `json:"processed,omitempty"`
	Failed    int `json:"failed,omitempty"`
}

// SyncCompleteResponse is the body returned for every status of
// POST /api/v1/index/sync-complete. The HTTP code carries the primary
// status signal; Status duplicates it for clients that can't see the
// code. Error/Reason are only set on 410 responses.
type SyncCompleteResponse struct {
	Status  string    `json:"status"`
	Files   CountPair `json:"files,omitempty"`
	Deletes CountPair `json:"deletes,omitempty"`
	Error   string    `json:"error,omitempty"`
	Reason  string    `json:"reason,omitempty"`
}
