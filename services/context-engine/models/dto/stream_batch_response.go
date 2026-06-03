package dto

// QueuedCounts is the success projection returned to the client when a
// batch is newly accepted. Bytes is the sum of file content lengths in
// the batch (file metadata only — deletes contribute zero).
type QueuedCounts struct {
	Files   int   `json:"files"`
	Deletes int   `json:"deletes"`
	Bytes   int64 `json:"bytes"`
}

// StreamBatchResponse is the /stream response envelope. Exactly one of
// Queued / Duplicate is populated; the omitempty tags ensure the JSON
// wire format never carries both. The endpoint never returns the
// success/failure envelope from apicontext — its shape is documented
// externally and must match byte-for-byte.
type StreamBatchResponse struct {
	Queued    *QueuedCounts `json:"queued,omitempty"`
	Duplicate bool          `json:"duplicate,omitempty"`
}
