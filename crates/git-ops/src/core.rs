use std::path::Path;

use crate::error::GitOpsError;
use crate::exec::run_git;
use crate::types::*;

/// Get repository status.
pub async fn status(repo_path: &Path) -> Result<StatusOutput, GitOpsError> {
    let output = run_git(repo_path, &["status", "--porcelain=v2", "--branch"]).await?;
    let raw = output.stdout.clone();

    let mut branch = String::new();
    let mut ahead: u32 = 0;
    let mut behind: u32 = 0;
    let mut staged = Vec::new();
    let mut unstaged = Vec::new();
    let mut untracked = Vec::new();

    for line in raw.lines() {
        if line.starts_with("# branch.head ") {
            branch = line.strip_prefix("# branch.head ").unwrap_or("").to_string();
        } else if line.starts_with("# branch.ab ") {
            // Format: # branch.ab +N -M
            let parts: Vec<&str> = line.split_whitespace().collect();
            if parts.len() >= 4 {
                ahead = parts[2]
                    .strip_prefix('+')
                    .and_then(|s| s.parse().ok())
                    .unwrap_or(0);
                behind = parts[3]
                    .strip_prefix('-')
                    .and_then(|s| s.parse().ok())
                    .unwrap_or(0);
            }
        } else if line.starts_with("1 ") || line.starts_with("2 ") {
            // Changed entries: "1 XY sub mH mI mW hH hI path" or "2 XY ... path\torigPath"
            let parts: Vec<&str> = line.splitn(9, ' ').collect();
            if parts.len() >= 9 {
                let xy = parts[1];
                let x = &xy[0..1]; // staged status
                let y = &xy[1..2]; // unstaged status

                // For rename entries (prefix "2"), path may contain a tab
                let path = if line.starts_with("2 ") {
                    parts[8].split('\t').next().unwrap_or(parts[8])
                } else {
                    parts[8]
                };

                if x != "." {
                    staged.push(FileStatus {
                        path: path.to_string(),
                        status: x.to_string(),
                    });
                }
                if y != "." {
                    unstaged.push(FileStatus {
                        path: path.to_string(),
                        status: y.to_string(),
                    });
                }
            }
        } else if line.starts_with("? ") {
            // Untracked: "? path"
            let path = line.strip_prefix("? ").unwrap_or("");
            untracked.push(FileStatus {
                path: path.to_string(),
                status: "?".to_string(),
            });
        }
    }

    Ok(StatusOutput {
        branch,
        ahead,
        behind,
        staged,
        unstaged,
        untracked,
        raw,
    })
}

/// Get diff output.
pub async fn diff(
    repo_path: &Path,
    files: Option<&[&str]>,
    staged: bool,
    ref_range: Option<&str>,
) -> Result<DiffOutput, GitOpsError> {
    // Get the raw diff
    let mut diff_args = vec!["diff"];
    if let Some(range) = ref_range {
        diff_args.push(range);
    } else if staged {
        diff_args.push("--cached");
    }
    if let Some(files) = files {
        diff_args.push("--");
        diff_args.extend_from_slice(files);
    }
    let diff_output = run_git(repo_path, &diff_args).await?;

    // Get the stat summary
    let mut stat_args = vec!["diff", "--stat"];
    if let Some(range) = ref_range {
        stat_args.push(range);
    } else if staged {
        stat_args.push("--cached");
    }
    if let Some(files) = files {
        stat_args.push("--");
        stat_args.extend_from_slice(files);
    }
    let stat_output = run_git(repo_path, &stat_args).await?;

    // Parse stat summary from the last line, e.g. " 2 files changed, 3 insertions(+), 1 deletion(-)"
    let stat_text = stat_output.stdout.trim().to_string();
    let (files_changed, insertions, deletions) = parse_diff_stat(&stat_text);

    Ok(DiffOutput {
        diff: diff_output.stdout,
        files_changed,
        insertions,
        deletions,
        stat: stat_text,
    })
}

fn parse_diff_stat(stat: &str) -> (u32, u32, u32) {
    let last_line = stat.lines().last().unwrap_or("");

    fn extract_number(line: &str, keyword: &str) -> u32 {
        line.find(keyword)
            .and_then(|idx| line[..idx].trim().split_whitespace().last())
            .and_then(|s| s.parse().ok())
            .unwrap_or(0)
    }

    (
        extract_number(last_line, "file"),
        extract_number(last_line, "insertion"),
        extract_number(last_line, "deletion"),
    )
}

