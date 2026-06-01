pub mod config;
pub mod loop_;
pub mod prompt;
pub mod compaction;
pub mod worktree;
pub mod model_profile;

use std::sync::Arc;
use tokio::sync::mpsc;
use tokio::task::JoinHandle;
use tokio_util::sync::CancellationToken;

use crate::approval::ApprovalHandler;
use crate::error::AgentError;
use crate::llm::types::ChatMessage;
use crate::persistence::MessagePersister;
use crate::tool::ToolRegistry;
use crate::types::{AgentEvent, AgentResult};
pub use config::AgentConfig;
pub use loop_::AgentLoop;

/// Handle returned by `spawn_agent` with everything needed to interact with a running session.
pub struct SpawnedAgent {
    /// Task handle — await for the final AgentResult or error.
    pub handle: JoinHandle<Result<AgentResult, AgentError>>,
    /// Receiver for streaming UI events (TextDelta, ToolStart, ToolEnd, Done, Error).
    pub event_rx: mpsc::Receiver<AgentEvent>,
    /// Cancel token — call `.cancel()` to stop the agent.
    pub cancel_token: CancellationToken,
    /// Session ID embedded in every emitted event, for UI correlation.
    pub session_id: String,
}

/// Spawn an agent loop as a background tokio task.
pub fn spawn_agent(
    mut config: AgentConfig,
    user_message: ChatMessage,
    persister: Option<Arc<dyn MessagePersister>>,
    persist_session_id: Option<String>,
    approval_handler: Option<Arc<dyn ApprovalHandler>>,
) -> SpawnedAgent {
    let (event_tx, event_rx) = mpsc::channel(256);
    let cancel_token = CancellationToken::new();
    let session_id = uuid::Uuid::new_v4().to_string();

    let context_engine_arg = config.context_engine.as_ref().map(|engine| {
        let repo_path = config.context_engine_repo_path.clone().unwrap_or_else(|| {
            log::warn!(
                "context_engine_repo_path not set; falling back to working_dir. \
                 Worktree overlay will be disabled."
            );
            config.working_dir.clone()
        });
        (engine.clone(), repo_path)
    });
    let skills_arg = config.skills.clone();
    let mut registry = ToolRegistry::for_mode(config.mode, context_engine_arg, skills_arg);
    if let (Some(sub_reg), Some(inherit)) = (config.subagents.clone(), config.subagent_inheritance.clone()) {
        registry.register_spawn_subagent(sub_reg, inherit);
    }

    // Set default system prompt if none provided — uses mode from config
    if config.system_prompt.is_none() {
        config.system_prompt = Some(prompt::build_system_prompt(
            config.mode,
            &config.working_dir,
            None,
            None,
            config.skills.as_deref(),
            config.subagents.as_deref(),
        ));
    }

    let handle = {
        let cancel_token = cancel_token.clone();
        let session_id = session_id.clone();
        tokio::spawn(async move {
            let persist_id = persist_session_id.unwrap_or_else(|| session_id.clone());
            let mut agent_loop = AgentLoop::new(config, registry, cancel_token, event_tx, session_id);
            if let Some(p) = persister {
                agent_loop = agent_loop.with_persister(p, persist_id);
            }
            if let Some(h) = approval_handler {
                agent_loop = agent_loop.with_approval_handler(h);
            }
            agent_loop.run(user_message).await
        })
    };

    SpawnedAgent {
        handle,
        event_rx,
        cancel_token,
        session_id,
    }
}
