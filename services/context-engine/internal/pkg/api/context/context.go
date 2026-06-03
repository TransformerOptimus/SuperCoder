package api_context

import (
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/TransformerOptimus/SuperCoder/services/context-engine/internal/pkg/api/responses"
	"github.com/TransformerOptimus/SuperCoder/services/context-engine/internal/pkg/utils"
)

type Context struct {
	*gin.Context
}

type SortOption struct {
	Field     string
	Direction string
}

func (c Context) GetUserId() (*uint, error) {
	userId, exists := c.Get("userId")
	if exists {
		if userIdUint, ok := userId.(uint); ok {
			return &userIdUint, nil
		}
	}

	userIdHeader := c.Request.Header.Get("X-USER-ID")

	if userIdHeader != "" {
		userId, err := strconv.ParseUint(userIdHeader, 10, 64)
		userIdUint := uint(userId)
		if err == nil {
			return &userIdUint, nil
		}
	}

	userIdQueryParam := c.Query("userId")
	if userIdQueryParam != "" {
		userId, err := strconv.ParseUint(userIdQueryParam, 10, 64)
		userIdUint := uint(userId)
		if err == nil {
			return &userIdUint, nil
		}
	}

	return nil, errors.New("userId not found")
}

func (c Context) IsAdmin() bool {
	userIdHeader := c.Request.Header.Get("X-USER-ID")
	return userIdHeader == "-555" || userIdHeader == "0" || userIdHeader == "-999"
}

func (c Context) GetUserIdInt64() (int64, error) {
	if userId, exists := c.Get("userId"); exists {
		if userIdUint, ok := userId.(uint); ok {
			return int64(userIdUint), nil
		}
		if userIdInt64, ok := userId.(int64); ok {
			return userIdInt64, nil
		}
	}

	userIdHeader := c.Request.Header.Get("X-USER-ID")
	if userIdHeader != "" {
		if id, err := strconv.ParseInt(userIdHeader, 10, 64); err == nil {
			return id, nil
		}
	}

	return 0, errors.New("userId not found")
}

func (c Context) GetWorkspaceId() (*uint, error) {
	workspaceId, exists := c.Get("workspaceId")
	if exists {
		if workspaceIdUint, ok := workspaceId.(uint); ok {
			return &workspaceIdUint, nil
		}
	}

	workspaceIdHeader := c.Request.Header.Get("X-WORKSPACE-ID")
	if workspaceIdHeader != "" {
		workspaceId, err := strconv.ParseUint(workspaceIdHeader, 10, 64)
		workspaceIdUint := uint(workspaceId)
		if err == nil {
			return &workspaceIdUint, nil
		}
	}

	workspaceIdQueryParam := c.Query("workspaceId")
	if workspaceIdQueryParam != "" {
		workspaceId, err := strconv.ParseUint(workspaceIdQueryParam, 10, 64)
		workspaceIdUint := uint(workspaceId)
		if err == nil {
			return &workspaceIdUint, nil
		}
	}

	workspaceIdQueryParam = c.Query("current_workspace_id")
	if workspaceIdQueryParam != "" {
		workspaceId, err := strconv.ParseUint(workspaceIdQueryParam, 10, 64)
		workspaceIdUint := uint(workspaceId)
		if err == nil {
			return &workspaceIdUint, nil
		}
	}

	return nil, errors.New("workspaceId not found")
}

// allowedSourceServices bounds what can be written into the inference_logs
// LowCardinality(String) column. Unknown values are silently dropped to prevent
// unbounded dictionary growth and audit/billing attribution spoofing from
// caller-controlled headers. Add new services here as they start calling the
// inference router.
var allowedSourceServices = map[string]struct{}{
	"supermind": {},
}

func (c Context) GetSourceService() string {
	v := c.Request.Header.Get("X-Source-Service")
	if _, ok := allowedSourceServices[v]; !ok {
		return ""
	}
	return v
}

func (c Context) GetBillingVersion() string {
	// Check headers
	for _, key := range []string{"X-BILLING-VERSION", "X-Billing-Version"} {
		if v := c.Request.Header.Get(key); v != "" {
			return v
		}
	}

	// Check query params
	for _, key := range []string{"billingVersion", "billing_version"} {
		if v := c.Query(key); v != "" {
			return v
		}
	}

	return "v1"
}

func (c Context) Ok(payloads ...gin.H) {
	c.success(200, payloads...)
}

func (c Context) Created(payloads ...gin.H) {
	c.success(201, payloads...)
}

func (c Context) BadRequest(err error, payloads ...gin.H) {
	c.failure(400, err, payloads...)
}

func (c Context) BadRequestParam(param string, err error) {
	c.AbortWithStatusJSON(400, responses.NewBadRequestParamResponse(param, err))
}

func (c Context) BadRequestHeader(header string, err error) {
	c.AbortWithStatusJSON(400, responses.NewBadRequestHeaderResponse(header, err))
}

func (c Context) StatusConflict(err error) {
	c.AbortWithStatusJSON(409, responses.NewConflictResponse(err))
}

func (c Context) InternalServerError(err error) {
	c.AbortWithStatusJSON(500, responses.NewInternalServerErrorResponse(err))
}

