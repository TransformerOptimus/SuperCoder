use parking_lot::Mutex;
use rusqlite::Connection;
use serde::Serialize;
use std::path::PathBuf;
use std::sync::Arc;
use tauri::Manager;

pub mod agent_bridge;

/// Local application data directory: `<data_local_dir>/.supercoder`.
/// Holds the settings DB, the agent DB, skills/subagents, and the checkpoint root.
pub fn app_data_dir() -> PathBuf {
    dirs::data_local_dir()
        .unwrap_or_else(|| PathBuf::from("."))
        .join(".supercoder")
}

// ── Database (local settings store) ───────────────────────────────────────────

pub struct Database {
    pub(crate) conn: Mutex<Connection>,
}

impl Database {
    pub fn new() -> Result<Self, String> {
        let db_dir = app_data_dir();
        std::fs::create_dir_all(&db_dir).map_err(|e| e.to_string())?;
        let db_path = db_dir.join("supercoder.db");

        let conn = Connection::open(&db_path).map_err(|e| e.to_string())?;
        conn.execute_batch(
            "CREATE TABLE IF NOT EXISTS settings (
                key TEXT PRIMARY KEY,
                value TEXT NOT NULL,
                updated_at TEXT DEFAULT CURRENT_TIMESTAMP
            );",
        )
        .map_err(|e| e.to_string())?;

        Ok(Self {
            conn: Mutex::new(conn),
        })
    }

    pub fn get_setting(&self, key: &str) -> Result<Option<String>, String> {
        let conn = self.conn.lock();
        let mut stmt = conn
            .prepare("SELECT value FROM settings WHERE key = ?")
            .map_err(|e| e.to_string())?;
        let result: Option<String> = stmt.query_row([key], |row| row.get(0)).ok();
        Ok(result)
    }

    pub fn set_setting(&self, key: &str, value: &str) -> Result<(), String> {
        let conn = self.conn.lock();
        conn.execute(
            "INSERT OR REPLACE INTO settings (key, value, updated_at) VALUES (?, ?, CURRENT_TIMESTAMP)",
            [key, value],
        )
        .map_err(|e| e.to_string())?;
        Ok(())
    }

    pub fn delete_setting(&self, key: &str) -> Result<(), String> {
        let conn = self.conn.lock();
        conn.execute("DELETE FROM settings WHERE key = ?", [key])
            .map_err(|e| e.to_string())?;
        Ok(())
    }
}

// ── AppState ────────────────────────────────────────────────────────────────

pub struct AppState {
    pub db: Database,
}

// ── File I/O ──────────────────────────────────────────────────────────────────

#[tauri::command]
async fn read_file_text(path: String) -> Result<Option<String>, String> {
    let p = std::path::Path::new(&path);
    if !p.exists() {
        return Ok(None);
    }
    std::fs::read_to_string(p)
        .map(Some)
        .map_err(|e| format!("Failed to read file: {e}"))
}

#[tauri::command]
async fn read_file_bytes(path: String) -> Result<Vec<u8>, String> {
    std::fs::read(&path).map_err(|e| format!("Failed to read file: {e}"))
}

/// Save raw bytes to a temp file and return the path.
/// Used for clipboard-pasted images that need a local path for the agent.
#[tauri::command]
fn save_temp_file(file_bytes: Vec<u8>, file_name: String) -> Result<String, String> {
    let temp_dir = std::env::temp_dir().join("supercoder-pastes");
    std::fs::create_dir_all(&temp_dir).map_err(|e| format!("Failed to create temp dir: {e}"))?;
    let dest = temp_dir.join(&file_name);
    std::fs::write(&dest, &file_bytes).map_err(|e| format!("Failed to write temp file: {e}"))?;
    Ok(dest.to_string_lossy().to_string())
}

/// Download a file from a URL and save it to the user's Downloads folder.
/// Returns the full path of the saved file.
#[tauri::command]
async fn download_file(url: String, file_name: String) -> Result<String, String> {
    let downloads_dir = dirs::download_dir()
        .unwrap_or_else(|| dirs::home_dir().unwrap_or_else(|| PathBuf::from(".")).join("Downloads"));

    std::fs::create_dir_all(&downloads_dir).map_err(|e| format!("Failed to create downloads dir: {}", e))?;

    // Avoid overwriting: if file exists, append (1), (2), etc.
    let mut dest = downloads_dir.join(&file_name);
    if dest.exists() {
        let stem = dest.file_stem().and_then(|s| s.to_str()).unwrap_or(&file_name).to_string();
        let ext = dest.extension().and_then(|s| s.to_str()).map(|s| format!(".{}", s)).unwrap_or_default();
        let mut counter = 1u32;
        loop {
            dest = downloads_dir.join(format!("{} ({}){}", stem, counter, ext));
            if !dest.exists() {
                break;
            }
            counter += 1;
        }
    }

    log::info!("[Download] {} -> {:?}", url, dest);

    let client = reqwest::Client::new();
    let response = client
        .get(&url)
        .send()
        .await
        .map_err(|e| format!("Download failed: {}", e))?;

    if !response.status().is_success() {
        return Err(format!("Download failed with status: {}", response.status()));
    }

    let bytes = response
        .bytes()
        .await
        .map_err(|e| format!("Failed to read response: {}", e))?;

    tokio::fs::write(&dest, &bytes)
        .await
        .map_err(|e| format!("Failed to write file: {}", e))?;

    let path_str = dest.to_string_lossy().to_string();
    log::info!("[Download] Saved to {}", path_str);
    Ok(path_str)
}

