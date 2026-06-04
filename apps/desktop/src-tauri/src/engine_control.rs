//! App-managed context-engine lifecycle.
//!
//! In `app` mode the desktop app owns the docker-compose stack: it pulls the
//! published images, runs `docker compose up -d`, waits for `/api/health`, and
//! stops the stack on quit. In `user` mode this controller is inert — the user
//! runs their own backend and the app just connects to its URL (Phase 4/5).
//!
//! Everything downstream (the file watcher, the search/graph client) is
//! mode-agnostic: it reads `base_url` from the settings DB. In app mode this
//! controller writes the resolved `http://127.0.0.1:<port>` there on start.

use std::path::PathBuf;
use std::process::Stdio;
use std::sync::Arc;
use std::time::Duration;

use parking_lot::Mutex;
use serde::Serialize;
use tauri::{AppHandle, Emitter, Manager, State};
use tokio::io::{AsyncBufReadExt, BufReader};
use tokio_util::sync::CancellationToken;

use crate::Database;

/// Compose project for the app-managed stack. Distinct from the hand-run dev
/// stack (`supercoder-context-engine-dev`) so the two never collide.
const PROJECT: &str = "supercoder-context-engine";
const DEFAULT_PORT: u16 = 8106;

// Settings-DB keys.
const CE_KEY: &str = "context_engine"; // shared with agent_bridge::commands
const APP_PORT_KEY: &str = "context_engine_app_port";
const OPENAI_KEY_KEY: &str = "context_engine_openai_key";

#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize)]
#[serde(rename_all = "lowercase")]
pub enum EngineMode {
    User,
    App,
}

/// Resolve the lifecycle mode once at startup. Explicit env wins; otherwise the
/// build profile decides (dev → user, release → app).
pub fn resolve_mode() -> EngineMode {
    match std::env::var("SUPERCODER_CE_MODE").ok().as_deref() {
        Some("app") => EngineMode::App,
        Some("user") => EngineMode::User,
        _ => {
            if cfg!(debug_assertions) {
                EngineMode::User
            } else {
                EngineMode::App
            }
        }
    }
}

/// Lifecycle status, mirrored to the frontend via the `engine:status` event.
#[derive(Debug, Clone, Serialize)]
#[serde(tag = "state", rename_all = "snake_case")]
pub enum EngineStatus {
    /// Docker CLI/daemon/compose missing or unreachable.
    DockerMissing { reason: String },
    Stopped,
    /// Pulling images / creating containers. `line` is the latest compose line.
    Pulling { line: String },
    /// Containers up; waiting for `/api/health`.
    Starting,
    Running { base_url: String },
    Error { reason: String, logs_tail: Option<String> },
}

pub struct EngineController {
    mode: EngineMode,
    app: AppHandle,
    db: Arc<Database>,
    status: Mutex<EngineStatus>,
    /// Cancels an in-flight `start()` (compose up + health wait).
    cancel: Mutex<Option<CancellationToken>>,
}

impl EngineController {
    pub fn new(mode: EngineMode, app: AppHandle, db: Arc<Database>) -> Self {
        Self {
            mode,
            app,
            db,
            status: Mutex::new(EngineStatus::Stopped),
            cancel: Mutex::new(None),
        }
    }

    pub fn mode(&self) -> EngineMode {
        self.mode
    }

    pub fn status(&self) -> EngineStatus {
        self.status.lock().clone()
    }

    fn set_status(&self, s: EngineStatus) {
        *self.status.lock() = s.clone();
        let _ = self.app.emit("engine:status", &s);
    }

    // ── docker plumbing ───────────────────────────────────────────────────

    async fn run_docker(&self, args: &[&str]) -> Result<std::process::Output, String> {
        let mut cmd = tokio::process::Command::new("docker");
        cmd.args(args);
        git_ops::no_window::no_window_tokio(&mut cmd);
        cmd.output().await.map_err(|e| {
            if e.kind() == std::io::ErrorKind::NotFound {
                "Docker CLI not found on PATH. Install Docker Desktop and retry.".to_string()
            } else {
                e.to_string()
            }
        })
    }

