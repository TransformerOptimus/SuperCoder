use std::sync::Arc;

use tokio::sync::mpsc;
use tokio_util::sync::CancellationToken;

use crate::approval::{ApprovalDecision, ApprovalHandler};
use crate::error::AgentError;
use crate::llm::client::{LlmClient, LlmProvider};
use crate::llm::types::{ChatMessage, ContentBlock, LlmResponse, MessageContent, ToolDefinition};
use crate::persistence::{AgentMessage, MessagePersister, MessageRole, MessageType, Sender};
use crate::tool::schema::validate_args;
use crate::tool::{ToolContext, ToolRegistry, ToolResult};
use crate::types::{AgentEvent, AgentResult};
use crate::util::{truncate_and_persist, truncate_str};
use super::compaction;
use super::config::AgentConfig;

const MAX_TOOL_OUTPUT_LINES: usize = 2000;
const MAX_TOOL_OUTPUT_BYTES: usize = 50 * 1024; // 50KB
const DOOM_LOOP_THRESHOLD: u32 = 3;

/// Message sent through the ordered persistence channel.
struct PersistItem {
    message: AgentMessage,
}

/// The core agent loop that drives LLM ↔ tool execution cycles.
pub struct AgentLoop {
    config: AgentConfig,
    client: Box<dyn LlmProvider>,
    /// Dedicated client for compaction summarization. Built from `config.compaction_llm`
    /// when present (typically a cheaper model like Sonnet), else None and we fall back
    /// to `client` for summarization.
    compaction_client: Option<Box<dyn LlmProvider>>,
    registry: Arc<ToolRegistry>,
    messages: Vec<ChatMessage>,
    cancel_token: CancellationToken,
    event_tx: mpsc::Sender<AgentEvent>,
    session_id: String,
    /// Ordered persistence channel — messages are written sequentially by a background worker.
    persist_tx: Option<mpsc::UnboundedSender<PersistItem>>,
    /// Handle to the persistence worker — awaited at end of run() to ensure all writes complete.
    persist_worker: Option<tokio::task::JoinHandle<()>>,
    /// Optional approval handler — when Some, tools are checked for approval before execution.
    approval_handler: Option<Arc<dyn ApprovalHandler>>,
    /// Actual token count from the most recent LLM API response.
    /// Updated after each `chat_completion` call from `response.usage.total_tokens`.
    /// Used for compaction decisions instead of the chars/4 heuristic when available.
    total_tokens_used: usize,
    /// Offset added to iteration counter for globally unique turn numbering
    /// across multiple AgentLoop invocations in the same thread.
    iteration_offset: u32,
    /// When this loop is a subagent child, the parent's session id. Stamped into
    /// every persisted message's metadata so child history stays linkable to the
    /// parent. `None` for top-level loops.
    parent_session_id: Option<String>,
}

impl AgentLoop {
    pub fn new(
        config: AgentConfig,
        registry: ToolRegistry,
        cancel_token: CancellationToken,
        event_tx: mpsc::Sender<AgentEvent>,
        session_id: String,
    ) -> Self {
        let client: Box<dyn LlmProvider> = Box::new(LlmClient::new(config.llm.clone()));
        Self::with_provider(config, client, registry, cancel_token, event_tx, session_id)
    }

    /// Create an agent loop with a custom LLM provider (useful for testing with mocks).
    pub fn with_provider(
        config: AgentConfig,
        client: Box<dyn LlmProvider>,
        registry: ToolRegistry,
        cancel_token: CancellationToken,
        event_tx: mpsc::Sender<AgentEvent>,
        session_id: String,
    ) -> Self {
        let mut messages = Vec::new();

        // System prompt: if configured, emit as a block-array message so each
        // segment can carry its own `cache_control` marker. Anthropic caches by
        // block; OpenAI ignores the markers and treats the joined content as a
        // single system string (same behavior as before).
        if let Some(ref blocks) = config.system_prompt {
            if blocks.len() == 1 && blocks[0].cache_control.is_none() {
                // Single uncached block → emit as a plain string for OpenAI-shape
                // minimalism. Functionally identical to a 1-element block array.
                messages.push(ChatMessage::system(blocks[0].text.clone()));
            } else {
                let content_blocks: Vec<ContentBlock> = blocks
                    .iter()
                    .map(|b| ContentBlock::Text {
                        text: b.text.clone(),
                        cache_control: b.cache_control.clone(),
                    })
                    .collect();
                messages.push(ChatMessage {
                    role: "system".into(),
                    content: Some(MessageContent::Blocks(content_blocks)),
                    tool_calls: None,
                    tool_call_id: None,
                    name: None,
                    thinking: None,
                });
            }
        }

        let compaction_client: Option<Box<dyn LlmProvider>> = config
            .compaction_llm
            .as_ref()
            .map(|cfg| Box::new(LlmClient::new(cfg.clone())) as Box<dyn LlmProvider>);

        Self {
            config,
            client,
            compaction_client,
            registry: Arc::new(registry),
            messages,
            cancel_token,
            event_tx,
            session_id,
            persist_tx: None,
            persist_worker: None,
            approval_handler: None,
            total_tokens_used: 0,
            iteration_offset: 0,
            parent_session_id: None,
        }
    }

    /// Attach an approval handler that gates tool execution on user approval.
    pub fn with_approval_handler(mut self, handler: Arc<dyn ApprovalHandler>) -> Self {
        self.approval_handler = Some(handler);
        self
    }

    /// Attach a persister — spawns a background worker that writes messages in order.
    /// Panics (in debug builds) if called more than once on the same AgentLoop.
    pub fn with_persister(mut self, persister: Arc<dyn MessagePersister>, session_id: String) -> Self {
        debug_assert!(self.persist_tx.is_none(), "with_persister called twice on the same AgentLoop");
        let (tx, mut rx) = mpsc::unbounded_channel::<PersistItem>();
        // Spawn a single ordered worker that drains messages sequentially.
        // session_id is captured once here — it never changes for the lifetime of an AgentLoop.
        let handle = tokio::spawn(async move {
            while let Some(item) = rx.recv().await {
                if let Err(e) = persister.persist_message(&item.message, &session_id).await {
                    log::error!("Persist failed: {e}");
                }
            }
        });
        self.persist_tx = Some(tx);
        self.persist_worker = Some(handle);
        self
    }

    /// Mark this loop as a subagent child of `parent_session_id`. The id is
    /// stamped into every persisted message's metadata so child history stays
    /// linkable to the parent session.
    pub fn with_parent_session_id(mut self, parent_session_id: String) -> Self {
        self.parent_session_id = Some(parent_session_id);
        self
    }

    /// Number of messages currently in the conversation context.
    pub fn message_count(&self) -> usize {
        self.messages.len()
    }

    /// Seed the token count from a previously persisted value (session resume).
    pub fn with_initial_token_count(mut self, tokens: usize) -> Self {
        self.total_tokens_used = tokens;
        self
    }

    /// Seed the iteration offset for globally unique turn numbering across
    /// multiple AgentLoop invocations in the same coding session thread.
    pub fn with_turn_offset(mut self, offset: u32) -> Self {
        log::info!("[AgentLoop {}] turn_offset set to {offset}", self.session_id);
        self.iteration_offset = offset;
        self
    }

    /// Update context limit when the user switches models mid-conversation.
    /// If the new limit is smaller and current token usage exceeds the new threshold,
    /// compaction will trigger automatically on the next loop iteration.
    pub fn update_context_limit(&mut self, new_context_window: usize) {
        let old = self.config.compaction_config.context_limit;
        self.config.compaction_config.context_limit = new_context_window;
        log::info!(
            "[AgentLoop {}] Context limit updated: {} → {}",
            self.session_id, old, new_context_window
        );
    }

    /// Seed the conversation with initial context messages (e.g., sliding window from ask mode).
    /// Seed the conversation with messages from a DIFFERENT scope (ask→coding transfer).
    /// These are COPIES of messages already posted to the chat service (in the ask session),
    /// so they're persisted as "completion_summary" type — which is excluded from the retry
    /// queue's `get_pending_messages` query. This prevents double-posting to chat service.
    ///
    /// ONLY use for cross-scope transfers (ask→coding, plan→coding). For same-scope resume
    /// (ask turn N, coding thread resume), use `with_resumed_context` instead.
    pub fn with_initial_context(mut self, messages: Vec<ChatMessage>) -> Self {
        for msg in &messages {
            let sender = if msg.role == "user" { Sender::HumanUser } else { Sender::Agent };
            self.persist_fire_and_forget(Self::chat_to_agent_message(msg, MessageType::CompletionSummary, sender, None));
        }
        self.messages.extend(messages);
        self
    }

    /// Reload prior context into the conversation buffer WITHOUT re-persisting.
    /// Use for same-scope resume (ask turn N, coding thread resume, rewind).
    /// The messages already exist in SQLite — writing copies would cause exponential
    /// duplication (each resume doubles the row count).
    pub fn with_resumed_context(mut self, messages: Vec<ChatMessage>) -> Self {
        self.messages.extend(messages);
        self
    }

    /// Run the agent loop with a user message. Returns an AgentResult indicating
    /// whether the agent completed normally or wants to start a coding session.
    pub async fn run(&mut self, user_message: ChatMessage) -> Result<AgentResult, AgentError> {
        let result = self.run_inner(user_message).await;
        // Flush persistence: drop the sender so the worker drains, then await it.
        // This guarantees ALL messages are written to SQLite before we return.
        self.persist_tx.take(); // drop sender — worker's recv() will return None after draining
        if let Some(worker) = self.persist_worker.take() {
            let _ = worker.await;
        }
        result
    }

    async fn run_inner(&mut self, user_message: ChatMessage) -> Result<AgentResult, AgentError> {
        // Push user message (caller provides ChatMessage directly — may contain image blocks)
        let user_text_preview: String = user_message.content.as_ref()
            .map_or_else(String::new, |c| c.text()[..c.text().len().min(100)].to_string());
        let user_turn = 1 + self.iteration_offset;
        if self.iteration_offset > 0 {
            log::info!("[AgentLoop {}] User message persisted with turn_count={user_turn} (1 + offset {})", self.session_id, self.iteration_offset);
        }
        self.persist_fire_and_forget(Self::chat_to_agent_message(
            &user_message,
            MessageType::Text,
            Sender::HumanUser,
            Some(user_turn),
        ));
        self.messages.push(user_message);

        log::info!(
            "[AgentLoop {}] run() started — {} messages in context, user_message: {:?}",
            self.session_id,
            self.messages.len(),
            user_text_preview
        );

        let tool_defs = self.registry.tool_definitions();
        let tool_names: Vec<&str> = tool_defs.iter().map(|d| d.function.name.as_str()).collect();
        log::info!(
            "[v1.0] Session {} | mode={:?} | tools=[{}] | doom_loop=enabled | max_iter={}",
            self.session_id, self.config.mode, tool_names.join(", "), self.config.max_iterations
        );
        let mut iteration: u32 = 0;
        let mut turn_modified_files: Vec<String> = Vec::new();
        let mut consecutive_failures: u32 = 0;
        let mut compaction_cooldown: u32 = 0;
        // Index into self.messages at which total_tokens_used was last measured.
        // Messages after this index were added since the last LLM response (tool results, etc.)
        // and need chars/4 estimation for the pre-LLM compaction check.
        let mut tokens_measured_at: usize = self.messages.len();

        // Safety: on session resume, total_tokens_used may be stale (seeded from a previous
        // session) or 0 (lookup failed). Re-estimate ALL messages so the compaction check
        // uses an accurate count on the first iteration.
        // Skip for truly fresh sessions (system prompt + user message only = 2 messages).
        if self.messages.len() > 2 {
            let full_estimate = compaction::estimate_token_count(&self.messages);
            log::info!(
                "[AgentLoop {}] Resume token check: seeded={}, full_estimate={}, context_limit={}, threshold={}",
                self.session_id, self.total_tokens_used, full_estimate,
                self.config.compaction_config.context_limit,
                (self.config.compaction_config.context_limit as f64 * self.config.compaction_config.threshold_pct) as usize
            );
            if full_estimate > self.total_tokens_used {
                log::info!(
                    "[AgentLoop {}] Stale token count corrected: seeded={}, re-estimated={}. Using estimate.",
                    self.session_id, self.total_tokens_used, full_estimate
                );
                self.total_tokens_used = full_estimate;
            }
        } else {
            log::info!(
                "[AgentLoop {}] Fresh session, total_tokens_used={}, messages={}",
                self.session_id, self.total_tokens_used, self.messages.len()
            );
        }

        loop {
            // Check cancellation
            if self.cancel_token.is_cancelled() {
                return Err(AgentError::Cancelled);
            }

            // Check iteration limit
            iteration += 1;
            if iteration > self.config.max_iterations {
                let summary = format!(
                    "Reached maximum of {} steps. Progress preserved.",
                    self.config.max_iterations
                );
                let _ = self.event_tx.send(AgentEvent::Done {
                    session_id: self.session_id.clone(),
                    summary: Some(summary.clone()),
                }).await;
                return Ok(AgentResult::Done { summary });
            }

            // Pre-LLM compaction check (Bug 3 fix).
            // total_tokens_used reflects the last LLM response, but tool results added
            // since then can push context way over the limit. We use the real token count
            // + chars/4 estimate of only the new messages added since measurement.
            if compaction_cooldown == 0 {
                let new_msg_tokens = if tokens_measured_at < self.messages.len() {
                    compaction::estimate_token_count(&self.messages[tokens_measured_at..])
                } else {
                    0
                };
                let estimated = self.total_tokens_used + new_msg_tokens;
                let needs = compaction::needs_compaction_by_tokens(estimated, self.messages.len(), &self.config.compaction_config);
                if iteration == 1 {
                    log::info!(
                        "[AgentLoop {}] Pre-LLM compaction check (iter {}): total_tokens_used={}, new_msg_tokens={}, estimated={}, messages={}, needs_compaction={}",
                        self.session_id, iteration, self.total_tokens_used, new_msg_tokens, estimated, self.messages.len(), needs
                    );
                }
                if needs {
                    log::info!(
                        "[AgentLoop {}] Compaction triggered at iter {}: estimated={}, messages={}",
                        self.session_id, iteration, estimated, self.messages.len()
                    );
                    compaction_cooldown = self.try_compaction().await;
                    tokens_measured_at = self.messages.len();
                }
            }

            // Pre-flight: strip orphaned tool_result messages before API call
            self.strip_orphaned_tool_results();

            // Call LLM with retry
            let response = self
                .call_llm_with_retry(&tool_defs)
                .await?;

            // Track actual token usage from API response.
            // The gateway normalizes total_tokens to include cache tokens for all
            // providers (Anthropic's adapter sums input + cache_read + cache_creation
            // + output). So usage.total_tokens is the real context size — no
            // client-side adjustment needed.
            if let Some(ref usage) = response.usage {
                let (cache_read, cache_write) = usage
                    .prompt_tokens_details
                    .as_ref()
                    .map(|d| (d.cached_tokens, d.cache_creation_tokens))
                    .unwrap_or((None, None));

                self.total_tokens_used = usage.total_tokens as usize;
                tokens_measured_at = self.messages.len(); // mark measurement point
                log::info!(
                    "[AgentLoop {}] tokens={}/{} ({}%)",
                    self.session_id, self.total_tokens_used, self.config.compaction_config.context_limit,
                    (self.total_tokens_used as f64 / self.config.compaction_config.context_limit as f64 * 100.0) as u32
                );
                if cache_read.unwrap_or(0) > 0 || cache_write.unwrap_or(0) > 0 {
                    log::info!(
                        "[AgentLoop {}] cache: read={} write={}",
                        self.session_id,
                        cache_read.unwrap_or(0),
                        cache_write.unwrap_or(0)
                    );
                }
                let _ = self.event_tx.send(AgentEvent::TokenUsage {
                    session_id: self.session_id.clone(),
                    total_tokens: usage.total_tokens,
                    context_limit: self.config.compaction_config.context_limit as u32,
                    cache_read_tokens: cache_read,
                    cache_creation_tokens: cache_write,
                }).await;
            }

            // Push assistant message to history
            let tool_calls = if response.tool_calls.is_empty() {
                None
            } else {
                Some(response.tool_calls.clone())
            };
            let has_tool_calls = tool_calls.is_some();
            let assistant_msg = ChatMessage::assistant(
                response.content.clone(),
                tool_calls,
                response.thinking.clone(),
            );
            let msg_type = if has_tool_calls { MessageType::ToolCall } else { MessageType::Text };
            self.persist_fire_and_forget(Self::chat_to_agent_message(
                &assistant_msg,
                msg_type,
                Sender::Agent,
                Some(iteration + self.iteration_offset),
            ));
            self.messages.push(assistant_msg);

            // If no tool calls, we're done
            if !has_tool_calls {
                let summary = response.content.unwrap_or_default();
                // Guard against the LLM returning an empty response with no tool_calls.
                // This happens occasionally after context compaction and produces a
                // spurious "Done" state that looks to the user like the agent silently
                // stopped. Surface it as an error with retry guidance instead.
                if summary.trim().is_empty() {
                    log::warn!(
                        "[AgentLoop {}] Empty response with no tool_calls — treating as error",
                        self.session_id
                    );
                    let _ = self.event_tx.send(AgentEvent::Error {
                        session_id: self.session_id.clone(),
                        message: "The model returned an empty response. This sometimes happens after context compaction. Please try again.".into(),
                        retrying: false,
                    }).await;
                    return Err(AgentError::LlmParseError(
                        "empty response with no tool_calls".into()
                    ));
                }
                let _ = self.event_tx.send(AgentEvent::Done {
                    session_id: self.session_id.clone(),
                    summary: Some(summary.clone()),
                }).await;
                return Ok(AgentResult::Done { summary });
            }

            // Execute tool calls — check cancellation once before the batch
            if self.cancel_token.is_cancelled() {
                return Err(AgentError::Cancelled);
            }

            // Parallel execution via JoinSet
            // Save (idx, tool_call_id) so we can recover context if a task panics
            let mut task_index: Vec<(usize, String)> = Vec::new();
            let mut join_set = tokio::task::JoinSet::new();
            for (idx, tool_call) in response.tool_calls.iter().enumerate() {
                task_index.push((idx, tool_call.id.clone()));

                let registry = Arc::clone(&self.registry);
                let working_dir = self.config.working_dir.clone();
                let cancel_token = self.cancel_token.clone();
                let event_tx = self.event_tx.clone();
                let session_id = self.session_id.clone();
                let tool_call_id = tool_call.id.clone();
                let tool_name = tool_call.function.name.clone();
                let arguments_json = tool_call.function.arguments.clone();
                let approval_ref = self.approval_handler.clone();
                let checkpoint_dir = self.config.checkpoint_dir.clone();
                let checkpoint_turn = iteration + self.iteration_offset;

                join_set.spawn(async move {
                    let result = execute_tool_call_impl(
                        &tool_call_id,
                        &tool_name,
                        &arguments_json,
                        &registry,
                        &working_dir,
                        cancel_token,
                        &event_tx,
                        &session_id,
                        approval_ref.as_deref(),
                        checkpoint_dir,
                        checkpoint_turn,
                    )
                    .await;
                    (idx, tool_call_id, result)
                });
            }

            // Collect results and sort by original index
            let mut results = Vec::new();
            while let Some(join_result) = join_set.join_next().await {
                match join_result {
                    Ok(tuple) => results.push(tuple),
                    Err(e) => {
                        log::error!("Tool task panicked: {e}");
                    }
                }
            }

            // If a task panicked, its tool_call_id is missing from results.
            // Inject synthetic error responses to avoid API protocol violations.
            let completed_ids: std::collections::HashSet<String> =
                results.iter().map(|(_, id, _)| id.clone()).collect();
            for (idx, tc_id) in &task_index {
                if !completed_ids.contains(tc_id) {
                    log::error!("Injecting synthetic error for panicked tool call: {tc_id}");
                    results.push((*idx, tc_id.clone(), ToolResult::error(
                        "Tool execution crashed unexpectedly. Please try again.",
                    )));
                }
            }

            results.sort_by_key(|(idx, _, _)| *idx);

            // Doom loop detection: track consecutive all-failed tool rounds
            let all_failed = !results.is_empty() && results.iter().all(|(_, _, r)| r.is_error);
            if all_failed {
                consecutive_failures += 1;
            } else {
                consecutive_failures = 0;
            }

            // Push tool results to message history FIRST (before checking yield)
            let mut yield_data: Option<serde_json::Value> = None;
            for (_, tc_id, result) in &results {
                let tool_msg = ChatMessage::tool_result(tc_id, &result.output);
                self.persist_fire_and_forget(Self::chat_to_agent_message(
                    &tool_msg,
                    MessageType::ToolResult,
                    Sender::Agent,
                    Some(iteration + self.iteration_offset),
                ));
                self.messages.push(tool_msg);
                // Capture the first yield_data if any
                if yield_data.is_none() {
                    if let Some(ref data) = result.yield_data {
                        yield_data = Some(data.clone());
                    }
                }
                // Accumulate modified files for TurnCompleted
                turn_modified_files.extend(result.modified_files.iter().cloned());
            }

            // Doom loop: if N consecutive rounds had ALL tools fail, inject a hint
            if consecutive_failures >= DOOM_LOOP_THRESHOLD {
                log::warn!(
                    "[AgentLoop {}] Doom loop detected: {} consecutive all-failed tool rounds",
                    self.session_id, consecutive_failures
                );
                let hint = ChatMessage::system(
                    "You have failed multiple consecutive tool calls. Stop retrying the same approach. \
                     Either try a completely different strategy, or explain to the user what is blocking you.",
                );
                self.messages.push(hint);
                consecutive_failures = 0;

                let _ = self.event_tx.send(AgentEvent::Error {
                    session_id: self.session_id.clone(),
                    message: format!(
                        "Doom loop detected: {} consecutive tool failures — injecting course-correction hint",
                        DOOM_LOOP_THRESHOLD
                    ),
                    retrying: true,
                }).await;
            }

            // Check for yield_data (e.g., save_plan, ask_user) — after results are persisted.
            // Note: TurnCompleted is intentionally NOT emitted on yield. Ask/Plan modes
            // (the only modes with yielding tools) have no file-modifying tools,
            // so turn_modified_files is always empty here.
            if let Some(ref data) = yield_data {
                let yield_type = data["yield_type"].as_str().unwrap_or_default();

                match yield_type {
                    "save_plan" => {
                        let plan = data["plan"]
                            .as_str()
                            .unwrap_or_default()
                            .to_string();
                        let plan_path = data["plan_path"]
                            .as_str()
                            .unwrap_or_default()
                            .to_string();

                        log::info!("[v1.0] save_plan yield — plan saved to {}", plan_path);

                        let _ = self.event_tx.send(AgentEvent::PlanReady {
                            session_id: self.session_id.clone(),
                            plan: plan.clone(),
                            plan_path: plan_path.clone(),
                            project_path: self.config.working_dir.to_string_lossy().to_string(),
                        }).await;

                        return Ok(AgentResult::PlanReady { plan, plan_path });
                    }
                    "ask_user" => {
                        log::info!("[v1.0] ask_user yield triggered — agent asking user a question");
                        let question = data["question"]
                            .as_str()
                            .unwrap_or_default()
                            .to_string();
                        let options: Option<Vec<String>> = data["options"].as_array().map(|arr| {
                            arr.iter()
                                .filter_map(|v| v.as_str().map(String::from))
                                .collect()
                        });

                        let _ = self.event_tx.send(AgentEvent::UserQuestionAsked {
                            session_id: self.session_id.clone(),
                            question: question.clone(),
                            options: options.clone(),
                        }).await;

                        return Ok(AgentResult::AskUser { question, options });
                    }
                    other => {
                        // Unknown yield type — log and ignore rather than acting on it.
                        log::warn!("[agent] ignoring unknown yield_type: {other:?}");
                    }
                }
            }

            // Check cancellation after tool execution
            if self.cancel_token.is_cancelled() {
                // Emit TurnCompleted before returning — tools already ran and may have
                // modified files on disk. The relay needs to know about these changes.
                if !turn_modified_files.is_empty() {
                    let _ = self.event_tx.send(AgentEvent::TurnCompleted {
                        session_id: self.session_id.clone(),
                        turn_count: iteration + self.iteration_offset,
                        modified_files: std::mem::take(&mut turn_modified_files),
                    }).await;
                }
                return Err(AgentError::Cancelled);
            }

            // Post-tool compaction check (using actual token count from API)
            if compaction_cooldown > 0 {
                compaction_cooldown -= 1;
            } else if compaction::needs_compaction_by_tokens(self.total_tokens_used, self.messages.len(), &self.config.compaction_config) {
                compaction_cooldown = self.try_compaction().await;
                tokens_measured_at = self.messages.len();
            }

            // Emit turn boundary event
            let _ = self.event_tx.send(AgentEvent::TurnCompleted {
                session_id: self.session_id.clone(),
                turn_count: iteration + self.iteration_offset,
                modified_files: std::mem::take(&mut turn_modified_files),
            }).await;

            // Continue loop — send tool results back to LLM
        }
    }

