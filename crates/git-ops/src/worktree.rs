use std::path::Path;

use crate::error::GitOpsError;
use crate::exec::run_git;

/// Add a git worktree.
pub async fn worktree_add(
    repo_path: &Path,
    worktree_path: &Path,
    branch: &str,
    create_branch: bool,
    start_point: Option<&str>,
) -> Result<(), GitOpsError> {
    let worktree_str = worktree_path.display().to_string();
    let mut args = vec!["worktree", "add"];
    if create_branch {
        args.extend_from_slice(&["-b", branch, &worktree_str]);
    } else {
        args.extend_from_slice(&[&worktree_str, branch]);
    }
    // Start point: create branch FROM this ref (e.g., "main", "develop")
    if let Some(sp) = start_point {
        args.push(sp);
    }
    run_git(repo_path, &args).await?;
    Ok(())
}

/// Remove a git worktree.
pub async fn worktree_remove(repo_path: &Path, worktree_path: &Path) -> Result<(), GitOpsError> {
    let worktree_str = worktree_path.display().to_string();
    run_git(repo_path, &["worktree", "remove", &worktree_str, "--force"]).await?;
    Ok(())
}

/// A parsed entry from `git worktree list --porcelain`.
#[derive(Debug, Clone)]
pub struct WorktreeEntry {
    pub path: std::path::PathBuf,
    pub branch: Option<String>,
    pub bare: bool,
}

/// List all git worktrees by parsing `git worktree list --porcelain`.
pub async fn worktree_list(repo_path: &Path) -> Result<Vec<WorktreeEntry>, GitOpsError> {
    let output = run_git(repo_path, &["worktree", "list", "--porcelain"]).await?;
    Ok(parse_worktree_porcelain(&output.stdout))
}

/// Parse porcelain output from `git worktree list --porcelain`.
fn parse_worktree_porcelain(output: &str) -> Vec<WorktreeEntry> {
    let mut entries = Vec::new();
    let mut path: Option<std::path::PathBuf> = None;
    let mut branch: Option<String> = None;
    let mut bare = false;

    for line in output.lines() {
        if let Some(p) = line.strip_prefix("worktree ") {
            // Flush previous entry
            if let Some(prev_path) = path.take() {
                entries.push(WorktreeEntry { path: prev_path, branch: branch.take(), bare });
                bare = false;
            }
            path = Some(std::path::PathBuf::from(p));
        } else if let Some(b) = line.strip_prefix("branch refs/heads/") {
            branch = Some(b.to_string());
        } else if line == "bare" {
            bare = true;
        }
        // Skip HEAD, detached, prunable lines — we don't need them
    }
    // Flush last entry
    if let Some(prev_path) = path {
        entries.push(WorktreeEntry { path: prev_path, branch: branch.take(), bare });
    }
    entries
}

/// Prune stale worktree entries.
pub async fn worktree_prune(repo_path: &Path) -> Result<(), GitOpsError> {
    run_git(repo_path, &["worktree", "prune"]).await?;
    Ok(())
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::fs;
    use tempfile::tempdir;
    use tokio::process::Command;
    use crate::test_util::init_repo_with_commit;

    #[tokio::test]
    async fn test_worktree_add_new_branch() {
        let dir = tempdir().unwrap();
        init_repo_with_commit(dir.path()).await;

        let wt_path = dir.path().join("worktree-feature");
        worktree_add(dir.path(), &wt_path, "feature-wt", true, None)
            .await
            .unwrap();

        // Worktree dir should exist
        assert!(wt_path.exists());
        // Should have a README.md from the parent
        assert!(wt_path.join("README.md").exists());
    }

    #[tokio::test]
    async fn test_worktree_add_existing_branch() {
        let dir = tempdir().unwrap();
        init_repo_with_commit(dir.path()).await;

        // Create a branch first
        Command::new("git")
            .args(["branch", "existing-branch"])
            .current_dir(dir.path())
            .output()
            .await
            .unwrap();

        let wt_path = dir.path().join("worktree-existing");
        worktree_add(dir.path(), &wt_path, "existing-branch", false, None)
            .await
            .unwrap();

        assert!(wt_path.exists());
    }

    #[tokio::test]
    async fn test_worktree_remove() {
        let dir = tempdir().unwrap();
        init_repo_with_commit(dir.path()).await;

        let wt_path = dir.path().join("worktree-remove");
        worktree_add(dir.path(), &wt_path, "remove-branch", true, None)
            .await
            .unwrap();
        assert!(wt_path.exists());

        worktree_remove(dir.path(), &wt_path).await.unwrap();
        assert!(!wt_path.exists());
    }

    #[test]
    fn test_parse_worktree_porcelain() {
        let output = "\
worktree /Users/me/project
HEAD abc123
branch refs/heads/main

worktree /Users/me/project/.agent-worktrees/session-1
HEAD def456
branch refs/heads/agent/fix-bug-abc12345

worktree /Users/me/project/.agent-worktrees/session-2
HEAD 789012
detached

";
        let entries = parse_worktree_porcelain(output);
        assert_eq!(entries.len(), 3);

        assert_eq!(entries[0].path, std::path::PathBuf::from("/Users/me/project"));
        assert_eq!(entries[0].branch.as_deref(), Some("main"));
        assert!(!entries[0].bare);

        assert_eq!(entries[1].branch.as_deref(), Some("agent/fix-bug-abc12345"));

        // Detached worktree has no branch
        assert!(entries[2].branch.is_none());
    }

    #[tokio::test]
    async fn test_worktree_list() {
        let dir = tempdir().unwrap();
        init_repo_with_commit(dir.path()).await;

        let wt_path = dir.path().join("wt-list-test");
        worktree_add(dir.path(), &wt_path, "list-branch", true, None)
            .await
            .unwrap();

        let entries = worktree_list(dir.path()).await.unwrap();
        // At least 2: the main repo + the worktree we created
        assert!(entries.len() >= 2);
        // macOS: /tmp is a symlink to /private/tmp, git canonicalizes paths
        let wt_canon = wt_path.canonicalize().unwrap();
        assert!(entries.iter().any(|e| e.path == wt_canon));
    }

    #[tokio::test]
    async fn test_worktree_prune_after_manual_delete() {
        let dir = tempdir().unwrap();
        init_repo_with_commit(dir.path()).await;

        let wt_path = dir.path().join("worktree-prune");
        worktree_add(dir.path(), &wt_path, "prune-branch", true, None)
            .await
            .unwrap();

        // Manually delete the worktree directory
        fs::remove_dir_all(&wt_path).unwrap();

        // Prune should succeed (cleans up stale entries)
        worktree_prune(dir.path()).await.unwrap();
    }
}
