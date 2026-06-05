//! bulk-indexer — offline host tool: "index this checkout into this engine under this key."
//!
//! Runs `context_sync::Streamer::full_sync` once against a repo checkout and a context-engine.
//! It knows nothing about the manifest — the Python eval harness owns `git clone`/`checkout` and
//! calls this per instance with `--repo-path <instance_id>` (the engine isolation key, which MUST
//! match the bench-runner's `--ce-repo-path` at run time). Native build; never enters a container.

use std::path::PathBuf;
use std::sync::Arc;

use clap::Parser;
use context_sync::{IgnoreFilter, Streamer, StreamerConfig};
use serde::Serialize;
use tokio_util::sync::CancellationToken;

#[derive(Parser, Debug)]
#[command(name = "bulk-indexer", about = "Index a repo checkout into a context-engine.")]
struct Cli {
    /// Repo checkout to index.
    #[arg(long)]
    repo: PathBuf,

    /// Context-engine base URL (bare, e.g. http://localhost:8106). `/api/v1` is appended internally.
    #[arg(long)]
    engine_url: String,

    /// Engine isolation key for this repo. Use a deterministic scheme (e.g. the instance_id).
    /// MUST match the bench-runner's --ce-repo-path so the run can find this index.
    #[arg(long)]
    repo_path: String,

    /// Identity (must match the bench-runner run). Defaults are the harness convention.
    #[arg(long, default_value = "bench")]
    user_id: String,
    #[arg(long, default_value_t = 1)]
    workspace_id: u64,
    #[arg(long, default_value = "bench")]
    machine_id: String,
    #[arg(long, default_value = "")]
    auth_token: String,
}

/// JSON-friendly view of `SyncStats` (its `duration` is a `Duration`).
#[derive(Serialize)]
struct StatsSummary {
    files_hashed: usize,
    files_uploaded: usize,
    files_deleted: usize,
    bytes_uploaded: u64,
    duplicate_batches: usize,
    duration_secs: f64,
}

#[tokio::main]
async fn main() {
    let cli = Cli::parse();

    // Mirror the desktop's watcher_manager: the engine serves index routes under /api/v1, and the
    // streamer appends only `/index/...`, so the /api/v1 segment belongs in the configured URL.
    let cfg = StreamerConfig {
        context_engine_url: format!("{}/api/v1", cli.engine_url.trim_end_matches('/')),
        user_id: cli.user_id,
        workspace_id: cli.workspace_id,
        machine_id: cli.machine_id,
        repo_path: cli.repo_path,
        auth_token: cli.auth_token,
    };

    let filter = Arc::new(IgnoreFilter::new(&cli.repo));
    let result = Streamer::new(cfg)
        .full_sync(&cli.repo, filter, CancellationToken::new())
        .await;

    match result {
        Ok(stats) => {
            let summary = StatsSummary {
                files_hashed: stats.files_hashed,
                files_uploaded: stats.files_uploaded,
                files_deleted: stats.files_deleted,
                bytes_uploaded: stats.bytes_uploaded,
                duplicate_batches: stats.duplicate_batches,
                duration_secs: stats.duration.as_secs_f64(),
            };
            println!("{}", serde_json::to_string(&summary).expect("serialize stats"));
        }
        Err(e) => {
            eprintln!("bulk-indexer failed: {e}");
            std::process::exit(1);
        }
    }
}
