use std::sync::Arc;
use tauri::State;

use super::watcher_manager::{IndexWatcherStatus, WatcherManager};

#[tauri::command]
pub async fn context_watcher_start(
    repo_path: String,
    state: State<'_, Arc<WatcherManager>>,
) -> Result<(), String> {
    let wm = state.inner().clone();
    wm.start_watching(&repo_path).await
}

#[tauri::command]
pub async fn context_watcher_stop(
    repo_path: String,
    state: State<'_, Arc<WatcherManager>>,
) -> Result<(), String> {
    state.stop_watching(&repo_path).await;
    Ok(())
}

#[tauri::command]
pub async fn context_watcher_status(
    repo_path: String,
    state: State<'_, Arc<WatcherManager>>,
) -> Result<Option<IndexWatcherStatus>, String> {
    Ok(state.get_status(&repo_path).await)
}
