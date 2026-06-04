use serde::Serialize;
use std::collections::HashMap;
use std::path::{Path, PathBuf};
use std::sync::Arc;
use std::time::Duration;
use tauri::Emitter;
use tokio::sync::{Mutex, RwLock, Semaphore};

use tokio::task::{JoinHandle, JoinSet};
use tokio_util::sync::CancellationToken;

use super::file_watcher::{ChangeBatch, FileWatcher};
use super::ignore_filter::IgnoreFilter;
use super::streamer::{Streamer, StreamerConfig, SyncError};

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

#[derive(Debug, Clone, Serialize)]
#[serde(tag = "status")]
pub enum IndexWatcherStatus {
    #[serde(rename = "not_indexed")]
    NotIndexed,
    #[serde(rename = "indexing")]
    Indexing,
    #[serde(rename = "indexed")]
    Indexed { file_count: Option<u64> },
    #[serde(rename = "error")]
    Error { reason: String, details: String },
}

struct RepoWatcher {
    cancel_token: CancellationToken,
    sync_task: JoinHandle<()>,
    _file_watcher: FileWatcher, // kept alive to hold the notify::RecommendedWatcher
    status: IndexWatcherStatus,
}

// ---------------------------------------------------------------------------
// WatcherManager
// ---------------------------------------------------------------------------

pub struct WatcherManager {
    watchers: Arc<RwLock<HashMap<String, RepoWatcher>>>,
    /// Stable per-machine id — the SAME value `commands::machine_id` resolves,
    /// so the watcher's collection key matches the search client's.
    machine_id: String,
    /// Shared settings DB (`supercoder.db`). Holds the `watched_repos` table and
    /// the `context_engine` settings the streamer reads live.
    db: Arc<crate::Database>,
    app_handle: tauri::AppHandle,
    /// Per-repo serialization mutex. Ensures only one full_sync or
    /// incremental_sync runs per repo_path at a time.
    sync_locks: Arc<RwLock<HashMap<String, Arc<Mutex<()>>>>>,
}

impl WatcherManager {
    /// Create a watcher manager. `machine_id` must be the same value the search
    /// client uses (`commands::machine_id`) so sync and queries hit the same
    /// collection. `db` is the shared settings DB.
    pub fn new(
        machine_id: String,
        db: Arc<crate::Database>,
        app_handle: tauri::AppHandle,
    ) -> Self {
        Self {
            watchers: Arc::new(RwLock::new(HashMap::new())),
            machine_id,
            db,
            app_handle,
            sync_locks: Arc::new(RwLock::new(HashMap::new())),
        }
    }

    /// Get (or create) the per-repo sync mutex.
    async fn get_sync_lock(&self, repo_path: &str) -> Arc<Mutex<()>> {
        let mut locks = self.sync_locks.write().await;
        locks
            .entry(repo_path.to_string())
            .or_insert_with(|| Arc::new(Mutex::new(())))
            .clone()
    }

    /// Build a `Streamer` for the given repo. Reads the context-engine settings
    /// live from the DB; returns `None` when the feature is disabled or the base
    /// URL is empty (a stale watcher then no-ops).
    async fn build_streamer(&self, repo_path: &str) -> Option<Streamer> {
        let settings = crate::agent_bridge::commands::read_context_engine_db(&self.db);
        if !settings.enabled {
            return None;
        }
        let base = crate::agent_bridge::commands::normalize_base_url(&settings.base_url);
        if base.is_empty() {
            return None;
        }
        Some(Streamer::new(StreamerConfig {
            // chat-desktop ran behind an APISIX gateway that stripped `/api/v1`;
            // the OSS server serves the index routes under `/api/v1`, so we append
            // it here and leave streamer.rs' `{url}/index/...` construction intact.
            context_engine_url: format!("{base}/api/v1"),
            // Local single-user identity — no accounts, no auth gateway.
            user_id: "local".to_string(),
            workspace_id: 0,
            machine_id: self.machine_id.clone(),
            repo_path: repo_path.to_string(),
            auth_token: String::new(),
        }))
    }

