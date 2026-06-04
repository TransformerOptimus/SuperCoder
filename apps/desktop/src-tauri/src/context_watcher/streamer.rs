//! WS7: Client streaming module that replaces the S3 upload path.
//!
//! Four-phase pipeline against the supercoder context engine:
//!   Phase 0: walk repo, read & hash files (with hard caps)
//!   Phase 1: POST {base}/index/diff       → server returns sync_id + need[] + delete[]
//!   Phase 2: POST {base}/index/stream     → batched NDJSON, parallel, deterministic batch_id
//!   Phase 3: POST {base}/index/sync-complete (poll) → 200/202/409/410
//!
//! See `llm_context/streaming-refactor/07-ws7-client-streamer.md` for the full spec.

use std::collections::HashSet;
use std::path::{Path, PathBuf};
use std::sync::Arc;
use std::time::{Duration, Instant};

use base64::engine::general_purpose::STANDARD as BASE64_STANDARD;
use base64::Engine;
use flate2::write::GzEncoder;
use flate2::Compression;
use reqwest::header::{CONTENT_ENCODING, CONTENT_TYPE};
use reqwest::StatusCode;
use serde::{Deserialize, Serialize};
use sha2::{Digest, Sha256};
use tokio::sync::Semaphore;
use tokio::task::JoinSet;
use tokio_util::sync::CancellationToken;

use super::ignore_filter::IgnoreFilter;

// ────────────────── Constants ──────────────────

const MAX_BATCH_BYTES: usize = 2 * 1024 * 1024; // 2 MB uncompressed
const MAX_BATCH_FILES: usize = 100;
const STREAM_CONCURRENCY: usize = 10;
const SYNC_DEADLINE: Duration = Duration::from_secs(300);
const POLL_INTERVAL: Duration = Duration::from_secs(2);
const MAX_RETRY_ATTEMPTS: u32 = 3;

// Phase 0 hard caps (plan §3.7)
const MAX_FILE_SIZE_BYTES: u64 = 1_048_576; // 1 MB per file
const MAX_REPO_BYTES: u64 = 100 * 1024 * 1024; // 100 MB total
const MAX_REPO_FILES: usize = 20_000;
const MAX_DIFF_BODY_GZ_BYTES: usize = 5 * 1024 * 1024; // 5 MB gzipped

// ────────────────── Public types ──────────────────

#[derive(Debug, Clone)]
pub struct StreamerConfig {
    pub context_engine_url: String,
    pub user_id: String,
    pub workspace_id: u64,
    pub machine_id: String,
    pub repo_path: String,
    /// APISIX gateway auth token (X-Auth-Token header). Empty = no auth.
    pub auth_token: String,
}

#[derive(Debug, Clone, Default)]
pub struct SyncStats {
    pub files_hashed: usize,
    pub files_uploaded: usize,
    pub files_deleted: usize,
    pub bytes_uploaded: u64,
    pub duplicate_batches: usize,
    pub duration: Duration,
}

#[derive(Debug, thiserror::Error)]
pub enum SyncError {
    #[error("repo too large: {bytes} bytes (max {max})")]
    RepoTooLarge { bytes: u64, max: u64 },

    #[error("too many files: {count} (max {max})")]
    TooManyFiles { count: usize, max: usize },

    #[error("sync cancelled")]
    Cancelled,

    #[error("sync deadline exceeded")]
    DeadlineExceeded,

    #[error("server returned 410 Gone: {reason}")]
    SyncExpired { reason: String },

    #[error("server returned 400 Bad Request: {reason} ({details})")]
    BadRequest { reason: String, details: String },

    #[error("diff request too large: {bytes} bytes gzipped (max {max})")]
    RequestTooLarge { bytes: usize, max: usize },

    #[error("auth required: HTTP {status}")]
    AuthRequired { status: u16 },

