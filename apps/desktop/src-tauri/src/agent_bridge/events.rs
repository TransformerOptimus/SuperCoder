use std::collections::{HashMap, HashSet};
use std::path::PathBuf;
use std::sync::{Arc, Mutex};
use std::time::Instant;

use async_trait::async_trait;
use serde_json::Value;
use tokio::sync::{mpsc, oneshot};

use agent::approval::{ApprovalDecision, ApprovalHandler};
use agent::types::AgentEvent;

use crate::agent_bridge::db::AgentDb;
use crate::agent_bridge::permissions::PermissionConfig;
use crate::agent_bridge::traits::{emit_or_log, EventEmitter};

// ── Message accumulator ───────────────────────────────────────────────────────
//
// Builds the finalized assistant message content (thinking markers + text) that
// the frontend renders as a collapsible "Thought for N seconds" block followed
// by the reply text. Tool calls between text become thinking steps.

pub struct MessageAccumulator {
    thinking_steps: Vec<String>,
    thinking_start: Option<Instant>,
    text_buffer: String,
}

impl MessageAccumulator {
    pub fn new() -> Self {
        Self {
            thinking_steps: Vec::new(),
            thinking_start: None,
            text_buffer: String::new(),
        }
    }

    /// On tool start: flush any pending text as a finalized message, record the step.
    pub fn on_tool_start(&mut self, _tool_name: &str, args_summary: &str) -> Option<String> {
        let flushed = (!self.text_buffer.is_empty()).then(|| self.build_content());
        if self.thinking_start.is_none() {
            self.thinking_start = Some(Instant::now());
        }
        self.thinking_steps.push(args_summary.to_string());
        flushed
    }

    pub fn on_text_delta(&mut self, delta: &str) {
        self.text_buffer.push_str(delta);
    }

    /// Build the final content string with thinking markers + text, then reset.
    pub fn build_content(&mut self) -> String {
        let mut content = String::new();
        if !self.thinking_steps.is_empty() {
            let duration = self.thinking_start.map(|s| s.elapsed().as_secs()).unwrap_or(0);
            content.push_str(&format!("<!-- thinking duration={duration} -->\n"));
            for step in &self.thinking_steps {
                content.push_str(step);
                content.push('\n');
            }
            content.push_str("<!-- /thinking -->\n\n");
        }
        content.push_str(&self.text_buffer);
        self.thinking_steps.clear();
        self.thinking_start = None;
        self.text_buffer.clear();
        content
    }

    pub fn has_content(&self) -> bool {
        !self.text_buffer.is_empty() || !self.thinking_steps.is_empty()
    }
}

impl Default for MessageAccumulator {
    fn default() -> Self {
        Self::new()
    }
}

// ── Relay ─────────────────────────────────────────────────────────────────────

/// Bundles relay configuration. `session_id` doubles as the `thread_id` carried
/// in every frontend event (the UI keys streaming state by it).
struct RelayContext {
    emitter: Arc<dyn EventEmitter>,
    db: Option<Arc<AgentDb>>,
    session_id: String,
    project_path: String,
    /// Snapshot checkpoint root (`<data>/.supercoder/checkpoints`). `diff_turn`
    /// is computed against `<root>/<session_id>/turn-N`. None disables diffs.
    checkpoint_dir: Option<PathBuf>,
    checkpoint_handles: Arc<tokio::sync::Mutex<Vec<tokio::task::JoinHandle<()>>>>,
}

impl RelayContext {
    fn flush_content(&self, content: &str) {
        emit_message_complete(self.emitter.as_ref(), &self.session_id, content);
    }
}

/// Emit a finalized assistant message to the frontend so the streaming bubble
/// becomes a permanent thread message. The agent loop has already persisted the
/// real message to SQLite; this carries the display content (incl. thinking).
fn emit_message_complete(emitter: &dyn EventEmitter, session_id: &str, content: &str) {
    let message = serde_json::json!({
        "id": uuid::Uuid::new_v4().to_string(),
        "role": "assistant",
        "content": content,
        "created_at": chrono::Utc::now().format("%Y-%m-%dT%H:%M:%S%.3fZ").to_string(),
    });
    emit_or_log(
        emitter,
        "agent:message_complete",
        serde_json::json!({ "thread_id": session_id, "message": message }),
    );
}

