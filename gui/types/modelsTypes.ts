export interface LLMAPIKey {
  llm_model: string;
  llm_api_key: string;
}

export interface CreateOrUpdateLLMAPIKeyPayload {
  api_keys: LLMAPIKey[];
}
