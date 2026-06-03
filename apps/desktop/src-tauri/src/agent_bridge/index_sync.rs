//! Streaming-sync client for the optional context engine.
//!
//! Pushes a repository up to the context-engine HTTP API for indexing using
//! the three-endpoint v3 flow: `/index/diff` → `/index/stream` →
//! `/index/sync-complete`. The server is built to match this client
//! byte-for-byte (deterministic batch IDs, base64 `content`, gzip NDJSON).
//!
//! Identity is the local single-user tuple (`user_id="local"`,
//! `workspace_id=0`, a stable `machine_id`, and the repo path). The same
//! tuple is what `ContextEngineClient` sends on `/search` + `/graph/query`,
//! so the server keys the index collection identically.

use std::io::Write;
use std::path::PathBuf;
use std::sync::Arc;
use std::time::Duration;

use base64::Engine as _;
use flate2::write::GzEncoder;
use flate2::Compression;
use serde::{Deserialize, Serialize};
use sha2::{Digest, Sha256};

use crate::agent_bridge::traits::EventEmitter;

/// Per-file content cap. Files larger than this are skipped entirely (neither
/// hashed into /diff nor uploaded) — matches the server's 1 MB per-file cap.
const MAX_FILE_BYTES: u64 = 1024 * 1024;
/// Soft cap on a batch's total file content before flushing. Kept well under
/// the server's 50 MB decompressed /stream limit to leave headroom for base64
/// + JSON framing.
const BATCH_BYTE_CAP: usize = 8 * 1024 * 1024;
/// How long to keep polling /sync-complete before giving up.
const POLL_TIMEOUT: Duration = Duration::from_secs(600);

/// Local single-user identity + the repo to index.
#[derive(Clone)]
pub struct SyncIdentity {
    pub user_id: String,
    pub workspace_id: u64,
    pub machine_id: String,
    /// Canonical repo path the index is keyed by (the session folder). MUST
    /// match the `repo_path` the search/graph client sends.
    pub repo_path: String,
}

// ── Wire DTOs (match services/context-engine/models/dto) ──────────────────────

#[derive(Serialize)]
struct DiffFileHash {
    path: String,
    sha256: String,
}

#[derive(Serialize)]
struct DiffRequest {
    user_id: String,
    workspace_id: u64,
    machine_id: String,
    repo_path: String,
    files: Vec<DiffFileHash>,
    incremental: bool,
    deletes: Vec<String>,
}

#[derive(Deserialize)]
struct DiffResponse {
    sync_id: String,
    #[serde(default)]
    need: Vec<String>,
    #[serde(default)]
    delete: Vec<String>,
}

/// One NDJSON line of a /stream body. Go marshals `[]byte` as base64, so
/// `content` is an explicit base64 string (not a serde byte array). Empty
/// `sha256`/`content` (deletes) are omitted to match the server's omitempty.
#[derive(Serialize)]
struct StreamLine {
    op: String,
    path: String,
    #[serde(skip_serializing_if = "String::is_empty")]
    sha256: String,
    #[serde(skip_serializing_if = "String::is_empty")]
    content: String,
}

#[derive(Deserialize, Default)]
struct SyncCompleteResponse {
    #[serde(default)]
    status: String,
    #[serde(default)]
    error: String,
    #[serde(default)]
    reason: String,
}

// ── Internal types ────────────────────────────────────────────────────────────

/// A repo-relative file plus its absolute path and content hash.
struct WalkedFile {
    rel_path: String,
    abs_path: PathBuf,
    sha256: String,
}

/// One entry queued for /stream: either a file (with content) or a delete.
struct BatchItem {
    op: &'static str,
    path: String,
    sha256: String,
    content_b64: String,
}

#[derive(Debug)]
enum SyncError {
    Http(String),
    Server { status: u16, body: String },
    Failed { status: String, reason: String },
    Timeout,
}

impl std::fmt::Display for SyncError {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        match self {
            SyncError::Http(e) => write!(f, "http error: {e}"),
            SyncError::Server { status, body } => write!(f, "server {status}: {body}"),
            SyncError::Failed { status, reason } => write!(f, "indexing {status}: {reason}"),
            SyncError::Timeout => write!(f, "indexing did not complete within timeout"),
        }
    }
}

