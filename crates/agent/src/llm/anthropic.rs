//! Native Anthropic Messages API path: request building + SSE parsing.
//!
//! Ported from the Go LLM gateway's `anthropic_adapter.go` / `anthropic_stream.go`.
//! `build_anthropic_request` translates the crate's OpenAI-shaped `ChatMessage`s
//! into a native `/v1/messages` request (system extraction, role-adjacency
//! merging, tool_use/tool_result, cache_control, extended thinking, images).
//! `parse_anthropic_sse_stream` maps Anthropic's SSE event stream into the SAME
//! `LlmResponse` the OpenAI parser produces, so the agent loop is provider-agnostic.

use std::collections::HashMap;

use bytes::Bytes;
use serde::Serialize;
use serde_json::{json, Value};
use tokio::sync::mpsc;
use tokio_util::sync::CancellationToken;

use crate::error::AgentError;
use crate::types::AgentEvent;
use super::client::LlmClientConfig;
use super::sse::{build_response, SseLineReader, ToolCallAccumulator};
use super::types::*;

/// Pinned Anthropic API version. Overridable per-request via `extra_headers`
/// (e.g. to opt into a beta). Sent as the `anthropic-version` header.
pub const ANTHROPIC_VERSION: &str = "2023-06-01";

/// Anthropic requires `max_tokens`; used when the config leaves it unset.
pub const DEFAULT_MAX_TOKENS: u32 = 8192;

// ── Request DTOs (serialize → /v1/messages body) ──

#[derive(Debug, Serialize)]
pub struct AnthropicRequest {
    pub model: String,
    pub max_tokens: u32,
    pub messages: Vec<AnthropicMessage>,
    pub stream: bool,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub system: Option<AnthropicSystem>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub temperature: Option<f32>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub thinking: Option<Value>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub tools: Option<Vec<AnthropicTool>>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub tool_choice: Option<Value>,
}

/// System prompt: a plain string when no block carries cache_control, otherwise
/// an array of text blocks (so per-block cache breakpoints survive).
#[derive(Debug, Serialize)]
#[serde(untagged)]
pub enum AnthropicSystem {
    Text(String),
    Blocks(Vec<AnthropicTextBlock>),
}

#[derive(Debug, Serialize)]
pub struct AnthropicTextBlock {
    #[serde(rename = "type")]
    pub type_: &'static str, // "text"
    pub text: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub cache_control: Option<CacheControl>,
}

#[derive(Debug, Serialize)]
pub struct AnthropicMessage {
    pub role: String, // "user" | "assistant"
    pub content: Vec<AnthropicContentBlock>,
}

#[derive(Debug, Serialize)]
#[serde(tag = "type", rename_all = "snake_case")]
pub enum AnthropicContentBlock {
    Text {
        text: String,
        #[serde(skip_serializing_if = "Option::is_none")]
        cache_control: Option<CacheControl>,
    },
    Image {
        source: AnthropicImageSource,
    },
    ToolUse {
        id: String,
        name: String,
        input: Value,
    },
    ToolResult {
        tool_use_id: String,
        content: String,
        #[serde(skip_serializing_if = "Option::is_none")]
        cache_control: Option<CacheControl>,
    },
    Thinking {
        thinking: String,
        #[serde(skip_serializing_if = "Option::is_none")]
        signature: Option<String>,
    },
}

#[derive(Debug, Serialize)]
pub struct AnthropicImageSource {
    #[serde(rename = "type")]
    pub type_: &'static str, // "base64"
    pub media_type: String,
    pub data: String,
}

#[derive(Debug, Serialize)]
pub struct AnthropicTool {
    pub name: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub description: Option<String>,
    pub input_schema: Value,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub cache_control: Option<CacheControl>,
}

// ── Request builder ──