func (c Context) NotImplemented() {
	c.AbortWithStatusJSON(501, responses.NewNotImplementedResponse())
}

func (c Context) NotFound(err error) {
	c.AbortWithStatusJSON(404, responses.NewNotFoundResponse(err))
}

func (c Context) Forbidden(err error) {
	c.AbortWithStatusJSON(403, responses.NewForbiddenResponse(err))
}

func (c Context) failure(status int, err error, payloads ...gin.H) {
	merge := gin.H{}
	for _, payload := range payloads {
		for k, v := range payload {
			merge[k] = v
		}
	}

	merge["error"] = err.Error()
	merge["success"] = false
	c.AbortWithStatusJSON(status, merge)
}

func (c Context) success(status int, payloads ...gin.H) {
	merge := gin.H{}
	for _, payload := range payloads {
		for k, v := range payload {
			merge[k] = v
		}
	}
	merge["success"] = true
	c.JSON(status, merge)
}

func (c Context) GetSort() (sortOptions *SortOption) {
	sort := c.Query("sort")
	isAsc := strings.HasSuffix(sort, "_asc")
	isDesc := strings.HasSuffix(sort, "_desc")
	field := strings.TrimSuffix(strings.TrimSuffix(sort, "_asc"), "_desc")
	if isAsc {
		return &SortOption{Field: field, Direction: "asc"}
	} else if isDesc {
		return &SortOption{Field: field, Direction: "desc"}
	} else {
		return nil
	}
}

func (c Context) GetOptionalQueryString(key string) *string {
	value := c.Query(key)
	if value == "" {
		return nil
	}
	return &value
}

func (c Context) GetOptionalQueryStringSlice(key string) *[]string {
	value := c.QueryArray(key)
	if len(value) == 0 {
		return nil
	} else {
		return &value
	}
}

func (c Context) GetOptionalQueryUintSlice(key string) *[]uint {
	value := c.QueryArray(key)
	uintValues := make([]uint, 0, len(value))
	for _, v := range value {
		uintValue, err := strconv.ParseUint(v, 10, 64)
		if err == nil {
			uintValues = append(uintValues, uint(uintValue))
		}
	}
	if len(uintValues) == 0 {
		return nil
	} else {
		return &uintValues
	}
}

func (c Context) GetOptionalQueryUint64(key string) *uint64 {
	value := c.Query(key)
	if value == "" {
		return nil
	}
	uintValue, err := strconv.ParseUint(value, 10, 64)
	if err != nil {
		return nil
	}
	return &uintValue
}

func (c Context) GetOptionalQueryUint(key string) *uint {
	value := c.Query(key)
	if value == "" {
		return nil
	}
	uintValue64, err := strconv.ParseUint(value, 10, 64)
	if err != nil {
		return nil
	}
	uintValue := uint(uintValue64)
	return &uintValue
}

func (c Context) GetOptionalQueryTime(key string) *time.Time {
	value := c.Query(key)
	if value == "" {
		return nil
	}
	timeValue, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return nil
	}
	return &timeValue
}

func (c Context) GetOptionalQueryUUID(key string) *uuid.UUID {
	value := c.Query(key)
	if value == "" {
		return nil
	}
	uuidValue, err := uuid.Parse(value)
	if err != nil {
		return nil
	}
	return &uuidValue
}

func (c Context) GetUintQueryParam(key string) (*uint, error) {
	value := c.Query(key)
	uintValue64, err := strconv.ParseUint(value, 10, 64)
	if err != nil {
		return nil, err
	}
	uintValue := uint(uintValue64)
	return &uintValue, nil
}

func (c Context) Page() uint {
	page, err := c.GetUintQueryParam("page")
	if err != nil {
		return 0
	} else {
		return *page
	}
}

func (c Context) PageWithDefault(defaultValue int64) int64 {
	page, err := strconv.ParseInt(c.DefaultQuery("page", strconv.Itoa(int(defaultValue))), 10, 64)
	if err != nil {
		return defaultValue
	} else {
		return page
	}
}

func (c Context) PageSizeWithDefault(defaultValue int64) int64 {
	size, err := strconv.ParseInt(c.DefaultQuery("page_size", strconv.Itoa(int(defaultValue))), 10, 64)
	if err != nil {
		return defaultValue
	} else {
		return size
	}
}

func (c Context) Size() uint {
	size, err := c.GetUintQueryParam("size")
	if err != nil {
		return 10
	} else {
		return *size
	}
}

func (c Context) GetStringParam(param string) (*string, error) {
	value := c.Param(param)
	if value == "" {
		return nil, errors.New("param not found")
	}
	return &value, nil
}

func (c Context) GetUintParam(param string) (*uint, error) {
	value := c.Param(param)
	uintValue, err := strconv.ParseUint(value, 10, 64)
	if err != nil {
		return nil, err
	}
	return utils.UintPtr(uint(uintValue)), nil
}

func WithApiContext(appHandler func(ctx *Context)) func(ctx *gin.Context) {
	return func(c *gin.Context) {
		appHandler(&Context{c})
	}
}