/// Spawn the event relay: drains `event_rx`, maps each `AgentEvent` to a
/// namespaced Tauri event, captures turn diffs from the snapshot dir, and
/// persists token usage. Returns a JoinHandle the caller tracks for cancel.
#[allow(clippy::too_many_arguments)]
pub fn spawn_event_relay(
    emitter: Arc<dyn EventEmitter>,
    mut event_rx: mpsc::Receiver<AgentEvent>,
    session_id: String,
    db: Option<Arc<AgentDb>>,
    project_path: String,
    checkpoint_dir: Option<PathBuf>,
) -> tokio::task::JoinHandle<()> {
    let ctx = RelayContext {
        emitter,
        db,
        session_id,
        project_path,
        checkpoint_dir,
        checkpoint_handles: Arc::new(tokio::sync::Mutex::new(Vec::new())),
    };

    tokio::spawn(async move {
        let mut accumulator = MessageAccumulator::new();
        let mut subagent_tool_call_ids: HashSet<String> = HashSet::new();
        let mut saw_done = false;

        while let Some(event) = event_rx.recv().await {
            if relay_event(&ctx, &mut accumulator, &mut subagent_tool_call_ids, event).await {
                saw_done = true;
            }
        }

        // Flush any trailing content if the channel closed without a Done.
        if !saw_done && accumulator.has_content() {
            let content = accumulator.build_content();
            ctx.flush_content(&content);
            emit_or_log(
                ctx.emitter.as_ref(),
                "agent:done",
                serde_json::json!({ "thread_id": ctx.session_id, "summary": Value::Null }),
            );
        }

        // Await in-flight turn-diff tasks so the final turn's diff lands.
        let handles: Vec<_> = std::mem::take(&mut *ctx.checkpoint_handles.lock().await);
        for h in handles {
            let _ = h.await;
        }
    })
}

