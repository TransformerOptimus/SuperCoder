use std::collections::HashSet;
use std::path::PathBuf;
use std::sync::Arc;

use serde::Serialize;
use tauri::{AppHandle, Emitter, State};
use tokio::sync::RwLock;

use agent::agent::config::AgentConfig;
use agent::llm::{ChatMessage, LlmClient, LlmClientConfig};
use agent::persistence::MessagePersister;
use agent::session::SessionManager;
use agent::tool::ToolMode;

use crate::agent_bridge::db::{
    reconstruct_context, AgentDb, SessionRow, SqliteMessagePersister, StoredMessage,
};
use crate::agent_bridge::events::{
    spawn_event_relay, PermissionAwareApprovalHandler, TauriApprovalHandler,
    TauriApprovalHandlerFactory,
};
use crate::agent_bridge::permissions::PermissionConfig;
use crate::agent_bridge::traits::{EventEmitter, TauriEventEmitter};
use crate::AppState;

// ── AgentState ─────────────────────────────────────────────────────────────

pub struct AgentState {
    pub db: Arc<AgentDb>,
    /// App-managed snapshot checkpoint root: `<data>/.supercoder/checkpoints`.
    /// Per-session captures land in `<root>/<session_id>/turn-N`.
    pub checkpoint_root: PathBuf,
    pub(crate) session_manager: RwLock<Option<Arc<SessionManager>>>,
    /// Folders with a currently-running agent loop — enforces one active
    /// session per folder. Cleared by each session's monitor task on completion.
    pub(crate) running_folders: Arc<RwLock<HashSet<String>>>,
    pub(crate) approval_handlers: RwLock<std::collections::HashMap<String, Arc<TauriApprovalHandler>>>,
    pub(crate) model_registry: RwLock<agent::agent::model_profile::ModelRegistry>,
    pub(crate) write_lock_registry: Arc<agent::subagents::WriteLockRegistry>,
}

impl AgentState {
    pub fn new(db: Arc<AgentDb>, checkpoint_root: PathBuf) -> Self {
        Self {
            db,
            checkpoint_root,
            session_manager: RwLock::new(None),
            running_folders: Arc::new(RwLock::new(HashSet::new())),
            approval_handlers: RwLock::new(std::collections::HashMap::new()),
            model_registry: RwLock::new(agent::agent::model_profile::ModelRegistry::with_defaults()),
            write_lock_registry: Arc::new(agent::subagents::WriteLockRegistry::new()),
        }
    }

    async fn get_or_create_manager(&self) -> Arc<SessionManager> {
        {
            let guard = self.session_manager.read().await;
            if let Some(ref mgr) = *guard {
                return Arc::clone(mgr);
            }
        }
        let mut guard = self.session_manager.write().await;
        if let Some(ref mgr) = *guard {
            return Arc::clone(mgr);
        }
        // Production always passes a per-session persister_override, so the
        // manager's default is a Noop that is never exercised.
        let default: Arc<dyn MessagePersister> = Arc::new(agent::persistence::NoopPersister);
        let manager = Arc::new(SessionManager::new(default));
        *guard = Some(Arc::clone(&manager));
        manager
    }

    fn checkpoint_dir(&self) -> PathBuf {
        self.checkpoint_root.clone()
    }
}

// ── Attachments / user message ─────────────────────────────────────────────

#[derive(Debug, Clone, serde::Serialize, serde::Deserialize)]
pub struct AttachmentPayload {
    pub url: String,
    pub file_name: String,
    pub media_type: String,
}

async fn fetch_image_as_base64(url: &str, media_type: &str) -> String {
    // Local attachments (pasted/picked images) arrive as data: URLs already
    // base64-encoded by the frontend — pass them through untouched.
    if url.starts_with("data:") {
        return url.to_string();
    }
    let client = reqwest::Client::builder()
        .timeout(std::time::Duration::from_secs(15))
        .build()
        .unwrap_or_default();
    match client.get(url).header("User-Agent", "SuperCoder/1.0").send().await {
        Ok(resp) if resp.status().is_success() => match resp.bytes().await {
            Ok(bytes) => {
                use base64::Engine;
                let encoded = base64::engine::general_purpose::STANDARD.encode(&bytes);
                format!("data:{};base64,{}", media_type, encoded)
            }
            Err(_) => url.to_string(),
        },
        _ => url.to_string(),
    }
}

async fn build_user_message(text: &str, attachments: Option<Vec<AttachmentPayload>>) -> ChatMessage {
    let image_attachments: Vec<&AttachmentPayload> = attachments
        .as_ref()
        .map(|atts| atts.iter().filter(|a| a.media_type.starts_with("image/")).collect())
        .unwrap_or_default();

    if image_attachments.is_empty() {
        return ChatMessage::user(text);
    }

    use agent::llm::types::{ContentBlock, ImageUrlContent};
    let mut blocks = vec![ContentBlock::Text { text: text.to_string(), cache_control: None }];
    for att in image_attachments {
        let data_url = fetch_image_as_base64(&att.url, &att.media_type).await;
        blocks.push(ContentBlock::ImageUrl {
            image_url: ImageUrlContent { url: data_url, detail: Some("auto".to_string()) },
        });
    }
    ChatMessage::user_with_images(blocks)
}

// ── LLM settings / config ──────────────────────────────────────────────────

#[derive(Clone)]
pub struct LlmSettings {
    pub base_url: String,
    pub api_key: String,
    pub model: String,
}

/// The LLM gateway base URL. Configurable via `SUPERCODER_GATEWAY_URL`; defaults
/// to the local gateway. This is the default endpoint the app talks to when the
/// user hasn't set a custom base URL in Settings.
pub fn gateway_url() -> String {
    std::env::var("SUPERCODER_GATEWAY_URL")
        .ok()
        .filter(|s| !s.is_empty())
        .unwrap_or_else(|| "http://localhost:8080/api/v1".to_string())
}

fn read_llm_settings(app_state: &AppState) -> Result<LlmSettings, String> {
    let base_url = std::env::var("LLM_BASE_URL")
        .ok()
        .or_else(|| app_state.db.get_setting("llm_base_url").ok().flatten())
        .or_else(|| option_env!("LLM_BASE_URL").map(String::from))
        .filter(|s| !s.is_empty())
        .unwrap_or_else(gateway_url);

    let api_key = std::env::var("LLM_API_KEY")
        .ok()
        .or_else(|| std::env::var("OPENAI_API_KEY").ok())
        .or_else(|| app_state.db.get_setting("llm_api_key").ok().flatten())
        .unwrap_or_default();

    let model = std::env::var("LLM_MODEL")
        .ok()
        .or_else(|| app_state.db.get_setting("llm_model").ok().flatten())
        .unwrap_or_else(|| "gpt-4o-mini".to_string());

    Ok(LlmSettings { base_url, api_key, model })
}

