package controllers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"resty.dev/v3"

	"github.com/TransformerOptimus/SuperCoder/services/gateway/models/dto"
	"github.com/TransformerOptimus/SuperCoder/services/gateway/services"
)

// CompletionsController handles POST /v1/chat/completions.
type CompletionsController struct {
	router *services.Router
	client *resty.Client
	logger *zap.Logger
}

func NewCompletionsController(router *services.Router, logger *zap.Logger) *CompletionsController {
	client := resty.New().
		SetDoNotParseResponse(true)

	return &CompletionsController{
		router: router,
		client: client,
		logger: logger.Named("controllers.completions"),
	}
}

func (ctrl *CompletionsController) HandleCompletion(c *gin.Context) {
	var req dto.ChatCompletionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{
			Error: dto.ErrorDetail{
				Message: "invalid request body: " + err.Error(),
				Type:    "invalid_request_error",
			},
		})
		return
	}

	if req.Model == "" {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{
			Error: dto.ErrorDetail{
				Message: "model is required",
				Type:    "invalid_request_error",
			},
		})
		return
	}

	// Route to provider adapter
	providerOverride := c.GetHeader("X-Provider")
	adapter, err := ctrl.router.Route(req.Model, providerOverride)
	if err != nil {
		ctrl.logger.Error("routing failed", zap.Error(err), zap.String("model", req.Model))
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{
			Error: dto.ErrorDetail{
				Message: err.Error(),
				Type:    "invalid_request_error",
			},
		})
		return
	}

	// Resolve API key: server-side config takes priority, then caller header
	apiKey := adapter.ConfiguredAPIKey()
	if apiKey == "" {
		apiKey = extractAPIKey(c.GetHeader("Authorization"))
	}
	if apiKey == "" {
		c.JSON(http.StatusUnauthorized, dto.ErrorResponse{
			Error: dto.ErrorDetail{
				Message: "missing API key: provide Authorization header or configure server-side key",
				Type:    "authentication_error",
			},
		})
		return
	}

	ctrl.logger.Info("gateway request",
		zap.String("model", req.Model),
		zap.String("provider", adapter.Name()),
		zap.String("user_id", c.GetHeader("X-USER-ID")),
		zap.String("workspace_id", c.GetHeader("X-Workspace-ID")),
	)

	// Translate request
	url, headers, body, err := adapter.TranslateRequest(&req, apiKey)
	if err != nil {
		ctrl.logger.Error("translation failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{
			Error: dto.ErrorDetail{
				Message: "failed to translate request: " + err.Error(),
				Type:    "server_error",
			},
		})
		return
	}

	// Make provider request
	providerReq := ctrl.client.R().
		SetContext(c.Request.Context()).
		SetBody(body)

	for k, v := range headers {
		providerReq.SetHeader(k, v)
	}

	resp, err := providerReq.Post(url)
	if err != nil {
		ctrl.logger.Error("provider request failed", zap.Error(err), zap.String("url", sanitizeURL(url)))
		c.JSON(http.StatusBadGateway, dto.ErrorResponse{
			Error: dto.ErrorDetail{
				Message: "provider request failed: " + err.Error(),
				Type:    "server_error",
			},
		})
		return
	}
	defer resp.Body.Close()

	// Check for error status
	if resp.StatusCode() >= 400 {
		ctrl.handleProviderError(c, resp)
		return
	}

	// Stream SSE response
	ctrl.streamResponse(c, adapter, resp)
}

func (ctrl *CompletionsController) handleProviderError(c *gin.Context, resp *resty.Response) {
	var errBody json.RawMessage
	if err := json.NewDecoder(resp.Body).Decode(&errBody); err != nil {
		c.JSON(resp.StatusCode(), dto.ErrorResponse{
			Error: dto.ErrorDetail{
				Message: fmt.Sprintf("provider returned status %d", resp.StatusCode()),
				Type:    "server_error",
			},
		})
		return
	}

	c.Data(resp.StatusCode(), "application/json", errBody)
}

func (ctrl *CompletionsController) streamResponse(c *gin.Context, adapter services.ProviderAdapter, resp *resty.Response) {
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")
	c.Status(http.StatusOK)

	chunks := make(chan *dto.ChatCompletionChunk, 32)

	go func() {
		defer func() {
			if r := recover(); r != nil {
				ctrl.logger.Error("stream goroutine panic", zap.Any("panic", r))
			}
		}()
		if err := adapter.TranslateStream(c.Request.Context(), resp.Body, chunks); err != nil {
			if !strings.Contains(err.Error(), "response body closed") {
				ctrl.logger.Error("stream translation error", zap.Error(err))
			}
		}
	}()

	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

loop:
	for {
		select {
		case chunk, ok := <-chunks:
			if !ok {
				break loop
			}
			if chunk == nil {
				fmt.Fprint(c.Writer, "data: [DONE]\n\n")
				c.Writer.Flush()
				break loop
			}
			if chunk.Object == "ping" {
				fmt.Fprint(c.Writer, ": ping\n\n")
				c.Writer.Flush()
				ticker.Reset(15 * time.Second)
				continue
			}

			data, err := json.Marshal(chunk)
			if err != nil {
				ctrl.logger.Error("failed to marshal chunk", zap.Error(err))
				break loop
			}

			fmt.Fprintf(c.Writer, "data: %s\n\n", data)
			c.Writer.Flush()
			ticker.Reset(15 * time.Second)
		case <-ticker.C:
			fmt.Fprint(c.Writer, ": keepalive\n\n")
			c.Writer.Flush()
		}
	}
}

func extractAPIKey(header string) string {
	if strings.HasPrefix(header, "Bearer ") {
		return strings.TrimPrefix(header, "Bearer ")
	}
	return header
}

// sanitizeURL strips query params and fragment from a URL before logging
// to avoid leaking auth tokens or sensitive parameters.
func sanitizeURL(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "<invalid-url>"
	}
	u.RawQuery = ""
	u.Fragment = ""
	return u.String()
}