/// Translate the crate's chat messages + tools into a native Anthropic request.
pub fn build_anthropic_request(
    messages: &[ChatMessage],
    tools: &[ToolDefinition],
    config: &LlmClientConfig,
) -> AnthropicRequest {
    let cache_enabled = !config.disable_cache_control;

    // 1. System extraction (plain string vs cache-bearing block array).
    let system = extract_system(messages, cache_enabled);

    // 2. Translate non-system messages, flushing accumulated tool_results as a
    //    `user` message before each subsequent non-tool message (and at the end).
    let mut translated: Vec<AnthropicMessage> = Vec::with_capacity(messages.len());
    let mut pending_tool_results: Vec<AnthropicContentBlock> = Vec::new();

    for msg in messages {
        match msg.role.as_str() {
            "system" => continue,
            "tool" => {
                let tool_use_id = sanitize_tool_id(msg.tool_call_id.as_deref().unwrap_or(""));
                let content = msg.content.as_ref().map(|c| c.text().to_string()).unwrap_or_default();
                pending_tool_results.push(AnthropicContentBlock::ToolResult {
                    tool_use_id,
                    content,
                    cache_control: None,
                });
            }
            role => {
                if !pending_tool_results.is_empty() {
                    translated.push(AnthropicMessage {
                        role: "user".into(),
                        content: std::mem::take(&mut pending_tool_results),
                    });
                }
                match role {
                    "assistant" => translated.push(translate_assistant(msg)),
                    "user" => translated.push(translate_user(msg, cache_enabled)),
                    other => {
                        log::warn!("[Anthropic] Unknown message role {other:?}, treating as user");
                        translated.push(translate_user(msg, cache_enabled));
                    }
                }
            }
        }
    }
    if !pending_tool_results.is_empty() {
        translated.push(AnthropicMessage {
            role: "user".into(),
            content: pending_tool_results,
        });
    }

    // 3. Merge consecutive same-role messages (Anthropic requires alternation).
    let mut out_messages = merge_adjacent_roles(translated);

    // Conversation-level breakpoint: cache the message-history prefix by marking
    // the last content block of the last message. (Anthropic has no request-level
    // cache_control field — the breakpoint must live on a block.)
    if cache_enabled {
        if let Some(last_block) = out_messages.last_mut().and_then(|m| m.content.last_mut()) {
            set_block_cache_control(last_block, Some(CacheControl::ephemeral()));
        }
    }

    // 5. Extended thinking forces temperature=1.0 (Anthropic requirement).
    let (thinking, temperature) = if thinking_enabled(&config.thinking) {
        (config.thinking.clone(), Some(1.0))
    } else {
        (None, config.temperature)
    };

    // 6. Tools — cache the full tool-definitions prefix via the last tool.
    let tools_out = if tools.is_empty() {
        None
    } else {
        let mut v: Vec<AnthropicTool> = tools.iter().map(translate_tool).collect();
        if cache_enabled {
            if let Some(last) = v.last_mut() {
                last.cache_control = Some(CacheControl::ephemeral());
            }
        }
        Some(v)
    };

    // 7. tool_choice — default auto when tools are present (mirrors OpenAI path).
    let tool_choice = if tools.is_empty() { None } else { Some(json!({"type": "auto"})) };

    AnthropicRequest {
        model: config.model.clone(),
        max_tokens: config.max_completion_tokens.unwrap_or(DEFAULT_MAX_TOKENS),
        messages: out_messages,
        stream: true,
        system,
        temperature,
        thinking,
        tools: tools_out,
        tool_choice,
    }
}

fn extract_system(messages: &[ChatMessage], cache_enabled: bool) -> Option<AnthropicSystem> {
    let mut segments: Vec<(String, Option<CacheControl>)> = Vec::new();
    for msg in messages {
        if msg.role != "system" {
            continue;
        }
        match &msg.content {
            Some(MessageContent::Text(s)) => segments.push((s.clone(), None)),
            Some(MessageContent::Blocks(blocks)) => {
                for b in blocks {
                    if let ContentBlock::Text { text, cache_control } = b {
                        let cc = if cache_enabled { cache_control.clone() } else { None };
                        segments.push((text.clone(), cc));
                    }
                }
            }
            None => {}
        }
    }
    if segments.is_empty() {
        return None;
    }
    if segments.iter().any(|(_, cc)| cc.is_some()) {
        let blocks = segments
            .into_iter()
            .map(|(text, cache_control)| AnthropicTextBlock { type_: "text", text, cache_control })
            .collect();
        Some(AnthropicSystem::Blocks(blocks))
    } else {
        let text = segments.into_iter().map(|(t, _)| t).collect::<Vec<_>>().join("\n\n");
        Some(AnthropicSystem::Text(text))
    }
}

