use std::path::Path;

use crate::error::GitOpsError;
use crate::exec::{run_git, run_git_raw};
use crate::types::DiffOutput;

/// Info about a captured checkpoint.
#[derive(Debug, Clone)]
pub struct CheckpointInfo {
    pub ref_name: String,
    pub commit_sha: String,
    pub turn_count: u32,
    pub created_at: String, // ISO 8601 from git creatordate
}

/// Generate the git ref path for a checkpoint.
/// Format: `refs/agent/checkpoints/{thread_id}/turn-{turn}`
pub fn checkpoint_ref(thread_id: &str, turn: u32) -> String {
    format!("refs/agent/checkpoints/{}/turn-{}", thread_id, turn)
}

/// Check if a checkpoint ref exists in the repository.
pub async fn has_checkpoint(
    repo_path: &Path,
    ref_name: &str,
) -> Result<bool, GitOpsError> {
    let output = run_git_raw(repo_path, &["rev-parse", "--verify", ref_name]).await?;
    Ok(output.exit_code == 0)
}

/// Capture the current worktree state as a hidden git ref.
///
/// Performs:
///   1. git add -A (stage everything including untracked)
///   2. git write-tree -> tree SHA
///   3. git rev-parse HEAD -> parent SHA
///   4. git commit-tree {tree} -p {parent} -m "checkpoint: {ref_name}" -> commit SHA
///   5. git update-ref {ref_name} {commit_sha}
///
/// Returns the commit SHA of the checkpoint.
pub async fn capture_checkpoint(
    repo_path: &Path,
    ref_name: &str,
) -> Result<String, GitOpsError> {
    // 1. Stage everything (including untracked files)
    run_git(repo_path, &["add", "-A"]).await?;

    // 2. Write the current index as a tree object
    let tree_output = run_git(repo_path, &["write-tree"]).await?;
    let tree_sha = tree_output.stdout.trim().to_string();

    // 3. Get the current HEAD as parent
    let head_output = run_git(repo_path, &["rev-parse", "HEAD"]).await?;
    let parent_sha = head_output.stdout.trim().to_string();

    // 4. Create a commit object (dangling — not on any branch)
    let msg = format!("checkpoint: {}", ref_name);
    let commit_output = run_git(
        repo_path,
        &["commit-tree", &tree_sha, "-p", &parent_sha, "-m", &msg],
    )
    .await?;
    let commit_sha = commit_output.stdout.trim().to_string();

    // 5. Point the ref at the new commit
    run_git(repo_path, &["update-ref", ref_name, &commit_sha]).await?;

    // NOTE: We intentionally do NOT run `git reset HEAD` here.
    // The staged state from `git add -A` is harmless — the agent's tools (write/edit/bash)
    // work on the filesystem directly, not via git staging. And final diffs use
    // checkpoint-based refs, not `git diff` (unstaged).
    // Running `git reset HEAD` would unstage new files, which breaks the LLM's
    // `git add` + `git commit` workflow (the checkpoint fires between tool calls
    // and would unstage files the LLM just staged).

    Ok(commit_sha)
}

/// Compute the diff between two checkpoint refs.
/// Returns a DiffOutput with raw unified diff text + stats.
pub async fn diff_checkpoints(
    repo_path: &Path,
    from_ref: &str,
    to_ref: &str,
) -> Result<DiffOutput, GitOpsError> {
    let range = format!("{}..{}", from_ref, to_ref);
    crate::core::diff(repo_path, None, false, Some(&range)).await
}

/// Restore the worktree to the state of a checkpoint.
///
/// Steps:
///   1. Capture a pre-restore snapshot at {ref_name}-pre-restore
///   2. git checkout {ref_name} -- .  (restore files without moving branch)
///
/// The pre-restore snapshot ensures recovery is always possible.
pub async fn restore_checkpoint(
    repo_path: &Path,
    ref_name: &str,
) -> Result<(), GitOpsError> {
    // 1. Safety: capture the current state before restoring
    let pre_restore_ref = format!("{}-pre-restore", ref_name);
    capture_checkpoint(repo_path, &pre_restore_ref).await?;

    // 2. Restore working tree to match the checkpoint exactly.
    //    read-tree sets the index to the checkpoint's tree.
    //    checkout-index writes all index entries to the working tree.
    //    clean -fd removes working tree files no longer in the index.
    //    This handles modifications, deletions, AND additions correctly
    //    (unlike `git checkout <ref> -- .` which leaves added files behind).
    run_git(repo_path, &["read-tree", ref_name]).await?;
    run_git(repo_path, &["checkout-index", "-f", "-a"]).await?;
    run_git(repo_path, &["clean", "-fd"]).await?;

    Ok(())
}