    /// Send a message to the ordered persistence worker. Non-blocking.
    fn persist_fire_and_forget(&self, mut msg: AgentMessage) {
        if let Some(ref tx) = self.persist_tx {
            // For subagent children, record the parent session id in metadata so
            // child history stays linkable to the parent (replaces the old
            // composite child-session naming scheme).
            if let Some(ref parent) = self.parent_session_id {
                if let Some(obj) = msg.metadata.as_object_mut() {
                    obj.insert("parent_session_id".to_string(), serde_json::json!(parent));
                }
            }
            let _ = tx.send(PersistItem { message: msg });
        }
    }

    /// Convert a ChatMessage to an AgentMessage for persistence.
    fn chat_to_agent_message(
        msg: &ChatMessage,
        message_type: MessageType,
        sender: Sender,
        turn_count: Option<u32>,
    ) -> AgentMessage {
        let content = msg.content.as_ref().map(|c| c.text().to_string()).unwrap_or_default();
        let llm_message = serde_json::to_value(msg).unwrap_or_default();
        AgentMessage {
            content,
            llm_message,
            metadata: serde_json::json!({}),
            role: match msg.role.as_str() {
                "user" => MessageRole::User,
                "assistant" => MessageRole::Assistant,
                "tool" => MessageRole::Tool,
                "system" => MessageRole::System,
                _ => MessageRole::User,
            },
            message_type,
            sender,
            turn_count,
        }
    }

    async fn call_llm_with_retry(
        &self,
        tool_defs: &[ToolDefinition],
    ) -> Result<LlmResponse, AgentError> {
        let max_retries = self.config.retry_config.max_retries;
        let mut delay = self.config.retry_config.initial_delay;
        let mut last_error: Option<AgentError> = None;

        for attempt in 0..=max_retries {
            if attempt > 0 {
                // Emit retry event
                let _ = self.event_tx.send(AgentEvent::Error {
                    session_id: self.session_id.clone(),
                    message: format!(
                        "LLM call failed, retrying (attempt {}/{}): {}",
                        attempt,
                        max_retries,
                        last_error.as_ref().map(|e| e.to_string()).unwrap_or_default()
                    ),
                    retrying: true,
                }).await;

                tokio::time::sleep(delay).await;
                delay = std::time::Duration::from_secs_f64(
                    delay.as_secs_f64() * self.config.retry_config.multiplier,
                ).min(self.config.retry_config.max_delay);
            }

            match self
                .client
                .chat_completion(&self.messages, tool_defs, &self.event_tx, &self.session_id, Some(&self.cancel_token))
                .await
            {
                Ok(response) => return Ok(response),
                Err(e) => {
                    // Don't retry on cancellation
                    if matches!(e, AgentError::Cancelled) {
                        return Err(e);
                    }
                    // Don't retry on permanent client errors (only retry transient/server errors)
                    if let AgentError::LlmApiError { status, body } = &e {
                        match *status {
                            429 | 500 | 502 | 503 | 504 => {} // retryable
                            _ => {
                                // Permanent — don't retry. Emit an explicit error event so
                                // the UI sees the failure even if the session-level watcher's
                                // Error+Done emission races with relay teardown.
                                let _ = self.event_tx.send(AgentEvent::Error {
                                    session_id: self.session_id.clone(),
                                    message: format!(
                                        "LLM API error (status {}): {}",
                                        status, body
                                    ),
                                    retrying: false,
                                }).await;
                                return Err(e);
                            }
                        }
                    }
                    last_error = Some(e);
                }
            }
        }

        // All retries exhausted
        let error = last_error.expect("at least one error must have occurred if all retries exhausted");
        let _ = self.event_tx.send(AgentEvent::Error {
            session_id: self.session_id.clone(),
            message: format!("LLM call failed after {} retries: {}", max_retries, error),
            retrying: false,
        }).await;
        Err(error)
    }

    /// Attempt compaction with retry and aggressive truncation fallback.
    /// Returns the cooldown (number of iterations to skip before next check).
    async fn try_compaction(&mut self) -> u32 {
        let Some((start, end)) = compaction::compaction_boundaries(
            &self.messages,
            self.config.compaction_config.keep_recent_messages,
        ) else {
            return 0;
        };

        log::info!(
            "[AgentLoop {}] Compacting [{}..{}) of {} messages",
            self.session_id, start, end, self.messages.len()
        );

        // Try LLM summarization (with one retry)
        let summary_result = self.try_llm_summarization(start, end).await;

        match summary_result {
            Some(summary_text) => {
                let summary_msg = ChatMessage::user(format!(
                    "[Context summary from earlier in this conversation]\n{summary_text}"
                ));

                // Bug 1 fix: write kept_before_count into metadata
                let kept_before = start; // messages before compaction range (typically 1 = system prompt)
                let mut agent_msg = Self::chat_to_agent_message(
                    &summary_msg,
                    MessageType::Compaction,
                    Sender::Agent,
                    None,
                );
                agent_msg.metadata = serde_json::json!({
                    "version": 1,
                    "kept_before_count": kept_before,
                });
                self.persist_fire_and_forget(agent_msg);

                self.apply_compaction(start, end, summary_msg).await;
            }
            None => {
                // Bug 4 fallback: aggressive truncation — drop messages without summarization
                log::warn!(
                    "[AgentLoop {}] Compaction LLM failed twice — falling back to aggressive truncation",
                    self.session_id
                );
                let truncation_msg = ChatMessage::user(
                    "[Earlier context was truncated due to length. Some conversation history has been lost.]"
                        .to_string(),
                );

                let kept_before = start;
                let mut agent_msg = Self::chat_to_agent_message(
                    &truncation_msg,
                    MessageType::Compaction,
                    Sender::Agent,
                    None,
                );
                agent_msg.metadata = serde_json::json!({
                    "version": 1,
                    "kept_before_count": kept_before,
                    "truncated": true,
                });
                self.persist_fire_and_forget(agent_msg);

                self.apply_compaction(start, end, truncation_msg).await;
            }
        }

        // Bug 2 fix: cooldown — skip compaction checks for 2 iterations
        2
    }

    /// Try LLM summarization with one retry. Returns None if both attempts fail.
    async fn try_llm_summarization(&self, start: usize, end: usize) -> Option<String> {
        let to_compact = &self.messages[start..end];
        let prompt = compaction::build_compaction_prompt(to_compact);
        let compact_msgs = vec![
            ChatMessage::system(
                "Summarize this conversation. Structure your summary as:\n\
                 ## Goal\nWhat the user is trying to accomplish.\n\
                 ## Key Decisions\nImportant choices made and why.\n\
                 ## Work Completed\nWhat was done — files modified, code written, tests run.\n\
                 ## Current State\nWhere things stand — what works, what's broken, what's next.\n\
                 ## Relevant Files\nFile paths referenced or modified.\n\n\
                 Preserve exact file paths, function names, error messages, and technical details. Be concise but complete."
            ),
            ChatMessage::user(prompt),
        ];

        // Bare `_` drops the receiver immediately so send() never blocks.
        let (silent_tx, _) = tokio::sync::mpsc::channel(1);

        // Prefer the dedicated compaction client (cheaper model) when configured;
        // fall back to the main client otherwise. Summarization is a formatting task —
        // it does not need the same model as the main loop.
        let summarizer: &dyn LlmProvider = self
            .compaction_client
            .as_deref()
            .unwrap_or(self.client.as_ref());

        for attempt in 1..=2 {
            match summarizer.chat_completion(&compact_msgs, &[], &silent_tx, &self.session_id, Some(&self.cancel_token)).await {
                Ok(response) => {
                    let text = response.content.unwrap_or_default();
                    if !text.trim().is_empty() {
                        return Some(text);
                    }
                    log::warn!(
                        "[AgentLoop {}] Compaction LLM attempt {}/2 returned empty content",
                        self.session_id, attempt
                    );
                }
                Err(e) => {
                    // Don't retry on cancellation — let the caller handle it
                    if matches!(e, AgentError::Cancelled) {
                        return None;
                    }
                    log::error!(
                        "[AgentLoop {}] Compaction LLM attempt {}/2 failed: {e}",
                        self.session_id, attempt
                    );
                }
            }
        }

        None
    }

    /// Remove tool_result messages whose corresponding assistant+tool_use was lost,
    /// and clear `tool_calls` from assistant messages whose results were lost.
    /// Both cases are rejected by Anthropic with `unexpected tool_use_id` /
    /// `tool_use without tool_result` errors. This runs before every LLM call AND
    /// after every compaction (defense in depth).
    fn strip_orphaned_tool_results(&mut self) {
        // Phase 1: collect all tool_call_ids that exist in any assistant message.
        let mut valid_tool_ids = std::collections::HashSet::new();
        for msg in &self.messages {
            if let Some(ref tcs) = msg.tool_calls {
                for tc in tcs {
                    valid_tool_ids.insert(tc.id.clone());
                }
            }
        }

        // Phase 2: drop tool_result messages whose tool_call_id is unknown.
        let before = self.messages.len();
        self.messages.retain(|msg| {
            if msg.role == "tool" {
                if let Some(ref id) = msg.tool_call_id {
                    return valid_tool_ids.contains(id);
                }
            }
            true
        });
        let removed_results = before - self.messages.len();

        // Phase 3: collect tool_call_ids that have at least one matching tool_result
        // in the (possibly-trimmed) message list.
        let mut answered_tool_ids = std::collections::HashSet::new();
        for msg in &self.messages {
            if msg.role == "tool" {
                if let Some(ref id) = msg.tool_call_id {
                    answered_tool_ids.insert(id.clone());
                }
            }
        }

        // Phase 4: for any assistant message with tool_calls, drop tool_calls whose
        // results are missing. If ALL of an assistant's tool_calls are unanswered,
        // clear the field entirely (Anthropic rejects an assistant with dangling
        // tool_use blocks just as harshly as orphan tool_results).
        let mut cleared_calls = 0usize;
        for msg in &mut self.messages {
            if msg.role != "assistant" {
                continue;
            }
            let Some(ref mut tcs) = msg.tool_calls else { continue };
            let original = tcs.len();
            tcs.retain(|tc| answered_tool_ids.contains(&tc.id));
            cleared_calls += original - tcs.len();
            if tcs.is_empty() {
                msg.tool_calls = None;
            }
        }

        if removed_results > 0 || cleared_calls > 0 {
            log::warn!(
                "[AgentLoop {}] Sanitized message history: removed {removed_results} orphan tool_results, cleared {cleared_calls} dangling tool_calls",
                self.session_id
            );
        }
    }

