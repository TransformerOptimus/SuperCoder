use async_trait::async_trait;
use serde_json::{json, Value};

use crate::error::ToolError;
use crate::types::AgentEvent;
use super::{Tool, ToolContext, ToolResult};

pub struct PrTool;

#[async_trait]
impl Tool for PrTool {
    fn name(&self) -> &str {
        "create_pr"
    }

    fn description(&self) -> &str {
        "Create a GitHub pull request. Requires the repository to have a GitHub remote \
         and a valid GitHub auth token (GITHUB_TOKEN env var or gh CLI)."
    }

    fn parameters_schema(&self) -> Value {
        json!({
            "type": "object",
            "required": ["title", "body", "branch"],
            "properties": {
                "title": {
                    "type": "string",
                    "description": "The title of the pull request"
                },
                "body": {
                    "type": "string",
                    "description": "The body/description of the pull request (markdown supported)"
                },
                "branch": {
                    "type": "string",
                    "description": "The head branch to create the PR from"
                },
                "base": {
                    "type": "string",
                    "description": "The base branch to merge into (default: main)"
                }
            }
        })
    }

    async fn execute(&self, args: Value, ctx: &ToolContext) -> Result<ToolResult, ToolError> {
        let title = args
            .get("title")
            .and_then(|v| v.as_str())
            .ok_or_else(|| ToolError("Missing required parameter: title".into()))?;

        let body = args
            .get("body")
            .and_then(|v| v.as_str())
            .ok_or_else(|| ToolError("Missing required parameter: body".into()))?;

        let branch = args
            .get("branch")
            .and_then(|v| v.as_str())
            .ok_or_else(|| ToolError("Missing required parameter: branch".into()))?;

        let base = args
            .get("base")
            .and_then(|v| v.as_str())
            .unwrap_or("main");

        // Emit status event
        let _ = ctx
            .event_tx
            .send(AgentEvent::ToolStatus {
                session_id: ctx.session_id.clone(),
                tool_call_id: ctx.tool_call_id.clone(),
                status: format!("Creating PR: {title}"),
            })
            .await;

        match git_ops::pr::create(&ctx.working_dir, title, body, branch, base).await {
            Ok(pr) => Ok(ToolResult::success(format!("Pull request created: {}\nPR #{}", pr.url, pr.number))),
            Err(git_ops::GitOpsError::NoAuthToken) => Ok(ToolResult::error(
                "Error: No GitHub auth token found. Set GITHUB_TOKEN environment \
                 variable or authenticate with `gh auth login`.",
            )),
            Err(git_ops::GitOpsError::GitHubApi { status, body }) => {
                // Log the full body for debugging but don't expose it to the LLM —
                // GitHub error responses can contain token scopes or OAuth details.
                log::warn!("GitHub API error (HTTP {status}): {body}");
                Ok(ToolResult::error(format!(
                    "Error: GitHub API returned HTTP {status}. \
                     Check that the branch is pushed and the token has repo scope."
                )))
            }
            Err(git_ops::GitOpsError::InvalidRemoteUrl(url)) => Ok(ToolResult::error(
                format!("Error: Could not parse GitHub remote URL: {url}"),
            )),
            Err(git_ops::GitOpsError::GitCommand { stderr, .. })
                if stderr.contains("not found on remote") =>
            {
                Ok(ToolResult::error(format!("Error: {stderr}")))
            }
            Err(e) => {
                // Log full error internally; only return a safe summary to the LLM
                // to avoid leaking connection details or headers.
                log::warn!("PR creation error: {e}");
                Ok(ToolResult::error("Error creating PR. Check git remote, network, and auth configuration."))
            }
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::sync::Mutex;
    use tempfile::tempdir;

    /// Mutex to serialize tests that mutate the GITHUB_TOKEN env var.
    static ENV_MUTEX: Mutex<()> = Mutex::new(());

    #[tokio::test]
    async fn test_missing_title() {
        let dir = tempdir().unwrap();
        let tool = PrTool;
        let ctx = ToolContext::test_context(dir.path());

        let result = tool
            .execute(
                json!({"body": "desc", "branch": "feat"}),
                &ctx,
            )
            .await;

        assert!(result.is_err());
    }

    #[tokio::test]
    async fn test_missing_body() {
        let dir = tempdir().unwrap();
        let tool = PrTool;
        let ctx = ToolContext::test_context(dir.path());

        let result = tool
            .execute(
                json!({"title": "PR", "branch": "feat"}),
                &ctx,
            )
            .await;

        assert!(result.is_err());
    }

    #[tokio::test]
    async fn test_missing_branch() {
        let dir = tempdir().unwrap();
        let tool = PrTool;
        let ctx = ToolContext::test_context(dir.path());

        let result = tool
            .execute(
                json!({"title": "PR", "body": "desc"}),
                &ctx,
            )
            .await;

        assert!(result.is_err());
    }

    #[tokio::test]
    async fn test_no_auth_token_graceful() {
        // Hold mutex to prevent other tests from seeing our env var mutation
        let _lock = ENV_MUTEX.lock().unwrap_or_else(|e| e.into_inner());

        let dir = tempdir().unwrap();

        // Init a git repo with a fake remote
        tokio::process::Command::new("git")
            .args(["init"])
            .current_dir(dir.path())
            .output()
            .await
            .unwrap();
        tokio::process::Command::new("git")
            .args(["config", "user.email", "test@test.com"])
            .current_dir(dir.path())
            .output()
            .await
            .unwrap();
        tokio::process::Command::new("git")
            .args(["config", "user.name", "Test"])
            .current_dir(dir.path())
            .output()
            .await
            .unwrap();
        tokio::process::Command::new("git")
            .args([
                "remote",
                "add",
                "origin",
                "https://github.com/test/repo.git",
            ])
            .current_dir(dir.path())
            .output()
            .await
            .unwrap();

        // Ensure GITHUB_TOKEN is not set for this test
        let original = std::env::var("GITHUB_TOKEN").ok();
        std::env::remove_var("GITHUB_TOKEN");

        let tool = PrTool;
        let ctx = ToolContext::test_context(dir.path());

        let result = tool
            .execute(
                json!({
                    "title": "Test PR",
                    "body": "Test body",
                    "branch": "feature"
                }),
                &ctx,
            )
            .await
            .unwrap();

        // Should return an error about auth, not panic
        assert!(result.is_error);
        assert!(
            result.output.contains("auth") || result.output.contains("Error"),
            "Expected auth error, got: {}",
            result.output
        );

        // Restore
        if let Some(val) = original {
            std::env::set_var("GITHUB_TOKEN", val);
        }
    }

    #[tokio::test]
    async fn test_invalid_remote_url_error() {
        let dir = tempdir().unwrap();

        // Init a git repo with a non-GitHub remote
        tokio::process::Command::new("git")
            .args(["init"])
            .current_dir(dir.path())
            .output()
            .await
            .unwrap();
        tokio::process::Command::new("git")
            .args(["config", "user.email", "test@test.com"])
            .current_dir(dir.path())
            .output()
            .await
            .unwrap();
        tokio::process::Command::new("git")
            .args(["config", "user.name", "Test"])
            .current_dir(dir.path())
            .output()
            .await
            .unwrap();
        tokio::process::Command::new("git")
            .args([
                "remote",
                "add",
                "origin",
                "https://gitlab.com/user/repo.git",
            ])
            .current_dir(dir.path())
            .output()
            .await
            .unwrap();

        let tool = PrTool;
        let ctx = ToolContext::test_context(dir.path());

        let result = tool
            .execute(
                json!({
                    "title": "Test PR",
                    "body": "Test body",
                    "branch": "feature"
                }),
                &ctx,
            )
            .await
            .unwrap();

        assert!(result.is_error);
        assert!(
            result.output.contains("remote URL"),
            "Expected remote URL error, got: {}",
            result.output
        );
    }

    #[tokio::test]
    async fn test_branch_not_pushed_error() {
        // Test at the git-ops layer directly — the PR tool wraps this
        // We can't test through PrTool::execute with a fake remote because
        // ls-remote would try to connect to the network.
        // Instead, verify the branch_exists_on_remote function works correctly
        // with a local "remote" (a bare repo).
        let dir = tempdir().unwrap();
        let bare_dir = dir.path().join("bare.git");
        let work_dir = dir.path().join("work");

        // Create a bare repo as "remote"
        tokio::process::Command::new("git")
            .args(["init", "--bare"])
            .arg(&bare_dir)
            .output()
            .await
            .unwrap();

        // Create a working repo pointing to the bare repo
        tokio::process::Command::new("git")
            .args(["clone"])
            .arg(&bare_dir)
            .arg(&work_dir)
            .output()
            .await
            .unwrap();
        tokio::process::Command::new("git")
            .args(["config", "user.email", "test@test.com"])
            .current_dir(&work_dir)
            .output()
            .await
            .unwrap();
        tokio::process::Command::new("git")
            .args(["config", "user.name", "Test"])
            .current_dir(&work_dir)
            .output()
            .await
            .unwrap();

        // Create an initial commit and push
        std::fs::write(work_dir.join("README.md"), "# Test").unwrap();
        tokio::process::Command::new("git")
            .args(["add", "-A"])
            .current_dir(&work_dir)
            .output()
            .await
            .unwrap();
        tokio::process::Command::new("git")
            .args(["commit", "-m", "initial"])
            .current_dir(&work_dir)
            .output()
            .await
            .unwrap();

        // Detect the default branch name (master or main)
        let branch_output = tokio::process::Command::new("git")
            .args(["rev-parse", "--abbrev-ref", "HEAD"])
            .current_dir(&work_dir)
            .output()
            .await
            .unwrap();
        let default_branch = String::from_utf8_lossy(&branch_output.stdout).trim().to_string();

        tokio::process::Command::new("git")
            .args(["push", "-u", "origin", &default_branch])
            .current_dir(&work_dir)
            .output()
            .await
            .unwrap();

        // branch_exists_on_remote should return false for a non-existent branch
        let exists =
            git_ops::pr::branch_exists_on_remote(&work_dir, "nonexistent-branch", "origin")
                .await
                .unwrap();
        assert!(!exists, "Branch should not exist on remote");

        // And true for the pushed branch
        let exists =
            git_ops::pr::branch_exists_on_remote(&work_dir, &default_branch, "origin")
                .await
                .unwrap();
        assert!(exists, "{default_branch} should exist on remote");
    }

    #[tokio::test]
    async fn test_no_remote_error() {
        let dir = tempdir().unwrap();

        // Init a git repo with NO remote at all
        tokio::process::Command::new("git")
            .args(["init"])
            .current_dir(dir.path())
            .output()
            .await
            .unwrap();
        tokio::process::Command::new("git")
            .args(["config", "user.email", "test@test.com"])
            .current_dir(dir.path())
            .output()
            .await
            .unwrap();
        tokio::process::Command::new("git")
            .args(["config", "user.name", "Test"])
            .current_dir(dir.path())
            .output()
            .await
            .unwrap();

        let tool = PrTool;
        let ctx = ToolContext::test_context(dir.path());

        let result = tool
            .execute(
                json!({
                    "title": "Test PR",
                    "body": "Test body",
                    "branch": "feature"
                }),
                &ctx,
            )
            .await
            .unwrap();

        // Should gracefully error, not panic
        assert!(result.is_error);
    }

    #[tokio::test]
    #[ignore] // Requires real GitHub token and repo
    async fn test_create_pr_real() {
        // This test would need a real repo with a pushed branch
    }
}