/// Extract the turn number from a checkpoint ref name.
/// Expected format: `refs/agent/checkpoints/{thread_id}/turn-{N}`
/// Also handles pre-restore refs like `turn-2-pre-restore`.
fn parse_turn_from_ref(ref_name: &str) -> Option<u32> {
    let last_segment = ref_name.rsplit('/').next()?;
    let turn_part = last_segment.strip_prefix("turn-")?;
    // Handle "turn-5" or "turn-5-pre-restore"
    let num_str = turn_part.split('-').next()?;
    num_str.parse().ok()
}

/// Delete all checkpoint refs for a thread from a given turn onward.
/// Used after restore to clean up stale future checkpoints.
/// Also deletes associated pre-restore snapshots (e.g. `turn-2-pre-restore`
/// is treated as belonging to turn 2 and deleted when `from_turn <= 2`).
/// Returns the number of refs deleted.
pub async fn delete_checkpoint_refs(
    repo_path: &Path,
    thread_id: &str,
    from_turn: u32,
) -> Result<u32, GitOpsError> {
    let prefix = format!("refs/agent/checkpoints/{}/", thread_id);
    let output = run_git(repo_path, &["for-each-ref", "--format=%(refname)", &prefix]).await?;

    let mut deleted = 0u32;
    for line in output.stdout.lines() {
        let line = line.trim();
        if line.is_empty() {
            continue;
        }
        if let Some(turn) = parse_turn_from_ref(line) {
            if turn >= from_turn {
                run_git(repo_path, &["update-ref", "-d", line]).await?;
                deleted += 1;
            }
        }
    }

    Ok(deleted)
}

/// List all checkpoints for a thread, ordered by turn count.
/// Excludes pre-restore snapshots from the listing.
pub async fn list_checkpoints(
    repo_path: &Path,
    thread_id: &str,
) -> Result<Vec<CheckpointInfo>, GitOpsError> {
    let prefix = format!("refs/agent/checkpoints/{}/", thread_id);
    let output = run_git(
        repo_path,
        &[
            "for-each-ref",
            "--format=%(refname) %(objectname:short) %(creatordate:iso-strict)",
            "--sort=refname",
            &prefix,
        ],
    )
    .await?;

    let mut checkpoints = Vec::new();
    for line in output.stdout.lines() {
        let line = line.trim();
        if line.is_empty() {
            continue;
        }

        // Skip pre-restore snapshots
        if line.contains("-pre-restore") {
            continue;
        }

        let parts: Vec<&str> = line.splitn(3, ' ').collect();
        if parts.len() < 3 {
            continue; // Malformed line, skip
        }

        let ref_name = parts[0].to_string();
        let commit_sha = parts[1].to_string();
        let created_at = parts[2].to_string();
        let turn_count = parse_turn_from_ref(&ref_name).unwrap_or(0);

        checkpoints.push(CheckpointInfo {
            ref_name,
            commit_sha,
            turn_count,
            created_at,
        });
    }

    // Sort numerically by turn count (--sort=refname is lexicographic)
    checkpoints.sort_by_key(|c| c.turn_count);

    Ok(checkpoints)
}