fn translate_user(msg: &ChatMessage, cache_enabled: bool) -> AnthropicMessage {
    let content = match &msg.content {
        Some(MessageContent::Blocks(blocks)) => {
            let mut out = Vec::with_capacity(blocks.len());
            for b in blocks {
                match b {
                    ContentBlock::Text { text, cache_control } => {
                        let cc = if cache_enabled { cache_control.clone() } else { None };
                        out.push(AnthropicContentBlock::Text { text: text.clone(), cache_control: cc });
                    }
                    ContentBlock::ImageUrl { image_url } => match parse_data_uri(&image_url.url) {
                        Some((media_type, data)) => out.push(AnthropicContentBlock::Image {
                            source: AnthropicImageSource { type_: "base64", media_type, data },
                        }),
                        None => log::warn!("[Anthropic] Skipping non-data-URI image"),
                    },
                }
            }
            out
        }
        Some(MessageContent::Text(s)) => {
            vec![AnthropicContentBlock::Text { text: s.clone(), cache_control: None }]
        }
        None => vec![],
    };
    AnthropicMessage { role: "user".into(), content }
}

fn translate_assistant(msg: &ChatMessage) -> AnthropicMessage {
    let mut content: Vec<AnthropicContentBlock> = Vec::new();

    // Thinking block first (if any). NOTE: round-tripping thinking back to
    // Anthropic with extended thinking enabled requires the original signature,
    // which the crate does not retain (thinking is stored as a plain string).
    // Thinking is not enabled in the app today, so this never breaks current
    // flows; revisit if/when signatures are threaded through.
    if let Some(thinking) = &msg.thinking {
        if !thinking.is_empty() {
            content.push(AnthropicContentBlock::Thinking { thinking: thinking.clone(), signature: None });
        }
    }

    let text = msg.content.as_ref().map(|c| c.text()).unwrap_or("");
    if !text.is_empty() {
        content.push(AnthropicContentBlock::Text { text: text.to_string(), cache_control: None });
    }

    if let Some(tool_calls) = &msg.tool_calls {
        for tc in tool_calls {
            let input: Value = if tc.function.arguments.is_empty() {
                json!({})
            } else {
                serde_json::from_str(&tc.function.arguments).unwrap_or_else(|_| json!({}))
            };
            content.push(AnthropicContentBlock::ToolUse {
                id: sanitize_tool_id(&tc.id),
                name: tc.function.name.clone(),
                input,
            });
        }
    }

    AnthropicMessage { role: "assistant".into(), content }
}

fn translate_tool(t: &ToolDefinition) -> AnthropicTool {
    AnthropicTool {
        name: t.function.name.clone(),
        description: t.function.description.clone(),
        input_schema: t.function.parameters.clone().unwrap_or_else(|| json!({"type": "object"})),
        cache_control: None,
    }
}

/// Merge consecutive same-role messages by concatenating their content blocks.
fn merge_adjacent_roles(messages: Vec<AnthropicMessage>) -> Vec<AnthropicMessage> {
    let mut out: Vec<AnthropicMessage> = Vec::with_capacity(messages.len());
    for msg in messages {
        if let Some(last) = out.last_mut() {
            if last.role == msg.role {
                last.content.extend(msg.content);
                continue;
            }
        }
        out.push(msg);
    }
    out
}

fn set_block_cache_control(block: &mut AnthropicContentBlock, cc: Option<CacheControl>) {
    match block {
        AnthropicContentBlock::Text { cache_control, .. } => *cache_control = cc,
        AnthropicContentBlock::ToolResult { cache_control, .. } => *cache_control = cc,
        // tool_use / image / thinking are rare as a trailing block; skip rather
        // than restructure to add a breakpoint field.
        _ => {}
    }
}

fn thinking_enabled(thinking: &Option<Value>) -> bool {
    thinking
        .as_ref()
        .and_then(|v| v.get("budget_tokens"))
        .and_then(|b| b.as_u64())
        .map(|n| n > 0)
        .unwrap_or(false)
}

/// Strip characters Anthropic disallows in tool ids: anything outside
/// `[a-zA-Z0-9_-]` becomes `_`.
fn sanitize_tool_id(id: &str) -> String {
    id.chars()
        .map(|c| if c.is_ascii_alphanumeric() || c == '_' || c == '-' { c } else { '_' })
        .collect()
}

/// Parse a `data:{media_type};base64,{data}` URI → `(media_type, data)`.
fn parse_data_uri(url: &str) -> Option<(String, String)> {
    let rest = url.strip_prefix("data:")?;
    let (meta, data) = rest.split_once(',')?;
    let media_type = meta.strip_suffix(";base64").unwrap_or(meta);
    if media_type.is_empty() {
        return None;
    }
    Some((media_type.to_string(), data.to_string()))
}

