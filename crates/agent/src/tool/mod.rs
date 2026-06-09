pub mod schema;
pub mod read;
pub mod write;
pub mod bash;
pub mod edit;
pub mod glob;
pub mod grep;
pub mod git_tool;
pub mod pr_tool;
pub mod ask_user;
pub mod todo_write;
pub mod apply_patch;
pub mod save_plan;
pub mod edit_plan;
pub mod codebase_search;
pub mod codebase_graph;

use std::collections::HashMap;
use std::path::PathBuf;
use std::sync::Arc;

use crate::context_engine::ContextEngineApi;
use crate::skills::{SkillRegistry, SkillTool};
use crate::subagents::{SpawnSubagentTool, SubagentInheritance, SubagentRegistry};

use async_trait::async_trait;
use serde_json::Value;
use tokio::sync::mpsc;
use tokio_util::sync::CancellationToken;

use crate::error::ToolError;
use crate::llm::types::ToolDefinition;
use crate::types::AgentEvent;

/// Result of a tool execution.
#[derive(Debug)]
pub struct ToolResult {
    pub output: String,
    pub is_error: bool,
    /// Optional yield data — when set, the agent loop will yield control
    /// (e.g., save_plan / ask_user set this to signal a yield to the host).
    pub yield_data: Option<serde_json::Value>,
    /// Files modified by this tool execution (for write/edit/apply_patch tools).
    pub modified_files: Vec<String>,
}

impl ToolResult {
    /// Create a successful result with no yield.
    pub fn success(output: impl Into<String>) -> Self {
        Self {
            output: output.into(),
            is_error: false,
            yield_data: None,
            modified_files: Vec::new(),
        }
    }

    /// Create an error result with no yield.
    pub fn error(output: impl Into<String>) -> Self {
        Self {
            output: output.into(),
            is_error: true,
            yield_data: None,
            modified_files: Vec::new(),
        }
    }
}

/// Context passed to every tool execution.
pub struct ToolContext {
    pub working_dir: PathBuf,
    pub cancel_token: CancellationToken,
    pub event_tx: mpsc::Sender<AgentEvent>,
    pub session_id: String,
    pub tool_call_id: String,
    /// App-managed directory for file-snapshot checkpoints (outside the project).
    /// `None` disables capture (bench/tests). File-mutating tools call
    /// `git_ops::backup_file` here before mutating, keyed to `(session_id, checkpoint_turn)`.
    pub checkpoint_dir: Option<PathBuf>,
    /// Current turn number, used as the checkpoint key alongside `session_id`.
    pub checkpoint_turn: u32,
}

impl ToolContext {
    /// Best-effort: back up `path`'s prior contents BEFORE a mutating tool edits it,
    /// so the turn can be undone. No-op when checkpointing is disabled. Failures are
    /// logged, not propagated — a missing backup must never block the edit itself.
    pub(crate) async fn checkpoint(&self, path: &std::path::Path) {
        if let Some(dir) = &self.checkpoint_dir {
            if let Err(e) =
                git_ops::backup_file(dir, &self.session_id, self.checkpoint_turn, path).await
            {
                log::warn!("checkpoint backup_file failed for {}: {e}", path.display());
            }
        }
    }
}

#[cfg(test)]
impl ToolContext {
    pub(crate) fn test_context(dir: &std::path::Path) -> Self {
        let (tx, _rx) = mpsc::channel(32);
        Self {
            working_dir: dir.to_path_buf(),
            cancel_token: CancellationToken::new(),
            event_tx: tx,
            session_id: "test".into(),
            tool_call_id: "tc_1".into(),
            checkpoint_dir: None,
            checkpoint_turn: 0,
        }
    }
}

/// Trait that all tools must implement.
#[async_trait]
pub trait Tool: Send + Sync {
    fn name(&self) -> &str;
    fn description(&self) -> &str;
    fn parameters_schema(&self) -> Value;
    async fn execute(&self, args: Value, ctx: &ToolContext) -> Result<ToolResult, ToolError>;
}

/// Mode that determines which tools are available.
#[derive(Debug, Clone, Copy, PartialEq, Eq, serde::Serialize)]
pub enum ToolMode {
    /// Ask mode: read-only tools + ask_user.
    Ask,
    /// Coding mode: full tool suite for making changes.
    Coding,
    /// Plan mode: read-only tools + ask_user + save_plan + edit_plan.
    Plan,
}

/// Registry of available tools.
pub struct ToolRegistry {
    tools: HashMap<String, Arc<dyn Tool>>,
}

impl Default for ToolRegistry {
    fn default() -> Self {
        Self::new()
    }
}

impl ToolRegistry {
    pub fn new() -> Self {
        Self {
            tools: HashMap::new(),
        }
    }

