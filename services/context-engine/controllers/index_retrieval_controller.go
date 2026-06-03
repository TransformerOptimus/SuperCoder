package controllers

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	apicontext "github.com/TransformerOptimus/SuperCoder/services/context-engine/internal/pkg/api/context"
	"github.com/TransformerOptimus/SuperCoder/services/context-engine/models/dto"
	"github.com/TransformerOptimus/SuperCoder/services/context-engine/services"
)

type IndexRetrievalController struct {
	logger  *zap.Logger
	service services.IndexRetrievalService
}

func NewIndexRetrievalController(
	logger *zap.Logger,
	service services.IndexRetrievalService,
) *IndexRetrievalController {
	return &IndexRetrievalController{
		logger:  logger.Named("controllers.index_retrieval"),
		service: service,
	}
}

func (ctrl *IndexRetrievalController) Search(c *apicontext.Context) {
	var req dto.SearchRequest
	if err := c.BindJSON(&req); err != nil {
		c.BadRequest(err)
		return
	}

	resp, err := ctrl.service.Search(c.Request.Context(), &req)
	if err != nil {
		ctrl.logger.Error("Search failed", zap.Error(err))
		c.InternalServerError(err)
		return
	}

	c.JSON(http.StatusOK, resp)
}

func (ctrl *IndexRetrievalController) GraphQuery(c *apicontext.Context) {
	var req dto.GraphQueryRequest
	if err := c.BindJSON(&req); err != nil {
		c.BadRequest(err)
		return
	}

	resp, err := ctrl.service.GraphQuery(c.Request.Context(), &req)
	if err != nil {
		ctrl.logger.Error("Graph query failed", zap.Error(err))
		c.InternalServerError(err)
		return
	}

	c.JSON(http.StatusOK, resp)
}

func (ctrl *IndexRetrievalController) GetContext(c *apicontext.Context) {
	var req dto.ContextRequest
	if err := c.BindJSON(&req); err != nil {
		c.BadRequest(err)
		return
	}

	resp, err := ctrl.service.GetContext(c.Request.Context(), &req)
	if err != nil {
		ctrl.logger.Error("GetContext failed", zap.Error(err))
		c.InternalServerError(err)
		return
	}

	c.JSON(http.StatusOK, resp)
}

func (ctrl *IndexRetrievalController) TriggerIndex(c *apicontext.Context) {
	var req dto.IndexRequest
	if err := c.BindJSON(&req); err != nil {
		c.BadRequest(err)
		return
	}

	taskID, err := ctrl.service.TriggerIndex(c.Request.Context(), &req)
	if err != nil {
		ctrl.logger.Error("Failed to trigger index", zap.Error(err))
		c.InternalServerError(err)
		return
	}

	c.JSON(http.StatusAccepted, gin.H{
		"task_id": taskID,
		"status":  "queued",
	})
}

func (ctrl *IndexRetrievalController) GetIndexStatus(c *apicontext.Context) {
	repoPath := c.Query("repo_path")
	if repoPath == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "repo_path is required"})
		return
	}

	userID := c.Query("user_id")
	machineID := c.Query("machine_id")

	var workspaceID uint64
	if ws := c.Query("workspace_id"); ws != "" {
		workspaceID, _ = strconv.ParseUint(ws, 10, 64)
	}

	resp, err := ctrl.service.GetIndexStatus(c.Request.Context(), repoPath, userID, workspaceID, machineID)
	if err != nil {
		ctrl.logger.Error("GetIndexStatus failed", zap.Error(err))
		c.InternalServerError(err)
		return
	}

	c.JSON(http.StatusOK, resp)
}

func (ctrl *IndexRetrievalController) DeleteIndex(c *apicontext.Context) {
	var req dto.IndexDeleteRequest
	if err := c.BindJSON(&req); err != nil {
		c.BadRequest(err)
		return
	}

	resp, err := ctrl.service.DeleteIndex(c.Request.Context(), &req)
	if err != nil {
		ctrl.logger.Error("DeleteIndex failed", zap.Error(err))
		c.InternalServerError(err)
		return
	}

	c.JSON(http.StatusOK, resp)
}