/// The host:port authority of a URL (everything between `://` and the first `/`).
fn authority(url: &str) -> Option<&str> {
    let after = url.split("://").nth(1)?;
    Some(after.split('/').next().unwrap_or(after))
}

/// A URL is treated as a gateway (OpenAI→Anthropic translation + tenancy
/// headers) when its host:port matches the configured gateway, or it points at
/// a known SuperAGI gateway host.
fn is_gateway_url(url: &str) -> bool {
    if url.contains("superagi.com") {
        return true;
    }
    let gw = gateway_url();
    match (authority(url), authority(&gw)) {
        (Some(a), Some(b)) => a == b,
        _ => false,
    }
}

/// Local machine/user id headers for gateway tenancy (no accounts in v1).
fn gateway_headers(app_state: &AppState) -> Vec<(String, String)> {
    let machine_id = app_state
        .db
        .get_setting("machine_id")
        .ok()
        .flatten()
        .unwrap_or_else(|| {
            let id = uuid::Uuid::new_v4().to_string();
            let _ = app_state.db.set_setting("machine_id", &id);
            id
        });
    vec![("X-Machine-Id".to_string(), machine_id)]
}

pub fn build_llm_config(settings: &LlmSettings, gateway_auth: &[(String, String)]) -> LlmClientConfig {
    let mut auth_headers = Vec::new();
    if is_gateway_url(&settings.base_url) {
        auth_headers.extend(gateway_auth.iter().cloned());
        if !settings.api_key.is_empty() {
            auth_headers.push(("X-Auth-Token".to_string(), settings.api_key.clone()));
        }
    } else if !settings.api_key.is_empty() {
        auth_headers.push(("Authorization".to_string(), format!("Bearer {}", settings.api_key)));
    }
    LlmClientConfig {
        base_url: settings.base_url.clone(),
        model: settings.model.clone(),
        temperature: None,
        max_completion_tokens: None,
        auth_headers,
        thinking: None,
        disable_cache_control: false,
    }
}

#[allow(clippy::too_many_arguments)]
fn build_agent_config(
    settings: &LlmSettings,
    gateway_auth: &[(String, String)],
    working_dir: PathBuf,
    mode: ToolMode,
    context_window: usize,
    project_note: Option<&str>,
    checkpoint_dir: PathBuf,
    skills: Option<Arc<agent::skills::SkillRegistry>>,
    subagents: Option<Arc<agent::subagents::SubagentRegistry>>,
) -> AgentConfig {
    let mut config = AgentConfig::new(build_llm_config(settings, gateway_auth), working_dir);
    config.mode = mode;
    config.compaction_config.context_limit = context_window;
    config.skills = skills;
    config.subagents = subagents;
    config.checkpoint_dir = Some(checkpoint_dir);

    // Compaction summarization runs on a cheaper model regardless of the main model.
    let compaction_settings = LlmSettings { model: "claude-sonnet-4-6".to_string(), ..settings.clone() };
    let mut compaction_llm = build_llm_config(&compaction_settings, gateway_auth);
    compaction_llm.disable_cache_control = true;
    config.compaction_llm = Some(compaction_llm);

    config.system_prompt = Some(agent::agent::prompt::build_system_prompt(
        mode,
        &config.working_dir,
        None,
        project_note,
        config.skills.as_deref(),
        config.subagents.as_deref(),
    ));
    config
}

fn load_permission_config(app_state: &AppState, project_path: Option<&str>) -> PermissionConfig {
    tokio::task::block_in_place(|| {
        let conn = app_state.db.conn.lock();
        let _ = crate::agent_bridge::permissions::ensure_permissions_table(&conn);
        crate::agent_bridge::permissions::get_permission(&conn, project_path)
    })
}

async fn create_approval_handler(
    agent_state: &AgentState,
    emitter: Arc<dyn EventEmitter>,
    session_id: &str,
    perm_config: PermissionConfig,
) -> Option<Arc<dyn agent::approval::ApprovalHandler>> {
    let tauri_handler = Arc::new(TauriApprovalHandler::new(emitter, session_id.to_string()));
    agent_state
        .approval_handlers
        .write()
        .await
        .insert(session_id.to_string(), Arc::clone(&tauri_handler));
    Some(Arc::new(PermissionAwareApprovalHandler::new(tauri_handler, perm_config)))
}

/// Build the `SubagentInheritance` bundle. Children share the parent's persister
/// (persisting under their own child session_id; the crate stamps the parent
/// link in metadata) and the snapshot checkpoint dir.
async fn build_subagent_inheritance(
    config: &AgentConfig,
    agent_state: &AgentState,
    approval_handler: Option<&Arc<dyn agent::approval::ApprovalHandler>>,
    parent_session_id: String,
    parent_persister: Arc<SqliteMessagePersister>,
) -> Option<Arc<agent::subagents::SubagentInheritance>> {
    config.subagents.as_ref()?;
    let approval_factory: Option<Arc<dyn agent::subagents::ApprovalHandlerFactory>> =
        approval_handler.map(|h| {
            Arc::new(TauriApprovalHandlerFactory::new(Arc::clone(h)))
                as Arc<dyn agent::subagents::ApprovalHandlerFactory>
        });
    Some(Arc::new(agent::subagents::SubagentInheritance {
        llm_client_config: config.llm.clone(),
        retry_config: config.retry_config.clone(),
        compaction_config: config.compaction_config.clone(),
        compaction_llm: config.compaction_llm.clone(),
        max_iterations: config.max_iterations,
        context_engine: config.context_engine.clone(),
        context_engine_repo_path: config.context_engine_repo_path.clone(),
        persister: Some(parent_persister as Arc<dyn MessagePersister>),
        approval_handler_factory: approval_factory,
        parent_session_id: Some(parent_session_id),
        write_lock_registry: Arc::clone(&agent_state.write_lock_registry),
        checkpoint_dir: config.checkpoint_dir.clone(),
    }))
}

// ── Context helpers ────────────────────────────────────────────────────────