    /// Create a registry pre-loaded with all default tools (coding mode, no context engine).
    pub fn with_defaults() -> Self {
        Self::for_mode(ToolMode::Coding, None, None)
    }

    /// Create a registry for a specific mode.
    /// When `context_engine` is provided, `codebase_search` and `codebase_graph` tools
    /// are registered in all modes.
    /// When `skills` is provided, the `skill` tool is registered in all modes.
    pub fn for_mode(
        mode: ToolMode,
        context_engine: Option<(Arc<dyn ContextEngineApi>, PathBuf)>,
        skills: Option<Arc<SkillRegistry>>,
    ) -> Self {
        let mut registry = Self::new();
        match mode {
            ToolMode::Ask => {
                registry.register(Arc::new(read::ReadTool));
                registry.register(Arc::new(glob::GlobTool));
                registry.register(Arc::new(grep::GrepTool));
                registry.register(Arc::new(ask_user::AskUserTool));
            }
            ToolMode::Coding => {
                registry.register(Arc::new(read::ReadTool));
                registry.register(Arc::new(write::WriteTool));
                registry.register(Arc::new(bash::BashTool));
                registry.register(Arc::new(edit::EditTool));
                registry.register(Arc::new(glob::GlobTool));
                registry.register(Arc::new(grep::GrepTool));
                registry.register(Arc::new(git_tool::GitTool));
                registry.register(Arc::new(pr_tool::PrTool));
                registry.register(Arc::new(todo_write::TodoWriteTool));
                registry.register(Arc::new(apply_patch::ApplyPatchTool));
            }
            ToolMode::Plan => {
                registry.register(Arc::new(read::ReadTool));
                registry.register(Arc::new(glob::GlobTool));
                registry.register(Arc::new(grep::GrepTool));
                registry.register(Arc::new(ask_user::AskUserTool));
                registry.register(Arc::new(save_plan::SavePlanTool));
                registry.register(Arc::new(edit_plan::EditPlanTool));
            }
        }
        // Register context engine tools in ALL modes if available
        if let Some((engine, repo_path)) = context_engine {
            registry.register(Arc::new(
                codebase_search::CodebaseSearchTool::new(engine.clone(), repo_path),
            ));
            registry.register(Arc::new(
                codebase_graph::CodebaseGraphTool::new(engine),
            ));
        }
        // Register skill tool in ALL modes if a registry is provided
        if let Some(skills_registry) = skills {
            log::info!(
                "[skills] ToolRegistry::for_mode: registering `skill` tool in {:?} mode with {} skill(s) available",
                mode,
                skills_registry.len()
            );
            registry.register(Arc::new(SkillTool::new(skills_registry)));
        }
        registry
    }

    pub fn register(&mut self, tool: Arc<dyn Tool>) {
        self.tools.insert(tool.name().to_string(), tool);
    }

    /// Register `spawn_subagent` with the given registry + inheritance bundle.
    /// Called post-`for_mode` by the Tauri layer (parent-turn construction) —
    /// child loops never call this, which enforces depth-1.
    pub fn register_spawn_subagent(
        &mut self,
        registry: Arc<SubagentRegistry>,
        inherit: Arc<SubagentInheritance>,
    ) {
        if registry.is_empty() {
            log::info!("[subagents] spawn_subagent NOT registered (empty registry)");
            return;
        }
        log::info!(
            "[subagents] registering spawn_subagent: {} subagent(s) available, names={:?}, persister={}, approval_factory={}, parent_session_id={:?}",
            registry.len(),
            registry.names(),
            inherit.persister.is_some(),
            inherit.approval_handler_factory.is_some(),
            inherit.parent_session_id,
        );
        self.register(Arc::new(SpawnSubagentTool::new(registry, inherit)));
    }

    pub fn get(&self, name: &str) -> Option<Arc<dyn Tool>> {
        self.tools.get(name).cloned()
    }

    /// Keep only tools whose name matches the predicate. Used by subagents
    /// to apply `allowed-tools` filters post-construction.
    pub fn retain<F: Fn(&str) -> bool>(&mut self, pred: F) {
        self.tools.retain(|name, _| pred(name));
    }

    /// List registered tool names.
    pub fn names(&self) -> Vec<String> {
        let mut out: Vec<String> = self.tools.keys().cloned().collect();
        out.sort();
        out
    }