// ── Helpers ─────────────────────────────────────────────────────────────────

fn sha256_hex(bytes: &[u8]) -> String {
    let mut h = Sha256::new();
    h.update(bytes);
    hex(&h.finalize())
}

fn hex(bytes: &[u8]) -> String {
    let mut s = String::with_capacity(bytes.len() * 2);
    for b in bytes {
        s.push_str(&format!("{b:02x}"));
    }
    s
}

fn gzip(data: &[u8]) -> std::io::Result<Vec<u8>> {
    let mut enc = GzEncoder::new(Vec::new(), Compression::default());
    enc.write_all(data)?;
    enc.finish()
}

/// Deterministic batch id: sha256(hex) of sorted `op|path|hash` lines joined
/// by `\n`. MUST stay byte-identical with the server's `computeBatchID`.
fn deterministic_batch_id(items: &[BatchItem]) -> String {
    let mut parts: Vec<String> = items
        .iter()
        .map(|it| format!("{}|{}|{}", it.op, it.path, it.sha256))
        .collect();
    parts.sort();
    sha256_hex(parts.join("\n").as_bytes())
}

/// Walk the repo respecting .gitignore, hashing every file under the size cap.
/// Runs synchronously (the `ignore` crate is blocking) — call via spawn_blocking.
fn walk_repo(root: &std::path::Path) -> Vec<WalkedFile> {
    let mut out = Vec::new();
    let walker = ignore::WalkBuilder::new(root)
        .standard_filters(true) // .gitignore, .ignore, hidden files
        .git_global(true)
        .build();
    for entry in walker.flatten() {
        let Some(ft) = entry.file_type() else { continue };
        if !ft.is_file() {
            continue;
        }
        let meta = match entry.metadata() {
            Ok(m) => m,
            Err(_) => continue,
        };
        if meta.len() > MAX_FILE_BYTES {
            continue;
        }
        let abs = entry.path().to_path_buf();
        let rel = match abs.strip_prefix(root) {
            Ok(r) => r.to_string_lossy().replace('\\', "/"),
            Err(_) => continue,
        };
        let content = match std::fs::read(&abs) {
            Ok(c) => c,
            Err(_) => continue,
        };
        out.push(WalkedFile {
            rel_path: rel,
            abs_path: abs,
            sha256: sha256_hex(&content),
        });
    }
    out
}

// ── Public entry ──────────────────────────────────────────────────────────────

/// Run a full background sync of `identity.repo_path` against the context
/// engine at `base_url`. Best-effort: emits `indexing:progress` /
/// `indexing:complete` events and never panics. Errors surface as a failed
/// `indexing:complete` event, not a returned error.
pub async fn sync_repo(
    base_url: String,
    identity: SyncIdentity,
    emitter: Arc<dyn EventEmitter>,
    session_id: String,
) {
    emit_progress(&emitter, &session_id, "walking", None);
    match run_sync(&base_url, &identity, &emitter, &session_id).await {
        Ok(()) => emit_complete(&emitter, &session_id, "done", None),
        Err(e) => {
            log::warn!("context-engine sync failed for {}: {e}", identity.repo_path);
            emit_complete(&emitter, &session_id, "failed", Some(&e.to_string()));
        }
    }
}

