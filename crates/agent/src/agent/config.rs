use std::path::PathBuf;
use std::sync::Arc;
use std::time::Duration;

use crate::context_engine::ContextEngineApi;
use crate::llm::LlmClientConfig;
use crate::skills::SkillRegistry;
use crate::subagents::{SubagentInheritance, SubagentRegistry};
use crate::tool::ToolMode;
use super::model_profile::ModelProfile;
use super::prompt::SystemBlock;

/// Configuration for the agent loop.
pub struct AgentConfig {
    /// LLM client configuration
    pub llm: LlmClientConfig,
    /// Working directory for tool execution
    pub working_dir: PathBuf,
    /// Agent mode — determines tool set and system prompt (default Coding)
    pub mode: ToolMode,
    /// Maximum loop iterations before stopping (default 100)
    pub max_iterations: u32,
    /// System prompt prepended to conversation. A vector of ordered blocks so
    /// individual segments can carry `cache_control` markers for prompt caching.
    pub system_prompt: Option<Vec<SystemBlock>>,
    /// Retry configuration for LLM API calls
    pub retry_config: RetryConfig,
    /// Compaction configuration for context window management
    pub compaction_config: CompactionConfig,
    /// Optional dedicated LLM client config for compaction summarization.
    /// When set, summarization uses this (typically a cheaper model) instead of `llm`.
    /// When None, falls back to the main `llm` client.
    pub compaction_llm: Option<LlmClientConfig>,
    /// Optional context engine. When set, codebase_search and codebase_graph are registered.
    pub context_engine: Option<Arc<dyn ContextEngineApi>>,
    /// Canonical repo path the context-engine index was built from.
    /// Required when context_engine is Some; defaults to working_dir if unset.
    pub context_engine_repo_path: Option<PathBuf>,
    /// Optional skill registry. When set, the skills list is injected into the
    /// system prompt and the `skill` tool is registered in all modes.
    pub skills: Option<Arc<SkillRegistry>>,
    /// Optional subagent registry. When set, the subagent list is injected
    /// into the system prompt and `spawn_subagent` is registered in all modes.
    pub subagents: Option<Arc<SubagentRegistry>>,
    /// Bundle supplied by the Tauri layer so `SpawnSubagentTool` can build
    /// child loops (LLM client config, persister factory, approval handler
    /// factory, write-lock registry, etc.). Required at parent-turn
    /// registration time when `subagents` is Some.
    pub subagent_inheritance: Option<Arc<SubagentInheritance>>,
    /// App-managed directory for file-snapshot checkpoints, stored OUTSIDE the
    /// project. When set, file-mutating tools back up a file's prior contents
    /// before editing it (keyed by `(session_id, turn)`), enabling per-turn undo.
    /// `None` disables checkpoint capture (bench/tests).
    pub checkpoint_dir: Option<PathBuf>,
}

impl AgentConfig {
    pub fn new(llm: LlmClientConfig, working_dir: PathBuf) -> Self {
        Self {
            llm,
            working_dir,
            mode: ToolMode::Coding,
            max_iterations: 100,
            system_prompt: None,
            retry_config: RetryConfig::default(),
            compaction_config: CompactionConfig::default(),
            compaction_llm: None,
            context_engine: None,
            context_engine_repo_path: None,
            skills: None,
            subagents: None,
            subagent_inheritance: None,
            checkpoint_dir: None,
        }
    }

    /// Set compaction context_limit from a model profile.
    /// Call this after construction to get the correct context window for the model.
    pub fn with_model_profile(mut self, profile: &ModelProfile) -> Self {
        self.compaction_config.context_limit = profile.context_window;
        self
    }
}

/// Configuration for exponential backoff retries on LLM API errors.
#[derive(Debug, Clone)]
pub struct RetryConfig {
    /// Maximum number of retries (default 3)
    pub max_retries: u32,
    /// Initial backoff delay (default 1s)
    pub initial_delay: Duration,
    /// Backoff multiplier (default 2.0)
    pub multiplier: f64,
    /// Maximum backoff delay cap (default 30s)
    pub max_delay: Duration,
}

/// Configuration for context window compaction.
#[derive(Debug, Clone)]
pub struct CompactionConfig {
    /// Maximum context size in estimated tokens (default 128_000)
    pub context_limit: usize,
    /// Threshold percentage at which to trigger compaction (default 0.80)
    pub threshold_pct: f64,
    /// Number of recent messages to keep uncompacted (default 10)
    pub keep_recent_messages: usize,
    /// Maximum number of messages in the array before forcing compaction (default 10_000).
    /// OpenAI's limit is 16,384 — we compact well before that.
    pub max_messages: usize,
}

impl Default for CompactionConfig {
    fn default() -> Self {
        Self {
            context_limit: 128_000,
            threshold_pct: 0.80,
            keep_recent_messages: 10,
            max_messages: 10_000,
        }
    }
}

impl Default for RetryConfig {
    fn default() -> Self {
        Self {
            max_retries: 3,
            initial_delay: Duration::from_secs(1),
            multiplier: 2.0,
            max_delay: Duration::from_secs(30),
        }
    }
}