    /// Locate the bundled dist compose. Env override (for local testing) →
    /// bundled resource → dev fallback (walk up from the executable).
    fn compose_file(&self) -> Result<PathBuf, String> {
        if let Ok(p) = std::env::var("SUPERCODER_CE_COMPOSE_FILE") {
            let pb = PathBuf::from(p);
            if pb.exists() {
                return Ok(pb);
            }
        }
        if let Ok(dir) = self.app.path().resource_dir() {
            for cand in ["docker-compose.dist.yaml", "resources/docker-compose.dist.yaml"] {
                let pb = dir.join(cand);
                if pb.exists() {
                    return Ok(pb);
                }
            }
        }
        // Dev (`tauri dev`): walk up from the binary to the repo checkout.
        if let Ok(exe) = std::env::current_exe() {
            let mut cur = exe.parent().map(PathBuf::from);
            while let Some(dir) = cur {
                let pb = dir.join("services/context-engine/docker-compose.dist.yaml");
                if pb.exists() {
                    return Ok(pb);
                }
                cur = dir.parent().map(PathBuf::from);
            }
        }
        Err("could not locate docker-compose.dist.yaml (bundled resource missing)".to_string())
    }

    /// Base `docker compose -p <project> -f <file>` argv as owned strings.
    fn compose_argv(&self, file: &str, extra: &[&str]) -> Vec<String> {
        let mut v = vec![
            "compose".to_string(),
            "-p".to_string(),
            PROJECT.to_string(),
            "-f".to_string(),
            file.to_string(),
        ];
        v.extend(extra.iter().map(|s| s.to_string()));
        v
    }

    // ── preflight + port ──────────────────────────────────────────────────

    /// Docker CLI present + daemon up + compose v2 available.
    pub async fn preflight(&self) -> Result<(), String> {
        // `docker version` exits non-zero (with a server-side error) when the
        // daemon is down, even though the client part succeeds.
        let v = self
            .run_docker(&["version", "--format", "{{.Server.Version}}"])
            .await?;
        if !v.status.success() {
            return Err("Docker daemon is not running. Start Docker and retry.".to_string());
        }
        let c = self.run_docker(&["compose", "version"]).await?;
        if !c.status.success() {
            return Err("Docker Compose v2 is required (`docker compose`).".to_string());
        }
        Ok(())
    }

    async fn project_running(&self) -> bool {
        match self
            .run_docker(&["compose", "-p", PROJECT, "ps", "-q"])
            .await
        {
            Ok(o) => o.status.success() && !o.stdout.is_empty(),
            Err(_) => false,
        }
    }

    fn port_free(port: u16) -> bool {
        std::net::TcpListener::bind(("127.0.0.1", port)).is_ok()
    }

    fn saved_port(&self) -> Option<u16> {
        self.db
            .get_setting(APP_PORT_KEY)
            .ok()
            .flatten()
            .and_then(|s| s.parse().ok())
    }

    /// 8106 if it's free or already ours; else the next free port in a small
    /// range; else an error (foreign process holding the range).
    async fn resolve_port(&self) -> Result<u16, String> {
        if self.project_running().await {
            return Ok(self.saved_port().unwrap_or(DEFAULT_PORT));
        }
        if Self::port_free(DEFAULT_PORT) {
            return Ok(DEFAULT_PORT);
        }
        for p in (DEFAULT_PORT + 1)..=(DEFAULT_PORT + 20) {
            if Self::port_free(p) {
                return Ok(p);
            }
        }
        Err(format!(
            "port {DEFAULT_PORT} (and the next 20) are all in use by other processes"
        ))
    }

    // ── settings-DB helpers ─────────────────────────────────────────────────

    /// Point the shared context-engine settings at the controller-owned URL so
    /// the watcher + search client use it without knowing the mode.
    fn set_base_url(&self, url: &str) {
        let mut v: serde_json::Value = self
            .db
            .get_setting(CE_KEY)
            .ok()
            .flatten()
            .and_then(|s| serde_json::from_str(&s).ok())
            .unwrap_or_else(|| serde_json::json!({ "enabled": true, "base_url": url }));
        v["base_url"] = serde_json::json!(url);
        if let Ok(s) = serde_json::to_string(&v) {
            let _ = self.db.set_setting(CE_KEY, &s);
        }
    }

    pub fn set_openai_key(&self, key: &str) -> Result<(), String> {
        self.db.set_setting(OPENAI_KEY_KEY, key)
    }

