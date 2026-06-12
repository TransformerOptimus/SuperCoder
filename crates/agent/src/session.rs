use std::collections::HashMap;
use std::path::PathBuf;
use std::sync::Arc;

use tokio::sync::{mpsc, RwLock};
use tokio_util::sync::CancellationToken;

use crate::agent::config::AgentConfig;
use crate::agent::loop_::AgentLoop;
use crate::approval::ApprovalHandler;
use crate::error::AgentError;
use crate::llm::client::{LlmClient, LlmProvider};
use crate::llm::types::ChatMessage;
use crate::persistence::MessagePersister;
use crate::tool::{ToolMode, ToolRegistry};
use crate::types::AgentEvent;

pub type SessionId = String;

/// Status of a session.
#[derive(Debug, Clone, serde::Serialize)]
pub enum SessionStatus {
    Active,
    Completed,
    Error(String),
}

/// Summary info for listing sessions.
#[derive(Debug, Clone, serde::Serialize)]
pub struct SessionSummary {
    pub id: SessionId,
    pub status: SessionStatus,
    pub mode: ToolMode,
    pub project_path: Option<PathBuf>,
    pub branch: Option<String>,
}

/// Internal handle for a running session.
struct SessionHandle {
    cancel_token: CancellationToken,
    status: SessionStatus,
    mode: ToolMode,
    project_path: Option<PathBuf>,
    branch: Option<String>,
}

/// Manages multiple concurrent agent sessions.
pub struct SessionManager {
    sessions: Arc<RwLock<HashMap<SessionId, SessionHandle>>>,
    persister: Arc<dyn MessagePersister>,
}

impl SessionManager {
    pub fn new(persister: Arc<dyn MessagePersister>) -> Self {
        Self {
            sessions: Arc::new(RwLock::new(HashMap::new())),
            persister,
        }
    }

    /// Start an ask-mode session. Returns a receiver for events.
    /// `initial_context` carries prior ask-mode messages for conversation continuity.
    /// Always treated as a resume (messages already exist in SQLite).
    /// `persister_override`: when `Some`, the session uses this per-call persister
    /// instead of the manager's default. Production callers always supply a
    /// per-session persister with the project_path frozen at construction (H1).
    #[allow(clippy::too_many_arguments)]
    pub async fn start_ask_session(
        &self,
        session_id: SessionId,
        config: AgentConfig,
        message: ChatMessage,
        initial_context: Option<Vec<ChatMessage>>,
        initial_token_count: Option<usize>,
        approval_handler: Option<Arc<dyn ApprovalHandler>>,
        persister_override: Option<Arc<dyn MessagePersister>>,
    ) -> Result<mpsc::Receiver<AgentEvent>, AgentError> {
        // Use the mode from config (Ask or Plan) rather than hardcoding Ask
        let mode = config.mode;
        self.start_session_inner(
            session_id,
            config,
            message,
            mode,
            None,
            None,
            initial_context,
            None,
            None, // ask mode persists under its own session_id (no override)
            initial_token_count,
            approval_handler,
            None, // no turn offset for ask mode
            true, // is_resume: ask mode loads its own prior messages, don't re-persist
            persister_override,
        )
        .await
    }

    /// Start a coding session. Returns a receiver for events.
    /// `initial_context` carries the sliding window of ask-mode messages (with their original roles).
    /// `persist_session_id` overrides the persistence key (defaults to session_id if None).
    /// `is_resume`: true when resuming an existing thread (don't re-persist context),
    ///              false when creating a new thread from ask mode (persist as completion_summary).
    /// `persister_override`: see `start_ask_session`.
    #[allow(clippy::too_many_arguments)]
    pub async fn start_coding_session(
        &self,
        session_id: SessionId,
        config: AgentConfig,
        message: ChatMessage,
        branch: Option<String>,
        initial_context: Option<Vec<ChatMessage>>,
        persist_session_id: Option<String>,
        initial_token_count: Option<usize>,
        approval_handler: Option<Arc<dyn ApprovalHandler>>,
        turn_offset: Option<u32>,
        is_resume: bool,
        persister_override: Option<Arc<dyn MessagePersister>>,
    ) -> Result<mpsc::Receiver<AgentEvent>, AgentError> {
        // The agent edits the project in place, so the session's project path IS
        // its working directory (no separate worktree).
        let project_path = Some(config.working_dir.clone());
        self.start_session_inner(
            session_id,
            config,
            message,
            ToolMode::Coding,
            project_path,
            branch,
            initial_context,
            None,
            persist_session_id,
            initial_token_count,
            approval_handler,
            turn_offset,
            is_resume,
            persister_override,
        )
        .await
    }

