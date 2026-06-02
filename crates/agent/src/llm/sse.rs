use std::collections::HashMap;
use std::time::Duration;

use bytes::Bytes;
use futures::StreamExt;
use tokio::sync::mpsc;
use tokio::time::timeout;
use tokio_util::sync::CancellationToken;

use crate::error::AgentError;
use crate::types::AgentEvent;
use super::types::*;

/// Maximum SSE buffer size (1 MB). If a single line exceeds this, the stream
/// is considered malformed / adversarial and we bail rather than eating memory.
const MAX_BUFFER_SIZE: usize = 1024 * 1024;

/// Maximum time to wait for the next SSE chunk before considering the stream stalled.
/// Anthropic sends keepalive pings every ~15-30s during extended thinking, so 60s
/// provides ample margin while still catching genuinely dead connections.
const CHUNK_IDLE_TIMEOUT: Duration = Duration::from_secs(60);

/// Accumulator for a single tool call being assembled from streaming chunks.
/// Shared with the Anthropic parser, which assembles the same `LlmResponse`.
#[derive(Debug)]
pub(crate) struct ToolCallAccumulator {
    pub(crate) id: String,
    pub(crate) name: String,
    pub(crate) arguments: String,
}

/// Shared SSE line reader: owns the byte stream + line buffer and yields one
/// complete line at a time, applying the idle-timeout, cancellation, and
/// buffer-overflow guards. Both the OpenAI (`parse_sse_stream`) and Anthropic
/// (`anthropic::parse_anthropic_sse_stream`) parsers drive this so the network
/// scaffolding lives in exactly one place; each parser keeps its own async
/// event-emitting loop on top.
pub(crate) struct SseLineReader<S> {
    stream: S,
    buffer: String,
}

impl<S> SseLineReader<S>
where
    S: futures::Stream<Item = Result<Bytes, reqwest::Error>> + Unpin,
{
    pub(crate) fn new(stream: S) -> Self {
        Self { stream, buffer: String::new() }
    }

    /// Return the next complete line (CRLF/LF trimmed) or `None` when the stream
    /// ends. A trailing partial line without a newline is dropped on stream end —
    /// SSE data lines always terminate with a newline, so this never loses an event.
    pub(crate) async fn next_line(
        &mut self,
        cancel_token: Option<&CancellationToken>,
    ) -> Result<Option<String>, AgentError> {
        loop {
            // Emit any complete line already buffered before reading more bytes.
            if let Some(newline_pos) = self.buffer.find('\n') {
                let line = self.buffer[..newline_pos].trim_end_matches('\r').to_string();
                self.buffer.replace_range(..=newline_pos, "");
                return Ok(Some(line));
            }

            // Race the next SSE chunk against cancellation and an idle timeout.
            // The idle timeout (CHUNK_IDLE_TIMEOUT) resets on each chunk, so active
            // streams are never killed — only stalled ones.
            let chunk_result = if let Some(token) = cancel_token {
                tokio::select! {
                    biased;  // check cancel first for faster response
                    _ = token.cancelled() => {
                        log::info!("[SSE] Stream cancelled by user");
                        return Err(AgentError::Cancelled);
                    }
                    timed = timeout(CHUNK_IDLE_TIMEOUT, self.stream.next()) => match timed {
                        Ok(Some(result)) => result,
                        Ok(None) => return Ok(None), // stream ended
                        Err(_) => {
                            log::error!("[SSE] No chunk received for {}s — aborting", CHUNK_IDLE_TIMEOUT.as_secs());
                            return Err(AgentError::ChunkTimeout(CHUNK_IDLE_TIMEOUT.as_secs()));
                        }
                    }
                }
            } else {
                match timeout(CHUNK_IDLE_TIMEOUT, self.stream.next()).await {
                    Ok(Some(result)) => result,
                    Ok(None) => return Ok(None),
                    Err(_) => {
                        log::error!("[SSE] No chunk received for {}s — aborting", CHUNK_IDLE_TIMEOUT.as_secs());
                        return Err(AgentError::ChunkTimeout(CHUNK_IDLE_TIMEOUT.as_secs()));
                    }
                }
            };
            let chunk_bytes = chunk_result.map_err(AgentError::HttpError)?;
            self.buffer.push_str(&String::from_utf8_lossy(&chunk_bytes));

            // Guard against unbounded buffer growth (malformed / adversarial stream)
            if self.buffer.len() > MAX_BUFFER_SIZE {
                return Err(AgentError::LlmParseError(format!(
                    "SSE buffer exceeded {} bytes without a newline — aborting",
                    MAX_BUFFER_SIZE
                )));
            }
        }
    }
}