    #[error("HTTP error: {0}")]
    Http(#[from] reqwest::Error),

    #[error("I/O error: {0}")]
    Io(#[from] std::io::Error),

    #[error("serialization error: {0}")]
    Serialization(#[from] serde_json::Error),

    #[error("other: {0}")]
    Other(String),
}

// ────────────────── Streamer ──────────────────

pub struct Streamer {
    config: StreamerConfig,
    http: reqwest::Client,
    /// Deadline for `/sync-complete` polling. Defaults to `SYNC_DEADLINE`,
    /// overridable in tests via `new_with_timings`.
    sync_deadline: Duration,
    /// Sleep between `/sync-complete` polls. Defaults to `POLL_INTERVAL`,
    /// overridable in tests via `new_with_timings`.
    poll_interval: Duration,
}

impl Streamer {
    pub fn new(config: StreamerConfig) -> Self {
        let http = reqwest::Client::builder()
            .connect_timeout(Duration::from_secs(10))
            .timeout(Duration::from_secs(60))
            .build()
            .expect("reqwest client");
        Self {
            config,
            http,
            sync_deadline: SYNC_DEADLINE,
            poll_interval: POLL_INTERVAL,
        }
    }

    /// Test-only constructor that allows overriding the polling timings so
    /// tests don't have to wait minutes.
    #[cfg(test)]
    pub(crate) fn new_with_timings(
        config: StreamerConfig,
        sync_deadline: Duration,
        poll_interval: Duration,
    ) -> Self {
        let mut s = Self::new(config);
        s.sync_deadline = sync_deadline;
        s.poll_interval = poll_interval;
        s
    }

    /// Run a full sync: walk the tree, hash everything, call /diff, stream all needed
    /// batches, poll /sync-complete. Sends `incremental=false` so the server computes
    /// deletes by comparing the client's complete view against the Merkle tree.
    pub async fn full_sync(
        &self,
        repo_root: &Path,
        filter: Arc<IgnoreFilter>,
        cancel: CancellationToken,
    ) -> Result<SyncStats, SyncError> {
        let started = Instant::now();
        let mut stats = SyncStats::default();

        // Phase 0 — walk + hash. `walk_and_hash` does synchronous filesystem
        // I/O (ignore::WalkBuilder + std::fs::read) which would block a Tokio
        // worker thread for seconds on large repos, so we offload it to the
        // blocking pool. The CancellationToken is checked per-entry inside
        // the walk, so early cancellation still works.
        let repo_root_owned = repo_root.to_path_buf();
        let filter_for_walk = Arc::clone(&filter);
        let cancel_for_walk = cancel.clone();
        let entries = tokio::task::spawn_blocking(move || {
            walk_and_hash(
                &repo_root_owned,
                &filter_for_walk,
                &cancel_for_walk,
                MAX_FILE_SIZE_BYTES,
                MAX_REPO_BYTES,
                MAX_REPO_FILES,
            )
        })
        .await
        .map_err(|e| SyncError::Other(format!("walk_and_hash join: {e}")))??;
        stats.files_hashed = entries.len();

        // Phase 1
        let diff_response = self.call_diff(&entries, false, &[], &cancel).await?;
        log::info!(
            "[streamer] /diff sync_id={} need={} delete={}",
            diff_response.sync_id,
            diff_response.need.len(),
            diff_response.delete.len()
        );

        if diff_response.need.is_empty() && diff_response.delete.is_empty() {
            stats.duration = started.elapsed();
            return Ok(stats);
        }

        // Phase 2
        let needed = entries_filtered_by_need(&entries, &diff_response.need);
        let batch_stats = self
            .stream_batches(
                &diff_response.sync_id,
                needed,
                diff_response.delete.clone(),
                &cancel,
            )
            .await?;
        stats.files_uploaded = batch_stats.files_uploaded;
        stats.files_deleted = batch_stats.files_deleted;
        stats.bytes_uploaded = batch_stats.bytes_uploaded;
        stats.duplicate_batches = batch_stats.duplicate_batches;

        // Phase 3
        self.poll_sync_complete(&diff_response.sync_id, &cancel)
            .await?;

        stats.duration = started.elapsed();
        Ok(stats)
    }

    /// Incremental sync from the file watcher. `changed` is the list of files reported
    /// as added/modified; `deleted` is the explicit list of files reported as removed.
    ///
    /// Sends `incremental=true` + the explicit `deleted` list so the server does NOT
    /// infer deletes from "every Merkle path missing from this request" (which would
    /// otherwise wipe the index every keystroke). See WS3 §1 for the server contract.
    pub async fn incremental_sync(
        &self,
        changed: &[PathBuf],
        deleted: &[PathBuf],
        repo_root: &Path,
        cancel: CancellationToken,
    ) -> Result<SyncStats, SyncError> {
        let started = Instant::now();
        let mut stats = SyncStats::default();

        let mut entries = Vec::with_capacity(changed.len());
        for path in changed {
            if cancel.is_cancelled() {
                return Err(SyncError::Cancelled);
            }
            // Skip directories — file watchers emit events for directory creation
            // (e.g. `mkdir foo`) and reading a directory with fs::read returns
            // `EISDIR (21)` on Unix / ACCESS_DENIED on Windows, which would fail
            // the entire sync.
            if !path.is_file() {
                log::debug!("[streamer] skipping non-file: {}", path.display());
                continue;
            }
            let rel_path = path
                .strip_prefix(repo_root)
                .map(|p| p.to_string_lossy().to_string())
                .unwrap_or_else(|_| path.to_string_lossy().to_string());
            let content = match tokio::fs::read(path).await {
                Ok(c) => c,
                Err(err)
                    if err.kind() == std::io::ErrorKind::NotFound
                        || err.kind() == std::io::ErrorKind::PermissionDenied
                        || err.raw_os_error() == Some(21)   // EISDIR
                        || err.raw_os_error() == Some(26)   // ETXTBSY (Linux) — busy executable
                        || err.raw_os_error() == Some(32) => // ERROR_SHARING_VIOLATION (Windows)
                {
                    log::debug!("[streamer] skipping {}: {}", path.display(), err);
                    continue;
                }
                Err(err) => return Err(SyncError::Io(err)),
            };
            if content.len() as u64 > MAX_FILE_SIZE_BYTES {
                continue;
            }
            // See `walk_and_hash` for the rationale: non-UTF-8 content would be
            // mangled by `String::from_utf8_lossy` in `build_batch_ndjson` and
            // fail server-side hash validation.
            if std::str::from_utf8(&content).is_err() {
                log::debug!(
                    "[streamer] skipping non-UTF-8 file: {}",
                    path.display()
                );
                continue;
            }
            let hash = sha256_hex(&content);
            entries.push(FileEntry {
                path: rel_path,
                hash,
                content,
            });
        }
        stats.files_hashed = entries.len();

        let delete_strs: Vec<String> = deleted
            .iter()
            .map(|p| {
                p.strip_prefix(repo_root)
                    .map(|pp| pp.to_string_lossy().to_string())
                    .unwrap_or_else(|_| p.to_string_lossy().to_string())
            })
            .collect();

        let diff_response = self.call_diff(&entries, true, &delete_strs, &cancel).await?;

        if diff_response.need.is_empty() && diff_response.delete.is_empty() {
            stats.duration = started.elapsed();
            return Ok(stats);
        }

        let needed = entries_filtered_by_need(&entries, &diff_response.need);
        let batch_stats = self
            .stream_batches(
                &diff_response.sync_id,
                needed,
                diff_response.delete.clone(),
                &cancel,
            )
            .await?;
        stats.files_uploaded = batch_stats.files_uploaded;
        stats.files_deleted = batch_stats.files_deleted;
        stats.bytes_uploaded = batch_stats.bytes_uploaded;
        stats.duplicate_batches = batch_stats.duplicate_batches;

        self.poll_sync_complete(&diff_response.sync_id, &cancel)
            .await?;

        stats.duration = started.elapsed();
        Ok(stats)
    }

    // ────────────────── Phase 1: /diff ──────────────────

    async fn call_diff(
        &self,
        entries: &[FileEntry],
        incremental: bool,
        deletes: &[String],
        cancel: &CancellationToken,
    ) -> Result<DiffResponse, SyncError> {
        if cancel.is_cancelled() {
            return Err(SyncError::Cancelled);
        }

        let files: Vec<DiffFileHash> = entries
            .iter()
            .map(|e| DiffFileHash {
                path: e.path.clone(),
                sha256: e.hash.clone(),
            })
            .collect();

        let req = DiffRequest {
            user_id: self.config.user_id.clone(),
            workspace_id: self.config.workspace_id,
            machine_id: self.config.machine_id.clone(),
            repo_path: self.config.repo_path.clone(),
            github_org_id: None,
            files,
            incremental,
            deletes: deletes.to_vec(),
        };

        let body_json = serde_json::to_vec(&req)?;
        let body_gz = gzip_encode(&body_json)?;

        if body_gz.len() > MAX_DIFF_BODY_GZ_BYTES {
            return Err(SyncError::RequestTooLarge {
                bytes: body_gz.len(),
                max: MAX_DIFF_BODY_GZ_BYTES,
            });
        }

        let url = format!("{}/index/diff", self.config.context_engine_url);

        // Retry loop: 429 from the server means another client is currently
        // syncing the same identity. Transient, self-resolves — back off and
        // retry up to MAX_DIFF_RETRY times before surfacing as a user-visible
        // error. 5xx and other transport errors still propagate immediately
        // (reqwest handles those; we only retry the explicit 429 here).
        const MAX_DIFF_RETRY: u32 = 3;
        let mut attempt: u32 = 0;
        loop {
            if cancel.is_cancelled() {
                return Err(SyncError::Cancelled);
            }
            let response = self
                .http
                .post(&url)
                .header("X-USER-ID", &self.config.user_id)
                .header("X-Workspace-ID", self.config.workspace_id.to_string())
                .header("X-Auth-Token", &self.config.auth_token)
                .header(CONTENT_TYPE, "application/json")
                .header(CONTENT_ENCODING, "gzip")
                .body(body_gz.clone())
                .send()
                .await?;

            match response.status() {
                StatusCode::OK => return Ok(response.json::<DiffResponse>().await?),
                StatusCode::TOO_MANY_REQUESTS if attempt < MAX_DIFF_RETRY => {
                    attempt += 1;
                    let backoff = Duration::from_secs(2u64.saturating_pow(attempt));
                    log::warn!(
                        "[streamer] /diff 429 (attempt {}/{}), backoff {:?}",
                        attempt, MAX_DIFF_RETRY, backoff
                    );
                    tokio::select! {
                        _ = cancel.cancelled() => return Err(SyncError::Cancelled),
                        _ = tokio::time::sleep(backoff) => {}
                    }
                    continue;
                }
                StatusCode::TOO_MANY_REQUESTS => {
                    return Err(SyncError::Other("concurrent sync in progress".into()));
                }
                StatusCode::UNAUTHORIZED | StatusCode::FORBIDDEN => {
                    return Err(SyncError::AuthRequired {
                        status: response.status().as_u16(),
                    });
                }
                status => {
                    let body = response.text().await.unwrap_or_default();
                    return Err(SyncError::Other(format!(
                        "unexpected /diff response {}: {}",
                        status, body
                    )));
                }
            }
        }
    }

    // ────────────────── Phase 2: /stream ──────────────────

    async fn stream_batches(
        &self,
        sync_id: &str,
        needed: Vec<FileEntry>,
        deletes: Vec<String>,
        cancel: &CancellationToken,
    ) -> Result<BatchStats, SyncError> {
        let batches = chunk_into_batches(needed, deletes);
        log::info!("[streamer] stream_batches: {} batches", batches.len());

        let sem = Arc::new(Semaphore::new(STREAM_CONCURRENCY));
        let mut joinset: JoinSet<Result<SingleBatchOutcome, SyncError>> = JoinSet::new();
        let mut stats = BatchStats::default();

        // Per-call child token. Scope cancellation to this single stream_batches
        // invocation so early-return (parent cancel or batch error) can cancel
        // every in-flight upload without killing the repo-level parent token
        // (which is reused across future syncs).
        let child_cancel = cancel.child_token();

        for batch in batches {
            if cancel.is_cancelled() {
                abort_and_drain(&mut joinset, &child_cancel).await;
                return Err(SyncError::Cancelled);
            }
            let permit = sem.clone().acquire_owned().await.unwrap();
            let sync_id = sync_id.to_string();
            let streamer = self.clone_for_task();
            let task_cancel = child_cancel.clone();
            joinset.spawn(async move {
                let _permit = permit;
                streamer
                    .stream_one_batch(&sync_id, batch, &task_cancel)
                    .await
            });
        }

        while let Some(join_res) = joinset.join_next().await {
            match join_res {
                Ok(Ok(outcome)) => {
                    stats.files_uploaded += outcome.files_uploaded;
                    stats.files_deleted += outcome.files_deleted;
                    stats.bytes_uploaded += outcome.bytes_uploaded;
                    if outcome.duplicate {
                        stats.duplicate_batches += 1;
                    }
                }
                Ok(Err(err)) => {
                    abort_and_drain(&mut joinset, &child_cancel).await;
                    return Err(err);
                }
                Err(join_err) => {
                    abort_and_drain(&mut joinset, &child_cancel).await;
                    return Err(SyncError::Other(format!("join: {join_err}")));
                }
            }
        }

        Ok(stats)
    }

    async fn stream_one_batch(
        &self,
        sync_id: &str,
        batch: Batch,
        cancel: &CancellationToken,
    ) -> Result<SingleBatchOutcome, SyncError> {
        let batch_id = deterministic_batch_id(&batch);
        let ndjson = build_batch_ndjson(&batch);
        let body_gz = gzip_encode(&ndjson)?;

        let url = format!("{}/index/stream", self.config.context_engine_url);

        let mut attempt = 0u32;
        loop {
            if cancel.is_cancelled() {
                return Err(SyncError::Cancelled);
            }

            let response = self
                .http
                .post(&url)
                .header("X-USER-ID", &self.config.user_id)
                .header("X-Workspace-ID", self.config.workspace_id.to_string())
                .header("X-Auth-Token", &self.config.auth_token)
                .header("X-Sync-Id", sync_id)
                .header("X-Batch-Id", &batch_id)
                .header("X-Batch-Seq", "0")
                .header(CONTENT_TYPE, "application/x-ndjson")
                .header(CONTENT_ENCODING, "gzip")
                .body(body_gz.clone())
                .send()
                .await;

            match response {
                Ok(resp) if resp.status() == StatusCode::ACCEPTED => {
                    let parsed: StreamBatchResponse = resp.json().await?;
                    let queued = parsed.queued.unwrap_or_default();
                    return Ok(SingleBatchOutcome {
                        files_uploaded: queued.files,
                        files_deleted: queued.deletes,
                        bytes_uploaded: queued.bytes,
                        duplicate: false,
                    });
                }
                Ok(resp) if resp.status() == StatusCode::OK => {
                    return Ok(SingleBatchOutcome {
                        files_uploaded: 0,
                        files_deleted: 0,
                        bytes_uploaded: 0,
                        duplicate: true,
                    });
                }
                Ok(resp) if resp.status() == StatusCode::GONE => {
                    let body = resp.text().await.unwrap_or_default();
                    return Err(SyncError::SyncExpired { reason: body });
                }
                Ok(resp) if resp.status() == StatusCode::BAD_REQUEST => {
                    let body: serde_json::Value = resp.json().await.unwrap_or_default();
                    let reason = body
                        .get("error")
                        .and_then(|v| v.as_str())
                        .unwrap_or("unknown")
                        .to_string();
                    let details = body
                        .get("details")
                        .and_then(|v| v.as_str())
                        .unwrap_or("")
                        .to_string();
                    return Err(SyncError::BadRequest { reason, details });
                }
                Ok(resp)
                    if matches!(
                        resp.status(),
                        StatusCode::UNAUTHORIZED | StatusCode::FORBIDDEN | StatusCode::NOT_FOUND
                    ) =>
                {
                    return Err(SyncError::AuthRequired {
                        status: resp.status().as_u16(),
                    });
                }
                Ok(resp) => {
                    let status = resp.status();
                    if attempt < MAX_RETRY_ATTEMPTS {
                        attempt += 1;
                        let backoff = Duration::from_millis(100u64 * (1 << attempt));
                        tokio::time::sleep(backoff).await;
                        continue;
                    }
                    return Err(SyncError::Other(format!("HTTP {}", status)));
                }
                Err(e) if e.is_timeout() || e.is_connect() => {
                    if attempt < MAX_RETRY_ATTEMPTS {
                        attempt += 1;
                        let backoff = Duration::from_millis(100u64 * (1 << attempt));
                        tokio::time::sleep(backoff).await;
                        continue;
                    }
                    return Err(SyncError::Http(e));
                }
                Err(e) => return Err(SyncError::Http(e)),
            }
        }
    }

    // ────────────────── Phase 3: /sync-complete ──────────────────

    async fn poll_sync_complete(
        &self,
        sync_id: &str,
        cancel: &CancellationToken,
    ) -> Result<(), SyncError> {
        let deadline = Instant::now() + self.sync_deadline;
        let url = format!("{}/index/sync-complete", self.config.context_engine_url);

        loop {
            if cancel.is_cancelled() {
                return Err(SyncError::Cancelled);
            }
            if Instant::now() >= deadline {
                // All batches were already accepted by the server (we only reach
                // poll_sync_complete after stream_batches returned Ok). Exceeding
                // the poll deadline doesn't mean the data is lost — the server is
                // just slow to finalize. Treat as success with a warning.
                log::warn!(
                    "[streamer] sync-complete poll deadline exceeded for sync_id={} \
                     — batches were delivered; treating as success",
                    sync_id
                );
                return Ok(());
            }

            let body = serde_json::json!({ "sync_id": sync_id }).to_string();
            let response = self
                .http
                .post(&url)
                .header("X-USER-ID", &self.config.user_id)
                .header("X-Workspace-ID", self.config.workspace_id.to_string())
                .header("X-Auth-Token", &self.config.auth_token)
                .header(CONTENT_TYPE, "application/json")
                .body(body)
                .send()
                .await?;

            match response.status() {
                StatusCode::OK => {
                    log::info!("[streamer] sync complete sync_id={}", sync_id);
                    return Ok(());
                }
                StatusCode::ACCEPTED | StatusCode::CONFLICT => {
                    tokio::select! {
                        _ = cancel.cancelled() => return Err(SyncError::Cancelled),
                        _ = tokio::time::sleep(self.poll_interval) => {}
                    }
                    continue;
                }
                StatusCode::GONE => {
                    // Server already finalized and cleaned up the sync_id record.
                    // This is a success for /sync-complete (unlike /stream, where
                    // 410 means the sync was aborted). The data IS indexed.
                    let body = response.text().await.unwrap_or_default();
                    log::info!(
                        "[streamer] sync_id={} already finalized (410): {}",
                        sync_id, body
                    );
                    return Ok(());
                }
                status => {
                    let body = response.text().await.unwrap_or_default();
                    return Err(SyncError::Other(format!(
                        "unexpected /sync-complete response {}: {}",
                        status, body
                    )));
                }
            }
        }
    }

    fn clone_for_task(&self) -> Self {
        Self {
            config: self.config.clone(),
            http: self.http.clone(),
            sync_deadline: self.sync_deadline,
            poll_interval: self.poll_interval,
        }
    }
}

/// Cancel the scoped child token, hard-abort every remaining task in the
/// `JoinSet`, and drain it so no detached tasks survive the early return.
/// See C1 in `llm_context/streaming-refactor/followups-prioritized.md`.
async fn abort_and_drain(
    joinset: &mut JoinSet<Result<SingleBatchOutcome, SyncError>>,
    child: &CancellationToken,
) {
    child.cancel();
    joinset.abort_all();
    while joinset.join_next().await.is_some() {}
}

// ────────────────── Internal types ──────────────────

#[derive(Debug, Clone)]
struct FileEntry {
    path: String,
    hash: String,
    content: Vec<u8>,
}

#[derive(Debug)]
struct Batch {
    files: Vec<FileEntry>,
    deletes: Vec<String>,
}

#[derive(Default)]
struct BatchStats {
    files_uploaded: usize,
    files_deleted: usize,
    bytes_uploaded: u64,
    duplicate_batches: usize,
}

struct SingleBatchOutcome {
    files_uploaded: usize,
    files_deleted: usize,
    bytes_uploaded: u64,
    duplicate: bool,
}

#[derive(Serialize)]
struct DiffRequest {
    // Identity MUST live in the request body (WS3 decision #0 in
    // `llm_context/streaming-refactor/ws-takeways.md`). The server's
    // `dto.IndexRequest` reads these from the JSON body, not from
    // X-USER-ID / X-Workspace-ID headers. We still send the headers
    // on the HTTP request for forward-compat, but the body is what
    // the supercoder `/diff` handler actually validates against.
    user_id: String,
    workspace_id: u64,
    machine_id: String,
    repo_path: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    github_org_id: Option<String>,
    files: Vec<DiffFileHash>,
    // Defaults are correct for full_sync (false / empty), so the wire format
    // for an existing full sync still default-omits both fields.
    #[serde(skip_serializing_if = "std::ops::Not::not")]
    incremental: bool,
    #[serde(skip_serializing_if = "Vec::is_empty")]
    deletes: Vec<String>,
}

#[derive(Serialize)]
struct DiffFileHash {
    path: String,
    sha256: String,
}

#[derive(Deserialize)]
struct DiffResponse {
    sync_id: String,
    need: Vec<String>,
    delete: Vec<String>,
}

#[derive(Deserialize)]
struct StreamBatchResponse {
    queued: Option<QueuedCounts>,
    #[serde(default)]
    #[allow(dead_code)]
    duplicate: bool,
}

#[derive(Deserialize, Default)]
struct QueuedCounts {
    files: usize,
    deletes: usize,
    bytes: u64,
}

// ────────────────── Helpers ──────────────────

fn sha256_hex(data: &[u8]) -> String {
    let mut hasher = Sha256::new();
    hasher.update(data);
    format!("{:x}", hasher.finalize())
}

fn gzip_encode(data: &[u8]) -> Result<Vec<u8>, std::io::Error> {
    use std::io::Write;
    let mut encoder = GzEncoder::new(Vec::new(), Compression::default());
    encoder.write_all(data)?;
    encoder.finish()
}

/// Walk + read + hash, with overridable caps. Public path uses module constants;
/// tests pass smaller limits to keep test repos cheap.
fn walk_and_hash(
    repo_root: &Path,
    filter: &IgnoreFilter,
    cancel: &CancellationToken,
    max_file_bytes: u64,
    max_repo_bytes: u64,
    max_repo_files: usize,
) -> Result<Vec<FileEntry>, SyncError> {
    let mut entries = Vec::new();
    let mut total_bytes: u64 = 0;

    let walker = ignore::WalkBuilder::new(repo_root).build();
    for result in walker {
        if cancel.is_cancelled() {
            return Err(SyncError::Cancelled);
        }

        let entry = match result {
            Ok(e) => e,
            Err(err) => {
                log::warn!("[streamer] walk error: {err}");
                continue;
            }
        };
        let path = entry.path();
        if !path.is_file() || !filter.should_include(path) {
            continue;
        }

        let metadata = match std::fs::metadata(path) {
            Ok(m) => m,
            Err(_) => continue,
        };
        if metadata.len() > max_file_bytes {
            continue;
        }

        let content = match std::fs::read(path) {
            Ok(c) => c,
            Err(e)
                if e.kind() == std::io::ErrorKind::NotFound
                    || e.kind() == std::io::ErrorKind::PermissionDenied
                    || e.raw_os_error() == Some(21)   // EISDIR
                    || e.raw_os_error() == Some(26)   // ETXTBSY (Linux) — busy executable
                    || e.raw_os_error() == Some(32) => // ERROR_SHARING_VIOLATION (Windows)
            {
                log::debug!("[streamer] skipping {}: {}", path.display(), e);
                continue;
            }
            Err(e) => return Err(SyncError::Io(e)),
        };

        // Skip non-UTF-8 files: the wire format encodes `content` as a JSON string
        // (built via `String::from_utf8_lossy` in `build_batch_ndjson`). Any lossy
        // replacement would change the bytes the server hashes, causing the WS4
        // stale-content check to reject the batch with `400 sha256_mismatch`. The
        // `IgnoreFilter` already excludes the common binary extensions; this is a
        // belt-and-suspenders guard for non-UTF-8 text files (Latin-1, etc.).
        if std::str::from_utf8(&content).is_err() {
            log::debug!(
                "[streamer] skipping non-UTF-8 file: {}",
                path.display()
            );
            continue;
        }

        total_bytes += content.len() as u64;

        if total_bytes > max_repo_bytes {
            return Err(SyncError::RepoTooLarge {
                bytes: total_bytes,
                max: max_repo_bytes,
            });
        }
        if entries.len() >= max_repo_files {
            return Err(SyncError::TooManyFiles {
                count: entries.len() + 1,
                max: max_repo_files,
            });
        }

        let hash = sha256_hex(&content);
        let rel_path = path
            .strip_prefix(repo_root)
            .unwrap_or(path)
            .to_string_lossy()
            .to_string();
        entries.push(FileEntry {
            path: rel_path,
            hash,
            content,
        });
    }

    log::info!(
        "[streamer] walk_and_hash: {} files, {} bytes",
        entries.len(),
        total_bytes
    );
    Ok(entries)
}

/// Greedy chunker: pack files into batches respecting MAX_BATCH_FILES and MAX_BATCH_BYTES.
/// Deletes are tacked onto the first batch (or a delete-only batch if there are no files).
fn chunk_into_batches(needed: Vec<FileEntry>, deletes: Vec<String>) -> Vec<Batch> {
    let mut batches: Vec<Batch> = Vec::new();
    let mut current = Batch {
        files: Vec::new(),
        deletes: Vec::new(),
    };
    let mut current_bytes: usize = 0;

    for entry in needed {
        let entry_size = entry.content.len();
        if !current.files.is_empty()
            && (current.files.len() >= MAX_BATCH_FILES
                || current_bytes + entry_size > MAX_BATCH_BYTES)
        {
            batches.push(current);
            current = Batch {
                files: Vec::new(),
                deletes: Vec::new(),
            };
            current_bytes = 0;
        }
        current_bytes += entry_size;
        current.files.push(entry);
    }
    if !current.files.is_empty() {
        batches.push(current);
    }

    if let Some(first) = batches.first_mut() {
        first.deletes = deletes;
    } else if !deletes.is_empty() {
        batches.push(Batch {
            files: Vec::new(),
            deletes,
        });
    }

    batches
}

/// Build the canonical batch entry list and compute the deterministic batch_id.
/// MUST match the server-side recomputation in WS4 §5.2 byte-for-byte.
fn deterministic_batch_id(batch: &Batch) -> String {
    let mut entries: Vec<String> = Vec::new();
    for f in &batch.files {
        entries.push(format!("file|{}|{}", f.path, f.hash));
    }
    for d in &batch.deletes {
        entries.push(format!("delete|{}|", d));
    }
    entries.sort();
    sha256_hex(entries.join("\n").as_bytes())
}

fn build_batch_ndjson(batch: &Batch) -> Vec<u8> {
    let mut out = Vec::new();
    for f in &batch.files {
        // Server expects base64-encoded bytes so the wire format is binary-safe
        // (matches `services/supercoder/scripts/smoke_index_routes.py` and the
        // WS6 #17 reference wire format). Raw UTF-8 strings get rejected with
        // `illegal base64 data at input byte N` by the server's decoder.
        let line = serde_json::json!({
            "op": "file",
            "path": f.path,
            "sha256": f.hash,
            "content": BASE64_STANDARD.encode(&f.content),
        });
        out.extend_from_slice(serde_json::to_vec(&line).unwrap().as_slice());
        out.push(b'\n');
    }
    for d in &batch.deletes {
        let line = serde_json::json!({ "op": "delete", "path": d });
        out.extend_from_slice(serde_json::to_vec(&line).unwrap().as_slice());
        out.push(b'\n');
    }
    out
}

fn entries_filtered_by_need(entries: &[FileEntry], need: &[String]) -> Vec<FileEntry> {
    let need_set: HashSet<&String> = need.iter().collect();
    entries
        .iter()
        .filter(|e| need_set.contains(&e.path))
        .cloned()
        .collect()
}

// ────────────────── Tests ──────────────────

#[cfg(test)]
mod tests {
    use super::*;
    use flate2::read::GzDecoder;
    use std::fs;
    use std::io::Read;
    use std::sync::Arc;
    use tempfile::TempDir;
    use wiremock::matchers::{header, method, path};
    use wiremock::{Mock, MockServer, Request, Respond, ResponseTemplate};

    // ─── helpers ───

    fn make_filter(dir: &Path) -> Arc<IgnoreFilter> {
        Arc::new(IgnoreFilter::new(dir))
    }

    fn make_config(url: String) -> StreamerConfig {
        StreamerConfig {
            context_engine_url: url,
            user_id: "user-1".into(),
            workspace_id: 42,
            machine_id: "machine-1".into(),
            repo_path: "/repo".into(),
            auth_token: String::new(),
        }
    }

    fn gunzip(data: &[u8]) -> Vec<u8> {
        let mut decoder = GzDecoder::new(data);
        let mut out = Vec::new();
        decoder.read_to_end(&mut out).unwrap();
        out
    }

    fn make_entry(path: &str, content: &[u8]) -> FileEntry {
        FileEntry {
            path: path.into(),
            hash: sha256_hex(content),
            content: content.to_vec(),
        }
    }

    // ─── pure-function tests ───

    #[test]
    fn test_walk_and_hash_respects_caps() {
        let tmp = TempDir::new().unwrap();
        for i in 0..15 {
            fs::write(tmp.path().join(format!("f{i}.txt")), b"x").unwrap();
        }
        let filter = IgnoreFilter::new(tmp.path());
        let err = walk_and_hash(
            tmp.path(),
            &filter,
            &CancellationToken::new(),
            1024,
            1_000_000,
            10, // cap at 10 files
        )
        .unwrap_err();
        match err {
            SyncError::TooManyFiles { max, .. } => assert_eq!(max, 10),
            other => panic!("expected TooManyFiles, got {other:?}"),
        }
    }

    #[test]
    fn test_walk_and_hash_respects_byte_cap() {
        let tmp = TempDir::new().unwrap();
        // 5 files * 1000 bytes = 5000 bytes total, cap at 2000
        for i in 0..5 {
            fs::write(tmp.path().join(format!("f{i}.txt")), vec![b'x'; 1000]).unwrap();
        }
        let filter = IgnoreFilter::new(tmp.path());
        let err = walk_and_hash(
            tmp.path(),
            &filter,
            &CancellationToken::new(),
            10_000,
            2_000,
            1000,
        )
        .unwrap_err();
        match err {
            SyncError::RepoTooLarge { max, .. } => assert_eq!(max, 2_000),
            other => panic!("expected RepoTooLarge, got {other:?}"),
        }
    }

    #[test]
    fn test_walk_and_hash_skips_oversized_files() {
        let tmp = TempDir::new().unwrap();
        fs::write(tmp.path().join("small.txt"), b"hello").unwrap();
        fs::write(tmp.path().join("big.txt"), vec![b'x'; 5000]).unwrap();
        let filter = IgnoreFilter::new(tmp.path());
        let entries = walk_and_hash(
            tmp.path(),
            &filter,
            &CancellationToken::new(),
            1000, // per-file cap below "big.txt"
            1_000_000,
            1000,
        )
        .unwrap();
        let names: Vec<&str> = entries.iter().map(|e| e.path.as_str()).collect();
        assert!(names.contains(&"small.txt"));
        assert!(!names.contains(&"big.txt"));
    }

    #[test]
    fn test_walk_and_hash_respects_ignore_filter() {
        let tmp = TempDir::new().unwrap();
        fs::create_dir_all(tmp.path().join("node_modules")).unwrap();
        fs::create_dir_all(tmp.path().join(".git")).unwrap();
        fs::write(tmp.path().join("node_modules/foo.js"), b"x").unwrap();
        fs::write(tmp.path().join(".git/config"), b"x").unwrap();
        fs::write(tmp.path().join("image.png"), b"x").unwrap();
        fs::write(tmp.path().join("good.rs"), b"fn main() {}").unwrap();

        let filter = IgnoreFilter::new(tmp.path());
        let entries = walk_and_hash(
            tmp.path(),
            &filter,
            &CancellationToken::new(),
            1_000_000,
            1_000_000,
            1000,
        )
        .unwrap();
        let names: Vec<&str> = entries.iter().map(|e| e.path.as_str()).collect();
        assert!(names.contains(&"good.rs"));
        assert!(!names.iter().any(|n| n.contains("node_modules")));
        assert!(!names.iter().any(|n| n.contains(".git/")));
        assert!(!names.contains(&"image.png"));
    }

    #[test]
    fn test_walk_and_hash_skips_non_utf8_files() {
        let tmp = TempDir::new().unwrap();
        // Latin-1 byte 0xff is invalid UTF-8 on its own.
        fs::write(tmp.path().join("bad.txt"), [b'h', b'i', 0xff]).unwrap();
        fs::write(tmp.path().join("good.txt"), b"hello").unwrap();
        let filter = IgnoreFilter::new(tmp.path());
        let entries = walk_and_hash(
            tmp.path(),
            &filter,
            &CancellationToken::new(),
            1_000_000,
            1_000_000,
            1000,
        )
        .unwrap();
        let names: Vec<&str> = entries.iter().map(|e| e.path.as_str()).collect();
        assert!(names.contains(&"good.txt"));
        assert!(!names.contains(&"bad.txt"));
    }

    #[test]
    fn test_walk_and_hash_hash_correctness() {
        let tmp = TempDir::new().unwrap();
        fs::write(tmp.path().join("a.txt"), b"hello world").unwrap();
        let filter = IgnoreFilter::new(tmp.path());
        let entries = walk_and_hash(
            tmp.path(),
            &filter,
            &CancellationToken::new(),
            1_000_000,
            1_000_000,
            1000,
        )
        .unwrap();
        let entry = entries.iter().find(|e| e.path == "a.txt").unwrap();
        // sha256("hello world") known constant
        assert_eq!(
            entry.hash,
            "b94d27b9934d3e08a52e52d7da7dabfac484efe37a5380ee9088f7ace2efcde9"
        );
    }

    #[test]
    fn test_deterministic_batch_id_matches_server_format() {
        // Pinned digest. Server-side `computeBatchID` (WS4 §5.2) MUST produce the same
        // value for the same input. The expected hash here is computed as:
        //   sorted(["delete|old.rs|", "file|src/a.rs|aaaa"]).join("\n")
        // = "delete|old.rs|\nfile|src/a.rs|aaaa"
        // sha256 of that → expected below.
        let batch = Batch {
            files: vec![FileEntry {
                path: "src/a.rs".into(),
                hash: "aaaa".into(),
                content: b"ignored for batch_id".to_vec(),
            }],
            deletes: vec!["old.rs".into()],
        };
        let id = deterministic_batch_id(&batch);
        let expected = sha256_hex(b"delete|old.rs|\nfile|src/a.rs|aaaa");
        assert_eq!(id, expected);
    }

    #[test]
    fn test_deterministic_batch_id_different_hashes_produce_different_ids() {
        let b1 = Batch {
            files: vec![make_entry("a.rs", b"hello")],
            deletes: vec![],
        };
        let b2 = Batch {
            files: vec![make_entry("a.rs", b"world")],
            deletes: vec![],
        };
        assert_ne!(deterministic_batch_id(&b1), deterministic_batch_id(&b2));
    }

    #[test]
    fn test_deterministic_batch_id_order_independent() {
        let b1 = Batch {
            files: vec![make_entry("a.rs", b"x"), make_entry("b.rs", b"y")],
            deletes: vec!["d1.rs".into(), "d2.rs".into()],
        };
        let b2 = Batch {
            files: vec![make_entry("b.rs", b"y"), make_entry("a.rs", b"x")],
            deletes: vec!["d2.rs".into(), "d1.rs".into()],
        };
        assert_eq!(deterministic_batch_id(&b1), deterministic_batch_id(&b2));
    }

    #[test]
    fn test_chunk_into_batches_respects_file_cap() {
        let entries: Vec<FileEntry> = (0..250)
            .map(|i| make_entry(&format!("f{i}.rs"), b"x"))
            .collect();
        let batches = chunk_into_batches(entries, vec![]);
        assert_eq!(batches.len(), 3);
        assert_eq!(batches[0].files.len(), 100);
        assert_eq!(batches[1].files.len(), 100);
        assert_eq!(batches[2].files.len(), 50);
    }

    #[test]
    fn test_chunk_into_batches_respects_byte_cap() {
        let payload = vec![b'x'; 512 * 1024]; // 512 KB each
        let entries: Vec<FileEntry> = (0..10)
            .map(|i| make_entry(&format!("f{i}.rs"), &payload))
            .collect();
        let batches = chunk_into_batches(entries, vec![]);
        // 10 × 512 KB = 5 MB; 2 MB cap → 4 files per batch (4×512KB=2MB) → 3 batches
        assert!(batches.len() >= 3);
        for b in &batches {
            let total: usize = b.files.iter().map(|e| e.content.len()).sum();
            assert!(total <= MAX_BATCH_BYTES);
        }
    }

    #[test]
    fn test_chunk_into_batches_places_deletes() {
        // With files: deletes attach to the first batch
        let entries = vec![make_entry("a.rs", b"x")];
        let deletes = vec!["old.rs".into()];
        let batches = chunk_into_batches(entries, deletes.clone());
        assert_eq!(batches.len(), 1);
        assert_eq!(batches[0].deletes, deletes);

        // No files: a delete-only batch is created
        let batches = chunk_into_batches(vec![], deletes.clone());
        assert_eq!(batches.len(), 1);
        assert!(batches[0].files.is_empty());
        assert_eq!(batches[0].deletes, deletes);

        // No files, no deletes: empty
        let batches = chunk_into_batches(vec![], vec![]);
        assert!(batches.is_empty());
    }

    #[test]
    fn test_gzip_encode_round_trip() {
        let payload = b"the quick brown fox jumps over the lazy dog";
        let gz = gzip_encode(payload).unwrap();
        let back = gunzip(&gz);
        assert_eq!(back, payload);
    }

    #[test]
    fn test_build_batch_ndjson_format() {
        let batch = Batch {
            files: vec![make_entry("a.rs", b"hello")],
            deletes: vec!["old.rs".into()],
        };
        let bytes = build_batch_ndjson(&batch);
        let text = String::from_utf8(bytes).unwrap();
        let lines: Vec<&str> = text.lines().collect();
        assert_eq!(lines.len(), 2);
        let line0: serde_json::Value = serde_json::from_str(lines[0]).unwrap();
        assert_eq!(line0["op"], "file");
        assert_eq!(line0["path"], "a.rs");
        assert_eq!(line0["sha256"], sha256_hex(b"hello"));
        assert_eq!(line0["content"], BASE64_STANDARD.encode(b"hello"));
        let line1: serde_json::Value = serde_json::from_str(lines[1]).unwrap();
        assert_eq!(line1["op"], "delete");
        assert_eq!(line1["path"], "old.rs");
    }

    // ─── mock-server tests ───

    fn diff_response(sync_id: &str, need: &[&str], delete: &[&str]) -> serde_json::Value {
        serde_json::json!({
            "sync_id": sync_id,
            "need": need,
            "delete": delete,
        })
    }

    async fn mount_diff(server: &MockServer, body: serde_json::Value) {
        Mock::given(method("POST"))
            .and(path("/index/diff"))
            .respond_with(ResponseTemplate::new(200).set_body_json(body))
            .mount(server)
            .await;
    }

    async fn mount_stream_accepted(server: &MockServer, files: usize, deletes: usize, bytes: u64) {
        Mock::given(method("POST"))
            .and(path("/index/stream"))
            .respond_with(ResponseTemplate::new(202).set_body_json(serde_json::json!({
                "queued": { "files": files, "deletes": deletes, "bytes": bytes }
            })))
            .mount(server)
            .await;
    }

    async fn mount_sync_complete_ok(server: &MockServer) {
        Mock::given(method("POST"))
            .and(path("/index/sync-complete"))
            .respond_with(ResponseTemplate::new(200))
            .mount(server)
            .await;
    }

    async fn write_repo(tmp: &TempDir, files: &[(&str, &[u8])]) {
        for (name, content) in files {
            let path = tmp.path().join(name);
            if let Some(parent) = path.parent() {
                fs::create_dir_all(parent).unwrap();
            }
            fs::write(path, content).unwrap();
        }
    }

    #[tokio::test]
    async fn test_full_sync_happy_path() {
        let tmp = TempDir::new().unwrap();
        write_repo(
            &tmp,
            &[
                ("a.rs", b"alpha"),
                ("b.rs", b"beta"),
                ("c.rs", b"gamma"),
            ],
        )
        .await;

        let server = MockServer::start().await;
        mount_diff(
            &server,
            diff_response("sync-1", &["a.rs", "b.rs", "c.rs"], &["old.rs"]),
        )
        .await;
        mount_stream_accepted(&server, 3, 1, 14).await;
        mount_sync_complete_ok(&server).await;

        let streamer = Streamer::new_with_timings(
            make_config(server.uri()),
            Duration::from_secs(5),
            Duration::from_millis(50),
        );
        let stats = streamer
            .full_sync(tmp.path(), make_filter(tmp.path()), CancellationToken::new())
            .await
            .unwrap();
        assert_eq!(stats.files_hashed, 3);
        assert_eq!(stats.files_uploaded, 3);
        assert_eq!(stats.files_deleted, 1);
        assert_eq!(stats.bytes_uploaded, 14);
    }

    #[tokio::test]
    async fn test_full_sync_empty_diff() {
        let tmp = TempDir::new().unwrap();
        write_repo(&tmp, &[("a.rs", b"alpha")]).await;

        let server = MockServer::start().await;
        mount_diff(&server, diff_response("sync-empty", &[], &[])).await;
        // /stream and /sync-complete should NEVER be called.
        Mock::given(method("POST"))
            .and(path("/index/stream"))
            .respond_with(ResponseTemplate::new(500))
            .expect(0)
            .mount(&server)
            .await;
        Mock::given(method("POST"))
            .and(path("/index/sync-complete"))
            .respond_with(ResponseTemplate::new(500))
            .expect(0)
            .mount(&server)
            .await;

        let streamer = Streamer::new_with_timings(
            make_config(server.uri()),
            Duration::from_secs(5),
            Duration::from_millis(50),
        );
        let stats = streamer
            .full_sync(tmp.path(), make_filter(tmp.path()), CancellationToken::new())
            .await
            .unwrap();
        assert_eq!(stats.files_uploaded, 0);
        assert_eq!(stats.files_deleted, 0);
    }

    #[tokio::test]
    async fn test_full_sync_410_restarts() {
        let tmp = TempDir::new().unwrap();
        write_repo(&tmp, &[("a.rs", b"alpha")]).await;

        let server = MockServer::start().await;
        mount_diff(&server, diff_response("sync-410", &["a.rs"], &[])).await;
        Mock::given(method("POST"))
            .and(path("/index/stream"))
            .respond_with(ResponseTemplate::new(410).set_body_string("expired"))
            .mount(&server)
            .await;

        let streamer = Streamer::new_with_timings(
            make_config(server.uri()),
            Duration::from_secs(5),
            Duration::from_millis(50),
        );
        let err = streamer
            .full_sync(tmp.path(), make_filter(tmp.path()), CancellationToken::new())
            .await
            .unwrap_err();
        assert!(matches!(err, SyncError::SyncExpired { .. }));
    }

    #[tokio::test]
    async fn test_full_sync_sync_complete_timeout_treated_as_success() {
        // After stream_batches succeeds, the data is already on the server.
        // If /sync-complete polling exceeds the deadline, we treat it as a
        // warning (not an error) so the badge doesn't go red for a healthy
        // index. See watcher_manager plan item: "server-success / client-error
        // asymmetry".
        let tmp = TempDir::new().unwrap();
        write_repo(&tmp, &[("a.rs", b"alpha")]).await;

        let server = MockServer::start().await;
        mount_diff(&server, diff_response("sync-timeout", &["a.rs"], &[])).await;
        mount_stream_accepted(&server, 1, 0, 5).await;
        Mock::given(method("POST"))
            .and(path("/index/sync-complete"))
            .respond_with(ResponseTemplate::new(202))
            .mount(&server)
            .await;

        let streamer = Streamer::new_with_timings(
            make_config(server.uri()),
            Duration::from_millis(200),
            Duration::from_millis(50),
        );
        let stats = streamer
            .full_sync(tmp.path(), make_filter(tmp.path()), CancellationToken::new())
            .await
            .expect("deadline on /sync-complete should be treated as success");
        assert_eq!(stats.files_uploaded, 1);
    }

    #[tokio::test]
    async fn test_full_sync_stream_400_bad_request() {
        let tmp = TempDir::new().unwrap();
        write_repo(&tmp, &[("a.rs", b"alpha")]).await;

        let server = MockServer::start().await;
        mount_diff(&server, diff_response("sync-400", &["a.rs"], &[])).await;
        Mock::given(method("POST"))
            .and(path("/index/stream"))
            .respond_with(ResponseTemplate::new(400).set_body_json(serde_json::json!({
                "error": "sha256_mismatch",
                "details": "client hash does not match content"
            })))
            .mount(&server)
            .await;

        let streamer = Streamer::new_with_timings(
            make_config(server.uri()),
            Duration::from_secs(5),
            Duration::from_millis(50),
        );
        let err = streamer
            .full_sync(tmp.path(), make_filter(tmp.path()), CancellationToken::new())
            .await
            .unwrap_err();
        match err {
            SyncError::BadRequest { reason, details } => {
                assert_eq!(reason, "sha256_mismatch");
                assert_eq!(details, "client hash does not match content");
            }
            other => panic!("expected BadRequest, got {other:?}"),
        }
    }

    #[tokio::test]
    async fn test_full_sync_duplicate_batch() {
        let tmp = TempDir::new().unwrap();
        write_repo(&tmp, &[("a.rs", b"alpha")]).await;

        let server = MockServer::start().await;
        mount_diff(&server, diff_response("sync-dup", &["a.rs"], &[])).await;
        Mock::given(method("POST"))
            .and(path("/index/stream"))
            .respond_with(
                ResponseTemplate::new(200).set_body_json(serde_json::json!({ "duplicate": true })),
            )
            .mount(&server)
            .await;
        mount_sync_complete_ok(&server).await;

        let streamer = Streamer::new_with_timings(
            make_config(server.uri()),
            Duration::from_secs(5),
            Duration::from_millis(50),
        );
        let stats = streamer
            .full_sync(tmp.path(), make_filter(tmp.path()), CancellationToken::new())
            .await
            .unwrap();
        assert_eq!(stats.duplicate_batches, 1);
        assert_eq!(stats.files_uploaded, 0);
    }

    /// State machine that returns 503 twice then 202.
    struct FlakyStream {
        count: std::sync::Mutex<u32>,
    }

    impl Respond for FlakyStream {
        fn respond(&self, _req: &Request) -> ResponseTemplate {
            let mut c = self.count.lock().unwrap();
            *c += 1;
            if *c <= 2 {
                ResponseTemplate::new(503)
            } else {
                ResponseTemplate::new(202).set_body_json(serde_json::json!({
                    "queued": { "files": 1, "deletes": 0, "bytes": 5 }
                }))
            }
        }
    }

    #[tokio::test]
    async fn test_full_sync_retry_on_transient_error() {
        let tmp = TempDir::new().unwrap();
        write_repo(&tmp, &[("a.rs", b"alpha")]).await;

        let server = MockServer::start().await;
        mount_diff(&server, diff_response("sync-retry", &["a.rs"], &[])).await;
        Mock::given(method("POST"))
            .and(path("/index/stream"))
            .respond_with(FlakyStream {
                count: std::sync::Mutex::new(0),
            })
            .mount(&server)
            .await;
        mount_sync_complete_ok(&server).await;

        let streamer = Streamer::new_with_timings(
            make_config(server.uri()),
            Duration::from_secs(5),
            Duration::from_millis(50),
        );
        let stats = streamer
            .full_sync(tmp.path(), make_filter(tmp.path()), CancellationToken::new())
            .await
            .unwrap();
        assert_eq!(stats.files_uploaded, 1);
    }

    /// Capture inbound /diff bodies for assertion.
    #[derive(Clone)]
    struct CaptureBody {
        inner: Arc<std::sync::Mutex<Vec<u8>>>,
        status: u16,
        body: serde_json::Value,
    }

    impl Respond for CaptureBody {
        fn respond(&self, req: &Request) -> ResponseTemplate {
            *self.inner.lock().unwrap() = req.body.clone();
            ResponseTemplate::new(self.status).set_body_json(self.body.clone())
        }
    }

    #[tokio::test]
    async fn test_full_sync_diff_request_serialization() {
        let tmp = TempDir::new().unwrap();
        write_repo(&tmp, &[("a.rs", b"alpha")]).await;

        let server = MockServer::start().await;
        let captured = Arc::new(std::sync::Mutex::new(Vec::new()));
        Mock::given(method("POST"))
            .and(path("/index/diff"))
            .and(header("content-encoding", "gzip"))
            .respond_with(CaptureBody {
                inner: captured.clone(),
                status: 200,
                body: diff_response("sync-x", &[], &[]),
            })
            .mount(&server)
            .await;

        let streamer = Streamer::new_with_timings(
            make_config(server.uri()),
            Duration::from_secs(5),
            Duration::from_millis(50),
        );
        streamer
            .full_sync(tmp.path(), make_filter(tmp.path()), CancellationToken::new())
            .await
            .unwrap();

        let raw = captured.lock().unwrap().clone();
        let decoded = gunzip(&raw);
        let json: serde_json::Value = serde_json::from_slice(&decoded).unwrap();
        // Identity lives in the body (WS3 contract).
        assert_eq!(json["user_id"], "user-1");
        assert_eq!(json["workspace_id"], 42);
        assert_eq!(json["machine_id"], "machine-1");
        assert_eq!(json["repo_path"], "/repo");
        // Default-omit for full sync.
        assert!(json.get("incremental").is_none(), "full sync must not serialize 'incremental'");
        assert!(json.get("deletes").is_none(), "full sync must not serialize 'deletes'");
        assert_eq!(json["files"][0]["path"], "a.rs");
    }

    // ─── incremental tests ───

    #[tokio::test]
    async fn test_incremental_sync_happy_path() {
        let tmp = TempDir::new().unwrap();
        write_repo(&tmp, &[("a.rs", b"new content")]).await;

        let server = MockServer::start().await;
        let captured = Arc::new(std::sync::Mutex::new(Vec::new()));
        Mock::given(method("POST"))
            .and(path("/index/diff"))
            .respond_with(CaptureBody {
                inner: captured.clone(),
                status: 200,
                body: diff_response("sync-inc", &["a.rs"], &["old.rs"]),
            })
            .mount(&server)
            .await;
        mount_stream_accepted(&server, 1, 1, 11).await;
        mount_sync_complete_ok(&server).await;

        let streamer = Streamer::new_with_timings(
            make_config(server.uri()),
            Duration::from_secs(5),
            Duration::from_millis(50),
        );
        let stats = streamer
            .incremental_sync(
                &[tmp.path().join("a.rs")],
                &[tmp.path().join("old.rs")],
                tmp.path(),
                CancellationToken::new(),
            )
            .await
            .unwrap();
        assert_eq!(stats.files_hashed, 1);
        assert_eq!(stats.files_uploaded, 1);
        assert_eq!(stats.files_deleted, 1);

        let raw = captured.lock().unwrap().clone();
        let json: serde_json::Value = serde_json::from_slice(&gunzip(&raw)).unwrap();
        assert_eq!(json["user_id"], "user-1");
        assert_eq!(json["workspace_id"], 42);
        assert_eq!(json["incremental"], true);
        assert_eq!(json["deletes"], serde_json::json!(["old.rs"]));
        assert_eq!(json["files"][0]["path"], "a.rs");
    }

    #[tokio::test]
    async fn test_incremental_sync_pure_delete_event() {
        let tmp = TempDir::new().unwrap();

        let server = MockServer::start().await;
        let captured = Arc::new(std::sync::Mutex::new(Vec::new()));
        Mock::given(method("POST"))
            .and(path("/index/diff"))
            .respond_with(CaptureBody {
                inner: captured.clone(),
                status: 200,
                body: diff_response("sync-pure-del", &[], &["old.rs"]),
            })
            .mount(&server)
            .await;
        mount_stream_accepted(&server, 0, 1, 0).await;
        mount_sync_complete_ok(&server).await;

        let streamer = Streamer::new_with_timings(
            make_config(server.uri()),
            Duration::from_secs(5),
            Duration::from_millis(50),
        );
        let stats = streamer
            .incremental_sync(
                &[],
                &[tmp.path().join("old.rs")],
                tmp.path(),
                CancellationToken::new(),
            )
            .await
            .unwrap();
        assert_eq!(stats.files_hashed, 0);
        assert_eq!(stats.files_uploaded, 0);
        assert_eq!(stats.files_deleted, 1);

        let raw = captured.lock().unwrap().clone();
        let json: serde_json::Value = serde_json::from_slice(&gunzip(&raw)).unwrap();
        assert_eq!(json["user_id"], "user-1");
        assert_eq!(json["workspace_id"], 42);
        assert_eq!(json["incremental"], true);
        assert_eq!(json["files"], serde_json::json!([]));
        assert_eq!(json["deletes"], serde_json::json!(["old.rs"]));
    }

    #[tokio::test]
    async fn test_incremental_sync_noop_unchanged_hash() {
        let tmp = TempDir::new().unwrap();
        write_repo(&tmp, &[("a.rs", b"unchanged")]).await;

        let server = MockServer::start().await;
        mount_diff(&server, diff_response("sync-noop", &[], &[])).await;
        // /stream and /sync-complete should NEVER be called
        Mock::given(method("POST"))
            .and(path("/index/stream"))
            .respond_with(ResponseTemplate::new(500))
            .expect(0)
            .mount(&server)
            .await;
        Mock::given(method("POST"))
            .and(path("/index/sync-complete"))
            .respond_with(ResponseTemplate::new(500))
            .expect(0)
            .mount(&server)
            .await;

        let streamer = Streamer::new_with_timings(
            make_config(server.uri()),
            Duration::from_secs(5),
            Duration::from_millis(50),
        );
        let stats = streamer
            .incremental_sync(
                &[tmp.path().join("a.rs")],
                &[],
                tmp.path(),
                CancellationToken::new(),
            )
            .await
            .unwrap();
        assert_eq!(stats.files_hashed, 1);
        assert_eq!(stats.files_uploaded, 0);
    }

    #[tokio::test]
    async fn test_incremental_sync_does_not_request_full_walk() {
        // Repo has many files but incremental_sync is told only one of them changed.
        // Verify only the named file is read (we don't actually walk the tree).
        let tmp = TempDir::new().unwrap();
        write_repo(
            &tmp,
            &[
                ("a.rs", b"a"),
                ("b.rs", b"b"),
                ("c.rs", b"c"),
                ("d.rs", b"d"),
            ],
        )
        .await;

        let server = MockServer::start().await;
        let captured = Arc::new(std::sync::Mutex::new(Vec::new()));
        Mock::given(method("POST"))
            .and(path("/index/diff"))
            .respond_with(CaptureBody {
                inner: captured.clone(),
                status: 200,
                body: diff_response("sync-only-one", &[], &[]),
            })
            .mount(&server)
            .await;

        let streamer = Streamer::new_with_timings(
            make_config(server.uri()),
            Duration::from_secs(5),
            Duration::from_millis(50),
        );
        streamer
            .incremental_sync(
                &[tmp.path().join("b.rs")],
                &[],
                tmp.path(),
                CancellationToken::new(),
            )
            .await
            .unwrap();

        let raw = captured.lock().unwrap().clone();
        let json: serde_json::Value = serde_json::from_slice(&gunzip(&raw)).unwrap();
        let files = json["files"].as_array().unwrap();
        assert_eq!(files.len(), 1);
        assert_eq!(files[0]["path"], "b.rs");
    }

    // ─── PR-2 follow-ups: C1 abort + C4 fail-fast 4xx ───

    /// Mock /stream responder: returns 400 on the very first request and a
    /// long-delayed 202 on every subsequent request. Simulates the scenario
    /// where one batch fails fast while sibling batches are still in-flight,
    /// exercising the abort_and_drain path in stream_batches (C1).
    struct FirstFailsOthersSlow {
        count: std::sync::Mutex<u32>,
        delay: Duration,
    }

    impl Respond for FirstFailsOthersSlow {
        fn respond(&self, _req: &Request) -> ResponseTemplate {
            let mut c = self.count.lock().unwrap();
            *c += 1;
            if *c == 1 {
                ResponseTemplate::new(400).set_body_json(serde_json::json!({
                    "error": "forced_fail",
                    "details": "first batch intentionally fails"
                }))
            } else {
                ResponseTemplate::new(202)
                    .set_delay(self.delay)
                    .set_body_json(serde_json::json!({
                        "queued": { "files": 1, "deletes": 0, "bytes": 1 }
                    }))
            }
        }
    }

    /// C1 — when one batch fails, the remaining in-flight batches must be
    /// aborted instead of being detached and running against a dead sync_id.
    /// We assert this by timing: the sibling batches are mocked with a 10s
    /// delay; without abort the test would block for ~10s, with abort it
    /// returns almost immediately.
    #[tokio::test]
    async fn test_stream_batches_aborts_siblings_on_error() {
        use std::time::Instant;

        let tmp = TempDir::new().unwrap();
        // Write 150 tiny files → 2 batches (MAX_BATCH_FILES=100) so there's
        // at least one sibling in-flight when the first response arrives.
        let mut files: Vec<(String, Vec<u8>)> = Vec::new();
        for i in 0..150 {
            files.push((format!("f{i:03}.rs"), b"x".to_vec()));
        }
        for (name, content) in &files {
            fs::write(tmp.path().join(name), content).unwrap();
        }

        let server = MockServer::start().await;
        let need: Vec<String> = files.iter().map(|(n, _)| n.clone()).collect();
        let need_refs: Vec<&str> = need.iter().map(|s| s.as_str()).collect();
        mount_diff(&server, diff_response("sync-abort", &need_refs, &[])).await;
        Mock::given(method("POST"))
            .and(path("/index/stream"))
            .respond_with(FirstFailsOthersSlow {
                count: std::sync::Mutex::new(0),
                delay: Duration::from_secs(10),
            })
            .mount(&server)
            .await;

        let streamer = Streamer::new_with_timings(
            make_config(server.uri()),
            Duration::from_secs(5),
            Duration::from_millis(50),
        );
        let started = Instant::now();
        let err = streamer
            .full_sync(tmp.path(), make_filter(tmp.path()), CancellationToken::new())
            .await
            .unwrap_err();
        let elapsed = started.elapsed();

        assert!(
            matches!(err, SyncError::BadRequest { .. }),
            "expected BadRequest, got {err:?}"
        );
        // Sibling delay is 10s. If abort is working, we return in well under
        // that. 4s is a generous ceiling for CI variance.
        assert!(
            elapsed < Duration::from_secs(4),
            "elapsed {elapsed:?} — siblings not aborted (C1 regression)"
        );
    }

    /// C4 helper: single-file repo + /stream returning the given status.
    /// Verifies we return `SyncError::AuthRequired` and the mock received
    /// exactly one request (no retries).
    async fn assert_stream_fails_fast(status: u16) {
        let tmp = TempDir::new().unwrap();
        write_repo(&tmp, &[("a.rs", b"alpha")]).await;

        let server = MockServer::start().await;
        mount_diff(
            &server,
            diff_response(&format!("sync-{status}"), &["a.rs"], &[]),
        )
        .await;
        Mock::given(method("POST"))
            .and(path("/index/stream"))
            .respond_with(ResponseTemplate::new(status))
            .expect(1) // no retries
            .mount(&server)
            .await;

        let streamer = Streamer::new_with_timings(
            make_config(server.uri()),
            Duration::from_secs(5),
            Duration::from_millis(50),
        );
        let err = streamer
            .full_sync(tmp.path(), make_filter(tmp.path()), CancellationToken::new())
            .await
            .unwrap_err();
        match err {
            SyncError::AuthRequired { status: s } => assert_eq!(s, status),
            other => panic!("expected AuthRequired({status}), got {other:?}"),
        }
    }

    #[tokio::test]
    async fn test_stream_one_batch_fails_fast_on_401() {
        assert_stream_fails_fast(401).await;
    }

    #[tokio::test]
    async fn test_stream_one_batch_fails_fast_on_403() {
        assert_stream_fails_fast(403).await;
    }

    #[tokio::test]
    async fn test_stream_one_batch_fails_fast_on_404() {
        assert_stream_fails_fast(404).await;
    }

    /// Regression guard: 429 Too Many Requests is 4xx but canonically
    /// retriable. Must not fall into the AuthRequired fail-fast arm.
    struct Flaky429 {
        count: std::sync::Mutex<u32>,
    }

    impl Respond for Flaky429 {
        fn respond(&self, _req: &Request) -> ResponseTemplate {
            let mut c = self.count.lock().unwrap();
            *c += 1;
            if *c <= 2 {
                ResponseTemplate::new(429)
            } else {
                ResponseTemplate::new(202).set_body_json(serde_json::json!({
                    "queued": { "files": 1, "deletes": 0, "bytes": 5 }
                }))
            }
        }
    }

    #[tokio::test]
    async fn test_stream_one_batch_still_retries_429() {
        let tmp = TempDir::new().unwrap();
        write_repo(&tmp, &[("a.rs", b"alpha")]).await;

        let server = MockServer::start().await;
        mount_diff(&server, diff_response("sync-429", &["a.rs"], &[])).await;
        Mock::given(method("POST"))
            .and(path("/index/stream"))
            .respond_with(Flaky429 {
                count: std::sync::Mutex::new(0),
            })
            .mount(&server)
            .await;
        mount_sync_complete_ok(&server).await;

        let streamer = Streamer::new_with_timings(
            make_config(server.uri()),
            Duration::from_secs(5),
            Duration::from_millis(50),
        );
        let stats = streamer
            .full_sync(tmp.path(), make_filter(tmp.path()), CancellationToken::new())
            .await
            .unwrap();
        assert_eq!(stats.files_uploaded, 1);
    }

    /// C4 — /diff 401 returns AuthRequired (replaces old string-based Other).
    #[tokio::test]
    async fn test_diff_fails_fast_on_401() {
        let tmp = TempDir::new().unwrap();
        write_repo(&tmp, &[("a.rs", b"alpha")]).await;

        let server = MockServer::start().await;
        Mock::given(method("POST"))
            .and(path("/index/diff"))
            .respond_with(ResponseTemplate::new(401))
            .expect(1)
            .mount(&server)
            .await;

        let streamer = Streamer::new_with_timings(
            make_config(server.uri()),
            Duration::from_secs(5),
            Duration::from_millis(50),
        );
        let err = streamer
            .full_sync(tmp.path(), make_filter(tmp.path()), CancellationToken::new())
            .await
            .unwrap_err();
        assert!(
            matches!(err, SyncError::AuthRequired { status: 401 }),
            "expected AuthRequired(401), got {err:?}"
        );
    }

    // ─── 1.3.28 fix regression tests ───

    #[tokio::test]
    async fn test_sync_complete_410_treated_as_success() {
        // /sync-complete returning 410 means the server already finalized and
        // GC'd the sync_id record — data IS indexed. Previously surfaced as
        // SyncError::SyncExpired → badge red. Now treated as Ok.
        let tmp = TempDir::new().unwrap();
        write_repo(&tmp, &[("a.rs", b"alpha")]).await;

        let server = MockServer::start().await;
        mount_diff(&server, diff_response("sync-sc-410", &["a.rs"], &[])).await;
        mount_stream_accepted(&server, 1, 0, 5).await;
        Mock::given(method("POST"))
            .and(path("/index/sync-complete"))
            .respond_with(ResponseTemplate::new(410).set_body_string("already finalized"))
            .mount(&server)
            .await;

        let streamer = Streamer::new_with_timings(
            make_config(server.uri()),
            Duration::from_secs(5),
            Duration::from_millis(50),
        );
        let stats = streamer
            .full_sync(tmp.path(), make_filter(tmp.path()), CancellationToken::new())
            .await
            .expect("410 on /sync-complete should be a success");
        assert_eq!(stats.files_uploaded, 1);
    }

    #[tokio::test]
    async fn test_call_diff_retries_on_429_then_succeeds() {
        // First 2 /diff calls return 429 (concurrent sync), 3rd returns 200.
        // With 3 retries and exponential backoff starting at 2s, we use very
        // short-lived mocks via up_to_n_times.
        let tmp = TempDir::new().unwrap();
        write_repo(&tmp, &[("a.rs", b"alpha")]).await;

        let server = MockServer::start().await;
        // First response: 429
        Mock::given(method("POST"))
            .and(path("/index/diff"))
            .respond_with(ResponseTemplate::new(429).set_body_string("busy"))
            .up_to_n_times(1)
            .mount(&server)
            .await;
        // Subsequent responses: 200 with normal diff body
        mount_diff(&server, diff_response("retry-ok", &[], &[])).await;

        let streamer = Streamer::new_with_timings(
            make_config(server.uri()),
            Duration::from_secs(30),
            Duration::from_millis(50),
        );
        // full_sync should succeed after the retry (empty need/delete → no stream/poll)
        let stats = streamer
            .full_sync(tmp.path(), make_filter(tmp.path()), CancellationToken::new())
            .await
            .expect("should retry past 429 and succeed");
        assert_eq!(stats.files_hashed, 1);
    }

    #[tokio::test]
    async fn test_incremental_sync_cancellation_returns_cancelled_error() {
        // Cancellation must surface as SyncError::Cancelled, not as a generic
        // error. The watcher_manager layer then maps Cancelled to Indexed
        // (not Error) so the badge doesn't flash red on repo switch.
        let tmp = TempDir::new().unwrap();
        write_repo(&tmp, &[("a.rs", b"alpha")]).await;

        // Cancel immediately — incremental_sync checks at the top of its loop
        // before the first fs::read.
        let cancel = CancellationToken::new();
        cancel.cancel();

        let server = MockServer::start().await;
        let streamer = Streamer::new(make_config(server.uri()));
        let changed = vec![tmp.path().join("a.rs")];
        let err = streamer
            .incremental_sync(&changed, &[], tmp.path(), cancel)
            .await
            .unwrap_err();
        assert!(
            matches!(err, SyncError::Cancelled),
            "expected Cancelled, got {err:?}"
        );
    }
}