    /// Start watching a repo directory. Always runs a fresh `full_sync` first
    /// (plan §3.8 — catches offline edits).
    pub async fn start_watching(self: &Arc<Self>, repo_path: &str) -> Result<(), String> {
        if !crate::agent_bridge::commands::read_context_engine_db(&self.db).enabled {
            return Ok(());
        }

        let repo = PathBuf::from(repo_path);
        if !repo.is_dir() {
            return Err(format!("Not a directory: {repo_path}"));
        }

        // If already watching, bump DB timestamp and run a fresh full_sync
        // to catch offline edits (plan §3.8, decision #11). We reuse the
        // existing cancel_token so stop_watching can interrupt this sync.
        let existing_cancel = {
            let watchers = self.watchers.read().await;
            watchers.get(repo_path).map(|w| w.cancel_token.clone())
        };
        if let Some(cancel) = existing_cancel {
            self.upsert_repo_db(repo_path);
            let filter = Arc::new(IgnoreFilter::new(&repo));
            let status = match self.run_full_sync(repo_path, filter, cancel).await {
                Ok(s) => s,
                Err(e) => {
                    log::warn!(
                        "[ContextWatcher] Re-watch full_sync failed for {repo_path}: {e}"
                    );
                    IndexWatcherStatus::Error {
                        reason: "sync_failed".to_string(),
                        details: e,
                    }
                }
            };
            self.update_cached_status(repo_path, status).await;
            return Ok(());
        }

        let filter = Arc::new(IgnoreFilter::new(&repo));
        let cancel_token = CancellationToken::new();

        // Run the initial full_sync inline so the caller (and UI) sees the
        // indexing→indexed transition before the watcher starts.
        let initial_status = match self
            .run_full_sync(repo_path, Arc::clone(&filter), cancel_token.clone())
            .await
        {
            Ok(status) => status,
            Err(e) => {
                log::warn!("[ContextWatcher] Initial full_sync failed for {repo_path}: {e}");
                // Continue anyway — the file watcher will pick up incremental changes
                // and keep retrying. Reflect the error in the cached status.
                IndexWatcherStatus::Error {
                    reason: "sync_failed".to_string(),
                    details: e,
                }
            }
        };

        // Start the file watcher
        let (watcher, batch_rx) =
            FileWatcher::start(repo.clone(), Arc::clone(&filter), cancel_token.clone())
                .map_err(|e| e.to_string())?;

        // Spawn sync worker with an Arc clone of self
        let me = Arc::clone(self);
        let repo_path_owned = repo_path.to_string();
        let cancel_clone = cancel_token.clone();
        let sync_task = tokio::spawn(async move {
            sync_worker(me, repo_path_owned, batch_rx, cancel_clone).await;
        });

        // Store watcher (FileWatcher must be kept alive to hold notify::RecommendedWatcher)
        {
            let mut watchers = self.watchers.write().await;
            watchers.insert(
                repo_path.to_string(),
                RepoWatcher {
                    cancel_token,
                    sync_task,
                    _file_watcher: watcher,
                    status: initial_status,
                },
            );
        }

        self.upsert_repo_db(repo_path);

        log::info!("[ContextWatcher] Now watching {repo_path}");
        Ok(())
    }

