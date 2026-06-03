package consumers

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"

	"github.com/TransformerOptimus/SuperCoder/services/context-engine/services"
)

// decodeBatchContent is the inverse of
// services/impl/sync_session_service_ingest.go:serializeBatchForRedis. The
// /stream writer emits gzipped NDJSON with one JSON object per line; this
// reader decodes the blob pulled out of Redis by the WS5 stream_batch
// consumer and partitions the entries into files (op=file, with Content)
// and delete paths (op=delete).
//
// Any error returned here is wrapped by the caller as
// services.Terminal("decode_error", err) so the worker marks the batch
// failed via the durable manifest and does not retry — a malformed blob
// will never decode successfully no matter how many times we try.
//
// The worker re-validates NOTHING here: /stream already validated each
// entry's sha256 against the claimed hash and the session's expected
// manifest before the batch was accepted. This decoder is trust-the-wire.
func decodeBatchContent(data []byte) (files []services.QueueFile, deletes []string, err error) {
	gz, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, nil, fmt.Errorf("gzip reader: %w", err)
	}
	defer gz.Close()

	dec := json.NewDecoder(gz)
	for dec.More() {
		// The anonymous struct matches serializeBatchForRedis exactly.
		// encoding/json handles the base64 round-trip for []byte on the
		// Content field.
		var row struct {
			Op      string `json:"op"`
			Path    string `json:"path"`
			SHA256  string `json:"sha256,omitempty"`
			Content []byte `json:"content,omitempty"`
		}
		if err := dec.Decode(&row); err != nil {
			return nil, nil, fmt.Errorf("decode row: %w", err)
		}
		switch row.Op {
		case "file":
			files = append(files, services.QueueFile{
				Path:    row.Path,
				Hash:    row.SHA256,
				Content: row.Content,
			})
		case "delete":
			deletes = append(deletes, row.Path)
		default:
			return nil, nil, fmt.Errorf("unknown op %q", row.Op)
		}
	}
	return files, deletes, nil
}
