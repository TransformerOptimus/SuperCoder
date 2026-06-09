//! bench-runner — run the SuperCoder agent headless against ONE task and emit a structured
//! JSON result. Built as a static musl binary, `docker cp`'d into a SWE-bench task container,
//! and driven by a Python orchestrator (Phase 2). This is the thin proof-of-life: parse a task
//! spec, run the agent, extract a git patch, print JSON. Mirrors `crates/agent/tests/integration.rs`.

use std::collections::HashMap;
use std::io::Read;
use std::path::PathBuf;
use std::sync::Arc;
use std::time::{Duration, Instant};

use clap::{Parser, ValueEnum};
use serde::{Deserialize, Serialize};

use agent::agent::config::AgentConfig;
use agent::agent::spawn_agent;
use agent::context_engine::{ContextEngineClient, ContextEngineConfig};
use agent::llm::types::ChatMessage;
use agent::llm::{LlmClientConfig, Provider};
use agent::tool::ToolMode;
use agent::types::{AgentEvent, AgentResult};

/// CLI flags. Per-task fields come from the task-spec JSON; model/provider/base-url are flags;
/// the API key is read from the environment only (never a flag or the spec — it's a secret).
#[derive(Parser, Debug)]
#[command(name = "bench-runner", about = "Run the SuperCoder agent on one task, emit JSON.")]
struct Cli {
    /// Path to the task-spec JSON. Reads from stdin when omitted.
    #[arg(long)]
    task_file: Option<PathBuf>,

    /// Model id, e.g. "claude-sonnet-4-6" or "gpt-4o-mini".
    #[arg(long)]
    model: String,

    /// Wire format. Auto-detected from the model name when omitted
    /// (claude-* → anthropic, else openai). Set explicitly for OSS models on
    /// OpenAI-compatible endpoints (Kimi/Qwen → --provider openai).
    #[arg(long, value_enum)]
    provider: Option<ProviderArg>,

    /// API base URL. Defaults to the provider's public endpoint when omitted.
    #[arg(long)]
    base_url: Option<String>,

    /// Extra HTTP headers for the LLM endpoint, repeatable: `--llm-header "X-Run-Id: abc"`.
    /// Used to pass an inference-router/gateway's tenancy + attribution headers
    /// (e.g. X-USER-ID, X-WORKSPACE-ID, X-Run-Id). Format "Key: Value" (first ':' splits).
    #[arg(long = "llm-header", value_name = "K: V")]
    llm_headers: Vec<String>,

    /// Context-engine base URL (bare, e.g. http://localhost:8106). When set, the agent gets
    /// `codebase_search`/`codebase_graph` (the ON path); when absent it falls back to grep/glob.
    #[arg(long)]
    context_engine_url: Option<String>,

    /// Context-engine isolation key for this repo. Defaults to the spec's instance_id.
    /// MUST match the bulk-indexer's --repo-path used to build the index.
    #[arg(long)]
    ce_repo_path: Option<String>,

    /// Context-engine identity (must match indexing). Defaults are the harness convention.
    #[arg(long, default_value = "bench")]
    ce_user_id: String,
    #[arg(long, default_value_t = 1)]
    ce_workspace_id: u64,
    #[arg(long, default_value = "bench")]
    ce_machine_id: String,
    #[arg(long, default_value = "")]
    ce_auth_token: String,
}

#[derive(Copy, Clone, Debug, ValueEnum)]
enum ProviderArg {
    Openai,
    Anthropic,
}

impl From<ProviderArg> for Provider {
    fn from(p: ProviderArg) -> Self {
        match p {
            ProviderArg::Openai => Provider::OpenAI,
            ProviderArg::Anthropic => Provider::Anthropic,
        }
    }
}

/// Per-task input (via --task-file or stdin).
#[derive(Deserialize, Debug)]
struct TaskSpec {
    instance_id: String,
    working_dir: PathBuf,
    base_commit: String,
    problem_statement: String,
    max_iterations: u32,
    timeout_secs: u64,
}

/// Structured result printed to stdout.
#[derive(Serialize)]
struct RunResult {
    instance_id: String,
    status: String,
    patch: String,
    summary: String,
    turns: u32,
    tokens: Tokens,
    modified_files: Vec<String>,
    tool_calls: Vec<ToolCallResult>,
    wall_clock_secs: f64,
    error: Option<String>,
}

#[derive(Serialize, Default)]
struct Tokens {
    total: u32,
    cache_read: u32,
    cache_creation: u32,
}

#[derive(Serialize)]
struct ToolCallResult {
    name: String,
    ok: bool,
}

