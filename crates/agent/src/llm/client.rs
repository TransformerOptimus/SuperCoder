use std::sync::LazyLock;

use async_trait::async_trait;
use reqwest::header::{HeaderMap, CONTENT_TYPE};
use tokio::sync::mpsc;
use tokio_util::sync::CancellationToken;

use crate::error::AgentError;
use crate::types::AgentEvent;
use super::sse::parse_sse_stream;
use super::types::*;

/// Trait for LLM providers — allows mocking in tests.
#[async_trait]
pub trait LlmProvider: Send + Sync {
    async fn chat_completion(
        &self,
        messages: &[ChatMessage],
        tools: &[ToolDefinition],
        event_tx: &mpsc::Sender<AgentEvent>,
        session_id: &str,
        cancel_token: Option<&CancellationToken>,
    ) -> Result<LlmResponse, AgentError>;
}

#[async_trait]
impl LlmProvider for LlmClient {
    async fn chat_completion(
        &self,
        messages: &[ChatMessage],
        tools: &[ToolDefinition],
        event_tx: &mpsc::Sender<AgentEvent>,
        session_id: &str,
        cancel_token: Option<&CancellationToken>,
    ) -> Result<LlmResponse, AgentError> {
        LlmClient::chat_completion(self, messages, tools, event_tx, session_id, cancel_token).await
    }
}

/// Shared HTTP client — reuses connection pool across all agent sessions.
/// `reqwest::Client` is `Arc`-based internally, so clone is cheap.
/// 30s connect timeout prevents indefinite hang if the API server is unreachable.
/// No total timeout — streaming responses can run indefinitely; idle detection
/// is handled per-chunk in the SSE parser (see sse.rs CHUNK_IDLE_TIMEOUT).
static SHARED_HTTP_CLIENT: LazyLock<reqwest::Client> = LazyLock::new(|| {
    reqwest::Client::builder()
        .connect_timeout(std::time::Duration::from_secs(30))
        .build()
        .expect("Failed to create HTTP client")
});

/// Configuration for the LLM HTTP client.
#[derive(Debug, Clone)]
pub struct LlmClientConfig {
    /// Base URL for the API, e.g. "https://api.openai.com/v1"
    pub base_url: String,
    /// Model ID, e.g. "claude-sonnet-4-6"
    pub model: String,
    /// Sampling temperature (0-2)
    pub temperature: Option<f32>,
    /// Upper bound on output tokens
    pub max_completion_tokens: Option<u32>,
    /// Optional auth headers forwarded with each request (e.g. X-Auth-Token, X-USER-ID, X-Workspace-ID).
    /// Injected by the host adapter.
    pub auth_headers: Vec<(String, String)>,
    /// Extended thinking config (e.g., {"type": "enabled", "budget_tokens": 10000}).
    pub thinking: Option<serde_json::Value>,
    /// Skip cache_control / prompt_cache_key — set for one-shot calls like compaction where writes never read back.
    pub disable_cache_control: bool,
}

/// HTTP client for OpenAI-compatible /v1/chat/completions endpoint.
pub struct LlmClient {
    config: LlmClientConfig,
    http: reqwest::Client,
}

impl LlmClient {
    pub fn new(config: LlmClientConfig) -> Self {
        Self {
            config,
            http: SHARED_HTTP_CLIENT.clone(),
        }
    }