/// Fetch a URL and return the body as a string (bypasses browser CORS).
#[tauri::command]
async fn fetch_url(url: String) -> Result<String, String> {
    let resp = reqwest::get(&url).await.map_err(|e| format!("fetch failed: {e}"))?;
    if !resp.status().is_success() {
        return Err(format!("HTTP {}", resp.status()));
    }
    resp.text().await.map_err(|e| format!("read body failed: {e}"))
}

// ── OS integration ────────────────────────────────────────────────────────────

#[tauri::command]
async fn open_in_vscode(path: String) -> Result<(), String> {
    #[cfg(target_os = "macos")]
    {
        std::process::Command::new("open")
            .args(["-a", "Visual Studio Code", &path])
            .spawn()
            .map_err(|e| e.to_string())?;
    }
    #[cfg(target_os = "windows")]
    {
        std::process::Command::new("cmd")
            .args(["/c", "code", &path])
            .spawn()
            .map_err(|e| e.to_string())?;
    }
    #[cfg(target_os = "linux")]
    {
        std::process::Command::new("code")
            .arg(&path)
            .spawn()
            .map_err(|e| e.to_string())?;
    }
    Ok(())
}

#[tauri::command]
async fn open_in_terminal(path: String) -> Result<(), String> {
    #[cfg(target_os = "macos")]
    {
        std::process::Command::new("open")
            .args(["-a", "Terminal", &path])
            .spawn()
            .map_err(|e| e.to_string())?;
    }
    #[cfg(target_os = "windows")]
    {
        std::process::Command::new("cmd")
            .args(["/c", "start", "cmd", "/k", &format!("cd /d {}", path)])
            .spawn()
            .map_err(|e| e.to_string())?;
    }
    #[cfg(target_os = "linux")]
    {
        std::process::Command::new("xterm")
            .args(["-e", &format!("cd {} && bash", path)])
            .spawn()
            .map_err(|e| e.to_string())?;
    }
    Ok(())
}

#[tauri::command]
async fn open_in_finder(path: String) -> Result<(), String> {
    #[cfg(target_os = "macos")]
    {
        std::process::Command::new("open")
            .arg(&path)
            .spawn()
            .map_err(|e| e.to_string())?;
    }
    #[cfg(target_os = "windows")]
    {
        std::process::Command::new("explorer")
            .arg(&path)
            .spawn()
            .map_err(|e| e.to_string())?;
    }
    #[cfg(target_os = "linux")]
    {
        std::process::Command::new("xdg-open")
            .arg(&path)
            .spawn()
            .map_err(|e| e.to_string())?;
    }
    Ok(())
}

#[cfg(target_os = "macos")]
#[link(name = "ApplicationServices", kind = "framework")]
extern "C" {
    fn AXIsProcessTrusted() -> bool;
}

#[tauri::command]
fn check_accessibility_permission() -> bool {
    #[cfg(target_os = "macos")]
    {
        unsafe { AXIsProcessTrusted() }
    }
    #[cfg(not(target_os = "macos"))]
    true
}

#[tauri::command]
fn open_accessibility_settings() -> Result<(), String> {
    #[cfg(target_os = "macos")]
    {
        std::process::Command::new("open")
            .arg("x-apple.systempreferences:com.apple.preference.security?Privacy_Accessibility")
            .spawn()
            .map_err(|e| e.to_string())?;
    }
    Ok(())
}

#[derive(Serialize)]
struct FileNode {
    title: String,
    key: String,
    is_leaf: bool,
    children: Vec<FileNode>,
}