async fn relay_event(
    ctx: &RelayContext,
    accumulator: &mut MessageAccumulator,
    subagent_tool_call_ids: &mut HashSet<String>,
    event: AgentEvent,
) -> bool {
    let tid = ctx.session_id.clone();
    match event {
        AgentEvent::ThinkingDelta { .. } => false,

        AgentEvent::TextDelta { delta, .. } => {
            accumulator.on_text_delta(&delta);
            emit_or_log(
                ctx.emitter.as_ref(),
                "agent:text_delta",
                serde_json::json!({ "thread_id": tid, "delta": delta }),
            );
            false
        }

        AgentEvent::ToolStart { tool_call_id, tool_name, args_summary, .. } => {
            // spawn_subagent renders only as a SubagentStart chip.
            if tool_name == "spawn_subagent" {
                subagent_tool_call_ids.insert(tool_call_id);
                return false;
            }
            if let Some(content) = accumulator.on_tool_start(&tool_name, &args_summary) {
                ctx.flush_content(&content);
            }
            emit_or_log(
                ctx.emitter.as_ref(),
                "agent:tool_start",
                serde_json::json!({
                    "thread_id": tid,
                    "tool_call_id": tool_call_id,
                    "tool_name": tool_name,
                    "args_summary": args_summary,
                }),
            );
            false
        }

        AgentEvent::ToolEnd { tool_call_id, success, summary, modified_files, .. } => {
            if subagent_tool_call_ids.remove(&tool_call_id) {
                return false;
            }
            emit_or_log(
                ctx.emitter.as_ref(),
                "agent:tool_end",
                serde_json::json!({
                    "thread_id": tid,
                    "tool_call_id": tool_call_id,
                    "success": success,
                    "summary": summary,
                    "modified_files": modified_files,
                }),
            );
            false
        }

        AgentEvent::ToolStatus { .. } => false,

        AgentEvent::Error { message, retrying, .. } => {
            emit_or_log(
                ctx.emitter.as_ref(),
                "agent:error",
                serde_json::json!({ "thread_id": tid, "message": message, "retrying": retrying }),
            );
            false
        }

        AgentEvent::Compaction { .. } => {
            emit_or_log(
                ctx.emitter.as_ref(),
                "agent:compaction",
                serde_json::json!({ "thread_id": tid }),
            );
            false
        }

        AgentEvent::TokenUsage {
            total_tokens, context_limit, cache_read_tokens, cache_creation_tokens, ..
        } => {
            emit_or_log(
                ctx.emitter.as_ref(),
                "agent:token_usage",
                serde_json::json!({
                    "thread_id": tid,
                    "total_tokens": total_tokens,
                    "context_limit": context_limit,
                    "cache_read_tokens": cache_read_tokens,
                    "cache_creation_tokens": cache_creation_tokens,
                }),
            );
            // Persist usage so the indicator survives app refresh (fire-and-forget).
            if let Some(ref db_ref) = ctx.db {
                let db = Arc::clone(db_ref);
                let sid = ctx.session_id.clone();
                let pp = ctx.project_path.clone();
                let _ = tokio::task::spawn_blocking(move || {
                    if let Err(e) = db.upsert_context_usage(&sid, &pp, total_tokens, context_limit, 0) {
                        log::warn!("[Relay] Failed to persist context usage: {e}");
                    }
                });
            }
            false
        }

        AgentEvent::Done { summary, .. } => {
            if accumulator.has_content() {
                let content = accumulator.build_content();
                ctx.flush_content(&content);
            }
            emit_or_log(
                ctx.emitter.as_ref(),
                "agent:done",
                serde_json::json!({ "thread_id": tid, "summary": summary }),
            );
            true
        }

        AgentEvent::TurnCompleted { turn_count, modified_files, .. } => {
            if let Some(ref dir) = ctx.checkpoint_dir {
                let dir = dir.clone();
                let sid = ctx.session_id.clone();
                let emitter = ctx.emitter.clone();
                let files = modified_files;
                let handle = tokio::spawn(async move {
                    match git_ops::checkpoint::diff_turn(&dir, &sid, turn_count).await {
                        Ok(d) => {
                            emit_or_log(
                                emitter.as_ref(),
                                "agent:turn_diff_completed",
                                serde_json::json!({
                                    "thread_id": sid,
                                    "turn_count": turn_count,
                                    "files": files,
                                    "additions": d.insertions,
                                    "deletions": d.deletions,
                                    "diff": d.diff,
                                    "stat": d.stat,
                                    "status": "ready",
                                }),
                            );
                        }
                        Err(e) => {
                            log::warn!("[Checkpoint] diff_turn failed for turn {turn_count}: {e}");
                            emit_or_log(
                                emitter.as_ref(),
                                "agent:turn_diff_completed",
                                serde_json::json!({
                                    "thread_id": sid,
                                    "turn_count": turn_count,
                                    "files": files,
                                    "additions": 0,
                                    "deletions": 0,
                                    "diff": "",
                                    "stat": "",
                                    "status": "partial",
                                }),
                            );
                        }
                    }
                });
                ctx.checkpoint_handles.lock().await.push(handle);
            }
            false
        }

        AgentEvent::UserQuestionAsked { session_id, question, options } => {
            emit_or_log(
                ctx.emitter.as_ref(),
                "agent:question_asked",
                serde_json::json!({
                    "thread_id": tid,
                    "session_id": session_id,
                    "question": question,
                    "options": options,
                }),
            );
            false
        }

        AgentEvent::PlanReady { session_id, plan, plan_path, project_path } => {
            emit_or_log(
                ctx.emitter.as_ref(),
                "agent:plan_ready",
                serde_json::json!({
                    "thread_id": tid,
                    "session_id": session_id,
                    "plan": plan,
                    "plan_path": plan_path,
                    "project_path": project_path,
                }),
            );
            false
        }

        AgentEvent::TodoUpdated { todos, .. } => {
            emit_or_log(
                ctx.emitter.as_ref(),
                "agent:todo_updated",
                serde_json::json!({ "thread_id": tid, "todos": todos }),
            );
            false
        }

        AgentEvent::SkillLoaded { skill_name, .. } => {
            emit_or_log(
                ctx.emitter.as_ref(),
                "agent:skill_loaded",
                serde_json::json!({ "thread_id": tid, "skill_name": skill_name }),
            );
            false
        }

        AgentEvent::SubagentStart {
            parent_tool_call_id, child_session_id, subagent_name, prompt_preview, ..
        } => {
            let step_label = format!("Subagent: {subagent_name}");
            if let Some(content) = accumulator.on_tool_start("subagent", &step_label) {
                ctx.flush_content(&content);
            }
            emit_or_log(
                ctx.emitter.as_ref(),
                "agent:subagent_start",
                serde_json::json!({
                    "thread_id": tid,
                    "parent_tool_call_id": parent_tool_call_id,
                    "child_session_id": child_session_id,
                    "subagent_name": subagent_name,
                    "prompt_preview": prompt_preview,
                }),
            );
            false
        }

        AgentEvent::SubagentEnd {
            parent_tool_call_id, child_session_id, success, summary, ..
        } => {
            emit_or_log(
                ctx.emitter.as_ref(),
                "agent:subagent_end",
                serde_json::json!({
                    "thread_id": tid,
                    "parent_tool_call_id": parent_tool_call_id,
                    "child_session_id": child_session_id,
                    "success": success,
                    "summary": summary,
                }),
            );
            false
        }
    }
}