#[tokio::main]
async fn main() {
    let cli = Cli::parse();

    let spec = match load_spec(&cli) {
        Ok(spec) => spec,
        Err(e) => {
            // No instance_id available yet — emit a minimal error envelope and exit non-zero.
            eprintln!("failed to read task spec: {e}");
            std::process::exit(2);
        }
    };

    let result = run(&cli, &spec).await;
    // The structured JSON is the contract with the orchestrator — always emit it to stdout.
    println!("{}", serde_json::to_string(&result).expect("serialize result"));
}

fn load_spec(cli: &Cli) -> Result<TaskSpec, String> {
    let raw = match &cli.task_file {
        Some(path) => std::fs::read_to_string(path).map_err(|e| e.to_string())?,
        None => {
            let mut buf = String::new();
            std::io::stdin()
                .read_to_string(&mut buf)
                .map_err(|e| e.to_string())?;
            buf
        }
    };
    serde_json::from_str(&raw).map_err(|e| e.to_string())
}

async fn run(cli: &Cli, spec: &TaskSpec) -> RunResult {
    let started = Instant::now();

    // Provider: explicit flag wins; otherwise claude-* → Anthropic, else OpenAI.
    let provider: Provider = match cli.provider {
        Some(p) => p.into(),
        None if cli.model.starts_with("claude-") => Provider::Anthropic,
        None => Provider::OpenAI,
    };
    let base_url = cli.base_url.clone().unwrap_or_else(|| match provider {
        Provider::Anthropic => "https://api.anthropic.com".to_string(),
        Provider::OpenAI => "https://api.openai.com/v1".to_string(),
    });
    // Secret comes from the environment only.
    let api_key = std::env::var("SUPERCODER_LLM_API_KEY")
        .or_else(|_| std::env::var("LLM_API_KEY"))
        .unwrap_or_default();
    // Reasoning models (GPT-5.x/o-series) reject temperature=0.0; keep it only for Claude.
    let temperature = if cli.model.starts_with("claude-") {
        Some(0.0)
    } else {
        None
    };

    // Parse repeatable `--llm-header "Key: Value"` flags into (name, value) pairs.
    // Forwarded verbatim with every LLM request (proxy tenancy/attribution headers).
    let extra_headers: Vec<(String, String)> = cli
        .llm_headers
        .iter()
        .filter_map(|h| {
            h.split_once(':')
                .map(|(k, v)| (k.trim().to_string(), v.trim().to_string()))
        })
        .collect();

    let llm = LlmClientConfig {
        provider,
        base_url,
        model: cli.model.clone(),
        api_key,
        temperature,
        max_completion_tokens: None,
        extra_headers,
        thinking: None,
        disable_cache_control: false,
    };

    let mut config = AgentConfig::new(llm, spec.working_dir.clone());
    config.mode = ToolMode::Coding;
    config.max_iterations = spec.max_iterations;
    // Everything else (system_prompt, checkpoint_dir, skills, subagents, …) stays at the
    // AgentConfig::new defaults of None — headless.

    // Context-engine ON when a URL is given: the agent auto-registers codebase_search/codebase_graph
    // (spawn_agent → ToolRegistry::for_mode). Absent → context_engine stays None → grep/glob fallback.
    if let Some(url) = &cli.context_engine_url {
        let ce = ContextEngineConfig {
            base_url: url.trim_end_matches('/').to_string(), // client appends /api/v1
            user_id: cli.ce_user_id.clone(),
            workspace_id: cli.ce_workspace_id,
            machine_id: cli.ce_machine_id.clone(),
            // Engine isolation key — MUST match the index built by bulk-indexer.
            repo_path: cli
                .ce_repo_path
                .clone()
                .unwrap_or_else(|| spec.instance_id.clone()),
            auth_token: cli.ce_auth_token.clone(),
        };
        config.context_engine = Some(Arc::new(ContextEngineClient::new(ce)));
        config.context_engine_repo_path = Some(spec.working_dir.clone()); // tool logging only
    }

    // persister/persist_session_id/approval all None → tools auto-execute headless.
    let spawned = spawn_agent(
        config,
        ChatMessage::user(spec.problem_statement.clone()),
        None,
        None,
        None,
    );
    let handle = spawned.handle;
    let cancel_token = spawned.cancel_token;
    let mut event_rx = spawned.event_rx;

    // Drain events concurrently until the channel closes.
    let collector = tokio::spawn(async move {
        let mut events = Vec::new();
        while let Some(event) = event_rx.recv().await {
            events.push(event);
        }
        events
    });

    // Await the result with a wall-clock cap.
    let (status, summary, error) =
        match tokio::time::timeout(Duration::from_secs(spec.timeout_secs), handle).await {
            Ok(Ok(Ok(AgentResult::Done { summary }))) => ("done".to_string(), summary, None),
            Ok(Ok(Ok(AgentResult::AskUser { question, .. }))) => {
                ("done".to_string(), question, None)
            }
            Ok(Ok(Ok(AgentResult::PlanReady { plan, .. }))) => ("done".to_string(), plan, None),
            Ok(Ok(Err(e))) => ("error".to_string(), String::new(), Some(e.to_string())),
            Ok(Err(join_err)) => (
                "error".to_string(),
                String::new(),
                Some(format!("agent task panicked: {join_err}")),
            ),
            Err(_elapsed) => {
                // Stop the agent loop so the event channel closes and the collector finishes.
                cancel_token.cancel();
                (
                    "timeout".to_string(),
                    String::new(),
                    Some(format!("exceeded timeout of {}s", spec.timeout_secs)),
                )
            }
        };

    let events = collector.await.unwrap_or_default();
    let (turns, modified_files, tool_calls, tokens) = summarize_events(&events);

    // Extract the patch: stage everything (so new files are captured) then diff vs base_commit.
    let patch = match extract_patch(&spec.working_dir, &spec.base_commit).await {
        Ok(patch) => patch,
        Err(e) => {
            // Surface the git failure without clobbering an agent error if one exists.
            eprintln!("patch extraction failed: {e}");
            String::new()
        }
    };

    RunResult {
        instance_id: spec.instance_id.clone(),
        status,
        patch,
        summary,
        turns,
        tokens,
        modified_files,
        tool_calls,
        wall_clock_secs: started.elapsed().as_secs_f64(),
        error,
    }
}

