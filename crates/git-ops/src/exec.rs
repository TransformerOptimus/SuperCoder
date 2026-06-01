use std::path::Path;

use tokio::process::Command;

use crate::error::GitOpsError;
use crate::no_window::no_window_tokio;
use crate::types::GitOutput;

/// Run a git command in the given repo directory.
/// Returns `Err(GitCommand)` on non-zero exit code.
/// Used by typed functions (core.rs) where failure is exceptional.
pub async fn run_git(repo_path: &Path, args: &[&str]) -> Result<GitOutput, GitOpsError> {
    let output = run_git_inner(repo_path, args).await?;
    if output.exit_code != 0 {
        return Err(GitOpsError::GitCommand {
            exit_code: output.exit_code,
            stderr: output.stderr.clone(),
            stdout: output.stdout.clone(),
        });
    }
    Ok(output)
}

/// Run a git command in the given repo directory.
/// Always returns `Ok` with the exit code — even on non-zero exit.
/// Used by the agent git tool where non-zero exit is informational.
pub async fn run_git_raw(repo_path: &Path, args: &[&str]) -> Result<GitOutput, GitOpsError> {
    run_git_inner(repo_path, args).await
}

async fn run_git_inner(repo_path: &Path, args: &[&str]) -> Result<GitOutput, GitOpsError> {
    if !repo_path.exists() {
        return Err(GitOpsError::InvalidRepoPath(
            repo_path.display().to_string(),
        ));
    }

    let mut cmd = Command::new("git");
    cmd.args(args)
        .current_dir(repo_path)
        .stdout(std::process::Stdio::piped())
        .stderr(std::process::Stdio::piped());
    no_window_tokio(&mut cmd);
    let output = cmd.output().await.map_err(GitOpsError::GitNotFound)?;

    let exit_code = output.status.code().unwrap_or(-1);
    let stdout = String::from_utf8_lossy(&output.stdout).to_string();
    let stderr = String::from_utf8_lossy(&output.stderr).to_string();

    Ok(GitOutput {
        exit_code,
        stdout,
        stderr,
    })
}

#[cfg(test)]
mod tests {
    use super::*;
    use tempfile::tempdir;
    use crate::test_util::init_repo;

    #[tokio::test]
    async fn test_run_git_valid_repo() {
        let dir = tempdir().unwrap();
        init_repo(dir.path()).await;

        let output = run_git(dir.path(), &["status"]).await.unwrap_or_else(|e| panic!("{e}"));
        assert_eq!(output.exit_code, 0);
    }

    #[tokio::test]
    async fn test_run_git_invalid_path() {
        let result = run_git(Path::new("/nonexistent/path"), &["status"]).await;
        assert!(matches!(result, Err(GitOpsError::InvalidRepoPath(_))));
    }

    #[tokio::test]
    async fn test_run_git_nonzero_exit_is_error() {
        let dir = tempdir().unwrap();
        init_repo(dir.path()).await;

        // checkout a nonexistent branch should fail
        let result = run_git(dir.path(), &["checkout", "nonexistent-branch-xyz"]).await;
        assert!(matches!(result, Err(GitOpsError::GitCommand { .. })));
    }

    #[tokio::test]
    async fn test_run_git_raw_nonzero_exit_is_ok() {
        let dir = tempdir().unwrap();
        init_repo(dir.path()).await;

        let output = run_git_raw(dir.path(), &["checkout", "nonexistent-branch-xyz"])
            .await
            .unwrap();
        assert_ne!(output.exit_code, 0);
    }

    #[tokio::test]
    async fn test_run_git_raw_invalid_path() {
        let result = run_git_raw(Path::new("/nonexistent/path"), &["status"]).await;
        assert!(matches!(result, Err(GitOpsError::InvalidRepoPath(_))));
    }
}
