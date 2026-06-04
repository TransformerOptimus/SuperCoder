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
use std::sync::atomic::{AtomicBool, Ordering};
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

// ── docker CLI resolution ─────────────────────────────────────────────────
// GUI-launched apps (macOS Finder/Dock, Linux .desktop) inherit a minimal PATH
// that usually omits where Docker installs its CLI, so a bare `docker` lookup
// fails even when Docker is installed. Resolve the binary from well-known
// locations and widen the child PATH so docker can also find its compose plugin
// and credential helpers.

#[cfg(target_os = "macos")]
const DOCKER_DIRS: &[&str] = &[
    "/usr/local/bin",
    "/opt/homebrew/bin",
    "/Applications/Docker.app/Contents/Resources/bin",
    "/Applications/OrbStack.app/Contents/MacOS/xbin",
];
#[cfg(target_os = "linux")]
const DOCKER_DIRS: &[&str] = &["/usr/bin", "/usr/local/bin", "/snap/bin"];
#[cfg(target_os = "windows")]
const DOCKER_DIRS: &[&str] = &[r"C:\Program Files\Docker\Docker\resources\bin"];
#[cfg(not(any(target_os = "macos", target_os = "linux", target_os = "windows")))]
const DOCKER_DIRS: &[&str] = &[];

#[cfg(windows)]
const DOCKER_EXE: &str = "docker.exe";
#[cfg(not(windows))]
const DOCKER_EXE: &str = "docker";

/// Absolute path to the `docker` binary, falling back to bare `"docker"` (PATH).
/// `SUPERCODER_DOCKER_BIN` overrides everything (escape hatch for exotic setups).
fn docker_bin() -> String {
    if let Ok(p) = std::env::var("SUPERCODER_DOCKER_BIN") {
        if !p.is_empty() {
            return p;
        }
    }
    if let Some(home) = dirs::home_dir() {
        let p = home.join(".docker/bin").join(DOCKER_EXE);
        if p.exists() {
            return p.to_string_lossy().into_owned();
        }
    }
    for d in DOCKER_DIRS {
        let p = std::path::Path::new(d).join(DOCKER_EXE);
        if p.exists() {
            return p.to_string_lossy().into_owned();
        }
    }
    DOCKER_EXE.to_string()
}

/// A `docker` command with the binary resolved and the child PATH widened to the
/// common CLI dirs (so credential helpers / plugins resolve under a minimal GUI PATH).
fn docker_command() -> tokio::process::Command {
    let mut cmd = tokio::process::Command::new(docker_bin());
    let sep = if cfg!(windows) { ";" } else { ":" };
    let mut path = DOCKER_DIRS.join(sep);
    if let Ok(existing) = std::env::var("PATH") {
        if !existing.is_empty() {
            path = if path.is_empty() { existing } else { format!("{path}{sep}{existing}") };
        }
    }
    cmd.env("PATH", path);
    cmd
}

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
    /// True while a `start()` is in flight, so concurrent starts (auto-start +
    /// a user click) can't race and clobber each other's cancel token.
    starting: AtomicBool,
}