/// Delete ALL checkpoint refs for a thread.
/// Called when a coding session is completed and worktree is removed.
pub async fn delete_all_checkpoints(
    repo_path: &Path,
    thread_id: &str,
) -> Result<u32, GitOpsError> {
    delete_checkpoint_refs(repo_path, thread_id, 0).await
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::test_util::init_repo_with_commit;
    use std::fs;
    use tempfile::tempdir;

    #[test]
    fn test_checkpoint_ref_format() {
        assert_eq!(
            checkpoint_ref("thread-abc", 0),
            "refs/agent/checkpoints/thread-abc/turn-0"
        );
        assert_eq!(
            checkpoint_ref("thread-abc", 5),
            "refs/agent/checkpoints/thread-abc/turn-5"
        );
    }

    #[tokio::test]
    async fn test_capture_checkpoint_creates_ref() {
        let dir = tempdir().unwrap();
        init_repo_with_commit(dir.path()).await;

        let ref_name = checkpoint_ref("t1", 0);
        let sha = capture_checkpoint(dir.path(), &ref_name).await.unwrap();

        // SHA should be a 40-char hex string
        assert_eq!(sha.len(), 40);
        assert!(sha.chars().all(|c| c.is_ascii_hexdigit()));

        // The ref should exist
        assert!(has_checkpoint(dir.path(), &ref_name).await.unwrap());
    }

    #[tokio::test]
    async fn test_capture_checkpoint_captures_uncommitted_changes() {
        let dir = tempdir().unwrap();
        init_repo_with_commit(dir.path()).await;

        // State 1: modify the file
        fs::write(dir.path().join("README.md"), "# State One").unwrap();
        let ref0 = checkpoint_ref("t1", 0);
        capture_checkpoint(dir.path(), &ref0).await.unwrap();

        // State 2: modify the file again (don't commit)
        fs::write(dir.path().join("README.md"), "# State Two").unwrap();
        let ref1 = checkpoint_ref("t1", 1);
        capture_checkpoint(dir.path(), &ref1).await.unwrap();

        // Diff between the two checkpoints should show the change
        let d = diff_checkpoints(dir.path(), &ref0, &ref1).await.unwrap();
        assert!(d.diff.contains("State Two"));
        assert!(d.diff.contains("State One"));
    }

    #[tokio::test]
    async fn test_has_checkpoint_false_for_missing() {
        let dir = tempdir().unwrap();
        init_repo_with_commit(dir.path()).await;

        let result = has_checkpoint(dir.path(), "refs/agent/checkpoints/nonexistent/turn-0")
            .await
            .unwrap();
        assert!(!result);
    }

    #[tokio::test]
    async fn test_diff_checkpoints_shows_changes() {
        let dir = tempdir().unwrap();
        init_repo_with_commit(dir.path()).await;

        let ref0 = checkpoint_ref("t1", 0);
        capture_checkpoint(dir.path(), &ref0).await.unwrap();

        // Write a new file between checkpoints
        fs::write(dir.path().join("new.txt"), "new file content").unwrap();
        let ref1 = checkpoint_ref("t1", 1);
        capture_checkpoint(dir.path(), &ref1).await.unwrap();

        let d = diff_checkpoints(dir.path(), &ref0, &ref1).await.unwrap();
        assert!(d.diff.contains("new.txt"));
        assert!(d.insertions > 0);
    }

    #[tokio::test]
    async fn test_diff_checkpoints_no_changes() {
        let dir = tempdir().unwrap();
        init_repo_with_commit(dir.path()).await;

        let ref0 = checkpoint_ref("t1", 0);
        capture_checkpoint(dir.path(), &ref0).await.unwrap();

        // No edits between captures
        let ref1 = checkpoint_ref("t1", 1);
        capture_checkpoint(dir.path(), &ref1).await.unwrap();

        let d = diff_checkpoints(dir.path(), &ref0, &ref1).await.unwrap();
        assert!(d.diff.is_empty());
        assert_eq!(d.insertions, 0);
        assert_eq!(d.deletions, 0);
    }

    #[tokio::test]
    async fn test_restore_checkpoint() {
        let dir = tempdir().unwrap();
        init_repo_with_commit(dir.path()).await;

        // Capture state with "hello"
        fs::write(dir.path().join("data.txt"), "hello").unwrap();
        let ref0 = checkpoint_ref("t1", 0);
        capture_checkpoint(dir.path(), &ref0).await.unwrap();

        // Overwrite to "world" and capture
        fs::write(dir.path().join("data.txt"), "world").unwrap();
        let ref1 = checkpoint_ref("t1", 1);
        capture_checkpoint(dir.path(), &ref1).await.unwrap();

        // Verify current state
        assert_eq!(
            fs::read_to_string(dir.path().join("data.txt")).unwrap(),
            "world"
        );

        // Restore to turn-0
        restore_checkpoint(dir.path(), &ref0).await.unwrap();

        // File should be back to "hello"
        assert_eq!(
            fs::read_to_string(dir.path().join("data.txt")).unwrap(),
            "hello"
        );
    }

    #[tokio::test]
    async fn test_restore_creates_pre_restore_snapshot() {
        let dir = tempdir().unwrap();
        init_repo_with_commit(dir.path()).await;

        let ref0 = checkpoint_ref("t1", 0);
        capture_checkpoint(dir.path(), &ref0).await.unwrap();

        fs::write(dir.path().join("file.txt"), "some content").unwrap();
        let ref1 = checkpoint_ref("t1", 1);
        capture_checkpoint(dir.path(), &ref1).await.unwrap();

        // Restore to turn-0
        restore_checkpoint(dir.path(), &ref0).await.unwrap();

        // Pre-restore snapshot should exist
        let pre_restore_ref = format!("{}-pre-restore", ref0);
        assert!(has_checkpoint(dir.path(), &pre_restore_ref).await.unwrap());
    }

    #[tokio::test]
    async fn test_delete_checkpoint_refs_from_turn() {
        let dir = tempdir().unwrap();
        init_repo_with_commit(dir.path()).await;

        let thread_id = "t1";
        // Create 4 checkpoints, modifying a file between each
        for turn in 0..4u32 {
            fs::write(dir.path().join("counter.txt"), format!("turn-{}", turn)).unwrap();
            let r = checkpoint_ref(thread_id, turn);
            capture_checkpoint(dir.path(), &r).await.unwrap();
        }

        // Delete from turn 2 onward
        let deleted = delete_checkpoint_refs(dir.path(), thread_id, 2).await.unwrap();
        assert_eq!(deleted, 2);

        // turn-0 and turn-1 should still exist
        assert!(has_checkpoint(dir.path(), &checkpoint_ref(thread_id, 0)).await.unwrap());
        assert!(has_checkpoint(dir.path(), &checkpoint_ref(thread_id, 1)).await.unwrap());

        // turn-2 and turn-3 should be gone
        assert!(!has_checkpoint(dir.path(), &checkpoint_ref(thread_id, 2)).await.unwrap());
        assert!(!has_checkpoint(dir.path(), &checkpoint_ref(thread_id, 3)).await.unwrap());
    }

    #[tokio::test]
    async fn test_list_checkpoints_ordered() {
        let dir = tempdir().unwrap();
        init_repo_with_commit(dir.path()).await;

        let thread_id = "t1";
        for turn in 0..3u32 {
            fs::write(dir.path().join("counter.txt"), format!("turn-{}", turn)).unwrap();
            let r = checkpoint_ref(thread_id, turn);
            capture_checkpoint(dir.path(), &r).await.unwrap();
        }

        let checkpoints = list_checkpoints(dir.path(), thread_id).await.unwrap();
        assert_eq!(checkpoints.len(), 3);
        assert_eq!(checkpoints[0].turn_count, 0);
        assert_eq!(checkpoints[1].turn_count, 1);
        assert_eq!(checkpoints[2].turn_count, 2);

        // Each should have a non-empty SHA and ISO date
        for cp in &checkpoints {
            assert!(!cp.commit_sha.is_empty());
            assert!(!cp.created_at.is_empty());
        }
    }

    #[tokio::test]
    async fn test_delete_all_checkpoints() {
        let dir = tempdir().unwrap();
        init_repo_with_commit(dir.path()).await;

        let thread_id = "t1";
        for turn in 0..2u32 {
            fs::write(dir.path().join("counter.txt"), format!("turn-{}", turn)).unwrap();
            let r = checkpoint_ref(thread_id, turn);
            capture_checkpoint(dir.path(), &r).await.unwrap();
        }

        let deleted = delete_all_checkpoints(dir.path(), thread_id).await.unwrap();
        assert_eq!(deleted, 2);

        let remaining = list_checkpoints(dir.path(), thread_id).await.unwrap();
        assert!(remaining.is_empty());
    }

    #[tokio::test]
    async fn test_restore_removes_files_added_after_checkpoint() {
        let dir = tempdir().unwrap();
        init_repo_with_commit(dir.path()).await;

        // Capture turn-0 (only README.md exists)
        let ref0 = checkpoint_ref("t1", 0);
        capture_checkpoint(dir.path(), &ref0).await.unwrap();

        // Create a new file after the checkpoint
        fs::write(dir.path().join("added_later.txt"), "I was added later").unwrap();
        let ref1 = checkpoint_ref("t1", 1);
        capture_checkpoint(dir.path(), &ref1).await.unwrap();
        assert!(dir.path().join("added_later.txt").exists());

        // Restore to turn-0 — the added file should be REMOVED
        restore_checkpoint(dir.path(), &ref0).await.unwrap();
        assert!(
            !dir.path().join("added_later.txt").exists(),
            "File added after checkpoint should be removed on restore"
        );
        // Original file should still be there
        assert!(dir.path().join("README.md").exists());
    }

    #[tokio::test]
    async fn test_capture_checkpoint_includes_untracked_files() {
        let dir = tempdir().unwrap();
        init_repo_with_commit(dir.path()).await;

        // Create an untracked file (NOT committed, NOT staged)
        fs::write(dir.path().join("untracked.txt"), "i am untracked").unwrap();

        let ref0 = checkpoint_ref("t1", 0);
        capture_checkpoint(dir.path(), &ref0).await.unwrap();

        // Delete the untracked file
        fs::remove_file(dir.path().join("untracked.txt")).unwrap();
        assert!(!dir.path().join("untracked.txt").exists());

        // Restore to turn-0 should bring it back
        restore_checkpoint(dir.path(), &ref0).await.unwrap();
        assert!(dir.path().join("untracked.txt").exists());
        assert_eq!(
            fs::read_to_string(dir.path().join("untracked.txt")).unwrap(),
            "i am untracked"
        );
    }
}