    /// Run a full sync against the context engine. Emits status events and
    /// returns the terminal `IndexWatcherStatus` for caching.
    async fn run_full_sync(
        &self,
        repo_path: &str,
        filter: Arc<IgnoreFilter>,
        cancel: CancellationToken,
    ) -> Result<IndexWatcherStatus, String> {
        let streamer = match self.build_streamer(repo_path).await {
            Some(s) => s,
            None => return Err("streamer not available (context engine disabled)".into()),
        };

        let lock = self.get_sync_lock(repo_path).await;
        let _guard = lock.lock().await;

        self.emit_status(repo_path, &IndexWatcherStatus::Indexing);

        let repo_root = Path::new(repo_path);
        match streamer.full_sync(repo_root, filter, cancel).await {
            Ok(stats) => {
                log::info!(
                    "[ContextWatcher] full_sync done repo={} hashed={} uploaded={} deleted={} bytes={} duration={:?}",
                    repo_path,
                    stats.files_hashed,
                    stats.files_uploaded,
                    stats.files_deleted,
                    stats.bytes_uploaded,
                    stats.duration,
                );
                let status = IndexWatcherStatus::Indexed {
                    file_count: Some(stats.files_hashed as u64),
                };
                self.emit_status(repo_path, &status);
                Ok(status)
            }
            Err(SyncError::Cancelled) => {
                // Cancellation is expected during repo-switch / shutdown — not an error.
                // Reset to NotIndexed so the next start_watching retries cleanly, and
                // emit it to clear any "Indexing..." spinner the UI may have cached.
                log::info!("[ContextWatcher] full_sync cancelled for {repo_path}");
                let status = IndexWatcherStatus::NotIndexed;
                self.emit_status(repo_path, &status);
                Ok(status)
            }
            Err(e) => {
                let (reason, details) = classify_sync_error(&e);
                log::warn!(
                    "[ContextWatcher] full_sync failed for {}: {} ({})",
                    repo_path,
                    reason,
                    details
                );
                let status = IndexWatcherStatus::Error {
                    reason: reason.clone(),
                    details: details.clone(),
                };
                self.emit_status(repo_path, &status);
                Err(format!("{reason}: {details}"))
            }
        }
    }

    /// Run an incremental sync triggered by a file-watcher batch. Emits status
    /// events and returns the terminal `IndexWatcherStatus`.
    async fn run_incremental_sync(
        &self,
        repo_path: &str,
        changed: &[PathBuf],
        deleted: &[PathBuf],
        cancel: CancellationToken,
    ) -> IndexWatcherStatus {
        let streamer = match self.build_streamer(repo_path).await {
            Some(s) => s,
            None => {
                return IndexWatcherStatus::Error {
                    reason: "unavailable".into(),
                    details: "streamer not configured".into(),
                }
            }
        };

        let lock = self.get_sync_lock(repo_path).await;
        let _guard = lock.lock().await;

        self.emit_status(repo_path, &IndexWatcherStatus::Indexing);

        let repo_root = Path::new(repo_path);
        match streamer
            .incremental_sync(changed, deleted, repo_root, cancel)
            .await
        {
            Ok(stats) => {
                log::info!(
                    "[ContextWatcher] incremental_sync done repo={} uploaded={} deleted={}",
                    repo_path,
                    stats.files_uploaded,
                    stats.files_deleted
                );
                let status = IndexWatcherStatus::Indexed { file_count: None };
                self.emit_status(repo_path, &status);
                status
            }
            Err(SyncError::Cancelled) => {
                // Cancellation is expected during repo-switch / shutdown. Roll back
                // to Indexed (file_count=None) so the UI clears the "Indexing..." spinner
                // and reflects that the server's index is still live from the last sync.
                log::info!("[ContextWatcher] incremental_sync cancelled for {repo_path}");
                let status = IndexWatcherStatus::Indexed { file_count: None };
                self.emit_status(repo_path, &status);
                status
            }
            Err(e) => {
                let (reason, details) = classify_sync_error(&e);
                log::warn!(
                    "[ContextWatcher] incremental_sync failed for {}: {} ({})",
                    repo_path,
                    reason,
                    details
                );
                let status = IndexWatcherStatus::Error { reason, details };
                self.emit_status(repo_path, &status);
                status
            }
        }
    }