fn build_file_tree(dir: &std::path::Path, prefix: &str) -> Vec<FileNode> {
    let mut nodes = Vec::new();
    let entries = match std::fs::read_dir(dir) {
        Ok(e) => e,
        Err(_) => return nodes,
    };

    let mut items: Vec<_> = entries.filter_map(|e| e.ok()).collect();
    items.sort_by_key(|e| {
        let is_dir = e.file_type().map(|ft| ft.is_dir()).unwrap_or(false);
        (!is_dir, e.file_name())
    });

    for entry in items {
        let name = entry.file_name().to_string_lossy().to_string();
        // Skip hidden files/dirs and common non-useful dirs
        if name.starts_with('.') || name == "node_modules" || name == "target" || name == "__pycache__" {
            continue;
        }
        let key = if prefix.is_empty() {
            name.clone()
        } else {
            format!("{}/{}", prefix, name)
        };
        let is_dir = entry.file_type().map(|ft| ft.is_dir()).unwrap_or(false);
        let children = if is_dir {
            build_file_tree(&entry.path(), &key)
        } else {
            vec![]
        };
        nodes.push(FileNode {
            title: name,
            key,
            is_leaf: !is_dir,
            children,
        });
    }
    nodes
}

#[tauri::command]
async fn list_directory_tree(path: String) -> Result<Vec<FileNode>, String> {
    let dir = std::path::Path::new(&path);
    if !dir.is_dir() {
        return Err(format!("Not a directory: {}", path));
    }
    Ok(build_file_tree(dir, ""))
}

// ── Git operations ──────────────────────────────────────────────────────────

#[tauri::command]
async fn git_branches(repo_path: String) -> Result<Vec<git_ops::BranchInfo>, String> {
    git_ops::branches(std::path::Path::new(&repo_path))
        .await
        .map_err(|e| e.to_string())
}

/// List tracked files via `git ls-files` (fast, respects .gitignore).
#[tauri::command]
async fn git_ls_files(repo_path: String) -> Result<Vec<String>, String> {
    let mut cmd = tokio::process::Command::new("git");
    cmd.args(["ls-files"]).current_dir(&repo_path);
    git_ops::no_window::no_window_tokio(&mut cmd);
    let output = cmd
        .output()
        .await
        .map_err(|e| format!("Failed to run git ls-files: {e}"))?;
    if !output.status.success() {
        return Err(format!(
            "git ls-files failed: {}",
            String::from_utf8_lossy(&output.stderr)
        ));
    }
    let files: Vec<String> = String::from_utf8_lossy(&output.stdout)
        .lines()
        .map(|s| s.to_string())
        .collect();
    Ok(files)
}

#[tauri::command]
async fn git_status(repo_path: String) -> Result<git_ops::StatusOutput, String> {
    git_ops::status(std::path::Path::new(&repo_path))
        .await
        .map_err(|e| e.to_string())
}

#[tauri::command]
async fn git_diff(repo_path: String) -> Result<git_ops::DiffOutput, String> {
    git_ops::diff(std::path::Path::new(&repo_path), None, false, None)
        .await
        .map_err(|e| e.to_string())
}

#[tauri::command]
async fn git_commit(repo_path: String, message: String) -> Result<git_ops::CommitOutput, String> {
    git_ops::commit(std::path::Path::new(&repo_path), &message, None)
        .await
        .map_err(|e| e.to_string())
}

#[tauri::command]
async fn git_push(repo_path: String, branch: String) -> Result<git_ops::PushOutput, String> {
    git_ops::push(std::path::Path::new(&repo_path), Some(&branch), None)
        .await
        .map_err(|e| e.to_string())
}

#[tauri::command]
async fn git_log(repo_path: String, count: u32) -> Result<Vec<git_ops::LogEntry>, String> {
    git_ops::log(std::path::Path::new(&repo_path), Some(count))
        .await
        .map_err(|e| e.to_string())
}

#[tauri::command]
async fn git_create_branch(
    repo_path: String,
    name: String,
    from: Option<String>,
) -> Result<(), String> {
    git_ops::create_branch(std::path::Path::new(&repo_path), &name, from.as_deref())
        .await
        .map_err(|e| e.to_string())
}

#[tauri::command]
async fn git_switch_branch(repo_path: String, name: String) -> Result<(), String> {
    git_ops::switch_branch(std::path::Path::new(&repo_path), &name)
        .await
        .map_err(|e| e.to_string())
}

#[tauri::command]
async fn git_create_pr(
    repo_path: String,
    title: String,
    body: String,
    branch: String,
    base: String,
) -> Result<git_ops::PrOutput, String> {
    git_ops::pr::create(std::path::Path::new(&repo_path), &title, &body, &branch, &base)
        .await
        .map_err(|e| e.to_string())
}

// ── App entry ───────────────────────────────────────────────────────────────