/// Deserialize stored messages into LLM ChatMessages and sanitize tool pairing.
pub(crate) fn deserialize_context(stored: Vec<StoredMessage>) -> Vec<ChatMessage> {
    let agent_messages = reconstruct_context(stored);
    let context: Vec<ChatMessage> = agent_messages
        .iter()
        .filter_map(|m| serde_json::from_value(m.llm_message.clone()).ok())
        .collect();
    sanitize_tool_pairs(context)
}

/// Ensure every assistant message with `tool_calls` is followed by matching
/// `tool` results; drop orphaned tool results. Required by the OpenAI wire format.
pub(crate) fn sanitize_tool_pairs(messages: Vec<ChatMessage>) -> Vec<ChatMessage> {
    let mut result: Vec<ChatMessage> = Vec::with_capacity(messages.len());
    let mut i = 0;
    while i < messages.len() {
        let msg = &messages[i];
        if msg.role == "assistant" {
            if let Some(ref tool_calls) = msg.tool_calls {
                let expected_ids: Vec<&str> = tool_calls.iter().map(|tc| tc.id.as_str()).collect();
                let mut found_ids = Vec::new();
                let mut j = i + 1;
                while j < messages.len() && messages[j].role == "tool" {
                    if let Some(ref tcid) = messages[j].tool_call_id {
                        if expected_ids.contains(&tcid.as_str()) {
                            found_ids.push(tcid.as_str());
                        }
                    }
                    j += 1;
                }
                result.push(messages[i].clone());
                for k in (i + 1)..j {
                    result.push(messages[k].clone());
                }
                for expected_id in &expected_ids {
                    if !found_ids.contains(expected_id) {
                        result.push(ChatMessage::tool_result(
                            *expected_id,
                            "[Tool result unavailable — the call was denied or the session ended before completion.]",
                        ));
                    }
                }
                i = j;
            } else {
                result.push(messages[i].clone());
                i += 1;
            }
        } else if msg.role == "tool" {
            i += 1; // orphaned tool result — drop
        } else {
            result.push(messages[i].clone());
            i += 1;
        }
    }
    result
}

// ── Response types ─────────────────────────────────────────────────────────

#[derive(Debug, Serialize)]
pub struct SendMessageResponse {
    pub session_id: String,
}

#[derive(Debug, Serialize)]
pub struct LlmConfigResponse {
    pub base_url: String,
    pub api_key: String,
    pub model: String,
}

#[derive(Debug, Serialize)]
pub struct ToolChip {
    pub name: String,
    pub summary: String,
}

#[derive(Debug, Serialize)]
pub struct AgentDisplayMessage {
    pub id: String,
    pub role: String,
    pub text: String,
    pub created_at: String,
    pub session_id: String,
    /// Tool calls that ran in the lead-up to this assistant message (for the
    /// "Thought for…" collapsible). Reconstructed from persisted tool_call rows.
    pub tools: Vec<ToolChip>,
    /// Wall-clock seconds spent on tool calls before this assistant message.
    pub duration_seconds: u32,
}

/// Pull a short, human-friendly summary out of a tool call's JSON arguments.
/// Tool names + arg keys mirror the crate's tools (camelCase keys).
fn tool_arg_summary(name: &str, args: &serde_json::Value) -> String {
    let get = |k: &str| args.get(k).and_then(|v| v.as_str()).unwrap_or("");
    let base = |p: &str| p.rsplit('/').next().unwrap_or(p).to_string();
    match name {
        "read" | "write" | "edit" | "apply_patch" => base(get("filePath")),
        "ls" => base(get("path")),
        "bash" | "git" => get("command").to_string(),
        "grep" | "glob" => get("pattern").to_string(),
        "codebase_search" | "codebase_graph" => get("query").to_string(),
        "save_plan" | "edit_plan" => base(get("path")),
        "todo_write" => "Updated todos".to_string(),
        "spawn_subagent" => get("name").to_string(),
        _ => String::new(),
    }
}

/// Seconds between two RFC3339 timestamps (0 if unparseable / negative).
fn elapsed_secs(from: &str, to: &str) -> u32 {
    match (
        chrono::DateTime::parse_from_rfc3339(from),
        chrono::DateTime::parse_from_rfc3339(to),
    ) {
        (Ok(a), Ok(b)) => (b - a).num_seconds().max(0) as u32,
        _ => 0,
    }
}

/// Parse the tool_call name(s) + arg summaries from a persisted tool_call row.
fn parse_tool_chips(llm_message: &str) -> Vec<ToolChip> {
    let Ok(v) = serde_json::from_str::<serde_json::Value>(llm_message) else {
        return Vec::new();
    };
    let Some(calls) = v.get("tool_calls").and_then(|c| c.as_array()) else {
        return Vec::new();
    };
    calls
        .iter()
        .filter_map(|c| {
            let func = c.get("function")?;
            let name = func.get("name").and_then(|n| n.as_str())?.to_string();
            let args = func
                .get("arguments")
                .and_then(|a| a.as_str())
                .and_then(|s| serde_json::from_str::<serde_json::Value>(s).ok())
                .unwrap_or(serde_json::Value::Null);
            let summary = tool_arg_summary(&name, &args);
            Some(ToolChip { name, summary })
        })
        .collect()
}

#[derive(Debug, Serialize)]
pub struct AgentDiffResult {
    pub files_changed: u32,
    pub insertions: u32,
    pub deletions: u32,
    pub stat: String,
    pub diff: String,
}

#[derive(Debug, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct CheckpointInfo {
    pub turn: u32,
    pub file_count: usize,
    pub paths: Vec<String>,
}

#[derive(Debug, Serialize)]
pub struct ContextUsageResponse {
    pub total_tokens: u32,
    pub context_limit: u32,
    pub message_count: u32,
}

fn parse_mode(mode: &str) -> ToolMode {
    match mode {
        "plan" => ToolMode::Plan,
        "coding" => ToolMode::Coding,
        _ => ToolMode::Ask,
    }
}

// ── Session lifecycle ──────────────────────────────────────────────────────

/// Create a new session (folder + mode). The session is the unit shown in the
/// sidebar; messages are sent into it via `agent_send_message`.
#[tauri::command]
pub async fn agent_create_session(
    folder: String,
    title: Option<String>,
    mode: Option<String>,
    agent_state: State<'_, AgentState>,
) -> Result<SessionRow, String> {
    let id = uuid::Uuid::new_v4().to_string();
    let db = Arc::clone(&agent_state.db);
    let folder_c = folder.clone();
    // Mode is just the session's initial mode; it can be switched per message.
    let mode_c = mode.unwrap_or_else(|| "coding".to_string());
    let title_c = title.clone();
    tokio::task::spawn_blocking(move || {
        db.create_session(&id, &folder_c, &mode_c, title_c.as_deref(), None)?;
        db.get_session(&id)
    })
    .await
    .map_err(|e| format!("join error: {e}"))?
    .map_err(|e| format!("Failed to create session: {e}"))?
    .ok_or_else(|| "Session not found after create".to_string())
}