/// Roll the event stream up into the summary fields of the result.
fn summarize_events(
    events: &[AgentEvent],
) -> (u32, Vec<String>, Vec<ToolCallResult>, Tokens) {
    let mut turns = 0u32;
    let mut modified: Vec<String> = Vec::new();
    let mut tokens = Tokens::default();

    // Pair ToolStart (name) with ToolEnd (success) by tool_call_id, preserving call order.
    let mut order: Vec<String> = Vec::new();
    let mut names: HashMap<String, String> = HashMap::new();
    let mut oks: HashMap<String, bool> = HashMap::new();

    for event in events {
        match event {
            AgentEvent::TurnCompleted {
                turn_count,
                modified_files,
                ..
            } => {
                turns = turns.max(*turn_count);
                for f in modified_files {
                    if !modified.contains(f) {
                        modified.push(f.clone());
                    }
                }
            }
            AgentEvent::ToolStart {
                tool_call_id,
                tool_name,
                ..
            } => {
                if !names.contains_key(tool_call_id) {
                    order.push(tool_call_id.clone());
                }
                names.insert(tool_call_id.clone(), tool_name.clone());
            }
            AgentEvent::ToolEnd {
                tool_call_id,
                success,
                ..
            } => {
                oks.insert(tool_call_id.clone(), *success);
            }
            AgentEvent::TokenUsage {
                total_tokens,
                cache_read_tokens,
                cache_creation_tokens,
                ..
            } => {
                // Last update wins — total is cumulative for the session.
                tokens.total = *total_tokens;
                tokens.cache_read = cache_read_tokens.unwrap_or(0);
                tokens.cache_creation = cache_creation_tokens.unwrap_or(0);
            }
            _ => {}
        }
    }

    let tool_calls = order
        .into_iter()
        .map(|id| ToolCallResult {
            name: names.get(&id).cloned().unwrap_or_default(),
            ok: *oks.get(&id).unwrap_or(&false),
        })
        .collect();

    (turns, modified, tool_calls, tokens)
}

/// `git diff <base_commit>` — diff the working tree against the base, tracked files only.
///
/// We deliberately do NOT `git add -A` first. Staging everything also sweeps in untracked
/// artifacts the agent produces by running the repo (e.g. `__pycache__/*.pyc` from invoking
/// pytest), which pollute the patch and can break `git apply` / eval downstream. Diffing
/// against the base captures every modification to tracked files cleanly.
///
/// Trade-off: brand-new *untracked* files the agent creates are not included. That's acceptable
/// for SWE-bench-style tasks (fixes edit existing files). If a task set needs new-file capture,
/// reintroduce staging with an ignore-aware path filter rather than a blanket `add -A`.
///
/// Shells out to `git` (no toolchain needed in-container).
async fn extract_patch(working_dir: &PathBuf, base_commit: &str) -> Result<String, String> {
    let diff = git_ops::exec::run_git(working_dir, &["diff", base_commit])
        .await
        .map_err(|e| e.to_string())?;
    Ok(diff.stdout)
}
