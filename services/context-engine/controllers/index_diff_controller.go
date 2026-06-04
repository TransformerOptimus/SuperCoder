package controllers

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	apicontext "github.com/TransformerOptimus/SuperCoder/services/context-engine/internal/pkg/api/context"
	"github.com/TransformerOptimus/SuperCoder/services/context-engine/models/dto"
	"github.com/TransformerOptimus/SuperCoder/services/context-engine/repositories"
	"github.com/TransformerOptimus/SuperCoder/services/context-engine/services"
)

// diffMaxDecompressedBytes is the 5 MB cap from plan §3.25. Large repos
// hit ~1 file/row in the Files list, and 5 MB covers well over 50k entries
// even at generous path lengths, so this only bites on pathological input.
const diffMaxDecompressedBytes = 5 * 1024 * 1024

// IndexDiffController handles POST /api/v1/index/diff, the preflight
// endpoint of a streaming sync. The implementation deliberately bypasses
// apicontext.Context.Ok / BadRequest (which wrap the response with a
// {"success":...} envelope) and uses raw c.JSON so the DiffResponse /
// error shapes match the documented streaming schema byte-for-byte.
type IndexDiffController struct {
	logger  *zap.Logger
	service services.SyncSessionService
}

func NewIndexDiffController(logger *zap.Logger, service services.SyncSessionService) *IndexDiffController {
	return &IndexDiffController{
		logger:  logger.Named("controllers.index_diff"),
		service: service,
	}
}

// Diff is the HTTP handler for POST /api/v1/index/diff. Flow:
//
//  1. Require Content-Encoding: gzip (the header IS the contract, not
//     magic-byte sniffing). Missing → 415.
//  2. Decompress with a 5 MB cap → 400 on overflow or malformed input.
//  3. json.Unmarshal into DiffRequest. NOTE: we cannot use c.BindJSON
//     here because it reads the raw request body directly and bypasses
//     the gzip reader.
//  4. Validate required identity fields + the
//     "full-mode must not carry deletes" invariant.
//  5. Delegate to SyncSessionService.Diff and map errors to HTTP:
//     - ErrConcurrentSyncInProgress → 429 + Retry-After: 5
//     - anything else → 500 internal_error
func (ctrl *IndexDiffController) Diff(c *apicontext.Context) {
	if !strings.EqualFold(c.GetHeader("Content-Encoding"), "gzip") {
		c.JSON(http.StatusUnsupportedMediaType, gin.H{"error": "gzip_required"})
		return
	}

	body, err := decompressGzipBody(c.Request.Body, diffMaxDecompressedBytes)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "malformed_body",
			"details": err.Error(),
		})
		return
	}

	var req dto.DiffRequest
	if err := json.Unmarshal(body, &req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "malformed_body",
			"details": err.Error(),
		})
		return
	}

	// workspace_id 0 is a valid identity in local single-user mode; only the
	// string fields are genuinely required.
	if req.UserID == "" || req.MachineID == "" || req.RepoPath == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing_identity_or_repo_path"})
		return
	}

	// Full-mode requests carrying Deletes are a client bug — fail loud
	// rather than silently discarding the field (plan §3.8 v3.4).
	if !req.Incremental && len(req.Deletes) > 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "deletes_only_allowed_in_incremental"})
		return
	}

	// Per-file validation. gin's binding:"required" tags on DiffFileHash
	// are bypassed by the manual json.Unmarshal path above (gin only
	// enforces them via c.BindJSON, which cannot read the gzip body). An
	// empty path or sha256 would otherwise slip into clientHashes and
	// wedge the affected file for the whole session, or collide on the
	// "" key when multiple empty-path entries are present.
	for i, f := range req.Files {
		if f.Path == "" || f.SHA256 == "" {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "invalid_file_entry",
				"index": i,
			})
			return
		}
	}

	clientHashes := make(map[string]string, len(req.Files))
	for _, f := range req.Files {
		clientHashes[f.Path] = f.SHA256
	}

	result, err := ctrl.service.Diff(c.Request.Context(), &services.DiffParams{
		UserID:          req.UserID,
		WorkspaceID:     req.WorkspaceID,
		MachineID:       req.MachineID,
		RepoPath:        req.RepoPath,
		GithubOrgID:     req.GithubOrgID,
		ClientHashes:    clientHashes,
		Incremental:     req.Incremental,
		ExplicitDeletes: req.Deletes,
	})
	if err != nil {
		if errors.Is(err, repositories.ErrConcurrentSyncInProgress) {
			c.Header("Retry-After", "5")
			c.JSON(http.StatusTooManyRequests, gin.H{"error": "sync_in_progress"})
			return
		}
		ctrl.logger.Error("diff failed",
			zap.Error(err),
			zap.String("user_id", req.UserID),
			zap.Uint64("workspace_id", req.WorkspaceID),
			zap.String("machine_id", req.MachineID),
			zap.String("repo_path", req.RepoPath),
		)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal_error"})
		return
	}

	// Defensive non-nil normalisation. The service already guarantees
	// this, but JSON null in the response would force every client to
	// special-case it — explicit is cheap.
	need := result.Need
	if need == nil {
		need = []string{}
	}
	del := result.Delete
	if del == nil {
		del = []string{}
	}

	c.JSON(http.StatusOK, dto.DiffResponse{
		SyncID: result.SyncID,
		Need:   need,
		Delete: del,
	})
}
