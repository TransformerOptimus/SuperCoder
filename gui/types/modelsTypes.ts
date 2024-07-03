export interface CreateOrUpdateLLMAPIKeyPayload {
  organisation_id: number;
  llm_model: string;
  llm_api_key: string;
}