    /// Return OpenAI-format tool definitions for all registered tools.
    pub fn tool_definitions(&self) -> Vec<ToolDefinition> {
        let mut defs: Vec<_> = self
            .tools
            .values()
            .map(|tool| ToolDefinition {
                type_: "function".to_string(),
                function: crate::llm::types::FunctionDefinition {
                    name: tool.name().to_string(),
                    description: Some(tool.description().to_string()),
                    parameters: Some(tool.parameters_schema()),
                },
                cache_control: None,
            })
            .collect();
        // Sort for deterministic ordering
        defs.sort_by(|a, b| a.function.name.cmp(&b.function.name));
        defs
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    // Probe: measure serialized size of system prompt + tool definitions per mode.
    // Run with: cargo test -p agent probe_cache_sizes -- --nocapture --ignored
    #[test]
    #[ignore]
    fn probe_cache_sizes() {
        use crate::agent::prompt::build_system_prompt;
        use std::path::PathBuf;
        let wd = PathBuf::from(".");
        for mode in [ToolMode::Ask, ToolMode::Coding, ToolMode::Plan] {
            let sys_blocks = build_system_prompt(mode, &wd, Some("main"), None, None, None, false);
            let sys: String = sys_blocks.iter().map(|b| b.text.as_str()).collect::<Vec<_>>().join("\n");
            let reg = ToolRegistry::for_mode(mode, None, None);
            let tools = reg.tool_definitions();
            let tools_json = serde_json::to_string(&tools).unwrap();
            let sys_chars = sys.chars().count();
            let tools_chars = tools_json.chars().count();
            let total_chars = sys_chars + tools_chars;
            eprintln!(
                "mode={:?}  sys={}c (~{}tok)  tools={}c (~{}tok, n={})  TOTAL={}c (~{}tok)",
                mode,
                sys_chars, sys_chars / 4,
                tools_chars, tools_chars / 4, tools.len(),
                total_chars, total_chars / 4,
            );
        }
    }

    // Probe: confirm tool_definitions() is byte-stable across calls (required for caching).
    #[test]
    #[ignore]
    fn probe_tool_definitions_byte_stable() {
        for mode in [ToolMode::Ask, ToolMode::Coding, ToolMode::Plan] {
            let reg = ToolRegistry::for_mode(mode, None, None);
            let a = serde_json::to_string(&reg.tool_definitions()).unwrap();
            let b = serde_json::to_string(&reg.tool_definitions()).unwrap();
            eprintln!("mode={:?}  bytes_equal={}  len={}", mode, a == b, a.len());
            assert_eq!(a, b, "tool definitions not byte-stable in {:?} mode", mode);
        }
    }

    #[test]
    fn test_with_defaults_registers_all_tools() {
        let reg = ToolRegistry::with_defaults();
        let names: Vec<String> = reg
            .tool_definitions()
            .iter()
            .map(|d| d.function.name.clone())
            .collect();
        // Sorted alphabetically by tool_definitions()
        assert_eq!(names, vec!["apply_patch", "bash", "create_pr", "edit", "git", "glob", "grep", "read", "todo_write", "write"]);
    }

    #[test]
    fn test_tool_definitions_sorted() {
        let mut reg = ToolRegistry::new();
        reg.register(Arc::new(write::WriteTool));
        reg.register(Arc::new(read::ReadTool));
        reg.register(Arc::new(bash::BashTool));
        let defs = reg.tool_definitions();
        let names: Vec<&str> = defs.iter().map(|d| d.function.name.as_str()).collect();
        assert_eq!(names, vec!["bash", "read", "write"]);
    }

    #[test]
    fn test_get_missing_tool_returns_none() {
        let reg = ToolRegistry::new();
        assert!(reg.get("nonexistent").is_none());
    }

    #[test]
    fn test_ask_mode_tools() {
        let reg = ToolRegistry::for_mode(ToolMode::Ask, None, None);
        let mut names: Vec<String> = reg
            .tool_definitions()
            .iter()
            .map(|d| d.function.name.clone())
            .collect();
        names.sort();
        assert_eq!(names, vec!["ask_user", "glob", "grep", "read"]);
    }

    #[test]
    fn test_coding_mode_tools() {
        let reg = ToolRegistry::for_mode(ToolMode::Coding, None, None);
        let mut names: Vec<String> = reg
            .tool_definitions()
            .iter()
            .map(|d| d.function.name.clone())
            .collect();
        names.sort();
        assert_eq!(names, vec!["apply_patch", "bash", "create_pr", "edit", "git", "glob", "grep", "read", "todo_write", "write"]);
    }

    #[test]
    fn test_plan_mode_tools() {
        let reg = ToolRegistry::for_mode(ToolMode::Plan, None, None);
        let mut names: Vec<String> = reg
            .tool_definitions()
            .iter()
            .map(|d| d.function.name.clone())
            .collect();
        names.sort();
        assert_eq!(names, vec!["ask_user", "edit_plan", "glob", "grep", "read", "save_plan"]);
    }

    #[test]
    fn test_ask_user_not_in_coding() {
        let reg = ToolRegistry::for_mode(ToolMode::Coding, None, None);
        assert!(reg.get("ask_user").is_none());
    }

    #[test]
    fn test_ask_user_in_ask_mode() {
        let reg = ToolRegistry::for_mode(ToolMode::Ask, None, None);
        assert!(reg.get("ask_user").is_some());
    }

    #[test]
    fn test_ask_user_in_plan_mode() {
        let reg = ToolRegistry::for_mode(ToolMode::Plan, None, None);
        assert!(reg.get("ask_user").is_some());
    }

    #[test]
    fn test_destructive_tools_not_in_ask() {
        let reg = ToolRegistry::for_mode(ToolMode::Ask, None, None);
        assert!(reg.get("write").is_none());
        assert!(reg.get("edit").is_none());
        assert!(reg.get("bash").is_none());
        assert!(reg.get("git").is_none());
        assert!(reg.get("create_pr").is_none());
    }

    #[test]
    fn test_destructive_tools_not_in_plan() {
        let reg = ToolRegistry::for_mode(ToolMode::Plan, None, None);
        assert!(reg.get("write").is_none());
        assert!(reg.get("edit").is_none());
        assert!(reg.get("bash").is_none());
        assert!(reg.get("git").is_none());
        assert!(reg.get("create_pr").is_none());
    }

    // --- Context engine tool registration tests ---

    fn mock_context_engine_arg() -> Option<(Arc<dyn ContextEngineApi>, PathBuf)> {
        use crate::context_engine::MockContextEngine;
        Some((
            Arc::new(MockContextEngine::indexed_empty()),
            PathBuf::from("/repo"),
        ))
    }

    #[test]
    fn test_ask_mode_with_context_engine() {
        let reg = ToolRegistry::for_mode(ToolMode::Ask, mock_context_engine_arg(), None);
        assert!(reg.get("codebase_search").is_some());
        assert!(reg.get("codebase_graph").is_some());
        // Original ask tools still present
        assert!(reg.get("read").is_some());
        assert!(reg.get("grep").is_some());
        assert!(reg.get("ask_user").is_some());
    }

    #[test]
    fn test_coding_mode_with_context_engine() {
        let reg = ToolRegistry::for_mode(ToolMode::Coding, mock_context_engine_arg(), None);
        assert!(reg.get("codebase_search").is_some());
        assert!(reg.get("codebase_graph").is_some());
        // Original coding tools still present
        assert!(reg.get("read").is_some());
        assert!(reg.get("write").is_some());
        assert!(reg.get("bash").is_some());
    }

    #[test]
    fn test_plan_mode_with_context_engine() {
        let reg = ToolRegistry::for_mode(ToolMode::Plan, mock_context_engine_arg(), None);
        assert!(reg.get("codebase_search").is_some());
        assert!(reg.get("codebase_graph").is_some());
        // Original plan tools still present
        assert!(reg.get("read").is_some());
        assert!(reg.get("save_plan").is_some());
    }

    #[test]
    fn test_ask_mode_without_context_engine() {
        let reg = ToolRegistry::for_mode(ToolMode::Ask, None, None);
        assert!(reg.get("codebase_search").is_none());
        assert!(reg.get("codebase_graph").is_none());
    }

    #[test]
    fn test_coding_mode_without_context_engine() {
        let reg = ToolRegistry::for_mode(ToolMode::Coding, None, None);
        assert!(reg.get("codebase_search").is_none());
        assert!(reg.get("codebase_graph").is_none());
    }

    // --- Skill tool registration tests ---

    fn mock_skill_registry() -> Arc<crate::skills::SkillRegistry> {
        use crate::skills::registry::SkillInput;
        use std::collections::HashSet;
        let input = SkillInput {
            raw: "---\nname: hello\ndescription: A greeting skill.\n---\nbody\n".to_string(),
            path: PathBuf::from("/hello"),
        };
        Arc::new(crate::skills::SkillRegistry::new(
            vec![],
            vec![input],
            vec![],
            &HashSet::new(),
        ))
    }

    #[test]
    fn test_skill_tool_registered_in_all_modes_when_provided() {
        for mode in [ToolMode::Ask, ToolMode::Coding, ToolMode::Plan] {
            let reg = ToolRegistry::for_mode(mode, None, Some(mock_skill_registry()));
            assert!(
                reg.get("skill").is_some(),
                "skill tool missing in {:?} mode",
                mode
            );
        }
    }

    #[test]
    fn test_skill_tool_absent_when_no_registry() {
        for mode in [ToolMode::Ask, ToolMode::Coding, ToolMode::Plan] {
            let reg = ToolRegistry::for_mode(mode, None, None);
            assert!(
                reg.get("skill").is_none(),
                "skill tool should be absent in {:?} mode without registry",
                mode
            );
        }
    }
}
