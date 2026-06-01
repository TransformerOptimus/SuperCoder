use std::collections::HashMap;
use std::path::{Path, PathBuf};
use std::sync::{Arc, Mutex};

/// Per-working_dir mutex registry for write-capable subagents. Two
/// concurrent write-capable children against the same worktree serialize;
/// read-only children bypass the registry entirely.
///
/// The outer Mutex is a cheap `std::sync::Mutex` because it only guards
/// the HashMap lookup/insert. The actual lock held across a child run is
/// a `tokio::sync::Mutex<()>` so `.lock().await` can be held across
/// `child_loop.run(...).await`.
#[derive(Default)]
pub struct WriteLockRegistry {
    locks: Mutex<HashMap<PathBuf, Arc<tokio::sync::Mutex<()>>>>,
}

impl WriteLockRegistry {
    pub fn new() -> Self {
        Self::default()
    }

    pub fn get_or_create(&self, working_dir: &Path) -> Arc<tokio::sync::Mutex<()>> {
        let mut guard = self.locks.lock().expect("write_lock registry poisoned");
        let existed = guard.contains_key(working_dir);
        let lock = guard
            .entry(working_dir.to_path_buf())
            .or_insert_with(|| Arc::new(tokio::sync::Mutex::new(())))
            .clone();
        if !existed {
            log::info!(
                "[subagents::lock] created new write-mutex for {:?} (registry now holds {} entries)",
                working_dir,
                guard.len()
            );
        }
        lock
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn same_path_returns_same_arc() {
        let reg = WriteLockRegistry::new();
        let a = reg.get_or_create(Path::new("/tmp/repo-a"));
        let b = reg.get_or_create(Path::new("/tmp/repo-a"));
        assert!(Arc::ptr_eq(&a, &b));
    }

    #[test]
    fn different_paths_return_different_arcs() {
        let reg = WriteLockRegistry::new();
        let a = reg.get_or_create(Path::new("/tmp/repo-a"));
        let b = reg.get_or_create(Path::new("/tmp/repo-b"));
        assert!(!Arc::ptr_eq(&a, &b));
    }

    #[tokio::test]
    async fn serializes_same_path() {
        use std::sync::atomic::{AtomicUsize, Ordering};
        use std::time::Duration;

        let reg = Arc::new(WriteLockRegistry::new());
        let counter = Arc::new(AtomicUsize::new(0));
        let max_concurrent = Arc::new(AtomicUsize::new(0));

        let mut tasks = Vec::new();
        for _ in 0..4 {
            let reg = reg.clone();
            let counter = counter.clone();
            let max_concurrent = max_concurrent.clone();
            tasks.push(tokio::spawn(async move {
                let lock = reg.get_or_create(Path::new("/tmp/same"));
                let _guard = lock.lock().await;
                let n = counter.fetch_add(1, Ordering::SeqCst) + 1;
                let mut m = max_concurrent.load(Ordering::SeqCst);
                while n > m {
                    match max_concurrent.compare_exchange(
                        m,
                        n,
                        Ordering::SeqCst,
                        Ordering::SeqCst,
                    ) {
                        Ok(_) => break,
                        Err(cur) => m = cur,
                    }
                }
                tokio::time::sleep(Duration::from_millis(10)).await;
                counter.fetch_sub(1, Ordering::SeqCst);
            }));
        }
        for t in tasks {
            t.await.unwrap();
        }
        assert_eq!(max_concurrent.load(Ordering::SeqCst), 1);
    }
}