/// Parse an SSE byte stream from the OpenAI chat completions API into an LlmResponse.
///
/// Emits TextDelta and ThinkingDelta events as tokens arrive. Tool calls are accumulated
/// internally and returned in the final LlmResponse.
pub async fn parse_sse_stream(
    stream: impl futures::Stream<Item = Result<Bytes, reqwest::Error>> + Unpin,
    event_tx: &mpsc::Sender<AgentEvent>,
    session_id: &str,
    cancel_token: Option<&CancellationToken>,
) -> Result<LlmResponse, AgentError> {
    let mut reader = SseLineReader::new(stream);
    let mut accumulated_text = String::new();
    let mut accumulated_thinking = String::new();
    let mut tool_accumulators: HashMap<u32, ToolCallAccumulator> = HashMap::new();
    let mut usage: Option<Usage> = None;
    let mut finish_reason: Option<String> = None;

    while let Some(line) = reader.next_line(cancel_token).await? {
        // Skip empty lines and SSE comments
        if line.is_empty() || line.starts_with(':') {
            continue;
        }

        // Must be a data: line
        let data = if let Some(data) = line.strip_prefix("data: ") {
            data.trim()
        } else if let Some(data) = line.strip_prefix("data:") {
            data.trim()
        } else {
            continue;
        };

        // End of stream
        if data == "[DONE]" {
            return Ok(build_response(
                accumulated_text,
                accumulated_thinking,
                tool_accumulators,
                usage,
                finish_reason,
            ));
        }

        // Parse the JSON chunk
        let chunk: ChatCompletionChunk = match serde_json::from_str(data) {
            Ok(c) => c,
            Err(e) => {
                log::warn!("Failed to parse SSE chunk: {e} — data: {data}");
                continue;
            }
        };

        // Handle usage-only final chunk (choices is empty)
        if chunk.choices.is_empty() {
            if chunk.usage.is_some() {
                usage = chunk.usage;
            }
            continue;
        }

        let choice = &chunk.choices[0];

        // Capture finish_reason
        if choice.finish_reason.is_some() {
            finish_reason = choice.finish_reason.clone();
        }

        // Accumulate thinking text
        if let Some(ref thinking) = choice.delta.thinking {
            if !thinking.is_empty() {
                accumulated_thinking.push_str(thinking);
                let _ = event_tx
                    .send(AgentEvent::ThinkingDelta {
                        session_id: session_id.to_string(),
                        delta: thinking.clone(),
                    })
                    .await;
            }
        }

        // Accumulate text content
        if let Some(ref content) = choice.delta.content {
            if !content.is_empty() {
                accumulated_text.push_str(content);
                let _ = event_tx
                    .send(AgentEvent::TextDelta {
                        session_id: session_id.to_string(),
                        delta: content.clone(),
                    })
                    .await;
            }
        }

        // Accumulate tool calls
        if let Some(ref tool_calls) = choice.delta.tool_calls {
            for tc_chunk in tool_calls {
                let acc = tool_accumulators
                    .entry(tc_chunk.index)
                    .or_insert_with(|| ToolCallAccumulator {
                        id: String::new(),
                        name: String::new(),
                        arguments: String::new(),
                    });

                if let Some(ref id) = tc_chunk.id {
                    if !id.is_empty() {
                        acc.id = id.clone();
                    }
                }

                if let Some(ref func) = tc_chunk.function {
                    if let Some(ref name) = func.name {
                        if !name.is_empty() {
                            acc.name = name.clone();
                        }
                    }
                    if let Some(ref args) = func.arguments {
                        acc.arguments.push_str(args);
                    }
                }
            }
        }

        // Capture usage from chunks that also have choices
        if chunk.usage.is_some() {
            usage = chunk.usage;
        }
    }

    // Stream ended without [DONE] — still return what we have
    Ok(build_response(
        accumulated_text,
        accumulated_thinking,
        tool_accumulators,
        usage,
        finish_reason,
    ))
}

