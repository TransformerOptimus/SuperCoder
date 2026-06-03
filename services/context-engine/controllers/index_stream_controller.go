package controllers

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	apicontext "github.com/TransformerOptimus/SuperCoder/services/context-engine/internal/pkg/api/context"
	"github.com/TransformerOptimus/SuperCoder/services/context-engine/models/dto"
	"github.com/TransformerOptimus/SuperCoder/services/context-engine/services"
)

// streamMaxDecompressedBytes is the 50 MB cap from plan §3.25 — large
// enough to hold one /stream batch (per-file 1 MB cap × ~50 files +
// JSON overhead) but small enough to bound memory pressure if a client
// sends a pathological payload.
const streamMaxDecompressedBytes = 50 * 1024 * 1024

// streamScannerInitialBuf / streamScannerMaxBuf size the bufio.Scanner
// used to split the NDJSON body. The max must comfortably exceed the
// 1 MB per-file content cap so the largest possible single line still
// fits — 10 MB gives ~10× headroom for base64 expansion + JSON keys.
const (
	streamScannerInitialBuf = 64 * 1024
	streamScannerMaxBuf     = 10 * 1024 * 1024
)

// IndexStreamController handles POST /api/v1/index/stream — the /stream
// endpoint of the streaming sync flow. The body is gzipped NDJSON; each
// line is one BatchEntry. Two custom headers identify the batch:
//
//	X-Sync-Id   — UUID returned by the prior /diff call
//	X-Batch-Id  — client-computed sha256 of sorted op|path|hash lines
//
// The handler does no auth (supercoder has no in-process auth — see
// CLAUDE.md / MEMORY.md); identity is implied by the session row that
// X-Sync-Id refers to.
//
// Like the WS3 controllers, this one bypasses
// apicontext.Ok/BadRequest/InternalServerError so the documented
// response shape (StreamBatchResponse / {error, details}) is not
// wrapped in a {success: true/false} envelope. The
// TestStream_InternalError_500_NoEnvelope test locks this in.
type IndexStreamController struct {
	logger  *zap.Logger
	service services.SyncSessionService
}

func NewIndexStreamController(logger *zap.Logger, service services.SyncSessionService) *IndexStreamController {
	return &IndexStreamController{
		logger:  logger.Named("controllers.index_stream"),
		service: service,
	}
}

// Stream is the HTTP handler. Flow:
//
//  1. Require Content-Encoding: gzip → 415 otherwise.
//  2. Read X-Sync-Id and X-Batch-Id → 400 missing_headers if absent.
//  3. Decompress with the 50 MB cap → 400 malformed_body on overflow
//     or malformed gzip.
//  4. Parse NDJSON into []dto.BatchEntry → 400 malformed_body on
//     parse failure.
//  5. Delegate to SyncSessionService.IngestBatch and map the result.
//
// Error mapping:
//
//	*services.IngestBatchError       → 400 {error: reason, details}
//	services.ErrSyncNotFound|...     → 410 {error: "sync_expired"}
//	other                            → 500 {error: "internal_error"}
//
// Success:
//
//	IngestBatchResult{Duplicate: true} → 200 {duplicate: true}
//	IngestBatchResult{Queued: …}        → 202 {queued: {…}}
func (ctrl *IndexStreamController) Stream(c *apicontext.Context) {
	if !strings.EqualFold(c.GetHeader("Content-Encoding"), "gzip") {
		c.JSON(http.StatusUnsupportedMediaType, gin.H{"error": "gzip_required"})
		return
	}

	syncID := c.GetHeader("X-Sync-Id")
	batchID := c.GetHeader("X-Batch-Id")
	if syncID == "" || batchID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing_headers"})
		return
	}

	body, err := decompressGzipBody(c.Request.Body, streamMaxDecompressedBytes)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "malformed_body",
			"details": err.Error(),
		})
		return
	}

	entries, err := parseNDJSONBatch(body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "malformed_body",
			"details": err.Error(),
		})
		return
	}

	result, err := ctrl.service.IngestBatch(c.Request.Context(), &services.IngestBatchParams{
		SyncID:  syncID,
		BatchID: batchID,
		Entries: entries,
	})
	if err != nil {
		var batchErr *services.IngestBatchError
		switch {
		case errors.As(err, &batchErr):
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   batchErr.Reason,
				"details": batchErr.Details,
			})
		case errors.Is(err, services.ErrSyncNotFound),
			errors.Is(err, services.ErrSyncNotReceiving):
			c.JSON(http.StatusGone, gin.H{"error": "sync_expired"})
		default:
			// IMPORTANT: do NOT log entries or any field that could
			// transitively contain file content. sync_id + batch_id are
			// the only identifying fields safe to emit.
			ctrl.logger.Error("ingest batch failed",
				zap.Error(err),
				zap.String("sync_id", syncID),
				zap.String("batch_id", batchID),
			)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal_error"})
		}
		return
	}

	if result.Duplicate {
		c.JSON(http.StatusOK, dto.StreamBatchResponse{Duplicate: true})
		return
	}
	c.JSON(http.StatusAccepted, dto.StreamBatchResponse{Queued: &result.Queued})
}

// parseNDJSONBatch decodes the gzip-decompressed body into a slice of
// BatchEntry. Each non-blank line is one JSON object with op, path,
// sha256, content fields. Content is decoded as []byte (base64 via
// encoding/json's default handling), matching the gzipped NDJSON format
// the Rust client emits.
//
// The bufio.Scanner buffer is sized to fit a single line up to
// streamScannerMaxBuf (10 MB) — larger than the 1 MB per-file content
// cap with comfortable headroom for base64 + JSON wrapping.
func parseNDJSONBatch(body []byte) ([]dto.BatchEntry, error) {
	scanner := bufio.NewScanner(bytes.NewReader(body))
	scanner.Buffer(make([]byte, streamScannerInitialBuf), streamScannerMaxBuf)

	var entries []dto.BatchEntry
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		var raw struct {
			Op      string `json:"op"`
			Path    string `json:"path"`
			SHA256  string `json:"sha256,omitempty"`
			Content []byte `json:"content,omitempty"`
		}
		if err := json.Unmarshal(line, &raw); err != nil {
			return nil, err
		}
		entries = append(entries, dto.BatchEntry{
			Op:      raw.Op,
			Path:    raw.Path,
			Hash:    raw.SHA256,
			Content: raw.Content,
		})
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return entries, nil
}
