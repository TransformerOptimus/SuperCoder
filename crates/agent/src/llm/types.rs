use serde::{Deserialize, Serialize};

// ── Prompt caching ──

/// Marks a block/tool/request as a prompt-cache breakpoint for Anthropic.
/// OpenAI ignores this field (their caching is automatic).
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct CacheControl {
    #[serde(rename = "type")]
    pub type_: String, // "ephemeral"
    #[serde(skip_serializing_if = "Option::is_none")]
    pub ttl: Option<String>, // "5m" (default) or "1h"
}

impl CacheControl {
    /// Ephemeral cache with 1h TTL. Anthropic's 5m default writes cheaper (1.25× base)
    /// but expires fast; 1h writes cost 2× base and survives typical user idle gaps.
    pub fn ephemeral() -> Self {
        Self { type_: "ephemeral".into(), ttl: Some("1h".into()) }
    }
}

// ── Request types ──

#[derive(Debug, Serialize)]
pub struct ChatCompletionRequest {
    pub model: String,
    pub messages: Vec<ChatMessage>,
    pub stream: bool,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub stream_options: Option<StreamOptions>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub tools: Option<Vec<ToolDefinition>>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub tool_choice: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub parallel_tool_calls: Option<bool>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub max_completion_tokens: Option<u32>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub temperature: Option<f32>,
    /// Extended thinking config (e.g., {"type": "enabled", "budget_tokens": 10000}).
    /// Gateway forwards to Anthropic, maps to reasoning for OpenAI.
    #[serde(skip_serializing_if = "Option::is_none")]
    pub thinking: Option<serde_json::Value>,
    /// OpenAI prompt-cache routing hint. Pins identical-prefix requests to the
    /// same cache machine for higher hit rate. Set to session id for OpenAI-family
    /// models; omitted for Anthropic (they use explicit cache_control markers).
    #[serde(skip_serializing_if = "Option::is_none")]
    pub prompt_cache_key: Option<String>,
    /// Top-level cache_control enables Anthropic's automatic conversation-level
    /// breakpoint (advances to the last cacheable block each turn). Set only
    /// for claude-* models.
    #[serde(skip_serializing_if = "Option::is_none")]
    pub cache_control: Option<CacheControl>,
}

#[derive(Debug, Serialize)]
pub struct StreamOptions {
    pub include_usage: bool,
}

// ── Content block types for multi-modal messages ──

#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(tag = "type")]
pub enum ContentBlock {
    #[serde(rename = "text")]
    Text {
        text: String,
        #[serde(default, skip_serializing_if = "Option::is_none")]
        cache_control: Option<CacheControl>,
    },

    #[serde(rename = "image_url")]
    ImageUrl { image_url: ImageUrlContent },
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ImageUrlContent {
    pub url: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub detail: Option<String>,
}

/// Message content — either a plain string or an array of content blocks.
/// Uses `#[serde(untagged)]` so `"hello"` → `Text("hello")` and `[{...}]` → `Blocks(...)`.
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(untagged)]
pub enum MessageContent {
    Text(String),
    Blocks(Vec<ContentBlock>),
}

impl MessageContent {
    /// Extract the text content. For blocks, returns the first text block's content.
    pub fn text(&self) -> &str {
        match self {
            MessageContent::Text(s) => s,
            MessageContent::Blocks(blocks) => {
                blocks.iter().find_map(|b| match b {
                    ContentBlock::Text { text, .. } => Some(text.as_str()),
                    _ => None,
                }).unwrap_or("")
            }
        }
    }

    pub fn has_images(&self) -> bool {
        match self {
            MessageContent::Text(_) => false,
            MessageContent::Blocks(blocks) => blocks.iter().any(|b| matches!(b, ContentBlock::ImageUrl { .. })),
        }
    }
}

impl Default for MessageContent {
    fn default() -> Self {
        MessageContent::Text(String::new())
    }
}