    /// Send a streaming chat completion request and return the assembled response.
    ///
    /// Emits `AgentEvent::TextDelta` via `event_tx` as text tokens arrive.
    pub async fn chat_completion(
        &self,
        messages: &[ChatMessage],
        tools: &[ToolDefinition],
        event_tx: &mpsc::Sender<AgentEvent>,
        session_id: &str,
        cancel_token: Option<&CancellationToken>,
    ) -> Result<LlmResponse, AgentError> {
        let url = format!("{}/chat/completions", self.config.base_url.trim_end_matches('/'));

        let tools_option = if tools.is_empty() {
            None
        } else {
            Some(tools.to_vec())
        };

        // Sanitize: strip orphaned tool_result messages whose tool_use was lost
        let mut sanitized_messages = {
            let mut valid_ids = std::collections::HashSet::new();
            for msg in messages {
                if let Some(ref tcs) = msg.tool_calls {
                    for tc in tcs {
                        valid_ids.insert(tc.id.as_str());
                    }
                }
            }
            let mut msgs: Vec<ChatMessage> = Vec::with_capacity(messages.len());
            for msg in messages {
                if msg.role == "tool" {
                    if let Some(ref id) = msg.tool_call_id {
                        if !valid_ids.contains(id.as_str()) {
                            log::warn!("[LLM] Stripping orphaned tool_result: {}", id);
                            continue;
                        }
                    }
                }
                msgs.push(msg.clone());
            }
            msgs
        };

        // Anthropic caching: hybrid strategy:
        //   - explicit cache_control on last tool (caches tool definitions)
        //   - explicit cache_control on system[0] (the static body, set by prompt.rs)
        //   - explicit cache_control on system[1] (skills list, set by prompt.rs
        //     when a registry is active)
        //   - top-level cache_control: ephemeral for automatic conversation-level
        //     advancement (Anthropic moves the breakpoint to the last cacheable
        //     block each turn, and uses the 20-block lookback to find prior writes)
        //
        // NOTE: Anthropic caps cache_control markers at 4 per request. With a
        // skill registry active we hit the cap exactly. Adding a 5th cached
        // prefix source (memory, CLAUDE.md, etc.) requires dropping one of:
        // tools[last], system[0], system[1], or the top-level breakpoint.
        //
        // OpenAI caching: automatic on their side; we only pass a prompt_cache_key
        // routing hint for consistent machine affinity.
        let is_anthropic = self.config.model.starts_with("claude-");
        let cache_enabled = !self.config.disable_cache_control;

        let prompt_cache_key = if is_anthropic || !cache_enabled {
            None
        } else {
            Some(session_id.to_string())
        };

        // Top-level auto cache_control: Anthropic places a breakpoint on the last
        // cacheable block each turn, caching the conversation prefix. Combined with
        // our explicit markers on tools[last] + system[0], this gives the biggest
        // possible write (6K+ tokens vs ~3.8K with explicit-only). Experimentally
        // doesn't affect the ~60% hit rate we see for Sonnet 4.6 (which appears to
        // stem from Anthropic-side cache routing non-determinism), but when it DOES
        // hit, the savings are ~60% larger. Keep enabled.
        let top_level_cache_control = if is_anthropic && cache_enabled {
            Some(CacheControl::ephemeral())
        } else {
            None
        };

        // Mark the last tool with cache_control so Anthropic caches the full
        // tool-definitions prefix. Only mutate for Claude — OpenAI ignores it
        // anyway, but keeping the field absent on the OpenAI wire is cleaner.
        let tools_option = if let Some(mut t) = tools_option {
            if is_anthropic && cache_enabled {
                if let Some(last) = t.last_mut() {
                    last.cache_control = Some(CacheControl::ephemeral());
                }
            }
            Some(t)
        } else {
            None
        };

        // Strip cache_control from message content blocks for non-Claude models OR when
        // caching is disabled. prompt.rs always sets cache_control on system block 0.
        if !is_anthropic || !cache_enabled {
            for msg in &mut sanitized_messages {
                if let Some(MessageContent::Blocks(ref mut blocks)) = msg.content {
                    for block in blocks.iter_mut() {
                        if let ContentBlock::Text { ref mut cache_control, .. } = block {
                            *cache_control = None;
                        }
                    }
                }
            }
        }

        let request_body = ChatCompletionRequest {
            model: self.config.model.clone(),
            messages: sanitized_messages,
            stream: true,
            stream_options: Some(StreamOptions {
                include_usage: true,
            }),
            tools: tools_option,
            tool_choice: if tools.is_empty() {
                None
            } else {
                Some("auto".to_string())
            },
            parallel_tool_calls: if tools.is_empty() { None } else { Some(true) },
            max_completion_tokens: self.config.max_completion_tokens,
            temperature: self.config.temperature,
            thinking: self.config.thinking.clone(),
            prompt_cache_key,
            cache_control: top_level_cache_control,
        };

        let mut headers = HeaderMap::new();
        headers.insert(CONTENT_TYPE, "application/json".parse().unwrap());
        for (key, value) in &self.config.auth_headers {
            match (key.parse::<reqwest::header::HeaderName>(), value.parse::<reqwest::header::HeaderValue>()) {
                (Ok(name), Ok(val)) => { headers.insert(name, val); }
                _ => log::warn!("[LLM] Skipping invalid auth header key={key:?} — bad name or non-ASCII value"),
            }
        }

        // Estimate request size for debugging
        let msg_count = messages.len();
        let approx_chars: usize = messages.iter().map(|m| {
            m.content.as_ref().map(|c| c.text().len()).unwrap_or(0)
                + m.tool_calls.as_ref().map(|tc| tc.iter().map(|t| t.function.arguments.len()).sum::<usize>()).unwrap_or(0)
        }).sum();
        log::info!(
            "[LLM] POST {} — model={}, messages={}, ~{}chars, tools={}",
            url, self.config.model, msg_count, approx_chars, tools.len()
        );

        // At debug level, log the serialized outbound body. Useful when diagnosing
        // cache_control breakpoints (expect `"cache_control":{"type":"ephemeral","ttl":"1h"}`
        // at system[0], system[1] when skills active, tools[last], and request-level).
        if log::log_enabled!(log::Level::Debug) {
            match serde_json::to_string(&request_body) {
                Ok(json) => log::debug!("[LLM] request body: {json}"),
                Err(e) => log::debug!("[LLM] request body serialize failed: {e}"),
            }
        }

        let response = match self
            .http
            .post(&url)
            .headers(headers)
            .json(&request_body)
            .send()
            .await
        {
            Ok(resp) => resp,
            Err(e) => {
                log::error!("[LLM] HTTP request failed: {e}");
                return Err(e.into());
            }
        };

        let status = response.status();
        if !status.is_success() {
            let body = response.text().await.unwrap_or_default();
            log::error!("[LLM] API error {}: {}", status, &body[..body.len().min(500)]);
            return Err(AgentError::LlmApiError {
                status: status.as_u16(),
                body,
            });
        }

        log::info!("[LLM] Streaming response started (status {})", status);
        let byte_stream = response.bytes_stream();
        parse_sse_stream(byte_stream, event_tx, session_id, cancel_token).await
    }
}