async fn run_sync(
    base_url: &str,
    identity: &SyncIdentity,
    emitter: &Arc<dyn EventEmitter>,
    session_id: &str,
) -> Result<(), SyncError> {
    let client = reqwest::Client::builder()
        .connect_timeout(Duration::from_secs(10))
        .timeout(Duration::from_secs(120))
        .build()
        .map_err(|e| SyncError::Http(e.to_string()))?;

    // 1. Walk + hash (blocking work off the async pool).
    let root = PathBuf::from(&identity.repo_path);
    let files = tokio::task::spawn_blocking(move || walk_repo(&root))
        .await
        .map_err(|e| SyncError::Http(format!("walk task: {e}")))?;

    // 2. /diff — send the full file list (incremental=false). The server
    //    Merkle-diffs it, so `need` already contains only changed files.
    let diff_req = DiffRequest {
        user_id: identity.user_id.clone(),
        workspace_id: identity.workspace_id,
        machine_id: identity.machine_id.clone(),
        repo_path: identity.repo_path.clone(),
        files: files
            .iter()
            .map(|f| DiffFileHash {
                path: f.rel_path.clone(),
                sha256: f.sha256.clone(),
            })
            .collect(),
        incremental: false,
        deletes: Vec::new(),
    };
    let diff: DiffResponse = post_diff(&client, base_url, &diff_req).await?;
    emit_progress(
        &emitter.clone(),
        session_id,
        "diffed",
        Some(diff.need.len() as u64),
    );

    // Nothing changed and nothing to delete → index is already current.
    if diff.need.is_empty() && diff.delete.is_empty() {
        return Ok(());
    }

    // 3. Build the upload set: changed files (with content) + deletions.
    let by_path: std::collections::HashMap<&str, &WalkedFile> =
        files.iter().map(|f| (f.rel_path.as_str(), f)).collect();

    let mut items: Vec<BatchItem> = Vec::new();
    for path in &diff.need {
        let Some(f) = by_path.get(path.as_str()) else {
            // Server asked for a path we don't have locally anymore — skip.
            continue;
        };
        let content = match std::fs::read(&f.abs_path) {
            Ok(c) => c,
            Err(_) => continue,
        };
        // Re-hash defensively in case the file changed since the walk.
        let sha = sha256_hex(&content);
        items.push(BatchItem {
            op: "file",
            path: f.rel_path.clone(),
            sha256: sha,
            content_b64: base64::engine::general_purpose::STANDARD.encode(&content),
        });
    }
    for path in &diff.delete {
        items.push(BatchItem {
            op: "delete",
            path: path.clone(),
            sha256: String::new(),
            content_b64: String::new(),
        });
    }

    // 4. /stream — flush in size-capped batches.
    let mut sent = 0usize;
    let mut batch: Vec<BatchItem> = Vec::new();
    let mut batch_bytes = 0usize;
    for item in items {
        let sz = item.content_b64.len();
        if !batch.is_empty() && batch_bytes + sz > BATCH_BYTE_CAP {
            sent += batch.len();
            post_stream(&client, base_url, &diff.sync_id, &batch).await?;
            emit_progress(&emitter.clone(), session_id, "uploading", Some(sent as u64));
            batch.clear();
            batch_bytes = 0;
        }
        batch_bytes += sz;
        batch.push(item);
    }
    if !batch.is_empty() {
        sent += batch.len();
        post_stream(&client, base_url, &diff.sync_id, &batch).await?;
        emit_progress(&emitter.clone(), session_id, "uploading", Some(sent as u64));
    }

    // 5. Poll /sync-complete until terminal.
    poll_until_done(&client, base_url, &diff.sync_id, emitter, session_id).await
}

async fn post_diff(
    client: &reqwest::Client,
    base_url: &str,
    req: &DiffRequest,
) -> Result<DiffResponse, SyncError> {
    let body = serde_json::to_vec(req).map_err(|e| SyncError::Http(e.to_string()))?;
    let gz = gzip(&body).map_err(|e| SyncError::Http(e.to_string()))?;

    // Retry once on 429 (another sync in progress) after the suggested delay.
    for attempt in 0..2 {
        let resp = client
            .post(format!("{base_url}/api/v1/index/diff"))
            .header("Content-Encoding", "gzip")
            .header("Content-Type", "application/json")
            .body(gz.clone())
            .send()
            .await
            .map_err(|e| SyncError::Http(e.to_string()))?;
        let status = resp.status();
        if status.is_success() {
            let text = resp.text().await.map_err(|e| SyncError::Http(e.to_string()))?;
            return serde_json::from_str(&text).map_err(|e| SyncError::Http(e.to_string()));
        }
        if status.as_u16() == 429 && attempt == 0 {
            let secs = resp
                .headers()
                .get("Retry-After")
                .and_then(|v| v.to_str().ok())
                .and_then(|s| s.parse::<u64>().ok())
                .unwrap_or(5);
            tokio::time::sleep(Duration::from_secs(secs.min(30))).await;
            continue;
        }
        let body = resp.text().await.unwrap_or_default();
        return Err(SyncError::Server { status: status.as_u16(), body });
    }
    Err(SyncError::Server { status: 429, body: "sync_in_progress".into() })
}