    /// Stop watching a repo. Cancels gracefully and waits up to 5s for
    /// in-flight uploads before hard-aborting the sync task. Also GCs the
    /// per-repo sync_locks entry (see C5).
    pub async fn stop_watching(&self, repo_path: &str) {
        let watcher = {
            let mut watchers = self.watchers.write().await;
            watchers.remove(repo_path)
        };
        if let Some(w) = watcher {
            shutdown_watcher(w, Duration::from_secs(5), repo_path).await;
        }
        // Unconditionally GC the lock map entry — even if no watcher was
        // registered, `get_sync_lock` may have populated it.
        self.sync_locks.write().await.remove(repo_path);
    }

    /// Stop all watchers (app quit). Shutdowns run concurrently with a 1s
    /// grace period per watcher, so total quit time stays bounded regardless
    /// of repo count.
    pub async fn stop_all(&self) {
        let drained: Vec<(String, RepoWatcher)> = {
            let mut watchers = self.watchers.write().await;
            watchers.drain().collect()
        };
        if drained.is_empty() {
            return;
        }
        let mut set: JoinSet<()> = JoinSet::new();
        for (path, w) in drained {
            set.spawn(async move {
                shutdown_watcher(w, Duration::from_secs(1), &path).await;
            });
        }
        while set.join_next().await.is_some() {}
        // Bulk GC all sync_lock entries — stop_all drains every watcher.
        self.sync_locks.write().await.clear();
    }

    /// Get cached status for a repo.
    pub async fn get_status(&self, repo_path: &str) -> Option<IndexWatcherStatus> {
        let watchers = self.watchers.read().await;
        watchers.get(repo_path).map(|w| w.status.clone())
    }

    /// Auto-start watchers for recently-active repos on app launch.
    /// Skips if identity/auth token are not yet set (pre-login) — will be called
    /// again after login via `save_auth_credentials`.
    pub async fn auto_start(self: &Arc<Self>) {
        if !crate::agent_bridge::commands::read_context_engine_db(&self.db).enabled {
            return;
        }

        // Cleanup stale repos
        {
            let conn = self.db.conn.lock();
            let _ = super::db::cleanup_stale_repos(&conn);
        }

        // Get active repos
        let repos = {
            let conn = self.db.conn.lock();
            super::db::get_active_watched_repos(&conn).unwrap_or_default()
        };

        const AUTO_START_CONCURRENCY: usize = 4;
        let sem = Arc::new(Semaphore::new(AUTO_START_CONCURRENCY));
        let mut set = JoinSet::new();

        for repo_path in repos {
            if !Path::new(&repo_path).is_dir() {
                log::info!("[ContextWatcher] Skipping missing repo: {repo_path}");
                continue;
            }
            let mgr = Arc::clone(self);
            let permit = Arc::clone(&sem);
            set.spawn(async move {
                let _permit = permit.acquire().await.expect("semaphore closed");
                if let Err(e) = mgr.start_watching(&repo_path).await {
                    log::warn!("[ContextWatcher] Failed to auto-start for {repo_path}: {e}");
                }
            });
        }

        while set.join_next().await.is_some() {}
    }

    // -----------------------------------------------------------------------
    // Internal helpers
    // -----------------------------------------------------------------------

    fn emit_status(&self, repo_path: &str, status: &IndexWatcherStatus) {
        #[derive(Serialize, Clone)]
        struct StatusEvent {
            repo_path: String,
            #[serde(flatten)]
            status: IndexWatcherStatus,
        }

        let _ = self.app_handle.emit(
            "context-watcher-status",
            StatusEvent {
                repo_path: repo_path.to_string(),
                status: status.clone(),
            },
        );
    }

    fn upsert_repo_db(&self, repo_path: &str) {
        let conn = self.db.conn.lock();
        let _ = super::db::upsert_watched_repo(&conn, repo_path);
    }

    /// Update the cached status for a repo. No-op if the repo isn't being watched.
    async fn update_cached_status(&self, repo_path: &str, status: IndexWatcherStatus) {
        let mut watchers = self.watchers.write().await;
        if let Some(w) = watchers.get_mut(repo_path) {
            w.status = status;
        }
    }
}