impl EngineController {
    pub fn new(mode: EngineMode, app: AppHandle, db: Arc<Database>) -> Self {
        Self {
            mode,
            app,
            db,
            status: Mutex::new(EngineStatus::Stopped),
            cancel: Mutex::new(None),
            starting: AtomicBool::new(false),
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
        let mut cmd = docker_command();
        cmd.args(args);
        git_ops::no_window::no_window_tokio(&mut cmd);
        cmd.output().await.map_err(|e| {
            if e.kind() == std::io::ErrorKind::NotFound {
                "Docker CLI not found. Install Docker Desktop, OrbStack, or colima \
                 (or set SUPERCODER_DOCKER_BIN to the docker path), then retry."
                    .to_string()
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

    /// `docker compose -p <project>` argv WITHOUT `-f`. Compose resolves the
    /// project from running-container labels, so teardown/inspection works even
    /// if the bundled compose file is missing or the app was moved.
    fn compose_project_argv(&self, extra: &[&str]) -> Vec<String> {
        let mut v = vec!["compose".to_string(), "-p".to_string(), PROJECT.to_string()];
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
        // Serialize starts: the first wins, a concurrent caller bails out.
        if self.starting.swap(true, Ordering::SeqCst) {
            return Err("engine is already starting".to_string());
        }
        struct StartGuard<'a>(&'a AtomicBool);
        impl Drop for StartGuard<'_> {
            fn drop(&mut self) {
                self.0.store(false, Ordering::SeqCst);
            }
        }
        let _start_guard = StartGuard(&self.starting);
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
        let mut cmd = docker_command();
        cmd.args(&argv);
        cmd.env("SUPERCODER_OPENAI_API_KEY", &key);
        cmd.env("SUPERCODER_PORT", port.to_string());
        // Pin the engine images to this app's version so an installed vX.Y.Z app
        // pulls the matching engine (the tag CI publishes). An explicit env
        // override wins, so a local unpushed build can still be tested.
        let ver = env!("CARGO_PKG_VERSION");
        cmd.env(
            "SUPERCODER_CE_IMAGE",
            std::env::var("SUPERCODER_CE_IMAGE").unwrap_or_else(|_| {
                format!("ghcr.io/transformeroptimus/supercoder/context-engine:v{ver}")
            }),
        );
        cmd.env(
            "SUPERCODER_CE_MIGRATE_IMAGE",
            std::env::var("SUPERCODER_CE_MIGRATE_IMAGE").unwrap_or_else(|_| {
                format!("ghcr.io/transformeroptimus/supercoder/context-engine-migrate:v{ver}")
            }),
        );
        cmd.stdout(Stdio::piped()).stderr(Stdio::piped());
        git_ops::no_window::no_window_tokio(&mut cmd);

        let mut child = cmd
            .spawn()
            .map_err(|e| format!("failed to launch docker compose: {e}"))?;

        // Stream compose stdout+stderr → progress events; keep the stderr tail so
        // a failed `up` can surface the real cause, not just "up failed".
        let stderr_tail = Arc::new(Mutex::new(Vec::<String>::new()));
        if let Some(out) = child.stdout.take() {
            self.spawn_line_relay(out, None);
        }
        if let Some(err) = child.stderr.take() {
            self.spawn_line_relay(err, Some(stderr_tail.clone()));
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
            let stderr = stderr_tail.lock().join("\n");
            let reason = if stderr.trim().is_empty() {
                "docker compose up failed".to_string()
            } else {
                format!("docker compose up failed:\n{}", stderr.trim())
            };
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

    fn spawn_line_relay<R>(&self, reader: R, sink: Option<Arc<Mutex<Vec<String>>>>)
    where
        R: tokio::io::AsyncRead + Unpin + Send + 'static,
    {
        let app = self.app.clone();
        tokio::spawn(async move {
            let mut lines = BufReader::new(reader).lines();
            while let Ok(Some(line)) = lines.next_line().await {
                let _ = app.emit("engine:progress", &line);
                if let Some(s) = &sink {
                    let mut g = s.lock();
                    g.push(line);
                    let overflow = g.len().saturating_sub(20);
                    if overflow > 0 {
                        g.drain(0..overflow);
                    }
                }
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
            // Wake immediately on cancellation instead of sitting out the sleep.
            tokio::select! {
                _ = token.cancelled() => return Err("startup cancelled".to_string()),
                _ = tokio::time::sleep(Duration::from_secs(3)) => {}
            }
        }
        Err("context engine did not become healthy within 3 minutes".to_string())
    }

    /// Stop containers, keep volumes (fast restart). Cancels any in-flight start.
    pub async fn stop(&self) -> Result<(), String> {
        if let Some(t) = self.cancel.lock().take() {
            t.cancel();
        }
        // Short stop timeout: the worker has no SIGTERM handler and would
        // otherwise hold the default ~10s grace before being killed, making quit
        // feel slow. 3s lets the datastores flush; volumes are kept either way.
        let argv = self.compose_project_argv(&["stop", "-t", "3"]);
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
        let extra: &[&str] = if remove_data {
            &["down", "-v", "--remove-orphans"]
        } else {
            &["down", "--remove-orphans"]
        };
        let argv = self.compose_project_argv(extra);
        let refs: Vec<&str> = argv.iter().map(String::as_str).collect();
        self.run_docker(&refs).await?;
        self.set_status(EngineStatus::Stopped);
        Ok(())
    }

    /// Tail of the combined compose logs (for surfacing a failure cause).
    pub async fn logs(&self, tail: usize) -> Result<String, String> {
        let tailn = tail.to_string();
        let argv = self.compose_project_argv(&["logs", "--tail", &tailn, "--no-color"]);
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

#[cfg(test)]
mod tests {
    use super::*;

    fn docker_installed_somewhere() -> bool {
        DOCKER_DIRS
            .iter()
            .any(|d| std::path::Path::new(d).join(DOCKER_EXE).exists())
            || dirs::home_dir()
                .map(|h| h.join(".docker/bin").join(DOCKER_EXE).exists())
                .unwrap_or(false)
    }

    /// The core of the fix: resolution must NOT depend on PATH. When docker is
    /// installed in a known location we get back an absolute, existing path —
    /// never the bare `"docker"` (which a minimal GUI PATH can't find).
    #[test]
    fn docker_bin_resolves_absolutely_not_via_path() {
        std::env::remove_var("SUPERCODER_DOCKER_BIN");
        let bin = docker_bin();
        if docker_installed_somewhere() {
            let p = std::path::Path::new(&bin);
            assert!(p.is_absolute() && p.exists(), "expected an absolute docker path, got {bin:?}");
        } else {
            assert_eq!(bin, DOCKER_EXE, "no docker in known dirs → bare PATH fallback");
        }
    }

    /// End-to-end of the bug + fix. Needs docker installed; run explicitly:
    ///   cargo test -p supercoder-agent-desktop -- --ignored docker_reachable
    #[tokio::test]
    #[ignore]
    async fn docker_reachable_under_finder_minimal_path() {
        std::env::remove_var("SUPERCODER_DOCKER_BIN");

        // (1) Baseline — on the normal PATH, plain `docker` is reachable. If not,
        // docker isn't installed here and there's nothing to prove, so skip.
        let baseline = tokio::process::Command::new("docker")
            .arg("version")
            .output()
            .await;
        if !baseline.map(|o| o.status.success()).unwrap_or(false) {
            eprintln!("skipping: docker not reachable on the normal PATH (not installed?)");
            return;
        }

        // (2) Reproduce the Finder/launchd environment: a minimal PATH that omits
        // where docker actually lives (/usr/local/bin, /opt/homebrew/bin, …).
        std::env::set_var("PATH", "/usr/bin:/bin:/usr/sbin:/sbin");

        // The original bug: a bare `docker` lookup typically fails now (unless this
        // box happens to keep docker in a system dir). Informational, not asserted.
        let bare_ok = tokio::process::Command::new("docker")
            .arg("version")
            .output()
            .await
            .map(|o| o.status.success())
            .unwrap_or(false);
        eprintln!("bare `docker` under minimal PATH reachable = {bare_ok} (false = bug reproduced)");

        // The fix: docker_command() resolves docker absolutely + widens PATH, so it
        // still works under the minimal Finder PATH.
        let fixed = docker_command()
            .arg("version")
            .arg("--format")
            .arg("{{.Client.Version}}")
            .output()
            .await
            .expect("spawn docker via docker_command()");
        assert!(
            fixed.status.success(),
            "docker_command() must reach docker under the minimal Finder PATH: {}",
            String::from_utf8_lossy(&fixed.stderr)
        );
    }
}