fn map_stop_reason(reason: &str) -> String {
    match reason {
        "end_turn" => "stop",
        "tool_use" => "tool_calls",
        "max_tokens" => "length",
        "stop_sequence" => "stop",
        _ => "stop",
    }
    .to_string()
}

// ── SSE parser ──

/// Parse Anthropic's Messages SSE stream into an `LlmResponse`, emitting
/// `TextDelta` / `ThinkingDelta` events as content arrives. Dispatches on each
/// event's `type` field (robust to missing `event:` lines).
pub async fn parse_anthropic_sse_stream(
    stream: impl futures::Stream<Item = Result<Bytes, reqwest::Error>> + Unpin,
    event_tx: &mpsc::Sender<AgentEvent>,
    session_id: &str,
    cancel_token: Option<&CancellationToken>,
) -> Result<LlmResponse, AgentError> {
    let mut reader = SseLineReader::new(stream);
    let mut accumulated_text = String::new();
    let mut accumulated_thinking = String::new();
    let mut tool_accumulators: HashMap<u32, ToolCallAccumulator> = HashMap::new();
    let mut finish_reason: Option<String> = None;

    // Usage builder. Anthropic reports cached input separately from `input_tokens`.
    let mut input_tokens: u32 = 0;
    let mut cache_read: u32 = 0;
    let mut cache_creation: u32 = 0;
    let mut output_tokens: u32 = 0;
    let mut have_usage = false;

    while let Some(line) = reader.next_line(cancel_token).await? {
        if line.is_empty() || line.starts_with(':') {
            continue;
        }
        // Only `data:` lines carry JSON; skip `event:` and others.
        let data = if let Some(d) = line.strip_prefix("data: ") {
            d.trim()
        } else if let Some(d) = line.strip_prefix("data:") {
            d.trim()
        } else {
            continue;
        };

        let v: Value = match serde_json::from_str(data) {
            Ok(v) => v,
            Err(e) => {
                log::warn!("Failed to parse Anthropic SSE chunk: {e} — data: {data}");
                continue;
            }
        };

        match v.get("type").and_then(|t| t.as_str()).unwrap_or("") {
            "message_start" => {
                if let Some(usage) = v.get("message").and_then(|m| m.get("usage")) {
                    input_tokens = u32_field(usage, "input_tokens");
                    cache_creation = u32_field(usage, "cache_creation_input_tokens");
                    cache_read = u32_field(usage, "cache_read_input_tokens");
                    have_usage = true;
                }
            }
            "content_block_start" => {
                let index = u32_field(&v, "index");
                if let Some(cb) = v.get("content_block") {
                    if cb.get("type").and_then(|t| t.as_str()) == Some("tool_use") {
                        tool_accumulators.insert(
                            index,
                            ToolCallAccumulator {
                                id: cb.get("id").and_then(|x| x.as_str()).unwrap_or("").to_string(),
                                name: cb.get("name").and_then(|x| x.as_str()).unwrap_or("").to_string(),
                                arguments: String::new(),
                            },
                        );
                    }
                }
            }
            "content_block_delta" => {
                let index = u32_field(&v, "index");
                if let Some(delta) = v.get("delta") {
                    match delta.get("type").and_then(|t| t.as_str()).unwrap_or("") {
                        "text_delta" => {
                            if let Some(text) = delta.get("text").and_then(|x| x.as_str()) {
                                if !text.is_empty() {
                                    accumulated_text.push_str(text);
                                    let _ = event_tx
                                        .send(AgentEvent::TextDelta {
                                            session_id: session_id.to_string(),
                                            delta: text.to_string(),
                                        })
                                        .await;
                                }
                            }
                        }
                        "thinking_delta" => {
                            if let Some(t) = delta.get("thinking").and_then(|x| x.as_str()) {
                                if !t.is_empty() {
                                    accumulated_thinking.push_str(t);
                                    let _ = event_tx
                                        .send(AgentEvent::ThinkingDelta {
                                            session_id: session_id.to_string(),
                                            delta: t.to_string(),
                                        })
                                        .await;
                                }
                            }
                        }
                        "input_json_delta" => {
                            if let Some(partial) = delta.get("partial_json").and_then(|x| x.as_str()) {
                                if let Some(acc) = tool_accumulators.get_mut(&index) {
                                    acc.arguments.push_str(partial);
                                }
                            }
                        }
                        _ => {} // signature_delta etc. — not retained
                    }
                }
            }
            "message_delta" => {
                if let Some(sr) = v.get("delta").and_then(|d| d.get("stop_reason")).and_then(|x| x.as_str()) {
                    finish_reason = Some(map_stop_reason(sr));
                }
                if let Some(usage) = v.get("usage") {
                    output_tokens = usage
                        .get("output_tokens")
                        .and_then(|x| x.as_u64())
                        .map(|n| n as u32)
                        .unwrap_or(output_tokens);
                    let cc = u32_field(usage, "cache_creation_input_tokens");
                    if cc > 0 {
                        cache_creation = cc;
                    }
                    let cr = u32_field(usage, "cache_read_input_tokens");
                    if cr > 0 {
                        cache_read = cr;
                    }
                    have_usage = true;
                }
            }
            "message_stop" => break,
            "error" => {
                let msg = v
                    .get("error")
                    .and_then(|e| e.get("message"))
                    .and_then(|m| m.as_str())
                    .unwrap_or("unknown error");
                return Err(AgentError::LlmParseError(format!("Anthropic stream error: {msg}")));
            }
            _ => {} // ping, content_block_stop, etc.
        }
    }

    let usage = if have_usage {
        // Anthropic's input_tokens excludes cached input; sum all three so
        // total_tokens reflects the full context (matches the OpenAI accounting
        // the compaction logic expects).
        let prompt_tokens = input_tokens + cache_read + cache_creation;
        Some(Usage {
            prompt_tokens,
            completion_tokens: output_tokens,
            total_tokens: prompt_tokens + output_tokens,
            prompt_tokens_details: Some(PromptTokensDetails {
                cached_tokens: Some(cache_read),
                cache_creation_tokens: Some(cache_creation),
            }),
        })
    } else {
        None
    };

    Ok(build_response(accumulated_text, accumulated_thinking, tool_accumulators, usage, finish_reason))
}