    #[allow(clippy::too_many_arguments)]
    async fn start_session_inner(
        &self,
        session_id: SessionId,
        config: AgentConfig,
        message: ChatMessage,
        mode: ToolMode,
        project_path: Option<PathBuf>,
        branch: Option<String>,
        initial_context: Option<Vec<ChatMessage>>,
        provider: Option<Box<dyn LlmProvider>>,
        persist_session_id: Option<String>,
        initial_token_count: Option<usize>,
        approval_handler: Option<Arc<dyn ApprovalHandler>>,
        turn_offset: Option<u32>,
        is_resume: bool,
        persister_override: Option<Arc<dyn MessagePersister>>,
    ) -> Result<mpsc::Receiver<AgentEvent>, AgentError> {
        let (event_tx, event_rx) = mpsc::channel(256);
        let error_tx = event_tx.clone(); // Clone for the watcher to emit errors
        let cancel_token = CancellationToken::new();
        let context_engine_arg = config.context_engine.as_ref().map(|engine| {
            let repo_path = config.context_engine_repo_path.clone().unwrap_or_else(|| {
                log::warn!("context_engine_repo_path not set; falling back to working_dir.");
                config.working_dir.clone()
            });
            (engine.clone(), repo_path)
        });
        let mut registry = ToolRegistry::for_mode(mode, context_engine_arg, config.skills.clone());
        if let (Some(sub_reg), Some(inherit)) = (config.subagents.clone(), config.subagent_inheritance.clone()) {
            registry.register_spawn_subagent(sub_reg, inherit);
        }
        let persister = persister_override.unwrap_or_else(|| Arc::clone(&self.persister));
        let persist_session_id = persist_session_id.unwrap_or_else(|| session_id.clone());

        let client: Box<dyn LlmProvider> = match provider {
            Some(p) => p,
            None => Box::new(LlmClient::new(config.llm.clone())),
        };

        let handle = {
            let cancel_token = cancel_token.clone();
            let sid = session_id.clone();
            tokio::spawn(async move {
                let mut agent_loop = AgentLoop::with_provider(
                    config,
                    client,
                    registry,
                    cancel_token,
                    event_tx,
                    sid.clone(),
                );
                agent_loop = agent_loop.with_persister(persister, persist_session_id);
                if let Some(h) = approval_handler {
                    agent_loop = agent_loop.with_approval_handler(h);
                }
                // Seed prior context if provided
                if let Some(ref context_msgs) = initial_context {
                    log::info!(
                        "[Session {}] Seeding {} prior context messages (is_resume={})",
                        sid, context_msgs.len(), is_resume
                    );
                }
                if let Some(context_msgs) = initial_context {
                    if is_resume {
                        // Same-scope resume: messages already exist in SQLite, just load into buffer
                        agent_loop = agent_loop.with_resumed_context(context_msgs);
                    } else {
                        // Cross-scope transfer (ask→coding): persist copies as completion_summary
                        agent_loop = agent_loop.with_initial_context(context_msgs);
                    }
                }
                // Seed persisted token count so compaction fires correctly on first turn
                if let Some(tokens) = initial_token_count {
                    log::info!("[Session {}] Seeding persisted token count: {}", sid, tokens);
                    agent_loop = agent_loop.with_initial_token_count(tokens);
                }
                // Seed turn offset for globally unique checkpoint numbering
                if let Some(offset) = turn_offset {
                    if offset > 0 {
                        log::info!("[Session {}] Seeding turn offset: {}", sid, offset);
                        agent_loop = agent_loop.with_turn_offset(offset);
                    }
                }
                log::info!("[Session {}] Starting agent loop with {} total messages", sid, agent_loop.message_count());
                agent_loop.run(message).await
            })
        };

        // Store session handle with Active status
        let session_handle = SessionHandle {
            cancel_token,
            status: SessionStatus::Active,
            mode,
            project_path,
            branch,
        };

        self.sessions
            .write()
            .await
            .insert(session_id.clone(), session_handle);

        // Spawn a watcher that updates status when the task completes
        // and emits error events so the frontend can exit the "thinking" state.
        {
            let sessions = Arc::clone(&self.sessions);
            let sid = session_id.clone();
            tokio::spawn(async move {
                let final_status = match handle.await {
                    Ok(Ok(ref result)) => {
                        log::info!("[Session {sid}] Completed: {result:?}");
                        SessionStatus::Completed
                    }
                    Ok(Err(AgentError::Cancelled)) => {
                        log::info!("[Session {sid}] Cancelled");
                        SessionStatus::Error("Cancelled".into())
                    }
                    Ok(Err(ref e)) => {
                        log::error!("[Session {sid}] Failed: {e}");
                        // Emit error event so the frontend exits "thinking" state
                        log::info!("[Session {sid}] Sending Error+Done events via error_tx...");
                        match error_tx.send(AgentEvent::Error {
                            session_id: sid.clone(),
                            message: e.to_string(),
                            retrying: false,
                        }).await {
                            Ok(()) => log::info!("[Session {sid}] Error event sent successfully"),
                            Err(e) => log::error!("[Session {sid}] Failed to send Error event: {e}"),
                        }
                        match error_tx.send(AgentEvent::Done {
                            session_id: sid.clone(),
                            summary: None,
                        }).await {
                            Ok(()) => log::info!("[Session {sid}] Done event sent successfully"),
                            Err(e) => log::error!("[Session {sid}] Failed to send Done event: {e}"),
                        }
                        SessionStatus::Error(e.to_string())
                    }
                    Err(ref e) => {
                        log::error!("[Session {sid}] Task panicked: {e}");
                        let _ = error_tx.send(AgentEvent::Error {
                            session_id: sid.clone(),
                            message: format!("Agent task panicked: {e}"),
                            retrying: false,
                        }).await;
                        let _ = error_tx.send(AgentEvent::Done {
                            session_id: sid.clone(),
                            summary: None,
                        }).await;
                        SessionStatus::Error(format!("Task panicked: {e}"))
                    }
                };
                let mut map = sessions.write().await;
                if let Some(h) = map.get_mut(&sid) {
                    h.status = final_status;
                }
            });
        }

        Ok(event_rx)
    }

