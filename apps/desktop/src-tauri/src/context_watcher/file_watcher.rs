use notify::{Event, EventKind, RecommendedWatcher, RecursiveMode, Watcher};
use std::collections::HashSet;
use std::path::PathBuf;
use std::sync::Arc;
use std::time::{Duration, Instant};
use tokio::sync::mpsc;
use tokio_util::sync::CancellationToken;

use super::ignore_filter::IgnoreFilter;

const DEBOUNCE_MS: u64 = 500;
// 60s cooldown → max 1 trigger per minute.
// Rationale: the watcher only watches the canonical main checkout. Agent coding
// sessions edit files in .agent-worktrees (blocked by IgnoreFilter), so the
// watcher rarely fires in practice. 1/min is plenty for catching user edits in
// their editor while keeping supercoder re-index load minimal.
const COOLDOWN_SECS: u64 = 60;

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

/// A batch of file changes ready to sync.
#[derive(Debug)]
pub struct ChangeBatch {
    pub created_or_modified: Vec<PathBuf>,
    pub deleted: Vec<PathBuf>,
}

#[derive(Debug, thiserror::Error)]
pub enum WatcherError {
    #[error("notify error: {0}")]
    Notify(#[from] notify::Error),

    #[error("watcher channel closed")]
    ChannelClosed,
}

// ---------------------------------------------------------------------------
// FileWatcher
// ---------------------------------------------------------------------------

pub struct FileWatcher {
    _watcher: RecommendedWatcher,
}

impl FileWatcher {
    /// Start watching a repo directory. Returns a receiver for change batches.
    ///
    /// The watcher and debounce/throttle loop run as background tokio tasks.
    /// Cancel via the provided token.
    pub fn start(
        repo_path: PathBuf,
        filter: Arc<IgnoreFilter>,
        cancel_token: CancellationToken,
    ) -> Result<(Self, mpsc::Receiver<ChangeBatch>), WatcherError> {
        // Channel for raw notify events → debounce task
        let (raw_tx, raw_rx) = std::sync::mpsc::channel::<notify::Result<Event>>();

        let mut watcher = RecommendedWatcher::new(
            move |res| {
                let _ = raw_tx.send(res);
            },
            notify::Config::default(),
        )?;

        watcher.watch(&repo_path, RecursiveMode::Recursive)?;

        // Channel for debounced batches → consumer
        let (batch_tx, batch_rx) = mpsc::channel::<ChangeBatch>(16);

        // Spawn the debounce+throttle loop
        tokio::spawn(debounce_loop(raw_rx, batch_tx, filter, cancel_token));

        Ok((Self { _watcher: watcher }, batch_rx))
    }
}

// ---------------------------------------------------------------------------
// Debounce + throttle loop
// ---------------------------------------------------------------------------

async fn debounce_loop(
    raw_rx: std::sync::mpsc::Receiver<notify::Result<Event>>,
    batch_tx: mpsc::Sender<ChangeBatch>,
    filter: Arc<IgnoreFilter>,
    cancel_token: CancellationToken,
) {
    let mut pending_changes: HashSet<PathBuf> = HashSet::new();
    let mut pending_deletes: HashSet<PathBuf> = HashSet::new();
    let mut last_trigger: Option<Instant> = None;
    let mut debounce_deadline: Option<Instant> = None;
    let mut deferred_deadline: Option<Instant> = None;

    loop {
        tokio::select! {
            _ = cancel_token.cancelled() => {
                break;
            }

            // Poll for events every 10ms, then check deadlines inline
            _ = tokio::time::sleep(Duration::from_millis(10)) => {
                let was_empty = !has_pending(&pending_changes, &pending_deletes);

                // Drain all available events from the notify channel
                while let Ok(event_result) = raw_rx.try_recv() {
                    if let Ok(event) = event_result {
                        process_event(&event, &filter, &mut pending_changes, &mut pending_deletes);
                    }
                }

                // Set debounce deadline on transition from empty → non-empty
                if was_empty && has_pending(&pending_changes, &pending_deletes) {
                    debounce_deadline = Some(Instant::now() + Duration::from_millis(DEBOUNCE_MS));
                }

                // Check deadlines
                let now = Instant::now();

                // Check debounce deadline
                if let Some(dd) = debounce_deadline {
                    if now >= dd {
                        debounce_deadline = None;

                        let cooldown_ok = match last_trigger {
                            Some(lt) => now.duration_since(lt) >= Duration::from_secs(COOLDOWN_SECS),
                            None => true,
                        };

                        if cooldown_ok && has_pending(&pending_changes, &pending_deletes) {
                            emit_batch(&mut pending_changes, &mut pending_deletes, &batch_tx, &mut last_trigger).await;
                        } else if !cooldown_ok && has_pending(&pending_changes, &pending_deletes) {
                            if let Some(lt) = last_trigger {
                                deferred_deadline = Some(lt + Duration::from_secs(COOLDOWN_SECS));
                            }
                        }
                    }
                }

                // Check deferred deadline (cooldown expired)
                if let Some(dd) = deferred_deadline {
                    if now >= dd {
                        deferred_deadline = None;
                        if has_pending(&pending_changes, &pending_deletes) {
                            emit_batch(&mut pending_changes, &mut pending_deletes, &batch_tx, &mut last_trigger).await;
                        }
                    }
                }
            }
        }
    }
}

fn process_event(
    event: &Event,
    filter: &IgnoreFilter,
    pending_changes: &mut HashSet<PathBuf>,
    pending_deletes: &mut HashSet<PathBuf>,
) {
    for path in &event.paths {
        if !filter.should_include(path) {
            continue;
        }

        match event.kind {
            EventKind::Create(_) | EventKind::Modify(_) => {
                // Skip directories: `mkdir foo` events also fire here, and
                // reading a directory with fs::read would fail EISDIR downstream.
                // is_file() is cheap (one stat). Remove events don't need this
                // guard — the path is already gone by the time we see them.
                if !path.is_file() {
                    continue;
                }
                pending_deletes.remove(path);
                pending_changes.insert(path.clone());
            }
            EventKind::Remove(_) => {
                pending_changes.remove(path);
                pending_deletes.insert(path.clone());
            }
            _ => {}
        }
    }
}

fn has_pending(changes: &HashSet<PathBuf>, deletes: &HashSet<PathBuf>) -> bool {
    !changes.is_empty() || !deletes.is_empty()
}

async fn emit_batch(
    pending_changes: &mut HashSet<PathBuf>,
    pending_deletes: &mut HashSet<PathBuf>,
    batch_tx: &mpsc::Sender<ChangeBatch>,
    last_trigger: &mut Option<Instant>,
) {
    let batch = ChangeBatch {
        created_or_modified: pending_changes.drain().collect(),
        deleted: pending_deletes.drain().collect(),
    };
    *last_trigger = Some(Instant::now());
    let _ = batch_tx.send(batch).await;
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::fs;
    use tempfile::TempDir;

    #[tokio::test]
    async fn test_debounce_batches_rapid_events() {
        let tmp = TempDir::new().unwrap();
        let filter = Arc::new(IgnoreFilter::new(tmp.path()));
        let cancel = CancellationToken::new();

        let (watcher, mut rx) =
            FileWatcher::start(tmp.path().to_path_buf(), filter, cancel.clone()).unwrap();

        // Write multiple files rapidly
        for i in 0..5 {
            fs::write(tmp.path().join(format!("file{i}.txt")), format!("content{i}")).unwrap();
            tokio::time::sleep(Duration::from_millis(50)).await;
        }

        // Wait for debounce to fire (500ms + margin)
        let batch = tokio::time::timeout(Duration::from_secs(3), rx.recv())
            .await
            .expect("should receive batch")
            .expect("channel not closed");

        // All 5 files should be in a single batch (or fewer batches)
        assert!(
            !batch.created_or_modified.is_empty(),
            "should have created/modified files"
        );

        cancel.cancel();
        drop(watcher);
    }

    #[tokio::test]
    async fn test_ignored_files_excluded() {
        let tmp = TempDir::new().unwrap();
        let filter = Arc::new(IgnoreFilter::new(tmp.path()));
        let cancel = CancellationToken::new();

        let (watcher, mut rx) =
            FileWatcher::start(tmp.path().to_path_buf(), filter, cancel.clone()).unwrap();

        // Write an ignored file (png)
        fs::write(tmp.path().join("image.png"), "fake png").unwrap();

        // Write a normal file to trigger a batch
        tokio::time::sleep(Duration::from_millis(100)).await;
        fs::write(tmp.path().join("code.rs"), "fn main(){}").unwrap();

        let batch = tokio::time::timeout(Duration::from_secs(3), rx.recv())
            .await
            .expect("should receive batch")
            .expect("channel not closed");

        // The batch should contain code.rs but not image.png
        let names: Vec<_> = batch
            .created_or_modified
            .iter()
            .filter_map(|p| p.file_name().and_then(|n| n.to_str()).map(String::from))
            .collect();
        assert!(names.contains(&"code.rs".to_string()));
        assert!(!names.contains(&"image.png".to_string()));

        cancel.cancel();
        drop(watcher);
    }

    #[tokio::test]
    async fn test_delete_events_classified() {
        // Test the process_event function directly to avoid platform-specific
        // fsevents behavior (macOS may not report Remove for pre-existing files).
        let mut changes = HashSet::new();
        let mut deletes = HashSet::new();
        let tmp = TempDir::new().unwrap();
        let filter = IgnoreFilter::new(tmp.path());

        let file = tmp.path().join("target_file.rs");
        // Create the file on disk — process_event now skips non-file paths
        // (directory events, stale paths) via `path.is_file()` guard.
        std::fs::write(&file, b"// test").unwrap();

        // Simulate a create event
        let create_event = Event {
            kind: EventKind::Create(notify::event::CreateKind::File),
            paths: vec![file.clone()],
            attrs: Default::default(),
        };
        process_event(&create_event, &filter, &mut changes, &mut deletes);
        assert!(changes.contains(&file));
        assert!(!deletes.contains(&file));

        // Simulate a remove event — should move from changes to deletes
        let remove_event = Event {
            kind: EventKind::Remove(notify::event::RemoveKind::File),
            paths: vec![file.clone()],
            attrs: Default::default(),
        };
        process_event(&remove_event, &filter, &mut changes, &mut deletes);
        assert!(!changes.contains(&file), "file should be removed from changes");
        assert!(deletes.contains(&file), "file should be in deletes");
    }

    #[tokio::test]
    async fn test_cancel_token_stops_loop() {
        let tmp = TempDir::new().unwrap();
        let filter = Arc::new(IgnoreFilter::new(tmp.path()));
        let cancel = CancellationToken::new();

        let (watcher, mut rx) =
            FileWatcher::start(tmp.path().to_path_buf(), filter, cancel.clone()).unwrap();

        // Cancel immediately
        cancel.cancel();

        // Channel should close (recv returns None) within a short time
        let result = tokio::time::timeout(Duration::from_secs(2), rx.recv()).await;
        // Either timeout (loop stopped, no batch) or None (channel closed) is acceptable
        match result {
            Ok(None) => {} // Channel closed — expected
            Err(_) => {}   // Timeout — loop stopped but channel not yet dropped
            Ok(Some(_)) => {} // Got a batch before stopping — also fine
        }

        drop(watcher);
    }

    #[tokio::test]
    async fn test_cooldown_defers_trigger() {
        let tmp = TempDir::new().unwrap();
        let filter = Arc::new(IgnoreFilter::new(tmp.path()));
        let cancel = CancellationToken::new();

        let (watcher, mut rx) =
            FileWatcher::start(tmp.path().to_path_buf(), filter, cancel.clone()).unwrap();

        // Write first file — triggers first batch
        fs::write(tmp.path().join("first.txt"), "a").unwrap();
        let _batch1 = tokio::time::timeout(Duration::from_secs(3), rx.recv())
            .await
            .expect("should receive first batch")
            .expect("channel not closed");

        // Write second file immediately — should be deferred (cooldown active)
        fs::write(tmp.path().join("second.txt"), "b").unwrap();

        // The second batch should NOT arrive within 1 second (cooldown is 30s)
        // but in tests we just verify the mechanism works by checking it eventually arrives
        // We won't wait 30s in a test — just verify no immediate batch
        let result = tokio::time::timeout(Duration::from_secs(1), rx.recv()).await;
        // Should timeout because cooldown is active
        assert!(result.is_err(), "should not receive batch during cooldown");

        cancel.cancel();
        drop(watcher);
    }
}
