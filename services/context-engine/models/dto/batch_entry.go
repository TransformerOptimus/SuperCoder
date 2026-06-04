package dto

// BatchEntry is one parsed line of the gzipped NDJSON /stream request
// body. Op is "file" or "delete". For deletes, Hash and Content are
// empty.
//
// Content holds raw file bytes and MUST NEVER appear in log lines (plan
// §3.23 — no Prometheus metrics, no CI redaction tests, manual discipline
// is the v1 enforcement mechanism). Treat Content like a credential.
type BatchEntry struct {
	Op      string
	Path    string
	Hash    string // sha256 hex; empty for deletes
	Content []byte // file body; never logged
}