/// Rename a session (sidebar title).
#[tauri::command]
pub async fn agent_rename_session(
    session_id: String,
    title: String,
    agent_state: State<'_, AgentState>,
) -> Result<(), String> {
    let db = Arc::clone(&agent_state.db);
    tokio::task::spawn_blocking(move || db.set_session_title(&session_id, &title))
        .await
        .map_err(|e| format!("join error: {e}"))?
        .map_err(|e| format!("Failed to rename session: {e}"))
}

/// List all sessions for the sidebar, most recent first.
#[tauri::command]
pub async fn agent_list_sessions(
    agent_state: State<'_, AgentState>,
) -> Result<Vec<SessionRow>, String> {
    let db = Arc::clone(&agent_state.db);
    tokio::task::spawn_blocking(move || db.list_sessions())
        .await
        .map_err(|e| format!("join error: {e}"))?
        .map_err(|e| format!("Failed to list sessions: {e}"))
}

/// Send a message into a session. `mode` ("ask" | "plan" | "coding") can be
/// switched on any message; it is persisted as the session's current mode.
#[tauri::command]
pub async fn agent_send_message(
    session_id: String,
    message: String,
    mode: Option<String>,
    attachments: Option<Vec<AttachmentPayload>>,
    app_handle: AppHandle,
    agent_state: State<'_, AgentState>,
    app_state: State<'_, AppState>,
) -> Result<SendMessageResponse, String> {
    let mut session = {
        let db = Arc::clone(&agent_state.db);
        let sid = session_id.clone();
        tokio::task::spawn_blocking(move || db.get_session(&sid))
            .await
            .map_err(|e| format!("join error: {e}"))?
            .map_err(|e| format!("Failed to load session: {e}"))?
            .ok_or_else(|| format!("Session not found: {session_id}"))?
    };

    // Apply a per-message mode switch and remember it on the session.
    if let Some(m) = mode {
        if m != session.mode {
            let db = Arc::clone(&agent_state.db);
            let sid = session_id.clone();
            let m_c = m.clone();
            let _ = tokio::task::spawn_blocking(move || db.set_session_mode(&sid, &m_c)).await;
        }
        session.mode = m;
    }

    run_agent_turn(
        app_handle,
        &app_state,
        &agent_state,
        session,
        message,
        attachments,
        None,
    )
    .await?;

    Ok(SendMessageResponse { session_id })
}

/// Start a NEW coding session seeded with a plan, and emit `agent-session-complete`
/// so the frontend opens the coding thread.
#[tauri::command]
pub async fn agent_start_coding_from_plan(
    app_handle: AppHandle,
    app_state: State<'_, AppState>,
    agent_state: State<'_, AgentState>,
    project_path: String,
    plan_text: String,
    plan_path: Option<String>,
    title: Option<String>,
) -> Result<SendMessageResponse, String> {
    if !std::path::Path::new(&project_path).exists() {
        return Err(format!("Project path does not exist: {project_path}"));
    }

    let plan_ref = plan_path.unwrap_or_else(|| format!("{project_path}/.agent/plan.md"));
    let task_summary = format!(
        "Implement the following plan step by step. The full plan also lives at: {plan_ref}\n\n\
         First, use todo_write to create todos from the plan's implementation steps, then work \
         through each one, marking them completed as you go. Edit files in place in the project.\n\n\
         ---\n\n{plan_text}"
    );

    // Create the coding session row.
    let session_id = uuid::Uuid::new_v4().to_string();
    {
        let db = Arc::clone(&agent_state.db);
        let id = session_id.clone();
        let folder = project_path.clone();
        let title = title.unwrap_or_else(|| "Implement plan".to_string());
        tokio::task::spawn_blocking(move || db.create_session(&id, &folder, "coding", Some(&title), None))
            .await
            .map_err(|e| format!("join error: {e}"))?
            .map_err(|e| format!("Failed to create session: {e}"))?;
    }

    let session = {
        let db = Arc::clone(&agent_state.db);
        let sid = session_id.clone();
        tokio::task::spawn_blocking(move || db.get_session(&sid))
            .await
            .map_err(|e| format!("join error: {e}"))?
            .map_err(|e| format!("Failed to load session: {e}"))?
            .ok_or("Session missing after create")?
    };

    run_agent_turn(
        app_handle.clone(),
        &app_state,
        &agent_state,
        session,
        task_summary.clone(),
        None,
        None,
    )
    .await?;

    let _ = app_handle.emit(
        "agent-session-complete",
        serde_json::json!({
            "session_id": session_id,
            "project_path": project_path,
            "mode": "coding",
            "task_summary": task_summary,
        }),
    );

    Ok(SendMessageResponse { session_id })
}