    pub fn has_openai_key(&self) -> bool {
        self.db
            .get_setting(OPENAI_KEY_KEY)
            .ok()
            .flatten()
            .map(|s| !s.trim().is_empty())
            .unwrap_or(false)
    }

    // ── lifecycle ───────────────────────────────────────────────────────────

    /// Bring the stack up: preflight → resolve port → `compose up -d` (streaming
    /// progress) → wait healthy. Idempotent (`up -d` adopts a running stack).
    pub async fn start(&self) -> Result<(), String> {
        if self.mode != EngineMode::App {
            return Err("engine start is only available in app mode".to_string());
        }
        self.set_status(EngineStatus::Starting);

        if let Err(e) = self.preflight().await {
            self.set_status(EngineStatus::DockerMissing { reason: e.clone() });
            return Err(e);
        }

        let port = match self.resolve_port().await {
            Ok(p) => p,
            Err(e) => {
                self.set_status(EngineStatus::Error { reason: e.clone(), logs_tail: None });
                return Err(e);
            }
        };
        let _ = self.db.set_setting(APP_PORT_KEY, &port.to_string());
        let base_url = format!("http://127.0.0.1:{port}");
        self.set_base_url(&base_url);

        let file = self.compose_file()?;
        let file_str = file.to_string_lossy().to_string();
        let key = self
            .db
            .get_setting(OPENAI_KEY_KEY)
            .ok()
            .flatten()
            .unwrap_or_default();

        let token = CancellationToken::new();
        *self.cancel.lock() = Some(token.clone());

        self.set_status(EngineStatus::Pulling {
            line: "Pulling images & starting containers…".to_string(),
        });

        let argv = self.compose_argv(
            &file_str,
            &["up", "-d", "--remove-orphans", "--pull", "missing"],
        );
        let mut cmd = tokio::process::Command::new("docker");
        cmd.args(&argv);
        cmd.env("SUPERCODER_OPENAI_API_KEY", &key);
        cmd.env("SUPERCODER_PORT", port.to_string());
        // Pass image overrides through so a local build can be tested unpushed.
        for k in ["SUPERCODER_CE_IMAGE", "SUPERCODER_CE_MIGRATE_IMAGE"] {
            if let Ok(val) = std::env::var(k) {
                cmd.env(k, val);
            }
        }
        cmd.stdout(Stdio::piped()).stderr(Stdio::piped());
        git_ops::no_window::no_window_tokio(&mut cmd);

        let mut child = cmd
            .spawn()
            .map_err(|e| format!("failed to launch docker compose: {e}"))?;

        // Stream compose stdout+stderr → progress events.
        if let Some(out) = child.stdout.take() {
            self.spawn_line_relay(out);
        }
        if let Some(err) = child.stderr.take() {
            self.spawn_line_relay(err);
        }

        let exit = tokio::select! {
            r = child.wait() => r.map_err(|e| e.to_string())?,
            _ = token.cancelled() => {
                let _ = child.start_kill();
                let _ = self.stop().await;
                let e = "startup cancelled".to_string();
                self.set_status(EngineStatus::Stopped);
                return Err(e);
            }
        };

        if !exit.success() {
            let tail = self.logs(120).await.ok();
            let reason = "docker compose up failed".to_string();
            self.set_status(EngineStatus::Error { reason: reason.clone(), logs_tail: tail });
            return Err(reason);
        }

        self.set_status(EngineStatus::Starting);
        match self.wait_health(&base_url, &token).await {
            Ok(()) => {
                self.set_status(EngineStatus::Running { base_url });
                Ok(())
            }
            Err(e) => {
                let tail = self.logs(120).await.ok();
                self.set_status(EngineStatus::Error { reason: e.clone(), logs_tail: tail });
                Err(e)
            }
        }
    }

    fn spawn_line_relay<R>(&self, reader: R)
    where
        R: tokio::io::AsyncRead + Unpin + Send + 'static,
    {
        let app = self.app.clone();
        tokio::spawn(async move {
            let mut lines = BufReader::new(reader).lines();
            while let Ok(Some(line)) = lines.next_line().await {
                let _ = app.emit("engine:progress", &line);
            }
        });
    }