fn u32_field(v: &Value, key: &str) -> u32 {
    v.get(key).and_then(|x| x.as_u64()).unwrap_or(0) as u32
}

#[cfg(test)]
mod tests {
    use super::*;
    use bytes::Bytes;
    use futures::stream;
    use tokio::sync::mpsc;

    fn cfg() -> LlmClientConfig {
        LlmClientConfig {
            provider: Provider::Anthropic,
            base_url: "https://api.anthropic.com".into(),
            model: "claude-sonnet-4-6".into(),
            api_key: "k".into(),
            temperature: Some(0.5),
            max_completion_tokens: None,
            extra_headers: vec![],
            thinking: None,
            disable_cache_control: false,
            policy: Default::default(),
        }
    }

    fn user(text: &str) -> ChatMessage {
        ChatMessage::user(text)
    }

    fn sse_stream(lines: &str) -> impl futures::Stream<Item = Result<Bytes, reqwest::Error>> + Unpin {
        let bytes = Bytes::from(lines.to_string());
        Box::pin(stream::once(async move { Ok(bytes) }))
    }

    // ── Request builder ──

    #[test]
    fn system_plain_string_when_no_cache_control() {
        let msgs = vec![ChatMessage::system("alpha"), ChatMessage::system("beta"), user("hi")];
        let req = build_anthropic_request(&msgs, &[], &cfg());
        match req.system {
            Some(AnthropicSystem::Text(s)) => assert_eq!(s, "alpha\n\nbeta"),
            other => panic!("expected plain text system, got {other:?}"),
        }
    }

    #[test]
    fn system_block_array_when_any_cache_control() {
        let sys = ChatMessage {
            role: "system".into(),
            content: Some(MessageContent::Blocks(vec![ContentBlock::Text {
                text: "cached".into(),
                cache_control: Some(CacheControl::ephemeral()),
            }])),
            tool_calls: None,
            tool_call_id: None,
            name: None,
            thinking: None,
        };
        let req = build_anthropic_request(&[sys, user("hi")], &[], &cfg());
        match req.system {
            Some(AnthropicSystem::Blocks(b)) => {
                assert_eq!(b.len(), 1);
                assert!(b[0].cache_control.is_some());
            }
            other => panic!("expected block-array system, got {other:?}"),
        }
    }

