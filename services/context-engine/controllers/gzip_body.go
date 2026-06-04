package controllers

import (
	"compress/gzip"
	"fmt"
	"io"
)

// decompressGzipBody reads a gzipped request body and enforces a hard cap
// on decompressed bytes. Zip-bomb defense: we read one byte beyond the
// limit so a payload at exactly maxBytes+1 is rejected rather than
// accepted as "≤ maxBytes".
//
// The caller must have already verified Content-Encoding: gzip on the
// request — this helper does not inspect headers. Shared by /diff
// (5 MB cap, plan §3.25) and will be reused by WS4 /stream (50 MB cap).
//
// Lifecycle: this function does NOT close the request body. gin's HTTP
// server owns the request lifecycle and closes the body after the handler
// returns. Closing it here would be a no-op for HTTP/1.1 but could send
// an early RST_STREAM under HTTP/2 multiplexing — undesirable for WS4's
// large /stream uploads when an error fires partway through.
func decompressGzipBody(body io.Reader, maxBytes int64) ([]byte, error) {
	gz, err := gzip.NewReader(body)
	if err != nil {
		return nil, fmt.Errorf("gzip reader: %w", err)
	}
	defer gz.Close()

	limited := io.LimitReader(gz, maxBytes+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return nil, fmt.Errorf("read gzip body: %w", err)
	}
	if int64(len(data)) > maxBytes {
		return nil, fmt.Errorf("decompressed body exceeds %d bytes", maxBytes)
	}
	return data, nil
}