    /// Replace compacted messages with a summary, reset token count, emit event.
    async fn apply_compaction(&mut self, start: usize, end: usize, summary_msg: ChatMessage) {
        let old_count = self.messages.len();
        let mut new_messages = Vec::new();
        if start > 0 {
            new_messages.extend_from_slice(&self.messages[..start]);
        }
        new_messages.push(summary_msg);
        new_messages.extend_from_slice(&self.messages[end..]);
        self.messages = new_messages;

        // Bug 3 fix: defense in depth — sanitize immediately after the splice.
        // The boundary snap should already prevent orphans, but compaction is one
        // of two writers to self.messages and the only one that removes messages.
        // Running the strip here guarantees orphan-free state regardless of which
        // call path triggers the next LLM call.
        self.strip_orphaned_tool_results();

        log::info!(
            "[AgentLoop {}] Compaction: {} → {} messages, tokens reset from {}",
            self.session_id, old_count, self.messages.len(), self.total_tokens_used
        );

        // Re-estimate from the compacted messages and persist the post-compaction count.
        // Without this, the persisted value would be the pre-compaction count (from the
        // last TokenUsage event), causing stale-high seeding on next resume.
        let post_compaction_estimate = compaction::estimate_token_count(&self.messages);
        self.total_tokens_used = post_compaction_estimate;

        let _ = self.event_tx.send(AgentEvent::TokenUsage {
            session_id: self.session_id.clone(),
            total_tokens: post_compaction_estimate as u32,
            context_limit: self.config.compaction_config.context_limit as u32,
            cache_read_tokens: None,
            cache_creation_tokens: None,
        }).await;

        let _ = self.event_tx.send(AgentEvent::Compaction {
            session_id: self.session_id.clone(),
        }).await;
    }

}

/// Free function for tool execution — can be sent to JoinSet tasks.
#[allow(clippy::too_many_arguments)]
async fn execute_tool_call_impl(
    tool_call_id: &str,
    tool_name: &str,
    arguments_json: &str,
    registry: &ToolRegistry,
    working_dir: &std::path::Path,
    cancel_token: CancellationToken,
    event_tx: &mpsc::Sender<AgentEvent>,
    session_id: &str,
    approval_handler: Option<&dyn ApprovalHandler>,
    checkpoint_dir: Option<std::path::PathBuf>,
    checkpoint_turn: u32,
) -> ToolResult {
    // Parse arguments first — if this fails, emit a basic ToolStart before the error ToolEnd
    let args: serde_json::Value = match serde_json::from_str(arguments_json) {
        Ok(v) => v,
        Err(e) => {
            let err_msg = format!("Invalid JSON in tool arguments: {e}");
            let _ = event_tx.send(AgentEvent::ToolStart {
                session_id: session_id.to_string(),
                tool_call_id: tool_call_id.to_string(),
                tool_name: tool_name.to_string(),
                args_summary: format!("Executing {tool_name}"),
            }).await;
            let _ = event_tx.send(AgentEvent::ToolEnd {
                session_id: session_id.to_string(),
                tool_call_id: tool_call_id.to_string(),
                success: false,
                summary: err_msg.clone(),
                modified_files: None,
            }).await;
            return ToolResult::error(err_msg);
        }
    };

    // Build nice summary from parsed args and emit ToolStart
    let args_summary = build_args_summary(tool_name, &args, working_dir);
    let _ = event_tx.send(AgentEvent::ToolStart {
        session_id: session_id.to_string(),
        tool_call_id: tool_call_id.to_string(),
        tool_name: tool_name.to_string(),
        args_summary: args_summary.clone(),
    }).await;

    // Look up tool
    let tool = match registry.get(tool_name) {
        Some(t) => t,
        None => {
            let err_msg = format!("Unknown tool: '{tool_name}'. Available tools: {}",
                registry.tool_definitions().iter().map(|t| t.function.name.as_str()).collect::<Vec<_>>().join(", "));
            let _ = event_tx.send(AgentEvent::ToolEnd {
                session_id: session_id.to_string(),
                tool_call_id: tool_call_id.to_string(),
                success: false,
                summary: err_msg.clone(),
                modified_files: None,
            }).await;
            return ToolResult::error(err_msg);
        }
    };

    // Validate args against schema
    let schema = tool.parameters_schema();
    if let Err(validation_err) = validate_args(&args, &schema) {
        let err_msg = format!("Invalid arguments for tool '{tool_name}': {validation_err}");
        let _ = event_tx.send(AgentEvent::ToolEnd {
            session_id: session_id.to_string(),
            tool_call_id: tool_call_id.to_string(),
            success: false,
            summary: err_msg.clone(),
            modified_files: None,
        }).await;
        return ToolResult::error(err_msg);
    }

    // Check if approval is needed
    if let Some(handler) = approval_handler {
        let decision = tokio::select! {
            d = handler.request_approval(tool_name, tool_call_id, &args, &args_summary) => d,
            _ = cancel_token.cancelled() => {
                ApprovalDecision::Denied { reason: Some("Session cancelled".to_string()) }
            }
        };

        if let ApprovalDecision::Denied { reason } = decision {
            let reason_text = reason.unwrap_or_else(|| "User denied".to_string());
            let err_msg = format!("Permission denied for tool '{tool_name}': {reason_text}");
            let _ = event_tx.send(AgentEvent::ToolEnd {
                session_id: session_id.to_string(),
                tool_call_id: tool_call_id.to_string(),
                success: false,
                summary: err_msg.clone(),
                modified_files: None,
            }).await;
            return ToolResult::error(err_msg);
        }
    }

    // Extract file path for file-modifying tools before args are moved
    let modified_file_path = match tool_name {
        "write" | "edit" => args
            .get("file_path")
            .or_else(|| args.get("filePath"))
            .and_then(|v| v.as_str())
            .map(|p| p.to_string()),
        _ => None,
    };

    // Path safety check: write/edit outside the working directory ALWAYS requires
    // explicit user approval, regardless of permission level. The [SECURITY] prefix
    // tells the PermissionAwareApprovalHandler to bypass auto-approve.
    if let Some(ref file_path_str) = modified_file_path {
        let resolved = crate::util::resolve_path(working_dir, file_path_str);
        let is_within = crate::util::is_path_within_working_dir(working_dir, &resolved);
        if !is_within {
            // Use ~ shorthand for home dir in the display path
            let display_path = {
                let p = resolved.display().to_string();
                match std::env::var("HOME") {
                    Ok(home) if p.starts_with(&home) => {
                        format!("~{}", &p[home.len()..])
                    }
                    _ => p,
                }
            };
            let outside_summary = format!(
                "[SECURITY] Write to {display_path} (outside project)"
            );
            log::warn!("[PathGuard] Outside-project write detected, requesting user approval");

            if let Some(handler) = approval_handler {
                let decision = tokio::select! {
                    d = handler.request_approval(tool_name, tool_call_id, &args, &outside_summary) => d,
                    _ = cancel_token.cancelled() => {
                        ApprovalDecision::Denied { reason: Some("Session cancelled".to_string()) }
                    }
                };

                if let ApprovalDecision::Denied { reason } = decision {
                    let reason_text = reason.unwrap_or_else(|| "User denied".to_string());
                    let err_msg = format!(
                        "Path '{}' is outside the project directory. User denied: {reason_text}",
                        resolved.display()
                    );
                    let _ = event_tx.send(AgentEvent::ToolEnd {
                        session_id: session_id.to_string(),
                        tool_call_id: tool_call_id.to_string(),
                        success: false,
                        summary: err_msg.clone(),
                        modified_files: None,
                    }).await;
                    return ToolResult::error(err_msg);
                }
                log::info!("[PathGuard] User approved outside-project write to '{}'", resolved.display());
            } else {
                // No approval handler — block for safety
                let err_msg = format!(
                    "Path '{}' is outside the project directory '{}'. No approval handler to ask user.",
                    resolved.display(), working_dir.display()
                );
                let _ = event_tx.send(AgentEvent::ToolEnd {
                    session_id: session_id.to_string(),
                    tool_call_id: tool_call_id.to_string(),
                    success: false,
                    summary: err_msg.clone(),
                    modified_files: None,
                }).await;
                return ToolResult::error(err_msg);
            }
        }
    }

    // Execute
    let ctx = ToolContext {
        working_dir: working_dir.to_path_buf(),
        cancel_token,
        event_tx: event_tx.clone(),
        session_id: session_id.to_string(),
        tool_call_id: tool_call_id.to_string(),
        checkpoint_dir,
        checkpoint_turn,
    };

    let mut result = match tool.execute(args, &ctx).await {
        Ok(mut r) => {
            let truncated = truncate_and_persist(
                &r.output,
                MAX_TOOL_OUTPUT_LINES,
                MAX_TOOL_OUTPUT_BYTES,
                working_dir,
                tool_call_id,
            ).await;
            r.output = truncated;
            r
        }
        Err(e) => ToolResult::error(format!("Tool execution error: {e}")),
    };

    // Propagate modified file path onto the result for TurnCompleted tracking
    if !result.is_error {
        if let Some(ref p) = modified_file_path {
            if result.modified_files.is_empty() {
                result.modified_files = vec![p.clone()];
            }
        }
    }

    // Use result.modified_files for ToolEnd event
    let modified_files = if !result.is_error && !result.modified_files.is_empty() {
        Some(result.modified_files.clone())
    } else {
        None
    };

    // Emit ToolEnd
    let summary = if result.output.len() > 200 {
        format!("{}...", truncate_str(&result.output, 200))
    } else {
        result.output.clone()
    };
    let _ = event_tx.send(AgentEvent::ToolEnd {
        session_id: session_id.to_string(),
        tool_call_id: tool_call_id.to_string(),
        success: !result.is_error,
        summary,
        modified_files,
    }).await;

    result
}

/// Shorten a file path for display: strip the working dir prefix to show a relative path.
/// If the path is outside working_dir, show it as-is.
fn shorten_path(path: &str, working_dir: &std::path::Path) -> String {
    let wd = working_dir.to_string_lossy();
    // Strip the project dir prefix (e.g., /Users/me/project/src/foo.rs → src/foo.rs)
    if let Some(rel) = path.strip_prefix(wd.as_ref()) {
        let rel = rel.strip_prefix('/').unwrap_or(rel);
        if rel.is_empty() { ".".to_string() } else { rel.to_string() }
    } else {
        path.to_string()
    }
}

