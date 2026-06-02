use std::collections::HashSet;
use std::path::PathBuf;
use std::sync::Arc;

use serde::{Deserialize, Serialize};
use tauri::{AppHandle, Emitter, State};
use tokio::sync::RwLock;

use agent::agent::config::AgentConfig;
use agent::llm::{ChatMessage, LlmClient, LlmClientConfig, Provider};
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

// ── Providers (endpoints) + model selections ────────────────────────────────

/// A saved LLM provider = an *endpoint* (no model bundled). Persisted as a JSON
/// array under `llm_providers`. `kind` maps to a wire format:
/// openai/openai_compatible → OpenAI; anthropic → Anthropic. OpenAI and Anthropic
/// are built-in singletons (ids "openai"/"anthropic"); openai_compatible can be added.
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct ProviderConfig {
    pub id: String,
    pub kind: String,
    /// Display name (shown for openai_compatible providers; built-ins use their kind name).
    #[serde(default)]
    pub label: String,
    pub base_url: String,
    #[serde(default)]
    pub api_key: String,
    /// Available model ids (populated by "Fetch models" or typed). Feeds the pickers.
    #[serde(default)]
    pub models: Vec<String>,
}

impl ProviderConfig {
    fn provider(&self) -> Provider {
        match self.kind.as_str() {
            "anthropic" => Provider::Anthropic,
            _ => Provider::OpenAI,
        }
    }

    fn is_builtin(&self) -> bool {
        self.kind == "openai" || self.kind == "anthropic"
    }
}

/// A reference to a specific model on a specific provider. Used for the global
/// active/compaction/title selections and recorded on each session.
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct ModelRef {
    pub provider_id: String,
    pub model: String,
}

/// Global, outer-level model selections — each picks a model from across the
/// configured providers. `active` = main coding model for new sessions.
#[derive(Debug, Clone, Default, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct ModelSelection {
    #[serde(default)]
    pub active: Option<ModelRef>,
    #[serde(default)]
    pub compaction: Option<ModelRef>,
    #[serde(default)]
    pub title: Option<ModelRef>,
}

const PROVIDERS_KEY: &str = "llm_providers";
const SELECTION_KEY: &str = "llm_selection";

fn default_providers() -> Vec<ProviderConfig> {
    vec![
        ProviderConfig {
            id: "openai".to_string(),
            kind: "openai".to_string(),
            label: String::new(),
            base_url: "https://api.openai.com/v1".to_string(),
            api_key: String::new(),
            models: Vec::new(),
        },
        ProviderConfig {
            id: "anthropic".to_string(),
            kind: "anthropic".to_string(),
            label: String::new(),
            base_url: "https://api.anthropic.com".to_string(),
            api_key: String::new(),
            models: Vec::new(),
        },
    ]
}

/// Read saved providers. Always guarantees the built-in OpenAI + Anthropic rows
/// exist (self-heals older stores that predate one of them), keeping built-ins
/// first in a stable order, then user-added OpenAI-compatible providers.
fn read_providers(app_state: &AppState) -> Vec<ProviderConfig> {
    let stored: Vec<ProviderConfig> = app_state
        .db
        .get_setting(PROVIDERS_KEY)
        .ok()
        .flatten()
        .and_then(|raw| serde_json::from_str(&raw).ok())
        .unwrap_or_default();

    let mut list: Vec<ProviderConfig> = Vec::new();
    let mut changed = false;
    for builtin in default_providers() {
        match stored.iter().find(|p| p.id == builtin.id) {
            Some(existing) => list.push(existing.clone()),
            None => {
                list.push(builtin);
                changed = true;
            }
        }
    }
    for p in &stored {
        if !p.is_builtin() && !list.iter().any(|x| x.id == p.id) {
            list.push(p.clone());
        }
    }
    if changed {
        let _ = write_providers(app_state, &list);
    }
    list
}

fn write_providers(app_state: &AppState, providers: &[ProviderConfig]) -> Result<(), String> {
    let raw = serde_json::to_string(providers).map_err(|e| e.to_string())?;
    app_state.db.set_setting(PROVIDERS_KEY, &raw)
}

