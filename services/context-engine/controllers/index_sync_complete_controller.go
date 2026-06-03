package controllers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	apicontext "github.com/TransformerOptimus/SuperCoder/services/context-engine/internal/pkg/api/context"
	"github.com/TransformerOptimus/SuperCoder/services/context-engine/models/dto"
	"github.com/TransformerOptimus/SuperCoder/services/context-engine/services"
)

// IndexSyncCompleteController handles POST /api/v1/index/sync-complete.
// It is a pure read — no locks, no writes — and maps the session status
// to an HTTP code per the streaming-architecture plan:
//
//	receiving   -> 409 Conflict
//	processing  -> 202 Accepted
//	finalizing  -> 202 Accepted
//	done        -> 200 OK
//	failed      -> 410 Gone (with reason)
//	expired     -> 410 Gone
//	not-found   -> 410 Gone (sync_not_found)
//
// Like the /diff controller, this one deliberately bypasses
// apicontext.Ok/BadRequest so the response shape matches the documented
// SyncCompleteResponse schema with no injected envelope fields.
type IndexSyncCompleteController struct {
	logger  *zap.Logger
	service services.SyncSessionService
}

func NewIndexSyncCompleteController(logger *zap.Logger, service services.SyncSessionService) *IndexSyncCompleteController {
	return &IndexSyncCompleteController{
		logger:  logger.Named("controllers.index_sync_complete"),
		service: service,
	}
}

func (ctrl *IndexSyncCompleteController) SyncComplete(c *apicontext.Context) {
	var req dto.SyncCompleteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "malformed_body"})
		return
	}

	snap, err := ctrl.service.GetStatus(c.Request.Context(), req.SyncID)
	if err != nil {
		ctrl.logger.Error("get sync status failed",
			zap.Error(err),
			zap.String("sync_id", req.SyncID),
		)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal_error"})
		return
	}
	if snap == nil {
		c.JSON(http.StatusGone, dto.SyncCompleteResponse{
			Status: "expired",
			Error:  "sync_not_found",
		})
		return
	}

	switch snap.Status {
	case "receiving":
		c.JSON(http.StatusConflict, dto.SyncCompleteResponse{
			Status:  "receiving",
			Files:   receivedPair(snap.FileCounts),
			Deletes: receivedPair(snap.DeleteCounts),
		})
	case "processing":
		c.JSON(http.StatusAccepted, dto.SyncCompleteResponse{
			Status:  "processing",
			Files:   processedPair(snap.FileCounts),
			Deletes: processedPair(snap.DeleteCounts),
		})
	case "finalizing":
		c.JSON(http.StatusAccepted, dto.SyncCompleteResponse{Status: "finalizing"})
	case "done":
		c.JSON(http.StatusOK, dto.SyncCompleteResponse{
			Status:  "done",
			Files:   finalPair(snap.FileCounts),
			Deletes: finalPair(snap.DeleteCounts),
		})
	case "failed":
		// Reason may legitimately be "" — omitempty drops the field in
		// that case, which is the schema we want.
		c.JSON(http.StatusGone, dto.SyncCompleteResponse{
			Status: "failed",
			Error:  "sync_failed",
			Reason: snap.FailedReason,
		})
	case "expired":
		c.JSON(http.StatusGone, dto.SyncCompleteResponse{
			Status: "expired",
			Error:  "sync_expired",
		})
	default:
		ctrl.logger.Error("unknown sync status",
			zap.String("status", snap.Status),
			zap.String("sync_id", req.SyncID),
		)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal_error"})
	}
}

func receivedPair(c services.Counts) dto.CountPair {
	return dto.CountPair{Expected: c.Expected, Received: c.Received}
}

func processedPair(c services.Counts) dto.CountPair {
	return dto.CountPair{
		Expected:  c.Expected,
		Received:  c.Received,
		Processed: c.Processed,
	}
}

func finalPair(c services.Counts) dto.CountPair {
	return dto.CountPair{
		Expected:  c.Expected,
		Processed: c.Processed,
		Failed:    c.Failed,
	}
}