    /// Poll `/api/health` until it answers 2xx or ~3 min elapse.
    async fn wait_health(&self, base_url: &str, token: &CancellationToken) -> Result<(), String> {
        let client = reqwest::Client::builder()
            .timeout(Duration::from_secs(4))
            .build()
            .map_err(|e| e.to_string())?;
        let url = format!("{base_url}/api/health");
        for _ in 0..60 {
            if token.is_cancelled() {
                return Err("startup cancelled".to_string());
            }
            if let Ok(r) = client.get(&url).send().await {
                if r.status().is_success() {
                    return Ok(());
                }
            }
            tokio::time::sleep(Duration::from_secs(3)).await;
        }
        Err("context engine did not become healthy within 3 minutes".to_string())
    }

    /// Stop containers, keep volumes (fast restart). Cancels any in-flight start.
    pub async fn stop(&self) -> Result<(), String> {
        if let Some(t) = self.cancel.lock().take() {
            t.cancel();
        }
        let file = self.compose_file()?;
        // Short stop timeout: the worker has no SIGTERM handler and would
        // otherwise hold the default ~10s grace before being killed, making quit
        // feel slow. 3s lets the datastores flush; volumes are kept either way.
        let argv = self.compose_argv(&file.to_string_lossy(), &["stop", "-t", "3"]);
        let refs: Vec<&str> = argv.iter().map(String::as_str).collect();
        self.run_docker(&refs).await?;
        self.set_status(EngineStatus::Stopped);
        Ok(())
    }

    /// `down` (optionally `-v` to drop the indexed-data volumes).
    pub async fn down(&self, remove_data: bool) -> Result<(), String> {
        if let Some(t) = self.cancel.lock().take() {
            t.cancel();
        }
        let file = self.compose_file()?;
        let extra: &[&str] = if remove_data {
            &["down", "-v", "--remove-orphans"]
        } else {
            &["down", "--remove-orphans"]
        };
        let argv = self.compose_argv(&file.to_string_lossy(), extra);
        let refs: Vec<&str> = argv.iter().map(String::as_str).collect();
        self.run_docker(&refs).await?;
        self.set_status(EngineStatus::Stopped);
        Ok(())
    }

    /// Tail of the combined compose logs (for surfacing a failure cause).
    pub async fn logs(&self, tail: usize) -> Result<String, String> {
        let file = self.compose_file()?;
        let tailn = tail.to_string();
        let argv = self.compose_argv(
            &file.to_string_lossy(),
            &["logs", "--tail", &tailn, "--no-color"],
        );
        let refs: Vec<&str> = argv.iter().map(String::as_str).collect();
        let o = self.run_docker(&refs).await?;
        let mut s = String::from_utf8_lossy(&o.stdout).into_owned();
        s.push_str(&String::from_utf8_lossy(&o.stderr));
        Ok(s)
    }
}

// ── Tauri commands ──────────────────────────────────────────────────────────

type Ctl<'a> = State<'a, Arc<EngineController>>;

#[tauri::command]
pub fn agent_engine_mode(controller: Ctl<'_>) -> EngineMode {
    controller.mode()
}

#[tauri::command]
pub fn agent_engine_status(controller: Ctl<'_>) -> EngineStatus {
    controller.status()
}

#[tauri::command]
pub async fn agent_engine_preflight(controller: Ctl<'_>) -> Result<(), String> {
    controller.preflight().await
}

#[tauri::command]
pub async fn agent_engine_start(
    controller: Ctl<'_>,
    watcher: State<'_, Arc<crate::context_watcher::WatcherManager>>,
) -> Result<(), String> {
    controller.start().await?;
    // Backend is healthy — (re)attach the live watchers for known repos.
    watcher.auto_start().await;
    Ok(())
}

#[tauri::command]
pub async fn agent_engine_stop(controller: Ctl<'_>) -> Result<(), String> {
    controller.stop().await
}

#[tauri::command]
pub async fn agent_engine_remove(remove_data: bool, controller: Ctl<'_>) -> Result<(), String> {
    controller.down(remove_data).await
}

#[tauri::command]
pub fn agent_engine_has_key(controller: Ctl<'_>) -> bool {
    controller.has_openai_key()
}

#[tauri::command]
pub fn agent_engine_set_key(key: String, controller: Ctl<'_>) -> Result<(), String> {
    controller.set_openai_key(key.trim())
}