fn read_selection(app_state: &AppState) -> ModelSelection {
    app_state
        .db
        .get_setting(SELECTION_KEY)
        .ok()
        .flatten()
        .and_then(|raw| serde_json::from_str(&raw).ok())
        .unwrap_or_default()
}

fn write_selection(app_state: &AppState, sel: &ModelSelection) -> Result<(), String> {
    let raw = serde_json::to_string(sel).map_err(|e| e.to_string())?;
    app_state.db.set_setting(SELECTION_KEY, &raw)
}

fn provider_by_id(app_state: &AppState, id: &str) -> Option<ProviderConfig> {
    read_providers(app_state).into_iter().find(|p| p.id == id)
}

/// Resolve a `ModelRef` to its provider + model id.
fn resolve_ref(app_state: &AppState, r: &ModelRef) -> Option<(ProviderConfig, String)> {
    provider_by_id(app_state, &r.provider_id).map(|p| (p, r.model.clone()))
}

/// The main (provider, model) a NEW session should use: the active selection,
/// else the first provider that has at least one model.
fn default_session_model(app_state: &AppState) -> Option<ModelRef> {
    let sel = read_selection(app_state);
    if let Some(ref a) = sel.active {
        if provider_by_id(app_state, &a.provider_id).is_some() {
            return sel.active;
        }
    }
    read_providers(app_state)
        .into_iter()
        .find(|p| !p.models.is_empty())
        .map(|p| ModelRef { provider_id: p.id, model: p.models[0].clone() })
}

/// Resolve the (provider, model) a session runs with: what it was created with
/// (`provider_id` + `model`), falling back to the active selection.
fn session_model(app_state: &AppState, session: &SessionRow) -> Result<(ProviderConfig, String), String> {
    if let (Some(pid), Some(model)) = (session.provider_id.as_ref(), session.model.as_ref()) {
        if let Some(p) = provider_by_id(app_state, pid) {
            return Ok((p, model.clone()));
        }
    }
    default_session_model(app_state)
        .and_then(|r| resolve_ref(app_state, &r))
        .ok_or_else(|| "No model selected. Pick one in Settings / the model picker.".to_string())
}

pub fn provider_to_llm_config(p: &ProviderConfig, model: &str) -> LlmClientConfig {
    LlmClientConfig {
        provider: p.provider(),
        base_url: p.base_url.clone(),
        model: model.to_string(),
        api_key: p.api_key.clone(),
        temperature: None,
        max_completion_tokens: None,
        extra_headers: vec![],
        thinking: None,
        disable_cache_control: false,
    }
}