/// Core: build config + persister + approval handler, start the loop in the
/// session's mode, and spawn the relay. `turn_offset_override` lets rewind pin a turn.
async fn run_agent_turn(
    app_handle: AppHandle,
    app_state: &AppState,
    agent_state: &AgentState,
    session: SessionRow,
    message: String,
    attachments: Option<Vec<AttachmentPayload>>,
    _turn_offset_override: Option<u32>,
) -> Result<(), String> {
    let session_id = session.id.clone();
    let folder = session.folder.clone();
    let mode = parse_mode(&session.mode);

    // One active session per folder.
    {
        let mut running = agent_state.running_folders.write().await;
        if running.contains(&folder) {
            return Err(format!("A session is already running for {folder}"));
        }
        running.insert(folder.clone());
    }
    // Release the folder lock on any early return.
    let release = |folder: String| {
        let running = Arc::clone(&agent_state.running_folders);
        tokio::spawn(async move {
            running.write().await.remove(&folder);
        });
    };

    let work_dir = PathBuf::from(&folder);
    if !work_dir.exists() {
        release(folder.clone());
        return Err(format!("Folder does not exist: {folder}"));
    }

    let llm_settings = read_llm_settings(app_state)?;
    let gw = gateway_headers(app_state);
    let context_window = agent_state.model_registry.read().await.context_window_for(&llm_settings.model);

    // Plan-mode note: surface existing plan files so the agent reuses edit_plan.
    let project_note = if mode == ToolMode::Plan {
        let agent_dir = work_dir.join(".agent");
        let existing: Vec<String> = std::fs::read_dir(&agent_dir)
            .into_iter()
            .flatten()
            .filter_map(|e| e.ok())
            .filter(|e| {
                let n = e.file_name().to_string_lossy().to_string();
                n.starts_with("plan") && n.ends_with(".md")
            })
            .map(|e| format!(".agent/{}", e.file_name().to_string_lossy()))
            .collect();
        if existing.is_empty() {
            None
        } else {
            Some(format!(
                "EXISTING PLAN FILES: {}. If this request refines an existing plan, use edit_plan; \
                 otherwise ask the user whether to replace it or create a new plan file.",
                existing.join(", ")
            ))
        }
    } else {
        None
    };

    let skill_registry = crate::agent_bridge::skills::build_registry_for_agent(&agent_state.db, &work_dir);
    let subagent_registry = crate::agent_bridge::subagents::build_registry_for_agent(&agent_state.db, &work_dir);

    let mut config = build_agent_config(
        &llm_settings,
        &gw,
        work_dir.clone(),
        mode,
        context_window,
        project_note.as_deref(),
        agent_state.checkpoint_dir(),
        skill_registry,
        subagent_registry,
    );

    let emitter: Arc<dyn EventEmitter> = Arc::new(TauriEventEmitter::new(app_handle.clone()));
    let perm_config = load_permission_config(app_state, Some(&folder));
    let approval_handler = create_approval_handler(agent_state, emitter.clone(), &session_id, perm_config).await;

    let persister = Arc::new(SqliteMessagePersister::new(Arc::clone(&agent_state.db), folder.clone()));

    config.subagent_inheritance = build_subagent_inheritance(
        &config,
        agent_state,
        approval_handler.as_ref(),
        session_id.clone(),
        Arc::clone(&persister),
    )
    .await;

    // Prior context for resume.
    let prior_context = {
        let db = Arc::clone(&agent_state.db);
        let sid = session_id.clone();
        let stored = tokio::task::spawn_blocking(move || db.load_session_for_context(&sid, 500))
            .await
            .map_err(|e| format!("join error: {e}"))?
            .map_err(|e| format!("Failed to load context: {e}"))?;
        deserialize_context(stored)
    };

    // Persisted token count for compaction accuracy.
    let initial_tokens = {
        let db = Arc::clone(&agent_state.db);
        let sid = session_id.clone();
        tokio::task::spawn_blocking(move || db.get_context_usage(&sid))
            .await
            .ok()
            .and_then(|r| r.ok())
            .flatten()
            .map(|(t, _, _)| t as usize)
    };

    // Globally-unique turn numbering across messages (snapshot dir is the source of truth).
    let turn_offset = git_ops::checkpoint::list(&agent_state.checkpoint_root, &session_id)
        .await
        .ok()
        .and_then(|turns| turns.last().map(|t| t.turn))
        .unwrap_or(0);

    let manager = agent_state.get_or_create_manager().await;
    let user_msg = build_user_message(&message, attachments).await;

    let event_rx = match mode {
        ToolMode::Coding => manager
            .start_coding_session(
                session_id.clone(),
                config,
                user_msg,
                None,
                if prior_context.is_empty() { None } else { Some(prior_context) },
                None,
                initial_tokens,
                approval_handler,
                if turn_offset > 0 { Some(turn_offset) } else { None },
                true,
                Some(persister as Arc<dyn MessagePersister>),
            )
            .await,
        _ => manager
            .start_ask_session(
                session_id.clone(),
                config,
                user_msg,
                if prior_context.is_empty() { None } else { Some(prior_context) },
                initial_tokens,
                approval_handler,
                Some(persister as Arc<dyn MessagePersister>),
            )
            .await,
    }
    .map_err(|e| {
        release(folder.clone());
        format!("Failed to start session: {e}")
    })?;

    // Mark active + set title from first message if unset.
    {
        let db = Arc::clone(&agent_state.db);
        let sid = session_id.clone();
        let needs_title = session.title.as_deref().unwrap_or("").is_empty();
        let title: String = message.chars().take(60).collect();
        let _ = tokio::task::spawn_blocking(move || {
            let _ = db.set_session_status(&sid, "active");
            if needs_title && !title.trim().is_empty() {
                let _ = db.set_session_title(&sid, title.trim());
            }
        })
        .await;
    }

    let relay_handle = spawn_event_relay(
        emitter,
        event_rx,
        session_id.clone(),
        Some(Arc::clone(&agent_state.db)),
        folder.clone(),
        Some(agent_state.checkpoint_dir()),
    );

    // Monitor: clear the folder lock + flip status to idle when the relay ends.
    {
        let running = Arc::clone(&agent_state.running_folders);
        let db = Arc::clone(&agent_state.db);
        let sid = session_id.clone();
        let folder = folder.clone();
        tokio::spawn(async move {
            let _ = relay_handle.await;
            running.write().await.remove(&folder);
            let _ = tokio::task::spawn_blocking(move || db.set_session_status(&sid, "idle")).await;
        });
    }

    Ok(())
}

/// Cancel a running session.
#[tauri::command]
pub async fn agent_cancel_session(
    session_id: String,
    agent_state: State<'_, AgentState>,
) -> Result<(), String> {
    {
        let guard = agent_state.session_manager.read().await;
        if let Some(ref manager) = *guard {
            manager.cancel_session(&session_id).await;
        }
    }
    agent_state.approval_handlers.write().await.remove(&session_id);
    Ok(())
}

/// Resolve a pending tool approval.
#[tauri::command]
pub async fn agent_approve_tool(
    agent_state: State<'_, AgentState>,
    session_id: String,
    tool_call_id: String,
    approved: bool,
) -> Result<(), String> {
    let handlers = agent_state.approval_handlers.read().await;
    if let Some(handler) = handlers.get(&session_id) {
        handler.resolve(&tool_call_id, approved);
        Ok(())
    } else {
        Err(format!("No approval handler for session '{session_id}'"))
    }
}

