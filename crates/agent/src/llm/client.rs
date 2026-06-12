use std::sync::LazyLock;

use async_trait::async_trait;
use reqwest::header::{HeaderMap, CONTENT_TYPE};
use tokio::sync::mpsc;
use tokio_util::sync::CancellationToken;

use crate::error::AgentError;
use crate::types::AgentEvent;
use super::anthropic;
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

/// Per-binary behavior knobs for the LLM HTTP client. The default is **permissive**
/// (byte-identical to the desktop app's historical behavior — pooled HTTP/2 via the
/// shared client); `LlmPolicy::bench()` is the strict variant the headless
/// `bench-runner` opts into so the eval harness matches opencode's working transport
/// shape (HTTP/1.1, fresh connection per request, 15s header-arrival timeout). Gating
/// these behind a policy keeps the shipped app client unchanged.
#[derive(Debug, Clone)]
pub struct LlmPolicy {
    /// Pin the LLM client to HTTP/1.1. `false` = let reqwest negotiate (defaults to h2
    /// when ALPN offers it).
    pub http1_only: bool,
    /// Disable connection pooling — every LLM request opens a fresh TCP+TLS connection.
    /// `false` = use the shared pooled client.
    pub no_pool: bool,
    /// Abort the request if response headers do not arrive within this many ms. `None`
    /// = no header-arrival timeout (only the per-chunk idle timeout applies once the
    /// body starts).
    pub header_timeout_ms: Option<u64>,
}

impl Default for LlmPolicy {
    /// Permissive — preserves the historical desktop-app behavior exactly.
    fn default() -> Self {
        Self {
            http1_only: false,
            no_pool: false,
            header_timeout_ms: None,
        }
    }
}

impl LlmPolicy {
    /// Strict policy for the eval harness: matches opencode's working transport shape
    /// to neutralize router-side mid-stream resets on long Fireworks/Kimi runs.
    pub fn bench() -> Self {
        Self {
            http1_only: true,
            no_pool: true,
            header_timeout_ms: Some(15_000),
        }
    }
}

/// Configuration for the LLM HTTP client.
#[derive(Debug, Clone)]
pub struct LlmClientConfig {
    /// Which wire format to speak. Drives URL/header/request-build + SSE-parse.
    pub provider: Provider,
    /// Base URL for the API. OpenAI: includes `/v1` (e.g. "https://api.openai.com/v1").
    /// Anthropic: host root (e.g. "https://api.anthropic.com").
    pub base_url: String,
    /// Model ID, e.g. "claude-sonnet-4-6"
    pub model: String,
    /// API key. The crate builds the provider-appropriate auth header from this
    /// (OpenAI: `Authorization: Bearer`, Anthropic: `x-api-key`). Empty = no auth header.
    pub api_key: String,
    /// Sampling temperature (0-2)
    pub temperature: Option<f32>,
    /// Upper bound on output tokens
    pub max_completion_tokens: Option<u32>,
    /// Extra headers forwarded verbatim with each request (escape hatch for
    /// self-hosted gateways / custom tenancy headers). Applied after the
    /// provider auth header, so an entry can override it.
    pub extra_headers: Vec<(String, String)>,
    /// Extended thinking config (e.g., {"type": "enabled", "budget_tokens": 10000}).
    pub thinking: Option<serde_json::Value>,
    /// Skip cache_control / prompt_cache_key — set for one-shot calls like compaction where writes never read back.
    pub disable_cache_control: bool,
    /// Per-binary LLM transport behavior. Default = pooled HTTP/2 (app); `bench()` =
    /// HTTP/1.1, no pool, 15s header timeout (matches opencode).
    pub policy: LlmPolicy,
}

/// HTTP client that speaks either OpenAI chat-completions or the Anthropic
/// Messages API natively, selected by `config.provider`.
pub struct LlmClient {
    config: LlmClientConfig,
    http: reqwest::Client,
}

