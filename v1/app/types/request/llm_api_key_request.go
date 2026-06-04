package request

type CreateLLMAPIKeyRequest struct {
	APIKeys []LLMAPIKey `json:"api_keys" binding:"required,dive,required"`
}

type LLMAPIKey struct {
	LLMModel  string  `json:"llm_model" binding:"required"`
	LLMAPIKey *string `json:"llm_api_key"`
}