#[cfg_attr(mobile, tauri::mobile_entry_point)]
pub fn run() {
    // Load environment-specific .env file, then base .env and .env.local as fallbacks.
    let app_env = option_env!("VITE_APP_ENV").unwrap_or("development");
    let _ = dotenvy::from_filename(format!("../.env.{}", app_env));
    let _ = dotenvy::from_filename("../.env.local");
    let _ = dotenvy::dotenv();

    env_logger::Builder::from_env(env_logger::Env::default().default_filter_or("info")).init();

    tauri::Builder::default()
        .plugin(tauri_plugin_shell::init())
        .plugin(tauri_plugin_pty::init())
        .plugin(tauri_plugin_notification::init())
        .plugin(tauri_plugin_dialog::init())
        .setup(|app| {
            let db = Database::new().expect("Failed to create database");

            // Ensure permissions table exists in settings DB
            {
                let conn = db.conn.lock();
                agent_bridge::permissions::ensure_permissions_table(&conn)
                    .expect("Failed to create agent_permissions table");
            }

            let state = AppState { db };
            app.manage(state);

            // Agent DB + checkpoint root, both under the app data dir.
            let data_dir = app_data_dir();
            let agent_db = agent_bridge::db::AgentDb::new(&data_dir)
                .expect("Failed to create agent database");
            let agent_db_arc = Arc::new(agent_db);

            let checkpoint_root = data_dir.join("checkpoints");
            std::fs::create_dir_all(&checkpoint_root)
                .expect("Failed to create checkpoint root");

            let agent_state =
                agent_bridge::commands::AgentState::new(Arc::clone(&agent_db_arc), checkpoint_root);
            app.manage(agent_state);

            let window = app.get_webview_window("main").unwrap();
            if let Some(monitor) = window.current_monitor().unwrap_or(None) {
                let size = monitor.size();
                let scale = monitor.scale_factor();
                let screen_w = size.width as f64 / scale;
                let screen_h = size.height as f64 / scale;
                let w = screen_w * 0.80;
                let h = screen_h * 0.80;
                let x = (screen_w - w) / 2.0;
                let y = (screen_h - h) / 2.0;
                let _ = window.set_size(tauri::LogicalSize::new(w, h));
                let _ = window.set_position(tauri::LogicalPosition::new(x, y));
            }
            Ok(())
        })
        .invoke_handler(tauri::generate_handler![
            open_in_vscode,
            open_in_terminal,
            open_in_finder,
            check_accessibility_permission,
            open_accessibility_settings,
            list_directory_tree,
            git_ls_files,
            fetch_url,
            read_file_text,
            read_file_bytes,
            save_temp_file,
            download_file,
            git_branches,
            git_status,
            git_diff,
            git_commit,
            git_push,
            git_log,
            git_create_branch,
            git_switch_branch,
            git_create_pr,
            agent_bridge::commands::agent_create_session,
            agent_bridge::commands::agent_rename_session,
            agent_bridge::commands::agent_delete_session,
            agent_bridge::commands::agent_send_message,
            agent_bridge::commands::agent_start_coding_from_plan,
            agent_bridge::commands::agent_cancel_session,
            agent_bridge::commands::agent_approve_tool,
            agent_bridge::commands::agent_get_messages,
            agent_bridge::commands::agent_get_context_usage,
            agent_bridge::commands::agent_clear_context,
            agent_bridge::commands::agent_compact_context,
            agent_bridge::commands::agent_get_diff,
            agent_bridge::commands::agent_get_working_diff,
            agent_bridge::commands::agent_list_sessions,
            agent_bridge::commands::agent_list_providers,
            agent_bridge::commands::agent_add_provider,
            agent_bridge::commands::agent_update_provider,
            agent_bridge::commands::agent_delete_provider,
            agent_bridge::commands::agent_set_model_selection,
            agent_bridge::commands::agent_fetch_provider_models,
            agent_bridge::commands::agent_verify_provider,
            agent_bridge::commands::agent_get_context_engine,
            agent_bridge::commands::agent_set_context_engine,
            agent_bridge::commands::agent_get_permissions,
            agent_bridge::commands::agent_set_permission,
            agent_bridge::commands::agent_list_skills,
            agent_bridge::commands::agent_set_skill_enabled,
            agent_bridge::commands::agent_get_skills_paths,
            agent_bridge::commands::agent_list_subagents,
            agent_bridge::commands::agent_set_subagent_enabled,
            agent_bridge::commands::agent_get_subagents_paths,
            agent_bridge::commands::agent_list_checkpoints,
            agent_bridge::commands::agent_get_turn_diff,
            agent_bridge::commands::agent_get_full_diff,
            agent_bridge::commands::agent_restore_checkpoint,
            agent_bridge::commands::agent_rewind_to_message,
        ])
        .run(tauri::generate_context!())
        .expect("error while running tauri application");
}
