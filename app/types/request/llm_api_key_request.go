package request

type CreateLLMAPIKeyRequest struct {
	LLMModel  string `json:"llm_model" binding:"required"`
	LLMAPIKey string `json:"llm_api_key" binding:"required"`
}