/// Commit changes.
pub async fn commit(
    repo_path: &Path,
    message: &str,
    files: Option<&[&str]>,
) -> Result<CommitOutput, GitOpsError> {
    // Stage files
    if let Some(files) = files {
        let mut add_args = vec!["add", "--"];
        add_args.extend_from_slice(files);
        run_git(repo_path, &add_args).await?;
    } else {
        run_git(repo_path, &["add", "-A"]).await?;
    }

    // Commit
    run_git(repo_path, &["commit", "-m", message]).await?;

    // Get the short SHA
    let sha_output = run_git(repo_path, &["rev-parse", "--short", "HEAD"]).await?;
    let sha = sha_output.stdout.trim().to_string();

    // Get files changed count from the commit
    let show_output = run_git(repo_path, &["show", "--stat", "--format=", "HEAD"]).await?;
    let (files_changed, _, _) = parse_diff_stat(&show_output.stdout);

    Ok(CommitOutput {
        sha,
        message: message.to_string(),
        files_changed,
    })
}

/// Push to remote.
pub async fn push(
    repo_path: &Path,
    branch: Option<&str>,
    remote: Option<&str>,
) -> Result<PushOutput, GitOpsError> {
    let remote = remote.unwrap_or("origin");
    let mut push_args = vec!["push", remote];
    if let Some(branch) = branch {
        push_args.push(branch);
    }

    let output = run_git(repo_path, &push_args).await?;

    // If no branch was specified, resolve the current branch name so the
    // output accurately reflects what was actually pushed.
    let resolved_branch = match branch {
        Some(b) => b.to_string(),
        None => run_git(repo_path, &["rev-parse", "--abbrev-ref", "HEAD"])
            .await
            .map(|o| o.stdout.trim().to_string())
            .unwrap_or_default(),
    };

    Ok(PushOutput {
        remote: remote.to_string(),
        branch: resolved_branch,
        raw: format!("{}{}", output.stdout, output.stderr),
    })
}

/// List branches.
pub async fn branches(repo_path: &Path) -> Result<Vec<BranchInfo>, GitOpsError> {
    let output = run_git(
        repo_path,
        &[
            "branch",
            "-a",
            "--format=%(if)%(HEAD)%(then)*%(else) %(end)\t%(refname:short)\t%(upstream:short)",
        ],
    )
    .await?;

    let mut result = Vec::new();
    for line in output.stdout.lines() {
        let parts: Vec<&str> = line.split('\t').collect();
        if parts.len() >= 2 {
            let is_current = parts[0].trim() == "*";
            let name = parts[1].to_string();
            let upstream = parts
                .get(2)
                .filter(|s| !s.is_empty())
                .map(|s| s.to_string());
            result.push(BranchInfo {
                name,
                is_current,
                upstream,
            });
        }
    }

    Ok(result)
}

/// Create a new branch.
pub async fn create_branch(
    repo_path: &Path,
    name: &str,
    from: Option<&str>,
) -> Result<(), GitOpsError> {
    let mut args = vec!["checkout", "-b", name];
    if let Some(from) = from {
        args.push(from);
    }
    run_git(repo_path, &args).await?;
    Ok(())
}

/// Switch to an existing branch.
pub async fn switch_branch(repo_path: &Path, name: &str) -> Result<(), GitOpsError> {
    run_git(repo_path, &["checkout", name]).await?;
    Ok(())
}