/// Build a human-readable summary of tool arguments for the ToolStart event.
fn build_args_summary(tool_name: &str, args: &serde_json::Value, working_dir: &std::path::Path) -> String {
    match tool_name {
        "read" => {
            let path = args.get("filePath").and_then(|v| v.as_str()).unwrap_or("?");
            format!("Reading {}", shorten_path(path, working_dir))
        }
        "write" => {
            let path = args.get("filePath").and_then(|v| v.as_str()).unwrap_or("?");
            format!("Writing {}", shorten_path(path, working_dir))
        }
        "bash" => {
            let desc = args.get("description").and_then(|v| v.as_str()).unwrap_or("?");
            desc.to_string()
        }
        "edit" => {
            let path = args.get("filePath").and_then(|v| v.as_str()).unwrap_or("?");
            format!("Editing {}", shorten_path(path, working_dir))
        }
        "glob" => {
            let pattern = args.get("pattern").and_then(|v| v.as_str()).unwrap_or("?");
            format!("Searching for {pattern}")
        }
        "grep" => {
            let pattern = args.get("pattern").and_then(|v| v.as_str()).unwrap_or("?");
            format!("Searching for /{pattern}/")
        }
        "git" => {
            let desc = args.get("description").and_then(|v| v.as_str()).unwrap_or("?");
            desc.to_string()
        }
        "create_pr" => {
            let title = args.get("title").and_then(|v| v.as_str()).unwrap_or("?");
            format!("Creating PR: {title}")
        }
        "save_plan" => {
            let filename = args.get("filename").and_then(|v| v.as_str()).unwrap_or("plan.md");
            format!("Saving plan: {filename}")
        }
        "edit_plan" => {
            let path = args.get("file_path").and_then(|v| v.as_str());
            match path {
                Some(p) => format!("Editing plan: {}", shorten_path(p, working_dir)),
                None => "Editing plan".to_string(),
            }
        }
        "todo_write" => {
            let first = args
                .get("todos")
                .and_then(|v| v.as_array())
                .and_then(|a| a.first())
                .and_then(|t| t.get("content"))
                .and_then(|v| v.as_str());
            match first {
                Some(content) => format!("Updating todos: {content}"),
                None => "Updating todos".to_string(),
            }
        }
        "todo_read" => "Reading todos".to_string(),
        "codebase_search" => {
            let query = args.get("query").and_then(|v| v.as_str()).unwrap_or("?");
            format!("Semantic search: {query}")
        }
        "codebase_graph" => {
            let query = args.get("query").and_then(|v| v.as_str())
                .or_else(|| args.get("function_name").and_then(|v| v.as_str()))
                .unwrap_or("?");
            format!("Querying graph: {query}")
        }
        "ask_user" => {
            let question = args.get("question").and_then(|v| v.as_str()).unwrap_or("?");
            let short = truncate_str(question, 60);
            format!("Asking: {short}")
        }
        "skill" => {
            let name = args.get("name").and_then(|v| v.as_str()).unwrap_or("?");
            format!("Loading skill: {name}")
        }
        _ => {
            // Generic: show first 100 chars of serialized args
            let s = args.to_string();
            if s.len() > 100 {
                format!("{}...", truncate_str(&s, 100))
            } else {
                s
            }
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::collections::VecDeque;
    use std::sync::{Arc, Mutex};

    use serde_json::json;

    use crate::agent::config::RetryConfig;
    use crate::error::ToolError;
    use crate::llm::types::{FunctionCall, LlmResponse, ToolCall};
    use crate::test_util::{MockLlm, text_response};
    use crate::tool::{Tool, ToolRegistry};
    use crate::types::AgentResult;
    use crate::util::truncate_output;

    // ── Capturing LLM (records messages sent to it) ──

    struct CapturingLlm {
        responses: Mutex<VecDeque<Result<LlmResponse, AgentError>>>,
        captured: Mutex<Vec<Vec<ChatMessage>>>,
    }

    #[async_trait::async_trait]
    impl LlmProvider for CapturingLlm {
        async fn chat_completion(
            &self,
            messages: &[ChatMessage],
            _tools: &[ToolDefinition],
            _event_tx: &mpsc::Sender<AgentEvent>,
            _session_id: &str,
            _cancel_token: Option<&CancellationToken>,
        ) -> Result<LlmResponse, AgentError> {
            self.captured.lock().unwrap().push(messages.to_vec());
            self.responses
                .lock()
                .unwrap()
                .pop_front()
                .expect("CapturingLlm: no more responses queued")
        }
    }

    /// Wrapper to use Arc<dyn LlmProvider> as Box<dyn LlmProvider>.
    struct ArcLlm(Arc<dyn LlmProvider>);

    #[async_trait::async_trait]
    impl LlmProvider for ArcLlm {
        async fn chat_completion(
            &self,
            messages: &[ChatMessage],
            tools: &[ToolDefinition],
            event_tx: &mpsc::Sender<AgentEvent>,
            session_id: &str,
            cancel_token: Option<&CancellationToken>,
        ) -> Result<LlmResponse, AgentError> {
            self.0.chat_completion(messages, tools, event_tx, session_id, cancel_token).await
        }
    }

    // ── Echo Tool (for testing tool dispatch) ──

    struct EchoTool;

    #[async_trait::async_trait]
    impl Tool for EchoTool {
        fn name(&self) -> &str {
            "echo"
        }
        fn description(&self) -> &str {
            "Echoes the message"
        }
        fn parameters_schema(&self) -> serde_json::Value {
            json!({
                "type": "object",
                "required": ["message"],
                "properties": {
                    "message": { "type": "string" }
                }
            })
        }
        async fn execute(
            &self,
            args: serde_json::Value,
            _ctx: &ToolContext,
        ) -> Result<ToolResult, ToolError> {
            let msg = args
                .get("message")
                .and_then(|v| v.as_str())
                .unwrap_or("")
                .to_string();
            Ok(ToolResult::success(msg))
        }
    }

    // ── CancelTriggerTool (cancels the token when executed) ──

    struct CancelTriggerTool;

    #[async_trait::async_trait]
    impl Tool for CancelTriggerTool {
        fn name(&self) -> &str {
            "cancel_trigger"
        }
        fn description(&self) -> &str {
            "Triggers cancellation"
        }
        fn parameters_schema(&self) -> serde_json::Value {
            json!({"type": "object"})
        }
        async fn execute(
            &self,
            _args: serde_json::Value,
            ctx: &ToolContext,
        ) -> Result<ToolResult, ToolError> {
            ctx.cancel_token.cancel();
            Ok(ToolResult::success("cancelled"))
        }
    }

    // ── Helpers ──

    fn tool_call_response(id: &str, name: &str, args: &str) -> LlmResponse {
        LlmResponse {
            content: None,
            tool_calls: vec![ToolCall {
                id: id.to_string(),
                type_: "function".to_string(),
                function: FunctionCall {
                    name: name.to_string(),
                    arguments: args.to_string(),
                },
            }],
            usage: None,
            finish_reason: Some("tool_calls".to_string()),
            thinking: None,
        }
    }

    fn tool_call_response_with_usage(id: &str, name: &str, args: &str, total_tokens: u32) -> LlmResponse {
        let mut resp = tool_call_response(id, name, args);
        resp.usage = Some(crate::llm::types::Usage {
            prompt_tokens: total_tokens.saturating_sub(10),
            completion_tokens: 10,
            total_tokens,
            prompt_tokens_details: None,
        });
        resp
    }

    fn multi_tool_call_response(calls: Vec<(&str, &str, &str)>) -> LlmResponse {
        LlmResponse {
            content: None,
            tool_calls: calls
                .into_iter()
                .map(|(id, name, args)| ToolCall {
                    id: id.to_string(),
                    type_: "function".to_string(),
                    function: FunctionCall {
                        name: name.to_string(),
                        arguments: args.to_string(),
                    },
                })
                .collect(),
            usage: None,
            finish_reason: Some("tool_calls".to_string()),
            thinking: None,
        }
    }

    fn echo_registry() -> ToolRegistry {
        let mut reg = ToolRegistry::new();
        reg.register(Arc::new(EchoTool));
        reg
    }

    fn make_agent(
        mock: MockLlm,
        registry: ToolRegistry,
        max_iterations: Option<u32>,
    ) -> (AgentLoop, mpsc::Receiver<AgentEvent>) {
        let (tx, rx) = mpsc::channel(256);
        let mut config = AgentConfig::new(
            crate::llm::LlmClientConfig {
                base_url: "http://unused".into(),
                model: "unused".into(),
                temperature: None,
                max_completion_tokens: None,
                auth_headers: vec![],
                thinking: None,
                disable_cache_control: false,
            },
            std::path::PathBuf::from("/tmp"),
        );
        if let Some(max) = max_iterations {
            config.max_iterations = max;
        }
        // No retries in tests — failures fail immediately
        config.retry_config = RetryConfig {
            max_retries: 0,
            initial_delay: std::time::Duration::from_millis(1),
            multiplier: 1.0,
            max_delay: std::time::Duration::from_millis(10),
        };
        let agent_loop = AgentLoop::with_provider(
            config,
            Box::new(mock),
            registry,
            CancellationToken::new(),
            tx,
            "test-session".into(),
        );
        (agent_loop, rx)
    }

    fn collect_events(rx: &mut mpsc::Receiver<AgentEvent>) -> Vec<AgentEvent> {
        let mut events = Vec::new();
        while let Ok(event) = rx.try_recv() {
            events.push(event);
        }
        events
    }

    // ════════════════════════════════════════════
    // build_args_summary tests
    // ════════════════════════════════════════════

    #[test]
    fn test_build_args_summary_read() {
        let args = json!({"filePath": "/src/main.rs"});
        assert_eq!(build_args_summary("read", &args, std::path::Path::new("/tmp")), "Reading /src/main.rs");
    }

    #[test]
    fn test_build_args_summary_write() {
        let args = json!({"filePath": "/src/lib.rs", "content": "..."});
        assert_eq!(build_args_summary("write", &args, std::path::Path::new("/tmp")), "Writing /src/lib.rs");
    }

    #[test]
    fn test_build_args_summary_bash() {
        let args = json!({"command": "ls -la", "description": "List files"});
        assert_eq!(build_args_summary("bash", &args, std::path::Path::new("/tmp")), "List files");
    }

    #[test]
    fn test_build_args_summary_generic() {
        let args = json!({"key": "value"});
        assert_eq!(
            build_args_summary("unknown_tool", &args, std::path::Path::new("/tmp")),
            r#"{"key":"value"}"#
        );
    }

    #[test]
    fn test_build_args_summary_generic_truncated() {
        let long_val = "x".repeat(200);
        let args = json!({"key": long_val});
        let result = build_args_summary("unknown_tool", &args, std::path::Path::new("/tmp"));
        assert!(result.ends_with("..."));
        // 100 bytes of content + "..."
        assert!(result.len() <= 104);
    }

    // ════════════════════════════════════════════
    // Agent loop — happy path
    // ════════════════════════════════════════════

    #[tokio::test]
    async fn test_text_only_response() {
        let mock = MockLlm::new(vec![Ok(text_response("Hello!"))]);
        let (mut agent, mut rx) = make_agent(mock, ToolRegistry::new(), None);

        let result = agent.run(ChatMessage::user("Hi")).await.unwrap().unwrap_done();
        assert_eq!(result, "Hello!");

        let events = collect_events(&mut rx);
        assert!(events.iter().any(
            |e| matches!(e, AgentEvent::Done { summary: Some(s), .. } if s == "Hello!")
        ));
    }

    #[tokio::test]
    async fn test_tool_call_then_text() {
        let mock = MockLlm::new(vec![
            Ok(tool_call_response(
                "call_1",
                "echo",
                r#"{"message":"pong"}"#,
            )),
            Ok(text_response("Done.")),
        ]);
        let (mut agent, mut rx) = make_agent(mock, echo_registry(), None);

        let result = agent.run(ChatMessage::user("ping")).await.unwrap().unwrap_done();
        assert_eq!(result, "Done.");

        let events = collect_events(&mut rx);
        assert!(events.iter().any(
            |e| matches!(e, AgentEvent::ToolStart { tool_name, args_summary, .. } if tool_name == "echo" && args_summary.contains("pong"))
        ));
        assert!(events
            .iter()
            .any(|e| matches!(e, AgentEvent::ToolEnd { success: true, .. })));
        assert!(events.iter().any(|e| matches!(e, AgentEvent::Done { .. })));
    }

    #[tokio::test]
    async fn test_none_content_with_no_tool_calls_is_error() {
        // Empty response with no tool_calls = the model silently gave up.
        // Previously this returned AgentResult::Done { summary: "" } which
        // showed in the UI as a successful completion — users saw the agent
        // just stop with no explanation. It's now surfaced as an error with
        // retry guidance so the UI can render it visibly.
        let response = LlmResponse {
            content: None,
            tool_calls: vec![],
            usage: None,
            finish_reason: Some("stop".to_string()),
            thinking: None,
        };
        let mock = MockLlm::new(vec![Ok(response)]);
        let (mut agent, mut rx) = make_agent(mock, ToolRegistry::new(), None);

        let err = agent.run(ChatMessage::user("test")).await.unwrap_err();
        assert!(
            matches!(err, AgentError::LlmParseError(_)),
            "expected LlmParseError, got {err:?}"
        );

        // Verify the user-visible AgentEvent::Error was emitted (retrying=false)
        let events = collect_events(&mut rx);
        assert!(
            events.iter().any(|e| matches!(
                e,
                AgentEvent::Error { retrying: false, .. }
            )),
            "expected AgentEvent::Error with retrying=false, got events: {events:?}"
        );
    }

    // ════════════════════════════════════════════
    // Agent loop — tool error paths (ToolStart/ToolEnd pairing)
    // ════════════════════════════════════════════

    #[tokio::test]
    async fn test_unknown_tool_emits_start_and_end() {
        let mock = MockLlm::new(vec![
            Ok(tool_call_response("call_1", "nonexistent", "{}")),
            Ok(text_response("recovered")),
        ]);
        let (mut agent, mut rx) = make_agent(mock, echo_registry(), None);

        let result = agent.run(ChatMessage::user("test")).await.unwrap().unwrap_done();
        assert_eq!(result, "recovered");

        let events = collect_events(&mut rx);
        let starts: Vec<_> = events
            .iter()
            .filter(|e| matches!(e, AgentEvent::ToolStart { .. }))
            .collect();
        let ends: Vec<_> = events
            .iter()
            .filter(|e| matches!(e, AgentEvent::ToolEnd { .. }))
            .collect();
        assert_eq!(starts.len(), 1, "should emit exactly one ToolStart");
        assert_eq!(ends.len(), 1, "should emit exactly one ToolEnd");
        assert!(events.iter().any(
            |e| matches!(e, AgentEvent::ToolEnd { success: false, summary, .. } if summary.contains("Unknown tool"))
        ));
    }

    #[tokio::test]
    async fn test_invalid_json_args_emits_start_and_end() {
        let mock = MockLlm::new(vec![
            Ok(tool_call_response("call_1", "echo", "not valid json")),
            Ok(text_response("OK")),
        ]);
        let (mut agent, mut rx) = make_agent(mock, echo_registry(), None);

        let result = agent.run(ChatMessage::user("test")).await.unwrap().unwrap_done();
        assert_eq!(result, "OK");

        let events = collect_events(&mut rx);
        let starts: Vec<_> = events
            .iter()
            .filter(|e| matches!(e, AgentEvent::ToolStart { .. }))
            .collect();
        let ends: Vec<_> = events
            .iter()
            .filter(|e| matches!(e, AgentEvent::ToolEnd { .. }))
            .collect();
        assert_eq!(starts.len(), 1);
        assert_eq!(ends.len(), 1);
        assert!(events.iter().any(
            |e| matches!(e, AgentEvent::ToolEnd { success: false, summary, .. } if summary.contains("Invalid JSON"))
        ));
    }

    #[tokio::test]
    async fn test_schema_validation_error_emits_start_and_end() {
        // echo requires "message" (string), send a number instead → type mismatch
        let mock = MockLlm::new(vec![
            Ok(tool_call_response("call_1", "echo", r#"{"message": 42}"#)),
            Ok(text_response("OK")),
        ]);
        let (mut agent, mut rx) = make_agent(mock, echo_registry(), None);

        let result = agent.run(ChatMessage::user("test")).await.unwrap().unwrap_done();
        assert_eq!(result, "OK");

        let events = collect_events(&mut rx);
        let starts: Vec<_> = events
            .iter()
            .filter(|e| matches!(e, AgentEvent::ToolStart { .. }))
            .collect();
        let ends: Vec<_> = events
            .iter()
            .filter(|e| matches!(e, AgentEvent::ToolEnd { .. }))
            .collect();
        assert_eq!(starts.len(), 1);
        assert_eq!(ends.len(), 1);
        assert!(events.iter().any(
            |e| matches!(e, AgentEvent::ToolEnd { success: false, summary, .. } if summary.contains("Invalid arguments"))
        ));
    }

    #[tokio::test]
    async fn test_tool_output_summary_truncated_at_200() {
        // Create a tool that returns a very long output
        struct LongOutputTool;

        #[async_trait::async_trait]
        impl Tool for LongOutputTool {
            fn name(&self) -> &str {
                "long"
            }
            fn description(&self) -> &str {
                "Returns long output"
            }
            fn parameters_schema(&self) -> serde_json::Value {
                json!({"type": "object"})
            }
            async fn execute(
                &self,
                _args: serde_json::Value,
                _ctx: &ToolContext,
            ) -> Result<ToolResult, ToolError> {
                Ok(ToolResult::success("x".repeat(500)))
            }
        }

        let mut reg = ToolRegistry::new();
        reg.register(Arc::new(LongOutputTool));
        let mock = MockLlm::new(vec![
            Ok(tool_call_response("call_1", "long", "{}")),
            Ok(text_response("done")),
        ]);
        let (mut agent, mut rx) = make_agent(mock, reg, None);

        agent.run(ChatMessage::user("test")).await.unwrap();

        let events = collect_events(&mut rx);
        let tool_end = events
            .iter()
            .find(|e| matches!(e, AgentEvent::ToolEnd { .. }))
            .unwrap();
        if let AgentEvent::ToolEnd { summary, .. } = tool_end {
            assert!(summary.ends_with("..."));
            assert!(summary.len() <= 204); // 200 + "..."
        }
    }

    // ════════════════════════════════════════════
    // Agent loop — iteration limit and cancellation
    // ════════════════════════════════════════════

    #[tokio::test]
    async fn test_max_iterations() {
        let responses: Vec<_> = (0..5)
            .map(|i| {
                Ok(tool_call_response(
                    &format!("call_{i}"),
                    "echo",
                    r#"{"message":"loop"}"#,
                ))
            })
            .collect();
        let mock = MockLlm::new(responses);
        let (mut agent, _rx) = make_agent(mock, echo_registry(), Some(3));

        let result = agent.run(ChatMessage::user("loop")).await.unwrap();
        match result {
            AgentResult::Done { ref summary } => {
                assert!(summary.contains("maximum of 3 steps"), "Got: {summary}");
            }
            other => panic!("Expected AgentResult::Done, got {:?}", other),
        }
    }

    #[tokio::test]
    async fn test_cancellation_at_loop_start() {
        let mock = MockLlm::new(vec![]);
        let (mut agent, _rx) = make_agent(mock, ToolRegistry::new(), None);
        agent.cancel_token.cancel();

        let result = agent.run(ChatMessage::user("test")).await;
        assert!(matches!(result, Err(AgentError::Cancelled)));
    }

    #[tokio::test]
    async fn test_cancellation_between_tool_calls() {
        // Two tool calls: first cancels the token
        // With parallel execution, both may start simultaneously
        let mut reg = ToolRegistry::new();
        reg.register(Arc::new(CancelTriggerTool));
        reg.register(Arc::new(EchoTool));

        let mock = MockLlm::new(vec![Ok(multi_tool_call_response(vec![
            ("call_1", "cancel_trigger", "{}"),
            ("call_2", "echo", r#"{"message":"may run"}"#),
        ]))]);
        let (mut agent, mut rx) = make_agent(mock, reg, None);

        let result = agent.run(ChatMessage::user("test")).await;
        assert!(matches!(result, Err(AgentError::Cancelled)));

        // With parallel execution, both tools may start.
        // The important thing is that the agent returns Cancelled.
        let events = collect_events(&mut rx);
        let starts: Vec<_> = events
            .iter()
            .filter(|e| matches!(e, AgentEvent::ToolStart { .. }))
            .collect();
        // Allow 1 or 2 ToolStarts (parallel execution may start both)
        assert!(
            starts.len() >= 1 && starts.len() <= 2,
            "Expected 1-2 ToolStart events, got {}",
            starts.len()
        );
    }

    // ════════════════════════════════════════════
    // Retry logic
    // ════════════════════════════════════════════

    #[tokio::test]
    async fn test_retry_on_500_then_success() {
        tokio::time::pause();
        let mock = MockLlm::new(vec![
            Err(AgentError::LlmApiError {
                status: 500,
                body: "Internal Server Error".into(),
            }),
            Ok(text_response("recovered")),
        ]);
        let (mut agent, mut rx) = make_agent(mock, ToolRegistry::new(), None);
        agent.config.retry_config.max_retries = 3;

        let result = agent.run(ChatMessage::user("test")).await.unwrap().unwrap_done();
        assert_eq!(result, "recovered");

        let events = collect_events(&mut rx);
        // Should have emitted an Error event with retrying=true
        assert!(events.iter().any(
            |e| matches!(e, AgentEvent::Error { retrying: true, .. })
        ));
    }

    #[tokio::test]
    async fn test_retry_on_429_then_success() {
        tokio::time::pause();
        let mock = MockLlm::new(vec![
            Err(AgentError::LlmApiError {
                status: 429,
                body: "Rate limited".into(),
            }),
            Ok(text_response("ok")),
        ]);
        let (mut agent, _rx) = make_agent(mock, ToolRegistry::new(), None);
        agent.config.retry_config.max_retries = 3;

        let result = agent.run(ChatMessage::user("test")).await.unwrap().unwrap_done();
        assert_eq!(result, "ok");
    }

    #[tokio::test]
    async fn test_no_retry_on_401() {
        let mock = MockLlm::new(vec![Err(AgentError::LlmApiError {
            status: 401,
            body: "Unauthorized".into(),
        })]);
        let (mut agent, _rx) = make_agent(mock, ToolRegistry::new(), None);

        let result = agent.run(ChatMessage::user("test")).await;
        match result {
            Err(AgentError::LlmApiError { status: 401, .. }) => {} // expected
            other => panic!("Expected 401 error, got: {other:?}"),
        }
    }

    #[tokio::test]
    async fn test_no_retry_on_400() {
        let mock = MockLlm::new(vec![Err(AgentError::LlmApiError {
            status: 400,
            body: "Bad Request".into(),
        })]);
        let (mut agent, mut rx) = make_agent(mock, ToolRegistry::new(), None);

        let result = agent.run(ChatMessage::user("test")).await;
        assert!(matches!(
            result,
            Err(AgentError::LlmApiError { status: 400, .. })
        ));

        // Permanent LLM errors must emit an AgentEvent::Error (retrying=false)
        // so the UI sees the failure without depending on the session-level
        // watcher's Error+Done emission.
        let events = collect_events(&mut rx);
        assert!(
            events.iter().any(|e| matches!(
                e,
                AgentEvent::Error { retrying: false, .. }
            )),
            "expected AgentEvent::Error with retrying=false on 400, got: {events:?}"
        );
    }

    #[tokio::test]
    async fn test_retry_exhausted() {
        tokio::time::pause();
        // 4 responses for max_retries=3 (attempts 0, 1, 2, 3)
        let mock = MockLlm::new(vec![
            Err(AgentError::LlmApiError {
                status: 500,
                body: "fail".into(),
            }),
            Err(AgentError::LlmApiError {
                status: 500,
                body: "fail".into(),
            }),
            Err(AgentError::LlmApiError {
                status: 500,
                body: "fail".into(),
            }),
            Err(AgentError::LlmApiError {
                status: 500,
                body: "fail".into(),
            }),
        ]);
        let (mut agent, mut rx) = make_agent(mock, ToolRegistry::new(), None);
        agent.config.retry_config.max_retries = 3;

        let result = agent.run(ChatMessage::user("test")).await;
        assert!(matches!(
            result,
            Err(AgentError::LlmApiError { status: 500, .. })
        ));

        let events = collect_events(&mut rx);
        // Should have retry events (retrying=true) and a final error (retrying=false)
        let retry_events: Vec<_> = events
            .iter()
            .filter(|e| matches!(e, AgentEvent::Error { retrying: true, .. }))
            .collect();
        let final_error = events
            .iter()
            .find(|e| matches!(e, AgentEvent::Error { retrying: false, .. }));
        assert_eq!(retry_events.len(), 3, "should have 3 retry events");
        assert!(final_error.is_some(), "should have a final error event");
        if let Some(AgentEvent::Error { message, .. }) = final_error {
            assert!(message.contains("after 3 retries"));
        }
    }

    #[tokio::test]
    async fn test_cancellation_not_retried() {
        let mock = MockLlm::new(vec![Err(AgentError::Cancelled)]);
        let (mut agent, _rx) = make_agent(mock, ToolRegistry::new(), None);

        let result = agent.run(ChatMessage::user("test")).await;
        assert!(matches!(result, Err(AgentError::Cancelled)));
    }

    // ════════════════════════════════════════════
    // build_args_summary — new tool tests
    // ════════════════════════════════════════════

    #[test]
    fn test_build_args_summary_edit() {
        let args = json!({"filePath": "/src/main.rs", "oldString": "x", "newString": "y"});
        assert_eq!(build_args_summary("edit", &args, std::path::Path::new("/tmp")), "Editing /src/main.rs");
    }

    #[test]
    fn test_build_args_summary_glob() {
        let args = json!({"pattern": "**/*.rs"});
        assert_eq!(build_args_summary("glob", &args, std::path::Path::new("/tmp")), "Searching for **/*.rs");
    }

    #[test]
    fn test_build_args_summary_grep() {
        let args = json!({"pattern": "fn main"});
        assert_eq!(build_args_summary("grep", &args, std::path::Path::new("/tmp")), "Searching for /fn main/");
    }

    #[test]
    fn test_build_args_summary_git() {
        let args = json!({"command": "status", "description": "Check repo status"});
        assert_eq!(build_args_summary("git", &args, std::path::Path::new("/tmp")), "Check repo status");
    }

    #[test]
    fn test_build_args_summary_create_pr() {
        let args = json!({"title": "Add feature X", "body": "...", "branch": "feat"});
        assert_eq!(
            build_args_summary("create_pr", &args, std::path::Path::new("/tmp")),
            "Creating PR: Add feature X"
        );
    }

    // ════════════════════════════════════════════
    // Global output truncation
    // ════════════════════════════════════════════

    #[test]
    fn test_truncate_tool_output_short_passes_through() {
        let input = "short output";
        assert_eq!(truncate_output(input, MAX_TOOL_OUTPUT_LINES, MAX_TOOL_OUTPUT_BYTES), input);
    }

    #[test]
    fn test_truncate_tool_output_exceeds_line_limit() {
        let input: String = (0..2500).map(|i| format!("line {i}")).collect::<Vec<_>>().join("\n");
        let result = truncate_output(&input, MAX_TOOL_OUTPUT_LINES, MAX_TOOL_OUTPUT_BYTES);
        assert!(result.contains("truncated"));
        assert!(result.contains("2000 of 2500 lines"));
    }

    #[test]
    fn test_truncate_tool_output_exceeds_byte_limit() {
        let input: String = (0..100).map(|i| format!("line {:04} {}", i, "x".repeat(600))).collect::<Vec<_>>().join("\n");
        assert!(input.len() > MAX_TOOL_OUTPUT_BYTES);
        let result = truncate_output(&input, MAX_TOOL_OUTPUT_LINES, MAX_TOOL_OUTPUT_BYTES);
        assert!(result.contains("truncated"));
    }

    // ════════════════════════════════════════════
    // Parallel execution
    // ════════════════════════════════════════════

    #[tokio::test]
    async fn test_parallel_tool_calls_return_correct_order() {
        // Two echo calls should return results in the correct order
        let mock = MockLlm::new(vec![
            Ok(multi_tool_call_response(vec![
                ("call_1", "echo", r#"{"message":"first"}"#),
                ("call_2", "echo", r#"{"message":"second"}"#),
            ])),
            Ok(text_response("Done.")),
        ]);
        let (mut agent, mut rx) = make_agent(mock, echo_registry(), None);

        let result = agent.run(ChatMessage::user("test")).await.unwrap().unwrap_done();
        assert_eq!(result, "Done.");

        let events = collect_events(&mut rx);
        let tool_ends: Vec<_> = events
            .iter()
            .filter_map(|e| {
                if let AgentEvent::ToolEnd { tool_call_id, summary, .. } = e {
                    Some((tool_call_id.clone(), summary.clone()))
                } else {
                    None
                }
            })
            .collect();

        assert_eq!(tool_ends.len(), 2);
        // Both tool calls should have completed
        let ids: Vec<_> = tool_ends.iter().map(|(id, _)| id.as_str()).collect();
        assert!(ids.contains(&"call_1"));
        assert!(ids.contains(&"call_2"));
    }

    #[tokio::test]
    async fn test_one_tool_failure_doesnt_affect_others() {
        // One tool call with unknown tool, one valid
        let mock = MockLlm::new(vec![
            Ok(multi_tool_call_response(vec![
                ("call_1", "nonexistent", "{}"),
                ("call_2", "echo", r#"{"message":"works"}"#),
            ])),
            Ok(text_response("Done.")),
        ]);
        let (mut agent, mut rx) = make_agent(mock, echo_registry(), None);

        let result = agent.run(ChatMessage::user("test")).await.unwrap().unwrap_done();
        assert_eq!(result, "Done.");

        let events = collect_events(&mut rx);
        let tool_ends: Vec<_> = events
            .iter()
            .filter_map(|e| {
                if let AgentEvent::ToolEnd { tool_call_id, success, .. } = e {
                    Some((tool_call_id.clone(), *success))
                } else {
                    None
                }
            })
            .collect();

        assert_eq!(tool_ends.len(), 2);
        // One failed, one succeeded
        let failed = tool_ends.iter().filter(|(_, s)| !s).count();
        let succeeded = tool_ends.iter().filter(|(_, s)| *s).count();
        assert_eq!(failed, 1);
        assert_eq!(succeeded, 1);
    }

    // ── PanickingTool (panics when executed — tests JoinSet recovery) ──

    struct PanickingTool;

    #[async_trait::async_trait]
    impl Tool for PanickingTool {
        fn name(&self) -> &str {
            "panicker"
        }
        fn description(&self) -> &str {
            "Panics on execute"
        }
        fn parameters_schema(&self) -> serde_json::Value {
            json!({"type": "object"})
        }
        async fn execute(
            &self,
            _args: serde_json::Value,
            _ctx: &ToolContext,
        ) -> Result<ToolResult, ToolError> {
            panic!("intentional panic for testing");
        }
    }

    #[tokio::test]
    async fn test_panic_recovery_injects_synthetic_error() {
        // A tool that panics should produce a synthetic error, not crash the agent
        let mut reg = ToolRegistry::new();
        reg.register(Arc::new(PanickingTool));

        let mock = MockLlm::new(vec![
            Ok(tool_call_response("call_1", "panicker", "{}")),
            Ok(text_response("recovered after panic")),
        ]);
        let (mut agent, mut rx) = make_agent(mock, reg, None);

        let result = agent.run(ChatMessage::user("test")).await.unwrap().unwrap_done();
        assert_eq!(result, "recovered after panic");

        let events = collect_events(&mut rx);
        // The agent should have continued after the panicked tool
        assert!(events.iter().any(|e| matches!(e, AgentEvent::Done { .. })));
    }

    #[tokio::test]
    async fn test_panic_alongside_healthy_tool() {
        // One tool panics, another succeeds — both should produce tool results
        let mut reg = ToolRegistry::new();
        reg.register(Arc::new(PanickingTool));
        reg.register(Arc::new(EchoTool));

        let mock = MockLlm::new(vec![
            Ok(multi_tool_call_response(vec![
                ("call_1", "panicker", "{}"),
                ("call_2", "echo", r#"{"message":"ok"}"#),
            ])),
            Ok(text_response("Done.")),
        ]);
        let (mut agent, mut rx) = make_agent(mock, reg, None);

        let result = agent.run(ChatMessage::user("test")).await.unwrap().unwrap_done();
        assert_eq!(result, "Done.");

        let events = collect_events(&mut rx);
        // The echo tool should have succeeded
        assert!(events.iter().any(
            |e| matches!(e, AgentEvent::ToolEnd { tool_call_id, success: true, .. } if tool_call_id == "call_2")
        ));
        // The agent should have completed
        assert!(events.iter().any(|e| matches!(e, AgentEvent::Done { .. })));
    }

    #[tokio::test]
    async fn test_parallel_cancellation() {
        // Pre-cancel before tool execution
        let mock = MockLlm::new(vec![Ok(multi_tool_call_response(vec![
            ("call_1", "echo", r#"{"message":"test"}"#),
        ]))]);
        let (mut agent, _rx) = make_agent(mock, echo_registry(), None);
        // Cancel after LLM returns but before tool exec (which the loop checks)
        // Actually we pre-cancel to ensure the batch-level check catches it
        agent.cancel_token.cancel();

        let result = agent.run(ChatMessage::user("test")).await;
        assert!(matches!(result, Err(AgentError::Cancelled)));
    }

    // ════════════════════════════════════════════
    // Persister integration
    // ════════════════════════════════════════════

    #[tokio::test]
    async fn test_persister_receives_all_messages() {
        use crate::persistence::MockPersister;

        let persister = Arc::new(MockPersister::new());

        let mock = MockLlm::new(vec![
            Ok(tool_call_response("call_1", "echo", r#"{"message":"pong"}"#)),
            Ok(text_response("Done.")),
        ]);
        let (tx, rx) = mpsc::channel(256);
        let mut config = AgentConfig::new(
            crate::llm::LlmClientConfig {
                base_url: "http://unused".into(),
                model: "unused".into(),
                temperature: None,
                max_completion_tokens: None,
                auth_headers: vec![],
                thinking: None,
                disable_cache_control: false,
            },
            std::path::PathBuf::from("/tmp"),
        );
        config.retry_config = RetryConfig {
            max_retries: 0,
            initial_delay: std::time::Duration::from_millis(1),
            multiplier: 1.0,
            max_delay: std::time::Duration::from_millis(10),
        };

        let mut agent = AgentLoop::with_provider(
            config,
            Box::new(mock),
            echo_registry(),
            CancellationToken::new(),
            tx,
            "test-persist".into(),
        )
        .with_persister(Arc::clone(&persister) as Arc<dyn crate::persistence::MessagePersister>, "session-1".into());

        let _result = agent.run(ChatMessage::user("ping")).await.unwrap();
        drop(rx);

        // Give fire-and-forget tasks a moment to complete
        tokio::time::sleep(std::time::Duration::from_millis(50)).await;

        let msgs = persister.messages();
        // Should have: user message, assistant (tool_call), tool result, assistant (done)
        assert!(msgs.len() >= 3, "Expected at least 3 persisted messages, got {}", msgs.len());

        // All should be in session-1
        for (sid, _) in &msgs {
            assert_eq!(sid, "session-1");
        }
    }

    #[tokio::test]
    async fn test_persister_failure_does_not_stop_loop() {
        use crate::persistence::{AgentMessage, MessagePersister, PersistError, PersistResult};

        struct FailingPersister;

        #[async_trait::async_trait]
        impl MessagePersister for FailingPersister {
            async fn persist_message(&self, _msg: &AgentMessage, _session_id: &str) -> Result<PersistResult, PersistError> {
                Err(PersistError::Storage("simulated failure".into()))
            }
            async fn load_context(&self, _session_id: &str) -> Result<Vec<AgentMessage>, PersistError> {
                Ok(vec![])
            }
        }

        let mock = MockLlm::new(vec![Ok(text_response("Hello!"))]);
        let (tx, rx) = mpsc::channel(256);
        let config = AgentConfig::new(
            crate::llm::LlmClientConfig {
                base_url: "http://unused".into(),
                model: "unused".into(),
                temperature: None,
                max_completion_tokens: None,
                auth_headers: vec![],
                thinking: None,
                disable_cache_control: false,
            },
            std::path::PathBuf::from("/tmp"),
        );

        let mut agent = AgentLoop::with_provider(
            config,
            Box::new(mock),
            ToolRegistry::new(),
            CancellationToken::new(),
            tx,
            "test-fail".into(),
        )
        .with_persister(Arc::new(FailingPersister), "test-fail".into());

        // Should complete successfully despite persistence failures
        let result = agent.run(ChatMessage::user("Hi")).await.unwrap().unwrap_done();
        assert_eq!(result, "Hello!");
        drop(rx);
    }

    #[tokio::test]
    async fn test_no_persister_works() {
        let mock = MockLlm::new(vec![Ok(text_response("Works!"))]);
        let (mut agent, _rx) = make_agent(mock, ToolRegistry::new(), None);

        let result = agent.run(ChatMessage::user("test")).await.unwrap().unwrap_done();
        assert_eq!(result, "Works!");
    }

    // ════════════════════════════════════════════
    // Compaction in loop
    // ════════════════════════════════════════════

    #[tokio::test]
    async fn test_compaction_triggers_in_loop() {
        use crate::agent::config::CompactionConfig;
        use crate::persistence::MockPersister;

        let persister = Arc::new(MockPersister::new());

        // 1 tool round with seeded initial context to provide compactable messages.
        // After tool execution: [sys, ctx_user, ctx_asst, user, a+tc, tool_result] = 6 msgs
        // keep_recent=2 → raw_end=4, snap: messages[3]=user → safe. end=4.
        // compact [1,4) = [ctx_user, ctx_asst, user] → 3 msgs, enough!
        let mock = MockLlm::new(vec![
            // Tool call, 45 tokens (above 40 threshold) → triggers compaction
            Ok(tool_call_response_with_usage("c1", "echo", r#"{"message":"a"}"#, 45)),
            // Compaction summary (consumed by compaction LLM call)
            Ok(text_response("Compacted: prior context")),
            // Final response after compaction
            Ok(text_response("Done with compaction.")),
        ]);

        let (tx, rx) = mpsc::channel(256);
        let mut config = AgentConfig::new(
            crate::llm::LlmClientConfig {
                base_url: "http://unused".into(),
                model: "unused".into(),
                temperature: None,
                max_completion_tokens: None,
                auth_headers: vec![],
                thinking: None,
                disable_cache_control: false,
            },
            std::path::PathBuf::from("/tmp"),
        );
        config.system_prompt = Some(vec![crate::agent::prompt::SystemBlock {
            text: "You are helpful.".into(),
            cache_control: None,
        }]);
        config.retry_config = RetryConfig {
            max_retries: 0,
            initial_delay: std::time::Duration::from_millis(1),
            multiplier: 1.0,
            max_delay: std::time::Duration::from_millis(10),
        };
        // keep_recent=2 means we keep the last 2 messages (a+tc, tool_result)
        // With seeded context: [sys, ctx_user, ctx_asst, user, a+tc, tool_result] = 6 msgs
        // raw_end = 6-2 = 4, snap: messages[3]=user → safe. end=4.
        // compact [1,4) = [ctx_user, ctx_asst, user] → 3 msgs, enough!
        config.compaction_config = CompactionConfig {
            context_limit: 50,
            threshold_pct: 0.80,
            keep_recent_messages: 2,
            max_messages: 10_000,
        };

        // Seed initial context so there are compactable messages before the tool pair
        let initial_context = vec![
            ChatMessage::user("What does main.rs do?"),
            ChatMessage::assistant(Some("It starts the server.".into()), None, None),
        ];

        let mut agent = AgentLoop::with_provider(
            config,
            Box::new(mock),
            echo_registry(),
            CancellationToken::new(),
            tx,
            "test-compact".into(),
        )
        .with_initial_context(initial_context)
        .with_persister(Arc::clone(&persister) as Arc<dyn crate::persistence::MessagePersister>, "thread-compact".into());

        let result = agent.run(ChatMessage::user("test compaction")).await.unwrap().unwrap_done();
        assert_eq!(result, "Done with compaction.");
        drop(rx);

        // Give fire-and-forget tasks a moment to complete
        tokio::time::sleep(std::time::Duration::from_millis(50)).await;

        // Check that a compaction record was persisted
        let msgs = persister.messages();
        let compaction_msgs: Vec<_> = msgs
            .iter()
            .filter(|(_, m)| m.message_type == crate::persistence::MessageType::Compaction)
            .collect();
        assert!(
            !compaction_msgs.is_empty(),
            "Expected at least one compaction record, got none. Total persisted: {}",
            msgs.len()
        );
    }

    // ════════════════════════════════════════════
    // Compaction: verify message structure after replacement
    // ════════════════════════════════════════════

    #[tokio::test]
    async fn test_compaction_replaces_messages_correctly() {
        use crate::agent::config::CompactionConfig;

        // 1 tool round with seeded context. Token usage=45 triggers compaction.
        let mock = CapturingLlm {
            responses: Mutex::new(VecDeque::from(vec![
                Ok(tool_call_response_with_usage("c1", "echo", r#"{"message":"a"}"#, 45)),
                // Compaction summary
                Ok(text_response("Summary: prior context")),
                // Final response after compaction
                Ok(text_response("All done.")),
            ])),
            captured: Mutex::new(Vec::new()),
        };

        let (tx, rx) = mpsc::channel(256);
        let mut config = AgentConfig::new(
            crate::llm::LlmClientConfig {
                base_url: "http://unused".into(),
                model: "unused".into(),
                temperature: None,
                max_completion_tokens: None,
                auth_headers: vec![],
                thinking: None,
                disable_cache_control: false,
            },
            std::path::PathBuf::from("/tmp"),
        );
        config.system_prompt = Some(vec![crate::agent::prompt::SystemBlock {
            text: "You are helpful.".into(),
            cache_control: None,
        }]);
        config.retry_config = RetryConfig {
            max_retries: 0,
            initial_delay: std::time::Duration::from_millis(1),
            multiplier: 1.0,
            max_delay: std::time::Duration::from_millis(10),
        };
        // context_limit=50, threshold=80% → compacts at 40 tokens
        config.compaction_config = CompactionConfig {
            context_limit: 50,
            threshold_pct: 0.80,
            keep_recent_messages: 2,
            max_messages: 10_000,
        };

        let mock = Arc::new(mock);
        let mock_clone: Box<dyn LlmProvider> = Box::new(ArcLlm(Arc::clone(&mock) as Arc<dyn LlmProvider>));

        // Seed initial context so there are compactable non-tool messages
        let initial_context = vec![
            ChatMessage::user("What does main.rs do?"),
            ChatMessage::assistant(Some("It starts the server.".into()), None, None),
        ];

        let mut agent = AgentLoop::with_provider(
            config,
            mock_clone,
            echo_registry(),
            CancellationToken::new(),
            tx,
            "test-compact-struct".into(),
        )
        .with_initial_context(initial_context);

        let result = agent.run(ChatMessage::user("test compaction")).await.unwrap().unwrap_done();
        assert_eq!(result, "All done.");
        drop(rx);

        // Check the messages sent to the LLM on the THIRD call (after compaction)
        let captured = mock.captured.lock().unwrap();
        // Call 0: initial (system + context + user), Call 1: compaction call, Call 2: after compaction
        assert!(captured.len() >= 3, "Expected 3+ LLM calls, got {}", captured.len());

        let post_compaction_msgs = &captured[2];
        // Should have: [system prompt, user(summary), tool_result(kept), ...]
        // System prompt must be first
        assert_eq!(post_compaction_msgs[0].role, "system");
        assert!(post_compaction_msgs[0].content.as_ref().unwrap().text().contains("You are helpful"));

        // No second system message should exist
        let system_count = post_compaction_msgs.iter().filter(|m| m.role == "system").count();
        assert_eq!(system_count, 1, "Should have exactly 1 system message after compaction, got {system_count}");

        // The summary should be a user-role message
        assert_eq!(post_compaction_msgs[1].role, "user");
        assert!(
            post_compaction_msgs[1].content.as_ref().unwrap().text().contains("[Context summary from earlier"),
            "Summary message should contain context summary prefix"
        );
    }

    // ════════════════════════════════════════════
    // with_initial_context: messages appear in LLM call and are persisted
    // ════════════════════════════════════════════

    #[tokio::test]
    async fn test_initial_context_appears_in_llm_call() {
        use crate::persistence::MockPersister;

        let mock = Arc::new(CapturingLlm {
            responses: Mutex::new(VecDeque::from(vec![
                Ok(text_response("I see the context!")),
            ])),
            captured: Mutex::new(Vec::new()),
        });

        let persister = Arc::new(MockPersister::new());
        let (tx, rx) = mpsc::channel(256);
        let config = AgentConfig::new(
            crate::llm::LlmClientConfig {
                base_url: "http://unused".into(),
                model: "unused".into(),
                temperature: None,
                max_completion_tokens: None,
                auth_headers: vec![],
                thinking: None,
                disable_cache_control: false,
            },
            std::path::PathBuf::from("/tmp"),
        );

        let mock_box: Box<dyn LlmProvider> = Box::new(ArcLlm(Arc::clone(&mock) as Arc<dyn LlmProvider>));
        let mut agent = AgentLoop::with_provider(
            config,
            mock_box,
            ToolRegistry::new(),
            CancellationToken::new(),
            tx,
            "test-ctx".into(),
        )
        .with_persister(Arc::clone(&persister) as Arc<dyn MessagePersister>, "thread-ctx".into())
        .with_initial_context(vec![
            ChatMessage::user("What does main.rs do?"),
            ChatMessage::assistant(Some("It starts the server.".into()), None, None),
        ]);

        let result = agent.run(ChatMessage::user("Now fix the bug")).await.unwrap().unwrap_done();
        assert_eq!(result, "I see the context!");
        drop(rx);

        // Verify the LLM received the initial context messages
        let captured = mock.captured.lock().unwrap();
        assert_eq!(captured.len(), 1);
        let msgs = &captured[0];
        // Should be: [user("What does main.rs do?"), assistant("It starts the server."), user("Now fix the bug")]
        assert!(msgs.len() >= 3, "Expected at least 3 messages, got {}", msgs.len());
        assert_eq!(msgs[0].role, "user");
        assert_eq!(msgs[0].content.as_ref().unwrap().text(), "What does main.rs do?");
        assert_eq!(msgs[1].role, "assistant");
        assert_eq!(msgs[1].content.as_ref().unwrap().text(), "It starts the server.");
        assert_eq!(msgs[2].role, "user");
        assert_eq!(msgs[2].content.as_ref().unwrap().text(), "Now fix the bug");

        // Verify initial context was persisted
        tokio::time::sleep(std::time::Duration::from_millis(50)).await;
        let persisted = persister.messages();
        // Should have: context msg 1, context msg 2, user msg, assistant response = at least 4
        assert!(
            persisted.len() >= 4,
            "Expected at least 4 persisted messages (2 context + user + assistant), got {}",
            persisted.len()
        );
        // First two should be the context messages
        assert_eq!(persisted[0].1.content, "What does main.rs do?");
        assert_eq!(persisted[1].1.content, "It starts the server.");
    }

    // ════════════════════════════════════════════
    // Persistence ordering: messages arrive in submission order
    // ════════════════════════════════════════════

    #[tokio::test]
    async fn test_persistence_ordering() {
        use crate::persistence::MockPersister;

        let persister = Arc::new(MockPersister::new());

        // LLM returns 3 parallel tool calls, then a text response
        let mock = MockLlm::new(vec![
            Ok(multi_tool_call_response(vec![
                ("call_a", "echo", r#"{"message":"alpha"}"#),
                ("call_b", "echo", r#"{"message":"beta"}"#),
                ("call_c", "echo", r#"{"message":"gamma"}"#),
            ])),
            Ok(text_response("Done.")),
        ]);

        let (tx, rx) = mpsc::channel(256);
        let mut config = AgentConfig::new(
            crate::llm::LlmClientConfig {
                base_url: "http://unused".into(),
                model: "unused".into(),
                temperature: None,
                max_completion_tokens: None,
                auth_headers: vec![],
                thinking: None,
                disable_cache_control: false,
            },
            std::path::PathBuf::from("/tmp"),
        );
        config.retry_config = RetryConfig {
            max_retries: 0,
            initial_delay: std::time::Duration::from_millis(1),
            multiplier: 1.0,
            max_delay: std::time::Duration::from_millis(10),
        };

        let mut agent = AgentLoop::with_provider(
            config,
            Box::new(mock),
            echo_registry(),
            CancellationToken::new(),
            tx,
            "test-order".into(),
        )
        .with_persister(Arc::clone(&persister) as Arc<dyn MessagePersister>, "thread-order".into());

        let result = agent.run(ChatMessage::user("test ordering")).await.unwrap().unwrap_done();
        assert_eq!(result, "Done.");
        drop(rx);

        // Let the persistence worker drain
        tokio::time::sleep(std::time::Duration::from_millis(50)).await;

        let persisted = persister.messages();
        // Expected order: user, assistant(tool_calls), tool_result(alpha), tool_result(beta), tool_result(gamma), assistant(Done)
        let contents: Vec<&str> = persisted.iter().map(|(_, m)| m.content.as_str()).collect();
        assert!(persisted.len() >= 6, "Expected at least 6 persisted messages, got {}: {:?}", persisted.len(), contents);

        // Tool results should be in submission order (alpha, beta, gamma)
        let tool_results: Vec<&str> = persisted.iter()
            .filter(|(_, m)| m.message_type == crate::persistence::MessageType::ToolResult)
            .map(|(_, m)| m.content.as_str())
            .collect();
        assert_eq!(tool_results, vec!["alpha", "beta", "gamma"], "Tool results should be in submission order");
    }

    // ════════════════════════════════════════════
    // Yield ordering: tool results are persisted before the yield return
    // ════════════════════════════════════════════

    #[tokio::test]
    async fn test_yield_persists_tool_results_before_returning() {
        use crate::persistence::MockPersister;
        use crate::tool::save_plan::SavePlanTool;

        let persister = Arc::new(MockPersister::new());

        let work_dir = tempfile::tempdir().unwrap();

        // LLM calls save_plan, which yields PlanReady.
        let args = r#"{"plan":"Goal: fix bug","filename":"plan.md"}"#;
        let mock = MockLlm::new(vec![
            Ok(tool_call_response("call_sp", "save_plan", args)),
        ]);

        let mut registry = ToolRegistry::new();
        registry.register(Arc::new(SavePlanTool));

        let (tx, rx) = mpsc::channel(256);
        let config = AgentConfig::new(
            crate::llm::LlmClientConfig {
                base_url: "http://unused".into(),
                model: "unused".into(),
                temperature: None,
                max_completion_tokens: None,
                auth_headers: vec![],
                thinking: None,
                disable_cache_control: false,
            },
            work_dir.path().to_path_buf(),
        );

        let mut agent = AgentLoop::with_provider(
            config,
            Box::new(mock),
            registry,
            CancellationToken::new(),
            tx,
            "test-yield".into(),
        )
        .with_persister(Arc::clone(&persister) as Arc<dyn MessagePersister>, "session-yield".into());

        let result = agent.run(ChatMessage::user("plan the fix")).await.unwrap();
        drop(rx);

        // Should be PlanReady
        match &result {
            AgentResult::PlanReady { plan, .. } => {
                assert!(plan.contains("fix bug"), "Got: {plan}");
            }
            other => panic!("Expected PlanReady, got {:?}", other),
        }

        // Let persistence drain
        tokio::time::sleep(std::time::Duration::from_millis(50)).await;

        let persisted = persister.messages();
        // Should have: user msg, assistant(tool_call), tool_result = at least 3
        assert!(
            persisted.len() >= 3,
            "Expected at least 3 persisted messages (user + assistant + tool_result), got {}",
            persisted.len()
        );

        // The tool result should be persisted (this was the original bug — it was skipped before)
        let has_tool_result = persisted.iter().any(|(_, m)| m.message_type == crate::persistence::MessageType::ToolResult);
        assert!(has_tool_result, "Tool result for the yielding tool should be persisted");
    }

    // ════════════════════════════════════════════
    // Approval handler tests
    // ════════════════════════════════════════════

    #[tokio::test]
    async fn test_execute_tool_with_approval_denied() {
        use crate::approval::test_util::AutoDenyHandler;

        let mock = MockLlm::new(vec![
            Ok(tool_call_response("call_1", "echo", r#"{"message":"hello"}"#)),
            Ok(text_response("Done.")),
        ]);
        let (mut agent, mut rx) = make_agent(mock, echo_registry(), None);
        agent = agent.with_approval_handler(Arc::new(AutoDenyHandler {
            reason: "not allowed".into(),
        }));

        let result = agent.run(ChatMessage::user("test")).await.unwrap().unwrap_done();
        assert_eq!(result, "Done.");

        let events = collect_events(&mut rx);

        // ToolEnd should show permission denied
        let tool_end = events.iter().find_map(|e| {
            if let AgentEvent::ToolEnd { success, summary, .. } = e {
                Some((success, summary))
            } else {
                None
            }
        }).expect("Should have ToolEnd event");
        assert!(!tool_end.0, "Tool should not have succeeded");
        assert!(tool_end.1.contains("Permission denied"), "Summary should mention permission denied, got: {}", tool_end.1);
    }

    #[tokio::test]
    async fn test_execute_tool_with_approval_approved() {
        use crate::approval::test_util::AutoApproveHandler;

        let mock = MockLlm::new(vec![
            Ok(tool_call_response("call_1", "echo", r#"{"message":"pong"}"#)),
            Ok(text_response("Done.")),
        ]);
        let (mut agent, mut rx) = make_agent(mock, echo_registry(), None);
        agent = agent.with_approval_handler(Arc::new(AutoApproveHandler));

        let result = agent.run(ChatMessage::user("test")).await.unwrap().unwrap_done();
        assert_eq!(result, "Done.");

        let events = collect_events(&mut rx);
        // Tool should have executed successfully
        let tool_end = events.iter().find_map(|e| {
            if let AgentEvent::ToolEnd { success, summary, .. } = e {
                Some((*success, summary.clone()))
            } else {
                None
            }
        }).expect("Should have ToolEnd event");
        assert!(tool_end.0, "Tool should have succeeded");
        assert!(tool_end.1.contains("pong"), "Summary should contain echo output, got: {}", tool_end.1);
    }

    #[tokio::test]
    async fn test_execute_tool_without_handler_auto_approves() {
        // No approval handler — current behavior preserved
        let mock = MockLlm::new(vec![
            Ok(tool_call_response("call_1", "echo", r#"{"message":"pong"}"#)),
            Ok(text_response("Done.")),
        ]);
        let (mut agent, mut rx) = make_agent(mock, echo_registry(), None);
        // Explicitly NOT setting an approval handler

        let result = agent.run(ChatMessage::user("test")).await.unwrap().unwrap_done();
        assert_eq!(result, "Done.");

        let events = collect_events(&mut rx);
        let tool_end = events.iter().find_map(|e| {
            if let AgentEvent::ToolEnd { success, .. } = e { Some(*success) } else { None }
        }).expect("Should have ToolEnd event");
        assert!(tool_end, "Tool should auto-approve when no handler is set");
    }

    #[tokio::test]
    async fn test_approval_check_after_schema_validation() {
        use crate::approval::test_util::RecordingApprovalHandler;

        // Send invalid args (missing required "message" field)
        let mock = MockLlm::new(vec![
            Ok(tool_call_response("call_1", "echo", r#"{}"#)),
            Ok(text_response("Done.")),
        ]);
        let handler = Arc::new(RecordingApprovalHandler::new());
        let (mut agent, _rx) = make_agent(mock, echo_registry(), None);
        agent = agent.with_approval_handler(handler.clone());

        let _result = agent.run(ChatMessage::user("test")).await.unwrap();

        // Handler should NOT have been called — schema validation fails first
        let calls = handler.calls.lock().unwrap();
        assert!(calls.is_empty(), "Approval should not be requested for invalid args, got: {:?}", *calls);
    }

    #[tokio::test]
    async fn test_parallel_tools_approval_independent() {
        use crate::approval::test_util::RecordingApprovalHandler;

        let mock = MockLlm::new(vec![
            Ok(multi_tool_call_response(vec![
                ("call_1", "echo", r#"{"message":"first"}"#),
                ("call_2", "echo", r#"{"message":"second"}"#),
            ])),
            Ok(text_response("Done.")),
        ]);
        let handler = Arc::new(RecordingApprovalHandler::new());
        handler.queue_decision(crate::approval::ApprovalDecision::Approved);
        handler.queue_decision(crate::approval::ApprovalDecision::Denied {
            reason: Some("blocked".into()),
        });

        let (mut agent, mut rx) = make_agent(mock, echo_registry(), None);
        agent = agent.with_approval_handler(handler.clone());

        let _result = agent.run(ChatMessage::user("test")).await.unwrap();

        let events = collect_events(&mut rx);
        let tool_ends: Vec<(String, bool)> = events
            .iter()
            .filter_map(|e| {
                if let AgentEvent::ToolEnd { tool_call_id, success, .. } = e {
                    Some((tool_call_id.clone(), *success))
                } else {
                    None
                }
            })
            .collect();

        assert_eq!(tool_ends.len(), 2, "Should have 2 ToolEnd events");

        // Both calls were made to the handler
        let calls = handler.calls.lock().unwrap();
        assert_eq!(calls.len(), 2, "Handler should have been called for both tools");

        // One succeeded, one failed
        let successes: Vec<bool> = tool_ends.iter().map(|(_, s)| *s).collect();
        assert!(successes.contains(&true), "One tool should have succeeded");
        assert!(successes.contains(&false), "One tool should have been denied");
    }

    // ════════════════════════════════════════════
    // TurnCompleted event
    // ════════════════════════════════════════════

    #[tokio::test]
    async fn test_turn_completed_event_fires_per_iteration() {
        let mock = MockLlm::new(vec![
            Ok(tool_call_response("call_1", "echo", r#"{"message":"turn1"}"#)),
            Ok(tool_call_response("call_2", "echo", r#"{"message":"turn2"}"#)),
            Ok(text_response("Done.")),
        ]);
        let (mut agent, mut rx) = make_agent(mock, echo_registry(), None);

        let result = agent.run(ChatMessage::user("test")).await.unwrap().unwrap_done();
        assert_eq!(result, "Done.");

        let events = collect_events(&mut rx);
        let turn_events: Vec<u32> = events
            .iter()
            .filter_map(|e| {
                if let AgentEvent::TurnCompleted { turn_count, .. } = e {
                    Some(*turn_count)
                } else {
                    None
                }
            })
            .collect();

        assert_eq!(turn_events.len(), 2, "Should have 2 TurnCompleted events");
        assert_eq!(turn_events[0], 1);
        assert_eq!(turn_events[1], 2);
    }

    #[tokio::test]
    async fn test_turn_completed_carries_modified_files() {
        let tmp = tempfile::tempdir().unwrap();
        let file_path = tmp.path().join("test.txt");
        let file_path_str = file_path.to_str().unwrap();

        let mock = MockLlm::new(vec![
            Ok(tool_call_response(
                "call_1",
                "write",
                &format!(r#"{{"filePath":"{}","content":"hello"}}"#, file_path_str),
            )),
            Ok(text_response("Done.")),
        ]);

        let reg = ToolRegistry::for_mode(crate::tool::ToolMode::Coding, None, None);
        let (tx, mut rx) = mpsc::channel(256);
        let mut config = AgentConfig::new(
            crate::llm::LlmClientConfig {
                base_url: "http://unused".into(),
                model: "unused".into(),
                temperature: None,
                max_completion_tokens: None,
                auth_headers: vec![],
                thinking: None,
                disable_cache_control: false,
            },
            tmp.path().to_path_buf(),
        );
        config.retry_config = RetryConfig {
            max_retries: 0,
            initial_delay: std::time::Duration::from_millis(1),
            multiplier: 1.0,
            max_delay: std::time::Duration::from_millis(10),
        };
        let mut agent = AgentLoop::with_provider(
            config,
            Box::new(mock),
            reg,
            CancellationToken::new(),
            tx,
            "test-turn".into(),
        );

        agent.run(ChatMessage::user("write a file")).await.unwrap();

        let events = collect_events(&mut rx);
        let turn_event = events.iter().find_map(|e| {
            if let AgentEvent::TurnCompleted { modified_files, .. } = e {
                Some(modified_files.clone())
            } else {
                None
            }
        });

        let modified = turn_event.expect("Should have TurnCompleted event");
        assert!(
            modified.iter().any(|f| f == file_path_str),
            "modified_files should contain {}, got: {:?}",
            file_path_str,
            modified
        );
    }

    #[tokio::test]
    async fn test_turn_completed_empty_modified_files_for_read_only() {
        let tmp = tempfile::tempdir().unwrap();
        let file_path = tmp.path().join("existing.txt");
        std::fs::write(&file_path, "existing content").unwrap();
        let file_path_str = file_path.to_str().unwrap();

        let mock = MockLlm::new(vec![
            Ok(tool_call_response(
                "call_1",
                "read",
                &format!(r#"{{"filePath":"{}"}}"#, file_path_str),
            )),
            Ok(text_response("Done.")),
        ]);

        let reg = ToolRegistry::for_mode(crate::tool::ToolMode::Coding, None, None);
        let (tx, mut rx) = mpsc::channel(256);
        let mut config = AgentConfig::new(
            crate::llm::LlmClientConfig {
                base_url: "http://unused".into(),
                model: "unused".into(),
                temperature: None,
                max_completion_tokens: None,
                auth_headers: vec![],
                thinking: None,
                disable_cache_control: false,
            },
            tmp.path().to_path_buf(),
        );
        config.retry_config = RetryConfig {
            max_retries: 0,
            initial_delay: std::time::Duration::from_millis(1),
            multiplier: 1.0,
            max_delay: std::time::Duration::from_millis(10),
        };
        let mut agent = AgentLoop::with_provider(
            config,
            Box::new(mock),
            reg,
            CancellationToken::new(),
            tx,
            "test-turn-ro".into(),
        );

        agent.run(ChatMessage::user("read a file")).await.unwrap();

        let events = collect_events(&mut rx);
        let turn_event = events.iter().find_map(|e| {
            if let AgentEvent::TurnCompleted { modified_files, .. } = e {
                Some(modified_files.clone())
            } else {
                None
            }
        });

        let modified = turn_event.expect("Should have TurnCompleted event");
        assert!(modified.is_empty(), "modified_files should be empty for read-only, got: {:?}", modified);
    }

    #[tokio::test]
    async fn test_no_turn_completed_on_text_only_response() {
        let mock = MockLlm::new(vec![Ok(text_response("Hello!"))]);
        let (mut agent, mut rx) = make_agent(mock, ToolRegistry::new(), None);

        agent.run(ChatMessage::user("Hi")).await.unwrap();

        let events = collect_events(&mut rx);
        let has_turn_completed = events.iter().any(|e| matches!(e, AgentEvent::TurnCompleted { .. }));
        assert!(!has_turn_completed, "Should not emit TurnCompleted on text-only response");
        assert!(events.iter().any(|e| matches!(e, AgentEvent::Done { .. })));
    }

    #[tokio::test]
    async fn test_turn_completed_accumulates_parallel_tool_files() {
        let tmp = tempfile::tempdir().unwrap();
        let file_a = tmp.path().join("a.txt");
        let file_b = tmp.path().join("b.txt");
        let path_a = file_a.to_str().unwrap();
        let path_b = file_b.to_str().unwrap();

        let mock = MockLlm::new(vec![
            Ok(multi_tool_call_response(vec![
                ("call_1", "write", &format!(r#"{{"filePath":"{}","content":"aaa"}}"#, path_a)),
                ("call_2", "write", &format!(r#"{{"filePath":"{}","content":"bbb"}}"#, path_b)),
            ])),
            Ok(text_response("Done.")),
        ]);

        let reg = ToolRegistry::for_mode(crate::tool::ToolMode::Coding, None, None);
        let (tx, mut rx) = mpsc::channel(256);
        let mut config = AgentConfig::new(
            crate::llm::LlmClientConfig {
                base_url: "http://unused".into(),
                model: "unused".into(),
                temperature: None,
                max_completion_tokens: None,
                auth_headers: vec![],
                thinking: None,
                disable_cache_control: false,
            },
            tmp.path().to_path_buf(),
        );
        config.retry_config = RetryConfig {
            max_retries: 0,
            initial_delay: std::time::Duration::from_millis(1),
            multiplier: 1.0,
            max_delay: std::time::Duration::from_millis(10),
        };
        let mut agent = AgentLoop::with_provider(
            config,
            Box::new(mock),
            reg,
            CancellationToken::new(),
            tx,
            "test-turn-parallel".into(),
        );

        agent.run(ChatMessage::user("write two files")).await.unwrap();

        let events = collect_events(&mut rx);
        let turn_event = events.iter().find_map(|e| {
            if let AgentEvent::TurnCompleted { modified_files, .. } = e {
                Some(modified_files.clone())
            } else {
                None
            }
        });

        let modified = turn_event.expect("Should have TurnCompleted event");
        assert_eq!(modified.len(), 2, "Should have 2 modified files, got: {:?}", modified);
        assert!(modified.contains(&path_a.to_string()), "Should contain {}", path_a);
        assert!(modified.contains(&path_b.to_string()), "Should contain {}", path_b);
    }

    #[tokio::test]
    async fn test_turn_completed_fires_on_cancelled_turn_with_modified_files() {
        // A write tool + cancel trigger run in parallel.
        // The write modifies a file, then cancellation fires.
        // TurnCompleted should still be emitted with the modified file.
        let tmp = tempfile::tempdir().unwrap();
        let file_path = tmp.path().join("modified.txt");
        let file_path_str = file_path.to_str().unwrap();

        let mut reg = ToolRegistry::for_mode(crate::tool::ToolMode::Coding, None, None);
        reg.register(Arc::new(CancelTriggerTool));

        let mock = MockLlm::new(vec![
            Ok(multi_tool_call_response(vec![
                ("call_1", "write", &format!(r#"{{"filePath":"{}","content":"data"}}"#, file_path_str)),
                ("call_2", "cancel_trigger", "{}"),
            ])),
        ]);

        let (tx, mut rx) = mpsc::channel(256);
        let mut config = AgentConfig::new(
            crate::llm::LlmClientConfig {
                base_url: "http://unused".into(),
                model: "unused".into(),
                temperature: None,
                max_completion_tokens: None,
                auth_headers: vec![],
                thinking: None,
                disable_cache_control: false,
            },
            tmp.path().to_path_buf(),
        );
        config.retry_config = RetryConfig {
            max_retries: 0,
            initial_delay: std::time::Duration::from_millis(1),
            multiplier: 1.0,
            max_delay: std::time::Duration::from_millis(10),
        };
        let mut agent = AgentLoop::with_provider(
            config,
            Box::new(mock),
            reg,
            CancellationToken::new(),
            tx,
            "test-cancel-turn".into(),
        );

        let result = agent.run(ChatMessage::user("write and cancel")).await;
        assert!(matches!(result, Err(AgentError::Cancelled)));

        let events = collect_events(&mut rx);
        let turn_event = events.iter().find_map(|e| {
            if let AgentEvent::TurnCompleted { modified_files, .. } = e {
                Some(modified_files.clone())
            } else {
                None
            }
        });

        let modified = turn_event.expect("TurnCompleted should fire even on cancelled turn");
        assert!(
            modified.iter().any(|f| f == file_path_str),
            "modified_files should contain {}, got: {:?}",
            file_path_str,
            modified
        );
    }

    #[tokio::test]
    async fn test_turn_completed_excludes_failed_write() {
        // Write to a path that will fail (directory that doesn't exist and can't be created
        // because we use a path with a null byte which is invalid).
        let tmp = tempfile::tempdir().unwrap();
        let bad_path = "/nonexistent_root_dir_xyz/impossible/file.txt";

        let mock = MockLlm::new(vec![
            Ok(tool_call_response(
                "call_1",
                "write",
                &format!(r#"{{"filePath":"{}","content":"data"}}"#, bad_path),
            )),
            Ok(text_response("Done.")),
        ]);

        let reg = ToolRegistry::for_mode(crate::tool::ToolMode::Coding, None, None);
        let (tx, mut rx) = mpsc::channel(256);
        let mut config = AgentConfig::new(
            crate::llm::LlmClientConfig {
                base_url: "http://unused".into(),
                model: "unused".into(),
                temperature: None,
                max_completion_tokens: None,
                auth_headers: vec![],
                thinking: None,
                disable_cache_control: false,
            },
            tmp.path().to_path_buf(),
        );
        config.retry_config = RetryConfig {
            max_retries: 0,
            initial_delay: std::time::Duration::from_millis(1),
            multiplier: 1.0,
            max_delay: std::time::Duration::from_millis(10),
        };
        let mut agent = AgentLoop::with_provider(
            config,
            Box::new(mock),
            reg,
            CancellationToken::new(),
            tx,
            "test-failed-write".into(),
        );

        agent.run(ChatMessage::user("write to bad path")).await.unwrap();

        let events = collect_events(&mut rx);
        let turn_event = events.iter().find_map(|e| {
            if let AgentEvent::TurnCompleted { modified_files, .. } = e {
                Some(modified_files.clone())
            } else {
                None
            }
        });

        let modified = turn_event.expect("Should have TurnCompleted event");
        assert!(
            modified.is_empty(),
            "Failed write should not appear in modified_files, got: {:?}",
            modified
        );
    }

    #[tokio::test]
    async fn test_turn_completed_resets_between_turns() {
        // Turn 1 writes file_a, turn 2 writes file_b.
        // Each TurnCompleted should only contain that turn's file.
        let tmp = tempfile::tempdir().unwrap();
        let file_a = tmp.path().join("a.txt");
        let file_b = tmp.path().join("b.txt");
        let path_a = file_a.to_str().unwrap();
        let path_b = file_b.to_str().unwrap();

        let mock = MockLlm::new(vec![
            Ok(tool_call_response(
                "call_1",
                "write",
                &format!(r#"{{"filePath":"{}","content":"aaa"}}"#, path_a),
            )),
            Ok(tool_call_response(
                "call_2",
                "write",
                &format!(r#"{{"filePath":"{}","content":"bbb"}}"#, path_b),
            )),
            Ok(text_response("Done.")),
        ]);

        let reg = ToolRegistry::for_mode(crate::tool::ToolMode::Coding, None, None);
        let (tx, mut rx) = mpsc::channel(256);
        let mut config = AgentConfig::new(
            crate::llm::LlmClientConfig {
                base_url: "http://unused".into(),
                model: "unused".into(),
                temperature: None,
                max_completion_tokens: None,
                auth_headers: vec![],
                thinking: None,
                disable_cache_control: false,
            },
            tmp.path().to_path_buf(),
        );
        config.retry_config = RetryConfig {
            max_retries: 0,
            initial_delay: std::time::Duration::from_millis(1),
            multiplier: 1.0,
            max_delay: std::time::Duration::from_millis(10),
        };
        let mut agent = AgentLoop::with_provider(
            config,
            Box::new(mock),
            reg,
            CancellationToken::new(),
            tx,
            "test-turn-reset".into(),
        );

        agent.run(ChatMessage::user("write files across turns")).await.unwrap();

        let events = collect_events(&mut rx);
        let turn_events: Vec<(u32, Vec<String>)> = events
            .iter()
            .filter_map(|e| {
                if let AgentEvent::TurnCompleted { turn_count, modified_files, .. } = e {
                    Some((*turn_count, modified_files.clone()))
                } else {
                    None
                }
            })
            .collect();

        assert_eq!(turn_events.len(), 2, "Should have 2 TurnCompleted events");

        // Turn 1 should only have file_a
        assert_eq!(turn_events[0].0, 1);
        assert_eq!(turn_events[0].1, vec![path_a.to_string()], "Turn 1 should only contain a.txt");

        // Turn 2 should only have file_b
        assert_eq!(turn_events[1].0, 2);
        assert_eq!(turn_events[1].1, vec![path_b.to_string()], "Turn 2 should only contain b.txt");
    }

    // ── Doom loop detection tests ──

    /// A tool that always returns an error.
    struct FailTool;

    #[async_trait::async_trait]
    impl Tool for FailTool {
        fn name(&self) -> &str { "fail" }
        fn description(&self) -> &str { "Always fails" }
        fn parameters_schema(&self) -> serde_json::Value {
            json!({"type": "object"})
        }
        async fn execute(
            &self,
            _args: serde_json::Value,
            _ctx: &ToolContext,
        ) -> Result<ToolResult, ToolError> {
            Ok(ToolResult::error("This tool always fails."))
        }
    }

    #[tokio::test]
    async fn test_doom_loop_injects_hint_after_3_failures() {
        // LLM calls the fail tool 4 times (3 failures trigger hint, then 1 more), then returns text.
        let mock = MockLlm::new(vec![
            Ok(tool_call_response("c1", "fail", "{}")),
            Ok(tool_call_response("c2", "fail", "{}")),
            Ok(tool_call_response("c3", "fail", "{}")),
            // After doom loop hint injected, LLM gets one more chance
            Ok(text_response("I give up.")),
        ]);

        let mut reg = ToolRegistry::new();
        reg.register(Arc::new(FailTool));

        let (mut agent, mut rx) = make_agent(mock, reg, Some(10));
        let result = agent.run(ChatMessage::user("do something")).await.unwrap();

        // Should complete normally (not error out)
        let summary = result.unwrap_done();
        assert!(summary.contains("give up"));

        // Should have emitted an Error event about doom loop
        let events = collect_events(&mut rx);
        let doom_event = events.iter().any(|e| {
            if let AgentEvent::Error { message, .. } = e {
                message.contains("Doom loop")
            } else {
                false
            }
        });
        assert!(doom_event, "Should have doom loop error event");

        // Verify the hint system message was injected (LLM received it on the 4th call)
        // The LLM returned text after the hint, which means it saw the hint message.
    }

    #[tokio::test]
    async fn test_doom_loop_resets_on_success() {
        // 2 failures, then 1 success (resets counter), then 2 more failures → no doom loop
        let mock = MockLlm::new(vec![
            Ok(tool_call_response("c1", "fail", "{}")),       // fail 1
            Ok(tool_call_response("c2", "fail", "{}")),       // fail 2
            Ok(tool_call_response("c3", "echo", r#"{"message":"ok"}"#)), // success → resets
            Ok(tool_call_response("c4", "fail", "{}")),       // fail 1 (reset)
            Ok(tool_call_response("c5", "fail", "{}")),       // fail 2
            Ok(text_response("Done.")),
        ]);

        let mut reg = ToolRegistry::new();
        reg.register(Arc::new(FailTool));
        reg.register(Arc::new(EchoTool));

        let (mut agent, mut rx) = make_agent(mock, reg, Some(10));
        agent.run(ChatMessage::user("test")).await.unwrap();

        // Should NOT have doom loop event (never hit 3 consecutive)
        let events = collect_events(&mut rx);
        let doom_event = events.iter().any(|e| {
            if let AgentEvent::Error { message, .. } = e {
                message.contains("Doom loop")
            } else {
                false
            }
        });
        assert!(!doom_event, "Should NOT have doom loop event — success reset the counter");
    }

    // ── AskUser yield tests ──

    #[tokio::test]
    async fn test_ask_user_yield_returns_result() {
        use crate::tool::ask_user::AskUserTool;

        let mock = MockLlm::new(vec![
            Ok(tool_call_response(
                "c1",
                "ask_user",
                r#"{"question":"Which database?","options":["Postgres","MySQL"]}"#,
            )),
        ]);

        let mut reg = ToolRegistry::new();
        reg.register(Arc::new(AskUserTool));

        let (mut agent, mut rx) = make_agent(mock, reg, Some(10));
        let result = agent.run(ChatMessage::user("help me choose")).await.unwrap();

        // Should yield AskUser, not Done
        match result {
            AgentResult::AskUser { question, options } => {
                assert!(question.contains("database"), "Got: {question}");
                assert_eq!(options.as_ref().unwrap().len(), 2);
            }
            other => panic!("Expected AskUser, got {:?}", other),
        }

        // Should have emitted UserQuestionAsked event
        let events = collect_events(&mut rx);
        let question_event = events.iter().any(|e| {
            matches!(e, AgentEvent::UserQuestionAsked { .. })
        });
        assert!(question_event, "Should have UserQuestionAsked event");
    }

    // ── Truncation handling tests ──

    /// Tool that returns output exceeding the 2000-line / 50KB limit.
    struct HugeOutputTool;

    #[async_trait::async_trait]
    impl Tool for HugeOutputTool {
        fn name(&self) -> &str { "huge" }
        fn description(&self) -> &str { "Returns huge output" }
        fn parameters_schema(&self) -> serde_json::Value {
            json!({"type": "object"})
        }
        async fn execute(
            &self,
            _args: serde_json::Value,
            _ctx: &ToolContext,
        ) -> Result<ToolResult, ToolError> {
            // Generate 3000 lines (exceeds 2000 line limit)
            let output: String = (0..3000)
                .map(|i| format!("line {}: {}", i, "x".repeat(50)))
                .collect::<Vec<_>>()
                .join("\n");
            Ok(ToolResult::success(output))
        }
    }

    #[tokio::test]
    async fn test_truncation_saves_full_output_to_file() {
        let tmp = tempfile::tempdir().unwrap();

        let mock = MockLlm::new(vec![
            Ok(tool_call_response("c1", "huge", "{}")),
            Ok(text_response("Done.")),
        ]);

        let mut reg = ToolRegistry::new();
        reg.register(Arc::new(HugeOutputTool));

        let (tx, mut rx) = mpsc::channel(256);
        let mut config = AgentConfig::new(
            crate::llm::LlmClientConfig {
                base_url: "http://unused".into(),
                model: "unused".into(),
                temperature: None,
                max_completion_tokens: None,
                auth_headers: vec![],
                thinking: None,
                disable_cache_control: false,
            },
            tmp.path().to_path_buf(),
        );
        config.retry_config = RetryConfig {
            max_retries: 0,
            initial_delay: std::time::Duration::from_millis(1),
            multiplier: 1.0,
            max_delay: std::time::Duration::from_millis(10),
        };
        let mut agent = AgentLoop::with_provider(
            config,
            Box::new(mock),
            reg,
            CancellationToken::new(),
            tx,
            "test-truncation".into(),
        );

        agent.run(ChatMessage::user("generate huge output")).await.unwrap();

        // Check that the tool output file was created
        let tool_output_dir = tmp.path().join(".agent").join("tool-output");
        assert!(tool_output_dir.exists(), ".agent/tool-output/ should exist");

        let files: Vec<_> = std::fs::read_dir(&tool_output_dir)
            .unwrap()
            .filter_map(|e| e.ok())
            .collect();
        assert_eq!(files.len(), 1, "Should have exactly one truncated output file");

        // The saved file should contain the full 3000 lines
        let saved_content = std::fs::read_to_string(files[0].path()).unwrap();
        let saved_lines = saved_content.lines().count();
        assert!(saved_lines >= 3000, "Saved file should have full output, got {} lines", saved_lines);

        // The tool result in the LLM context should mention the file path
        let events = collect_events(&mut rx);
        let tool_end = events.iter().find_map(|e| {
            if let AgentEvent::ToolEnd { summary, .. } = e {
                Some(summary.clone())
            } else {
                None
            }
        });
        assert!(tool_end.is_some(), "Should have ToolEnd event");
        // The summary is truncated at 200 chars, but the actual tool result sent to LLM
        // contains "Full output saved to" — we verify via the file existence above.
    }

    // ── Edit strategy tests (via tool execution) ──

    #[tokio::test]
    async fn test_edit_whitespace_normalized_strategy() {
        let tmp = tempfile::tempdir().unwrap();
        // Create file with extra whitespace — fuzzy matching needed
        std::fs::write(
            tmp.path().join("test.rs"),
            "fn  main()  {\n    println!(  \"hello\"  );\n}\n",
        ).unwrap();

        let mock = MockLlm::new(vec![
            Ok(tool_call_response(
                "c1",
                "edit",
                &serde_json::json!({
                    "filePath": "test.rs",
                    "oldString": "fn main() {\n    println!( \"hello\" );\n}",
                    "newString": "fn main() {\n    println!(\"world\");\n}"
                }).to_string(),
            )),
            Ok(text_response("Done.")),
        ]);

        let (tx, _rx) = mpsc::channel(256);
        let mut config = AgentConfig::new(
            crate::llm::LlmClientConfig {
                base_url: "http://unused".into(),
                model: "unused".into(),
                temperature: None,
                max_completion_tokens: None,
                auth_headers: vec![],
                thinking: None,
                disable_cache_control: false,
            },
            tmp.path().to_path_buf(),
        );
        config.retry_config = RetryConfig {
            max_retries: 0,
            initial_delay: std::time::Duration::from_millis(1),
            multiplier: 1.0,
            max_delay: std::time::Duration::from_millis(10),
        };
        let mut agent = AgentLoop::with_provider(
            config,
            Box::new(mock),
            ToolRegistry::for_mode(crate::tool::ToolMode::Coding, None, None),
            CancellationToken::new(),
            tx,
            "test-edit-ws".into(),
        );

        agent.run(ChatMessage::user("fix the file")).await.unwrap();

        let content = std::fs::read_to_string(tmp.path().join("test.rs")).unwrap();
        assert!(content.contains("world"), "Edit should have applied. Content: {content}");
    }

    // ── save_plan yield test ──

    #[tokio::test]
    async fn test_save_plan_yield_returns_plan_ready() {
        use crate::tool::save_plan::SavePlanTool;

        let tmp = tempfile::tempdir().unwrap();

        let mock = MockLlm::new(vec![
            Ok(tool_call_response(
                "c1",
                "save_plan",
                &serde_json::json!({"plan": "## Goal\nFix the bug\n## Steps\n1. Read file\n2. Edit file", "filename": "plan-fix-bug.md"}).to_string(),
            )),
        ]);

        let mut reg = ToolRegistry::new();
        reg.register(Arc::new(SavePlanTool));

        let (tx, mut rx) = mpsc::channel(256);
        let mut config = AgentConfig::new(
            crate::llm::LlmClientConfig {
                base_url: "http://unused".into(),
                model: "unused".into(),
                temperature: None,
                max_completion_tokens: None,
                auth_headers: vec![],
                thinking: None,
                disable_cache_control: false,
            },
            tmp.path().to_path_buf(),
        );
        config.retry_config = RetryConfig {
            max_retries: 0,
            initial_delay: std::time::Duration::from_millis(1),
            multiplier: 1.0,
            max_delay: std::time::Duration::from_millis(10),
        };
        let mut agent = AgentLoop::with_provider(
            config,
            Box::new(mock),
            reg,
            CancellationToken::new(),
            tx,
            "test-save-plan".into(),
        );

        let result = agent.run(ChatMessage::user("make a plan")).await.unwrap();

        // Should yield PlanReady
        match result {
            AgentResult::PlanReady { plan, plan_path } => {
                assert!(plan.contains("Fix the bug"), "Plan should contain goal. Got: {plan}");
                assert!(plan_path.contains("plan-fix-bug.md"), "Path should use the filename. Got: {plan_path}");
            }
            other => panic!("Expected PlanReady, got {:?}", other),
        }

        // File should exist on disk
        let plan_file = tmp.path().join(".agent").join("plan-fix-bug.md");
        assert!(plan_file.exists(), "Plan file should be written to disk");

        // Should have PlanReady event
        let events = collect_events(&mut rx);
        let plan_event = events.iter().any(|e| {
            matches!(e, AgentEvent::PlanReady { .. })
        });
        assert!(plan_event, "Should have PlanReady event");
    }

    // ══════════════════════════════════════════════
    // Compaction fix tests
    // ══════════════════════════════════════════════

    /// Helper: make an agent with tiny compaction thresholds for testing.
    fn make_compact_agent(
        mock: MockLlm,
        registry: ToolRegistry,
    ) -> (AgentLoop, mpsc::Receiver<AgentEvent>) {
        let (tx, rx) = mpsc::channel(256);
        let mut config = AgentConfig::new(
            crate::llm::LlmClientConfig {
                base_url: "http://unused".into(),
                model: "unused".into(),
                temperature: None,
                max_completion_tokens: None,
                auth_headers: vec![],
                thinking: None,
                disable_cache_control: false,
            },
            std::path::PathBuf::from("/tmp"),
        );
        config.max_iterations = 20;
        config.retry_config = RetryConfig {
            max_retries: 0,
            initial_delay: std::time::Duration::from_millis(1),
            multiplier: 1.0,
            max_delay: std::time::Duration::from_millis(10),
        };
        // Tiny context: 200 tokens, 80% threshold = 160 tokens
        config.compaction_config.context_limit = 200;
        config.compaction_config.threshold_pct = 0.80;
        config.compaction_config.keep_recent_messages = 2;

        let agent = AgentLoop::with_provider(
            config,
            Box::new(mock),
            registry,
            CancellationToken::new(),
            tx,
            "test-compact".into(),
        );
        (agent, rx)
    }

    /// Build a seeded context with clean compaction boundaries.
    /// Returns messages: [system, user, assistant_text, user, assistant_text, user, assistant_text]
    /// The text-only assistant messages create clean snap boundaries for compaction.
    fn compactable_context() -> Vec<ChatMessage> {
        vec![
            ChatMessage::user("question 1"),
            ChatMessage::assistant(Some("answer 1".into()), None, None),
            ChatMessage::user("question 2"),
            ChatMessage::assistant(Some("answer 2".into()), None, None),
            ChatMessage::user("question 3"),
            ChatMessage::assistant(Some("answer 3".into()), None, None),
        ]
    }

    #[tokio::test]
    async fn test_post_tool_compaction_triggers_on_high_token_usage() {
        // Seed with 6 context messages (clean boundaries), then one tool-call turn
        // reports 170 tokens (above 160 threshold) → post-tool compaction fires.
        let mock = MockLlm::new(vec![
            // Turn 1: high token usage → triggers post-tool compaction
            Ok(tool_call_response_with_usage("c1", "echo", r#"{"message":"hi"}"#, 170)),
            // Compaction LLM call → returns summary
            Ok(text_response("Summary of conversation so far.")),
            // Turn 2: after compaction, LLM finishes
            Ok(text_response("Done.")),
        ]);

        let (mut agent, mut rx) = make_compact_agent(mock, echo_registry());
        agent = agent.with_initial_context(compactable_context());
        agent.run(ChatMessage::user("now do something")).await.unwrap();

        let events = collect_events(&mut rx);
        let compaction_fired = events.iter().any(|e| matches!(e, AgentEvent::Compaction { .. }));
        assert!(compaction_fired, "Compaction should have fired from post-tool check");
    }

    #[tokio::test]
    async fn test_pre_llm_compaction_triggers_on_large_tool_output() {
        // Seed with context. Turn 1 reports low tokens (50), but HugeOutputTool
        // dumps massive text. Pre-LLM check before turn 2 estimates
        // 50 + huge_output/4 > threshold → compaction fires BEFORE the LLM call.
        let mock = MockLlm::new(vec![
            // Turn 1: low token usage, calls huge output tool
            Ok(tool_call_response_with_usage("c1", "huge", "{}", 50)),
            // Compaction LLM call (triggered by pre-LLM check before turn 2)
            Ok(text_response("Summary after huge output.")),
            // Turn 2: after compaction
            Ok(text_response("Done.")),
        ]);

        let mut reg = ToolRegistry::new();
        reg.register(Arc::new(HugeOutputTool));

        let (mut agent, mut rx) = make_compact_agent(mock, reg);
        agent = agent.with_initial_context(compactable_context());
        agent.run(ChatMessage::user("generate huge output")).await.unwrap();

        let events = collect_events(&mut rx);
        let compaction_fired = events.iter().any(|e| matches!(e, AgentEvent::Compaction { .. }));
        assert!(compaction_fired, "Compaction should have fired from pre-LLM check due to large tool output");
    }

    #[tokio::test]
    async fn test_compaction_cooldown_prevents_immediate_refire() {
        // Compaction fires on turn 1. Turns 2-3 report high tokens but cooldown
        // prevents compaction from firing again. The agent should complete normally
        // without consuming extra compaction LLM responses (proving cooldown worked).
        let mock = MockLlm::new(vec![
            // Turn 1: high usage → triggers compaction
            Ok(tool_call_response_with_usage("c1", "echo", r#"{"message":"a"}"#, 170)),
            // Compaction LLM call
            Ok(text_response("Summary.")),
            // Turn 2: high usage — cooldown blocks compaction (no compaction LLM call consumed)
            Ok(tool_call_response_with_usage("c2", "echo", r#"{"message":"b"}"#, 170)),
            // Turn 3: still cooling down (no compaction LLM call consumed)
            Ok(text_response("Done.")),
        ]);

        let (mut agent, mut rx) = make_compact_agent(mock, echo_registry());
        agent = agent.with_initial_context(compactable_context());
        agent.run(ChatMessage::user("keep going")).await.unwrap();

        let events = collect_events(&mut rx);
        let compaction_count = events.iter().filter(|e| matches!(e, AgentEvent::Compaction { .. })).count();
        // Compaction fires exactly once — if cooldown didn't work, the agent would
        // try to consume a compaction LLM response that doesn't exist and panic.
        assert_eq!(compaction_count, 1, "Compaction should fire exactly once — cooldown blocks turn 2");
    }

    #[tokio::test]
    async fn test_compaction_fallback_truncation_on_llm_failure() {
        // Compaction LLM call fails twice → fallback to aggressive truncation.
        // The agent should survive and continue.
        use crate::error::AgentError;

        let mock = MockLlm::new(vec![
            // Turn 1: high usage → triggers compaction
            Ok(tool_call_response_with_usage("c1", "echo", r#"{"message":"hi"}"#, 170)),
            // Compaction LLM attempt 1 → fails
            Err(AgentError::LlmParseError("compaction failed".into())),
            // Compaction LLM attempt 2 (retry) → fails again
            Err(AgentError::LlmParseError("compaction failed again".into())),
            // After fallback truncation, turn 2: agent finishes
            Ok(text_response("Done after truncation.")),
        ]);

        let (mut agent, mut rx) = make_compact_agent(mock, echo_registry());
        agent = agent.with_initial_context(compactable_context());
        let result = agent.run(ChatMessage::user("test")).await.unwrap();

        let summary = result.unwrap_done();
        assert!(summary.contains("truncation"), "Agent should finish after fallback truncation");

        let events = collect_events(&mut rx);
        let compaction_fired = events.iter().any(|e| matches!(e, AgentEvent::Compaction { .. }));
        assert!(compaction_fired, "Compaction event should fire even for fallback truncation");
    }

    // ════════════════════════════════════════════
    // strip_orphaned_tool_results tests (Bug 3 fix)
    // ════════════════════════════════════════════

    #[tokio::test]
    async fn test_strip_removes_orphan_tool_results() {
        let mock = MockLlm::new(vec![Ok(text_response("done"))]);
        let (mut agent, _rx) = make_agent(mock, echo_registry(), None);

        // Set up: assistant has tc1, but a stray tool_result for tc_orphan also exists
        let tc = vec![ToolCall {
            id: "tc1".into(),
            type_: "function".into(),
            function: FunctionCall { name: "echo".into(), arguments: "{}".into() },
        }];
        agent.messages = vec![
            ChatMessage::user("hi"),
            ChatMessage::assistant(None, Some(tc), None),
            ChatMessage::tool_result("tc1", "valid result"),
            ChatMessage::tool_result("tc_orphan", "orphaned result"),
        ];

        agent.strip_orphaned_tool_results();

        // Orphan removed, valid result kept, assistant tool_calls intact
        assert_eq!(agent.messages.len(), 3);
        assert!(agent.messages.iter().all(|m| m.tool_call_id.as_deref() != Some("tc_orphan")));
        let assistant = agent.messages.iter().find(|m| m.role == "assistant").unwrap();
        assert!(assistant.tool_calls.is_some(), "valid tool_calls should be preserved");
    }

    #[tokio::test]
    async fn test_strip_clears_dangling_tool_calls_when_all_results_missing() {
        let mock = MockLlm::new(vec![Ok(text_response("done"))]);
        let (mut agent, _rx) = make_agent(mock, echo_registry(), None);

        // Assistant declared two tool_calls but neither has a tool_result —
        // this is the scenario that produces "tool_use without tool_result" errors.
        let tc = vec![
            ToolCall {
                id: "tc1".into(),
                type_: "function".into(),
                function: FunctionCall { name: "echo".into(), arguments: "{}".into() },
            },
            ToolCall {
                id: "tc2".into(),
                type_: "function".into(),
                function: FunctionCall { name: "echo".into(), arguments: "{}".into() },
            },
        ];
        agent.messages = vec![
            ChatMessage::user("hi"),
            ChatMessage::assistant(Some("partial".into()), Some(tc), None),
        ];

        agent.strip_orphaned_tool_results();

        // tool_calls field should be cleared entirely (None), turning the
        // assistant message into a plain text turn.
        let assistant = agent.messages.iter().find(|m| m.role == "assistant").unwrap();
        assert!(
            assistant.tool_calls.is_none(),
            "assistant.tool_calls should be cleared when all results are missing"
        );
        // Content is preserved
        assert!(assistant.content.is_some());
    }

    #[tokio::test]
    async fn test_strip_partial_clears_only_unanswered_calls() {
        let mock = MockLlm::new(vec![Ok(text_response("done"))]);
        let (mut agent, _rx) = make_agent(mock, echo_registry(), None);

        // Assistant has tc1 (answered) and tc2 (unanswered)
        let tc = vec![
            ToolCall {
                id: "tc1".into(),
                type_: "function".into(),
                function: FunctionCall { name: "echo".into(), arguments: "{}".into() },
            },
            ToolCall {
                id: "tc2".into(),
                type_: "function".into(),
                function: FunctionCall { name: "echo".into(), arguments: "{}".into() },
            },
        ];
        agent.messages = vec![
            ChatMessage::user("hi"),
            ChatMessage::assistant(None, Some(tc), None),
            ChatMessage::tool_result("tc1", "ok"),
        ];

        agent.strip_orphaned_tool_results();

        let assistant = agent.messages.iter().find(|m| m.role == "assistant").unwrap();
        let tcs = assistant.tool_calls.as_ref().expect("tc1 is answered, field stays");
        assert_eq!(tcs.len(), 1, "only tc1 should remain");
        assert_eq!(tcs[0].id, "tc1");
    }

    #[tokio::test]
    async fn test_estimate_token_count() {
        // Verify the chars/4 estimate is in a reasonable ballpark
        let messages = vec![
            ChatMessage::system("You are helpful."),  // 16 chars → ~4 tokens + 4 overhead = 8
            ChatMessage::user("Hello world test"),     // 16 chars → ~4 tokens + 4 overhead = 8
        ];
        let estimate = compaction::estimate_token_count(&messages);
        assert!(estimate > 0, "Estimate should be positive");
        assert!(estimate < 100, "Estimate for 2 short messages should be small, got {}", estimate);

        // Large message should produce proportionally larger estimate
        let big_content = "x".repeat(4000); // 4000 chars → ~1000 tokens
        let big_messages = vec![ChatMessage::user(&big_content)];
        let big_estimate = compaction::estimate_token_count(&big_messages);
        assert!(big_estimate >= 1000, "4000 chars should estimate to at least 1000 tokens, got {}", big_estimate);
    }
}
