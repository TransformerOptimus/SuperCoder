use std::collections::HashSet;
use std::path::PathBuf;
use std::sync::Arc;

use async_trait::async_trait;
use serde_json::{json, Value};
use tokio::sync::mpsc;
use uuid::Uuid;

use crate::agent::config::{CompactionConfig, RetryConfig};
use crate::agent::prompt::SystemBlock;
use crate::agent::{AgentConfig, AgentLoop};
use crate::approval::ApprovalHandler;
use crate::context_engine::ContextEngineApi;
use crate::error::{AgentError, ToolError};
use crate::llm::types::{CacheControl, ChatMessage};
use crate::llm::{LlmClient, LlmClientConfig, LlmProvider};
use crate::persistence::PersisterFactory;
use crate::tool::{Tool, ToolContext, ToolMode, ToolRegistry, ToolResult};
use crate::types::{AgentEvent, AgentResult};

use super::registry::SubagentRegistry;
use super::write_lock::WriteLockRegistry;

/// Builds a child `ApprovalHandler` that tags every request with the
/// originating subagent name. Tauri-side impl wraps the existing
/// TauriApprovalHandler; tests use a no-op implementation.
pub trait ApprovalHandlerFactory: Send + Sync {
    fn for_subagent(&self, subagent_name: &str) -> Arc<dyn ApprovalHandler>;
}

/// Everything `SpawnSubagentTool` needs to build a child AgentLoop while
/// staying agnostic of the Tauri layer.
pub struct SubagentInheritance {
    pub llm_client_config: LlmClientConfig,
    pub retry_config: RetryConfig,
    pub compaction_config: CompactionConfig,
    pub compaction_llm: Option<LlmClientConfig>,
    pub max_iterations: u32,
    pub context_engine: Option<Arc<dyn ContextEngineApi>>,
    pub context_engine_repo_path: Option<PathBuf>,
    pub persister_factory: Option<Arc<dyn PersisterFactory>>,
    pub approval_handler_factory: Option<Arc<dyn ApprovalHandlerFactory>>,
    pub parent_thread_id: Option<String>,
    pub write_lock_registry: Arc<WriteLockRegistry>,
}

pub struct SpawnSubagentTool {
    registry: Arc<SubagentRegistry>,
    inherit: Arc<SubagentInheritance>,
    description: String,
}

/// Tools that make a subagent "write-capable" and thus force it to
/// serialize on the per-worktree mutex. Mirrors Coding-mode's mutating set
/// from `tool::mod::ToolRegistry::for_mode`.
const WRITE_CAPABLE_TOOLS: &[&str] = &[
    "write",
    "edit",
    "bash",
    "apply_patch",
    "git",
    "create_pr",
];

impl SpawnSubagentTool {
    pub fn new(registry: Arc<SubagentRegistry>, inherit: Arc<SubagentInheritance>) -> Self {
        let description = build_description(&registry);
        Self {
            registry,
            inherit,
            description,
        }
    }
}

fn build_description(registry: &SubagentRegistry) -> String {
    let mut s = String::from(
        "Dispatch a specialized subagent. The child runs as a nested AgentLoop, \
         sees only your `prompt` (no parent history), and returns a final \
         summary as this tool's result. Child shares the parent's working \
         directory. Multiple calls in one assistant turn run concurrently \
         unless the subagent is write-capable (write-capable children serialize \
         per worktree).\n\nAvailable subagents:\n",
    );
    for (name, description) in registry.list_for_prompt() {
        s.push_str(&format!("- {name}: {description}\n"));
    }
    s
}

fn is_write_capable(effective_tools: &[String]) -> bool {
    effective_tools
        .iter()
        .any(|t| WRITE_CAPABLE_TOOLS.contains(&t.as_str()))
}

