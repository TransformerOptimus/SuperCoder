//! context-sync — the index-streaming client extracted from the desktop app's
//! `context_watcher`. Speaks the engine's `/index/diff → /index/stream → /index/sync-complete`
//! protocol (walk→hash, deterministic batch-ids, gzip, base64, caps). Shared by the desktop app
//! (live edits) and the offline `bulk-indexer` (one-shot `full_sync`), so the indexer builds
//! headless without the Tauri GUI deps.

mod ignore_filter;
mod streamer;

pub use ignore_filter::IgnoreFilter;
pub use streamer::{Streamer, StreamerConfig, SyncError, SyncStats};