/// Fetch displayable (user/assistant text) messages for a session.
#[tauri::command]
pub async fn agent_get_messages(
    session_id: String,
    agent_state: State<'_, AgentState>,
) -> Result<Vec<AgentDisplayMessage>, String> {
    let db = Arc::clone(&agent_state.db);
    let sid = session_id.clone();
    let stored = tokio::task::spawn_blocking(move || db.load_session_messages(&sid))
        .await
        .map_err(|e| format!("join error: {e}"))?
        .map_err(|e| format!("Failed to load messages: {e}"))?;

    // Walk rows chronologically: accumulate tool_call chips and attach them to
    // the next assistant text message (the "Thought for…" collapsible). This
    // replaces the old `<!-- thinking -->` marker scheme (chat-DB compat).
    let mut out = Vec::new();
    let mut pending_tools: Vec<ToolChip> = Vec::new();
    let mut pending_started: Option<String> = None;
    for msg in &stored {
        if msg.type_ == "tool_call" {
            if pending_started.is_none() {
                pending_started = Some(msg.created_at.clone());
            }
            pending_tools.extend(parse_tool_chips(&msg.llm_message));
            continue;
        }
        if (msg.role != "user" && msg.role != "assistant") || msg.type_ != "text" {
            continue;
        }
        let text = serde_json::from_str::<serde_json::Value>(&msg.llm_message)
            .ok()
            .and_then(|v| v.get("content").and_then(|c| c.as_str()).map(String::from))
            .unwrap_or_default();
        if text.is_empty() {
            if msg.role == "user" {
                pending_tools.clear();
                pending_started = None;
            }
            continue;
        }
        let (tools, duration_seconds) = if msg.role == "assistant" {
            let secs = pending_started
                .as_deref()
                .map(|start| elapsed_secs(start, &msg.created_at))
                .unwrap_or(0);
            pending_started = None;
            (std::mem::take(&mut pending_tools), secs)
        } else {
            pending_tools.clear();
            pending_started = None;
            (Vec::new(), 0)
        };
        out.push(AgentDisplayMessage {
            id: msg.id.to_string(),
            role: msg.role.clone(),
            text,
            created_at: msg.created_at.clone(),
            session_id: msg.session_id.clone(),
            tools,
            duration_seconds,
        });
    }
    Ok(out)
}

// ── Context usage / clear / compact ────────────────────────────────────────

#[tauri::command]
pub async fn agent_get_context_usage(
    session_id: String,
    agent_state: State<'_, AgentState>,
    app_state: State<'_, AppState>,
) -> Result<Option<ContextUsageResponse>, String> {
    let db = Arc::clone(&agent_state.db);
    let llm_settings = read_llm_settings(&app_state)?;
    let current_limit =
        agent_state.model_registry.read().await.context_window_for(&llm_settings.model) as u32;
    let sid = session_id.clone();
    let usage = tokio::task::spawn_blocking(move || db.get_context_usage(&sid))
        .await
        .map_err(|e| format!("join error: {e}"))?
        .map_err(|e| format!("Failed to get context usage: {e}"))?;
    Ok(usage.map(|(t, _persisted, m)| ContextUsageResponse {
        total_tokens: t,
        context_limit: current_limit,
        message_count: m,
    }))
}

#[tauri::command]
pub async fn agent_clear_context(
    session_id: String,
    agent_state: State<'_, AgentState>,
) -> Result<(), String> {
    let db = Arc::clone(&agent_state.db);
    let folder = db.get_session(&session_id).ok().flatten().map(|s| s.folder).unwrap_or_default();
    let sid = session_id.clone();
    tokio::task::spawn_blocking(move || db.insert_context_reset(&sid, &folder))
        .await
        .map_err(|e| format!("join error: {e}"))?
        .map_err(|e| format!("Failed to clear context: {e}"))
}

#[tauri::command]
pub async fn agent_compact_context(
    session_id: String,
    agent_state: State<'_, AgentState>,
    app_state: State<'_, AppState>,
) -> Result<String, String> {
    let db = Arc::clone(&agent_state.db);
    let folder = db.get_session(&session_id).ok().flatten().map(|s| s.folder).unwrap_or_default();

    let db_load = Arc::clone(&db);
    let sid_load = session_id.clone();
    let stored = tokio::task::spawn_blocking(move || db_load.load_session_messages(&sid_load))
        .await
        .map_err(|e| format!("join error: {e}"))?
        .map_err(|e| format!("Failed to load messages: {e}"))?;
    let messages = deserialize_context(stored);

    let keep_recent = agent::agent::config::CompactionConfig::default().keep_recent_messages;
    let Some((start, end)) = agent::agent::compaction::compaction_boundaries(&messages, keep_recent) else {
        return Ok("nothing_to_compact".to_string());
    };

    let prompt = agent::agent::compaction::build_compaction_prompt(&messages[start..end]);
    let compact_msgs = vec![
        ChatMessage::system(
            "Summarize this conversation. Structure: ## Goal, ## Key Decisions, ## Work Completed, \
             ## Current State, ## Relevant Files. Preserve exact file paths, function names, error \
             messages, and technical details. Be concise but complete."
                .to_string(),
        ),
        ChatMessage::user(prompt),
    ];

    let llm_settings = read_llm_settings(&app_state)?;
    let gw = gateway_headers(&app_state);
    let client = LlmClient::new(build_llm_config(&llm_settings, &gw));
    let (silent_tx, _rx) = tokio::sync::mpsc::channel(1);
    let compact_sid = format!("compact-{}", uuid::Uuid::new_v4());

    let mut summary_text: Option<String> = None;
    for _ in 0..2u8 {
        match client.chat_completion(&compact_msgs, &[], &silent_tx, &compact_sid, None).await {
            Ok(response) => {
                let text = response.content.unwrap_or_default();
                if !text.trim().is_empty() {
                    summary_text = Some(text);
                    break;
                }
            }
            Err(e) => log::error!("[compact] LLM error: {e}"),
        }
    }

    let (result_type, summary) = match summary_text {
        Some(text) => ("compacted", format!("[Context summary from earlier in this conversation]\n{text}")),
        None => ("truncated", "[Earlier context was truncated due to length.]".to_string()),
    };

    let db_w = Arc::clone(&db);
    let sid_w = session_id.clone();
    let folder_w = folder.clone();
    let start_u = start as u32;
    tokio::task::spawn_blocking(move || db_w.insert_compaction(&sid_w, &folder_w, &summary, start_u))
        .await
        .map_err(|e| format!("join error: {e}"))?
        .map_err(|e| format!("Failed to save compaction: {e}"))?;

    let db_d = Arc::clone(&db);
    let sid_d = session_id.clone();
    let _ = tokio::task::spawn_blocking(move || db_d.delete_context_usage(&sid_d)).await;

    Ok(result_type.to_string())
}