impl LlmClient {
    pub fn new(config: LlmClientConfig) -> Self {
        // Build a dedicated client when the policy deviates from default; otherwise
        // share the pooled process-wide client. Bench-runner takes this path to match
        // opencode's transport shape (h1.1, no pool) which has zero decode deaths on
        // the same router that gave us 26.
        let http = if config.policy.http1_only || config.policy.no_pool {
            let mut b = reqwest::Client::builder()
                .connect_timeout(std::time::Duration::from_secs(30));
            if config.policy.http1_only {
                b = b.http1_only();
            }
            if config.policy.no_pool {
                b = b.pool_max_idle_per_host(0);
            }
            b.build().expect("Failed to build dedicated LLM HTTP client")
        } else {
            SHARED_HTTP_CLIENT.clone()
        };
        Self { config, http }
    }

    /// Send a streaming chat completion request and return the assembled response.
    ///
    /// Emits `AgentEvent::TextDelta` via `event_tx` as text tokens arrive.
    /// Dispatches to the OpenAI or Anthropic path based on `config.provider`.
    pub async fn chat_completion(
        &self,
        messages: &[ChatMessage],
        tools: &[ToolDefinition],
        event_tx: &mpsc::Sender<AgentEvent>,
        session_id: &str,
        cancel_token: Option<&CancellationToken>,
    ) -> Result<LlmResponse, AgentError> {
        match self.config.provider {
            Provider::OpenAI => {
                self.chat_completion_openai(messages, tools, event_tx, session_id, cancel_token).await
            }
            Provider::Anthropic => {
                self.chat_completion_anthropic(messages, tools, event_tx, session_id, cancel_token).await
            }
        }
    }