/// Build a human-readable one-line preview of the prompt for the chip label.
/// Collapses whitespace/newlines to single spaces and hard-truncates at
/// `max_chars` (byte-safe — relies on char boundaries).
fn truncate_prompt_preview(prompt: &str, max_chars: usize) -> String {
    let collapsed: String = prompt.split_whitespace().collect::<Vec<_>>().join(" ");
    if collapsed.chars().count() <= max_chars {
        return collapsed;
    }
    let mut out: String = collapsed.chars().take(max_chars).collect();
    out.push('…');
    out
}

#[async_trait]
impl Tool for SpawnSubagentTool {
    fn name(&self) -> &str {
        "spawn_subagent"
    }

    fn description(&self) -> &str {
        &self.description
    }

    fn parameters_schema(&self) -> Value {
        json!({
            "type": "object",
            "required": ["name", "prompt"],
            "properties": {
                "name": {
                    "type": "string",
                    "description": "Subagent name from the available list"
                },
                "prompt": {
                    "type": "string",
                    "description": "Full task description for the subagent"
                }
            }
        })
    }

    async fn execute(&self, args: Value, ctx: &ToolContext) -> Result<ToolResult, ToolError> {
        let name = args
            .get("name")
            .and_then(|v| v.as_str())
            .ok_or_else(|| ToolError("spawn_subagent: missing `name`".into()))?
            .to_string();
        let prompt = args
            .get("prompt")
            .and_then(|v| v.as_str())
            .ok_or_else(|| ToolError("spawn_subagent: missing `prompt`".into()))?
            .to_string();
        log::info!(
            "[subagents::spawn] called: name={} parent_session={} parent_tool_call_id={} prompt_chars={}",
            name,
            ctx.session_id,
            ctx.tool_call_id,
            prompt.len()
        );

        let Some(definition) = self.registry.get(&name) else {
            log::warn!(
                "[subagents::spawn] unknown subagent `{}` requested. available: {:?}",
                name,
                self.registry.names()
            );
            return Ok(ToolResult::error(format!(
                "unknown subagent `{}`. available: {}",
                name,
                self.registry.names().join(", ")
            )));
        };
        let definition = definition.clone();

        let child_session_id = Uuid::new_v4().to_string();
        let child_thread_id = format!("{}-sub-{}", ctx.session_id, &child_session_id[..8]);
        log::info!(
            "[subagents::spawn] resolved definition: name={} model_override={:?} allowed_tools={:?} child_session={} child_thread={}",
            definition.name,
            definition.model,
            definition.allowed_tools,
            child_session_id,
            child_thread_id,
        );

        // Build child's Coding-mode tool registry, then apply `allowed-tools` filter.
        // Depth-1 is enforced by not passing any subagents registry into the child.
        let context_engine_arg = self
            .inherit
            .context_engine
            .as_ref()
            .and_then(|e| {
                self.inherit
                    .context_engine_repo_path
                    .as_ref()
                    .map(|p| (e.clone(), p.clone()))
            });
        let mut child_registry =
            ToolRegistry::for_mode(ToolMode::Coding, context_engine_arg, None);
        if let Some(allowed) = &definition.allowed_tools {
            let allowed_set: HashSet<String> = allowed.iter().cloned().collect();
            child_registry.retain(|tool_name| allowed_set.contains(tool_name));
        }

        let effective_tools = child_registry.names();
        let write_capable = is_write_capable(&effective_tools);
        log::info!(
            "[subagents::spawn] child registry built: tools={:?} write_capable={} (working_dir={:?})",
            effective_tools,
            write_capable,
            ctx.working_dir,
        );

        // Child LLM: clone parent's client config, apply per-subagent model override.
        let mut child_llm = self.inherit.llm_client_config.clone();
        if let Some(m) = &definition.model {
            child_llm.model = m.clone();
        }

        // Child system prompt: subagent body cached + a minimal env footer.
        let env_block_text = format!(
            "# Environment\n- Working directory: {}\n- Depth: 1 (spawned by parent)\n- Date: {}\n",
            ctx.working_dir.display(),
            chrono::Local::now().format("%Y-%m-%d"),
        );
        let system_prompt = vec![
            SystemBlock {
                text: definition.body.clone(),
                cache_control: Some(CacheControl::ephemeral()),
            },
            SystemBlock {
                text: env_block_text,
                cache_control: None,
            },
        ];

        let child_config = AgentConfig {
            llm: child_llm,
            working_dir: ctx.working_dir.clone(),
            mode: ToolMode::Coding,
            max_iterations: self.inherit.max_iterations,
            system_prompt: Some(system_prompt),
            retry_config: self.inherit.retry_config.clone(),
            compaction_config: self.inherit.compaction_config.clone(),
            compaction_llm: self.inherit.compaction_llm.clone(),
            context_engine: self.inherit.context_engine.clone(),
            context_engine_repo_path: self.inherit.context_engine_repo_path.clone(),
            skills: None,
            subagents: None,
            subagent_inheritance: None, // depth-1 enforced
        };

        // Child event channel + drain. Per DEC-1 (in docs/subagent-bugs-and-fixes.md),
        // the parent UI does NOT mirror the child's stream — the child is
        // represented as a single tool-call chip via SubagentStart/End. We still
        // need a live receiver so the child's sends don't back up or error, but
        // the events are discarded after counting. The child's final summary
        // reaches the parent via `ToolResult::success(summary)`, which becomes
        // the tool_result entry in the parent's LLM message history.
        let (child_tx, mut child_rx) = mpsc::channel::<AgentEvent>(256);
        let drain_child_session = child_session_id.clone();
        // Collect modified_files emitted by the child's TurnCompleted events so
        // we can marshal them up to the parent's TurnCompleted via the returned
        // ToolResult. Without this the parent reports modified_files=[] for any
        // turn whose only tool call was spawn_subagent — UI per-turn file lists
        // under-report changes.
        let collected_modified_files = Arc::new(tokio::sync::Mutex::new(Vec::<String>::new()));
        let collected_for_drain = Arc::clone(&collected_modified_files);
        let drain_handle = tokio::spawn(async move {
            let mut count = 0usize;
            while let Some(ev) = child_rx.recv().await {
                count += 1;
                if let AgentEvent::TurnCompleted { modified_files, .. } = &ev {
                    if !modified_files.is_empty() {
                        collected_for_drain.lock().await.extend(modified_files.iter().cloned());
                    }
                }
            }
            log::info!(
                "[subagents::drain] child_session={} drained {} event(s) (not relayed to parent UI per DEC-1)",
                drain_child_session, count,
            );
        });

        let prompt_preview = truncate_prompt_preview(&prompt, 120);
        let _ = ctx
            .event_tx
            .send(AgentEvent::SubagentStart {
                session_id: ctx.session_id.clone(),
                parent_tool_call_id: ctx.tool_call_id.clone(),
                child_session_id: child_session_id.clone(),
                subagent_name: name.clone(),
                prompt_preview,
            })
            .await;

        // Write-capable subagents serialize per-working_dir. Lock is held for
        // the full child run; read-only subagents bypass. The wait is cancel-
        // aware — if the parent cancels while this child is still queued, we
        // short-circuit instead of waiting for the current holder to finish.
        let _write_guard = if write_capable {
            let lock = self.inherit.write_lock_registry.get_or_create(&ctx.working_dir);
            let wait_start = std::time::Instant::now();
            let guard = tokio::select! {
                g = lock.lock_owned() => {
                    let waited = wait_start.elapsed();
                    log::info!(
                        "[subagents::lock] write-capable guard acquired for {:?} (waited {}ms) child_session={}",
                        ctx.working_dir, waited.as_millis(), child_session_id,
                    );
                    g
                }
                _ = ctx.cancel_token.cancelled() => {
                    log::info!(
                        "[subagents::lock] cancelled while waiting for write-mutex (waited {}ms) child_session={}",
                        wait_start.elapsed().as_millis(), child_session_id,
                    );
                    let _ = ctx.event_tx.send(AgentEvent::SubagentEnd {
                        session_id: ctx.session_id.clone(),
                        parent_tool_call_id: ctx.tool_call_id.clone(),
                        child_session_id: child_session_id.clone(),
                        success: false,
                        summary: "subagent cancelled".to_string(),
                    }).await;
                    return Ok(ToolResult::error("subagent cancelled"));
                }
            };
            Some(guard)
        } else {
            log::info!(
                "[subagents::lock] read-only subagent, bypassing write mutex (child_session={})",
                child_session_id,
            );
            None
        };

        let child_cancel = ctx.cancel_token.child_token();

        let child_persister = self.inherit.persister_factory.as_ref().map(|f| {
            let parent_tid = self.inherit.parent_thread_id.as_deref().unwrap_or("");
            log::info!(
                "[subagents::spawn] attaching child persister parent_thread_id={:?} child_thread_id={}",
                parent_tid, child_thread_id,
            );
            f.for_subagent(parent_tid)
        });
        if self.inherit.persister_factory.is_none() {
            log::warn!(
                "[subagents::spawn] no persister_factory inherited — child messages will NOT be persisted"
            );
        }

        let child_approval = self
            .inherit
            .approval_handler_factory
            .as_ref()
            .map(|f| {
                log::info!(
                    "[subagents::spawn] attaching tagged approval handler for subagent={}",
                    name
                );
                f.for_subagent(&name)
            });

        let provider: Box<dyn LlmProvider> = Box::new(LlmClient::new(child_config.llm.clone()));
        let mut child_loop = AgentLoop::with_provider(
            child_config,
            provider,
            child_registry,
            child_cancel,
            child_tx,
            child_session_id.clone(),
        );
        if let Some(p) = child_persister {
            child_loop = child_loop.with_persister(p, Some(child_thread_id));
        }
        if let Some(h) = child_approval {
            child_loop = child_loop.with_approval_handler(h);
        }

        log::info!(
            "[subagents::spawn] child AgentLoop starting: subagent={} child_session={}",
            name, child_session_id,
        );
        let run_start = std::time::Instant::now();
        let run_result = child_loop.run(ChatMessage::user(prompt)).await;
        let run_elapsed = run_start.elapsed();

        // Drop the child loop so its event_tx sender closes; this lets the
        // drain task observe recv()→None and terminate. Awaiting the drain
        // before we return ensures the drain log and any future cleanup run
        // to completion inside the write guard's scope, so the next queued
        // subagent doesn't start before this one has fully unwound.
        drop(child_loop);
        let _ = drain_handle.await;

        let (success, summary) = match run_result {
            Ok(AgentResult::Done { summary }) => (true, summary),
            Ok(other) => (false, format!("subagent yielded unexpectedly: {other:?}")),
            Err(AgentError::Cancelled) => (false, "subagent cancelled".to_string()),
            Err(e) => (false, format!("subagent failed: {e}")),
        };
        log::info!(
            "[subagents::spawn] child AgentLoop finished: subagent={} success={} elapsed={}ms summary_chars={}",
            name, success, run_elapsed.as_millis(), summary.len(),
        );

        let _ = ctx
            .event_tx
            .send(AgentEvent::SubagentEnd {
                session_id: ctx.session_id.clone(),
                parent_tool_call_id: ctx.tool_call_id.clone(),
                child_session_id,
                success,
                summary: summary.clone(),
            })
            .await;

        // Deduplicate child's modified_files and attach to the returned
        // ToolResult so the parent's next TurnCompleted reflects the child's
        // filesystem changes too.
        let modified_files: Vec<String> = {
            let mut v = collected_modified_files.lock().await.clone();
            v.sort();
            v.dedup();
            v
        };

        Ok(ToolResult {
            output: summary,
            is_error: !success,
            yield_data: None,
            modified_files,
        })
    }
}