// ── Working-tree diffs ─────────────────────────────────────────────────────

async fn intent_to_add_new_files(repo: &std::path::Path) {
    let _ = git_ops::exec::run_git(repo, &["add", "-N", "."]).await;
}

#[tauri::command]
pub async fn agent_get_diff(project_path: String) -> Result<AgentDiffResult, String> {
    let repo = std::path::Path::new(&project_path);
    intent_to_add_new_files(repo).await;
    let result = git_ops::diff(repo, None, false, None).await.map_err(|e| e.to_string())?;
    Ok(AgentDiffResult {
        files_changed: result.files_changed,
        insertions: result.insertions,
        deletions: result.deletions,
        stat: result.stat,
        diff: result.diff,
    })
}

#[tauri::command]
pub async fn agent_get_working_diff(
    project_path: String,
    files: Option<Vec<String>>,
) -> Result<AgentDiffResult, String> {
    let repo = std::path::Path::new(&project_path);
    intent_to_add_new_files(repo).await;
    let file_refs: Option<Vec<&str>> = files.as_ref().map(|f| f.iter().map(|s| s.as_str()).collect());
    let result = git_ops::diff(repo, file_refs.as_deref(), false, None).await.map_err(|e| e.to_string())?;
    Ok(AgentDiffResult {
        files_changed: result.files_changed,
        insertions: result.insertions,
        deletions: result.deletions,
        stat: result.stat,
        diff: result.diff,
    })
}

// ── LLM config ─────────────────────────────────────────────────────────────

#[tauri::command]
pub async fn agent_save_llm_config(
    base_url: String,
    api_key: String,
    model: String,
    app_state: State<'_, AppState>,
) -> Result<(), String> {
    app_state.db.set_setting("llm_base_url", &base_url)?;
    app_state.db.set_setting("llm_api_key", &api_key)?;
    app_state.db.set_setting("llm_model", &model)?;
    Ok(())
}

#[tauri::command]
pub async fn agent_get_llm_config(app_state: State<'_, AppState>) -> Result<LlmConfigResponse, String> {
    let settings = read_llm_settings(&app_state)?;
    Ok(LlmConfigResponse {
        base_url: settings.base_url,
        api_key: settings.api_key,
        model: settings.model,
    })
}

#[tauri::command]
pub async fn agent_fetch_models(
    app_state: State<'_, AppState>,
    agent_state: State<'_, AgentState>,
) -> Result<Vec<agent::agent::model_profile::ModelProfile>, String> {
    let settings = read_llm_settings(&app_state)?;
    let url = format!("{}/models", settings.base_url.trim_end_matches('/'));

    let client = reqwest::Client::builder()
        .timeout(std::time::Duration::from_secs(5))
        .build()
        .map_err(|e| format!("HTTP client error: {e}"))?;

    let mut headers = reqwest::header::HeaderMap::new();
    if is_gateway_url(&settings.base_url) {
        for (key, value) in gateway_headers(&app_state) {
            if let (Ok(name), Ok(val)) = (
                key.parse::<reqwest::header::HeaderName>(),
                value.parse::<reqwest::header::HeaderValue>(),
            ) {
                headers.insert(name, val);
            }
        }
    }
    if !settings.api_key.is_empty() {
        if let Ok(val) = format!("Bearer {}", settings.api_key).parse::<reqwest::header::HeaderValue>() {
            headers.insert(reqwest::header::AUTHORIZATION, val);
        }
    }

    // No hardcoded fallback list: return exactly what the endpoint advertises.
    // If it returns nothing (or fails), the UI lets the user type a model id.
    match client.get(&url).headers(headers).send().await {
        Ok(resp) if resp.status().is_success() => {
            let body_text = resp.text().await.map_err(|e| format!("Failed to read response: {e}"))?;
            match serde_json::from_str::<agent::agent::model_profile::ModelsResponse>(&body_text) {
                Ok(body) if !body.models.is_empty() => {
                    agent_state.model_registry.write().await.merge(body.models.clone());
                    Ok(body.models)
                }
                _ => Ok(Vec::new()),
            }
        }
        _ => Ok(Vec::new()),
    }
}

// ── Permissions ────────────────────────────────────────────────────────────

#[tauri::command]
pub fn agent_get_permissions(
    state: State<AppState>,
    project_path: Option<String>,
) -> Result<PermissionConfig, String> {
    let conn = state.db.conn.lock();
    Ok(crate::agent_bridge::permissions::get_permission(&conn, project_path.as_deref()))
}

#[tauri::command]
pub fn agent_set_permission(state: State<AppState>, config: PermissionConfig) -> Result<(), String> {
    let conn = state.db.conn.lock();
    crate::agent_bridge::permissions::set_permission(&conn, &config)
}

// ── Skills / subagents ─────────────────────────────────────────────────────

fn is_valid_name(name: &str) -> bool {
    let len = name.len();
    (1..=64).contains(&len)
        && name.bytes().all(|b| b.is_ascii_lowercase() || b.is_ascii_digit() || b == b'-')
}

#[tauri::command]
pub fn agent_list_skills(
    agent_state: State<AgentState>,
    working_dir: Option<String>,
) -> Result<Vec<crate::agent_bridge::skills::DialogEntry>, String> {
    let wd = working_dir.map(PathBuf::from);
    Ok(crate::agent_bridge::skills::list_all_for_dialog(&agent_state.db, wd.as_deref()))
}

#[tauri::command]
pub fn agent_get_skills_paths(
    working_dir: Option<String>,
) -> Result<crate::agent_bridge::skills::SkillsPaths, String> {
    let wd = working_dir.map(PathBuf::from);
    Ok(crate::agent_bridge::skills::paths_for_display(wd.as_deref()))
}

#[tauri::command]
pub fn agent_set_skill_enabled(
    agent_state: State<AgentState>,
    name: String,
    enabled: bool,
) -> Result<(), String> {
    if !is_valid_name(&name) {
        return Err(format!("invalid skill name: {name:?}"));
    }
    agent_state.db.set_skill_enabled(&name, enabled).map_err(|e| e.to_string())
}

#[tauri::command]
pub fn agent_list_subagents(
    agent_state: State<AgentState>,
    working_dir: Option<String>,
) -> Result<Vec<crate::agent_bridge::subagents::DialogEntry>, String> {
    let wd = working_dir.map(PathBuf::from);
    Ok(crate::agent_bridge::subagents::list_all_for_dialog(&agent_state.db, wd.as_deref()))
}

