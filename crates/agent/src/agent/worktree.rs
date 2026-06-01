use std::path::{Path, PathBuf};

/// Maximum number of agent-managed worktrees per project.
/// When this limit is reached, the oldest worktree is force-removed before creating a new one.
pub const MAX_AGENT_WORKTREES: usize = 10;

/// Information about a created worktree.
#[derive(Debug, Clone)]
pub struct WorktreeInfo {
    pub worktree_path: PathBuf,
    pub branch: String,
    pub session_id: String,
    pub project_path: PathBuf,
}

/// State of an existing worktree.
#[derive(Debug, Clone, PartialEq, Eq)]
pub enum WorktreeState {
    Clean,
    Dirty,
    Missing,
}

#[derive(Debug, thiserror::Error)]
pub enum WorktreeError {
    #[error("Git error: {0}")]
    Git(#[from] git_ops::GitOpsError),
    #[error("IO error: {0}")]
    Io(#[from] std::io::Error),
    #[error("Worktree error: {0}")]
    Other(String),
}

/// Generate a branch name from a task summary: `agent/{slug}-{uuid8}`.
/// Slug is capped at 20 chars (down from 30) so the full branch name stays
/// under ~35 chars — `agent/` prefix + 20-char slug + `-` + 8-char uuid = 35.
/// The uuid suffix stays for collision avoidance when two tasks slugify the
/// same prefix (e.g. two "add a small comment" runs).
pub fn generate_branch_name(task_summary: &str) -> String {
    let slug = slugify(task_summary, 20);
    let uuid_short = &uuid::Uuid::new_v4().to_string()[..8];
    format!("agent/{slug}-{uuid_short}")
}

/// Compute the worktree path for a session: `{project}/.agent-worktrees/{session_id}/`.
pub fn worktree_path(project_path: &Path, session_id: &str) -> PathBuf {
    project_path.join(".agent-worktrees").join(session_id)
}

/// Create a new worktree with its own branch.
/// `base_branch` is the user-selected branch to create the new branch FROM.
/// `branch_name_hint` overrides `task_summary` for branch naming when provided.
pub async fn create_worktree(
    project_path: &Path,
    session_id: &str,
    base_branch: Option<&str>,
    task_summary: &str,
    branch_name_hint: Option<&str>,
) -> Result<WorktreeInfo, WorktreeError> {
    let branch_name = generate_branch_name(branch_name_hint.unwrap_or(task_summary));

    let wt_path = worktree_path(project_path, session_id);

    // Prune stale git worktree registrations before counting
    prune_stale_worktrees(project_path).await.ok();

    // Enforce worktree limit: evict oldest agent worktree if at capacity
    enforce_worktree_limit(project_path).await?;

    // Create parent directory if needed
    if let Some(parent) = wt_path.parent() {
        tokio::fs::create_dir_all(parent).await?;
    }

    // Add agent directories to git's info/exclude (invisible to user, not tracked).
    // .agent-worktrees/ in the main repo so VS Code doesn't show thousands of untracked files.
    crate::util::ensure_git_exclude(project_path, ".agent-worktrees/").await;

    // Create worktree with new branch FROM the user's selected base branch
    git_ops::worktree_add(project_path, &wt_path, &branch_name, true, base_branch).await?;

    // .agent/ in the main repo's info/exclude (git only honors info/exclude from
    // the shared gitdir, not per-worktree private gitdirs). Content inside worktrees
    // is already covered by the .agent-worktrees/ exclusion above.
    crate::util::ensure_git_exclude(project_path, ".agent/").await;

    Ok(WorktreeInfo {
        worktree_path: wt_path,
        branch: branch_name,
        session_id: session_id.to_string(),
        project_path: project_path.to_path_buf(),
    })
}

/// Delete a worktree.
pub async fn delete_worktree(info: &WorktreeInfo) -> Result<(), WorktreeError> {
    git_ops::worktree_remove(&info.project_path, &info.worktree_path).await?;
    Ok(())
}

/// Check the state of a worktree for a given session.
pub async fn check_worktree_state(
    project_path: &Path,
    session_id: &str,
) -> Result<WorktreeState, WorktreeError> {
    let wt_path = worktree_path(project_path, session_id);

    if !wt_path.exists() {
        return Ok(WorktreeState::Missing);
    }

    // Check for uncommitted changes via git status
    let status = git_ops::core::status(&wt_path).await?;
    if status.staged.is_empty()
        && status.unstaged.is_empty()
        && status.untracked.is_empty()
    {
        Ok(WorktreeState::Clean)
    } else {
        Ok(WorktreeState::Dirty)
    }
}

/// Prune stale worktree entries from git.
pub async fn prune_stale_worktrees(project_path: &Path) -> Result<(), WorktreeError> {
    git_ops::worktree_prune(project_path).await?;
    Ok(())
}

/// Enforce the worktree limit by evicting the oldest agent worktree(s) if at capacity.
async fn enforce_worktree_limit(project_path: &Path) -> Result<(), WorktreeError> {
    let agent_wts = list_agent_worktrees(project_path).await?;

    if agent_wts.len() < MAX_AGENT_WORKTREES {
        return Ok(());
    }

    // Sort by creation time (oldest first) — already sorted by list_agent_worktrees
    let to_evict = agent_wts.len() - MAX_AGENT_WORKTREES + 1; // +1 to make room for the new one
    for wt in agent_wts.iter().take(to_evict) {
        log::info!(
            "[worktree] Evicting oldest agent worktree to stay within limit ({}): {}",
            MAX_AGENT_WORKTREES,
            wt.worktree_path.display()
        );
        if let Err(e) = git_ops::worktree_remove(project_path, &wt.worktree_path).await {
            log::warn!("[worktree] Failed to remove worktree {}: {e}", wt.worktree_path.display());
            // Try filesystem removal as fallback
            let _ = tokio::fs::remove_dir_all(&wt.worktree_path).await;
        }
    }

    // Prune again after removal to clean up git metadata
    prune_stale_worktrees(project_path).await.ok();
    Ok(())
}

/// An existing agent-managed worktree found on disk.
#[derive(Debug)]
struct AgentWorktree {
    worktree_path: PathBuf,
    created: std::time::SystemTime,
}

/// List agent-managed worktrees under `.agent-worktrees/`, sorted oldest-first.
async fn list_agent_worktrees(project_path: &Path) -> Result<Vec<AgentWorktree>, WorktreeError> {
    let agent_wt_root = project_path.join(".agent-worktrees");

    if !agent_wt_root.exists() {
        return Ok(Vec::new());
    }

    // Get git-registered worktrees to cross-reference
    let git_worktrees = git_ops::worktree_list(project_path).await.unwrap_or_default();
    let git_paths: std::collections::HashSet<PathBuf> = git_worktrees.iter()
        .map(|e| e.path.clone())
        .collect();

    let mut agent_wts = Vec::new();
    let mut entries = tokio::fs::read_dir(&agent_wt_root).await?;

    while let Some(entry) = entries.next_entry().await? {
        let path = entry.path();
        if !path.is_dir() {
            continue;
        }

        // Only count worktrees that git knows about (or that exist on disk)
        let canonical = path.canonicalize().unwrap_or_else(|_| path.clone());
        let is_git_registered = git_paths.contains(&canonical) || git_paths.contains(&path);

        // Include if git-registered or if the directory simply exists (could be orphaned)
        if is_git_registered || path.exists() {
            let created = entry.metadata().await
                .and_then(|m| m.created().or_else(|_| m.modified()))
                .unwrap_or(std::time::SystemTime::UNIX_EPOCH);

            agent_wts.push(AgentWorktree {
                worktree_path: canonical,
                created,
            });
        }
    }

    // Sort oldest first
    agent_wts.sort_by_key(|w| w.created);
    Ok(agent_wts)
}

/// Convert text to a URL-safe slug, limited to `max_len` characters.
fn slugify(text: &str, max_len: usize) -> String {
    let slug: String = text
        .to_lowercase()
        .chars()
        .map(|c| {
            if c.is_ascii_alphanumeric() {
                c
            } else {
                '-'
            }
        })
        .collect();

    // Collapse multiple hyphens
    let mut result = String::new();
    let mut last_was_hyphen = false;
    for c in slug.chars() {
        if c == '-' {
            if !last_was_hyphen && !result.is_empty() {
                result.push(c);
                last_was_hyphen = true;
            }
        } else {
            result.push(c);
            last_was_hyphen = false;
        }
    }

    // Trim trailing hyphens
    let result = result.trim_end_matches('-').to_string();

    // Truncate to max_len
    if result.len() > max_len {
        result[..max_len].trim_end_matches('-').to_string()
    } else {
        result
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use tempfile::tempdir;
    use tokio::process::Command;

    async fn init_repo(dir: &Path) {
        Command::new("git")
            .args(["init"])
            .current_dir(dir)
            .output()
            .await
            .unwrap();
        Command::new("git")
            .args(["config", "user.email", "test@test.com"])
            .current_dir(dir)
            .output()
            .await
            .unwrap();
        Command::new("git")
            .args(["config", "user.name", "Test"])
            .current_dir(dir)
            .output()
            .await
            .unwrap();
        std::fs::write(dir.join("README.md"), "# Test").unwrap();
        Command::new("git")
            .args(["add", "-A"])
            .current_dir(dir)
            .output()
            .await
            .unwrap();
        Command::new("git")
            .args(["commit", "-m", "initial"])
            .current_dir(dir)
            .output()
            .await
            .unwrap();
    }

    #[test]
    fn test_generate_branch_name_format() {
        let name = generate_branch_name("Fix the login bug");
        assert!(name.starts_with("agent/fix-the-login-bug-"), "Got: {name}");
        // UUID part should be 8 chars
        let parts: Vec<&str> = name.rsplitn(2, '-').collect();
        assert_eq!(parts[0].len(), 8);
    }

    #[test]
    fn test_slugify_special_chars() {
        assert_eq!(slugify("Hello, World! 123", 50), "hello-world-123");
    }

    #[test]
    fn test_slugify_length_limit() {
        let result = slugify("this is a very long task summary that exceeds the limit", 20);
        assert!(result.len() <= 20, "Got len {}: {result}", result.len());
        assert!(!result.ends_with('-'));
    }

    #[test]
    fn test_worktree_path_format() {
        let path = worktree_path(Path::new("/home/user/project"), "abc123def456xyz");
        assert_eq!(
            path,
            PathBuf::from("/home/user/project/.agent-worktrees/abc123def456xyz")
        );
    }

    #[tokio::test]
    async fn test_create_and_delete_worktree() {
        let dir = tempdir().unwrap();
        init_repo(dir.path()).await;

        let info = create_worktree(dir.path(), "test-session-id", None, "Fix login bug", None)
            .await
            .unwrap();

        assert!(info.worktree_path.exists(), "Worktree should exist");
        assert!(info.branch.starts_with("agent/"));

        // Verify it's a valid git worktree
        let status = Command::new("git")
            .args(["status"])
            .current_dir(&info.worktree_path)
            .output()
            .await
            .unwrap();
        assert!(status.status.success());

        // Delete
        delete_worktree(&info).await.unwrap();
        assert!(!info.worktree_path.exists(), "Worktree should be gone");
    }

    #[tokio::test]
    async fn test_check_worktree_state_clean() {
        let dir = tempdir().unwrap();
        init_repo(dir.path()).await;

        let _info = create_worktree(dir.path(), "clean-session", None, "Clean task", None)
            .await
            .unwrap();

        let state = check_worktree_state(dir.path(), "clean-session").await.unwrap();
        assert_eq!(state, WorktreeState::Clean);
    }

    #[tokio::test]
    async fn test_check_worktree_state_dirty() {
        let dir = tempdir().unwrap();
        init_repo(dir.path()).await;

        let info = create_worktree(dir.path(), "dirty-session", None, "Dirty task", None)
            .await
            .unwrap();

        // Create an uncommitted file in the worktree
        std::fs::write(info.worktree_path.join("new_file.txt"), "dirty").unwrap();

        let state = check_worktree_state(dir.path(), "dirty-session").await.unwrap();
        assert_eq!(state, WorktreeState::Dirty);
    }

    #[tokio::test]
    async fn test_check_worktree_state_missing() {
        let dir = tempdir().unwrap();
        init_repo(dir.path()).await;

        let state = check_worktree_state(dir.path(), "nonexistent-session").await.unwrap();
        assert_eq!(state, WorktreeState::Missing);
    }

    #[tokio::test]
    async fn test_prune_stale_worktrees() {
        let dir = tempdir().unwrap();
        init_repo(dir.path()).await;

        // Should not error even with no stale worktrees
        prune_stale_worktrees(dir.path()).await.unwrap();
    }

    #[tokio::test]
    async fn test_check_worktree_state_returns_error_on_git_failure() {
        // Use a separate temp dir (outside any git repo) so git status actually fails
        let isolated = tempdir().unwrap();
        let fake_project = isolated.path().join("project");
        std::fs::create_dir_all(&fake_project).unwrap();

        // Create the worktree path so it exists, but it's not a git repo at all
        let fake_wt = fake_project.join(".agent-worktrees").join("broken-session");
        std::fs::create_dir_all(&fake_wt).unwrap();
        std::fs::write(fake_wt.join("not-a-repo.txt"), "hello").unwrap();

        let result = check_worktree_state(&fake_project, "broken-session").await;
        // Should return an error (not Missing), because the path exists but git status fails
        assert!(result.is_err(), "Expected error for non-git directory, got: {:?}", result);
    }

    #[tokio::test]
    async fn test_evicts_oldest_when_limit_reached() {
        let dir = tempdir().unwrap();
        init_repo(dir.path()).await;

        // Create MAX_AGENT_WORKTREES worktrees
        let mut infos = Vec::new();
        for i in 0..MAX_AGENT_WORKTREES {
            let info = create_worktree(dir.path(), &format!("evict-{i}"), None, &format!("task {i}"), None)
                .await
                .unwrap();
            infos.push(info);
            // Small delay so filesystem timestamps differ
            tokio::time::sleep(std::time::Duration::from_millis(50)).await;
        }

        // All should exist
        for info in &infos {
            assert!(info.worktree_path.exists(), "Worktree {} should exist", info.session_id);
        }

        // Create one more — should evict the oldest (evict-0)
        let new_info = create_worktree(dir.path(), "evict-new", None, "new task", None)
            .await
            .unwrap();

        assert!(new_info.worktree_path.exists(), "New worktree should exist");

        // The oldest worktree should have been removed
        // (canonicalize might differ, so check the session dir name)
        let agent_wt_root = dir.path().join(".agent-worktrees");
        assert!(!agent_wt_root.join("evict-0").exists(), "Oldest worktree (evict-0) should have been evicted");

        // Total count should be at most MAX_AGENT_WORKTREES
        let remaining = list_agent_worktrees(dir.path()).await.unwrap();
        assert!(
            remaining.len() <= MAX_AGENT_WORKTREES,
            "Expected at most {} worktrees, found {}",
            MAX_AGENT_WORKTREES,
            remaining.len()
        );
    }

    #[tokio::test]
    async fn test_no_eviction_when_below_limit() {
        let dir = tempdir().unwrap();
        init_repo(dir.path()).await;

        // Create 2 worktrees (well below limit)
        let info1 = create_worktree(dir.path(), "below-1", None, "task 1", None)
            .await
            .unwrap();
        let info2 = create_worktree(dir.path(), "below-2", None, "task 2", None)
            .await
            .unwrap();
        let info3 = create_worktree(dir.path(), "below-3", None, "task 3", None)
            .await
            .unwrap();

        // All 3 should still exist
        assert!(info1.worktree_path.exists());
        assert!(info2.worktree_path.exists());
        assert!(info3.worktree_path.exists());
    }

    #[tokio::test]
    async fn test_stale_worktree_pruned_before_limit_check() {
        let dir = tempdir().unwrap();
        init_repo(dir.path()).await;

        // Create a worktree, then manually delete its directory
        let info = create_worktree(dir.path(), "stale-session", None, "stale task", None)
            .await
            .unwrap();
        std::fs::remove_dir_all(&info.worktree_path).unwrap();

        // Creating another should succeed (stale entry gets pruned, doesn't count toward limit)
        let info2 = create_worktree(dir.path(), "fresh-session", None, "fresh task", None)
            .await
            .unwrap();
        assert!(info2.worktree_path.exists());
    }
}
