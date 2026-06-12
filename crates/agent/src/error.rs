#[derive(Debug, thiserror::Error)]
pub enum AgentError {
    #[error("LLM API error (status {status}): {body}")]
    LlmApiError { status: u16, body: String },

    #[error("LLM parse error: {0}")]
    LlmParseError(String),

    #[error("Agent cancelled")]
    Cancelled,

    #[error("SSE chunk timeout: no data received for {0}s")]
    ChunkTimeout(u64),

    #[error("LLM header timeout: response headers did not arrive within {0}ms")]
    HeaderTimeout(u64),

    #[error("HTTP error: {0}")]
    HttpError(#[from] reqwest::Error),

    #[error("JSON error: {0}")]
    JsonError(#[from] serde_json::Error),

    #[error("IO error: {0}")]
    IoError(#[from] std::io::Error),
}

/// Tool-level error that gets converted to an error result the LLM can see.
#[derive(Debug, thiserror::Error)]
#[error("{0}")]
pub struct ToolError(pub String);