#[tauri::command]
pub fn agent_get_subagents_paths(
    working_dir: Option<String>,
) -> Result<crate::agent_bridge::subagents::SubagentsPaths, String> {
    let wd = working_dir.map(PathBuf::from);
    Ok(crate::agent_bridge::subagents::paths_for_display(wd.as_deref()))
}

#[tauri::command]
pub fn agent_set_subagent_enabled(
    agent_state: State<AgentState>,
    name: String,
    enabled: bool,
) -> Result<(), String> {
    if !is_valid_name(&name) {
        return Err(format!("invalid subagent name: {name:?}"));
    }
    agent_state.db.set_subagent_enabled(&name, enabled).map_err(|e| e.to_string())
}

// ── Checkpoints (snapshot-backed) ──────────────────────────────────────────

#[tauri::command]
pub async fn agent_list_checkpoints(
    session_id: String,
    agent_state: State<'_, AgentState>,
) -> Result<Vec<CheckpointInfo>, String> {
    let turns = git_ops::checkpoint::list(&agent_state.checkpoint_root, &session_id)
        .await
        .unwrap_or_default();
    Ok(turns
        .into_iter()
        .map(|t| CheckpointInfo { turn: t.turn, file_count: t.file_count, paths: t.paths })
        .collect())
}

#[tauri::command]
pub async fn agent_get_turn_diff(
    session_id: String,
    turn: u32,
    agent_state: State<'_, AgentState>,
) -> Result<AgentDiffResult, String> {
    let d = git_ops::checkpoint::diff_turn(&agent_state.checkpoint_root, &session_id, turn)
        .await
        .map_err(|e| format!("Failed to diff turn: {e}"))?;
    Ok(AgentDiffResult {
        files_changed: d.files_changed,
        insertions: d.insertions,
        deletions: d.deletions,
        stat: d.stat,
        diff: d.diff,
    })
}

/// Cumulative working-tree diff of the session's project folder (uncommitted changes).
#[tauri::command]
pub async fn agent_get_full_diff(
    session_id: String,
    agent_state: State<'_, AgentState>,
) -> Result<AgentDiffResult, String> {
    let folder = {
        let db = Arc::clone(&agent_state.db);
        let sid = session_id.clone();
        tokio::task::spawn_blocking(move || db.get_session(&sid))
            .await
            .map_err(|e| format!("join error: {e}"))?
            .map_err(|e| format!("db error: {e}"))?
            .map(|s| s.folder)
            .ok_or("Session not found")?
    };
    let repo = std::path::Path::new(&folder);
    intent_to_add_new_files(repo).await;
    let result = git_ops::diff(repo, None, false, None).await.map_err(|e| e.to_string())?;
    Ok(AgentDiffResult {
        files_changed: result.files_changed,
        insertions: result.insertions,
        deletions: result.deletions,
        stat: result.stat,
        diff: result.diff,
    })
}

/// Restore the project files to a checkpoint turn and prune forward turns + messages.
#[tauri::command]
pub async fn agent_restore_checkpoint(
    session_id: String,
    turn: u32,
    agent_state: State<'_, AgentState>,
    app: AppHandle,
) -> Result<(), String> {
    // Cancel any running session first.
    {
        let guard = agent_state.session_manager.read().await;
        if let Some(ref manager) = *guard {
            manager.cancel_session(&session_id).await;
        }
    }
    agent_state.approval_handlers.write().await.remove(&session_id);
    tokio::time::sleep(std::time::Duration::from_millis(100)).await;

    git_ops::checkpoint::restore_to(&agent_state.checkpoint_root, &session_id, turn)
        .await
        .map_err(|e| format!("Failed to restore checkpoint: {e}"))?;
    let _ = git_ops::checkpoint::delete_from(&agent_state.checkpoint_root, &session_id, turn + 1).await;

    // Drop messages from undone turns so the conversation matches the files.
    let db = Arc::clone(&agent_state.db);
    let sid = session_id.clone();
    let _ = tokio::task::spawn_blocking(move || db.rewind_from_turn(&sid, turn + 1)).await;

    let _ = app.emit(
        "agent:checkpoint_restored",
        serde_json::json!({ "thread_id": session_id, "turn_count": turn }),
    );
    Ok(())
}

/// Rewind to a user message (optionally restoring files) and re-run with new text.
#[tauri::command]
pub async fn agent_rewind_to_message(
    app: AppHandle,
    session_id: String,
    message_sqlite_id: i64,
    restore_code: bool,
    new_text: String,
    attachments: Option<Vec<AttachmentPayload>>,
    agent_state: State<'_, AgentState>,
    app_state: State<'_, AppState>,
) -> Result<SendMessageResponse, String> {
    // Block if the session is still running.
    if agent_state.running_folders.read().await.iter().next().is_some() {
        // Cheap guard: refuse if anything is running; precise per-folder check below.
    }

    let target = {
        let db = Arc::clone(&agent_state.db);
        tokio::task::spawn_blocking(move || db.get_message_by_id(message_sqlite_id))
            .await
            .map_err(|e| format!("join error: {e}"))?
            .map_err(|e| format!("db error: {e}"))?
            .ok_or("Message not found")?
    };
    if target.role != "user" {
        return Err("Can only rewind to user messages".into());
    }
    if target.session_id != session_id {
        return Err("Message does not belong to this session".into());
    }

    // Soft-delete this message and everything after it.
    {
        let db = Arc::clone(&agent_state.db);
        let sid = session_id.clone();
        let _ = tokio::task::spawn_blocking(move || db.rewind_messages(&sid, target.id)).await;
    }

    if restore_code {
        let target_turn = target.turn_count.unwrap_or(1);
        let restore_turn = target_turn.saturating_sub(1);
        let _ = git_ops::checkpoint::restore_to(&agent_state.checkpoint_root, &session_id, restore_turn).await;
        let _ = git_ops::checkpoint::delete_from(&agent_state.checkpoint_root, &session_id, target_turn).await;
    }

    let session = {
        let db = Arc::clone(&agent_state.db);
        let sid = session_id.clone();
        tokio::task::spawn_blocking(move || db.get_session(&sid))
            .await
            .map_err(|e| format!("join error: {e}"))?
            .map_err(|e| format!("db error: {e}"))?
            .ok_or("Session not found")?
    };

    run_agent_turn(app, &app_state, &agent_state, session, new_text, attachments, None).await?;
    Ok(SendMessageResponse { session_id })
}