    /// Start a session with a custom LLM provider (for testing with mocks).
    #[cfg(test)]
    pub async fn start_ask_session_with_provider(
        &self,
        session_id: SessionId,
        config: AgentConfig,
        message: ChatMessage,
        provider: Box<dyn LlmProvider>,
        initial_context: Option<Vec<ChatMessage>>,
    ) -> Result<mpsc::Receiver<AgentEvent>, AgentError> {
        self.start_session_inner(
            session_id,
            config,
            message,
            ToolMode::Ask,
            None,
            None,
            initial_context,
            Some(provider),
            None,
            None,
            None,
            None, // no turn offset for test ask sessions
            true, // is_resume: test ask sessions treat context as already-persisted
            None, // tests use the manager's default persister (MockPersister)
        )
        .await
    }

    /// Start a coding session with a custom LLM provider (for testing with mocks).
    #[cfg(test)]
    pub async fn start_coding_session_with_provider(
        &self,
        session_id: SessionId,
        config: AgentConfig,
        message: ChatMessage,
        initial_context: Option<Vec<ChatMessage>>,
        provider: Box<dyn LlmProvider>,
        persist_session_id: Option<String>,
    ) -> Result<mpsc::Receiver<AgentEvent>, AgentError> {
        self.start_session_inner(
            session_id,
            config,
            message,
            ToolMode::Coding,
            None,
            None,
            initial_context,
            Some(provider),
            persist_session_id,
            None,
            None,
            None, // no turn offset for test coding sessions
            false, // is_resume=false: test coding sessions use with_initial_context (cross-scope default)
            None,  // tests use the manager's default persister (MockPersister)
        )
        .await
    }

    /// Cancel a running session.
    pub async fn cancel_session(&self, session_id: &str) {
        let sessions = self.sessions.read().await;
        if let Some(handle) = sessions.get(session_id) {
            handle.cancel_token.cancel();
            log::info!("[session {session_id}] cancel_token fired");
            // Status will be updated by the watcher when the task exits
        } else {
            log::debug!("[session {session_id}] cancel requested but session not found");
        }
    }

    /// Get the status of a session.
    pub async fn session_status(&self, session_id: &str) -> Option<SessionStatus> {
        let sessions = self.sessions.read().await;
        sessions.get(session_id).map(|h| h.status.clone())
    }

    /// List all sessions.
    pub async fn list_sessions(&self) -> Vec<SessionSummary> {
        let sessions = self.sessions.read().await;
        sessions
            .iter()
            .map(|(id, handle)| SessionSummary {
                id: id.clone(),
                status: handle.status.clone(),
                mode: handle.mode,
                project_path: handle.project_path.clone(),
                branch: handle.branch.clone(),
            })
            .collect()
    }