/// Get git log.
pub async fn log(repo_path: &Path, limit: Option<u32>) -> Result<Vec<LogEntry>, GitOpsError> {
    let limit_str = limit.unwrap_or(20).to_string();
    let output = run_git(
        repo_path,
        &["log", &format!("--format=%h\t%s\t%an\t%aI"), "-n", &limit_str],
    )
    .await?;

    let mut entries = Vec::new();
    for line in output.stdout.lines() {
        if line.is_empty() {
            continue;
        }
        let parts: Vec<&str> = line.splitn(4, '\t').collect();
        if parts.len() >= 4 {
            entries.push(LogEntry {
                sha: parts[0].to_string(),
                message: parts[1].to_string(),
                author: parts[2].to_string(),
                date: parts[3].to_string(),
            });
        }
    }

    Ok(entries)
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::fs;
    use tempfile::tempdir;
    use tokio::process::Command;
    use crate::test_util::init_repo_with_commit;

    // ── status tests ──

    #[tokio::test]
    async fn test_status_clean_repo() {
        let dir = tempdir().unwrap();
        init_repo_with_commit(dir.path()).await;

        let s = status(dir.path()).await.unwrap();
        assert!(s.staged.is_empty());
        assert!(s.unstaged.is_empty());
        assert!(s.untracked.is_empty());
    }

    #[tokio::test]
    async fn test_status_branch_name() {
        let dir = tempdir().unwrap();
        init_repo_with_commit(dir.path()).await;

        let s = status(dir.path()).await.unwrap();
        // Default branch could be "main" or "master" depending on git config
        assert!(!s.branch.is_empty());
    }

    #[tokio::test]
    async fn test_status_modified_file() {
        let dir = tempdir().unwrap();
        init_repo_with_commit(dir.path()).await;

        fs::write(dir.path().join("README.md"), "# Modified").unwrap();
        let s = status(dir.path()).await.unwrap();
        assert_eq!(s.unstaged.len(), 1);
        assert_eq!(s.unstaged[0].path, "README.md");
        assert_eq!(s.unstaged[0].status, "M");
    }

    #[tokio::test]
    async fn test_status_staged_file() {
        let dir = tempdir().unwrap();
        init_repo_with_commit(dir.path()).await;

        fs::write(dir.path().join("README.md"), "# Staged").unwrap();
        Command::new("git")
            .args(["add", "README.md"])
            .current_dir(dir.path())
            .output()
            .await
            .unwrap();

        let s = status(dir.path()).await.unwrap();
        assert_eq!(s.staged.len(), 1);
        assert_eq!(s.staged[0].status, "M");
    }

    #[tokio::test]
    async fn test_status_untracked_file() {
        let dir = tempdir().unwrap();
        init_repo_with_commit(dir.path()).await;

        fs::write(dir.path().join("new.txt"), "new file").unwrap();
        let s = status(dir.path()).await.unwrap();
        assert_eq!(s.untracked.len(), 1);
        assert_eq!(s.untracked[0].path, "new.txt");
    }

    // ── diff tests ──

    #[tokio::test]
    async fn test_diff_unstaged_changes() {
        let dir = tempdir().unwrap();
        init_repo_with_commit(dir.path()).await;

        fs::write(dir.path().join("README.md"), "# Modified\nNew line").unwrap();
        let d = diff(dir.path(), None, false, None).await.unwrap();
        assert!(!d.diff.is_empty());
        assert!(d.diff.contains("Modified"));
        assert_eq!(d.files_changed, 1);
    }

    #[tokio::test]
    async fn test_diff_staged_changes() {
        let dir = tempdir().unwrap();
        init_repo_with_commit(dir.path()).await;

        fs::write(dir.path().join("README.md"), "# Staged change").unwrap();
        Command::new("git")
            .args(["add", "README.md"])
            .current_dir(dir.path())
            .output()
            .await
            .unwrap();

        let d = diff(dir.path(), None, true, None).await.unwrap();
        assert!(!d.diff.is_empty());
        assert!(d.diff.contains("Staged change"));
    }

    #[tokio::test]
    async fn test_diff_specific_files() {
        let dir = tempdir().unwrap();
        init_repo_with_commit(dir.path()).await;

        fs::write(dir.path().join("README.md"), "# Changed").unwrap();
        fs::write(dir.path().join("other.txt"), "other").unwrap();
        Command::new("git")
            .args(["add", "other.txt"])
            .current_dir(dir.path())
            .output()
            .await
            .unwrap();
        Command::new("git")
            .args(["commit", "-m", "add other"])
            .current_dir(dir.path())
            .output()
            .await
            .unwrap();
        fs::write(dir.path().join("other.txt"), "changed other").unwrap();

        let d = diff(dir.path(), Some(&["README.md"]), false, None).await.unwrap();
        assert!(d.diff.contains("Changed"));
        // Should not contain other.txt changes
        assert!(!d.diff.contains("changed other"));
    }

    // ── commit tests ──

    #[tokio::test]
    async fn test_commit_all_files() {
        let dir = tempdir().unwrap();
        init_repo_with_commit(dir.path()).await;

        fs::write(dir.path().join("new.txt"), "new").unwrap();
        let c = commit(dir.path(), "add new file", None).await.unwrap();
        assert!(!c.sha.is_empty());
        assert_eq!(c.message, "add new file");
        assert!(c.files_changed >= 1);
    }

    #[tokio::test]
    async fn test_commit_specific_files() {
        let dir = tempdir().unwrap();
        init_repo_with_commit(dir.path()).await;

        fs::write(dir.path().join("a.txt"), "a").unwrap();
        fs::write(dir.path().join("b.txt"), "b").unwrap();

        let c = commit(dir.path(), "add a only", Some(&["a.txt"])).await.unwrap();
        assert!(!c.sha.is_empty());

        // b.txt should still be untracked
        let s = status(dir.path()).await.unwrap();
        assert!(s.untracked.iter().any(|f| f.path == "b.txt"));
    }

    #[tokio::test]
    async fn test_commit_sha_returned() {
        let dir = tempdir().unwrap();
        init_repo_with_commit(dir.path()).await;

        fs::write(dir.path().join("file.txt"), "content").unwrap();
        let c = commit(dir.path(), "test sha", None).await.unwrap();
        // Short SHA should be 7+ chars
        assert!(c.sha.len() >= 7);
    }

    // ── branches tests ──

    #[tokio::test]
    async fn test_branches_single() {
        let dir = tempdir().unwrap();
        init_repo_with_commit(dir.path()).await;

        let b = branches(dir.path()).await.unwrap();
        assert_eq!(b.len(), 1);
        assert!(b[0].is_current);
    }

    #[tokio::test]
    async fn test_branches_created_branch_appears() {
        let dir = tempdir().unwrap();
        init_repo_with_commit(dir.path()).await;

        create_branch(dir.path(), "feature", None).await.unwrap();
        // Switch back to verify both exist
        // Switch back - try master first, then main
        if switch_branch(dir.path(), "master").await.is_err() {
            switch_branch(dir.path(), "main").await.unwrap();
        }

        let b = branches(dir.path()).await.unwrap();
        assert!(b.len() >= 2);
        assert!(b.iter().any(|br| br.name == "feature"));
    }

    // ── create/switch branch tests ──

    #[tokio::test]
    async fn test_create_and_switch_branch() {
        let dir = tempdir().unwrap();
        init_repo_with_commit(dir.path()).await;

        create_branch(dir.path(), "dev", None).await.unwrap();
        let s = status(dir.path()).await.unwrap();
        assert_eq!(s.branch, "dev");

        // Switch back - try master first, then main
        if switch_branch(dir.path(), "master").await.is_err() {
            switch_branch(dir.path(), "main").await.unwrap();
        }
        let s = status(dir.path()).await.unwrap();
        assert_ne!(s.branch, "dev");
    }

    // ── log tests ──

    #[tokio::test]
    async fn test_log_entries() {
        let dir = tempdir().unwrap();
        init_repo_with_commit(dir.path()).await;

        fs::write(dir.path().join("a.txt"), "a").unwrap();
        commit(dir.path(), "second commit", None).await.unwrap();

        let entries = log(dir.path(), None).await.unwrap();
        assert_eq!(entries.len(), 2);
        assert_eq!(entries[0].message, "second commit");
        assert_eq!(entries[1].message, "initial");
    }

    #[tokio::test]
    async fn test_log_with_limit() {
        let dir = tempdir().unwrap();
        init_repo_with_commit(dir.path()).await;

        fs::write(dir.path().join("a.txt"), "a").unwrap();
        commit(dir.path(), "second", None).await.unwrap();
        fs::write(dir.path().join("b.txt"), "b").unwrap();
        commit(dir.path(), "third", None).await.unwrap();

        let entries = log(dir.path(), Some(2)).await.unwrap();
        assert_eq!(entries.len(), 2);
        assert_eq!(entries[0].message, "third");
    }

    #[tokio::test]
    async fn test_log_entries_have_fields() {
        let dir = tempdir().unwrap();
        init_repo_with_commit(dir.path()).await;

        let entries = log(dir.path(), None).await.unwrap();
        assert!(!entries[0].sha.is_empty());
        assert!(!entries[0].author.is_empty());
        assert!(!entries[0].date.is_empty());
    }

    // ── push test ──

    #[tokio::test]
    async fn test_push_fails_without_remote() {
        let dir = tempdir().unwrap();
        init_repo_with_commit(dir.path()).await;

        let result = push(dir.path(), None, None).await;
        assert!(result.is_err());
    }

    // ── parse_diff_stat tests ──

    #[test]
    fn test_parse_diff_stat_full() {
        let stat = " 2 files changed, 10 insertions(+), 3 deletions(-)";
        let (f, i, d) = parse_diff_stat(stat);
        assert_eq!(f, 2);
        assert_eq!(i, 10);
        assert_eq!(d, 3);
    }

    #[test]
    fn test_parse_diff_stat_insertions_only() {
        let stat = " 1 file changed, 5 insertions(+)";
        let (f, i, d) = parse_diff_stat(stat);
        assert_eq!(f, 1);
        assert_eq!(i, 5);
        assert_eq!(d, 0);
    }

    #[test]
    fn test_parse_diff_stat_empty() {
        let (f, i, d) = parse_diff_stat("");
        assert_eq!(f, 0);
        assert_eq!(i, 0);
        assert_eq!(d, 0);
    }
}