async fn post_stream(
    client: &reqwest::Client,
    base_url: &str,
    sync_id: &str,
    batch: &[BatchItem],
) -> Result<(), SyncError> {
    let batch_id = deterministic_batch_id(batch);

    let mut ndjson = Vec::new();
    for it in batch {
        let line = StreamLine {
            op: it.op.to_string(),
            path: it.path.clone(),
            sha256: it.sha256.clone(),
            content: it.content_b64.clone(),
        };
        let mut bytes = serde_json::to_vec(&line).map_err(|e| SyncError::Http(e.to_string()))?;
        bytes.push(b'\n');
        ndjson.extend_from_slice(&bytes);
    }
    let gz = gzip(&ndjson).map_err(|e| SyncError::Http(e.to_string()))?;

    let resp = client
        .post(format!("{base_url}/api/v1/index/stream"))
        .header("Content-Encoding", "gzip")
        .header("Content-Type", "application/x-ndjson")
        .header("X-Sync-Id", sync_id)
        .header("X-Batch-Id", batch_id)
        .body(gz)
        .send()
        .await
        .map_err(|e| SyncError::Http(e.to_string()))?;

    let status = resp.status();
    // 202 accepted, 200 duplicate (already ingested) — both fine.
    if status.is_success() {
        return Ok(());
    }
    let body = resp.text().await.unwrap_or_default();
    Err(SyncError::Server { status: status.as_u16(), body })
}

async fn poll_until_done(
    client: &reqwest::Client,
    base_url: &str,
    sync_id: &str,
    emitter: &Arc<dyn EventEmitter>,
    session_id: &str,
) -> Result<(), SyncError> {
    let deadline = tokio::time::Instant::now() + POLL_TIMEOUT;
    let body = serde_json::json!({ "sync_id": sync_id });
    loop {
        if tokio::time::Instant::now() >= deadline {
            return Err(SyncError::Timeout);
        }
        let resp = client
            .post(format!("{base_url}/api/v1/index/sync-complete"))
            .json(&body)
            .send()
            .await
            .map_err(|e| SyncError::Http(e.to_string()))?;
        let status = resp.status().as_u16();
        let parsed: SyncCompleteResponse = resp.json().await.unwrap_or_default();
        match status {
            200 => return Ok(()),
            // receiving (409) / processing|finalizing (202) — keep waiting.
            202 | 409 => {
                let phase = if parsed.status.is_empty() { "processing" } else { &parsed.status };
                emit_progress(&emitter.clone(), session_id, phase, None);
                tokio::time::sleep(Duration::from_millis(1200)).await;
            }
            410 => {
                return Err(SyncError::Failed {
                    status: if parsed.status.is_empty() { "failed".into() } else { parsed.status },
                    reason: if !parsed.reason.is_empty() { parsed.reason } else { parsed.error },
                })
            }
            other => {
                return Err(SyncError::Server {
                    status: other,
                    body: parsed.error,
                })
            }
        }
    }
}

// ── Event emission ────────────────────────────────────────────────────────────

fn emit_progress(emitter: &Arc<dyn EventEmitter>, session_id: &str, phase: &str, count: Option<u64>) {
    let _ = emitter.emit(
        "indexing:progress",
        serde_json::json!({ "session_id": session_id, "status": phase, "count": count }),
    );
}

fn emit_complete(emitter: &Arc<dyn EventEmitter>, session_id: &str, status: &str, error: Option<&str>) {
    let _ = emitter.emit(
        "indexing:complete",
        serde_json::json!({ "session_id": session_id, "status": status, "error": error }),
    );
}