// ── Approval handlers ─────────────────────────────────────────────────────────

/// Emits `agent:approval_needed` and blocks until the frontend resolves it via
/// `agent_approve_tool`.
pub struct TauriApprovalHandler {
    emitter: Arc<dyn EventEmitter>,
    pending: Arc<Mutex<HashMap<String, oneshot::Sender<bool>>>>,
    thread_id: String,
}

impl TauriApprovalHandler {
    pub fn new(emitter: Arc<dyn EventEmitter>, thread_id: String) -> Self {
        Self {
            emitter,
            pending: Arc::new(Mutex::new(HashMap::new())),
            thread_id,
        }
    }

    pub fn resolve(&self, tool_call_id: &str, approved: bool) {
        if let Some(tx) = self.pending.lock().unwrap().remove(tool_call_id) {
            let _ = tx.send(approved);
        }
    }
}

#[async_trait]
impl ApprovalHandler for TauriApprovalHandler {
    async fn request_approval(
        &self,
        tool_name: &str,
        tool_call_id: &str,
        args: &Value,
        args_summary: &str,
    ) -> ApprovalDecision {
        let (tx, rx) = oneshot::channel();
        self.pending.lock().unwrap().insert(tool_call_id.to_string(), tx);

        emit_or_log(
            self.emitter.as_ref(),
            "agent:approval_needed",
            serde_json::json!({
                "thread_id": self.thread_id,
                "tool_call_id": tool_call_id,
                "tool_name": tool_name,
                "description": args_summary,
                "args": args,
            }),
        );

        match rx.await {
            Ok(true) => ApprovalDecision::Approved,
            Ok(false) => ApprovalDecision::Denied { reason: Some("User denied".to_string()) },
            Err(_) => ApprovalDecision::Denied { reason: Some("Session cancelled".to_string()) },
        }
    }
}

/// Checks the permission config before asking the user; auto-approves tools the
/// config marks safe. `[SECURITY]`-tagged requests always go to the user.
pub struct PermissionAwareApprovalHandler {
    inner: Arc<TauriApprovalHandler>,
    config: PermissionConfig,
}

impl PermissionAwareApprovalHandler {
    pub fn new(inner: Arc<TauriApprovalHandler>, config: PermissionConfig) -> Self {
        Self { inner, config }
    }
}

#[async_trait]
impl ApprovalHandler for PermissionAwareApprovalHandler {
    async fn request_approval(
        &self,
        tool_name: &str,
        tool_call_id: &str,
        args: &Value,
        args_summary: &str,
    ) -> ApprovalDecision {
        let force_ask = args_summary.starts_with("[SECURITY]");
        if !force_ask && !crate::agent_bridge::permissions::needs_approval(&self.config, tool_name) {
            return ApprovalDecision::Approved;
        }
        self.inner.request_approval(tool_name, tool_call_id, args, args_summary).await
    }
}

/// Tags a subagent's approval requests with `(subagent:NAME)` while delegating
/// to the parent's permission-aware handler.
pub struct SubagentApprovalHandler {
    inner: Arc<dyn ApprovalHandler>,
    subagent_name: String,
}

#[async_trait]
impl ApprovalHandler for SubagentApprovalHandler {
    async fn request_approval(
        &self,
        tool_name: &str,
        tool_call_id: &str,
        args: &Value,
        args_summary: &str,
    ) -> ApprovalDecision {
        let tagged = format!("(subagent:{}) {}", self.subagent_name, args_summary);
        self.inner.request_approval(tool_name, tool_call_id, args, &tagged).await
    }
}

/// Produces tagged per-subagent approval handlers from the parent handler.
pub struct TauriApprovalHandlerFactory {
    inner: Arc<dyn ApprovalHandler>,
}

impl TauriApprovalHandlerFactory {
    pub fn new(inner: Arc<dyn ApprovalHandler>) -> Self {
        Self { inner }
    }
}

impl agent::subagents::ApprovalHandlerFactory for TauriApprovalHandlerFactory {
    fn for_subagent(&self, subagent_name: &str) -> Arc<dyn ApprovalHandler> {
        Arc::new(SubagentApprovalHandler {
            inner: Arc::clone(&self.inner),
            subagent_name: subagent_name.to_string(),
        })
    }
}