    /// Remove a session from the manager, cancelling it first if still running.
    pub async fn remove_session(&self, session_id: &str) {
        let mut sessions = self.sessions.write().await;
        if let Some(handle) = sessions.get(session_id) {
            handle.cancel_token.cancel();
        }
        sessions.remove(session_id);
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::agent::config::RetryConfig;
    use crate::llm::LlmClientConfig;
    use crate::llm::types::ChatMessage;
    use crate::persistence::MockPersister;
    use crate::test_util::{MockLlm, text_response};

    fn test_config() -> AgentConfig {
        let mut config = AgentConfig::new(
            LlmClientConfig {
                provider: crate::llm::Provider::OpenAI,
                base_url: "http://unused".into(),
                model: "unused".into(),
                api_key: String::new(),
                temperature: None,
                max_completion_tokens: None,
                extra_headers: vec![],
                thinking: None,
                disable_cache_control: false,
                policy: Default::default(),
            },
            PathBuf::from("/tmp"),
        );
        config.retry_config = RetryConfig {
            max_retries: 0,
            initial_delay: std::time::Duration::from_millis(1),
            multiplier: 1.0,
            max_delay: std::time::Duration::from_millis(10),
        };
        config
    }

    #[tokio::test]
    async fn test_start_ask_session_emits_events() {
        let persister = Arc::new(MockPersister::new());
        let manager = SessionManager::new(persister);
        let mock = MockLlm::new(vec![Ok(text_response("Hello!"))]);

        let mut rx = manager
            .start_ask_session_with_provider(
                "session-1".into(),
                test_config(),
                ChatMessage::user("Hi"),
                Box::new(mock),
                None,
            )
            .await
            .unwrap();

        // Collect events
        let mut events = Vec::new();
        while let Some(event) = rx.recv().await {
            events.push(event);
        }

        assert!(events.iter().any(|e| matches!(e, AgentEvent::Done { .. })));
    }

    #[tokio::test]
    async fn test_cancel_session() {
        let persister = Arc::new(MockPersister::new());
        let manager = SessionManager::new(persister);

        let fast_mock = MockLlm::new(vec![Ok(text_response("done"))]);
        let _rx = manager
            .start_ask_session_with_provider(
                "session-cancel".into(),
                test_config(),
                ChatMessage::user("test"),
                Box::new(fast_mock),
                None,
            )
            .await
            .unwrap();

        manager.cancel_session("session-cancel").await;

        // Yield to let the watcher task update the status
        tokio::task::yield_now().await;
        tokio::task::yield_now().await;

        let status = manager.session_status("session-cancel").await;
        assert!(matches!(status, Some(SessionStatus::Error(_)) | Some(SessionStatus::Completed)));
    }

    #[tokio::test]
    async fn test_list_sessions() {
        let persister = Arc::new(MockPersister::new());
        let manager = SessionManager::new(persister);

        let mock1 = MockLlm::new(vec![Ok(text_response("a"))]);
        let mock2 = MockLlm::new(vec![Ok(text_response("b"))]);

        let _rx1 = manager
            .start_ask_session_with_provider(
                "s1".into(),
                test_config(),
                ChatMessage::user("test1"),
                Box::new(mock1),
                None,
            )
            .await
            .unwrap();

        let _rx2 = manager
            .start_ask_session_with_provider(
                "s2".into(),
                test_config(),
                ChatMessage::user("test2"),
                Box::new(mock2),
                None,
            )
            .await
            .unwrap();

        let list = manager.list_sessions().await;
        assert_eq!(list.len(), 2);
    }

    #[tokio::test]
    async fn test_session_not_found_error() {
        let persister = Arc::new(MockPersister::new());
        let manager = SessionManager::new(persister);

        let status = manager.session_status("nonexistent").await;
        assert!(status.is_none());
    }

    #[tokio::test]
    async fn test_remove_session() {
        let persister = Arc::new(MockPersister::new());
        let manager = SessionManager::new(persister);

        let mock = MockLlm::new(vec![Ok(text_response("done"))]);
        let _rx = manager
            .start_ask_session_with_provider(
                "to-remove".into(),
                test_config(),
                ChatMessage::user("test"),
                Box::new(mock),
                None,
            )
            .await
            .unwrap();

        manager.remove_session("to-remove").await;
        assert!(manager.session_status("to-remove").await.is_none());
    }

    #[tokio::test]
    async fn test_session_status_updates_to_completed() {
        let persister = Arc::new(MockPersister::new());
        let manager = SessionManager::new(persister);

        let mock = MockLlm::new(vec![Ok(text_response("done"))]);
        let mut rx = manager
            .start_ask_session_with_provider(
                "complete-test".into(),
                test_config(),
                ChatMessage::user("test"),
                Box::new(mock),
                None,
            )
            .await
            .unwrap();

        // Drain all events — the session completes when the channel closes
        while rx.recv().await.is_some() {}

        // Give the watcher task a moment to update the status
        tokio::task::yield_now().await;
        tokio::task::yield_now().await;

        let status = manager.session_status("complete-test").await;
        assert!(
            matches!(status, Some(SessionStatus::Completed)),
            "Expected Completed, got: {:?}",
            status
        );
    }

    #[tokio::test]
    async fn test_start_coding_session_with_initial_context() {
        let persister = Arc::new(MockPersister::new());
        let manager = SessionManager::new(Arc::clone(&persister) as Arc<dyn MessagePersister>);

        let mock = MockLlm::new(vec![Ok(text_response("I see prior context!"))]);
        let context = vec![
            ChatMessage::user("What does main.rs do?"),
            ChatMessage::assistant(Some("It starts the server.".into()), None, None),
        ];

        let mut rx = manager
            .start_coding_session_with_provider(
                "coding-ctx".into(),
                test_config(),
                ChatMessage::user("Now fix the bug in main.rs"),
                Some(context),
                Box::new(mock),
                None,
            )
            .await
            .unwrap();

        // Drain events
        while rx.recv().await.is_some() {}

        // Give watcher + persistence worker time to finish
        tokio::task::yield_now().await;
        tokio::task::yield_now().await;
        tokio::time::sleep(std::time::Duration::from_millis(50)).await;

        // Verify session completed
        let status = manager.session_status("coding-ctx").await;
        assert!(
            matches!(status, Some(SessionStatus::Completed)),
            "Expected Completed, got: {:?}",
            status
        );

        // Verify initial context was persisted (user + assistant from sliding window)
        let msgs = persister.messages();
        let contents: Vec<&str> = msgs.iter().map(|(_, m)| m.content.as_str()).collect();
        assert!(
            contents.contains(&"What does main.rs do?"),
            "Initial context user message should be persisted. Got: {:?}",
            contents
        );
        assert!(
            contents.contains(&"It starts the server."),
            "Initial context assistant message should be persisted. Got: {:?}",
            contents
        );
    }

    #[tokio::test]
    async fn test_persist_session_id_routes_correctly() {
        let persister = Arc::new(MockPersister::new());
        let manager = SessionManager::new(Arc::clone(&persister) as Arc<dyn MessagePersister>);

        let mock = MockLlm::new(vec![Ok(text_response("Done!"))]);

        // Start a coding session with a custom persist_session_id
        let mut rx = manager
            .start_coding_session_with_provider(
                "session-uuid-123".into(),
                test_config(),
                ChatMessage::user("Fix the bug"),
                None,
                Box::new(mock),
                Some("real-session-id".into()), // This should be the persistence key
            )
            .await
            .unwrap();

        // Drain events
        while rx.recv().await.is_some() {}

        // Give persistence worker time to finish
        tokio::time::sleep(std::time::Duration::from_millis(50)).await;

        // Messages should be stored under "real-session-id", not "session-uuid-123"
        let msgs = persister.messages();
        let session_ids: Vec<&str> = msgs.iter().map(|(sid, _)| sid.as_str()).collect();

        assert!(
            !session_ids.is_empty(),
            "Should have persisted messages"
        );
        assert!(
            session_ids.iter().all(|sid| *sid == "real-session-id"),
            "All messages should be stored under 'real-session-id', got: {:?}",
            session_ids
        );
        assert!(
            !session_ids.iter().any(|sid| *sid == "session-uuid-123"),
            "No messages should be stored under the session UUID"
        );
    }

    #[tokio::test]
    async fn test_persist_session_id_defaults_to_session_id() {
        let persister = Arc::new(MockPersister::new());
        let manager = SessionManager::new(Arc::clone(&persister) as Arc<dyn MessagePersister>);

        let mock = MockLlm::new(vec![Ok(text_response("Done!"))]);

        // Start without persist_session_id (None) — should use session_id
        let mut rx = manager
            .start_coding_session_with_provider(
                "my-session".into(),
                test_config(),
                ChatMessage::user("Hello"),
                None,
                Box::new(mock),
                None, // No custom persist_session_id
            )
            .await
            .unwrap();

        while rx.recv().await.is_some() {}
        tokio::time::sleep(std::time::Duration::from_millis(50)).await;

        let msgs = persister.messages();
        let session_ids: Vec<&str> = msgs.iter().map(|(sid, _)| sid.as_str()).collect();

        assert!(
            session_ids.iter().all(|sid| *sid == "my-session"),
            "Messages should default to session_id for persistence, got: {:?}",
            session_ids
        );
    }
}