#[allow(clippy::too_many_arguments)]
fn build_agent_config(
    main_provider: &ProviderConfig,
    main_model: &str,
    compaction: Option<(&ProviderConfig, &str)>,
    working_dir: PathBuf,
    mode: ToolMode,
    context_window: usize,
    project_note: Option<&str>,
    checkpoint_dir: PathBuf,
    skills: Option<Arc<agent::skills::SkillRegistry>>,
    subagents: Option<Arc<agent::subagents::SubagentRegistry>>,
) -> AgentConfig {
    let mut config = AgentConfig::new(provider_to_llm_config(main_provider, main_model), working_dir);
    config.mode = mode;
    config.compaction_config.context_limit = context_window;
    config.skills = skills;
    config.subagents = subagents;
    config.checkpoint_dir = Some(checkpoint_dir);

    // Compaction runs on the globally-selected compaction model (any provider),
    // falling back to the session's own model when none is set.
    let mut compaction_llm = match compaction {
        Some((p, m)) => provider_to_llm_config(p, m),
        None => provider_to_llm_config(main_provider, main_model),
    };
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

/// Generate a concise session title from the first message using the configured
/// title model, off the turn's hot path. Updates the DB and emits
/// `agent:session_title` so the sidebar refreshes. Best-effort: any failure (bad
/// key, empty reply) silently leaves the substring fallback in place.
fn spawn_title_generation(
    app_handle: AppHandle,
    db: Arc<AgentDb>,
    session_id: String,
    first_message: String,
    provider: ProviderConfig,
    model: String,
) {
    tokio::spawn(async move {
        let mut cfg = provider_to_llm_config(&provider, &model);
        cfg.max_completion_tokens = Some(32);
        cfg.disable_cache_control = true;
        let client = LlmClient::new(cfg);
        // Few-shot: a labeling function, not a chat. The examples lock the model
        // into emitting a bare Title-Case title (3–6 words) even for vague input,
        // and demonstrate that it must never ask a question.
        let msgs = vec![
            ChatMessage::system(
                "You are a function that turns a developer's first message into a short coding-session \
                 title. Output ONLY the title: 3–6 words, Title Case, no quotes, no punctuation, no \
                 preamble. Never ask a question or request clarification — if the message is vague, \
                 title it literally from its words.",
            ),
            ChatMessage::user("fix the flaky auth test and add retries"),
            ChatMessage::assistant(Some("Fix Flaky Auth Test".to_string()), None, None),
            ChatMessage::user("add a dark mode toggle to the settings page"),
            ChatMessage::assistant(Some("Add Dark Mode Toggle".to_string()), None, None),
            ChatMessage::user("why is my build failing with a linker error"),
            ChatMessage::assistant(Some("Debug Linker Build Error".to_string()), None, None),
            ChatMessage::user("hey"),
            ChatMessage::assistant(Some("New Coding Session".to_string()), None, None),
            ChatMessage::user(first_message),
        ];
        let (tx, _rx) = tokio::sync::mpsc::channel(8);
        let probe_sid = format!("title-{}", uuid::Uuid::new_v4());
        let Ok(resp) = client.chat_completion(&msgs, &[], &tx, &probe_sid, None).await else {
            return;
        };
        // Reject refusals/questions/sentences — keep the substring fallback in that case.
        let Some(title) = resp.content.as_deref().and_then(clean_title) else { return };
        let title_db = title.clone();
        let sid_db = session_id.clone();
        let _ = tokio::task::spawn_blocking(move || db.set_session_title(&sid_db, &title_db)).await;
        let _ = app_handle.emit(
            "agent:session_title",
            serde_json::json!({ "session_id": session_id, "title": title }),
        );
    });
}

/// Sanitize an LLM title reply; returns `None` if it looks like a refusal,
/// a question, or a full sentence rather than a title.
fn clean_title(raw: &str) -> Option<String> {
    let mut t = raw.trim().trim_matches('"').trim();
    t = t.lines().next().unwrap_or("").trim();
    for prefix in ["Title:", "title:", "Title -", "Session:"] {
        if let Some(rest) = t.strip_prefix(prefix) {
            t = rest.trim();
        }
    }
    t = t.trim_matches('"').trim();
    if t.is_empty() {
        return None;
    }
    let lower = t.to_lowercase();
    let looks_bad = t.ends_with('?')
        || t.split_whitespace().count() > 10
        || lower.starts_with("i ")
        || lower.starts_with("i'm")
        || lower.starts_with("i need")
        || lower.starts_with("sorry")
        || lower.starts_with("could you")
        || lower.starts_with("can you")
        || lower.starts_with("please")
        || lower.contains("more context")
        || lower.contains("provide more");
    if looks_bad {
        return None;
    }
    Some(t.chars().take(60).collect())
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
    provider_id: Option<String>,
    model: Option<String>,
    app_state: State<'_, AppState>,
    agent_state: State<'_, AgentState>,
) -> Result<SessionRow, String> {
    let id = uuid::Uuid::new_v4().to_string();
    let db = Arc::clone(&agent_state.db);
    let folder_c = folder.clone();
    // Mode is just the session's initial mode; it can be switched per message.
    let mode_c = mode.unwrap_or_else(|| "coding".to_string());
    let title_c = title.clone();
    // Record the (provider, model) the session is created with: explicit pick,
    // else the active selection. Resume then uses the same one.
    let default = default_session_model(&app_state);
    let provider_c = provider_id.or_else(|| default.as_ref().map(|r| r.provider_id.clone()));
    let model_c = model.or_else(|| default.as_ref().map(|r| r.model.clone()));
    tokio::task::spawn_blocking(move || {
        db.create_session(
            &id,
            &folder_c,
            &mode_c,
            title_c.as_deref(),
            None,
            provider_c.as_deref(),
            model_c.as_deref(),
        )?;
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

/// Soft-delete a session (hidden from the sidebar; data preserved on disk).
/// Rejected while its folder has a running loop — stop that first.
#[tauri::command]
pub async fn agent_delete_session(
    session_id: String,
    agent_state: State<'_, AgentState>,
) -> Result<(), String> {
    let db = Arc::clone(&agent_state.db);
    let sid = session_id.clone();
    let session = tokio::task::spawn_blocking(move || db.get_session(&sid))
        .await
        .map_err(|e| format!("join error: {e}"))?
        .map_err(|e| format!("Failed to load session: {e}"))?;
    if let Some(s) = &session {
        if agent_state.running_folders.read().await.contains(&s.folder) {
            return Err("Stop the running session in this folder before deleting.".to_string());
        }
    }
    let db = Arc::clone(&agent_state.db);
    tokio::task::spawn_blocking(move || db.delete_session(&session_id))
        .await
        .map_err(|e| format!("join error: {e}"))?
        .map_err(|e| format!("Failed to delete session: {e}"))
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
        let default = default_session_model(&app_state);
        let provider_c = default.as_ref().map(|r| r.provider_id.clone());
        let model_c = default.as_ref().map(|r| r.model.clone());
        tokio::task::spawn_blocking(move || {
            db.create_session(&id, &folder, "coding", Some(&title), None, provider_c.as_deref(), model_c.as_deref())
        })
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

    let (provider, model) = match session_model(app_state, &session) {
        Ok(pm) => pm,
        Err(e) => {
            release(folder.clone());
            return Err(e);
        }
    };
    // Global compaction model (any provider), resolved once for this turn.
    let compaction = read_selection(app_state).compaction.and_then(|r| resolve_ref(app_state, &r));
    let context_window = agent_state.model_registry.read().await.context_window_for(&model);

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
        &provider,
        &model,
        compaction.as_ref().map(|(p, m)| (p, m.as_str())),
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

    // Mark active + set a title from the first message if unset.
    {
        let db = Arc::clone(&agent_state.db);
        let sid = session_id.clone();
        let needs_title = session.title.as_deref().unwrap_or("").is_empty();
        // Substring fallback so the sidebar has a label immediately.
        let fallback: String = message.chars().take(60).collect();
        let _ = tokio::task::spawn_blocking(move || {
            let _ = db.set_session_status(&sid, "active");
            if needs_title && !fallback.trim().is_empty() {
                let _ = db.set_session_title(&sid, fallback.trim());
            }
        })
        .await;

        // Then refine it asynchronously: use the configured title model, else fall
        // back to the session's own model. (If that LLM call fails, the substring
        // title set above stays — i.e. "user message only".)
        if needs_title && !message.trim().is_empty() {
            let (title_provider, title_model) = read_selection(app_state)
                .title
                .and_then(|r| resolve_ref(app_state, &r))
                .unwrap_or_else(|| (provider.clone(), model.clone()));
            spawn_title_generation(
                app_handle.clone(),
                Arc::clone(&agent_state.db),
                session_id.clone(),
                message.clone(),
                title_provider,
                title_model,
            );
        }
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
    let model = db
        .get_session(&session_id)
        .ok()
        .flatten()
        .and_then(|s| session_model(&app_state, &s).ok())
        .map(|(_, m)| m)
        .unwrap_or_default();
    let current_limit =
        agent_state.model_registry.read().await.context_window_for(&model) as u32;
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

    // Prefer the global compaction selection; else fall back to the session's own model.
    let (provider, comp_model) = read_selection(&app_state)
        .compaction
        .and_then(|r| resolve_ref(&app_state, &r))
        .map(Ok)
        .unwrap_or_else(|| {
            db.get_session(&session_id)
                .ok()
                .flatten()
                .map(|s| session_model(&app_state, &s))
                .unwrap_or_else(|| Err("No model selected. Pick one in Settings.".to_string()))
        })?;
    let mut compaction_cfg = provider_to_llm_config(&provider, &comp_model);
    compaction_cfg.disable_cache_control = true;
    let client = LlmClient::new(compaction_cfg);
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

// ── Providers (endpoints) + model selections ────────────────────────────────

#[derive(Debug, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct ProvidersResponse {
    pub providers: Vec<ProviderConfig>,
    pub selection: ModelSelection,
}

#[tauri::command]
pub async fn agent_list_providers(app_state: State<'_, AppState>) -> Result<ProvidersResponse, String> {
    Ok(ProvidersResponse {
        providers: read_providers(&app_state),
        selection: read_selection(&app_state),
    })
}

/// Add an OpenAI-compatible provider (the built-in OpenAI/Anthropic rows already exist).
#[tauri::command]
pub async fn agent_add_provider(
    provider: ProviderConfig,
    app_state: State<'_, AppState>,
) -> Result<ProviderConfig, String> {
    let mut providers = read_providers(&app_state);
    let mut provider = provider;
    if provider.id.trim().is_empty() {
        provider.id = uuid::Uuid::new_v4().to_string();
    }
    if provider.is_builtin() {
        return Err("Built-in providers already exist; add an OpenAI-compatible one".to_string());
    }
    providers.push(provider.clone());
    write_providers(&app_state, &providers)?;
    Ok(provider)
}

#[tauri::command]
pub async fn agent_update_provider(
    provider: ProviderConfig,
    app_state: State<'_, AppState>,
) -> Result<(), String> {
    let mut providers = read_providers(&app_state);
    match providers.iter_mut().find(|p| p.id == provider.id) {
        Some(slot) => *slot = provider,
        None => return Err(format!("Provider not found: {}", provider.id)),
    }
    write_providers(&app_state, &providers)
}

#[tauri::command]
pub async fn agent_delete_provider(id: String, app_state: State<'_, AppState>) -> Result<(), String> {
    let mut providers = read_providers(&app_state);
    if providers.iter().any(|p| p.id == id && p.is_builtin()) {
        return Err("Built-in providers can't be deleted".to_string());
    }
    providers.retain(|p| p.id != id);
    write_providers(&app_state, &providers)?;
    // Clear any selections that pointed at the removed provider.
    let mut sel = read_selection(&app_state);
    let clear = |r: &mut Option<ModelRef>| {
        if r.as_ref().map(|x| x.provider_id == id).unwrap_or(false) {
            *r = None;
        }
    };
    clear(&mut sel.active);
    clear(&mut sel.compaction);
    clear(&mut sel.title);
    write_selection(&app_state, &sel)
}

/// Set one of the global model selections. `role` ∈ "active" | "compaction" | "title".
#[tauri::command]
pub async fn agent_set_model_selection(
    role: String,
    provider_id: String,
    model: String,
    app_state: State<'_, AppState>,
) -> Result<(), String> {
    if provider_by_id(&app_state, &provider_id).is_none() {
        return Err(format!("Provider not found: {provider_id}"));
    }
    let mut sel = read_selection(&app_state);
    let r = Some(ModelRef { provider_id, model });
    match role.as_str() {
        "active" => sel.active = r,
        "compaction" => sel.compaction = r,
        "title" => sel.title = r,
        other => return Err(format!("Unknown selection role: {other}")),
    }
    write_selection(&app_state, &sel)
}

/// Both OpenAI (`/models`) and Anthropic (`/v1/models`) return `{ "data": [ { "id": .. } ] }`.
#[derive(Deserialize)]
struct ModelsListResponse {
    #[serde(default)]
    data: Vec<ModelEntry>,
}

#[derive(Deserialize)]
struct ModelEntry {
    id: String,
}

/// Build the GET request to a provider's models endpoint (URL + auth headers).
fn models_request(provider: &ProviderConfig) -> Result<reqwest::RequestBuilder, String> {
    use reqwest::header::{HeaderMap, AUTHORIZATION, HeaderValue};

    let base = provider.base_url.trim_end_matches('/');
    let mut headers = HeaderMap::new();
    let url = match provider.provider() {
        Provider::Anthropic => {
            if !provider.api_key.is_empty() {
                if let Ok(v) = HeaderValue::from_str(&provider.api_key) {
                    headers.insert("x-api-key", v);
                }
            }
            headers.insert(
                "anthropic-version",
                HeaderValue::from_static(agent::llm::anthropic::ANTHROPIC_VERSION),
            );
            format!("{base}/v1/models")
        }
        Provider::OpenAI => {
            if !provider.api_key.is_empty() {
                if let Ok(v) = HeaderValue::from_str(&format!("Bearer {}", provider.api_key)) {
                    headers.insert(AUTHORIZATION, v);
                }
            }
            format!("{base}/models")
        }
    };

    let client = reqwest::Client::builder()
        .timeout(std::time::Duration::from_secs(8))
        .build()
        .map_err(|e| format!("HTTP client error: {e}"))?;
    Ok(client.get(&url).headers(headers))
}

/// Query the provider's models endpoint and return the advertised model ids.
/// Uses the draft's base_url + api_key + kind (so it works before saving).
#[tauri::command]
pub async fn agent_fetch_provider_models(provider: ProviderConfig) -> Result<Vec<String>, String> {
    let resp = models_request(&provider)?
        .send()
        .await
        .map_err(|e| format!("Request failed: {e}"))?;
    if !resp.status().is_success() {
        return Err(format!("Models endpoint returned {}", resp.status()));
    }
    let body = resp.text().await.map_err(|e| format!("Failed to read response: {e}"))?;
    let parsed: ModelsListResponse =
        serde_json::from_str(&body).map_err(|_| "Unexpected models response shape".to_string())?;
    Ok(parsed.data.into_iter().map(|m| m.id).collect())
}

/// Key check used before saving. Sends a minimal "hi" (max 16 tokens) through the
/// SAME native path the agent uses (`/chat/completions` or `/v1/messages`), so it
/// validates the key + that the model actually responds. Rejects ONLY on a clear
/// auth failure (401/403); other errors (bad model, network, no models picked yet)
/// are inconclusive → allowed, so saving is never blocked spuriously.
#[tauri::command]
pub async fn agent_verify_provider(provider: ProviderConfig) -> Result<(), String> {
    if provider.api_key.is_empty() {
        return Ok(()); // nothing to verify
    }
    // Probe the first configured model; if none picked yet, fall back to a
    // models-endpoint key check so we can still catch a bad key.
    let Some(probe_model) = provider.models.first().cloned() else {
        return match models_request(&provider)?.send().await {
            Ok(resp) if resp.status() == reqwest::StatusCode::UNAUTHORIZED
                || resp.status() == reqwest::StatusCode::FORBIDDEN =>
            {
                Err(format!("API key rejected ({})", resp.status().as_u16()))
            }
            _ => Ok(()),
        };
    };

    let mut cfg = provider_to_llm_config(&provider, &probe_model);
    cfg.max_completion_tokens = Some(16);
    let client = LlmClient::new(cfg);
    let (tx, _rx) = tokio::sync::mpsc::channel(8);
    let sid = format!("verify-{}", uuid::Uuid::new_v4());
    match client
        .chat_completion(&[ChatMessage::user("hi")], &[], &tx, &sid, None)
        .await
    {
        Ok(_) => Ok(()),
        Err(agent::error::AgentError::LlmApiError { status, .. }) if status == 401 || status == 403 => {
            Err(format!("API key rejected ({status})"))
        }
        Err(_) => Ok(()), // bad model / network — inconclusive, don't block saving
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