// ── Message types (flat struct for serde compatibility) ──

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ChatMessage {
    pub role: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub content: Option<MessageContent>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub tool_calls: Option<Vec<ToolCall>>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub tool_call_id: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub name: Option<String>,
    /// Thinking text from Anthropic extended thinking (simple string).
    /// Preserved for multi-turn round-tripping — gateway reconstructs
    /// Anthropic's structured thinking blocks from this string.
    #[serde(skip_serializing_if = "Option::is_none")]
    pub thinking: Option<String>,
}

impl ChatMessage {
    pub fn system(content: impl Into<String>) -> Self {
        Self {
            role: "system".into(),
            content: Some(MessageContent::Text(content.into())),
            tool_calls: None,
            tool_call_id: None,
            name: None,
            thinking: None,
        }
    }

    pub fn user(content: impl Into<String>) -> Self {
        Self {
            role: "user".into(),
            content: Some(MessageContent::Text(content.into())),
            tool_calls: None,
            tool_call_id: None,
            name: None,
            thinking: None,
        }
    }

    pub fn user_with_images(blocks: Vec<ContentBlock>) -> Self {
        Self {
            role: "user".into(),
            content: Some(MessageContent::Blocks(blocks)),
            tool_calls: None,
            tool_call_id: None,
            name: None,
            thinking: None,
        }
    }

    pub fn assistant(
        content: Option<String>,
        tool_calls: Option<Vec<ToolCall>>,
        thinking: Option<String>,
    ) -> Self {
        Self {
            role: "assistant".into(),
            content: content.map(MessageContent::Text),
            tool_calls,
            tool_call_id: None,
            name: None,
            thinking,
        }
    }

    pub fn tool_result(tool_call_id: impl Into<String>, content: impl Into<String>) -> Self {
        Self {
            role: "tool".into(),
            content: Some(MessageContent::Text(content.into())),
            tool_calls: None,
            tool_call_id: Some(tool_call_id.into()),
            name: None,
            thinking: None,
        }
    }
}

