package llm_api_key

type LLMAPIKeyReturn struct {
	ModelName string `json:"model_name"`
	APIKey    string `json:"api_key"`
}
