export interface LLMAPIKey {
  llm_model: string;
  llm_api_key: string;
}

export interface CreateOrUpdateLLMAPIKeyPayload {
  organisation_id: number;
  api_keys: LLMAPIKey[];
}