    #[test]
    fn system_cache_control_stripped_when_disabled() {
        let sys = ChatMessage {
            role: "system".into(),
            content: Some(MessageContent::Blocks(vec![ContentBlock::Text {
                text: "cached".into(),
                cache_control: Some(CacheControl::ephemeral()),
            }])),
            tool_calls: None,
            tool_call_id: None,
            name: None,
            thinking: None,
        };
        let mut c = cfg();
        c.disable_cache_control = true;
        let req = build_anthropic_request(&[sys, user("hi")], &[], &c);
        // No cache_control anywhere → folds to a plain string.
        assert!(matches!(req.system, Some(AnthropicSystem::Text(_))));
    }

    #[test]
    fn role_adjacency_merge() {
        let msgs = vec![user("a"), user("b"), ChatMessage::assistant(Some("c".into()), None, None)];
        let req = build_anthropic_request(&msgs, &[], &cfg());
        assert_eq!(req.messages.len(), 2);
        assert_eq!(req.messages[0].role, "user");
        assert_eq!(req.messages[0].content.len(), 2); // a + b merged
        assert_eq!(req.messages[1].role, "assistant");
    }

    #[test]
    fn tool_result_flush_and_merge_into_following_user() {
        // assistant(tool_use) → tool → tool → user  ==>  assistant, then one user
        // message holding [tool_result, tool_result, text] (flush + adjacency merge).
        let assistant = ChatMessage::assistant(
            None,
            Some(vec![
                ToolCall { id: "call_1".into(), type_: "function".into(), function: FunctionCall { name: "read".into(), arguments: "{}".into() } },
                ToolCall { id: "call_2".into(), type_: "function".into(), function: FunctionCall { name: "read".into(), arguments: "{}".into() } },
            ]),
            None,
        );
        let msgs = vec![
            assistant,
            ChatMessage::tool_result("call_1", "r1"),
            ChatMessage::tool_result("call_2", "r2"),
            user("next"),
        ];
        let req = build_anthropic_request(&msgs, &[], &cfg());
        assert_eq!(req.messages.len(), 2);
        assert_eq!(req.messages[0].role, "assistant");
        assert_eq!(req.messages[1].role, "user");
        assert_eq!(req.messages[1].content.len(), 3); // two tool_results + text
        assert!(matches!(req.messages[1].content[0], AnthropicContentBlock::ToolResult { .. }));
        assert!(matches!(req.messages[1].content[2], AnthropicContentBlock::Text { .. }));
    }

    #[test]
    fn tool_id_sanitization_roundtrip() {
        let assistant = ChatMessage::assistant(
            None,
            Some(vec![ToolCall {
                id: "call/weird:id".into(),
                type_: "function".into(),
                function: FunctionCall { name: "read".into(), arguments: "{\"x\":1}".into() },
            }]),
            None,
        );
        let msgs = vec![assistant, ChatMessage::tool_result("call/weird:id", "ok")];
        let req = build_anthropic_request(&msgs, &[], &cfg());
        let tool_use_id = match &req.messages[0].content[0] {
            AnthropicContentBlock::ToolUse { id, input, .. } => {
                assert_eq!(input, &json!({"x": 1}));
                id.clone()
            }
            other => panic!("expected tool_use, got {other:?}"),
        };
        let result_id = match &req.messages[1].content[0] {
            AnthropicContentBlock::ToolResult { tool_use_id, .. } => tool_use_id.clone(),
            other => panic!("expected tool_result, got {other:?}"),
        };
        assert_eq!(tool_use_id, "call_weird_id");
        assert_eq!(result_id, "call_weird_id");
    }

    #[test]
    fn cache_control_on_last_tool_only() {
        let tool = |n: &str| ToolDefinition {
            type_: "function".into(),
            function: FunctionDefinition { name: n.into(), description: None, parameters: None },
            cache_control: None,
        };
        let req = build_anthropic_request(&[user("hi")], &[tool("a"), tool("b")], &cfg());
        let tools = req.tools.unwrap();
        assert!(tools[0].cache_control.is_none());
        assert!(tools[1].cache_control.is_some());
        assert_eq!(tools[1].input_schema, json!({"type": "object"}));
        assert_eq!(req.tool_choice, Some(json!({"type": "auto"})));
    }

    #[test]
    fn thinking_forces_temperature_1() {
        let mut c = cfg();
        c.temperature = Some(0.2);
        c.thinking = Some(json!({"type": "enabled", "budget_tokens": 1024}));
        let req = build_anthropic_request(&[user("hi")], &[], &c);
        assert_eq!(req.temperature, Some(1.0));
        assert!(req.thinking.is_some());
    }

