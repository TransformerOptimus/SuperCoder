use std::collections::VecDeque;
use std::sync::Mutex;

use tokio::sync::mpsc;

use crate::error::AgentError;
use crate::llm::client::LlmProvider;
use crate::llm::types::{ChatMessage, LlmResponse, ToolDefinition};
use crate::types::AgentEvent;

/// Mock LLM that returns pre-queued responses in order.
pub struct MockLlm {
    pub responses: Mutex<VecDeque<Result<LlmResponse, AgentError>>>,
}

impl MockLlm {
    pub fn new(responses: Vec<Result<LlmResponse, AgentError>>) -> Self {
        Self {
            responses: Mutex::new(VecDeque::from(responses)),
        }
    }
}

#[async_trait::async_trait]
impl LlmProvider for MockLlm {
    async fn chat_completion(
        &self,
        _messages: &[ChatMessage],
        _tools: &[ToolDefinition],
        _event_tx: &mpsc::Sender<AgentEvent>,
        _session_id: &str,
        _cancel_token: Option<&tokio_util::sync::CancellationToken>,
    ) -> Result<LlmResponse, AgentError> {
        self.responses
            .lock()
            .unwrap()
            .pop_front()
            .expect("MockLlm: no more responses queued")
    }
}

/// Create a simple text-only LLM response.
pub fn text_response(text: &str) -> LlmResponse {
    LlmResponse {
        content: Some(text.to_string()),
        tool_calls: vec![],
        usage: None,
        finish_reason: Some("stop".to_string()),
        thinking: None,
    }
}

/// Create a text response with specific token usage (for compaction tests).
pub fn text_response_with_usage(text: &str, total_tokens: u32) -> LlmResponse {
    LlmResponse {
        content: Some(text.to_string()),
        tool_calls: vec![],
        usage: Some(crate::llm::types::Usage {
            prompt_tokens: total_tokens.saturating_sub(10),
            completion_tokens: 10,
            total_tokens,
            prompt_tokens_details: None,
        }),
        finish_reason: Some("stop".to_string()),
        thinking: None,
    }
}
