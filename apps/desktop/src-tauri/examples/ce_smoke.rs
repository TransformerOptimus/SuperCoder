//! Headless end-to-end smoke test for the context-engine integration.
//!
//! Runs the REAL app code against a running stack:
//!   1. context_watcher::streamer::Streamer.full_sync — index a repo (the same
//!      streaming code the file watcher runs),
//!   2. ContextEngineClient.{index_status,search,graph_query} — the exact calls
//!      the agent's codebase_search / codebase_graph tools make.
//!
//! Usage:  cargo run --example ce_smoke -- <repo_path> [base_url]
//! Requires the stack up (docker compose up -d) with an embedding key set.

use std::path::Path;
use std::sync::Arc;

use agent::context_engine::{ContextEngineApi, ContextEngineClient, ContextEngineConfig};
use context_sync::{IgnoreFilter, Streamer, StreamerConfig};
use tokio_util::sync::CancellationToken;

#[tokio::main]
async fn main() {
    let mut args = std::env::args().skip(1);
    let repo_arg = args.next().expect("usage: ce_smoke <repo_path> [base_url]");
    let base_url = args.next().unwrap_or_else(|| "http://127.0.0.1:8106".to_string());

    // Canonicalize so index + query use the identical repo_path key.
    let repo_path = std::fs::canonicalize(&repo_arg)
        .expect("repo path must exist")
        .to_string_lossy()
        .to_string();

    let machine_id = "smoke-harness".to_string();
    println!("== full_sync {repo_path} via {base_url} ==");

    // The streamer talks to {url}/index/... — pass the /api/v1 base (same as
    // WatcherManager does when it builds the streamer).
    let streamer = Streamer::new(StreamerConfig {
        context_engine_url: format!("{base_url}/api/v1"),
        user_id: "local".to_string(),
        workspace_id: 0,
        machine_id: machine_id.clone(),
        repo_path: repo_path.clone(),
        auth_token: String::new(),
    });
    let filter = Arc::new(IgnoreFilter::new(Path::new(&repo_path)));
    match streamer
        .full_sync(Path::new(&repo_path), filter, CancellationToken::new())
        .await
    {
        Ok(stats) => println!(
            "  indexed: hashed={} uploaded={} deleted={} bytes={} dup_batches={} in {:?}",
            stats.files_hashed,
            stats.files_uploaded,
            stats.files_deleted,
            stats.bytes_uploaded,
            stats.duplicate_batches,
            stats.duration,
        ),
        Err(e) => {
            println!("  full_sync ERROR: {e}");
            return;
        }
    }

    let client = ContextEngineClient::new(ContextEngineConfig {
        base_url,
        user_id: "local".to_string(),
        workspace_id: 0,
        machine_id,
        repo_path,
        auth_token: String::new(),
    });

    println!("\n== index_status ==");
    match client.index_status().await {
        Ok(s) => println!("  exists={} empty={} collection={:?} repo_id={:?}", s.exists, s.empty, s.collection_name, s.repo_id),
        Err(e) => println!("  ERROR: {e}"),
    }

    for q in ["create a git branch", "parse remote url", "restore checkpoint"] {
        println!("\n== search: {q:?} ==");
        match client.search(q, Some("multi"), Some(5)).await {
            Ok(r) => {
                println!("  total={} indexing={} message={:?}", r.total, r.indexing, r.message);
                for (i, item) in r.results.iter().take(5).enumerate() {
                    println!("  [{i}] {} (lang={}, score={:.3}, src={})", item.file_path, item.language, item.score, item.source);
                }
            }
            Err(e) => println!("  ERROR: {e}"),
        }
    }

    println!("\n== graph_query: dependencies of open_diff ==");
    match client.graph_query(None, Some("open_diff"), None, Some("dependencies")).await {
        Ok(r) => {
            println!("  total={} indexing={} message={:?}", r.total, r.indexing, r.message);
            for (i, item) in r.results.iter().take(8).enumerate() {
                println!("  [{i}] {} @ {} (depth={}, dir={:?})", item.name, item.file_path, item.depth, item.direction);
            }
        }
        Err(e) => println!("  ERROR: {e}"),
    }
}