    #[test]
    fn no_thinking_passes_temperature_through() {
        let req = build_anthropic_request(&[user("hi")], &[], &cfg());
        assert_eq!(req.temperature, Some(0.5));
        assert!(req.thinking.is_none());
    }

    #[test]
    fn image_data_uri_to_base64_source() {
        let msg = ChatMessage::user_with_images(vec![
            ContentBlock::Text { text: "look".into(), cache_control: None },
            ContentBlock::ImageUrl { image_url: ImageUrlContent { url: "data:image/png;base64,QUJD".into(), detail: None } },
        ]);
        let req = build_anthropic_request(&[msg], &[], &cfg());
        match &req.messages[0].content[1] {
            AnthropicContentBlock::Image { source } => {
                assert_eq!(source.media_type, "image/png");
                assert_eq!(source.data, "QUJD");
                assert_eq!(source.type_, "base64");
            }
            other => panic!("expected image, got {other:?}"),
        }
    }

    #[test]
    fn assistant_block_order_thinking_text_tooluse() {
        let msg = ChatMessage::assistant(
            Some("answer".into()),
            Some(vec![ToolCall { id: "c1".into(), type_: "function".into(), function: FunctionCall { name: "read".into(), arguments: "{}".into() } }]),
            Some("pondering".into()),
        );
        let req = build_anthropic_request(&[msg], &[], &cfg());
        let blocks = &req.messages[0].content;
        assert!(matches!(blocks[0], AnthropicContentBlock::Thinking { .. }));
        assert!(matches!(blocks[1], AnthropicContentBlock::Text { .. }));
        assert!(matches!(blocks[2], AnthropicContentBlock::ToolUse { .. }));
    }

    #[test]
    fn max_tokens_default_and_from_config() {
        let req = build_anthropic_request(&[user("hi")], &[], &cfg());
        assert_eq!(req.max_tokens, DEFAULT_MAX_TOKENS);
        let mut c = cfg();
        c.max_completion_tokens = Some(256);
        let req2 = build_anthropic_request(&[user("hi")], &[], &c);
        assert_eq!(req2.max_tokens, 256);
    }

    #[test]
    fn empty_tools_omits_tools_and_choice() {
        let req = build_anthropic_request(&[user("hi")], &[], &cfg());
        assert!(req.tools.is_none());
        assert!(req.tool_choice.is_none());
    }

    #[test]
    fn conversation_breakpoint_on_last_block() {
        let req = build_anthropic_request(&[user("only")], &[], &cfg());
        match req.messages.last().unwrap().content.last().unwrap() {
            AnthropicContentBlock::Text { cache_control, .. } => assert!(cache_control.is_some()),
            other => panic!("expected text, got {other:?}"),
        }
        // Disabled → no breakpoint.
        let mut c = cfg();
        c.disable_cache_control = true;
        let req2 = build_anthropic_request(&[user("only")], &[], &c);
        match req2.messages.last().unwrap().content.last().unwrap() {
            AnthropicContentBlock::Text { cache_control, .. } => assert!(cache_control.is_none()),
            other => panic!("expected text, got {other:?}"),
        }
    }

    // ── SSE parser ──

    #[tokio::test]
    async fn message_start_text_then_delta_with_cache_usage() {
        let data = "\
event: message_start\n\
data: {\"type\":\"message_start\",\"message\":{\"id\":\"m1\",\"model\":\"claude\",\"usage\":{\"input_tokens\":10,\"cache_creation_input_tokens\":100,\"cache_read_input_tokens\":50}}}\n\n\
event: content_block_start\n\
data: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"text\",\"text\":\"\"}}\n\n\
event: content_block_delta\n\
data: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"Hello\"}}\n\n\
event: content_block_delta\n\
data: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\" world\"}}\n\n\
event: message_delta\n\
data: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"},\"usage\":{\"output_tokens\":7}}\n\n\
event: message_stop\n\
data: {\"type\":\"message_stop\"}\n\n";
        let (tx, mut rx) = mpsc::channel(32);
        let resp = parse_anthropic_sse_stream(sse_stream(data), &tx, "s", None).await.unwrap();
        assert_eq!(resp.content.as_deref(), Some("Hello world"));
        assert_eq!(resp.finish_reason.as_deref(), Some("stop"));
        let usage = resp.usage.unwrap();
        assert_eq!(usage.prompt_tokens, 160); // 10 + 50 + 100
        assert_eq!(usage.completion_tokens, 7);
        assert_eq!(usage.total_tokens, 167);
        let d = usage.prompt_tokens_details.unwrap();
        assert_eq!(d.cached_tokens, Some(50));
        assert_eq!(d.cache_creation_tokens, Some(100));