    /// OpenAI chat-completions path. Wire format is unchanged from before the
    /// provider split — the old `model.starts_with("claude-")` sniff is gone, so
    /// this path is always the former non-claude branch (no Anthropic-style
    /// cache_control on messages/tools/request; `prompt_cache_key` routing hint set).
    async fn chat_completion_openai(
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
        let mut sanitized_messages = sanitize_orphaned_tool_results(messages);

        // OpenAI caching is automatic on their side; we only pass a prompt_cache_key
        // routing hint (session id) for consistent machine affinity. Anthropic-style
        // explicit cache_control markers (on messages/tools/request) belong solely to
        // the Anthropic path (see anthropic::build_anthropic_request), so they are
        // never emitted here.
        let cache_enabled = !self.config.disable_cache_control;

        let prompt_cache_key = if cache_enabled {
            Some(session_id.to_string())
        } else {
            None
        };

        // Strip any cache_control that prompt.rs set on system/message blocks — the
        // OpenAI wire never carries it (OpenAI ignores it; keeping it absent is cleaner).
        for msg in &mut sanitized_messages {
            if let Some(MessageContent::Blocks(ref mut blocks)) = msg.content {
                for block in blocks.iter_mut() {
                    if let ContentBlock::Text { ref mut cache_control, .. } = block {
                        *cache_control = None;
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
            cache_control: None,
        };

        let mut headers = HeaderMap::new();
        headers.insert(CONTENT_TYPE, "application/json".parse().unwrap());
        // OpenAI auth: Authorization: Bearer <key>. Built from api_key (when set);
        // extra_headers applied after so a host can override.
        if !self.config.api_key.is_empty() {
            match format!("Bearer {}", self.config.api_key).parse::<reqwest::header::HeaderValue>() {
                Ok(val) => { headers.insert(reqwest::header::AUTHORIZATION, val); }
                Err(_) => log::warn!("[LLM] Skipping invalid Authorization value"),
            }
        }
        apply_extra_headers(&mut headers, &self.config.extra_headers);

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

        let send_future = self.http.post(&url).headers(headers).json(&request_body).send();
        let response = match send_with_header_timeout(send_future, self.config.policy.header_timeout_ms).await {
            Ok(resp) => resp,
            Err(e) => {
                log::error!("[LLM] HTTP request failed: {e}");
                return Err(e);
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

    /// Anthropic Messages API path. Builds the native `/v1/messages` request
    /// (role-merging, tool_use/tool_result, cache_control, extended thinking) and
    /// parses the native SSE event stream into the same `LlmResponse` as OpenAI.
    async fn chat_completion_anthropic(
        &self,
        messages: &[ChatMessage],
        tools: &[ToolDefinition],
        event_tx: &mpsc::Sender<AgentEvent>,
        session_id: &str,
        cancel_token: Option<&CancellationToken>,
    ) -> Result<LlmResponse, AgentError> {
        let url = format!("{}/v1/messages", self.config.base_url.trim_end_matches('/'));

        let sanitized_messages = sanitize_orphaned_tool_results(messages);
        let request_body = anthropic::build_anthropic_request(&sanitized_messages, tools, &self.config);

        let mut headers = HeaderMap::new();
        headers.insert(CONTENT_TYPE, "application/json".parse().unwrap());
        // Anthropic auth + version. extra_headers applied after so a host can
        // override (e.g. pin a different anthropic-version or add a beta header).
        if !self.config.api_key.is_empty() {
            match self.config.api_key.parse::<reqwest::header::HeaderValue>() {
                Ok(val) => { headers.insert("x-api-key", val); }
                Err(_) => log::warn!("[LLM] Skipping invalid x-api-key value"),
            }
        }
        headers.insert("anthropic-version", anthropic::ANTHROPIC_VERSION.parse().unwrap());
        apply_extra_headers(&mut headers, &self.config.extra_headers);

        let msg_count = messages.len();
        log::info!(
            "[LLM] POST {} — model={}, messages={}, tools={}",
            url, self.config.model, msg_count, tools.len()
        );
        if log::log_enabled!(log::Level::Debug) {
            match serde_json::to_string(&request_body) {
                Ok(json) => log::debug!("[LLM] request body: {json}"),
                Err(e) => log::debug!("[LLM] request body serialize failed: {e}"),
            }
        }

        let send_future = self.http.post(&url).headers(headers).json(&request_body).send();
        let response = match send_with_header_timeout(send_future, self.config.policy.header_timeout_ms).await {
            Ok(resp) => resp,
            Err(e) => {
                log::error!("[LLM] HTTP request failed: {e}");
                return Err(e);
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
        anthropic::parse_anthropic_sse_stream(byte_stream, event_tx, session_id, cancel_token).await
    }
}

/// Race the `.send()` future against an optional header-arrival timeout. Bounds the
/// request-issue + header-receive phase only; once headers arrive the body stream is
/// returned and bounded separately by `CHUNK_IDLE_TIMEOUT` in the SSE reader. A
/// `HeaderTimeout` is retryable by `call_llm_with_retry` (same arm as `HttpError`).
async fn send_with_header_timeout<F>(fut: F, timeout_ms: Option<u64>) -> Result<reqwest::Response, AgentError>
where
    F: std::future::Future<Output = Result<reqwest::Response, reqwest::Error>>,
{
    match timeout_ms {
        Some(ms) => match tokio::time::timeout(std::time::Duration::from_millis(ms), fut).await {
            Ok(res) => res.map_err(AgentError::HttpError),
            Err(_) => Err(AgentError::HeaderTimeout(ms)),
        },
        None => fut.await.map_err(AgentError::HttpError),
    }
}

/// Strip orphaned tool_result messages whose originating tool_use is no longer
/// present (e.g. lost to compaction). Shared by both provider paths.
fn sanitize_orphaned_tool_results(messages: &[ChatMessage]) -> Vec<ChatMessage> {
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
}

/// Inject extra/escape-hatch headers verbatim, skipping any with an invalid
/// header name or non-ASCII value. Applied after the provider auth header so an
/// entry can intentionally override it.
fn apply_extra_headers(headers: &mut HeaderMap, extra: &[(String, String)]) {
    for (key, value) in extra {
        match (key.parse::<reqwest::header::HeaderName>(), value.parse::<reqwest::header::HeaderValue>()) {
            (Ok(name), Ok(val)) => { headers.insert(name, val); }
            _ => log::warn!("[LLM] Skipping invalid extra header key={key:?} — bad name or non-ASCII value"),
        }
    }
}