// ---------------------------------------------------------------------------
// Shutdown helper
// ---------------------------------------------------------------------------

/// Cancel the watcher's token, wait up to `grace` for a clean exit, then
/// hard-abort the spawned sync task if the grace period elapses. Clones the
/// abort handle before awaiting so the timeout's consumption of the
/// JoinHandle doesn't prevent us from aborting.
async fn shutdown_watcher(w: RepoWatcher, grace: Duration, label: &str) {
    w.cancel_token.cancel();
    let abort_handle = w.sync_task.abort_handle();
    match tokio::time::timeout(grace, w.sync_task).await {
        Ok(Ok(())) => log::info!("[ContextWatcher] {label}: clean shutdown"),
        Ok(Err(e)) if e.is_cancelled() => {
            log::info!("[ContextWatcher] {label}: task cancelled")
        }
        Ok(Err(e)) => log::warn!("[ContextWatcher] {label}: task error: {e}"),
        Err(_) => {
            log::warn!("[ContextWatcher] {label}: grace expired, aborting");
            abort_handle.abort();
        }
    }
}

// ---------------------------------------------------------------------------
// Sync worker (spawned per repo)
// ---------------------------------------------------------------------------

async fn sync_worker(
    manager: Arc<WatcherManager>,
    repo_path: String,
    mut batch_rx: tokio::sync::mpsc::Receiver<ChangeBatch>,
    cancel_token: CancellationToken,
) {
    loop {
        tokio::select! {
            _ = cancel_token.cancelled() => break,
            batch = batch_rx.recv() => {
                let batch = match batch {
                    Some(b) => b,
                    None => break,
                };
                if batch.created_or_modified.is_empty() && batch.deleted.is_empty() {
                    continue;
                }

                // Keep cached status in lockstep with emitted events so that a
                // get_status query mid-sync returns Indexing rather than stale Indexed.
                manager
                    .update_cached_status(&repo_path, IndexWatcherStatus::Indexing)
                    .await;

                let status = manager
                    .run_incremental_sync(
                        &repo_path,
                        &batch.created_or_modified,
                        &batch.deleted,
                        cancel_token.clone(),
                    )
                    .await;
                manager.update_cached_status(&repo_path, status).await;
            }
        }
    }
}

// ---------------------------------------------------------------------------
// Error classification
// ---------------------------------------------------------------------------

/// Map a `SyncError` to a (reason, details) tuple for the `Error` status event.
/// `reason` is a stable machine-readable tag; `details` is human-readable.
fn classify_sync_error(err: &SyncError) -> (String, String) {
    match err {
        SyncError::RepoTooLarge { bytes, max } => (
            "repo_too_large".into(),
            format!("{bytes} bytes (max {max})"),
        ),
        SyncError::TooManyFiles { count, max } => (
            "too_many_files".into(),
            format!("{count} files (max {max})"),
        ),
        SyncError::RequestTooLarge { bytes, max } => (
            "request_too_large".into(),
            format!("{bytes} bytes gzipped (max {max})"),
        ),
        SyncError::Cancelled => ("cancelled".into(), "sync cancelled".into()),
        SyncError::DeadlineExceeded => (
            "deadline_exceeded".into(),
            "sync deadline exceeded".into(),
        ),
        SyncError::SyncExpired { reason } => ("sync_expired".into(), reason.clone()),
        SyncError::BadRequest { reason, details } => {
            (format!("bad_request:{reason}"), details.clone())
        }
        SyncError::AuthRequired { status } => {
            ("auth_required".into(), format!("HTTP {status}"))
        }
        SyncError::Http(e) => ("http_error".into(), e.to_string()),
        SyncError::Io(e) => ("io_error".into(), e.to_string()),
        SyncError::Serialization(e) => ("serialization_error".into(), e.to_string()),
        SyncError::Other(msg) => ("other".into(), msg.clone()),
    }
}
