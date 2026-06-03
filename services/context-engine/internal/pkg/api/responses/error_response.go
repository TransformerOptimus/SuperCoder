package responses

// ErrorResponse is the standard error response structure for all API failures.
type ErrorResponse struct {
	Success bool   `json:"success"`
	Error   string `json:"error"`
	Message string `json:"message,omitempty"`
}

// BadRequestResponse represents a 400 Bad Request response.
type BadRequestResponse struct {
	Success bool   `json:"success"`
	Error   string `json:"error"`
	Message string `json:"message,omitempty"`
}

// BadRequestParamResponse represents a 400 Bad Request response for invalid parameters.
type BadRequestParamResponse struct {
	Success bool   `json:"success"`
	Error   string `json:"error"`
	Message string `json:"message"` // Format: "Invalid param {paramName}"
}

// BadRequestHeaderResponse represents a 400 Bad Request response for invalid headers.
type BadRequestHeaderResponse struct {
	Success bool   `json:"success"`
	Error   string `json:"error"`
	Message string `json:"message"` // Format: "Invalid header {headerName}"
}

// NotFoundResponse represents a 404 Not Found response.
type NotFoundResponse struct {
	Success bool   `json:"success"`
	Error   string `json:"error"`
}

// ForbiddenResponse represents a 403 Forbidden response.
type ForbiddenResponse struct {
	Success bool   `json:"success"`
	Error   string `json:"error"`
}

// ConflictResponse represents a 409 Conflict response.
type ConflictResponse struct {
	Success bool   `json:"success"`
	Error   string `json:"error"`
}

// InternalServerErrorResponse represents a 500 Internal Server Error response.
type InternalServerErrorResponse struct {
	Success bool   `json:"success"`
	Error   string `json:"error"`
}

// NotImplementedResponse represents a 501 Not Implemented response.
type NotImplementedResponse struct {
	Success bool   `json:"success"`
	Error   string `json:"error"` // Always "api not implemented"
}

// NewErrorResponse creates a new ErrorResponse.
func NewErrorResponse(err error) ErrorResponse {
	return ErrorResponse{
		Success: false,
		Error:   err.Error(),
	}
}

// NewErrorResponseWithMessage creates a new ErrorResponse with a custom message.
func NewErrorResponseWithMessage(err error, message string) ErrorResponse {
	return ErrorResponse{
		Success: false,
		Error:   err.Error(),
		Message: message,
	}
}

// NewBadRequestResponse creates a new BadRequestResponse.
func NewBadRequestResponse(err error) BadRequestResponse {
	return BadRequestResponse{
		Success: false,
		Error:   err.Error(),
	}
}

// NewBadRequestParamResponse creates a new BadRequestParamResponse for an invalid parameter.
func NewBadRequestParamResponse(param string, err error) BadRequestParamResponse {
	return BadRequestParamResponse{
		Success: false,
		Error:   err.Error(),
		Message: "Invalid param " + param,
	}
}

// NewBadRequestHeaderResponse creates a new BadRequestHeaderResponse for an invalid header.
func NewBadRequestHeaderResponse(header string, err error) BadRequestHeaderResponse {
	return BadRequestHeaderResponse{
		Success: false,
		Error:   err.Error(),
		Message: "Invalid header " + header,
	}
}

// NewNotFoundResponse creates a new NotFoundResponse.
func NewNotFoundResponse(err error) NotFoundResponse {
	return NotFoundResponse{
		Success: false,
		Error:   err.Error(),
	}
}

// NewForbiddenResponse creates a new ForbiddenResponse.
func NewForbiddenResponse(err error) ForbiddenResponse {
	return ForbiddenResponse{
		Success: false,
		Error:   err.Error(),
	}
}

// NewConflictResponse creates a new ConflictResponse.
func NewConflictResponse(err error) ConflictResponse {
	return ConflictResponse{
		Success: false,
		Error:   err.Error(),
	}
}

// NewInternalServerErrorResponse creates a new InternalServerErrorResponse.
func NewInternalServerErrorResponse(err error) InternalServerErrorResponse {
	return InternalServerErrorResponse{
		Success: false,
		Error:   err.Error(),
	}
}

// NewNotImplementedResponse creates a new NotImplementedResponse.
func NewNotImplementedResponse() NotImplementedResponse {
	return NotImplementedResponse{
		Success: false,
		Error:   "api not implemented",
	}
}
