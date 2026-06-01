package dto

// ModelInfo represents a single model in the GET /v1/models response.
type ModelInfo struct {
	ID             string `json:"id"`
	DisplayName    string `json:"display_name"`
	Provider       string `json:"provider"`
	ContextWindow  int    `json:"context_window"`
	SupportsImages bool   `json:"supports_images"`
}

// ModelsResponse is the response body for GET /v1/models.
type ModelsResponse struct {
	Models []ModelInfo `json:"models"`
}