pub(crate) fn build_response(
    accumulated_text: String,
    accumulated_thinking: String,
    tool_accumulators: HashMap<u32, ToolCallAccumulator>,
    usage: Option<Usage>,
    finish_reason: Option<String>,
) -> LlmResponse {
    let content = if accumulated_text.is_empty() {
        None
    } else {
        Some(accumulated_text)
    };

    let thinking = if accumulated_thinking.is_empty() {
        None
    } else {
        Some(accumulated_thinking)
    };

    // Sort tool calls by index for deterministic ordering
    let mut tool_entries: Vec<(u32, ToolCallAccumulator)> = tool_accumulators.into_iter().collect();
    tool_entries.sort_by_key(|(idx, _)| *idx);

    let tool_calls: Vec<ToolCall> = tool_entries
        .into_iter()
        .filter(|(_, acc)| {
            if acc.id.is_empty() || acc.name.is_empty() {
                log::warn!(
                    "[SSE] Dropping malformed tool call: id={:?}, name={:?} — gateway never sent required fields",
                    acc.id, acc.name
                );
                false
            } else {
                true
            }
        })
        .map(|(_, acc)| ToolCall {
            id: acc.id,
            type_: "function".to_string(),
            function: FunctionCall {
                name: acc.name,
                arguments: acc.arguments,
            },
        })
        .collect();

    LlmResponse {
        content,
        tool_calls,
        usage,
        finish_reason,
        thinking,
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use bytes::Bytes;
    use futures::stream;
    use tokio::sync::mpsc;

    /// Helper to create an SSE byte stream from raw SSE text lines.
    fn sse_stream(lines: &str) -> impl futures::Stream<Item = Result<Bytes, reqwest::Error>> + Unpin {
        let bytes = Bytes::from(lines.to_string());
        Box::pin(stream::once(async move { Ok(bytes) }))
    }

    /// Helper to create an SSE stream from multiple chunks (simulates TCP fragmentation).
    fn sse_stream_chunks(chunks: Vec<&str>) -> impl futures::Stream<Item = Result<Bytes, reqwest::Error>> + Unpin {
        let chunks: Vec<Result<Bytes, reqwest::Error>> = chunks
            .into_iter()
            .map(|s| Ok(Bytes::from(s.to_string())))
            .collect();
        Box::pin(stream::iter(chunks))
    }

    #[tokio::test]
    async fn test_text_only_stream() {
        let sse_data = "\
data: {\"id\":\"chatcmpl-1\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"\"},\"finish_reason\":null}]}\n\n\
data: {\"id\":\"chatcmpl-1\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"Hello\"},\"finish_reason\":null}]}\n\n\
data: {\"id\":\"chatcmpl-1\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\" world\"},\"finish_reason\":null}]}\n\n\
data: {\"id\":\"chatcmpl-1\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n\
data: [DONE]\n\n";

        let (tx, mut rx) = mpsc::channel(32);
        let stream = sse_stream(sse_data);
        let response = parse_sse_stream(stream, &tx, "test-session", None).await.unwrap();

        assert_eq!(response.content.as_deref(), Some("Hello world"));
        assert!(response.tool_calls.is_empty());
        assert_eq!(response.finish_reason.as_deref(), Some("stop"));
        assert!(response.thinking.is_none());

        // Check that TextDelta events were emitted
        let mut deltas = Vec::new();
        while let Ok(event) = rx.try_recv() {
            if let AgentEvent::TextDelta { delta, .. } = event {
                deltas.push(delta);
            }
        }
        assert_eq!(deltas, vec!["Hello", " world"]);
    }

    #[tokio::test]
    async fn test_tool_call_stream() {
        let sse_data = "\
data: {\"id\":\"chatcmpl-1\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\"},\"finish_reason\":null}]}\n\n\
data: {\"id\":\"chatcmpl-1\",\"choices\":[{\"index\":0,\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"call_abc\",\"type\":\"function\",\"function\":{\"name\":\"read\",\"arguments\":\"\"}}]},\"finish_reason\":null}]}\n\n\
data: {\"id\":\"chatcmpl-1\",\"choices\":[{\"index\":0,\"delta\":{\"tool_calls\":[{\"index\":0,\"function\":{\"arguments\":\"{\\\"file\"}}]},\"finish_reason\":null}]}\n\n\
data: {\"id\":\"chatcmpl-1\",\"choices\":[{\"index\":0,\"delta\":{\"tool_calls\":[{\"index\":0,\"function\":{\"arguments\":\"Path\\\":\\\"test.rs\\\"}\"}}]},\"finish_reason\":null}]}\n\n\
data: {\"id\":\"chatcmpl-1\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"tool_calls\"}]}\n\n\
data: [DONE]\n\n";

        let (tx, _rx) = mpsc::channel(32);
        let stream = sse_stream(sse_data);
        let response = parse_sse_stream(stream, &tx, "test-session", None).await.unwrap();

        assert!(response.content.is_none());
        assert_eq!(response.tool_calls.len(), 1);
        assert_eq!(response.tool_calls[0].id, "call_abc");
        assert_eq!(response.tool_calls[0].function.name, "read");
        assert_eq!(
            response.tool_calls[0].function.arguments,
            "{\"filePath\":\"test.rs\"}"
        );
        assert_eq!(response.finish_reason.as_deref(), Some("tool_calls"));
    }

    #[tokio::test]
    async fn test_multiple_tool_calls() {
        let sse_data = "\
data: {\"id\":\"chatcmpl-1\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"Reading files.\"},\"finish_reason\":null}]}\n\n\
data: {\"id\":\"chatcmpl-1\",\"choices\":[{\"index\":0,\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"call_1\",\"type\":\"function\",\"function\":{\"name\":\"read\",\"arguments\":\"{\\\"filePath\\\":\\\"/a.rs\\\"}\"}}]},\"finish_reason\":null}]}\n\n\
data: {\"id\":\"chatcmpl-1\",\"choices\":[{\"index\":0,\"delta\":{\"tool_calls\":[{\"index\":1,\"id\":\"call_2\",\"type\":\"function\",\"function\":{\"name\":\"read\",\"arguments\":\"{\\\"filePath\\\":\\\"/b.rs\\\"}\"}}]},\"finish_reason\":null}]}\n\n\
data: {\"id\":\"chatcmpl-1\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"tool_calls\"}]}\n\n\
data: [DONE]\n\n";

        let (tx, _rx) = mpsc::channel(32);
        let stream = sse_stream(sse_data);
        let response = parse_sse_stream(stream, &tx, "test-session", None).await.unwrap();

        assert_eq!(response.content.as_deref(), Some("Reading files."));
        assert_eq!(response.tool_calls.len(), 2);
        assert_eq!(response.tool_calls[0].function.name, "read");
        assert_eq!(response.tool_calls[1].function.name, "read");
        assert_eq!(response.tool_calls[0].id, "call_1");
        assert_eq!(response.tool_calls[1].id, "call_2");
    }

    #[tokio::test]
    async fn test_usage_in_final_chunk() {
        let sse_data = "\
data: {\"id\":\"chatcmpl-1\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"Hi\"},\"finish_reason\":null}]}\n\n\
data: {\"id\":\"chatcmpl-1\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n\
data: {\"id\":\"chatcmpl-1\",\"choices\":[],\"usage\":{\"prompt_tokens\":10,\"completion_tokens\":5,\"total_tokens\":15}}\n\n\
data: [DONE]\n\n";

        let (tx, _rx) = mpsc::channel(32);
        let stream = sse_stream(sse_data);
        let response = parse_sse_stream(stream, &tx, "test-session", None).await.unwrap();

        assert_eq!(response.content.as_deref(), Some("Hi"));
        let usage = response.usage.unwrap();
        assert_eq!(usage.prompt_tokens, 10);
        assert_eq!(usage.completion_tokens, 5);
        assert_eq!(usage.total_tokens, 15);
    }

    #[tokio::test]
    async fn test_split_json_across_tcp_chunks() {
        // JSON split across two TCP chunks
        let chunks = vec![
            "data: {\"id\":\"chatcmpl-1\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"Hel",
            "lo\"},\"finish_reason\":null}]}\n\ndata: {\"id\":\"chatcmpl-1\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n",
        ];

        let (tx, _rx) = mpsc::channel(32);
        let stream = sse_stream_chunks(chunks);
        let response = parse_sse_stream(stream, &tx, "test-session", None).await.unwrap();

        assert_eq!(response.content.as_deref(), Some("Hello"));
        assert_eq!(response.finish_reason.as_deref(), Some("stop"));
    }

    #[tokio::test]
    async fn test_tool_args_split_across_tcp_chunks() {
        // TCP chunk boundary falls right in the middle of a tool-call SSE data line,
        // so the JSON for the arguments straddles two network reads.
        let chunks = vec![
            // Chunk 1: header + start of first argument chunk SSE line (cut mid-JSON)
            "data: {\"id\":\"chatcmpl-1\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\"},\"finish_reason\":null}]}\n\n\
             data: {\"id\":\"chatcmpl-1\",\"choices\":[{\"index\":0,\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"call_x\",\"type\":\"function\",\"function\":{\"name\":\"read\",\"arguments\":\"{\\\"file\"}}]},\"finish_reason\":null}]}\n\n\
             data: {\"id\":\"chatcmpl-1\",\"choices\":[{\"index\":0,\"delta\":{\"tool_calls\":[{\"index\":0,\"function\":{\"arguments\":\"Pa",
            // Chunk 2: rest of the argument line + finish + DONE
            "th\\\":\\\"src/main.rs\\\"}\"}}]},\"finish_reason\":null}]}\n\n\
             data: {\"id\":\"chatcmpl-1\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"tool_calls\"}]}\n\n\
             data: [DONE]\n\n",
        ];

        let (tx, _rx) = mpsc::channel(32);
        let stream = sse_stream_chunks(chunks);
        let response = parse_sse_stream(stream, &tx, "test-session", None).await.unwrap();

        assert_eq!(response.tool_calls.len(), 1);
        assert_eq!(response.tool_calls[0].id, "call_x");
        assert_eq!(response.tool_calls[0].function.name, "read");
        // The arguments were split across two SSE events AND across a TCP boundary
        assert_eq!(
            response.tool_calls[0].function.arguments,
            "{\"filePath\":\"src/main.rs\"}"
        );
    }

    #[tokio::test]
    async fn test_sse_buffer_overflow_rejected() {
        // Simulate a malformed stream: one huge chunk with no newline
        let giant_chunk = "x".repeat(1024 * 1024 + 1);
        let stream = sse_stream(&giant_chunk);
        let (tx, _rx) = mpsc::channel(32);
        let result = parse_sse_stream(stream, &tx, "test-session", None).await;
        assert!(result.is_err());
        let err_msg = result.unwrap_err().to_string();
        assert!(err_msg.contains("SSE buffer exceeded"));
    }

    #[tokio::test]
    async fn test_sse_comment_lines_ignored() {
        let sse_data = "\
: this is a comment\n\
data: {\"id\":\"chatcmpl-1\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"ok\"},\"finish_reason\":null}]}\n\n\
: another comment\n\
data: {\"id\":\"chatcmpl-1\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n\
data: [DONE]\n\n";

        let (tx, _rx) = mpsc::channel(32);
        let stream = sse_stream(sse_data);
        let response = parse_sse_stream(stream, &tx, "test-session", None).await.unwrap();

        assert_eq!(response.content.as_deref(), Some("ok"));
    }

    #[tokio::test]
    async fn test_stream_ends_without_done() {
        // Stream sends text chunks but ends abruptly — no [DONE] marker.
        // The parser should still return whatever was accumulated.
        let sse_data = "\
data: {\"id\":\"chatcmpl-1\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"\"},\"finish_reason\":null}]}\n\n\
data: {\"id\":\"chatcmpl-1\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"partial\"},\"finish_reason\":null}]}\n\n\
data: {\"id\":\"chatcmpl-1\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\" response\"},\"finish_reason\":\"stop\"}]}\n\n";

        let (tx, _rx) = mpsc::channel(32);
        let stream = sse_stream(sse_data);
        let response = parse_sse_stream(stream, &tx, "test-session", None).await.unwrap();

        assert_eq!(response.content.as_deref(), Some("partial response"));
        assert_eq!(response.finish_reason.as_deref(), Some("stop"));
    }

    #[tokio::test]
    async fn test_stream_ends_without_done_with_tool_calls() {
        // Stream sends a tool call and then ends without [DONE].
        let sse_data = "\
data: {\"id\":\"chatcmpl-1\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\"},\"finish_reason\":null}]}\n\n\
data: {\"id\":\"chatcmpl-1\",\"choices\":[{\"index\":0,\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"call_1\",\"type\":\"function\",\"function\":{\"name\":\"read\",\"arguments\":\"{\\\"filePath\\\":\\\"a.rs\\\"}\"}}]},\"finish_reason\":null}]}\n\n\
data: {\"id\":\"chatcmpl-1\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"tool_calls\"}]}\n\n";

        let (tx, _rx) = mpsc::channel(32);
        let stream = sse_stream(sse_data);
        let response = parse_sse_stream(stream, &tx, "test-session", None).await.unwrap();

        assert_eq!(response.tool_calls.len(), 1);
        assert_eq!(response.tool_calls[0].function.name, "read");
        assert_eq!(response.finish_reason.as_deref(), Some("tool_calls"));
    }

    // ════════════════════════════════════════════
    // §1: SSE Parser Robustness Tests
    // ════════════════════════════════════════════

    #[tokio::test]
    async fn test_tool_call_chunk_missing_index() {
        // Tool call chunk without "index" field — should default to 0
        let sse_data = "\
data: {\"id\":\"chatcmpl-1\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\"},\"finish_reason\":null}]}\n\n\
data: {\"id\":\"chatcmpl-1\",\"choices\":[{\"index\":0,\"delta\":{\"tool_calls\":[{\"id\":\"call_no_idx\",\"type\":\"function\",\"function\":{\"name\":\"read\",\"arguments\":\"{\\\"filePath\\\":\\\"test.rs\\\"}\"}}]},\"finish_reason\":null}]}\n\n\
data: {\"id\":\"chatcmpl-1\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"tool_calls\"}]}\n\n\
data: [DONE]\n\n";

        let (tx, _rx) = mpsc::channel(32);
        let stream = sse_stream(sse_data);
        let response = parse_sse_stream(stream, &tx, "test-session", None).await.unwrap();

        assert_eq!(response.tool_calls.len(), 1);
        assert_eq!(response.tool_calls[0].id, "call_no_idx");
        assert_eq!(response.tool_calls[0].function.name, "read");
    }

    #[tokio::test]
    async fn test_tool_call_empty_name_not_overwritten() {
        // Gateway sends name in first chunk, then empty name in subsequent chunks
        let sse_data = "\
data: {\"id\":\"chatcmpl-1\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\"},\"finish_reason\":null}]}\n\n\
data: {\"id\":\"chatcmpl-1\",\"choices\":[{\"index\":0,\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"call_1\",\"type\":\"function\",\"function\":{\"name\":\"read\",\"arguments\":\"{\\\"file\"}}]},\"finish_reason\":null}]}\n\n\
data: {\"id\":\"chatcmpl-1\",\"choices\":[{\"index\":0,\"delta\":{\"tool_calls\":[{\"index\":0,\"function\":{\"name\":\"\",\"arguments\":\"Path\\\":\\\"test.rs\\\"}\"}}]},\"finish_reason\":null}]}\n\n\
data: {\"id\":\"chatcmpl-1\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"tool_calls\"}]}\n\n\
data: [DONE]\n\n";

        let (tx, _rx) = mpsc::channel(32);
        let stream = sse_stream(sse_data);
        let response = parse_sse_stream(stream, &tx, "test-session", None).await.unwrap();

        assert_eq!(response.tool_calls.len(), 1);
        assert_eq!(response.tool_calls[0].function.name, "read");
    }

    #[tokio::test]
    async fn test_tool_call_empty_id_not_overwritten() {
        // Gateway sends id in first chunk, then empty id in subsequent chunks
        let sse_data = "\
data: {\"id\":\"chatcmpl-1\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\"},\"finish_reason\":null}]}\n\n\
data: {\"id\":\"chatcmpl-1\",\"choices\":[{\"index\":0,\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"call_real\",\"type\":\"function\",\"function\":{\"name\":\"read\",\"arguments\":\"{\\\"file\"}}]},\"finish_reason\":null}]}\n\n\
data: {\"id\":\"chatcmpl-1\",\"choices\":[{\"index\":0,\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"\",\"function\":{\"arguments\":\"Path\\\":\\\"test.rs\\\"}\"}}]},\"finish_reason\":null}]}\n\n\
data: {\"id\":\"chatcmpl-1\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"tool_calls\"}]}\n\n\
data: [DONE]\n\n";

        let (tx, _rx) = mpsc::channel(32);
        let stream = sse_stream(sse_data);
        let response = parse_sse_stream(stream, &tx, "test-session", None).await.unwrap();

        assert_eq!(response.tool_calls.len(), 1);
        assert_eq!(response.tool_calls[0].id, "call_real");
    }

    #[tokio::test]
    async fn test_finish_reason_tool_use() {
        // Anthropic-style finish_reason via gateway
        let sse_data = "\
data: {\"id\":\"chatcmpl-1\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\"},\"finish_reason\":null}]}\n\n\
data: {\"id\":\"chatcmpl-1\",\"choices\":[{\"index\":0,\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"call_1\",\"type\":\"function\",\"function\":{\"name\":\"read\",\"arguments\":\"{\\\"filePath\\\":\\\"a.rs\\\"}\"}}]},\"finish_reason\":null}]}\n\n\
data: {\"id\":\"chatcmpl-1\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"tool_use\"}]}\n\n\
data: [DONE]\n\n";

        let (tx, _rx) = mpsc::channel(32);
        let stream = sse_stream(sse_data);
        let response = parse_sse_stream(stream, &tx, "test-session", None).await.unwrap();

        assert_eq!(response.finish_reason.as_deref(), Some("tool_use"));
        assert_eq!(response.tool_calls.len(), 1);
    }

    #[tokio::test]
    async fn test_finish_reason_end_turn() {
        // Anthropic-style finish_reason via gateway
        let sse_data = "\
data: {\"id\":\"chatcmpl-1\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"done\"},\"finish_reason\":null}]}\n\n\
data: {\"id\":\"chatcmpl-1\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"end_turn\"}]}\n\n\
data: [DONE]\n\n";

        let (tx, _rx) = mpsc::channel(32);
        let stream = sse_stream(sse_data);
        let response = parse_sse_stream(stream, &tx, "test-session", None).await.unwrap();

        assert_eq!(response.finish_reason.as_deref(), Some("end_turn"));
        assert_eq!(response.content.as_deref(), Some("done"));
    }

    // ════════════════════════════════════════════
    // §3: Thinking Tests
    // ════════════════════════════════════════════

    #[tokio::test]
    async fn test_thinking_delta_stream() {
        let sse_data = "\
data: {\"id\":\"chatcmpl-1\",\"choices\":[{\"index\":0,\"delta\":{\"thinking\":\"Let me solve\"},\"finish_reason\":null}]}\n\n\
data: {\"id\":\"chatcmpl-1\",\"choices\":[{\"index\":0,\"delta\":{\"thinking\":\" this step\"},\"finish_reason\":null}]}\n\n\
data: {\"id\":\"chatcmpl-1\",\"choices\":[{\"index\":0,\"delta\":{\"thinking\":\" by step\"},\"finish_reason\":null}]}\n\n\
data: {\"id\":\"chatcmpl-1\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"x = 6\"},\"finish_reason\":null}]}\n\n\
data: {\"id\":\"chatcmpl-1\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n\
data: [DONE]\n\n";

        let (tx, mut rx) = mpsc::channel(32);
        let stream = sse_stream(sse_data);
        let response = parse_sse_stream(stream, &tx, "test-session", None).await.unwrap();

        assert_eq!(response.thinking.as_deref(), Some("Let me solve this step by step"));
        assert_eq!(response.content.as_deref(), Some("x = 6"));

        // Check ThinkingDelta events
        let mut thinking_deltas = Vec::new();
        let mut text_deltas = Vec::new();
        while let Ok(event) = rx.try_recv() {
            match event {
                AgentEvent::ThinkingDelta { delta, .. } => thinking_deltas.push(delta),
                AgentEvent::TextDelta { delta, .. } => text_deltas.push(delta),
                _ => {}
            }
        }
        assert_eq!(thinking_deltas, vec!["Let me solve", " this step", " by step"]);
        assert_eq!(text_deltas, vec!["x = 6"]);
    }

    #[tokio::test]
    async fn test_thinking_then_tool_call_stream() {
        let sse_data = "\
data: {\"id\":\"chatcmpl-1\",\"choices\":[{\"index\":0,\"delta\":{\"thinking\":\"I should read the file first\"},\"finish_reason\":null}]}\n\n\
data: {\"id\":\"chatcmpl-1\",\"choices\":[{\"index\":0,\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"call_1\",\"type\":\"function\",\"function\":{\"name\":\"read\",\"arguments\":\"{\\\"filePath\\\":\\\"a.rs\\\"}\"}}]},\"finish_reason\":null}]}\n\n\
data: {\"id\":\"chatcmpl-1\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"tool_calls\"}]}\n\n\
data: [DONE]\n\n";

        let (tx, _rx) = mpsc::channel(32);
        let stream = sse_stream(sse_data);
        let response = parse_sse_stream(stream, &tx, "test-session", None).await.unwrap();

        assert_eq!(response.thinking.as_deref(), Some("I should read the file first"));
        assert_eq!(response.tool_calls.len(), 1);
        assert!(response.content.is_none());
    }

    #[tokio::test]
    async fn test_thinking_ignored_when_absent() {
        let sse_data = "\
data: {\"id\":\"chatcmpl-1\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"Hello\"},\"finish_reason\":null}]}\n\n\
data: {\"id\":\"chatcmpl-1\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n\
data: [DONE]\n\n";

        let (tx, _rx) = mpsc::channel(32);
        let stream = sse_stream(sse_data);
        let response = parse_sse_stream(stream, &tx, "test-session", None).await.unwrap();

        assert!(response.thinking.is_none());
        assert_eq!(response.content.as_deref(), Some("Hello"));
    }

    // ════════════════════════════════════════════
    // §4: Image / MessageContent Tests
    // ════════════════════════════════════════════

    #[test]
    fn test_message_content_text_serialization() {
        let content = MessageContent::Text("hello".to_string());
        let json = serde_json::to_string(&content).unwrap();
        assert_eq!(json, "\"hello\"");
    }

    #[test]
    fn test_message_content_blocks_serialization() {
        let content = MessageContent::Blocks(vec![
            ContentBlock::Text { text: "Look at this:".to_string(), cache_control: None },
            ContentBlock::ImageUrl {
                image_url: ImageUrlContent {
                    url: "data:image/png;base64,abc123".to_string(),
                    detail: Some("auto".to_string()),
                },
            },
        ]);
        let json = serde_json::to_string(&content).unwrap();
        assert!(json.starts_with('['));
        assert!(json.contains("\"type\":\"text\""));
        assert!(json.contains("\"type\":\"image_url\""));
    }

    #[test]
    fn test_message_content_deserialization_both() {
        // String form
        let text: MessageContent = serde_json::from_str("\"hello\"").unwrap();
        assert_eq!(text.text(), "hello");

        // Array form
        let blocks: MessageContent = serde_json::from_str(
            r#"[{"type":"text","text":"world"},{"type":"image_url","image_url":{"url":"data:image/png;base64,x"}}]"#
        ).unwrap();
        assert_eq!(blocks.text(), "world");
        assert!(blocks.has_images());
    }

    #[test]
    fn test_message_content_text_helper() {
        assert_eq!(MessageContent::Text("hello".into()).text(), "hello");
        assert_eq!(
            MessageContent::Blocks(vec![
                ContentBlock::ImageUrl {
                    image_url: ImageUrlContent { url: "x".into(), detail: None },
                },
                ContentBlock::Text { text: "found".into(), cache_control: None },
            ]).text(),
            "found"
        );
        // No text block → empty string
        assert_eq!(
            MessageContent::Blocks(vec![
                ContentBlock::ImageUrl {
                    image_url: ImageUrlContent { url: "x".into(), detail: None },
                },
            ]).text(),
            ""
        );
    }

    #[test]
    fn test_message_content_has_images() {
        assert!(!MessageContent::Text("hello".into()).has_images());
        assert!(MessageContent::Blocks(vec![
            ContentBlock::ImageUrl {
                image_url: ImageUrlContent { url: "x".into(), detail: None },
            },
        ]).has_images());
        assert!(!MessageContent::Blocks(vec![
            ContentBlock::Text { text: "no images".into(), cache_control: None },
        ]).has_images());
    }

    #[test]
    fn test_user_with_images_constructor() {
        let msg = ChatMessage::user_with_images(vec![
            ContentBlock::Text { text: "Look:".into(), cache_control: None },
            ContentBlock::ImageUrl {
                image_url: ImageUrlContent {
                    url: "data:image/png;base64,abc".into(),
                    detail: Some("auto".into()),
                },
            },
        ]);
        assert_eq!(msg.role, "user");
        assert!(msg.content.as_ref().unwrap().has_images());
    }

    #[test]
    fn test_backward_compat_old_json() {
        // Old-format JSON with content as a bare string should deserialize correctly
        let json = r#"{"role":"user","content":"hello old world"}"#;
        let msg: ChatMessage = serde_json::from_str(json).unwrap();
        assert_eq!(msg.content.as_ref().unwrap().text(), "hello old world");
        assert!(msg.thinking.is_none());
    }

    #[test]
    fn test_chat_message_thinking_roundtrip() {
        let msg = ChatMessage::assistant(
            Some("answer".into()),
            None,
            Some("I thought about it".into()),
        );
        let json = serde_json::to_string(&msg).unwrap();
        assert!(json.contains("\"thinking\":\"I thought about it\""));

        let deserialized: ChatMessage = serde_json::from_str(&json).unwrap();
        assert_eq!(deserialized.thinking.as_deref(), Some("I thought about it"));
        assert_eq!(deserialized.content.as_ref().unwrap().text(), "answer");
    }

    #[test]
    fn test_chat_message_no_thinking_roundtrip() {
        let msg = ChatMessage::assistant(Some("answer".into()), None, None);
        let json = serde_json::to_string(&msg).unwrap();
        assert!(!json.contains("thinking"));
    }

    /// Creates a stream backed by an mpsc channel. Returns (sender, stream).
    /// The stream stays open until the sender is dropped.
    fn channel_stream() -> (
        tokio::sync::mpsc::Sender<Result<Bytes, reqwest::Error>>,
        std::pin::Pin<Box<dyn futures::Stream<Item = Result<Bytes, reqwest::Error>> + Unpin + Send>>,
    ) {
        let (tx, mut rx) = tokio::sync::mpsc::channel::<Result<Bytes, reqwest::Error>>(16);
        let stream = futures::stream::poll_fn(move |cx| rx.poll_recv(cx));
        (tx, Box::pin(stream))
    }

    #[tokio::test(start_paused = true)]
    async fn test_chunk_idle_timeout_fires_on_stalled_stream() {
        // Stream sends one valid chunk then stalls forever.
        // With tokio time paused, the 60s idle timeout advances instantly.
        let (stream_tx, stream) = channel_stream();

        let (event_tx, _event_rx) = mpsc::channel(32);

        // Send one chunk (no [DONE] — stream stays open)
        stream_tx.send(Ok(Bytes::from(
            "data: {\"id\":\"chatcmpl-1\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"Hi\"},\"finish_reason\":null}]}\n\n"
        ))).await.unwrap();

        // Don't send anything else — let the idle timeout fire
        let result = parse_sse_stream(stream, &event_tx, "test-session", None).await;
        assert!(
            matches!(result, Err(AgentError::ChunkTimeout(60))),
            "expected ChunkTimeout(60), got: {result:?}"
        );
    }

    #[tokio::test(start_paused = true)]
    async fn test_chunk_idle_timeout_does_not_fire_when_data_flows() {
        // Stream sends chunks with delays shorter than the idle timeout.
        // All chunks arrive "in time", so the stream completes normally.
        let (stream_tx, stream) = channel_stream();

        let (event_tx, _event_rx) = mpsc::channel(32);

        // Spawn a task that sends chunks with 30s gaps (well under the 60s timeout)
        tokio::spawn(async move {
            stream_tx.send(Ok(Bytes::from(
                "data: {\"id\":\"chatcmpl-1\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"Hello\"},\"finish_reason\":null}]}\n\n"
            ))).await.unwrap();

            tokio::time::sleep(std::time::Duration::from_secs(30)).await;

            stream_tx.send(Ok(Bytes::from(
                "data: {\"id\":\"chatcmpl-1\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\" world\"},\"finish_reason\":null}]}\n\n"
            ))).await.unwrap();

            tokio::time::sleep(std::time::Duration::from_secs(30)).await;

            stream_tx.send(Ok(Bytes::from(
                "data: {\"id\":\"chatcmpl-1\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n"
            ))).await.unwrap();
        });

        let result = parse_sse_stream(stream, &event_tx, "test-session", None).await;
        let response = result.expect("stream should complete successfully");
        assert_eq!(response.content.as_deref(), Some("Hello world"));
        assert_eq!(response.finish_reason.as_deref(), Some("stop"));
    }

    #[test]
    fn test_request_with_thinking_config() {
        let req = ChatCompletionRequest {
            model: "test".into(),
            messages: vec![],
            stream: true,
            stream_options: None,
            tools: None,
            tool_choice: None,
            parallel_tool_calls: None,
            max_completion_tokens: None,
            temperature: None,
            thinking: Some(serde_json::json!({"type": "enabled", "budget_tokens": 10000})),
            prompt_cache_key: None,
            cache_control: None,
        };
        let json = serde_json::to_string(&req).unwrap();
        assert!(json.contains("\"thinking\""));
        assert!(json.contains("budget_tokens"));
    }
}