// ── Tool definition ──

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ToolDefinition {
    #[serde(rename = "type")]
    pub type_: String,
    pub function: FunctionDefinition,
    /// When set on the LAST tool in a request, Anthropic caches the full
    /// tool-definitions prefix. Placed at the tool wrapper level (not inside
    /// FunctionDefinition) to match Anthropic's native tool-block shape.
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub cache_control: Option<CacheControl>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct FunctionDefinition {
    pub name: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub description: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub parameters: Option<serde_json::Value>,
}

// ── Tool call (in assistant response) ──

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ToolCall {
    pub id: String,
    #[serde(rename = "type")]
    pub type_: String,
    pub function: FunctionCall,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct FunctionCall {
    pub name: String,
    pub arguments: String,
}

// ── Stream chunk types ──

#[derive(Debug, Deserialize)]
pub struct ChatCompletionChunk {
    pub id: String,
    pub choices: Vec<ChunkChoice>,
    pub usage: Option<Usage>,
}

#[derive(Debug, Deserialize)]
pub struct ChunkChoice {
    pub index: u32,
    pub delta: ChunkDelta,
    pub finish_reason: Option<String>,
}

#[derive(Debug, Deserialize)]
pub struct ChunkDelta {
    pub role: Option<String>,
    pub content: Option<String>,
    pub tool_calls: Option<Vec<ToolCallChunk>>,
    /// Thinking text delta from Anthropic via gateway.
    pub thinking: Option<String>,
}

#[derive(Debug, Deserialize)]
pub struct ToolCallChunk {
    #[serde(default)]
    pub index: u32,
    pub id: Option<String>,
    #[serde(rename = "type")]
    pub type_: Option<String>,
    pub function: Option<FunctionCallChunk>,
}

#[derive(Debug, Deserialize)]
pub struct FunctionCallChunk {
    pub name: Option<String>,
    pub arguments: Option<String>,
}

// ── Usage ──

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Usage {
    pub prompt_tokens: u32,
    pub completion_tokens: u32,
    pub total_tokens: u32,
    /// Cache token breakdown when prompt caching is active.
    /// Populated for both OpenAI (`prompt_tokens_details.cached_tokens`) and
    /// Anthropic (mapped from `cache_read_input_tokens` + `cache_creation_input_tokens`
    /// by the gateway). Absent for providers/responses that don't emit it.
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub prompt_tokens_details: Option<PromptTokensDetails>,
}

#[derive(Debug, Clone, Default, Serialize, Deserialize)]
pub struct PromptTokensDetails {
    /// Tokens served from cache this request (read tier).
    /// OpenAI: `cached_tokens`. Anthropic: `cache_read_input_tokens`.
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub cached_tokens: Option<u32>,
    /// Tokens written to cache this request (write tier).
    /// OpenAI: not distinguished. Anthropic: `cache_creation_input_tokens`.
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub cache_creation_tokens: Option<u32>,
}

// ── Assembled LLM response ──

#[derive(Debug)]
pub struct LlmResponse {
    pub content: Option<String>,
    pub tool_calls: Vec<ToolCall>,
    pub usage: Option<Usage>,
    pub finish_reason: Option<String>,
    /// Accumulated thinking text from this response.
    pub thinking: Option<String>,
}

#[cfg(test)]
mod cache_control_tests {
    use super::*;

    #[test]
    fn test_cache_control_serializes_compactly() {
        // ephemeral() emits {"type":"ephemeral","ttl":"1h"} with no null ttl.
        let cc = CacheControl::ephemeral();
        let json = serde_json::to_string(&cc).unwrap();
        assert_eq!(json, r#"{"type":"ephemeral","ttl":"1h"}"#);
    }

    #[test]
    fn test_cache_control_with_ttl_serializes_ttl() {
        let cc = CacheControl { type_: "ephemeral".into(), ttl: Some("1h".into()) };
        let json = serde_json::to_string(&cc).unwrap();
        assert!(json.contains(r#""ttl":"1h""#));
    }

    #[test]
    fn test_content_block_text_without_cache_control_omits_field() {
        let block = ContentBlock::Text { text: "hi".into(), cache_control: None };
        let json = serde_json::to_string(&block).unwrap();
        assert!(!json.contains("cache_control"), "absent field must not serialize: {json}");
    }

    #[test]
    fn test_content_block_text_with_cache_control_serializes_nested() {
        let block = ContentBlock::Text {
            text: "hi".into(),
            cache_control: Some(CacheControl::ephemeral()),
        };
        let json = serde_json::to_string(&block).unwrap();
        assert_eq!(
            json,
            r#"{"type":"text","text":"hi","cache_control":{"type":"ephemeral","ttl":"1h"}}"#
        );
    }

    #[test]
    fn test_tool_definition_omits_cache_control_when_none() {
        let tool = ToolDefinition {
            type_: "function".into(),
            function: FunctionDefinition {
                name: "read".into(),
                description: None,
                parameters: None,
            },
            cache_control: None,
        };
        let json = serde_json::to_string(&tool).unwrap();
        assert!(!json.contains("cache_control"));
    }

    #[test]
    fn test_tool_definition_emits_cache_control_at_wrapper_level() {
        let tool = ToolDefinition {
            type_: "function".into(),
            function: FunctionDefinition {
                name: "read".into(),
                description: None,
                parameters: None,
            },
            cache_control: Some(CacheControl::ephemeral()),
        };
        let json = serde_json::to_string(&tool).unwrap();
        // cache_control must sit alongside `type` and `function`, not inside `function`.
        assert!(json.contains(r#""cache_control":{"type":"ephemeral","ttl":"1h"}"#));
        // Crude structural check that cache_control is NOT inside the function object.
        let function_idx = json.find("\"function\"").unwrap();
        let cc_idx = json.find("\"cache_control\"").unwrap();
        assert!(cc_idx > function_idx, "cache_control should appear after function in wire order");
    }
}