        let mut deltas = Vec::new();
        while let Ok(ev) = rx.try_recv() {
            if let AgentEvent::TextDelta { delta, .. } = ev {
                deltas.push(delta);
            }
        }
        assert_eq!(deltas, vec!["Hello", " world"]);
    }

    #[tokio::test]
    async fn tool_use_block_streaming() {
        let data = "\
data: {\"type\":\"message_start\",\"message\":{\"id\":\"m1\",\"model\":\"c\",\"usage\":{\"input_tokens\":5}}}\n\n\
data: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"tool_use\",\"id\":\"toolu_1\",\"name\":\"read\"}}\n\n\
data: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"input_json_delta\",\"partial_json\":\"{\\\"file\"}}\n\n\
data: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"input_json_delta\",\"partial_json\":\"Path\\\":\\\"a.rs\\\"}\"}}\n\n\
data: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"tool_use\"},\"usage\":{\"output_tokens\":3}}\n\n\
data: {\"type\":\"message_stop\"}\n\n";
        let (tx, _rx) = mpsc::channel(32);
        let resp = parse_anthropic_sse_stream(sse_stream(data), &tx, "s", None).await.unwrap();
        assert_eq!(resp.tool_calls.len(), 1);
        assert_eq!(resp.tool_calls[0].id, "toolu_1");
        assert_eq!(resp.tool_calls[0].function.name, "read");
        assert_eq!(resp.tool_calls[0].function.arguments, "{\"filePath\":\"a.rs\"}");
        assert_eq!(resp.finish_reason.as_deref(), Some("tool_calls"));
    }

    #[tokio::test]
    async fn thinking_deltas_and_signature_ignored() {
        let data = "\
data: {\"type\":\"message_start\",\"message\":{\"id\":\"m1\",\"model\":\"c\",\"usage\":{\"input_tokens\":1}}}\n\n\
data: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"thinking\"}}\n\n\
data: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"thinking_delta\",\"thinking\":\"hmm\"}}\n\n\
data: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"signature_delta\",\"signature\":\"sig\"}}\n\n\
data: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"thinking_delta\",\"thinking\":\" ok\"}}\n\n\
data: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"},\"usage\":{\"output_tokens\":2}}\n\n\
data: {\"type\":\"message_stop\"}\n\n";
        let (tx, _rx) = mpsc::channel(32);
        let resp = parse_anthropic_sse_stream(sse_stream(data), &tx, "s", None).await.unwrap();
        assert_eq!(resp.thinking.as_deref(), Some("hmm ok"));
        assert!(resp.content.is_none());
    }

    #[tokio::test]
    async fn error_event_returns_err() {
        let data = "data: {\"type\":\"error\",\"error\":{\"type\":\"overloaded_error\",\"message\":\"overloaded\"}}\n\n";
        let (tx, _rx) = mpsc::channel(32);
        let result = parse_anthropic_sse_stream(sse_stream(data), &tx, "s", None).await;
        assert!(matches!(result, Err(AgentError::LlmParseError(_))));
    }

    #[tokio::test]
    async fn stop_reason_mappings() {
        for (anthropic, expected) in [("max_tokens", "length"), ("stop_sequence", "stop"), ("tool_use", "tool_calls")] {
            let data = format!(
                "data: {{\"type\":\"message_start\",\"message\":{{\"id\":\"m\",\"model\":\"c\",\"usage\":{{\"input_tokens\":1}}}}}}\n\n\
                 data: {{\"type\":\"message_delta\",\"delta\":{{\"stop_reason\":\"{anthropic}\"}},\"usage\":{{\"output_tokens\":1}}}}\n\n\
                 data: {{\"type\":\"message_stop\"}}\n\n"
            );
            let (tx, _rx) = mpsc::channel(32);
            let resp = parse_anthropic_sse_stream(sse_stream(&data), &tx, "s", None).await.unwrap();
            assert_eq!(resp.finish_reason.as_deref(), Some(expected), "for {anthropic}");
        }
    }
}
